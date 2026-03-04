// Copyright 2025 Canonical.

package testing

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"slices"
	"testing"
	"time"

	petname "github.com/dustinkirkland/golang-petname"
	qt "github.com/frankban/quicktest"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/juju/juju/api/client/modelconfig"
	"github.com/juju/juju/api/client/modelmanager"
	"github.com/juju/juju/cloud"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v5"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/jujuapi"
	"github.com/canonical/jimm/v3/internal/openfga"
	ofganames "github.com/canonical/jimm/v3/internal/openfga/names"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
	"github.com/canonical/jimm/v3/pkg/api"
	apiparams "github.com/canonical/jimm/v3/pkg/api/params"
	jimmversion "github.com/canonical/jimm/v3/version"
)

func TestListControllersAdmin(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupJimmWithControllers(c)

	conn := s.Open(c, nil, "alice", nil)
	defer conn.Close()

	client := api.NewClient(conn)
	cis, err := client.ListControllers()
	c.Assert(err, qt.Equals, nil)
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
func assertControllerInfos(c *qt.C, actual []apiparams.ControllerInfo, expected []apiparams.ControllerInfo, confs *jimmtest.ControllersConfig) {
	c.Check(actual, qt.CmpEquals(
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
					c.Assert(found, qt.Equals, true, qt.Commentf(
						"controller %q: expected address %q not found in APIAddresses %v",
						name, expectedAddr, ci.APIAddresses))
				}
			}
		}
	}
}

