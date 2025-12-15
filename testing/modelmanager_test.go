// Copyright 2025 Canonical.

package testing

import (
	"context"
	"time"

	petname "github.com/dustinkirkland/golang-petname"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/client/modelmanager"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs/config"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/openfga"
	ofganames "github.com/canonical/jimm/v3/internal/openfga/names"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
)

type modelE2EManagerSuite struct {
	jimmtest.WebsocketE2ESuite
}

var _ = gc.Suite(&modelE2EManagerSuite{})

func (s *modelE2EManagerSuite) TestListModelSummaries(c *gc.C) {
	conn := s.Open(c, nil, "bob", nil)
	defer conn.Close()

	client := modelmanager.NewClient(conn)

	s.DeployApplication(c, s.AdminUser, s.Model.Tag(), "test-app")

	models, err := client.ListModelSummaries("bob@canonical.com", true)
	c.Assert(err, gc.Equals, nil)
	c.Assert(models, jimmtest.CmpEquals(
		cmpopts.IgnoreTypes(&time.Time{}),
		cmpopts.IgnoreFields(base.UserModelSummary{}, "DefaultSeries", "AgentVersion"),
		// Ignore machine counts as they depend on timing
		cmpopts.IgnoreSliceElements(func(ec base.EntityCount) bool {
			return ec.Entity == "machines"
		}),
		cmpopts.SortSlices(func(a, b base.UserModelSummary) bool {
			return a.Name < b.Name
		}),
	), []base.UserModelSummary{{
		Name:            s.Model.Name,
		UUID:            s.Model.UUID.String,
		ControllerUUID:  jimmtest.ControllerUUID,
		ProviderType:    jimmtest.TestE2EProviderType,
		DefaultSeries:   "jammy",
		Cloud:           jimmtest.TestE2ECloudName,
		CloudRegion:     jimmtest.TestE2ECloudName,
		CloudCredential: jimmtest.TestE2ECloudName + "/bob@canonical.com/cred",
		Owner:           "bob@canonical.com",
		Life:            life.Value(state.Alive.String()),
		Status: base.Status{
			Status: status.Available,
			Data:   map[string]interface{}{},
		},
		ModelUserAccess: "admin",
		Counts:          []base.EntityCount{{Entity: "units", Count: 1}},
		Type:            "iaas",
		SLA: &base.SLASummary{
			Level: "",
			Owner: "bob@canonical.com",
		},
	}, {
		Name:            s.Model3.Name,
		UUID:            s.Model3.UUID.String,
		ControllerUUID:  jimmtest.ControllerUUID,
		ProviderType:    jimmtest.TestE2EProviderType,
		DefaultSeries:   "jammy",
		Cloud:           jimmtest.TestE2ECloudName,
		CloudRegion:     jimmtest.TestE2ECloudRegionName,
		CloudCredential: jimmtest.TestE2ECloudName + "/charlie@canonical.com/cred",
		Owner:           "charlie@canonical.com",
		Life:            life.Value(state.Alive.String()),
		Status: base.Status{
			Status: status.Available,
			Data:   map[string]interface{}{},
		},
		ModelUserAccess: "read",
		Counts:          []base.EntityCount{},
		Type:            "iaas",
		SLA: &base.SLASummary{
			Level: "",
			Owner: "charlie@canonical.com",
		},
	}})
}

func (s *modelE2EManagerSuite) TestListModelSummariesWithoutControllerUUIDMasking(c *gc.C) {
	conn1 := s.Open(c, nil, "charlie", nil)
	defer conn1.Close()
	err := conn1.APICall("JIMM", 4, "", "DisableControllerUUIDMasking", nil, nil)
	c.Assert(err, gc.ErrorMatches, `unauthorized \(unauthorized access\)`)
	// we need to make bob jimm admin to disable controller UUID masking
	err = s.OFGAClient.AddRelation(context.Background(),
		openfga.Tuple{
			Object:   ofganames.ConvertTag(names.NewUserTag("bob@canonical.com")),
			Relation: ofganames.AdministratorRelation,
			Target:   ofganames.ConvertTag(s.JIMM.ResourceTag()),
		},
	)
	c.Assert(err, gc.Equals, nil)

	conn := s.Open(c, nil, "bob", nil)
	defer conn.Close()

	err = conn.APICall("JIMM", 4, "", "DisableControllerUUIDMasking", nil, nil)
	c.Assert(err, gc.Equals, nil)

	// now that UUID masking has been disabled for the duration of this
	// connection, we can make bob a regular user again.
	err = s.OFGAClient.RemoveRelation(context.Background(),
		openfga.Tuple{
			Object:   ofganames.ConvertTag(names.NewUserTag("bob@canonical.com")),
			Relation: ofganames.AdministratorRelation,
			Target:   ofganames.ConvertTag(s.JIMM.ResourceTag()),
		},
	)
	c.Assert(err, gc.Equals, nil)

	client := modelmanager.NewClient(conn)
	models, err := client.ListModelSummaries("bob@canonical.com", false)
	c.Assert(err, gc.Equals, nil)
	c.Assert(len(models), gc.Equals, 2)
	for _, model := range models {
		c.Assert(model.ControllerUUID, gc.Not(gc.Equals), jimmtest.ControllerUUID)
	}
}

