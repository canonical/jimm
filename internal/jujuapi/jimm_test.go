// Copyright 2016 Canonical Ltd.

package jujuapi_test

import (
	"context"
	"time"

	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/juju/juju/api/client/modelmanager"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/cloud"
	jujuparams "github.com/juju/juju/rpc/params"
	jujuversion "github.com/juju/juju/version"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/canonical/jimm/api"
	apiparams "github.com/canonical/jimm/api/params"
	"github.com/canonical/jimm/internal/dbmodel"
	"github.com/canonical/jimm/internal/errors"
	"github.com/canonical/jimm/internal/jimmtest"
	"github.com/canonical/jimm/internal/jujuapi"
)

type jimmSuite struct {
	websocketSuite
}

var _ = gc.Suite(&jimmSuite{})

func (s *jimmSuite) TestListControllers(c *gc.C) {
	s.AddController(c, "controller-0", s.APIInfo(c))
	s.AddController(c, "controller-2", s.APIInfo(c))

	// Open the API connection as user "alice".
	conn := s.open(c, nil, "alice")
	defer conn.Close()
	var resp jujuapi.LegacyListControllerResponse
	err := conn.APICall("JIMM", 2, "", "ListControllers", nil, &resp)
	c.Assert(err, gc.Equals, nil)

	c.Assert(resp, jc.DeepEquals, jujuapi.LegacyListControllerResponse{
		Controllers: []jujuapi.LegacyControllerResponse{{
			Path:     "admin/controller-0",
			Location: map[string]string{"cloud": jimmtest.TestCloudName, "region": jimmtest.TestCloudRegionName},
			Public:   true,
			UUID:     s.Model.Controller.UUID,
			Version:  s.Model.Controller.AgentVersion,
		}, {
			Path:     "admin/controller-1",
			Location: map[string]string{"cloud": jimmtest.TestCloudName, "region": jimmtest.TestCloudRegionName},
			Public:   true,
			UUID:     s.Model.Controller.UUID,
			Version:  s.Model.Controller.AgentVersion,
		}, {
			Path:     "admin/controller-2",
			Location: map[string]string{"cloud": jimmtest.TestCloudName, "region": jimmtest.TestCloudRegionName},
			Public:   true,
			UUID:     s.Model.Controller.UUID,
			Version:  s.Model.Controller.AgentVersion,
		}},
	})
}

func (s *jimmSuite) TestListControllersUnauthorizedUser(c *gc.C) {
	s.AddController(c, "controller-0", s.APIInfo(c))
	s.AddController(c, "controller-2", s.APIInfo(c))

	// Open the API connection as user "bob".
	conn := s.open(c, nil, "bob")
	defer conn.Close()
	var resp jujuapi.LegacyListControllerResponse
	err := conn.APICall("JIMM", 2, "", "ListControllers", nil, &resp)
	c.Assert(err, gc.Equals, nil)

	c.Assert(resp, jc.DeepEquals, jujuapi.LegacyListControllerResponse{
		Controllers: []jujuapi.LegacyControllerResponse{{
			Path:    "admin/jaas",
			Public:  true,
			UUID:    "914487b5-60e7-42bb-bd63-1adc3fd3a388",
			Version: jujuversion.Current.String(),
		}},
	})
}

func (s *jimmSuite) TestListControllersV3(c *gc.C) {
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
		Username:      "admin",
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
		Username:      "admin",
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
		Username:      "admin",
		AgentVersion:  s.Model.Controller.AgentVersion,
		Status: jujuparams.EntityStatus{
			Status: "available",
		},
	}})
}

