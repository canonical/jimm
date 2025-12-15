// Copyright 2025 Canonical.

package testing

import (
	"context"
	"database/sql"
	"slices"
	"time"

	petname "github.com/dustinkirkland/golang-petname"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/juju/juju/api/client/modelconfig"
	"github.com/juju/juju/api/client/modelmanager"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/network"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/jimm/juju"
	"github.com/canonical/jimm/v3/internal/jujuapi"
	"github.com/canonical/jimm/v3/internal/openfga"
	ofganames "github.com/canonical/jimm/v3/internal/openfga/names"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
	"github.com/canonical/jimm/v3/pkg/api"
	jimmversion "github.com/canonical/jimm/v3/version"

	apiparams "github.com/canonical/jimm/v3/pkg/api/params"
)

type jimmSuite struct {
	jimmtest.WebsocketE2ESuite
}

var _ = gc.Suite(&jimmSuite{})

func (s *jimmSuite) TestListControllersAdmin(c *gc.C) {
	conn := s.Open(c, nil, "alice", nil)
	defer conn.Close()

	client := api.NewClient(conn)
	cis, err := client.ListControllers()
	c.Assert(err, gc.Equals, nil)
	confs := s.GetControllersConfig(c)
	controllerInfos := make([]apiparams.ControllerInfo, 0, len(confs.Controllers))
	for name, conf := range confs.Controllers {
		controllerInfos = append(controllerInfos, apiparams.ControllerInfo{
			Name:          name,
			UUID:          conf.UUID,
			APIAddresses:  conf.ToAPIInfo().Addrs,
			CACertificate: conf.ToAPIInfo().CACert,
			CloudTag:      names.NewCloudTag(jimmtest.TestE2ECloudName).String(),
			CloudRegion:   jimmtest.TestE2ECloudRegionName,
			Status: jujuparams.EntityStatus{
				Status: "available",
			},
		})
	}

	assertControllerInfos(c, cis, controllerInfos, confs)
}

// assertControllerInfos verifies that the controller infos match expectations, including API addresses.
func assertControllerInfos(c *gc.C, actual []apiparams.ControllerInfo, expected []apiparams.ControllerInfo, confs *jimmtest.ControllersConfig) {
	c.Check(actual, jimmtest.CmpEquals(
		cmpopts.IgnoreFields(apiparams.ControllerInfo{}, "AgentVersion", "APIAddresses"),
		cmpopts.SortSlices(func(a, b apiparams.ControllerInfo) bool {
			return a.UUID < b.UUID
		}),
	), expected)

	// Verify all configured addresses are present in the response
	for _, ci := range actual {
		for name, conf := range confs.Controllers {
			if conf.UUID == ci.UUID {
				expectedAddrs := conf.ToAPIInfo().Addrs
				for _, expectedAddr := range expectedAddrs {
					found := slices.Contains(ci.APIAddresses, expectedAddr)
					c.Assert(found, gc.Equals, true, gc.Commentf(
						"controller %q: expected address %q not found in APIAddresses %v",
						name, expectedAddr, ci.APIAddresses))
				}
			}
		}
	}
}

func (s *jimmSuite) TestListControllersOrdinaryUser(c *gc.C) {
	ctx := context.Background()

	ctrl0 := &dbmodel.Controller{
		Name:      "dummy-0",
		UUID:      "00000001-0000-0000-0000-000000000000",
		CloudName: jimmtest.TestE2ECloudName,
	}

	ctrl1 := &dbmodel.Controller{
		Name:      "dummy-1",
		UUID:      "00000001-0000-0000-0000-000000000001",
		CloudName: jimmtest.TestE2ECloudName,
	}

	ctrl2 := &dbmodel.Controller{
		Name:      "dummy-2",
		UUID:      "00000001-0000-0000-0000-000000000002",
		CloudName: jimmtest.TestE2ECloudName,
	}

	err := s.JIMM.Database.AddController(ctx, ctrl0)
	c.Assert(err, gc.IsNil)

	err = s.JIMM.Database.AddController(ctx, ctrl1)
	c.Assert(err, gc.IsNil)

	err = s.JIMM.Database.AddController(ctx, ctrl2)
	c.Assert(err, gc.IsNil)

	// Explicitly set access to controllers 0 and 2, but not 1.
	u, err := dbmodel.NewIdentity("alex@canonical.com")
	c.Assert(err, gc.IsNil)

	err = s.JIMM.Database.GetIdentity(ctx, u)
	c.Assert(err, gc.IsNil)

	openfgaUser := openfga.NewUser(u, s.JIMM.OpenFGAClient)

	err = openfgaUser.SetControllerAccess(ctx, names.NewControllerTag(ctrl0.UUID), ofganames.CanAddModelRelation)
	c.Assert(err, gc.IsNil)

	err = openfgaUser.SetControllerAccess(ctx, names.NewControllerTag(ctrl2.UUID), ofganames.CanAddModelRelation)
	c.Assert(err, gc.IsNil)

	conn := s.Open(c, nil, "alex@canonical.com", nil)
	defer conn.Close()

	client := api.NewClient(conn)
	cis, err := client.ListControllers()
	c.Assert(err, gc.Equals, nil)
	c.Check(cis, jc.DeepEquals, []apiparams.ControllerInfo{
		{
			Name:     "dummy-0",
			UUID:     "00000001-0000-0000-0000-000000000000",
			CloudTag: names.NewCloudTag(jimmtest.TestE2ECloudName).String(),
			Status: jujuparams.EntityStatus{
				Status: "available",
			},
		},
		{
			Name:     "dummy-2",
			UUID:     "00000001-0000-0000-0000-000000000002",
			CloudTag: names.NewCloudTag(jimmtest.TestE2ECloudName).String(),
			Status: jujuparams.EntityStatus{
				Status: "available",
			},
		},
	})
}

func (s *jimmSuite) TestModelGet(c *gc.C) {
	conn := s.Open(c, nil, "alice", nil)
	defer conn.Close()

	client := modelconfig.NewClient(conn)

	jimmCfg, err := client.ModelGet()
	c.Assert(err, gc.IsNil)

	v, ok := jimmCfg["agent-version"]
	c.Assert(ok, gc.Equals, true)
	vers, ok := v.(string)
	c.Assert(ok, gc.Equals, true)
	c.Assert(vers, gc.Equals, jimmversion.ControllerVersion)
}

