// Copyright 2026 Canonical.

package testing

import (
	"testing"

	qt "github.com/frankban/quicktest"
	"github.com/juju/description/v10"
	"github.com/juju/juju/api/controller/migrationtarget"
	"github.com/juju/juju/core/migration"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v5"
	"github.com/juju/version/v2"

	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
	"github.com/canonical/jimm/v3/pkg/api"
	"github.com/canonical/jimm/v3/pkg/api/params"
)

func TestAbort(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupJimmWithControllers(c)

	conn := s.Open(c, nil, "alice", nil)
	defer conn.Close()

	modelUUID := "00000001-0000-0000-0000-000000000001"
	client := migrationtarget.NewClient(conn)
	err := client.Abort(modelUUID)
	c.Assert(err, qt.ErrorMatches, `.*model migration not found.*`)
}

func TestCheckMachines(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupJimmWithControllers(c)

	conn := s.Open(c, nil, "alice", nil)
	defer conn.Close()

	modelUUID := "00000001-0000-0000-0000-000000000001"
	client := migrationtarget.NewClient(conn)
	_, err := client.CheckMachines(modelUUID)
	c.Assert(err, qt.ErrorMatches, `.*model migration not found.*`)
}

func TestPrechecks(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupJimmWithControllers(c)

	conn := s.Open(c, nil, "alice", nil)
	defer conn.Close()

	cct := names.NewCloudCredentialTag(jimmtest.TestE2ECloudName + "/alice@canonical.com/cred")
	s.UpdateCloudCredential(c, cct, jujuparams.CloudCredential{
		AuthType: "empty",
	})

	modelDescriptionArgs := description.ModelArgs{
		Type:        description.IAAS,
		Owner:       names.NewUserTag("alice"),
		Cloud:       jimmtest.TestE2ECloudName,
		CloudRegion: jimmtest.TestE2ECloudRegionName,
	}
	modelUUID := "00000001-0000-0000-0000-000000000001"
	modelDescription := description.NewModel(modelDescriptionArgs)
	modelDescription.SetStatus(description.StatusArgs{Value: "available"})
	modelDescription.SetCloudCredential(description.CloudCredentialArgs{
		Name:  "cred",
		Cloud: names.NewCloudTag(jimmtest.TestE2ECloudName),
		Owner: names.NewUserTag("alice@canonical.com"),
	})
	model := migration.ModelInfo{
		UUID:                   modelUUID,
		Owner:                  names.NewUserTag("alice"),
		Name:                   "test-model",
		ControllerAgentVersion: version.MustParse("3.6.9"),
		AgentVersion:           version.MustParse("3.6.9"),
		ModelDescription:       modelDescription,
	}
	client := migrationtarget.NewClient(conn)
	err := client.Prechecks(model)
	c.Assert(err, qt.ErrorMatches, `.*model migration not found.*`)

	controllerName, _ := s.GetOneControllerConfig(c)

	prepareModelMigration := params.PrepareModelMigrationRequest{
		ModelTag:              names.NewModelTag(modelUUID).String(),
		BackingControllerName: controllerName,
		UserMapping:           map[string]string{"alice": "alice@canonical.com"},
	}
	jimmClient := api.NewClient(conn)
	migrationToken, err := jimmClient.PrepareModelMigration(&prepareModelMigration)
	c.Assert(err, qt.IsNil)
	c.Assert(migrationToken, qt.Not(qt.Equals), "")

	err = client.Prechecks(model)
	c.Assert(err, qt.IsNil)
}

func TestCACert(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupJimmWithControllers(c)

	conn := s.Open(c, nil, "alice", nil)
	defer conn.Close()

	client := migrationtarget.NewClient(conn)
	cert, err := client.CACert()
	c.Assert(err, qt.IsNil)
	c.Assert(cert, qt.Equals, "")
}

func TestAdoptResources(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupJimmWithControllers(c)

	conn := s.Open(c, nil, "alice", nil)
	defer conn.Close()

	modelUUID := "00000001-0000-0000-0000-000000000001"
	client := migrationtarget.NewClient(conn)
	err := client.AdoptResources(modelUUID)
	c.Assert(err, qt.ErrorMatches, `.*model not found.*`)
}

func TestActivate(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupJimmWithControllers(c)

	conn := s.Open(c, nil, "alice", nil)
	defer conn.Close()

	modelUUID := "00000001-0000-0000-0000-000000000001"
	sourceInfo := migration.SourceControllerInfo{
		ControllerTag: names.NewControllerTag("00000001-0000-0000-0000-000000000002"),
	}
	relatedModels := []string{"related-model-1", "related-model-2"}

	client := migrationtarget.NewClient(conn)
	err := client.Activate(modelUUID, sourceInfo, relatedModels)
	c.Assert(err, qt.ErrorMatches, `.*model migration not found.*`)
}

func TestLatestLogTime(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupJimmWithControllers(c)
	model := s.CreateModelForBob(c)

	conn := s.Open(c, nil, "alice", nil)
	defer conn.Close()

	client := migrationtarget.NewClient(conn)
	_, err := client.LatestLogTime(model.UUID.String)
	c.Assert(err, qt.IsNil)
}

func TestImport(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupJimmWithControllers(c)

	conn := s.Open(c, nil, "alice", nil)
	defer conn.Close()

	client := migrationtarget.NewClient(conn)
	err := client.Import([]byte{})
	c.Assert(err, qt.ErrorMatches, `^failed to import model.*`)
}