func (s *modelE2EManagerSuite) TestListModels(c *gc.C) {
	conn := s.Open(c, nil, "charlie@canonical.com", nil)
	defer conn.Close()

	client := modelmanager.NewClient(conn)
	models, err := client.ListModels("charlie@canonical.com")
	c.Assert(err, gc.Equals, nil)
	c.Assert(
		models,
		jimmtest.CmpEquals(
			cmpopts.IgnoreTypes(&time.Time{}),
			cmpopts.SortSlices(func(a, b base.UserModel) bool {
				return a.Name < b.Name
			}),
		),
		[]base.UserModel{
			{
				Name:  s.Model2.Name,
				UUID:  s.Model2.UUID.String,
				Owner: "charlie@canonical.com",
				Type:  "iaas",
			}, {
				Name:  s.Model3.Name,
				UUID:  s.Model3.UUID.String,
				Owner: "charlie@canonical.com",
				Type:  "iaas",
			},
		},
	)
}

func (s *modelE2EManagerSuite) TestModelInfo(c *gc.C) {
	model4Name := petname.Generate(2, "-")
	mt4 := s.AddModel(c, names.NewUserTag("charlie@canonical.com"), model4Name, names.NewCloudTag(jimmtest.TestE2ECloudName), jimmtest.TestE2ECloudRegionName, s.Model2.CloudCredential.ResourceTag())

	bobIdentity, err := dbmodel.NewIdentity("bob@canonical.com")
	c.Assert(err, gc.IsNil)
	bob := openfga.NewUser(bobIdentity, s.OFGAClient)
	err = bob.SetModelAccess(context.Background(), mt4, ofganames.WriterRelation)
	c.Assert(err, gc.Equals, nil)

	model5Name := petname.Generate(2, "-")
	mt5 := s.AddModel(c, names.NewUserTag("charlie@canonical.com"), model5Name, names.NewCloudTag(jimmtest.TestE2ECloudName), jimmtest.TestE2ECloudRegionName, s.Model2.CloudCredential.ResourceTag())
	err = bob.SetModelAccess(context.Background(), mt5, ofganames.AdministratorRelation)
	c.Assert(err, gc.Equals, nil)

	conn := s.Open(c, nil, "bob", nil)
	defer conn.Close()
	client := modelmanager.NewClient(conn)
	models, err := client.ModelInfo([]names.ModelTag{
		s.Model.ResourceTag(),
		s.Model2.ResourceTag(),
		s.Model3.ResourceTag(),
		mt4,
		mt5,
		names.NewModelTag("00000000-0000-0000-0000-000000000007"),
	})
	c.Assert(err, gc.Equals, nil)

	// Verify we got results for all 6 model requests
	c.Assert(models, gc.HasLen, 6)

	// Helper to find user access in Users slice
	findUserAccess := func(users []jujuparams.ModelUserInfo, username string) string {
		for _, u := range users {
			if u.UserName == username {
				return string(u.Access)
			}
		}
		return ""
	}

	// Model 1 (s.Model) - bob has admin access
	c.Assert(models[0].Error, gc.IsNil)
	c.Assert(models[0].Result.Name, gc.Equals, s.Model.Name)
	c.Assert(models[0].Result.UUID, gc.Equals, s.Model.UUID.String)
	c.Assert(models[0].Result.Life, gc.Equals, life.Alive)
	c.Assert(models[0].Result.ControllerUUID, gc.Equals, jimmtest.ControllerUUID)
	c.Assert(models[0].Result.CloudCredentialTag, gc.Equals, s.Model.CloudCredential.Tag().String())
	c.Assert(models[0].Result.OwnerTag, gc.Equals, names.NewUserTag("bob@canonical.com").String())
	// As admin, bob can see all users
	c.Assert(findUserAccess(models[0].Result.Users, "bob@canonical.com"), gc.Equals, "admin")
	c.Assert(findUserAccess(models[0].Result.Users, "alice@canonical.com"), gc.Equals, "admin")

	// Model 2 (s.Model2) - bob has no access
	c.Assert(models[1].Result, gc.IsNil)
	c.Assert(models[1].Error, gc.Not(gc.IsNil))
	c.Assert(models[1].Error.Code, gc.Equals, jujuparams.CodeUnauthorized)

	// Model 3 (s.Model3) - bob has read access
	c.Assert(models[2].Error, gc.IsNil)
	c.Assert(models[2].Result.Name, gc.Equals, s.Model3.Name)
	c.Assert(models[2].Result.UUID, gc.Equals, s.Model3.UUID.String)
	c.Assert(models[2].Result.ControllerUUID, gc.Equals, jimmtest.ControllerUUID)
	c.Assert(models[2].Result.CloudCredentialTag, gc.Equals, s.Model3.CloudCredential.Tag().String())
	c.Assert(models[2].Result.OwnerTag, gc.Equals, names.NewUserTag("charlie@canonical.com").String())
	// As reader, bob can only see himself in users list
	c.Assert(findUserAccess(models[2].Result.Users, "bob@canonical.com"), gc.Equals, "read")
	// Read access means no machines visible
	c.Assert(models[2].Result.Machines, gc.IsNil)

	// Model 4 (mt4) - bob has write access
	c.Assert(models[3].Error, gc.IsNil)
	c.Assert(models[3].Result.Name, gc.Equals, model4Name)
	c.Assert(models[3].Result.UUID, gc.Equals, mt4.Id())
	c.Assert(models[3].Result.ControllerUUID, gc.Equals, jimmtest.ControllerUUID)
	c.Assert(models[3].Result.OwnerTag, gc.Equals, names.NewUserTag("charlie@canonical.com").String())
	// As writer, bob can only see himself in users list
	c.Assert(findUserAccess(models[3].Result.Users, "bob@canonical.com"), gc.Equals, "write")
	// Write access means machines are visible (not nil, though may be empty)

	// Model 5 (mt5) - bob has admin access
	c.Assert(models[4].Error, gc.IsNil)
	c.Assert(models[4].Result.Name, gc.Equals, model5Name)
	c.Assert(models[4].Result.UUID, gc.Equals, mt5.Id())
	c.Assert(models[4].Result.ControllerUUID, gc.Equals, jimmtest.ControllerUUID)
	c.Assert(models[4].Result.OwnerTag, gc.Equals, names.NewUserTag("charlie@canonical.com").String())
	// As admin, bob can see all users with access
	c.Assert(findUserAccess(models[4].Result.Users, "bob@canonical.com"), gc.Equals, "admin")
	c.Assert(findUserAccess(models[4].Result.Users, "alice@canonical.com"), gc.Equals, "admin")
	c.Assert(findUserAccess(models[4].Result.Users, "charlie@canonical.com"), gc.Equals, "admin")

	// Non-existent model - unauthorized
	c.Assert(models[5].Result, gc.IsNil)
	c.Assert(models[5].Error, gc.Not(gc.IsNil))
	c.Assert(models[5].Error.Code, gc.Equals, jujuparams.CodeUnauthorized)
}