func (s *jimmSuite) TestListControllersUnauthorized(c *gc.C) {
	conn := s.Open(c, nil, "abrandnewuserwithnopermissions", nil)
	defer conn.Close()

	client := api.NewClient(conn)
	cis, err := client.ListControllers()
	c.Assert(err, gc.Equals, nil)
	c.Check(cis, jc.DeepEquals, []apiparams.ControllerInfo{})
}

func (s *jimmSuite) TestAddControllerPublicAddressWithoutPort(c *gc.C) {
	conn := s.Open(c, nil, "alice", nil)
	defer conn.Close()
	client := api.NewClient(conn)

	_, conf := s.GetOneControllerConfig(c)

	tests := []struct {
		req           apiparams.AddControllerRequest
		expectedError string
	}{{
		req: apiparams.AddControllerRequest{
			Name:          "controller-2",
			PublicAddress: "controller.test.com",
			CACertificate: conf.ToAPIInfo().CACert,
		},
		expectedError: `address controller.test.com: missing port in address \(bad request\)`,
	}, {
		req: apiparams.AddControllerRequest{
			Name:          "controller-2",
			PublicAddress: ":8080",
			CACertificate: conf.ToAPIInfo().CACert,
		},
		expectedError: `address :8080: host not specified in public address \(bad request\)`,
	}, {
		req: apiparams.AddControllerRequest{
			Name:          "controller-2",
			PublicAddress: "localhost:",
			CACertificate: conf.ToAPIInfo().CACert,
		},
		expectedError: `address localhost:: port not specified in public address \(bad request\)`,
	}}

	for _, test := range tests {
		ci, err := client.AddController(&test.req)
		c.Assert(err, gc.ErrorMatches, test.expectedError)
		c.Check(ci, jc.DeepEquals, apiparams.ControllerInfo{})
	}
}

func (s *jimmSuite) TestAddController(c *gc.C) {
	conn := s.Open(c, nil, "alice", nil)
	defer conn.Close()
	client := api.NewClient(conn)
	_, conf := s.GetOneControllerConfig(c)
	info := conf.ToAPIInfo()

	acr := apiparams.AddControllerRequest{
		UUID:          info.ControllerUUID,
		Name:          "controller-2",
		APIAddresses:  info.Addrs,
		CACertificate: info.CACert,
		TLSHostname:   "juju-apiserver",
		Username:      info.Tag.Id(),
		Password:      info.Password,
	}

	ci, err := client.AddController(&acr)
	c.Assert(err, gc.Equals, nil)
	ciExpected := apiparams.ControllerInfo{
		Name:          acr.Name,
		UUID:          acr.UUID,
		CACertificate: acr.CACertificate,
		APIAddresses:  acr.APIAddresses,
		CloudTag:      names.NewCloudTag(jimmtest.TestE2ECloudName).String(),
		CloudRegion:   jimmtest.TestE2ECloudRegionName,
		Status: jujuparams.EntityStatus{
			Status: "available",
		},
	}

	assertControllerInfos(c, []apiparams.ControllerInfo{ci}, []apiparams.ControllerInfo{ciExpected}, s.GetControllersConfig(c))

	_, err = client.AddController(&acr)
	c.Assert(err, gc.ErrorMatches, `failed to add controller: controller "controller-2" already exists \(already exists\)`)
	c.Assert(jujuparams.IsCodeAlreadyExists(err), gc.Equals, true)

	conn = s.Open(c, nil, "bob", nil)
	defer conn.Close()
	client = api.NewClient(conn)
	acr.Name = "controller-2"
	_, err = client.AddController(&acr)
	c.Assert(err, gc.ErrorMatches, `failed to add controller: unauthorized \(unauthorized access\)`)
	c.Assert(jujuparams.IsCodeUnauthorized(err), gc.Equals, true)

	acr.Name = "jimm"
	_, err = client.AddController(&acr)
	c.Assert(err, gc.ErrorMatches, `cannot add a controller with name "jimm" \(bad request\)`)
	c.Assert(jujuparams.IsBadRequest(err), gc.Equals, true)
}

func (s *jimmSuite) TestRemoveAndAddController(c *gc.C) {
	conn := s.Open(c, nil, "alice", nil)
	defer conn.Close()
	client := api.NewClient(conn)

	_, conf := s.GetOneControllerConfig(c)
	info := conf.ToAPIInfo()

	acr := apiparams.AddControllerRequest{
		UUID:          info.ControllerUUID,
		Name:          "controller-2",
		APIAddresses:  info.Addrs,
		CACertificate: info.CACert,
		TLSHostname:   "juju-apiserver",
		Username:      info.Tag.Id(),
		Password:      info.Password,
	}

	ci, err := client.AddController(&acr)
	c.Assert(err, gc.Equals, nil)
	_, err = client.RemoveController(&apiparams.RemoveControllerRequest{Name: acr.Name, Force: true})
	c.Assert(err, gc.Equals, nil)
	ciNew, err := client.AddController(&acr)
	c.Assert(err, gc.Equals, nil)
	c.Assert(ci, gc.DeepEquals, ciNew)
}

func (s *jimmSuite) TestAddControllerCustomTLSHostname(c *gc.C) {
	conn := s.Open(c, nil, "alice", nil)
	defer conn.Close()
	client := api.NewClient(conn)

	_, conf := s.GetOneControllerConfig(c)
	info := conf.ToAPIInfo()

	acr := apiparams.AddControllerRequest{
		UUID:          info.ControllerUUID,
		Name:          "controller-2",
		APIAddresses:  info.Addrs,
		CACertificate: info.CACert,
		Username:      info.Tag.Id(),
		Password:      info.Password,
		TLSHostname:   "foo",
	}

	_, err := client.AddController(&acr)
	c.Assert(err, gc.ErrorMatches, "failed to add controller: failed to dial the controller.*")
	acr.TLSHostname = "juju-apiserver"
	ci, err := client.AddController(&acr)
	c.Assert(err, gc.IsNil)
	ciExpected := apiparams.ControllerInfo{
		Name:          acr.Name,
		UUID:          acr.UUID,
		CACertificate: acr.CACertificate,
		APIAddresses:  acr.APIAddresses,
		CloudTag:      names.NewCloudTag(jimmtest.TestE2ECloudName).String(),
		CloudRegion:   jimmtest.TestE2ECloudRegionName,
		Status: jujuparams.EntityStatus{
			Status: "available",
		},
	}
	assertControllerInfos(c, []apiparams.ControllerInfo{ci}, []apiparams.ControllerInfo{ciExpected}, s.GetControllersConfig(c))
}

