// Copyright 2024 Canonical.

package jujuapi_test

import (
	"context"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/juju/juju/api/client/modelmanager"
	"github.com/juju/juju/cloud"
	jujuparams "github.com/juju/juju/rpc/params"
	jujuversion "github.com/juju/juju/version"
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/jujuapi"
	"github.com/canonical/jimm/v3/internal/openfga"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
	"github.com/canonical/jimm/v3/pkg/api"
	apiparams "github.com/canonical/jimm/v3/pkg/api/params"
)

type jimmSuite struct {
	websocketSuite
}

var _ = gc.Suite(&jimmSuite{})

func (s *jimmSuite) TestListControllers(c *gc.C) {
	s.AddController(c, "controller-0", s.APIInfo(c))
	s.AddController(c, "controller-2", s.APIInfo(c))

	conn := s.open(c, nil, "alice")
	defer conn.Close()

	client := api.NewClient(conn)
	cis, err := client.ListControllers()
	c.Assert(err, gc.Equals, nil)
	c.Check(cis, jc.DeepEquals, []apiparams.ControllerInfo{{
		Name:          "controller-0",
		UUID:          s.Model.Controller.UUID,
		APIAddresses:  s.APIInfo(c).Addrs,
		CACertificate: s.APIInfo(c).CACert,
		CloudTag:      names.NewCloudTag(jimmtest.TestCloudName).String(),
		CloudRegion:   jimmtest.TestCloudRegionName,
		AgentVersion:  s.Model.Controller.AgentVersion,
		Status: jujuparams.EntityStatus{
			Status: "available",
		},
	}, {
		Name:          "controller-1",
		UUID:          s.Model.Controller.UUID,
		APIAddresses:  s.APIInfo(c).Addrs,
		CACertificate: s.APIInfo(c).CACert,
		CloudTag:      names.NewCloudTag(jimmtest.TestCloudName).String(),
		CloudRegion:   jimmtest.TestCloudRegionName,
		AgentVersion:  s.Model.Controller.AgentVersion,
		Status: jujuparams.EntityStatus{
			Status: "available",
		},
	}, {
		Name:          "controller-2",
		UUID:          s.Model.Controller.UUID,
		APIAddresses:  s.APIInfo(c).Addrs,
		CACertificate: s.APIInfo(c).CACert,
		CloudTag:      names.NewCloudTag(jimmtest.TestCloudName).String(),
		CloudRegion:   jimmtest.TestCloudRegionName,
		AgentVersion:  s.Model.Controller.AgentVersion,
		Status: jujuparams.EntityStatus{
			Status: "available",
		},
	}})
}

func (s *jimmSuite) TestListControllersUnauthorized(c *gc.C) {
	s.AddController(c, "controller-0", s.APIInfo(c))
	s.AddController(c, "controller-2", s.APIInfo(c))

	conn := s.open(c, nil, "bob")
	defer conn.Close()

	client := api.NewClient(conn)
	cis, err := client.ListControllers()
	c.Assert(err, gc.Equals, nil)
	c.Check(cis, jc.DeepEquals, []apiparams.ControllerInfo{{
		Name:         "jaas",
		UUID:         jimmtest.ControllerUUID,
		AgentVersion: s.Model.Controller.AgentVersion,
		Status: jujuparams.EntityStatus{
			Status: "available",
		},
	}})
}