func (s *modelE2EManagerSuite) TestModelInfoDisableControllerUUIDMasking(c *gc.C) {
	// Make bob a JIMM administrator so he can disable UUID masking
	err := s.OFGAClient.AddRelation(context.Background(),
		openfga.Tuple{
			Object:   ofganames.ConvertTag(names.NewUserTag("bob@canonical.com")),
			Relation: ofganames.AdministratorRelation,
			Target:   ofganames.ConvertTag(s.JIMM.ResourceTag()),
		},
	)
	c.Assert(err, gc.Equals, nil)

	conn := s.Open(c, nil, "bob", nil)
	defer conn.Close()

	// Disable controller UUID masking
	err = conn.APICall("JIMM", 4, "", "DisableControllerUUIDMasking", nil, nil)
	c.Assert(err, gc.Equals, nil)

	client := modelmanager.NewClient(conn)
	models, err := client.ModelInfo([]names.ModelTag{s.Model.ResourceTag()})
	c.Assert(err, gc.Equals, nil)
	c.Assert(models, gc.HasLen, 1)
	c.Assert(models[0].Error, gc.IsNil)

	// The controller UUID should NOT be the masked JIMM controller UUID
	c.Assert(models[0].Result.ControllerUUID, gc.Not(gc.Equals), jimmtest.ControllerUUID)
	// It should be the actual backing controller's UUID
	c.Assert(models[0].Result.ControllerUUID, gc.Not(gc.Equals), "")
}