func (s *jimmSuite) TestRemoveController(c *gc.C) {
	conn := s.Open(c, nil, "alice", nil)
	defer conn.Close()
	client := api.NewClient(conn)

	name, conf := s.GetOneControllerConfig(c)

	_, err := client.RemoveController(&apiparams.RemoveControllerRequest{
		Name: name,
	})
	c.Check(err, gc.ErrorMatches, `controller is still alive \(still alive\)`)
	c.Check(jujuparams.ErrCode(err), gc.Equals, apiparams.CodeStillAlive)

	conn2 := s.Open(c, nil, "bob", nil)
	defer conn2.Close()
	client2 := api.NewClient(conn2)

	_, err = client2.RemoveController(&apiparams.RemoveControllerRequest{
		Name: name,
	})
	c.Check(err, gc.ErrorMatches, `unauthorized \(unauthorized access\)`)
	c.Check(jujuparams.ErrCode(err), gc.Equals, jujuparams.CodeUnauthorized)

	ci, err := client.RemoveController(&apiparams.RemoveControllerRequest{
		Name:  name,
		Force: true,
	})
	c.Assert(err, gc.Equals, nil)
	ciExpected := apiparams.ControllerInfo{
		Name:          name,
		UUID:          s.Model.Controller.UUID,
		APIAddresses:  conf.ToAPIInfo().Addrs,
		CACertificate: conf.ToAPIInfo().CACert,
		CloudTag:      names.NewCloudTag(jimmtest.TestE2ECloudName).String(),
		CloudRegion:   jimmtest.TestE2ECloudRegionName,
		Status: jujuparams.EntityStatus{
			Status: "available",
		},
	}
	assertControllerInfos(c, []apiparams.ControllerInfo{ci}, []apiparams.ControllerInfo{ciExpected}, s.GetControllersConfig(c))
}

func (s *jimmSuite) TestSetControllerDeprecated(c *gc.C) {
	conn := s.Open(c, nil, "alice", nil)
	defer conn.Close()
	client := api.NewClient(conn)

	name, conf := s.GetOneControllerConfig(c)

	ci, err := client.SetControllerDeprecated(&apiparams.SetControllerDeprecatedRequest{
		Name:       name,
		Deprecated: true,
	})
	c.Assert(err, gc.Equals, nil)
	ciExpected := apiparams.ControllerInfo{
		Name:          name,
		UUID:          s.Model.Controller.UUID,
		APIAddresses:  conf.ToAPIInfo().Addrs,
		CACertificate: conf.ToAPIInfo().CACert,
		CloudTag:      names.NewCloudTag(jimmtest.TestE2ECloudName).String(),
		CloudRegion:   jimmtest.TestE2ECloudRegionName,
		Status: jujuparams.EntityStatus{
			Status: "deprecated",
		},
	}
	assertControllerInfos(c, []apiparams.ControllerInfo{ci}, []apiparams.ControllerInfo{ciExpected}, s.GetControllersConfig(c))

	ci, err = client.SetControllerDeprecated(&apiparams.SetControllerDeprecatedRequest{
		Name:       name,
		Deprecated: false,
	})
	c.Assert(err, gc.Equals, nil)
	ciExpected = apiparams.ControllerInfo{
		Name:          name,
		UUID:          s.Model.Controller.UUID,
		APIAddresses:  conf.ToAPIInfo().Addrs,
		CACertificate: conf.ToAPIInfo().CACert,
		CloudTag:      names.NewCloudTag(jimmtest.TestE2ECloudName).String(),
		CloudRegion:   jimmtest.TestE2ECloudRegionName,
		AgentVersion:  s.Model.Controller.AgentVersion,
		Status: jujuparams.EntityStatus{
			Status: "available",
		},
	}
	assertControllerInfos(c, []apiparams.ControllerInfo{ci}, []apiparams.ControllerInfo{ciExpected}, s.GetControllersConfig(c))

	_, err = client.SetControllerDeprecated(&apiparams.SetControllerDeprecatedRequest{
		Name:       "controller-2",
		Deprecated: true,
	})
	c.Check(err, gc.ErrorMatches, `controller not found \(not found\)`)
	c.Check(jujuparams.IsCodeNotFound(err), gc.Equals, true)

	conn = s.Open(c, nil, "bob", nil)
	defer conn.Close()
	client = api.NewClient(conn)
	_, err = client.SetControllerDeprecated(&apiparams.SetControllerDeprecatedRequest{
		Name:       "controller-1",
		Deprecated: true,
	})
	c.Check(err, gc.ErrorMatches, `unauthorized \(unauthorized access\)`)
	c.Check(jujuparams.IsCodeUnauthorized(err), gc.Equals, true)
}