func (s *jimmSuite) TestListControllersV3Unauthorized(c *gc.C) {
	s.AddController(c, "controller-0", s.APIInfo(c))
	s.AddController(c, "controller-2", s.APIInfo(c))

	conn := s.open(c, nil, "bob")
	defer conn.Close()

	client := api.NewClient(conn)
	cis, err := client.ListControllers()
	c.Assert(err, gc.Equals, nil)
	c.Check(cis, jc.DeepEquals, []apiparams.ControllerInfo{{
		Name:         "jaas",
		UUID:         "914487b5-60e7-42bb-bd63-1adc3fd3a388",
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
			Username:      s.APIInfo(c).Tag.Id(),
			Password:      s.APIInfo(c).Password,
		},
		expectedError: `address controller.test.com: missing port in address \(bad request\)`,
	}, {
		req: apiparams.AddControllerRequest{
			Name:          "controller-2",
			PublicAddress: ":8080",
			CACertificate: s.APIInfo(c).CACert,
			Username:      s.APIInfo(c).Tag.Id(),
			Password:      s.APIInfo(c).Password,
		},
		expectedError: `address :8080: host not specified in public address \(bad request\)`,
	}, {
		req: apiparams.AddControllerRequest{
			Name:          "controller-2",
			PublicAddress: "localhost:",
			CACertificate: s.APIInfo(c).CACert,
			Username:      s.APIInfo(c).Tag.Id(),
			Password:      s.APIInfo(c).Password,
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

	acr := apiparams.AddControllerRequest{
		Name:          "controller-2",
		APIAddresses:  s.APIInfo(c).Addrs,
		CACertificate: s.APIInfo(c).CACert,
		Username:      s.APIInfo(c).Tag.Id(),
		Password:      s.APIInfo(c).Password,
	}

	ci, err := client.AddController(&acr)
	c.Assert(err, gc.Equals, nil)
	c.Check(ci, jc.DeepEquals, apiparams.ControllerInfo{
		Name:          "controller-2",
		UUID:          s.Model.Controller.UUID,
		APIAddresses:  s.APIInfo(c).Addrs,
		CACertificate: s.APIInfo(c).CACert,
		CloudTag:      names.NewCloudTag(jimmtest.TestCloudName).String(),
		CloudRegion:   jimmtest.TestCloudRegionName,
		Username:      s.APIInfo(c).Tag.Id(),
		AgentVersion:  s.Model.Controller.AgentVersion,
		Status: jujuparams.EntityStatus{
			Status: "available",
		},
	})

	_, err = client.AddController(&acr)
	c.Check(err, gc.ErrorMatches, `controller "controller-2" already exists \(already exists\)`)
	c.Check(jujuparams.IsCodeAlreadyExists(err), gc.Equals, true)

	conn = s.open(c, nil, "bob")
	defer conn.Close()
	client = api.NewClient(conn)
	acr.Name = "controller-3"
	_, err = client.AddController(&acr)
	c.Check(err, gc.ErrorMatches, `unauthorized \(unauthorized access\)`)
	c.Check(jujuparams.IsCodeUnauthorized(err), gc.Equals, true)
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
		Username:      s.APIInfo(c).Tag.Id(),
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
		Username:      s.APIInfo(c).Tag.Id(),
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
		Username:      s.APIInfo(c).Tag.Id(),
		AgentVersion:  s.Model.Controller.AgentVersion,
		Status: jujuparams.EntityStatus{
			Status: "available",
		},
	})

	ci, err = client.SetControllerDeprecated(&apiparams.SetControllerDeprecatedRequest{
		Name:       "controller-2",
		Deprecated: true,
	})
	c.Check(err, gc.ErrorMatches, `controller not found \(not found\)`)
	c.Check(jujuparams.IsCodeNotFound(err), gc.Equals, true)

	conn = s.open(c, nil, "bob")
	defer conn.Close()
	client = api.NewClient(conn)
	ci, err = client.SetControllerDeprecated(&apiparams.SetControllerDeprecatedRequest{
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
	err = mmclient.DestroyModel(s.Model.Tag().(names.ModelTag), nil, nil, nil, time.Duration(0))
	c.Assert(err, gc.Equals, nil)

	conn2 := s.open(c, nil, "alice")
	defer conn2.Close()
	client2 := api.NewClient(conn2)

	evs, err := client2.FindAuditEvents(&apiparams.FindAuditEventsRequest{})
	c.Assert(err, gc.Equals, nil)

	expectedEvents := apiparams.AuditEvents{
		Events: []apiparams.AuditEvent{{
			Time:    evs.Events[0].Time,
			Tag:     s.Model.Controller.Tag().String(),
			UserTag: names.NewUserTag("alice@external").String(),
			Action:  "add",
			Success: true,
			Params: map[string]string{
				"name": "controller-1",
			},
		}, {
			Time:    evs.Events[1].Time,
			Tag:     s.Model.CloudCredential.Tag().String(),
			UserTag: s.Model.Owner.Tag().String(),
			Action:  "update",
			Success: true,
			Params: map[string]string{
				"skip-check":  "true",
				"skip-update": "false",
			},
		}, {
			Time:    evs.Events[2].Time,
			Tag:     s.Model.Tag().String(),
			UserTag: s.Model.Owner.Tag().String(),
			Action:  "create",
			Success: true,
			Params: map[string]string{
				"cloud":            names.NewCloudTag(jimmtest.TestCloudName).String(),
				"cloud-credential": s.Model.CloudCredential.Tag().String(),
				"name":             "model-1",
				"owner":            s.Model.Owner.Tag().String(),
				"region":           jimmtest.TestCloudRegionName,
			},
		}, {
			Time:    evs.Events[3].Time,
			Tag:     s.Model2.CloudCredential.Tag().String(),
			UserTag: s.Model2.Owner.Tag().String(),
			Action:  "update",
			Success: true,
			Params: map[string]string{
				"skip-check":  "true",
				"skip-update": "false",
			},
		}, {
			Time:    evs.Events[4].Time,
			Tag:     s.Model2.Tag().String(),
			UserTag: s.Model2.Owner.Tag().String(),
			Action:  "create",
			Success: true,
			Params: map[string]string{
				"cloud":            names.NewCloudTag(jimmtest.TestCloudName).String(),
				"cloud-credential": s.Model2.CloudCredential.Tag().String(),
				"name":             "model-2",
				"owner":            s.Model2.Owner.Tag().String(),
				"region":           jimmtest.TestCloudRegionName,
			},
		}, {
			Time:    evs.Events[5].Time,
			Tag:     s.Model3.Tag().String(),
			UserTag: s.Model3.Owner.Tag().String(),
			Action:  "create",
			Success: true,
			Params: map[string]string{
				"cloud":            names.NewCloudTag(jimmtest.TestCloudName).String(),
				"cloud-credential": s.Model3.CloudCredential.Tag().String(),
				"name":             "model-3",
				"owner":            s.Model3.Owner.Tag().String(),
				"region":           jimmtest.TestCloudRegionName,
			},
		}, {
			Time:    evs.Events[6].Time,
			Tag:     s.Model3.Tag().String(),
			UserTag: s.Model3.Owner.Tag().String(),
			Action:  "grant",
			Success: true,
			Params: map[string]string{
				"access": "read",
				"user":   names.NewUserTag("bob@external").String(),
			},
		}, {
			Time:    evs.Events[7].Time,
			Tag:     s.Model.Tag().String(),
			UserTag: s.Model.Owner.Tag().String(),
			Action:  "destroy",
			Success: true,
			Params:  map[string]string{},
		}},
	}
	c.Check(evs, jc.DeepEquals, expectedEvents)

	// alice can grant bob access to audit log entries
	err = client2.GrantAuditLogAccess(&apiparams.AuditLogAccessRequest{
		UserTag: names.NewUserTag("bob@external").String(),
	})
	c.Assert(err, gc.Equals, nil)

	// now bob can access audit events as well
	conn3 := s.open(c, nil, "bob")
	defer conn3.Close()
	client3 := api.NewClient(conn3)

	evs, err = client3.FindAuditEvents(&apiparams.FindAuditEventsRequest{})
	c.Assert(err, gc.Equals, nil)
	c.Check(evs, jc.DeepEquals, expectedEvents)
}

func (s *jimmSuite) TestFullModelStatus(c *gc.C) {
	s.AddController(c, "controller-2", s.APIInfo(c))
	mt := s.AddModel(c, names.NewUserTag("charlie@external"), "model-1", names.NewCloudTag(jimmtest.TestCloudName), jimmtest.TestCloudRegionName, s.Model2.CloudCredential.Tag().(names.CloudCredentialTag))

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

	conn = s.open(c, nil, "alice@external")
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
	err := conn.APICall("JIMM", 3, "", "UpdateMigratedModel", &req, nil)
	c.Assert(err, gc.ErrorMatches, `unauthorized \(unauthorized access\)`)

	// Open the API connection as user "alice".
	conn = s.open(c, nil, "alice")
	defer conn.Close()

	req = apiparams.UpdateMigratedModelRequest{
		ModelTag:         names.NewModelTag(s.Model2.UUID.String).String(),
		TargetController: "controller-1",
	}
	err = conn.APICall("JIMM", 3, "", "UpdateMigratedModel", &req, nil)
	c.Assert(err, gc.Equals, nil)

	req = apiparams.UpdateMigratedModelRequest{
		ModelTag:         "invalid-model-tag",
		TargetController: "controller-1",
	}
	err = conn.APICall("JIMM", 3, "", "UpdateMigratedModel", &req, nil)
	c.Assert(err, gc.ErrorMatches, `"invalid-model-tag" is not a valid tag \(bad request\)`)
}

func (s *jimmSuite) TestImportModel(c *gc.C) {
	// Open the API connection as user "bob".
	conn := s.open(c, nil, "bob")
	defer conn.Close()

	err := s.JIMM.Database.DeleteModel(context.Background(), s.Model2)
	c.Assert(err, gc.Equals, nil)

	req := apiparams.ImportModelRequest{
		Controller: "controller-1",
		ModelTag:   s.Model2.Tag().String(),
		Owner:      "",
	}
	err = conn.APICall("JIMM", 3, "", "ImportModel", &req, nil)
	c.Assert(err, gc.ErrorMatches, `unauthorized \(unauthorized access\)`)

	// Open the API connection as user "alice".
	conn = s.open(c, nil, "alice")
	defer conn.Close()

	err = conn.APICall("JIMM", 3, "", "ImportModel", &req, nil)
	c.Assert(err, gc.Equals, nil)

	var model2 dbmodel.Model
	model2.SetTag(s.Model2.Tag().(names.ModelTag))
	err = s.JIMM.Database.GetModel(context.Background(), &model2)
	c.Assert(err, gc.Equals, nil)
	c.Check(model2.CreatedAt.After(s.Model2.CreatedAt), gc.Equals, true)

	req = apiparams.ImportModelRequest{
		Controller: "controller-1",
		ModelTag:   "invalid-model-tag",
	}
	err = conn.APICall("JIMM", 3, "", "ImportModel", &req, nil)
	c.Assert(err, gc.ErrorMatches, `"invalid-model-tag" is not a valid tag \(bad request\)`)
}

func (s *jimmSuite) TestAddCloudToController(c *gc.C) {
	ctx := context.Background()
	u := dbmodel.User{
		Username: "bob@external",
	}
	err := s.JIMM.Database.GetUser(ctx, &u)
	c.Assert(err, gc.IsNil)

	conn := s.open(c, nil, "bob@external")
	defer conn.Close()

	req := apiparams.AddCloudToControllerRequest{
		ControllerName: "controller-1",
		AddCloudArgs: jujuparams.AddCloudArgs{
			Name: "test-cloud",
			Cloud: common.CloudToParams(cloud.Cloud{
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
	err = conn.APICall("JIMM", 3, "", "AddCloudToController", &req, nil)
	c.Assert(err, gc.Equals, nil)

	cloud, err := s.JIMM.GetCloud(context.Background(), &u, names.NewCloudTag("test-cloud"))
	c.Assert(err, gc.IsNil)
	c.Assert(cloud.Name, gc.DeepEquals, "test-cloud")
	c.Assert(cloud.Type, gc.DeepEquals, "kubernetes")
}

func (s *jimmSuite) TestAddExistingCloudToController(c *gc.C) {
	ctx := context.Background()
	u := dbmodel.User{
		Username: "alice@external",
	}
	err := s.JIMM.Database.GetUser(ctx, &u)
	c.Assert(err, gc.IsNil)

	conn := s.open(c, nil, "alice@external")
	defer conn.Close()

	force := true
	req := apiparams.AddCloudToControllerRequest{
		ControllerName: "controller-1",
		AddCloudArgs: jujuparams.AddCloudArgs{
			Name: "test-cloud",
			Cloud: common.CloudToParams(cloud.Cloud{
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
	err = conn.APICall("JIMM", 3, "", "AddCloudToController", &req, nil)
	c.Assert(err, gc.Equals, nil)
	cloud, err := s.JIMM.GetCloud(context.Background(), &u, names.NewCloudTag("test-cloud"))
	c.Assert(err, gc.IsNil)
	c.Assert(cloud.Name, gc.DeepEquals, "test-cloud")
	c.Assert(cloud.Type, gc.DeepEquals, "MAAS")
	// Simulate the cloud being present on the Juju controller but not in JIMM.
	err = s.JIMM.Database.DeleteCloud(ctx, &cloud)
	c.Assert(err, gc.IsNil)
	cloud, err = s.JIMM.GetCloud(context.Background(), &u, names.NewCloudTag("test-cloud"))
	c.Assert(err, gc.NotNil)
	c.Assert(errors.ErrorCode(err), gc.Equals, errors.CodeNotFound)
	err = conn.APICall("JIMM", 3, "", "AddCloudToController", &req, nil)
	c.Assert(err, gc.Equals, nil)
	cloud, err = s.JIMM.GetCloud(context.Background(), &u, names.NewCloudTag("test-cloud"))
	c.Assert(err, gc.IsNil)
	c.Assert(cloud.Name, gc.DeepEquals, "test-cloud")
	c.Assert(cloud.Type, gc.DeepEquals, "MAAS")
}

func (s *jimmSuite) TestRemoveCloudFromController(c *gc.C) {
	ctx := context.Background()
	u := dbmodel.User{
		Username: "bob@external",
	}
	err := s.JIMM.Database.GetUser(ctx, &u)
	c.Assert(err, gc.IsNil)

	conn := s.open(c, nil, "bob@external")
	defer conn.Close()

	req := apiparams.AddCloudToControllerRequest{
		ControllerName: "controller-1",
		AddCloudArgs: jujuparams.AddCloudArgs{
			Name: "test-cloud",
			Cloud: common.CloudToParams(cloud.Cloud{
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
	err = conn.APICall("JIMM", 3, "", "AddCloudToController", &req, nil)
	c.Assert(err, gc.Equals, nil)

	_, err = s.JIMM.GetCloud(context.Background(), &u, names.NewCloudTag("test-cloud"))
	c.Assert(err, gc.Equals, nil)

	req1 := apiparams.RemoveCloudFromControllerRequest{
		CloudTag:       names.NewCloudTag("test-cloud").String(),
		ControllerName: "controller-1",
	}
	err = conn.APICall("JIMM", 3, "", "RemoveCloudFromController", &req1, nil)
	c.Assert(err, gc.Equals, nil)

	_, err = s.JIMM.GetCloud(context.Background(), &u, names.NewCloudTag("test-cloud"))
	c.Assert(err, gc.ErrorMatches, `cloud "test-cloud" not found`)
}