func (s *modelE2EManagerSuite) TestCreateModel(c *gc.C) {
	conn := s.Open(c, nil, "bob", nil)
	defer conn.Close()

	// Generate unique model names for each test
	generateModelName := func() string {
		return petname.Generate(2, "-")
	}

	createModelTests := []struct {
		about         string
		name          string
		ownerTag      string
		region        string
		cloudTag      string
		credentialTag string
		config        map[string]interface{}
		expectError   string
	}{{
		about:         "success",
		name:          generateModelName(),
		ownerTag:      names.NewUserTag("bob@canonical.com").String(),
		cloudTag:      names.NewCloudTag(jimmtest.TestE2ECloudName).String(),
		credentialTag: "cloudcred-" + jimmtest.TestE2ECloudName + "_bob@canonical.com_cred",
	}, {
		about:         "unauthorized user",
		name:          generateModelName(),
		ownerTag:      names.NewUserTag("noauthuser@canonical.com").String(),
		cloudTag:      names.NewCloudTag(jimmtest.TestE2ECloudName).String(),
		credentialTag: "cloudcred-" + jimmtest.TestE2ECloudName + "_bob@canonical.com_cred",
		expectError:   `unauthorized \(unauthorized access\)`,
	}, {
		about:         "existing model name",
		name:          s.Model.Name, // Use existing model name to trigger duplicate error
		ownerTag:      names.NewUserTag("bob@canonical.com").String(),
		cloudTag:      names.NewCloudTag(jimmtest.TestE2ECloudName).String(),
		credentialTag: "cloudcred-" + jimmtest.TestE2ECloudName + "_bob@canonical.com_cred",
		expectError:   "model bob@canonical.com/" + s.Model.Name + " already exists \\(already exists\\)",
	}, {
		about:         "no controller for region",
		name:          generateModelName(),
		ownerTag:      names.NewUserTag("bob@canonical.com").String(),
		region:        "no-such-region",
		cloudTag:      names.NewCloudTag(jimmtest.TestE2ECloudName).String(),
		credentialTag: "",
		expectError:   `cloud region "no-such-region" not found in cloud "localhost" \(not found\)`,
	}, {
		about:         "local user",
		name:          generateModelName(),
		ownerTag:      names.NewUserTag("bob").String(),
		cloudTag:      names.NewCloudTag(jimmtest.TestE2ECloudName).String(),
		credentialTag: "cloudcred-" + jimmtest.TestE2ECloudName + "_bob@canonical.com_cred",
		expectError:   `unauthorized \(unauthorized access\)`,
	}, {
		about:         "invalid user",
		name:          generateModelName(),
		ownerTag:      "user-bob/test@canonical.com",
		cloudTag:      names.NewCloudTag(jimmtest.TestE2ECloudName).String(),
		credentialTag: "cloudcred-" + jimmtest.TestE2ECloudName + "_bob@canonical.com_cred",
		expectError:   `"user-bob/test@canonical.com" is not a valid user tag \(bad request\)`,
	}, {
		about:         "specific cloud",
		name:          generateModelName(),
		ownerTag:      names.NewUserTag("bob@canonical.com").String(),
		cloudTag:      names.NewCloudTag(jimmtest.TestE2ECloudName).String(),
		credentialTag: "cloudcred-" + jimmtest.TestE2ECloudName + "_bob@canonical.com_cred",
	}, {
		about:         "specific cloud and region",
		name:          generateModelName(),
		ownerTag:      names.NewUserTag("bob@canonical.com").String(),
		cloudTag:      names.NewCloudTag(jimmtest.TestE2ECloudName).String(),
		region:        jimmtest.TestE2ECloudRegionName,
		credentialTag: "cloudcred-" + jimmtest.TestE2ECloudName + "_bob@canonical.com_cred",
	}, {
		about:         "bad cloud tag",
		name:          generateModelName(),
		ownerTag:      names.NewUserTag("bob@canonical.com").String(),
		cloudTag:      "not-a-cloud-tag",
		credentialTag: "cloudcred-" + jimmtest.TestE2ECloudName + "_bob@canonical.com_cred",
		expectError:   `"not-a-cloud-tag" is not a valid tag \(bad request\)`,
	}, {
		about:    "no credential tag selects unambiguous creds",
		name:     generateModelName(),
		ownerTag: names.NewUserTag("bob@canonical.com").String(),
		cloudTag: names.NewCloudTag(jimmtest.TestE2ECloudName).String(),
		region:   jimmtest.TestE2ECloudRegionName,
	}, {
		about:         "success - without a cloud tag",
		name:          generateModelName(),
		ownerTag:      names.NewUserTag("bob@canonical.com").String(),
		credentialTag: "cloudcred-" + jimmtest.TestE2ECloudName + "_bob@canonical.com_cred",
	}}

	for i, test := range createModelTests {
		c.Logf("test %d. %s", i, test.about)
		var mi jujuparams.ModelInfo
		err := conn.APICall("ModelManager", 9, "", "CreateModel", jujuparams.ModelCreateArgs{
			Name:               test.name,
			OwnerTag:           test.ownerTag,
			Config:             test.config,
			CloudTag:           test.cloudTag,
			CloudRegion:        test.region,
			CloudCredentialTag: test.credentialTag,
		}, &mi)
		if test.expectError != "" {
			c.Assert(err, gc.ErrorMatches, test.expectError)
			continue
		}
		c.Assert(err, gc.Equals, nil)
		c.Assert(mi.Name, gc.Equals, test.name)
		c.Assert(mi.UUID, gc.Not(gc.Equals), "")
		c.Assert(mi.OwnerTag, gc.Equals, test.ownerTag)
		c.Assert(mi.ControllerUUID, gc.Equals, jimmtest.ControllerUUID)
		c.Assert(mi.Users, gc.Not(gc.HasLen), 0)
		if test.credentialTag == "" {
			c.Assert(mi.CloudCredentialTag, gc.Not(gc.Equals), "")
		} else {
			tag, err := names.ParseCloudCredentialTag(mi.CloudCredentialTag)
			c.Assert(err, gc.Equals, nil)
			c.Assert(tag.String(), gc.Equals, test.credentialTag)
		}
		if test.cloudTag == "" {
			c.Assert(mi.CloudTag, gc.Equals, names.NewCloudTag(jimmtest.TestE2ECloudName).String())
		} else {
			ct, err := names.ParseCloudTag(test.cloudTag)
			c.Assert(err, gc.Equals, nil)
			c.Assert(mi.CloudTag, gc.Equals, names.NewCloudTag(ct.Id()).String())
		}
	}
}