func (s *jimmSuite) TestAuditLog(c *gc.C) {
	conn := s.Open(c, nil, "bob", nil)
	defer conn.Close()
	client := api.NewClient(conn)

	_, err := client.FindAuditEvents(&apiparams.FindAuditEventsRequest{})
	c.Check(err, gc.ErrorMatches, `unauthorized \(unauthorized access\)`)
	c.Check(jujuparams.ErrCode(err), gc.Equals, jujuparams.CodeUnauthorized)

	mmclient := modelmanager.NewClient(conn)
	zeroDuration := time.Duration(0)
	err = mmclient.DestroyModel(s.Model.ResourceTag(), nil, nil, nil, &zeroDuration)
	c.Assert(err, gc.Equals, nil)

	conn2 := s.Open(c, nil, "alice", nil)
	defer conn2.Close()
	client2 := api.NewClient(conn2)

	evs, err := client2.FindAuditEvents(&apiparams.FindAuditEventsRequest{})
	c.Assert(err, gc.Equals, nil)

	c.Assert(len(evs.Events), gc.Equals, 9)

	bobTag := names.NewUserTag("bob@canonical.com").String()

	expectedEvents := apiparams.AuditEvents{
		Events: []apiparams.AuditEvent{{
			Time:           evs.Events[0].Time,
			ConversationId: evs.Events[0].ConversationId,
			MessageId:      1,
			FacadeName:     "Admin",
			FacadeMethod:   "LoginWithSessionToken",
			FacadeVersion:  evs.Events[0].FacadeVersion,
			ObjectId:       "",
			UserTag:        "user-",
			IsResponse:     false,
			Params:         evs.Events[0].Params,
			Errors:         nil,
		}, {
			Time:           evs.Events[1].Time,
			ConversationId: evs.Events[1].ConversationId,
			MessageId:      1,
			FacadeName:     "Admin",
			FacadeMethod:   "LoginWithSessionToken",
			FacadeVersion:  evs.Events[1].FacadeVersion,
			ObjectId:       "",
			UserTag:        bobTag,
			IsResponse:     true,
			Params:         nil,
			Errors:         evs.Events[1].Errors,
		}, {
			Time:           evs.Events[2].Time,
			ConversationId: evs.Events[2].ConversationId,
			MessageId:      2,
			FacadeName:     "JIMM",
			FacadeMethod:   "FindAuditEvents",
			FacadeVersion:  evs.Events[2].FacadeVersion,
			ObjectId:       "",
			UserTag:        bobTag,
			IsResponse:     false,
			Params:         evs.Events[2].Params,
			Errors:         nil,
		}, {
			Time:           evs.Events[3].Time,
			ConversationId: evs.Events[3].ConversationId,
			MessageId:      2,
			FacadeName:     "JIMM",
			FacadeMethod:   "FindAuditEvents",
			FacadeVersion:  evs.Events[3].FacadeVersion,
			ObjectId:       "",
			UserTag:        bobTag,
			IsResponse:     true,
			Params:         nil,
			Errors:         evs.Events[3].Errors,
		}},
	}
	truncatedEvents := make([]apiparams.AuditEvent, 4)
	copy(truncatedEvents, evs.Events)
	evs.Events = truncatedEvents
	c.Check(evs, jc.DeepEquals, expectedEvents)

	// alice can grant bob access to audit log entries
	err = client2.GrantAuditLogAccess(&apiparams.AuditLogAccessRequest{
		UserTag: names.NewUserTag("bob@canonical.com").String(),
	})
	c.Assert(err, gc.Equals, nil)

	// now bob can access audit events as well
	conn3 := s.Open(c, nil, "bob", nil)
	defer conn3.Close()
	client3 := api.NewClient(conn3)

	evs, err = client3.FindAuditEvents(&apiparams.FindAuditEventsRequest{})
	evs.Events = truncatedEvents
	c.Assert(err, gc.Equals, nil)
	c.Check(evs, jc.DeepEquals, expectedEvents)
}

func (s *jimmSuite) TestAuditLogFilterByMethod(c *gc.C) {
	conn := s.Open(c, nil, "alice", nil)
	defer conn.Close()
	client := api.NewClient(conn)
	evs, err := client.FindAuditEvents(&apiparams.FindAuditEventsRequest{Method: "Deploy"})
	c.Assert(err, gc.Equals, nil)
	c.Assert(len(evs.Events), gc.Equals, 0)
}

func (s *jimmSuite) TestFullModelStatus(c *gc.C) {
	_, conf := s.GetOneControllerConfig(c)

	s.AddController(c, "controller-2", conf.ToAPIInfo())
	modelName := petname.Generate(2, "-")
	mt := s.AddModel(c, names.NewUserTag("charlie@canonical.com"),
		modelName,
		names.NewCloudTag(jimmtest.TestE2ECloudName),
		jimmtest.TestE2ECloudRegionName,
		s.Model2.CloudCredential.ResourceTag())

	conn := s.Open(c, nil, "bob", nil)
	defer conn.Close()
	client := api.NewClient(conn)

	_, err := client.FullModelStatus(&apiparams.FullModelStatusRequest{
		ModelTag: "invalid-model-tag",
	})
	c.Assert(err, gc.ErrorMatches, `"invalid-model-tag" is not a valid tag \(bad request\)`)

	_, err = client.FullModelStatus(&apiparams.FullModelStatusRequest{
		ModelTag: mt.String(),
	})
	c.Assert(err, gc.ErrorMatches, "unauthorized.*")

	conn = s.Open(c, nil, "alice@canonical.com", nil)
	defer conn.Close()
	client = api.NewClient(conn)

	status, err := client.FullModelStatus(&apiparams.FullModelStatusRequest{
		ModelTag: mt.String(),
	})
	c.Assert(err, gc.Equals, nil)
	c.Assert(status,
		jimmtest.CmpEquals(
			cmpopts.EquateEmpty(),
			cmpopts.IgnoreTypes(&time.Time{}),
			cmpopts.IgnoreFields(jujuparams.ModelStatusInfo{}, "Version"),
		),
		jujuparams.FullStatus{
			Model: jujuparams.ModelStatusInfo{
				Name:        modelName,
				Type:        "iaas",
				CloudTag:    names.NewCloudTag(jimmtest.TestE2ECloudName).String(),
				CloudRegion: jimmtest.TestE2ECloudRegionName,
				ModelStatus: jujuparams.DetailedStatus{
					Status: "available",
				},
				SLA: "unsupported",
			},
		})
}