func TestListControllersOrdinaryUser(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupJimmWithControllers(c)

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
	c.Assert(err, qt.IsNil)

	err = s.JIMM.Database.AddController(ctx, ctrl1)
	c.Assert(err, qt.IsNil)

	err = s.JIMM.Database.AddController(ctx, ctrl2)
	c.Assert(err, qt.IsNil)

	// Explicitly set access to controllers 0 and 2, but not 1.
	u, err := dbmodel.NewIdentity("alex@canonical.com")
	c.Assert(err, qt.IsNil)

	err = s.JIMM.Database.GetIdentity(ctx, u)
	c.Assert(err, qt.IsNil)

	openfgaUser := openfga.NewUser(u, s.JIMM.OpenFGAClient)

	err = openfgaUser.SetControllerAccess(ctx, names.NewControllerTag(ctrl0.UUID), ofganames.CanAddModelRelation)
	c.Assert(err, qt.IsNil)

	err = openfgaUser.SetControllerAccess(ctx, names.NewControllerTag(ctrl2.UUID), ofganames.CanAddModelRelation)
	c.Assert(err, qt.IsNil)

	conn := s.Open(c, nil, "alex@canonical.com", nil)
	defer conn.Close()

	client := api.NewClient(conn)
	cis, err := client.ListControllers()
	c.Assert(err, qt.Equals, nil)
	c.Check(cis, qt.DeepEquals, []apiparams.ControllerInfo{
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

func TestModelGet(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupJimmWithControllers(c)

	conn := s.Open(c, nil, "alice", nil)
	defer conn.Close()

	client := modelconfig.NewClient(conn)

	jimmCfg, err := client.ModelGet()
	c.Assert(err, qt.IsNil)

	v, ok := jimmCfg["agent-version"]
	c.Assert(ok, qt.Equals, true)
	vers, ok := v.(string)
	c.Assert(ok, qt.Equals, true)
	c.Assert(vers, qt.Equals, jimmversion.ControllerVersion)
}

func TestListControllersUnauthorized(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupJimmWithControllers(c)

	conn := s.Open(c, nil, "abrandnewuserwithnopermissions", nil)
	defer conn.Close()

	client := api.NewClient(conn)
	cis, err := client.ListControllers()
	c.Assert(err, qt.Equals, nil)
	c.Check(cis, qt.DeepEquals, []apiparams.ControllerInfo{})
}

func TestAddControllerPublicAddressWithoutPort(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupJimmWithControllers(c)

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
		c.Assert(err, qt.ErrorMatches, test.expectedError)
		c.Check(ci, qt.DeepEquals, apiparams.ControllerInfo{})
	}
}

func TestAddController(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupJimmWithControllers(c)

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
	c.Assert(err, qt.Equals, nil)
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
	c.Assert(err, qt.ErrorMatches, `failed to add controller: controller "controller-2" already exists \(already exists\)`)
	c.Assert(jujuparams.IsCodeAlreadyExists(err), qt.Equals, true)

	acr.Name = "jimm"
	_, err = client.AddController(&acr)
	c.Assert(err, qt.ErrorMatches, `cannot add a controller with name "jimm" \(bad request\)`)
	c.Assert(jujuparams.IsBadRequest(err), qt.Equals, true)

	conn = s.Open(c, nil, "bob", nil)
	defer conn.Close()
	client = api.NewClient(conn)
	acr.Name = "controller-2"
	_, err = client.AddController(&acr)
	c.Assert(err, qt.ErrorMatches, `unauthorized \(unauthorized access\)`)
	c.Assert(jujuparams.IsCodeUnauthorized(err), qt.Equals, true)
}

func TestRemoveAndAddController(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupJimmWithControllers(c)

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
	c.Assert(err, qt.Equals, nil)
	_, err = client.RemoveController(&apiparams.RemoveControllerRequest{Name: acr.Name, Force: true})
	c.Assert(err, qt.Equals, nil)
	ciNew, err := client.AddController(&acr)
	c.Assert(err, qt.Equals, nil)
	c.Assert(ci, qt.DeepEquals, ciNew)
}

func TestAddControllerCustomTLSHostname(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupJimmWithControllers(c)

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
	c.Assert(err, qt.ErrorMatches, "failed to add controller: failed to dial the controller.*")
	acr.TLSHostname = "juju-apiserver"
	ci, err := client.AddController(&acr)
	c.Assert(err, qt.IsNil)
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

func TestRemoveController(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupJimmWithControllers(c)
	model := s.CreateModelForBob(c)

	conn := s.Open(c, nil, "alice", nil)
	defer conn.Close()
	client := api.NewClient(conn)

	name, conf := s.GetOneControllerConfig(c)

	_, err := client.RemoveController(&apiparams.RemoveControllerRequest{
		Name: name,
	})
	c.Check(err, qt.ErrorMatches, `controller still has models.*`)
	c.Check(jujuparams.ErrCode(err), qt.Equals, apiparams.CodeStillAlive)
	s.DestroyModelAndDeleteFromDatabase(c, model.ResourceTag())

	conn2 := s.Open(c, nil, "bob", nil)
	defer conn2.Close()
	client2 := api.NewClient(conn2)

	_, err = client2.RemoveController(&apiparams.RemoveControllerRequest{
		Name: name,
	})
	c.Check(err, qt.ErrorMatches, `unauthorized \(unauthorized access\)`)
	c.Check(jujuparams.ErrCode(err), qt.Equals, jujuparams.CodeUnauthorized)

	ci, err := client.RemoveController(&apiparams.RemoveControllerRequest{
		Name: name,
	})
	c.Assert(err, qt.Equals, nil)
	ciExpected := apiparams.ControllerInfo{
		Name:          name,
		UUID:          conf.UUID,
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

func TestSetControllerDeprecated(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupJimmWithControllers(c)
	model := s.CreateModelForBob(c)

	conn := s.Open(c, nil, "alice", nil)
	defer conn.Close()
	client := api.NewClient(conn)

	name, conf := s.GetOneControllerConfig(c)

	ci, err := client.SetControllerDeprecated(&apiparams.SetControllerDeprecatedRequest{
		Name:       name,
		Deprecated: true,
	})
	c.Assert(err, qt.Equals, nil)
	ciExpected := apiparams.ControllerInfo{
		Name:          name,
		UUID:          conf.UUID,
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
	c.Assert(err, qt.Equals, nil)
	ciExpected = apiparams.ControllerInfo{
		Name:          name,
		UUID:          conf.UUID,
		APIAddresses:  conf.ToAPIInfo().Addrs,
		CACertificate: conf.ToAPIInfo().CACert,
		CloudTag:      names.NewCloudTag(jimmtest.TestE2ECloudName).String(),
		CloudRegion:   jimmtest.TestE2ECloudRegionName,
		AgentVersion:  model.Controller.AgentVersion,
		Status: jujuparams.EntityStatus{
			Status: "available",
		},
	}
	assertControllerInfos(c, []apiparams.ControllerInfo{ci}, []apiparams.ControllerInfo{ciExpected}, s.GetControllersConfig(c))

	_, err = client.SetControllerDeprecated(&apiparams.SetControllerDeprecatedRequest{
		Name:       "controller-2",
		Deprecated: true,
	})
	c.Check(err, qt.ErrorMatches, `controller not found \(not found\)`)
	c.Check(jujuparams.IsCodeNotFound(err), qt.Equals, true)

	conn = s.Open(c, nil, "bob", nil)
	defer conn.Close()
	client = api.NewClient(conn)
	_, err = client.SetControllerDeprecated(&apiparams.SetControllerDeprecatedRequest{
		Name:       "controller-1",
		Deprecated: true,
	})
	c.Check(err, qt.ErrorMatches, `unauthorized \(unauthorized access\)`)
	c.Check(jujuparams.IsCodeUnauthorized(err), qt.Equals, true)
}

func TestAuditLog(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupJimmWithControllers(c)
	model := s.CreateModelForBob(c)

	conn := s.Open(c, nil, "bob", nil)
	defer conn.Close()
	client := api.NewClient(conn)

	_, err := client.FindAuditEvents(&apiparams.FindAuditEventsRequest{})
	c.Check(err, qt.ErrorMatches, `unauthorized \(unauthorized access\)`)
	c.Check(jujuparams.ErrCode(err), qt.Equals, jujuparams.CodeUnauthorized)

	mmclient := modelmanager.NewClient(conn)
	zeroDuration := time.Duration(0)
	err = mmclient.DestroyModel(model.ResourceTag(), nil, nil, nil, &zeroDuration)
	c.Assert(err, qt.Equals, nil)

	conn2 := s.Open(c, nil, "alice", nil)
	defer conn2.Close()
	client2 := api.NewClient(conn2)

	evs, err := client2.FindAuditEvents(&apiparams.FindAuditEventsRequest{})
	c.Assert(err, qt.Equals, nil)

	c.Assert(len(evs.Events), qt.Equals, 9)

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
	c.Check(evs, qt.DeepEquals, expectedEvents)

	// alice can grant bob access to audit log entries
	err = client2.GrantAuditLogAccess(&apiparams.AuditLogAccessRequest{
		UserTag: names.NewUserTag("bob@canonical.com").String(),
	})
	c.Assert(err, qt.Equals, nil)

	// now bob can access audit events as well
	conn3 := s.Open(c, nil, "bob", nil)
	defer conn3.Close()
	client3 := api.NewClient(conn3)

	evs, err = client3.FindAuditEvents(&apiparams.FindAuditEventsRequest{})
	evs.Events = truncatedEvents
	c.Assert(err, qt.Equals, nil)
	c.Check(evs, qt.DeepEquals, expectedEvents)
}

func TestAuditLogFilterByMethod(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupJimmWithControllers(c)

	conn := s.Open(c, nil, "alice", nil)
	defer conn.Close()
	client := api.NewClient(conn)
	evs, err := client.FindAuditEvents(&apiparams.FindAuditEventsRequest{Method: "Deploy"})
	c.Assert(err, qt.Equals, nil)
	c.Assert(len(evs.Events), qt.Equals, 0)
}

func TestFullModelStatus(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupJimmWithControllers(c)

	charlieModel := s.CreateModelForCharlie(c)

	conn := s.Open(c, nil, "bob@canonical.com", nil)
	defer conn.Close()
	client := api.NewClient(conn)

	_, err := client.FullModelStatus(&apiparams.FullModelStatusRequest{
		ModelTag: "invalid-model-tag",
	})
	c.Assert(err, qt.ErrorMatches, `"invalid-model-tag" is not a valid tag \(bad request\)`)

	_, err = client.FullModelStatus(&apiparams.FullModelStatusRequest{
		ModelTag: charlieModel.ResourceTag().String(),
	})
	c.Assert(err, qt.ErrorMatches, "unauthorized.*")

	conn = s.Open(c, nil, "alice@canonical.com", nil)
	defer conn.Close()
	client = api.NewClient(conn)

	status, err := client.FullModelStatus(&apiparams.FullModelStatusRequest{
		ModelTag: charlieModel.ResourceTag().String(),
	})
	c.Assert(err, qt.Equals, nil)
	c.Assert(status,
		qt.CmpEquals(
			cmpopts.EquateEmpty(),
			cmpopts.IgnoreTypes(&time.Time{}),
			cmpopts.IgnoreFields(jujuparams.ModelStatusInfo{}, "Version"),
		),
		jujuparams.FullStatus{
			Model: jujuparams.ModelStatusInfo{
				Name:        charlieModel.Name,
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

func TestUpdateMigratedModel(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupJimmWithControllers(c)
	model2 := s.CreateModelForCharlie(c)

	// Open the API connection as user "bob".
	conn := s.Open(c, nil, "bob", nil)
	defer conn.Close()

	req := apiparams.UpdateMigratedModelRequest{
		ModelTag:         names.NewModelTag(model2.UUID.String).String(),
		TargetController: model2.Controller.Name,
	}
	err := conn.APICall("JIMM", 4, "", "UpdateMigratedModel", &req, nil)
	c.Assert(err, qt.ErrorMatches, `unauthorized \(unauthorized access\)`)

	// Open the API connection as user "alice".
	conn = s.Open(c, nil, "alice", nil)
	defer conn.Close()

	req = apiparams.UpdateMigratedModelRequest{
		ModelTag:         names.NewModelTag(model2.UUID.String).String(),
		TargetController: model2.Controller.Name,
	}
	err = conn.APICall("JIMM", 4, "", "UpdateMigratedModel", &req, nil)
	c.Assert(err, qt.Equals, nil)

	req = apiparams.UpdateMigratedModelRequest{
		ModelTag:         "invalid-model-tag",
		TargetController: model2.Controller.Name,
	}
	err = conn.APICall("JIMM", 4, "", "UpdateMigratedModel", &req, nil)
	c.Assert(err, qt.ErrorMatches, `"invalid-model-tag" is not a valid tag \(bad request\)`)
}

func TestImportModel(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupJimmWithControllers(c)
	model2 := s.CreateModelForCharlie(c)

	// Open the API connection as user "bob".
	conn := s.Open(c, nil, "bob", nil)
	defer conn.Close()
	controllerName := model2.Controller.Name

	err := s.JIMM.OpenFGAClient.RemoveControllerModel(context.Background(), model2.Controller.ResourceTag(), model2.ResourceTag())
	c.Assert(err, qt.Equals, nil)
	err = s.JIMM.Database.DeleteModel(context.Background(), model2)
	c.Assert(err, qt.Equals, nil)

	req := apiparams.ImportModelRequest{
		Controller: controllerName,
		ModelTag:   model2.Tag().String(),
		Owner:      "",
	}
	err = conn.APICall("JIMM", 4, "", "ImportModel", &req, nil)
	c.Assert(err, qt.ErrorMatches, `unauthorized \(unauthorized access\)`)

	// Open the API connection as user "alice".
	conn = s.Open(c, nil, "alice", nil)
	defer conn.Close()

	err = conn.APICall("JIMM", 4, "", "ImportModel", &req, nil)
	c.Assert(err, qt.Equals, nil)

	var importedModel dbmodel.Model
	importedModel.SetTag(model2.ResourceTag())
	err = s.JIMM.Database.GetModel(context.Background(), &importedModel)
	c.Assert(err, qt.Equals, nil)
	c.Check(importedModel.CreatedAt.After(model2.CreatedAt), qt.Equals, true)

	req = apiparams.ImportModelRequest{
		Controller: controllerName,
		ModelTag:   "invalid-model-tag",
	}
	err = conn.APICall("JIMM", 4, "", "ImportModel", &req, nil)
	c.Assert(err, qt.ErrorMatches, `"invalid-model-tag" is not a valid tag \(bad request\)`)
}

func TestAddCloudToController(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupJimmWithControllers(c)

	ctx := context.Background()

	u, err := dbmodel.NewIdentity("alice@canonical.com")
	c.Assert(err, qt.IsNil)

	err = s.JIMM.Database.GetIdentity(ctx, u)
	c.Assert(err, qt.IsNil)

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
	c.Assert(err, qt.Equals, nil)

	user := openfga.NewUser(u, s.OFGAClient)

	cloud, err := s.JIMM.JujuManager.GetCloud(context.Background(), user, names.NewCloudTag(cloudName))
	c.Assert(err, qt.IsNil)
	c.Assert(cloud.Name, qt.DeepEquals, cloudName)
	c.Assert(cloud.Type, qt.DeepEquals, "kubernetes")

	req1 := apiparams.RemoveCloudFromControllerRequest{
		CloudTag:       names.NewCloudTag(cloudName).String(),
		ControllerName: name,
	}
	err = conn.APICall("JIMM", 4, "", "RemoveCloudFromController", &req1, nil)
	c.Assert(err, qt.Equals, nil)
}

func TestAddExistingCloudToController(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupJimmWithControllers(c)

	ctx := context.Background()

	u, err := dbmodel.NewIdentity("alice@canonical.com")
	c.Assert(err, qt.IsNil)

	err = s.JIMM.Database.GetIdentity(ctx, u)
	c.Assert(err, qt.IsNil)

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
	c.Assert(err, qt.Equals, nil)
	user := openfga.NewUser(u, s.OFGAClient)
	cloud, err := s.JIMM.JujuManager.GetCloud(context.Background(), user, names.NewCloudTag(cloudName))
	c.Assert(err, qt.IsNil)
	c.Assert(cloud.Name, qt.DeepEquals, cloudName)
	c.Assert(cloud.Type, qt.DeepEquals, "MAAS")
	// Simulate the cloud being present on the Juju controller but not in JIMM.
	err = s.JIMM.Database.DeleteCloud(ctx, &cloud)
	c.Assert(err, qt.IsNil)
	cloud, err = s.JIMM.JujuManager.GetCloud(context.Background(), user, names.NewCloudTag(cloudName))
	c.Assert(err, qt.Not(qt.IsNil))
	c.Assert(errors.ErrorCode(err), qt.Equals, errors.CodeNotFound)
	err = conn.APICall("JIMM", 4, "", "AddCloudToController", &req, nil)
	c.Assert(err, qt.Equals, nil)
	cloud, err = s.JIMM.JujuManager.GetCloud(context.Background(), user, names.NewCloudTag(cloudName))
	c.Assert(err, qt.IsNil)
	c.Assert(cloud.Name, qt.DeepEquals, cloudName)
	c.Assert(cloud.Type, qt.DeepEquals, "MAAS")

	req1 := apiparams.RemoveCloudFromControllerRequest{
		CloudTag:       names.NewCloudTag(cloudName).String(),
		ControllerName: name,
	}
	err = conn.APICall("JIMM", 4, "", "RemoveCloudFromController", &req1, nil)
	c.Assert(err, qt.Equals, nil)

}

func TestRemoveCloudFromController(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupJimmWithControllers(c)

	ctx := context.Background()

	u, err := dbmodel.NewIdentity("alice@canonical.com")
	c.Assert(err, qt.IsNil)

	err = s.JIMM.Database.GetIdentity(ctx, u)
	c.Assert(err, qt.IsNil)

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
	c.Assert(err, qt.Equals, nil)

	user := openfga.NewUser(u, s.OFGAClient)

	_, err = s.JIMM.JujuManager.GetCloud(context.Background(), user, names.NewCloudTag(cloudName))
	c.Assert(err, qt.Equals, nil)

	req1 := apiparams.RemoveCloudFromControllerRequest{
		CloudTag:       names.NewCloudTag(cloudName).String(),
		ControllerName: name,
	}
	err = conn.APICall("JIMM", 4, "", "RemoveCloudFromController", &req1, nil)
	c.Assert(err, qt.Equals, nil)

	_, err = s.JIMM.JujuManager.GetCloud(context.Background(), user, names.NewCloudTag(cloudName))
	c.Assert(err, qt.ErrorMatches, `cloud "`+cloudName+`" not found`)
}

func TestCrossModelQuery(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupJimmWithControllers(c)

	_ = s.CreateModelForCharlie(c)
	model2 := s.CreateModelForCharlie(c)
	model3 := s.CreateModelForCharlie(c)

	conn := s.Open(c, nil, "charlie", nil)
	defer conn.Close()
	client := api.NewClient(conn)

	_, err := client.CrossModelQuery(&apiparams.CrossModelQueryRequest{
		Type:  "some-type-not-supported",
		Query: ".",
	})
	c.Assert(err, qt.ErrorMatches, `unable to query models \(invalid query type\)`)

	_, err = client.CrossModelQuery(&apiparams.CrossModelQueryRequest{
		Type:  "jimmsql",
		Query: ".",
	})
	c.Assert(err, qt.ErrorMatches, `(?s).*not implemented \(not implemented\).*`)

	res, err := client.CrossModelQuery(&apiparams.CrossModelQueryRequest{
		Type:  "jq",
		Query: ".",
	})
	c.Assert(err, qt.IsNil)
	c.Assert(res.Results, qt.HasLen, 3)
	c.Assert(res.Errors, qt.HasLen, 0)

	// Query with broken jq, this JQ will run against each model and return the same error
	res, err = client.CrossModelQuery(&apiparams.CrossModelQueryRequest{
		Type:  "jq",
		Query: "dig-lett",
	})
	c.Assert(err, qt.IsNil)
	c.Assert(res.Results, qt.HasLen, 0)
	c.Assert(res.Errors, qt.HasLen, 3)
	for _, errString := range res.Errors {
		c.Assert(errString[0], qt.Equals, "jq error: function not defined: lett/0")
	}

	// Query for two very specific models
	res, err = client.CrossModelQuery(&apiparams.CrossModelQueryRequest{
		Type:  "jq",
		Query: "select((.model.name==\"" + model2.Name + "\") or .model.name==\"" + model3.Name + "\")",
	})
	c.Assert(err, qt.IsNil)
	c.Assert(res.Results, qt.HasLen, 2)
	c.Assert(res.Errors, qt.HasLen, 0)
}

// TestJimmModelMigration tests that a migration request makes it through to the Juju controller.
// Because our test suite only spins up 1 controller the further we can go is reaching Juju pre-checks which
// detect that a model with the same UUID already exists on the target controller.
func TestJimmModelMigrationSuperuser(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupJimmWithControllers(c)

	model := s.CreateModelForCharlie(c)
	ctrlName := model.Controller.Name

	conn := s.Open(c, nil, "alice", nil)
	defer conn.Close()
	client := api.NewClient(conn)

	res, err := client.MigrateModel(&apiparams.MigrateModelRequest{
		Specs: []apiparams.MigrateModelInfo{
			{TargetModelNameOrUUID: model.UUID.String, TargetController: ctrlName},
			{TargetModelNameOrUUID: "charlie@canonical.com/" + model.Name, TargetController: ctrlName},
		},
	})
	c.Assert(err, qt.IsNil)
	c.Assert(res.Results, qt.HasLen, 2)

	item := res.Results[0]
	c.Assert(item.ModelTag, qt.Equals, model.ResourceTag().String())
	c.Assert(item.MigrationId, qt.Equals, "")
	c.Assert(item.Error.Message, qt.Matches, "target prechecks failed: model with same UUID already exists .*")

	item2 := res.Results[1]
	c.Assert(item2.ModelTag, qt.Equals, model.ResourceTag().String())
	c.Assert(item2.MigrationId, qt.Equals, "")
	c.Assert(item2.Error.Message, qt.Matches, "target prechecks failed: model with same UUID already exists .*")
}

func TestJimmModelMigrationNonSuperuser(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupJimmWithControllers(c)

	model := s.CreateModelForCharlie(c)

	conn := s.Open(c, nil, "bob@canonical.com", nil)
	defer conn.Close()
	client := api.NewClient(conn)
	ctrlName, _ := s.GetOneControllerConfig(c)
	res, err := client.MigrateModel(&apiparams.MigrateModelRequest{
		Specs: []apiparams.MigrateModelInfo{
			{TargetModelNameOrUUID: model.UUID.String, TargetController: ctrlName},
		},
	})
	c.Assert(err, qt.IsNil)
	c.Assert(res.Results, qt.HasLen, 1)
	item := res.Results[0]
	c.Assert(item.Error.Message, qt.Matches, "unauthorized access")
}

func TestVersion(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupJimmWithControllers(c)

	conn := s.Open(c, nil, "bob", nil)
	defer conn.Close()
	client := api.NewClient(conn)
	versionInfo, err := client.Version()
	c.Assert(err, qt.IsNil)
	c.Assert(versionInfo.Version, qt.Not(qt.Equals), "")
	c.Assert(versionInfo.Commit, qt.Not(qt.Equals), "")
}

func TestPrepareModelMigration(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupJimmWithControllers(c)

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
	c.Assert(err, qt.IsNil)
	c.Assert(migrationToken, qt.Not(qt.Equals), "")
}

func TestListMigrationTargets(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupJimmWithControllers(c)

	// Add model and verify other controllers are listed as migration targets.
	model := s.CreateModelForCharlie(c)

	confs := s.GetControllersConfig(c)
	otherControllers := []apiparams.ControllerInfo{}
	for name, conf := range confs.Controllers {
		if name == model.Controller.Name {
			continue
		}
		otherControllers = append(otherControllers, apiparams.ControllerInfo{
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

	conn := s.Open(c, nil, "alice", nil)
	defer conn.Close()

	client := api.NewClient(conn)
	cis, err := client.ListMigrationTargets(&apiparams.ListMigrationTargetsRequest{
		ModelTag: model.ResourceTag().String(),
	})
	c.Assert(err, qt.Equals, nil)

	assertControllerInfos(c, cis, otherControllers, s.GetControllersConfig(c))
}

// TestUpgradeTo_Unauthorized verifies non-admins cannot call the facade.
func TestUpgradeTo_Unauthorized(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupJimmWithControllers(c)
	model := s.CreateModelForBob(c)
	model2 := s.CreateModelForCharlie(c)

	conn := s.Open(c, nil, "bob", nil)
	defer conn.Close()

	client := api.NewClient(conn)
	req := apiparams.UpgradeToRequest{
		ModelTag:             names.NewModelTag(model2.UUID.String).String(),
		TargetControllerName: model.Controller.Name,
	}
	_, err := client.UpgradeTo(&req)
	c.Assert(err, qt.ErrorMatches, `unauthorized \(unauthorized access\)`)
	c.Assert(jujuparams.IsCodeUnauthorized(err), qt.Equals, true)
}

// TestUpgradeTo_InvalidModelTag verifies invalid model tags are rejected.
func TestUpgradeTo_InvalidModelTag(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupJimmWithControllers(c)
	model := s.CreateModelForBob(c)

	conn := s.Open(c, nil, "alice", nil)
	defer conn.Close()

	client := api.NewClient(conn)
	req := apiparams.UpgradeToRequest{
		ModelTag:             "invalid-model-tag",
		TargetControllerName: model.Controller.Name,
	}
	_, err := client.UpgradeTo(&req)
	c.Assert(err, qt.ErrorMatches, `(invalid model tag "invalid-model-tag": )?"invalid-model-tag" is not a valid tag \(bad request\)`)
}

// TestUpgradeTo_InvalidController verifies invalid controllers are rejected.
func TestUpgradeTo_InvalidController(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupJimmWithControllers(c)
	model2 := s.CreateModelForCharlie(c)

	conn := s.Open(c, nil, "alice", nil)
	defer conn.Close()

	client := api.NewClient(conn)
	req := apiparams.UpgradeToRequest{
		ModelTag:             names.NewModelTag(model2.UUID.String).String(),
		TargetControllerName: "does-not-exist",
	}
	_, err := client.UpgradeTo(&req)
	c.Assert(err, qt.ErrorMatches, regexp.QuoteMeta(`failed to run upgrade to: target controller does-not-exist is not a valid migration target for this model (bad request)`))
}

func TestCreateModelOnTargetController(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupJimmWithControllers(c)

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
	c.Assert(err, qt.IsNil)

	model := dbmodel.Model{
		UUID: sql.NullString{String: mi.UUID, Valid: true},
	}
	err = s.JIMM.Database.GetModel(context.Background(), &model)
	c.Assert(err, qt.IsNil)

	// Make sure the model is hosted on the specified controller
	c.Assert(model.Controller.UUID, qt.Equals, conf.UUID)

	c.Assert(err, qt.Equals, nil)
	c.Assert(mi.Name, qt.Equals, name)
	c.Assert(mi.UUID, qt.Not(qt.Equals), "")
	c.Assert(mi.OwnerTag, qt.Equals, ownerTag)
	c.Assert(mi.ControllerUUID, qt.Equals, jimmtest.ControllerUUID)
	c.Assert(mi.Users, qt.Not(qt.HasLen), 0)

	tag, err := names.ParseCloudCredentialTag(mi.CloudCredentialTag)
	c.Assert(err, qt.Equals, nil)
	c.Assert(tag.String(), qt.Equals, credentialTag)

	ct, err := names.ParseCloudTag(cloudTag)
	c.Assert(err, qt.Equals, nil)
	c.Assert(mi.CloudTag, qt.Equals, names.NewCloudTag(ct.Id()).String())
}

func TestModelControllerInfo(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupJimmWithControllers(c)
	model := s.CreateModelForBob(c)

	conn := s.Open(c, nil, "alice", nil)
	defer conn.Close()

	client := api.NewClient(conn)

	modelControllerInfo, err := client.ModelControllerInfo(model.UUID.String)
	c.Assert(err, qt.IsNil)
	c.Assert(modelControllerInfo, qt.DeepEquals, &apiparams.ModelControllerInfo{
		ModelName:      model.Name,
		ModelUUID:      model.UUID.String,
		ControllerName: model.Controller.Name,
		ControllerUUID: model.Controller.UUID,
	})

	modelControllerInfo, err = client.ModelControllerInfo(fmt.Sprintf("%s/%s", model.OwnerIdentityName, model.Name))
	c.Assert(err, qt.IsNil)
	c.Assert(modelControllerInfo, qt.DeepEquals, &apiparams.ModelControllerInfo{
		ModelName:      model.Name,
		ModelUUID:      model.UUID.String,
		ControllerName: model.Controller.Name,
		ControllerUUID: model.Controller.UUID,
	})
}

func TestPurgeLogs(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupJimmWithControllers(c)

	ctx := context.Background()
	relativeNow := time.Now().AddDate(-1, 0, 0)
	ale := dbmodel.AuditLogEntry{
		Time:        relativeNow.UTC().Round(time.Millisecond),
		IdentityTag: names.NewUserTag("alice@canonical.com").String(),
	}
	ale_past := dbmodel.AuditLogEntry{
		Time:        relativeNow.AddDate(0, 0, -1).UTC().Round(time.Millisecond),
		IdentityTag: names.NewUserTag("alice@canonical.com").String(),
	}
	ale_future := dbmodel.AuditLogEntry{
		Time:        relativeNow.AddDate(0, 0, 5).UTC().Round(time.Millisecond),
		IdentityTag: names.NewUserTag("alice@canonical.com").String(),
	}

	err := s.JIMM.Database.Migrate(context.Background())
	c.Assert(err, qt.IsNil)
	err = s.JIMM.Database.AddAuditLogEntry(ctx, &ale)
	c.Assert(err, qt.IsNil)
	err = s.JIMM.Database.AddAuditLogEntry(ctx, &ale_past)
	c.Assert(err, qt.IsNil)
	err = s.JIMM.Database.AddAuditLogEntry(ctx, &ale_future)
	c.Assert(err, qt.IsNil)

	tomorrow := relativeNow.AddDate(0, 0, 1)

	// alice is superuser
	conn := s.Open(c, nil, "alice", nil)
	defer conn.Close()

	client := api.NewClient(conn)
	resp, err := client.PurgeLogs(&apiparams.PurgeLogsRequest{
		Date: tomorrow,
	})
	// check that logs have been deleted
	c.Assert(err, qt.IsNil)
	c.Assert(resp.DeletedCount, qt.Equals, int64(2))
}

func TestPurgeLogs_NotAdmin(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupJimmWithControllers(c)

	// bob is not a superuser
	conn := s.Open(c, nil, "bob", nil)
	defer conn.Close()

	client := api.NewClient(conn)
	_, err := client.PurgeLogs(&apiparams.PurgeLogsRequest{
		Date: time.Now(),
	})
	c.Assert(err, qt.ErrorMatches, `unauthorized \(unauthorized access\)`)
}

func TestJobInfo(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupJimmWithControllers(c)

	conn := s.Open(c, nil, "alice", nil)
	defer conn.Close()

	client := api.NewClient(conn)

	req := apiparams.JobInfoRequest{JobID: "123"}
	_, err := client.JobInfo(&req)
	c.Assert(err, qt.ErrorMatches, `failed to get job info: not found`)
}