func (s *modelE2EManagerSuite) TestCreateDuplicateModelsFails(c *gc.C) {
	conn := s.Open(c, nil, "bob", nil)
	defer conn.Close()

	modelName := petname.Generate(2, "-")
	createModel := func(mi jujuparams.ModelInfo) error {
		return conn.APICall("ModelManager", 9, "", "CreateModel", jujuparams.ModelCreateArgs{
			Name:               modelName,
			OwnerTag:           names.NewUserTag("bob@canonical.com").String(),
			CloudTag:           names.NewCloudTag(jimmtest.TestE2ECloudName).String(),
			CloudCredentialTag: "cloudcred-" + jimmtest.TestE2ECloudName + "_bob@canonical.com_cred",
		}, &mi)
	}
	var mi jujuparams.ModelInfo
	err := createModel(mi)
	c.Assert(err, gc.IsNil)
	err = createModel(mi)
	c.Assert(err, gc.ErrorMatches, `model bob@canonical\.com/`+modelName+` already exists \(already exists\)`)
}

func (s *modelE2EManagerSuite) TestGrantAndRevokeModel(c *gc.C) {
	conn := s.Open(c, nil, "bob", nil)
	defer conn.Close()
	client := modelmanager.NewClient(conn)

	conn2 := s.Open(c, nil, "charlie", nil)
	defer conn2.Close()
	client2 := modelmanager.NewClient(conn2)

	res, err := client2.ModelInfo([]names.ModelTag{s.Model.ResourceTag()})
	c.Assert(err, gc.Equals, nil)
	c.Assert(res, gc.HasLen, 1)
	c.Assert(res[0].Error, gc.ErrorMatches, "unauthorized")

	err = client.GrantModel("charlie@canonical.com", "write", s.Model.UUID.String)
	c.Assert(err, gc.Equals, nil)

	res, err = client2.ModelInfo([]names.ModelTag{s.Model.ResourceTag()})
	c.Assert(err, gc.Equals, nil)
	c.Assert(res, gc.HasLen, 1)
	c.Assert(res[0].Error, gc.IsNil)
	c.Assert(res[0].Result.UUID, gc.Equals, s.Model.UUID.String)

	err = client.RevokeModel("charlie@canonical.com", "read", s.Model.UUID.String)
	c.Assert(err, gc.Equals, nil)

	res, err = client2.ModelInfo([]names.ModelTag{s.Model.ResourceTag()})
	c.Assert(err, gc.Equals, nil)
	c.Assert(res, gc.HasLen, 1)
	c.Assert(res[0].Error, gc.Not(gc.IsNil))
	c.Assert(res[0].Error, gc.ErrorMatches, "unauthorized")
}

func (s *modelE2EManagerSuite) TestUserRevokeOwnAccess(c *gc.C) {
	conn := s.Open(c, nil, "bob", nil)
	defer conn.Close()
	client := modelmanager.NewClient(conn)

	conn2 := s.Open(c, nil, "charlie", nil)
	defer conn2.Close()
	client2 := modelmanager.NewClient(conn2)

	err := client.GrantModel("charlie@canonical.com", "read", s.Model.UUID.String)
	c.Assert(err, gc.Equals, nil)

	res, err := client2.ModelInfo([]names.ModelTag{names.NewModelTag(s.Model.UUID.String)})
	c.Assert(err, gc.Equals, nil)
	c.Assert(res, gc.HasLen, 1)
	c.Assert(res[0].Error, gc.IsNil)
	c.Assert(res[0].Result.UUID, gc.Equals, s.Model.UUID.String)

	err = client2.RevokeModel("charlie@canonical.com", "read", s.Model.UUID.String)
	c.Assert(err, gc.Equals, nil)

	res, err = client2.ModelInfo([]names.ModelTag{names.NewModelTag(s.Model.UUID.String)})
	c.Assert(err, gc.Equals, nil)
	c.Assert(res, gc.HasLen, 1)
	c.Assert(res[0].Error, gc.Not(gc.IsNil))
	c.Assert(res[0].Error, gc.ErrorMatches, "unauthorized")
}