func (s *jimmSuite) TestUpdateMigratedModel(c *gc.C) {
	name, conf := s.GetOneControllerConfig(c)

	s.AddController(c, "controller-2", conf.ToAPIInfo())

	// Open the API connection as user "bob".
	conn := s.Open(c, nil, "bob", nil)
	defer conn.Close()

	req := apiparams.UpdateMigratedModelRequest{
		ModelTag:         names.NewModelTag(s.Model2.UUID.String).String(),
		TargetController: name,
	}
	err := conn.APICall("JIMM", 4, "", "UpdateMigratedModel", &req, nil)
	c.Assert(err, gc.ErrorMatches, `unauthorized \(unauthorized access\)`)

	// Open the API connection as user "alice".
	conn = s.Open(c, nil, "alice", nil)
	defer conn.Close()

	req = apiparams.UpdateMigratedModelRequest{
		ModelTag:         names.NewModelTag(s.Model2.UUID.String).String(),
		TargetController: name,
	}
	err = conn.APICall("JIMM", 4, "", "UpdateMigratedModel", &req, nil)
	c.Assert(err, gc.Equals, nil)

	req = apiparams.UpdateMigratedModelRequest{
		ModelTag:         "invalid-model-tag",
		TargetController: name,
	}
	err = conn.APICall("JIMM", 4, "", "UpdateMigratedModel", &req, nil)
	c.Assert(err, gc.ErrorMatches, `"invalid-model-tag" is not a valid tag \(bad request\)`)
}

func (s *jimmSuite) TestImportModel(c *gc.C) {
	// Open the API connection as user "bob".
	conn := s.Open(c, nil, "bob", nil)
	defer conn.Close()

	name, _ := s.GetOneControllerConfig(c)

	err := s.JIMM.OpenFGAClient.RemoveControllerModel(context.Background(), s.Model2.Controller.ResourceTag(), s.Model2.ResourceTag())
	c.Assert(err, gc.Equals, nil)
	err = s.JIMM.Database.DeleteModel(context.Background(), s.Model2)
	c.Assert(err, gc.Equals, nil)

	req := apiparams.ImportModelRequest{
		Controller: name,
		ModelTag:   s.Model2.Tag().String(),
		Owner:      "",
	}
	err = conn.APICall("JIMM", 4, "", "ImportModel", &req, nil)
	c.Assert(err, gc.ErrorMatches, `unauthorized \(unauthorized access\)`)

	// Open the API connection as user "alice".
	conn = s.Open(c, nil, "alice", nil)
	defer conn.Close()

	err = conn.APICall("JIMM", 4, "", "ImportModel", &req, nil)
	c.Assert(err, gc.Equals, nil)

	var model2 dbmodel.Model
	model2.SetTag(s.Model2.ResourceTag())
	err = s.JIMM.Database.GetModel(context.Background(), &model2)
	c.Assert(err, gc.Equals, nil)
	c.Check(model2.CreatedAt.After(s.Model2.CreatedAt), gc.Equals, true)

	req = apiparams.ImportModelRequest{
		Controller: name,
		ModelTag:   "invalid-model-tag",
	}
	err = conn.APICall("JIMM", 4, "", "ImportModel", &req, nil)
	c.Assert(err, gc.ErrorMatches, `"invalid-model-tag" is not a valid tag \(bad request\)`)
}

func (s *jimmSuite) TestAddCloudToController(c *gc.C) {
	ctx := context.Background()

	u, err := dbmodel.NewIdentity("alice@canonical.com")
	c.Assert(err, gc.IsNil)

	err = s.JIMM.Database.GetIdentity(ctx, u)
	c.Assert(err, gc.IsNil)

	conn := s.Open(c, nil, "alice@canonical.com", nil)
	defer conn.Close()

	name, _ := s.GetOneControllerConfig(c)
	cloudName := petname.Generate(2, "-")

	req := apiparams.AddCloudToControllerRequest{
		ControllerName: name,
		AddCloudArgs: jujuparams.AddCloudArgs{
			Name: cloudName,
			Cloud: jujuapi.CloudToParams(cloud.Cloud{
				Name:             cloudName,
				Type:             "kubernetes",
				AuthTypes:        cloud.AuthTypes{cloud.CertificateAuthType},
				Endpoint:         "https://0.1.2.3:5678",
				IdentityEndpoint: "https://0.1.2.3:5679",
				StorageEndpoint:  "https://0.1.2.3:5680",
				HostCloudRegion:  jimmtest.TestE2EProviderType + "/" + jimmtest.TestE2ECloudRegionName,
			}),
		},
	}
	err = conn.APICall("JIMM", 4, "", "AddCloudToController", &req, nil)
	c.Assert(err, gc.Equals, nil)

	user := openfga.NewUser(u, s.OFGAClient)

	cloud, err := s.JIMM.JujuManager().GetCloud(context.Background(), user, names.NewCloudTag(cloudName))
	c.Assert(err, gc.IsNil)
	c.Assert(cloud.Name, gc.DeepEquals, cloudName)
	c.Assert(cloud.Type, gc.DeepEquals, "kubernetes")

	req1 := apiparams.RemoveCloudFromControllerRequest{
		CloudTag:       names.NewCloudTag(cloudName).String(),
		ControllerName: name,
	}
	err = conn.APICall("JIMM", 4, "", "RemoveCloudFromController", &req1, nil)
	c.Assert(err, gc.Equals, nil)
}