func (s *jimmSuite) TestAddControllerPublicAddressWithoutPort(c *gc.C) {
	conn := s.open(c, nil, "alice")
	defer conn.Close()
	client := api.NewClient(conn)

	tests := []struct {
		req           apiparams.AddControllerRequest
		expectedError string
	}{{
		req: apiparams.AddControllerRequest{
			Name:          "controller-2",
			PublicAddress: "controller.test.com",
			CACertificate: s.APIInfo(c).CACert,
		},
		expectedError: `address controller.test.com: missing port in address \(bad request\)`,
	}, {
		req: apiparams.AddControllerRequest{
			Name:          "controller-2",
			PublicAddress: ":8080",
			CACertificate: s.APIInfo(c).CACert,
		},
		expectedError: `address :8080: host not specified in public address \(bad request\)`,
	}, {
		req: apiparams.AddControllerRequest{
			Name:          "controller-2",
			PublicAddress: "localhost:",
			CACertificate: s.APIInfo(c).CACert,
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
	conn := s.open(c, nil, "alice")
	defer conn.Close()
	client := api.NewClient(conn)

	info := s.APIInfo(c)

	acr := apiparams.AddControllerRequest{
		UUID:          info.ControllerUUID,
		Name:          "controller-2",
		APIAddresses:  info.Addrs,
		CACertificate: info.CACert,
		Username:      info.Tag.Id(),
		Password:      info.Password,
	}

	ci, err := client.AddController(&acr)
	c.Assert(err, gc.Equals, nil)
	c.Assert(ci, jc.DeepEquals, apiparams.ControllerInfo{
		Name:          "controller-2",
		UUID:          info.ControllerUUID,
		APIAddresses:  info.Addrs,
		CACertificate: info.CACert,
		CloudTag:      names.NewCloudTag(jimmtest.TestCloudName).String(),
		CloudRegion:   jimmtest.TestCloudRegionName,
		AgentVersion:  s.Model.Controller.AgentVersion,
		Status: jujuparams.EntityStatus{
			Status: "available",
		},
	})

	_, err = client.AddController(&acr)
	c.Assert(err, gc.ErrorMatches, `controller "controller-2" already exists \(already exists\)`)
	c.Assert(jujuparams.IsCodeAlreadyExists(err), gc.Equals, true)

	conn = s.open(c, nil, "bob")
	defer conn.Close()
	client = api.NewClient(conn)
	acr.Name = "controller-2"
	_, err = client.AddController(&acr)
	c.Assert(err, gc.ErrorMatches, `unauthorized \(unauthorized access\)`)
	c.Assert(jujuparams.IsCodeUnauthorized(err), gc.Equals, true)

	acr.Name = "jimm"
	_, err = client.AddController(&acr)
	c.Assert(err, gc.ErrorMatches, `cannot add a controller with name "jimm" \(bad request\)`)
	c.Assert(jujuparams.IsBadRequest(err), gc.Equals, true)
}

func (s *jimmSuite) TestRemoveAndAddController(c *gc.C) {
	conn := s.open(c, nil, "alice")
	defer conn.Close()
	client := api.NewClient(conn)

	info := s.APIInfo(c)

	acr := apiparams.AddControllerRequest{
		UUID:          info.ControllerUUID,
		Name:          "controller-2",
		APIAddresses:  info.Addrs,
		CACertificate: info.CACert,
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
	conn := s.open(c, nil, "alice")
	defer conn.Close()
	client := api.NewClient(conn)

	info := s.APIInfo(c)

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
	c.Assert(err, gc.ErrorMatches, "failed to dial the controller")
	acr.TLSHostname = "juju-apiserver"
	ci, err := client.AddController(&acr)
	c.Assert(err, gc.IsNil)
	c.Assert(ci, jc.DeepEquals, apiparams.ControllerInfo{
		Name:          "controller-2",
		UUID:          info.ControllerUUID,
		APIAddresses:  info.Addrs,
		CACertificate: info.CACert,
		CloudTag:      names.NewCloudTag(jimmtest.TestCloudName).String(),
		CloudRegion:   jimmtest.TestCloudRegionName,
		AgentVersion:  s.Model.Controller.AgentVersion,
		Status: jujuparams.EntityStatus{
			Status: "available",
		},
	})

}

func (s *jimmSuite) TestRemoveController(c *gc.C) {
	conn := s.open(c, nil, "alice")
	defer conn.Close()
	client := api.NewClient(conn)

	_, err := client.RemoveController(&apiparams.RemoveControllerRequest{
		Name: "controller-1",
	})
	c.Check(err, gc.ErrorMatches, `controller is still alive \(still alive\)`)
	c.Check(jujuparams.ErrCode(err), gc.Equals, apiparams.CodeStillAlive)

	conn2 := s.open(c, nil, "bob")
	defer conn2.Close()
	client2 := api.NewClient(conn2)

	_, err = client2.RemoveController(&apiparams.RemoveControllerRequest{
		Name: "controller-1",
	})
	c.Check(err, gc.ErrorMatches, `unauthorized \(unauthorized access\)`)
	c.Check(jujuparams.ErrCode(err), gc.Equals, jujuparams.CodeUnauthorized)

	ci, err := client.RemoveController(&apiparams.RemoveControllerRequest{
		Name:  "controller-1",
		Force: true,
	})
	c.Assert(err, gc.Equals, nil)
	c.Check(ci, jc.DeepEquals, apiparams.ControllerInfo{
		Name:          "controller-1",
		UUID:          s.Model.Controller.UUID,
		APIAddresses:  s.APIInfo(c).Addrs,
		CACertificate: s.APIInfo(c).CACert,
		CloudTag:      names.NewCloudTag(jimmtest.TestCloudName).String(),
		CloudRegion:   jimmtest.TestCloudRegionName,
		AgentVersion:  s.Model.Controller.AgentVersion,
		Status: jujuparams.EntityStatus{
			Status: "available",
		},
	})
}

func (s *jimmSuite) TestSetControllerDeprecated(c *gc.C) {
	conn := s.open(c, nil, "alice")
	defer conn.Close()
	client := api.NewClient(conn)

	ci, err := client.SetControllerDeprecated(&apiparams.SetControllerDeprecatedRequest{
		Name:       "controller-1",
		Deprecated: true,
	})
	c.Assert(err, gc.Equals, nil)
	c.Check(ci, jc.DeepEquals, apiparams.ControllerInfo{
		Name:          "controller-1",
		UUID:          s.Model.Controller.UUID,
		APIAddresses:  s.APIInfo(c).Addrs,
		CACertificate: s.APIInfo(c).CACert,
		CloudTag:      names.NewCloudTag(jimmtest.TestCloudName).String(),
		CloudRegion:   jimmtest.TestCloudRegionName,
		AgentVersion:  s.Model.Controller.AgentVersion,
		Status: jujuparams.EntityStatus{
			Status: "deprecated",
		},
	})

	ci, err = client.SetControllerDeprecated(&apiparams.SetControllerDeprecatedRequest{
		Name:       "controller-1",
		Deprecated: false,
	})
	c.Assert(err, gc.Equals, nil)
	c.Check(ci, jc.DeepEquals, apiparams.ControllerInfo{
		Name:          "controller-1",
		UUID:          s.Model.Controller.UUID,
		APIAddresses:  s.APIInfo(c).Addrs,
		CACertificate: s.APIInfo(c).CACert,
		CloudTag:      names.NewCloudTag(jimmtest.TestCloudName).String(),
		CloudRegion:   jimmtest.TestCloudRegionName,
		AgentVersion:  s.Model.Controller.AgentVersion,
		Status: jujuparams.EntityStatus{
			Status: "available",
		},
	})

	_, err = client.SetControllerDeprecated(&apiparams.SetControllerDeprecatedRequest{
		Name:       "controller-2",
		Deprecated: true,
	})
	c.Check(err, gc.ErrorMatches, `controller not found \(not found\)`)
	c.Check(jujuparams.IsCodeNotFound(err), gc.Equals, true)

	conn = s.open(c, nil, "bob")
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
	conn := s.open(c, nil, "bob")
	defer conn.Close()
	client := api.NewClient(conn)

	_, err := client.FindAuditEvents(&apiparams.FindAuditEventsRequest{})
	c.Check(err, gc.ErrorMatches, `unauthorized \(unauthorized access\)`)
	c.Check(jujuparams.ErrCode(err), gc.Equals, jujuparams.CodeUnauthorized)

	mmclient := modelmanager.NewClient(conn)
	zeroDuration := time.Duration(0)
	err = mmclient.DestroyModel(s.Model.ResourceTag(), nil, nil, nil, &zeroDuration)
	c.Assert(err, gc.Equals, nil)

	conn2 := s.open(c, nil, "alice")
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
	conn3 := s.open(c, nil, "bob")
	defer conn3.Close()
	client3 := api.NewClient(conn3)

	evs, err = client3.FindAuditEvents(&apiparams.FindAuditEventsRequest{})
	evs.Events = truncatedEvents
	c.Assert(err, gc.Equals, nil)
	c.Check(evs, jc.DeepEquals, expectedEvents)
}

func (s *jimmSuite) TestAuditLogFilterByMethod(c *gc.C) {
	conn := s.open(c, nil, "alice")
	defer conn.Close()
	client := api.NewClient(conn)
	evs, err := client.FindAuditEvents(&apiparams.FindAuditEventsRequest{Method: "Deploy"})
	c.Assert(err, gc.Equals, nil)
	c.Assert(len(evs.Events), gc.Equals, 0)
}

// TestAuditLogAPIParamsConversion tests the conversion of API params to a AuditLogFilter struct.
// Note that this test doesn't require a running Juju/JIMM controller so it doesn't use gc + the jimmSuite.
func TestAuditLogAPIParamsConversion(t *testing.T) {
	c := qt.New(t)
	testCases := []struct {
		about   string
		request apiparams.FindAuditEventsRequest
		result  db.AuditLogFilter
		err     error
	}{
		{
			about: "Test basic conversion",
			request: apiparams.FindAuditEventsRequest{
				After:    "2023-08-14T00:00:00Z",
				Before:   "2023-08-14T00:00:00Z",
				UserTag:  "user-alice",
				Model:    "123",
				Method:   "Deploy",
				Offset:   10,
				Limit:    10,
				SortTime: false,
			},
			result: db.AuditLogFilter{
				Start:       time.Date(2023, 8, 14, 0, 0, 0, 0, time.UTC),
				End:         time.Date(2023, 8, 14, 0, 0, 0, 0, time.UTC),
				IdentityTag: "user-alice",
				Model:       "123",
				Method:      "Deploy",
				Offset:      10,
				Limit:       10,
				SortTime:    false,
			},
		}, {
			about: "Test limit lower bound",
			request: apiparams.FindAuditEventsRequest{
				Limit: 0,
			},
			result: db.AuditLogFilter{
				Limit: jujuapi.AuditLogDefaultLimit,
			},
		}, {
			about: "Test limit upper bound",
			request: apiparams.FindAuditEventsRequest{
				Limit: jujuapi.AuditLogUpperLimit + 1,
			},
			result: db.AuditLogFilter{
				Limit: jujuapi.AuditLogUpperLimit,
			},
		},
	}
	for _, test := range testCases {
		c.Log(test.about)
		res, err := jujuapi.AuditParamsToFilter(test.request)
		if test.err == nil {
			c.Assert(err, qt.IsNil)
			c.Assert(res, qt.DeepEquals, test.result)
		} else {
			c.Assert(err, qt.ErrorMatches, test.err)
		}
	}
}

func (s *jimmSuite) TestFullModelStatus(c *gc.C) {
	s.AddController(c, "controller-2", s.APIInfo(c))
	mt := s.AddModel(c, names.NewUserTag("charlie@canonical.com"), "model-1", names.NewCloudTag(jimmtest.TestCloudName), jimmtest.TestCloudRegionName, s.Model2.CloudCredential.ResourceTag())

	conn := s.open(c, nil, "bob")
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

	conn = s.open(c, nil, "alice@canonical.com")
	defer conn.Close()
	client = api.NewClient(conn)

	status, err := client.FullModelStatus(&apiparams.FullModelStatusRequest{
		ModelTag: mt.String(),
	})
	c.Assert(err, gc.Equals, nil)
	c.Assert(status, jimmtest.CmpEquals(cmpopts.EquateEmpty(), cmpopts.IgnoreTypes(&time.Time{})), jujuparams.FullStatus{
		Model: jujuparams.ModelStatusInfo{
			Name:        "model-1",
			Type:        "iaas",
			CloudTag:    names.NewCloudTag(jimmtest.TestCloudName).String(),
			CloudRegion: jimmtest.TestCloudRegionName,
			Version:     jujuversion.Current.String(),
			ModelStatus: jujuparams.DetailedStatus{
				Status: "available",
			},
			SLA: "unsupported",
		},
	})
}

func (s *jimmSuite) TestUpdateMigratedModel(c *gc.C) {
	s.AddController(c, "controller-2", s.APIInfo(c))

	// Open the API connection as user "bob".
	conn := s.open(c, nil, "bob")
	defer conn.Close()

	req := apiparams.UpdateMigratedModelRequest{
		ModelTag:         names.NewModelTag(s.Model2.UUID.String).String(),
		TargetController: "controller-1",
	}
	err := conn.APICall("JIMM", 4, "", "UpdateMigratedModel", &req, nil)
	c.Assert(err, gc.ErrorMatches, `unauthorized \(unauthorized access\)`)

	// Open the API connection as user "alice".
	conn = s.open(c, nil, "alice")
	defer conn.Close()

	req = apiparams.UpdateMigratedModelRequest{
		ModelTag:         names.NewModelTag(s.Model2.UUID.String).String(),
		TargetController: "controller-1",
	}
	err = conn.APICall("JIMM", 4, "", "UpdateMigratedModel", &req, nil)
	c.Assert(err, gc.Equals, nil)

	req = apiparams.UpdateMigratedModelRequest{
		ModelTag:         "invalid-model-tag",
		TargetController: "controller-1",
	}
	err = conn.APICall("JIMM", 4, "", "UpdateMigratedModel", &req, nil)
	c.Assert(err, gc.ErrorMatches, `"invalid-model-tag" is not a valid tag \(bad request\)`)
}

func (s *jimmSuite) TestImportModel(c *gc.C) {
	// Open the API connection as user "bob".
	conn := s.open(c, nil, "bob")
	defer conn.Close()

	err := s.JIMM.OpenFGAClient.RemoveControllerModel(context.Background(), s.Model2.Controller.ResourceTag(), s.Model2.ResourceTag())
	c.Assert(err, gc.Equals, nil)
	err = s.JIMM.Database.DeleteModel(context.Background(), s.Model2)
	c.Assert(err, gc.Equals, nil)

	req := apiparams.ImportModelRequest{
		Controller: "controller-1",
		ModelTag:   s.Model2.Tag().String(),
		Owner:      "",
	}
	err = conn.APICall("JIMM", 4, "", "ImportModel", &req, nil)
	c.Assert(err, gc.ErrorMatches, `unauthorized \(unauthorized access\)`)

	// Open the API connection as user "alice".
	conn = s.open(c, nil, "alice")
	defer conn.Close()

	err = conn.APICall("JIMM", 4, "", "ImportModel", &req, nil)
	c.Assert(err, gc.Equals, nil)

	var model2 dbmodel.Model
	model2.SetTag(s.Model2.ResourceTag())
	err = s.JIMM.Database.GetModel(context.Background(), &model2)
	c.Assert(err, gc.Equals, nil)
	c.Check(model2.CreatedAt.After(s.Model2.CreatedAt), gc.Equals, true)

	req = apiparams.ImportModelRequest{
		Controller: "controller-1",
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

	conn := s.open(c, nil, "alice@canonical.com")
	defer conn.Close()

	req := apiparams.AddCloudToControllerRequest{
		ControllerName: "controller-1",
		AddCloudArgs: jujuparams.AddCloudArgs{
			Name: "test-cloud",
			Cloud: jujuapi.CloudToParams(cloud.Cloud{
				Name:             "test-cloud",
				Type:             "kubernetes",
				AuthTypes:        cloud.AuthTypes{cloud.CertificateAuthType},
				Endpoint:         "https://0.1.2.3:5678",
				IdentityEndpoint: "https://0.1.2.3:5679",
				StorageEndpoint:  "https://0.1.2.3:5680",
				HostCloudRegion:  jimmtest.TestCloudName + "/" + jimmtest.TestCloudRegionName,
			}),
		},
	}
	err = conn.APICall("JIMM", 4, "", "AddCloudToController", &req, nil)
	c.Assert(err, gc.Equals, nil)

	user := openfga.NewUser(u, s.OFGAClient)

	cloud, err := s.JIMM.GetCloud(context.Background(), user, names.NewCloudTag("test-cloud"))
	c.Assert(err, gc.IsNil)
	c.Assert(cloud.Name, gc.DeepEquals, "test-cloud")
	c.Assert(cloud.Type, gc.DeepEquals, "kubernetes")
}

func (s *jimmSuite) TestAddExistingCloudToController(c *gc.C) {
	ctx := context.Background()

	u, err := dbmodel.NewIdentity("alice@canonical.com")
	c.Assert(err, gc.IsNil)

	err = s.JIMM.Database.GetIdentity(ctx, u)
	c.Assert(err, gc.IsNil)

	conn := s.open(c, nil, "alice@canonical.com")
	defer conn.Close()

	force := true
	req := apiparams.AddCloudToControllerRequest{
		ControllerName: "controller-1",
		AddCloudArgs: jujuparams.AddCloudArgs{
			Name: "test-cloud",
			Cloud: jujuapi.CloudToParams(cloud.Cloud{
				Name:             "test-cloud",
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
	cloud, err := s.JIMM.GetCloud(context.Background(), user, names.NewCloudTag("test-cloud"))
	c.Assert(err, gc.IsNil)
	c.Assert(cloud.Name, gc.DeepEquals, "test-cloud")
	c.Assert(cloud.Type, gc.DeepEquals, "MAAS")
	// Simulate the cloud being present on the Juju controller but not in JIMM.
	err = s.JIMM.Database.DeleteCloud(ctx, &cloud)
	c.Assert(err, gc.IsNil)
	cloud, err = s.JIMM.GetCloud(context.Background(), user, names.NewCloudTag("test-cloud"))
	c.Assert(err, gc.NotNil)
	c.Assert(errors.ErrorCode(err), gc.Equals, errors.CodeNotFound)
	err = conn.APICall("JIMM", 4, "", "AddCloudToController", &req, nil)
	c.Assert(err, gc.Equals, nil)
	cloud, err = s.JIMM.GetCloud(context.Background(), user, names.NewCloudTag("test-cloud"))
	c.Assert(err, gc.IsNil)
	c.Assert(cloud.Name, gc.DeepEquals, "test-cloud")
	c.Assert(cloud.Type, gc.DeepEquals, "MAAS")
}

func (s *jimmSuite) TestRemoveCloudFromController(c *gc.C) {
	ctx := context.Background()

	u, err := dbmodel.NewIdentity("alice@canonical.com")
	c.Assert(err, gc.IsNil)

	err = s.JIMM.Database.GetIdentity(ctx, u)
	c.Assert(err, gc.IsNil)

	conn := s.open(c, nil, "alice@canonical.com")
	defer conn.Close()

	req := apiparams.AddCloudToControllerRequest{
		ControllerName: "controller-1",
		AddCloudArgs: jujuparams.AddCloudArgs{
			Name: "test-cloud",
			Cloud: jujuapi.CloudToParams(cloud.Cloud{
				Name:             "test-cloud",
				Type:             "kubernetes",
				AuthTypes:        cloud.AuthTypes{cloud.CertificateAuthType},
				Endpoint:         "https://0.1.2.3:5678",
				IdentityEndpoint: "https://0.1.2.3:5679",
				StorageEndpoint:  "https://0.1.2.3:5680",
				HostCloudRegion:  jimmtest.TestCloudName + "/" + jimmtest.TestCloudRegionName,
			}),
		},
	}
	err = conn.APICall("JIMM", 4, "", "AddCloudToController", &req, nil)
	c.Assert(err, gc.Equals, nil)

	user := openfga.NewUser(u, s.OFGAClient)

	_, err = s.JIMM.GetCloud(context.Background(), user, names.NewCloudTag("test-cloud"))
	c.Assert(err, gc.Equals, nil)

	req1 := apiparams.RemoveCloudFromControllerRequest{
		CloudTag:       names.NewCloudTag("test-cloud").String(),
		ControllerName: "controller-1",
	}
	err = conn.APICall("JIMM", 4, "", "RemoveCloudFromController", &req1, nil)
	c.Assert(err, gc.Equals, nil)

	_, err = s.JIMM.GetCloud(context.Background(), user, names.NewCloudTag("test-cloud"))
	c.Assert(err, gc.ErrorMatches, `cloud "test-cloud" not found`)
}

func (s *jimmSuite) TestCrossModelQuery(c *gc.C) {
	s.AddController(c, "controller-2", s.APIInfo(c))
	s.AddModel(
		c,
		names.NewUserTag("charlie@canonical.com"),
		"model-20",
		names.NewCloudTag(jimmtest.TestCloudName),
		jimmtest.TestCloudRegionName,
		s.Model2.CloudCredential.ResourceTag(),
	)
	s.AddModel(
		c,
		names.NewUserTag("charlie@canonical.com"),
		"model-21",
		names.NewCloudTag(jimmtest.TestCloudName),
		jimmtest.TestCloudRegionName,
		s.Model2.CloudCredential.ResourceTag(),
	)
	s.AddModel(
		c,
		names.NewUserTag("charlie@canonical.com"),
		"model-22",
		names.NewCloudTag(jimmtest.TestCloudName),
		jimmtest.TestCloudRegionName,
		s.Model2.CloudCredential.ResourceTag(),
	)

	conn := s.open(c, nil, "charlie")
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
	c.Assert(err, gc.ErrorMatches, `not implemented \(not implemented\)`)

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
		Query: "select((.model.name==\"model-21\") or .model.name==\"model-22\")",
	})
	c.Assert(err, gc.IsNil)
	c.Assert(res.Results, gc.HasLen, 2)
	c.Assert(res.Errors, gc.HasLen, 0)
}

// TestJimmModelMigration tests that a migration request makes it through to the Juju controller.
// Because our test suite only spins up 1 controller the further we can go is reaching Juju pre-checks which
// detect that a model with the same UUID already exists on the target controller.
func (s *jimmSuite) TestJimmModelMigrationSuperuser(c *gc.C) {
	mt := s.AddModel(
		c,
		names.NewUserTag("charlie@canonical.com"),
		"model-20",
		names.NewCloudTag(jimmtest.TestCloudName),
		jimmtest.TestCloudRegionName,
		s.Model2.CloudCredential.ResourceTag(),
	)

	conn := s.open(c, nil, "alice")
	defer conn.Close()
	client := api.NewClient(conn)

	res, err := client.MigrateModel(&apiparams.MigrateModelRequest{
		Specs: []apiparams.MigrateModelInfo{
			{ModelTag: mt.String(), TargetController: "controller-1"},
		},
	})
	c.Assert(err, gc.IsNil)
	c.Assert(res.Results, gc.HasLen, 1)
	item := res.Results[0]
	c.Assert(item.ModelTag, gc.Equals, mt.String())
	c.Assert(item.MigrationId, gc.Equals, "")
	c.Assert(item.Error.Message, gc.Matches, "target prechecks failed: model with same UUID already exists .*")
}

func (s *jimmSuite) TestJimmModelMigrationNonSuperuser(c *gc.C) {
	mt := s.AddModel(
		c,
		names.NewUserTag("charlie@canonical.com"),
		"model-20",
		names.NewCloudTag(jimmtest.TestCloudName),
		jimmtest.TestCloudRegionName,
		s.Model2.CloudCredential.ResourceTag(),
	)

	conn := s.open(c, nil, "bob")
	defer conn.Close()
	client := api.NewClient(conn)

	res, err := client.MigrateModel(&apiparams.MigrateModelRequest{
		Specs: []apiparams.MigrateModelInfo{
			{ModelTag: mt.String(), TargetController: "controller-1"},
		},
	})
	c.Assert(err, gc.IsNil)
	c.Assert(res.Results, gc.HasLen, 1)
	item := res.Results[0]
	c.Assert(item.Error.Message, gc.Matches, "unauthorized access")
}

func (s *jimmSuite) TestVersion(c *gc.C) {
	conn := s.open(c, nil, "bob")
	defer conn.Close()
	client := api.NewClient(conn)
	versionInfo, err := client.Version()
	c.Assert(err, gc.IsNil)
	c.Assert(versionInfo.Version, gc.Not(gc.Equals), "")
	c.Assert(versionInfo.Commit, gc.Not(gc.Equals), "")
}