func (s *modelE2EManagerSuite) TestModifyModelAccessErrors(c *gc.C) {
	conn := s.Open(c, nil, "bob", nil)
	defer conn.Close()

	modifyModelAccessErrorTests := []struct {
		about             string
		modifyModelAccess jujuparams.ModifyModelAccess
		expectError       string
	}{{
		about: "unauthorized",
		modifyModelAccess: jujuparams.ModifyModelAccess{
			UserTag:  names.NewUserTag("eve@canonical.com").String(),
			Action:   jujuparams.GrantModelAccess,
			Access:   jujuparams.ModelReadAccess,
			ModelTag: s.Model2.Tag().String(),
		},
		expectError: `unauthorized`,
	}, {
		about: "bad user domain",
		modifyModelAccess: jujuparams.ModifyModelAccess{
			UserTag:  names.NewUserTag("eve@local").String(),
			Action:   jujuparams.GrantModelAccess,
			Access:   jujuparams.ModelReadAccess,
			ModelTag: s.Model.Tag().String(),
		},
		expectError: `unsupported local user; if this is a service account add @serviceaccount domain`,
	}, {
		about: "no such model",
		modifyModelAccess: jujuparams.ModifyModelAccess{
			UserTag:  names.NewUserTag("eve@canonical.com").String(),
			Action:   jujuparams.GrantModelAccess,
			Access:   jujuparams.ModelReadAccess,
			ModelTag: names.NewModelTag("00000000-0000-0000-0000-000000000000").String(),
		},
		expectError: `unauthorized`,
	}, {
		about: "invalid model tag",
		modifyModelAccess: jujuparams.ModifyModelAccess{
			UserTag:  names.NewUserTag("eve@canonical.com").String(),
			Action:   jujuparams.GrantModelAccess,
			Access:   jujuparams.ModelReadAccess,
			ModelTag: "not-a-model-tag",
		},
		expectError: `"not-a-model-tag" is not a valid tag`,
	}, {
		about: "invalid user tag",
		modifyModelAccess: jujuparams.ModifyModelAccess{
			UserTag:  "not-a-user-tag",
			Action:   jujuparams.GrantModelAccess,
			Access:   jujuparams.ModelReadAccess,
			ModelTag: s.Model.Tag().String(),
		},
		expectError: `"not-a-user-tag" is not a valid tag`,
	}, {
		about: "unknown action",
		modifyModelAccess: jujuparams.ModifyModelAccess{
			UserTag:  names.NewUserTag("eve@canonical.com").String(),
			Action:   "not-an-action",
			Access:   jujuparams.ModelReadAccess,
			ModelTag: s.Model.Tag().String(),
		},
		expectError: `invalid action "not-an-action"`,
	}, {
		about: "invalid access",
		modifyModelAccess: jujuparams.ModifyModelAccess{
			UserTag:  names.NewUserTag("eve@canonical.com").String(),
			Action:   jujuparams.GrantModelAccess,
			Access:   "not-an-access",
			ModelTag: s.Model.Tag().String(),
		},
		expectError: `failed to recognize given access: "not-an-access"`,
	}}

	for i, test := range modifyModelAccessErrorTests {
		c.Logf("%d. %s", i, test.about)
		var res jujuparams.ErrorResults
		req := jujuparams.ModifyModelAccessRequest{
			Changes: []jujuparams.ModifyModelAccess{
				test.modifyModelAccess,
			},
		}
		err := conn.APICall("ModelManager", 9, "", "ModifyModelAccess", req, &res)
		c.Assert(err, gc.Equals, nil)
		c.Assert(res.Results, gc.HasLen, 1)
		c.Assert(res.Results[0].Error, gc.ErrorMatches, test.expectError)
	}
}

var zeroDuration = time.Duration(0)