func (s *jimmSuite) TestAddExistingCloudToController(c *gc.C) {
	ctx := context.Background()

	u, err := dbmodel.NewIdentity("alice@canonical.com")
	c.Assert(err, gc.IsNil)

	err = s.JIMM.Database.GetIdentity(ctx, u)
	c.Assert(err, gc.IsNil)

	conn := s.Open(c, nil, "alice@canonical.com", nil)
	defer conn.Close()

	name, _ := s.GetOneControllerConfig(c)

	cloudName := petname.Generate(2, "-")
	force := true
	req := apiparams.AddCloudToControllerRequest{
		ControllerName: name,
		AddCloudArgs: jujuparams.AddCloudArgs{
			Name: cloudName,
			Cloud: jujuapi.CloudToParams(cloud.Cloud{
				Name:             cloudName,
				Type:             "MAAS",
				AuthTypes:        cloud.AuthTypes{cloud.OAuth1AuthType},
				Endpoint:         "https://0.1.2.3:5678",
				IdentityEndpoint: "https://0.1.2.3:5679",
				StorageEndpoint:  "https://0.1.2.3:5680",
			}),
			Force: &force,
		},
	}
	err = conn.APICall("JIMM", 4, "", "AddCloudToController", &req, nil)
	c.Assert(err, gc.Equals, nil)
	user := openfga.NewUser(u, s.OFGAClient)
	cloud, err := s.JIMM.JujuManager().GetCloud(context.Background(), user, names.NewCloudTag(cloudName))
	c.Assert(err, gc.IsNil)
	c.Assert(cloud.Name, gc.DeepEquals, cloudName)
	c.Assert(cloud.Type, gc.DeepEquals, "MAAS")
	// Simulate the cloud being present on the Juju controller but not in JIMM.
	err = s.JIMM.Database.DeleteCloud(ctx, &cloud)
	c.Assert(err, gc.IsNil)
	cloud, err = s.JIMM.JujuManager().GetCloud(context.Background(), user, names.NewCloudTag(cloudName))
	c.Assert(err, gc.NotNil)
	c.Assert(errors.ErrorCode(err), gc.Equals, errors.CodeNotFound)
	err = conn.APICall("JIMM", 4, "", "AddCloudToController", &req, nil)
	c.Assert(err, gc.Equals, nil)
	cloud, err = s.JIMM.JujuManager().GetCloud(context.Background(), user, names.NewCloudTag(cloudName))
	c.Assert(err, gc.IsNil)
	c.Assert(cloud.Name, gc.DeepEquals, cloudName)
	c.Assert(cloud.Type, gc.DeepEquals, "MAAS")

	req1 := apiparams.RemoveCloudFromControllerRequest{
		CloudTag:       names.NewCloudTag(cloudName).String(),
		ControllerName: name,
	}
	err = conn.APICall("JIMM", 4, "", "RemoveCloudFromController", &req1, nil)
	c.Assert(err, gc.Equals, nil)

}

func (s *jimmSuite) TestRemoveCloudFromController(c *gc.C) {
	ctx := context.Background()

	u, err := dbmodel.NewIdentity("alice@canonical.com")
	c.Assert(err, gc.IsNil)

	err = s.JIMM.Database.GetIdentity(ctx, u)
	c.Assert(err, gc.IsNil)

	conn := s.Open(c, nil, "alice@canonical.com", nil)
	defer conn.Close()

	name, _ := s.GetOneControllerConfig(c)
	cloudName := petname.Generate(2, "-")
	req := apiparams.AddCloudToControllerRequest{
		ControllerName: name,
		AddCloudArgs: jujuparams.AddCloudArgs{
			Name: cloudName,
			Cloud: jujuapi.CloudToParams(cloud.Cloud{
				Name:             cloudName,
				Type:             "kubernetes",
				AuthTypes:        cloud.AuthTypes{cloud.CertificateAuthType},
				Endpoint:         "https://0.1.2.3:5678",
				IdentityEndpoint: "https://0.1.2.3:5679",
				StorageEndpoint:  "https://0.1.2.3:5680",
				HostCloudRegion:  jimmtest.TestE2EProviderType + "/" + jimmtest.TestE2ECloudRegionName,
			}),
		},
	}
	err = conn.APICall("JIMM", 4, "", "AddCloudToController", &req, nil)
	c.Assert(err, gc.Equals, nil)

	user := openfga.NewUser(u, s.OFGAClient)

	_, err = s.JIMM.JujuManager().GetCloud(context.Background(), user, names.NewCloudTag(cloudName))
	c.Assert(err, gc.Equals, nil)

	req1 := apiparams.RemoveCloudFromControllerRequest{
		CloudTag:       names.NewCloudTag(cloudName).String(),
		ControllerName: name,
	}
	err = conn.APICall("JIMM", 4, "", "RemoveCloudFromController", &req1, nil)
	c.Assert(err, gc.Equals, nil)

	_, err = s.JIMM.JujuManager().GetCloud(context.Background(), user, names.NewCloudTag(cloudName))
	c.Assert(err, gc.ErrorMatches, `cloud "`+cloudName+`" not found`)
}

func (s *jimmSuite) TestCrossModelQuery(c *gc.C) {
	s.AddModel(
		c,
		names.NewUserTag("charlie@canonical.com"),
		petname.Generate(2, "-"),
		names.NewCloudTag(jimmtest.TestE2ECloudName),
		jimmtest.TestE2ECloudRegionName,
		s.Model2.CloudCredential.ResourceTag(),
	)
	model21Name := petname.Generate(2, "-")
	s.AddModel(
		c,
		names.NewUserTag("charlie@canonical.com"),
		model21Name,
		names.NewCloudTag(jimmtest.TestE2ECloudName),
		jimmtest.TestE2ECloudRegionName,
		s.Model2.CloudCredential.ResourceTag(),
	)
	model22Name := petname.Generate(2, "-")
	s.AddModel(
		c,
		names.NewUserTag("charlie@canonical.com"),
		model22Name,
		names.NewCloudTag(jimmtest.TestE2ECloudName),
		jimmtest.TestE2ECloudRegionName,
		s.Model2.CloudCredential.ResourceTag(),
	)

	conn := s.Open(c, nil, "charlie", nil)
	defer conn.Close()
	client := api.NewClient(conn)

	_, err := client.CrossModelQuery(&apiparams.CrossModelQueryRequest{
		Type:  "some-type-not-supported",
		Query: ".",
	})
	c.Assert(err, gc.ErrorMatches, `unable to query models \(invalid query type\)`)

	_, err = client.CrossModelQuery(&apiparams.CrossModelQueryRequest{
		Type:  "jimmsql",
		Query: ".",
	})
	c.Assert(err, gc.ErrorMatches, `(?s).*not implemented \(not implemented\).*`)

	res, err := client.CrossModelQuery(&apiparams.CrossModelQueryRequest{
		Type:  "jq",
		Query: ".",
	})
	c.Assert(err, gc.IsNil)
	c.Assert(res.Results, gc.HasLen, 5)
	c.Assert(res.Errors, gc.HasLen, 0)

	// Query with broken jq, this JQ will run against each model and return the same error
	res, err = client.CrossModelQuery(&apiparams.CrossModelQueryRequest{
		Type:  "jq",
		Query: "dig-lett",
	})
	c.Assert(err, gc.IsNil)
	c.Assert(res.Results, gc.HasLen, 0)
	c.Assert(res.Errors, gc.HasLen, 5)
	for _, errString := range res.Errors {
		c.Assert(errString[0], gc.Equals, "jq error: function not defined: lett/0")
	}

	// Query for two very specific models
	res, err = client.CrossModelQuery(&apiparams.CrossModelQueryRequest{
		Type:  "jq",
		Query: "select((.model.name==\"" + model21Name + "\") or .model.name==\"" + model22Name + "\")",
	})
	c.Assert(err, gc.IsNil)
	c.Assert(res.Results, gc.HasLen, 2)
	c.Assert(res.Errors, gc.HasLen, 0)
}

