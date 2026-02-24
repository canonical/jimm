// Copyright 2026 Canonical.

package testing

import (
	"testing"
	"time"

	petname "github.com/dustinkirkland/golang-petname"
	qt "github.com/frankban/quicktest"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/juju/juju/api/base"
	cloudapi "github.com/juju/juju/api/client/cloud"
	"github.com/juju/juju/api/client/modelmanager"
	"github.com/juju/juju/core/model"
	"github.com/juju/names/v5"

	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
)

// caasModelManagerSuite requires additional setup, described in README.md#Setup microk8s cloud
type caasModelManagerDeps struct {
	jimmtest.WebsocketEnv

	cred      names.CloudCredentialTag
	cloudName string
}

func SetupCaasModelTest(c *qt.C) caasModelManagerDeps {
	s := jimmtest.SetupWebsocketEnv(c)
	deps := caasModelManagerDeps{
		WebsocketEnv: s,
	}

	conn := s.Open(c, nil, "bob@canonical.com", nil)
	defer conn.Close()
	cloudclient := cloudapi.NewClient(conn)
	cloud, credential := s.GetMicrok8sCloudAndCloudCredential(c)
	deps.cloudName = cloud.Name
	err := cloudclient.AddCloud(cloud, false)
	c.Assert(err, qt.Equals, nil)
	credentialName := petname.Generate(2, "-")
	deps.cred = names.NewCloudCredentialTag(deps.cloudName + "/bob@canonical.com/" + credentialName)
	s.UpdateCloudCredential(c, deps.cred, credential)

	c.Cleanup(func() {
		conn := s.Open(c, nil, "bob@canonical.com", nil)
		defer conn.Close()
		cloudclient := cloudapi.NewClient(conn)
		err := cloudclient.RemoveCloud(deps.cloudName)
		c.Check(err, qt.Equals, nil)
	})

	return deps
}

func TestCreateModelKubernetes(t *testing.T) {
	c := qt.New(t)
	s := SetupCaasModelTest(c)
	conn := s.Open(c, nil, "bob", nil)
	defer conn.Close()

	client := modelmanager.NewClient(conn)
	modelName := petname.Generate(2, "-")
	mi, err := client.CreateModel(modelName, "bob@canonical.com", s.cloudName, "", s.cred, nil)
	c.Assert(err, qt.Equals, nil)
	c.Cleanup(func() {
		s.DestroyModelAndDeleteFromDatabase(c, names.NewModelTag(mi.UUID))
	})
	c.Assert(mi.Name, qt.Equals, modelName)
	c.Assert(mi.Type, qt.Equals, model.CAAS)
	c.Assert(mi.ProviderType, qt.Equals, "kubernetes")
	c.Assert(mi.Cloud, qt.Equals, s.cloudName)
	c.Assert(mi.CloudRegion, qt.Equals, "localhost")
	c.Assert(mi.Owner, qt.Equals, "bob@canonical.com")
}

func TestListCAASModelSummaries(t *testing.T) {
	c := qt.New(t)
	s := SetupCaasModelTest(c)
	conn := s.Open(c, nil, "bob", nil)
	defer conn.Close()

	client := modelmanager.NewClient(conn)
	modelName := petname.Generate(2, "-")
	mi, err := client.CreateModel(modelName, "bob@canonical.com", s.cloudName, "", s.cred, nil)
	c.Assert(err, qt.Equals, nil)
	c.Cleanup(func() {
		s.DestroyModelAndDeleteFromDatabase(c, names.NewModelTag(mi.UUID))
	})

	models, err := client.ListModelSummaries("bob", false)
	c.Assert(err, qt.Equals, nil)

	var caasMS *base.UserModelSummary
	for _, m := range models {
		if m.Name == modelName {
			caasMS = &m
		}
	}
	if caasMS == nil {
		c.Fail()
	}
	expectedCaas := &base.UserModelSummary{
		Name:            modelName,
		UUID:            mi.UUID,
		Type:            "caas",
		ControllerUUID:  jimmtest.ControllerUUID,
		IsController:    false,
		ProviderType:    "kubernetes",
		DefaultSeries:   "jammy",
		Cloud:           s.cloudName,
		CloudRegion:     "localhost",
		CloudCredential: s.cloudName + "/bob@canonical.com/" + s.cred.Name(),
		Owner:           "bob@canonical.com",
		Life:            "alive",
		Status: base.Status{
			Status: "available",
			Info:   "",
			Data:   map[string]interface{}{},
			Since:  nil,
		},
		ModelUserAccess:    "admin",
		UserLastConnection: nil,
		Counts:             []base.EntityCount{},
		Error:              nil,
		Migration:          nil,
		SLA: &base.SLASummary{
			Level: "",
			Owner: "bob@canonical.com",
		},
	}
	c.Assert(
		caasMS,
		qt.CmpEquals(
			cmpopts.IgnoreFields(
				base.UserModelSummary{},
				"DefaultSeries",
				"AgentVersion",
			),
			cmpopts.IgnoreTypes(
				&time.Time{},
				&base.MigrationSummary{},
			),
		),
		expectedCaas,
	)
}

func TestListCAASModels(t *testing.T) {
	c := qt.New(t)
	s := SetupCaasModelTest(c)
	conn := s.Open(c, nil, "bob", nil)
	defer conn.Close()

	client := modelmanager.NewClient(conn)
	modelName := petname.Generate(2, "-")
	mi, err := client.CreateModel(modelName, "bob@canonical.com", s.cloudName, "", s.cred, nil)
	c.Assert(err, qt.Equals, nil)
	c.Cleanup(func() {
		s.DestroyModelAndDeleteFromDatabase(c, names.NewModelTag(mi.UUID))
	})

	models, err := client.ListModels("bob")
	c.Assert(err, qt.Equals, nil)
	c.Assert(
		models,
		qt.CmpEquals(
			cmpopts.IgnoreTypes(
				&time.Time{},
			),
			cmpopts.SortSlices(func(a, b base.UserModel) bool {
				return a.Name < b.Name
			}),
		),
		[]base.UserModel{
			{
				Name:  modelName,
				UUID:  mi.UUID,
				Owner: "bob@canonical.com",
				Type:  "caas",
			}, {
				Name:  s.Model.Name,
				UUID:  s.Model.UUID.String,
				Owner: "bob@canonical.com",
				Type:  "iaas",
			},
			{
				Name:  s.Model3.Name,
				UUID:  s.Model3.UUID.String,
				Owner: "charlie@canonical.com",
				Type:  "iaas",
			},
		},
	)
}