func (s *modelE2EManagerSuite) TestDestroyModel(c *gc.C) {
	// Create a new model to destroy so we don't affect other tests
	conn := s.Open(c, nil, "bob", nil)
	defer conn.Close()

	modelName := petname.Generate(2, "-")
	var mi jujuparams.ModelInfo
	err := conn.APICall("ModelManager", 9, "", "CreateModel", jujuparams.ModelCreateArgs{
		Name:               modelName,
		OwnerTag:           names.NewUserTag("bob@canonical.com").String(),
		CloudTag:           names.NewCloudTag(jimmtest.TestE2ECloudName).String(),
		CloudCredentialTag: "cloudcred-" + jimmtest.TestE2ECloudName + "_bob@canonical.com_cred",
	}, &mi)
	c.Assert(err, gc.Equals, nil)

	client := modelmanager.NewClient(conn)
	tag := names.NewModelTag(mi.UUID)
	err = client.DestroyModel(tag, nil, nil, nil, &zeroDuration)
	c.Assert(err, gc.Equals, nil)

	// Check the model is now dying.
	mis, err := client.ModelInfo([]names.ModelTag{tag})
	c.Assert(err, gc.Equals, nil)
	c.Assert(mis, gc.HasLen, 1)
	c.Assert(mis[0].Error, gc.Equals, (*jujuparams.Error)(nil))
	c.Assert(mis[0].Result.Life, gc.Equals, life.Dying)

	// Make sure it's not an error if you destroy a model that's not there.
	err = client.DestroyModel(tag, nil, nil, nil, &zeroDuration)
	c.Assert(err, gc.Equals, nil)
}

func (s *modelE2EManagerSuite) TestDumpModel(c *gc.C) {
	conn := s.Open(c, nil, "bob", nil)
	defer conn.Close()

	tag := s.Model.ResourceTag()
	client := modelmanager.NewClient(conn)
	res, err := client.DumpModel(tag, false)
	c.Check(err, gc.Equals, nil)
	c.Check(res, gc.Not(gc.HasLen), 0)
}

func (s *modelE2EManagerSuite) TestDumpModelUnauthorized(c *gc.C) {
	conn := s.Open(c, nil, "charlie", nil)
	defer conn.Close()

	tag := s.Model.ResourceTag()
	client := modelmanager.NewClient(conn)
	res, err := client.DumpModel(tag, true)
	c.Check(err, gc.ErrorMatches, `unauthorized`)
	c.Check(res, gc.IsNil)
}

func (s *modelE2EManagerSuite) TestDumpModelDB(c *gc.C) {
	conn := s.Open(c, nil, "bob", nil)
	defer conn.Close()

	tag := s.Model.ResourceTag()
	client := modelmanager.NewClient(conn)
	res, err := client.DumpModelDB(tag)
	c.Check(err, gc.Equals, nil)
	c.Check(res, gc.Not(gc.HasLen), 0)
}

func (s *modelE2EManagerSuite) TestDumpModelDBUnauthorized(c *gc.C) {
	conn := s.Open(c, nil, "charlie", nil)
	defer conn.Close()

	tag := s.Model.ResourceTag()
	client := modelmanager.NewClient(conn)
	res, err := client.DumpModelDB(tag)
	c.Check(err, gc.ErrorMatches, `unauthorized`)
	c.Check(res, gc.IsNil)
}

func (s *modelE2EManagerSuite) TestChangeModelCredential(c *gc.C) {
	conn := s.Open(c, nil, "bob", nil)
	defer conn.Close()

	modelTag := s.Model.ResourceTag()
	credTag := names.NewCloudCredentialTag(jimmtest.TestE2ECloudName + "/bob@canonical.com/cred2")
	cred := s.GetExistingClientCredentialsForCloud(c, jimmtest.TestE2ECloudName)
	s.UpdateCloudCredential(c, credTag, cred)
	client := modelmanager.NewClient(conn)
	err := client.ChangeModelCredential(modelTag, credTag)
	c.Assert(err, gc.Equals, nil)
	mir, err := client.ModelInfo([]names.ModelTag{modelTag})
	c.Assert(err, gc.Equals, nil)
	c.Assert(mir, gc.HasLen, 1)
	c.Assert(mir[0].Error, gc.IsNil)
	c.Assert(mir[0].Result.CloudCredentialTag, gc.Equals, credTag.String())
}

func (s *modelE2EManagerSuite) TestChangeModelCredentialUnauthorizedModel(c *gc.C) {
	conn := s.Open(c, nil, "charlie", nil)
	defer conn.Close()

	modelTag := s.Model.ResourceTag()
	credTag := names.NewCloudCredentialTag(jimmtest.TestE2ECloudName + "/bob@canonical.com/cred2")
	client := modelmanager.NewClient(conn)
	err := client.ChangeModelCredential(modelTag, credTag)
	c.Assert(err, gc.ErrorMatches, `unauthorized`)
}