// TestJimmModelMigration tests that a migration request makes it through to the Juju controller.
// Because our test suite only spins up 1 controller the further we can go is reaching Juju pre-checks which
// detect that a model with the same UUID already exists on the target controller.
func (s *jimmSuite) TestJimmModelMigrationSuperuser(c *gc.C) {
	modelName := petname.Generate(2, "-")
	mt := s.AddModel(
		c,
		names.NewUserTag("charlie@canonical.com"),
		modelName,
		names.NewCloudTag(jimmtest.TestE2ECloudName),
		jimmtest.TestE2ECloudRegionName,
		s.Model2.CloudCredential.ResourceTag(),
	)

	conn := s.Open(c, nil, "alice", nil)
	defer conn.Close()
	client := api.NewClient(conn)

	name, _ := s.GetOneControllerConfig(c)
	res, err := client.MigrateModel(&apiparams.MigrateModelRequest{
		Specs: []apiparams.MigrateModelInfo{
			{TargetModelNameOrUUID: mt.Id(), TargetController: name},
			{TargetModelNameOrUUID: "charlie@canonical.com/" + modelName, TargetController: name},
		},
	})
	c.Assert(err, gc.IsNil)
	c.Assert(res.Results, gc.HasLen, 2)

	item := res.Results[0]
	c.Assert(item.ModelTag, gc.Equals, mt.String())
	c.Assert(item.MigrationId, gc.Equals, "")
	c.Assert(item.Error.Message, gc.Matches, "target prechecks failed: model with same UUID already exists .*")

	item2 := res.Results[1]
	c.Assert(item2.ModelTag, gc.Equals, mt.String())
	c.Assert(item2.MigrationId, gc.Equals, "")
	c.Assert(item2.Error.Message, gc.Matches, "target prechecks failed: model with same UUID already exists .*")
}

func (s *jimmSuite) TestJimmModelMigrationNonSuperuser(c *gc.C) {
	modelName := petname.Generate(2, "-")
	mt := s.AddModel(
		c,
		names.NewUserTag("charlie@canonical.com"),
		modelName,
		names.NewCloudTag(jimmtest.TestE2ECloudName),
		jimmtest.TestE2ECloudRegionName,
		s.Model2.CloudCredential.ResourceTag(),
	)

	conn := s.Open(c, nil, "bob", nil)
	defer conn.Close()
	client := api.NewClient(conn)
	name, _ := s.GetOneControllerConfig(c)
	res, err := client.MigrateModel(&apiparams.MigrateModelRequest{
		Specs: []apiparams.MigrateModelInfo{
			{TargetModelNameOrUUID: mt.Id(), TargetController: name},
		},
	})
	c.Assert(err, gc.IsNil)
	c.Assert(res.Results, gc.HasLen, 1)
	item := res.Results[0]
	c.Assert(item.Error.Message, gc.Matches, "unauthorized access")
}

func (s *jimmSuite) TestVersion(c *gc.C) {
	conn := s.Open(c, nil, "bob", nil)
	defer conn.Close()
	client := api.NewClient(conn)
	versionInfo, err := client.Version()
	c.Assert(err, gc.IsNil)
	c.Assert(versionInfo.Version, gc.Not(gc.Equals), "")
	c.Assert(versionInfo.Commit, gc.Not(gc.Equals), "")
}

func (s *jimmSuite) TestPrepareModelMigration(c *gc.C) {
	conn := s.Open(c, nil, "alice", nil)
	defer conn.Close()
	client := api.NewClient(conn)
	_, conf := s.GetOneControllerConfig(c)

	ctlName := "prepare-model-migration-controller"
	s.AddController(c, ctlName, conf.ToAPIInfo())

	migrationToken, err := client.PrepareModelMigration(&apiparams.PrepareModelMigrationRequest{
		ModelTag:              names.NewModelTag("5650ac3f-8332-437f-874f-089e0e447e7f").String(),
		BackingControllerName: ctlName,
		UserMapping:           map[string]string{"alice": "alice@canonical.com"}, // `{"alice": "alice@canonical.com"}`,
	})
	c.Assert(err, gc.IsNil)
	c.Assert(migrationToken, gc.Not(gc.Equals), "")
}

func (s *jimmSuite) TestListMigrationTargets(c *gc.C) {
	// Add model that could migrate to target
	mt := s.AddModel(
		c,
		names.NewUserTag("charlie@canonical.com"),
		petname.Generate(2, "-"),
		names.NewCloudTag(jimmtest.TestE2ECloudName),
		jimmtest.TestE2ECloudRegionName,
		s.Model2.CloudCredential.ResourceTag(),
	)

	// Add migration target controller
	_, conf := s.GetOneControllerConfig(c)
	info := conf.ToAPIInfo()
	ctl := &dbmodel.Controller{
		UUID:          info.ControllerUUID,
		Name:          "controller-2",
		CACertificate: info.CACert,
		Addresses:     nil,
		CloudName:     jimmtest.TestE2ECloudName,
		TLSHostname:   "juju-apiserver",
		CloudRegion:   jimmtest.TestE2ECloudRegionName,
	}
	ctlCreds := juju.ControllerCreds{
		AdminIdentityName: info.Tag.Id(),
		AdminPassword:     info.Password,
	}
	ctl.Addresses = make(dbmodel.HostPorts, 0, len(info.Addrs))
	for _, addr := range info.Addrs {
		hp, err := network.ParseMachineHostPort(addr)
		c.Assert(err, gc.Equals, nil)
		ctl.Addresses = append(ctl.Addresses, []jujuparams.HostPort{{
			Address: jujuparams.FromMachineAddress(hp.MachineAddress),
			Port:    hp.Port(),
		}})
	}
	err := s.JIMM.JujuManager().AddController(context.Background(), s.AdminUser, ctl, ctlCreds)
	c.Assert(err, gc.Equals, nil)

	conn := s.Open(c, nil, "alice", nil)
	defer conn.Close()

	client := api.NewClient(conn)
	cis, err := client.ListMigrationTargets(&apiparams.ListMigrationTargetsRequest{
		ModelTag: mt.String(),
	})
	c.Assert(err, gc.Equals, nil)

	expectedControllerInfo := []apiparams.ControllerInfo{{
		Name:          "controller-2",
		UUID:          s.Model.Controller.UUID,
		APIAddresses:  info.Addrs,
		CACertificate: info.CACert,
		CloudTag:      names.NewCloudTag(jimmtest.TestE2ECloudName).String(),
		CloudRegion:   jimmtest.TestE2ECloudRegionName,
		AgentVersion:  s.Model.Controller.AgentVersion,
		Status: jujuparams.EntityStatus{
			Status: "available",
		},
	}}

	assertControllerInfos(c, cis, expectedControllerInfo, s.GetControllersConfig(c))
}