func (s *modelE2EManagerSuite) TestChangeModelCredentialUnauthorizedCredential(c *gc.C) {
	conn := s.Open(c, nil, "bob", nil)
	defer conn.Close()

	modelTag := s.Model.ResourceTag()
	credTag := names.NewCloudCredentialTag(jimmtest.TestE2ECloudName + "/alice@canonical.com/cred2")
	client := modelmanager.NewClient(conn)
	err := client.ChangeModelCredential(modelTag, credTag)
	c.Assert(err, gc.ErrorMatches, `unauthorized`)
}

func (s *modelE2EManagerSuite) TestChangeModelCredentialNotFoundModel(c *gc.C) {
	conn := s.Open(c, nil, "bob", nil)
	defer conn.Close()

	modelTag := names.NewModelTag("00000000-0000-0000-0000-000000000000")
	credTag := s.Model.CloudCredential.ResourceTag()
	client := modelmanager.NewClient(conn)
	err := client.ChangeModelCredential(modelTag, credTag)
	c.Assert(err, gc.ErrorMatches, `model not found`)
}

func (s *modelE2EManagerSuite) TestChangeModelCredentialNotFoundCredential(c *gc.C) {
	conn := s.Open(c, nil, "bob", nil)
	defer conn.Close()

	modelTag := s.Model.ResourceTag()
	credTag := names.NewCloudCredentialTag(jimmtest.TestE2ECloudName + "/bob@canonical.com/cred2")
	client := modelmanager.NewClient(conn)
	err := client.ChangeModelCredential(modelTag, credTag)
	c.Assert(err, gc.ErrorMatches, `cloudcredential "`+jimmtest.TestE2ECloudName+`/bob@canonical.com/cred2" not found`)
}

func (s *modelE2EManagerSuite) TestChangeModelCredentialLocalUserCredential(c *gc.C) {
	conn := s.Open(c, nil, "bob", nil)
	defer conn.Close()

	modelTag := s.Model.ResourceTag()
	credTag := names.NewCloudCredentialTag(jimmtest.TestE2ECloudName + "/bob/cred2")
	client := modelmanager.NewClient(conn)
	err := client.ChangeModelCredential(modelTag, credTag)
	c.Assert(err, gc.ErrorMatches, `unauthorized`)
}

func (s *modelE2EManagerSuite) TestModelDefaults(c *gc.C) {
	err := s.JIMM.Database.AddCloud(context.Background(), &dbmodel.Cloud{
		Name: "aws",
		Type: "ec2",
		Regions: []dbmodel.CloudRegion{{
			Name: "eu-central-1",
		}, {
			Name: "eu-central-2",
		}},
	})
	c.Assert(err, jc.ErrorIsNil)

	conn := s.Open(c, nil, "alice", nil)
	defer conn.Close()
	client := modelmanager.NewClient(conn)

	err = client.SetModelDefaults("aws", "eu-central-1", map[string]interface{}{
		"a": 1,
		"b": "value1",
	})
	c.Assert(err, jc.ErrorIsNil)
	err = client.SetModelDefaults("aws", "eu-central-2", map[string]interface{}{
		"b": "value2",
		"c": 17,
	})
	c.Assert(err, jc.ErrorIsNil)

	values, err := client.ModelDefaults("aws")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(values, jc.DeepEquals, config.ModelDefaultAttributes{
		"a": config.AttributeDefaultValues{
			Regions: []config.RegionDefaultValue{{
				Name:  "eu-central-1",
				Value: float64(1),
			}},
		},
		"b": config.AttributeDefaultValues{
			Regions: []config.RegionDefaultValue{{
				Name:  "eu-central-1",
				Value: "value1",
			}, {
				Name:  "eu-central-2",
				Value: "value2",
			}},
		},
		"c": config.AttributeDefaultValues{
			Regions: []config.RegionDefaultValue{{
				Name:  "eu-central-2",
				Value: float64(17),
			}},
		},
	})

	err = client.UnsetModelDefaults("aws", "eu-central-1", "b", "c")
	c.Assert(err, jc.ErrorIsNil)

	err = client.UnsetModelDefaults("aws", "eu-central-2", "a", "b")
	c.Assert(err, jc.ErrorIsNil)

	values, err = client.ModelDefaults("aws")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(values, jc.DeepEquals, config.ModelDefaultAttributes{
		"a": config.AttributeDefaultValues{
			Regions: []config.RegionDefaultValue{{
				Name:  "eu-central-1",
				Value: float64(1),
			}},
		},
		"c": config.AttributeDefaultValues{
			Regions: []config.RegionDefaultValue{{
				Name:  "eu-central-2",
				Value: float64(17),
			}},
		},
	})

	conn1 := s.Open(c, nil, "bob", nil)
	defer conn1.Close()
	client1 := modelmanager.NewClient(conn1)

	values, err = client1.ModelDefaults("aws")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(values, jc.DeepEquals, config.ModelDefaultAttributes{})
}