// TestUpgradeTo_Unauthorized verifies non-admins cannot call the facade.
func (s *jimmSuite) TestUpgradeTo_Unauthorized(c *gc.C) {
	conn := s.Open(c, nil, "bob", nil)
	defer conn.Close()

	client := api.NewClient(conn)
	req := apiparams.UpgradeToRequest{
		ModelTag:                names.NewModelTag(s.Model2.UUID.String).String(),
		TargetControllerVersion: s.Model.Controller.AgentVersion,
	}
	_, err := client.UpgradeTo(&req)
	c.Assert(err, gc.ErrorMatches, `unauthorized \(unauthorized access\)`)
	c.Assert(jujuparams.IsCodeUnauthorized(err), gc.Equals, true)
}

// TestUpgradeTo_InvalidModelTag verifies invalid model tags are rejected.
func (s *jimmSuite) TestUpgradeTo_InvalidModelTag(c *gc.C) {
	conn := s.Open(c, nil, "alice", nil)
	defer conn.Close()

	client := api.NewClient(conn)
	req := apiparams.UpgradeToRequest{
		ModelTag:                "invalid-model-tag",
		TargetControllerVersion: s.Model.Controller.AgentVersion,
	}
	_, err := client.UpgradeTo(&req)
	c.Assert(err, gc.ErrorMatches, `(invalid model tag "invalid-model-tag": )?"invalid-model-tag" is not a valid tag \(bad request\)`)
}

// TestUpgradeTo_InvalidVersion verifies invalid version strings are rejected.
func (s *jimmSuite) TestUpgradeTo_InvalidVersion(c *gc.C) {
	conn := s.Open(c, nil, "alice", nil)
	defer conn.Close()

	client := api.NewClient(conn)
	req := apiparams.UpgradeToRequest{
		ModelTag:                names.NewModelTag(s.Model2.UUID.String).String(),
		TargetControllerVersion: "not-a-version",
	}
	_, err := client.UpgradeTo(&req)
	c.Assert(err, gc.ErrorMatches, `invalid target controller version "not-a-version": .* \(bad request\)`)
}

// TestUpgradeTo_TargetVersionLowerOrEqual ensures we return a success=false response when the target is <= current.
func (s *jimmSuite) TestUpgradeTo_TargetVersionLower(c *gc.C) {
	conn := s.Open(c, nil, "alice", nil)
	defer conn.Close()

	client := api.NewClient(conn)
	// Use the current controller version to guarantee target <= current.
	req := apiparams.UpgradeToRequest{
		ModelTag:                names.NewModelTag(s.Model2.UUID.String).String(),
		TargetControllerVersion: "1.0.0",
	}
	_, err := client.UpgradeTo(&req)
	c.Assert(err, gc.ErrorMatches, `failed to run upgrade to: failed to prepare for upgrade: target version must be greater than or equal to current version \(bad request\)`)
}

func (s *jimmSuite) TestCreateModelOnTargetController(c *gc.C) {
	conn := s.Open(c, nil, "bob", nil)
	defer conn.Close()

	// Generate unique model names for each test
	generateModelName := func() string {
		return petname.Generate(2, "-")
	}

	controllerName, conf := s.GetOneControllerConfig(c)

	name := generateModelName()
	ownerTag := names.NewUserTag("bob@canonical.com").String()
	cloudTag := names.NewCloudTag(jimmtest.TestE2ECloudName).String()
	credentialTag := "cloudcred-" + jimmtest.TestE2ECloudName + "_bob@canonical.com_cred"

	var mi jujuparams.ModelInfo
	err := conn.APICall("JIMM", 4, "", "AddModelToController", apiparams.AddModelToControllerRequest{
		ModelCreateArgs: jujuparams.ModelCreateArgs{
			Name:               name,
			OwnerTag:           ownerTag,
			CloudTag:           cloudTag,
			CloudCredentialTag: credentialTag,
		},
		ControllerName: controllerName,
	}, &mi)
	c.Assert(err, gc.IsNil)

	model := dbmodel.Model{
		UUID: sql.NullString{String: mi.UUID, Valid: true},
	}
	err = s.JIMM.Database.GetModel(context.Background(), &model)
	c.Assert(err, gc.IsNil)

	// Make sure the model is hosted on the specified controller
	c.Assert(model.Controller.UUID, gc.Equals, conf.UUID)

	c.Assert(err, gc.Equals, nil)
	c.Assert(mi.Name, gc.Equals, name)
	c.Assert(mi.UUID, gc.Not(gc.Equals), "")
	c.Assert(mi.OwnerTag, gc.Equals, ownerTag)
	c.Assert(mi.ControllerUUID, gc.Equals, jimmtest.ControllerUUID)
	c.Assert(mi.Users, gc.Not(gc.HasLen), 0)

	tag, err := names.ParseCloudCredentialTag(mi.CloudCredentialTag)
	c.Assert(err, gc.Equals, nil)
	c.Assert(tag.String(), gc.Equals, credentialTag)

	ct, err := names.ParseCloudTag(cloudTag)
	c.Assert(err, gc.Equals, nil)
	c.Assert(mi.CloudTag, gc.Equals, names.NewCloudTag(ct.Id()).String())
}
