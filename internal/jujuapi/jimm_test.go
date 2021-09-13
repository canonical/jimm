// Copyright 2016 Canonical Ltd.

package jujuapi_test

import (
	"context"
	"time"

	"github.com/juju/juju/api/modelmanager"
	jujuparams "github.com/juju/juju/apiserver/params"
	jujuversion "github.com/juju/juju/version"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/CanonicalLtd/jimm/api"
	apiparams "github.com/CanonicalLtd/jimm/api/params"
	"github.com/CanonicalLtd/jimm/internal/conv"
	"github.com/CanonicalLtd/jimm/internal/jemtest"
	"github.com/CanonicalLtd/jimm/internal/mongodoc"
	"github.com/CanonicalLtd/jimm/params"
)

type jimmSuite struct {
	websocketSuite
}

var _ = gc.Suite(&jimmSuite{})

func (s *jimmSuite) TestJIMMFacadeVersion(c *gc.C) {
	conn := s.open(c, nil, "test")
	defer conn.Close()
	c.Assert(conn.AllFacadeVersions()["JIMM"], jc.DeepEquals, []int{1, 2, 3})
}

func (s *jimmSuite) TestUserModelStats(c *gc.C) {
	ctx := context.Background()

	// Update some stats for the models we've just created'
	t0 := time.Unix(0, 0)

	err := s.JEM.UpdateModelCounts(ctx, s.Controller.Path, s.Model.UUID, map[params.EntityCount]int{
		params.UnitCount: 99,
	}, t0)
	c.Assert(err, gc.Equals, nil)

	err = s.JEM.UpdateModelCounts(ctx, s.Controller.Path, s.Model2.UUID, map[params.EntityCount]int{
		params.MachineCount: 10,
	}, t0)
	c.Assert(err, gc.Equals, nil)

	err = s.JEM.UpdateModelCounts(ctx, s.Controller.Path, s.Model3.UUID, map[params.EntityCount]int{
		params.ApplicationCount: 1,
	}, t0)
	c.Assert(err, gc.Equals, nil)

	// Open the API connection as user "bob". We should only see the one model.
	conn := s.open(c, nil, "bob")
	defer conn.Close()
	var resp params.UserModelStatsResponse
	err = conn.APICall("JIMM", 1, "", "UserModelStats", nil, &resp)
	c.Assert(err, gc.Equals, nil)

	c.Assert(resp, jc.DeepEquals, params.UserModelStatsResponse{
		Models: map[string]params.ModelStats{
			s.Model.UUID: {
				Model: jujuparams.Model{
					Name:     "model-1",
					UUID:     s.Model.UUID,
					Type:     "iaas",
					OwnerTag: conv.ToUserTag(s.Model.Owner()).String(),
				},
				Counts: map[params.EntityCount]params.Count{
					params.UnitCount: {
						Time:    t0,
						Current: 99,
						Max:     99,
						Total:   99,
					},
				},
			},
		},
	})

	// As charlie, we should see the other two models.
	conn = s.open(c, nil, "charlie")
	defer conn.Close()
	resp = params.UserModelStatsResponse{}
	err = conn.APICall("JIMM", 1, "", "UserModelStats", nil, &resp)
	c.Assert(err, gc.Equals, nil)

	c.Assert(resp, jc.DeepEquals, params.UserModelStatsResponse{
		Models: map[string]params.ModelStats{
			s.Model2.UUID: {
				Model: jujuparams.Model{
					Name:     "model-2",
					UUID:     s.Model2.UUID,
					Type:     "iaas",
					OwnerTag: conv.ToUserTag(s.Model2.Owner()).String(),
				},
				Counts: map[params.EntityCount]params.Count{
					params.MachineCount: {
						Time:    t0,
						Current: 10,
						Max:     10,
						Total:   10,
					},
				},
			},
			s.Model3.UUID: {
				Model: jujuparams.Model{
					Name:     "model-3",
					UUID:     s.Model3.UUID,
					Type:     "iaas",
					OwnerTag: conv.ToUserTag(s.Model3.Owner()).String(),
				},
				Counts: map[params.EntityCount]params.Count{
					params.ApplicationCount: {
						Time:    t0,
						Current: 1,
						Max:     1,
						Total:   1,
					},
				},
			},
		},
	})
}

func (s *jimmSuite) TestListControllers(c *gc.C) {
	c0 := mongodoc.Controller{Path: params.EntityPath{User: jemtest.ControllerAdmin, Name: "controller-0"}}
	s.AddController(c, &c0)
	c2 := mongodoc.Controller{Path: params.EntityPath{User: jemtest.ControllerAdmin, Name: "controller-2"}}
	s.AddController(c, &c2)

	// Open the API connection as user "alice".
	conn := s.open(c, nil, "alice")
	defer conn.Close()
	var resp params.ListControllerResponse
	err := conn.APICall("JIMM", 2, "", "ListControllers", nil, &resp)
	c.Assert(err, gc.Equals, nil)

	c.Assert(resp, jc.DeepEquals, params.ListControllerResponse{
		Controllers: []params.ControllerResponse{{
			Path:     c0.Path,
			Location: map[string]string{"cloud": jemtest.TestCloudName, "region": jemtest.TestCloudRegionName},
			Public:   true,
			UUID:     c0.UUID,
			Version:  c0.Version.String(),
		}, {
			Path:     s.Controller.Path,
			Location: map[string]string{"cloud": jemtest.TestCloudName, "region": jemtest.TestCloudRegionName},
			Public:   true,
			UUID:     s.Controller.UUID,
			Version:  s.Controller.Version.String(),
		}, {
			Path:     c2.Path,
			Location: map[string]string{"cloud": jemtest.TestCloudName, "region": jemtest.TestCloudRegionName},
			Public:   true,
			UUID:     c2.UUID,
			Version:  c2.Version.String(),
		}},
	})
}

func (s *jimmSuite) TestListControllersUnauthorizedUser(c *gc.C) {
	c0 := mongodoc.Controller{Path: params.EntityPath{User: "alice", Name: "controller-0"}}
	s.AddController(c, &c0)
	c2 := mongodoc.Controller{Path: params.EntityPath{User: "alice", Name: "controller-2"}}
	s.AddController(c, &c2)

	// Open the API connection as user "bob".
	conn := s.open(c, nil, "bob")
	defer conn.Close()
	var resp params.ListControllerResponse
	err := conn.APICall("JIMM", 2, "", "ListControllers", nil, &resp)
	c.Assert(err, gc.Equals, nil)

	c.Assert(resp, jc.DeepEquals, params.ListControllerResponse{
		Controllers: []params.ControllerResponse{{
			Path:    params.EntityPath{User: "admin", Name: "jaas"},
			Public:  true,
			UUID:    "914487b5-60e7-42bb-bd63-1adc3fd3a388",
			Version: jujuversion.Current.String(),
		}},
	})
}

func (s *jimmSuite) TestListControllersV3(c *gc.C) {
	c0 := mongodoc.Controller{Path: params.EntityPath{User: jemtest.ControllerAdmin, Name: "controller-0"}}
	s.AddController(c, &c0)
	c2 := mongodoc.Controller{Path: params.EntityPath{User: jemtest.ControllerAdmin, Name: "controller-2"}}
	s.AddController(c, &c2)

	conn := s.open(c, nil, "alice")
	defer conn.Close()

	client := api.NewClient(conn)
	cis, err := client.ListControllers()
	c.Assert(err, gc.Equals, nil)
	c.Check(cis, jc.DeepEquals, []apiparams.ControllerInfo{{
		Name:          "controller-0",
		UUID:          s.Controller.UUID,
		APIAddresses:  []string{s.Controller.HostPorts[0][0].Address()},
		CACertificate: s.Controller.CACert,
		CloudTag:      names.NewCloudTag(jemtest.TestCloudName).String(),
		CloudRegion:   jemtest.TestCloudRegionName,
		Username:      "admin",
		AgentVersion:  s.Controller.Version.String(),
		Status: jujuparams.EntityStatus{
			Status: "available",
		},
	}, {
		Name:          "controller-1",
		UUID:          s.Controller.UUID,
		APIAddresses:  []string{s.Controller.HostPorts[0][0].Address()},
		CACertificate: s.Controller.CACert,
		CloudTag:      names.NewCloudTag(jemtest.TestCloudName).String(),
		CloudRegion:   jemtest.TestCloudRegionName,
		Username:      "admin",
		AgentVersion:  s.Controller.Version.String(),
		Status: jujuparams.EntityStatus{
			Status: "available",
		},
	}, {
		Name:          "controller-2",
		UUID:          s.Controller.UUID,
		APIAddresses:  []string{s.Controller.HostPorts[0][0].Address()},
		CACertificate: s.Controller.CACert,
		CloudTag:      names.NewCloudTag(jemtest.TestCloudName).String(),
		CloudRegion:   jemtest.TestCloudRegionName,
		Username:      "admin",
		AgentVersion:  s.Controller.Version.String(),
		Status: jujuparams.EntityStatus{
			Status: "available",
		},
	}})
}

func (s *jimmSuite) TestListControllersV3Unauthorized(c *gc.C) {
	c0 := mongodoc.Controller{Path: params.EntityPath{User: jemtest.ControllerAdmin, Name: "controller-0"}}
	s.AddController(c, &c0)
	c2 := mongodoc.Controller{Path: params.EntityPath{User: jemtest.ControllerAdmin, Name: "controller-2"}}
	s.AddController(c, &c2)

	conn := s.open(c, nil, "bob")
	defer conn.Close()

	client := api.NewClient(conn)
	cis, err := client.ListControllers()
	c.Assert(err, gc.Equals, nil)
	c.Check(cis, jc.DeepEquals, []apiparams.ControllerInfo{{
		Name:         "jaas",
		UUID:         "914487b5-60e7-42bb-bd63-1adc3fd3a388",
		AgentVersion: s.Controller.Version.String(),
		Status: jujuparams.EntityStatus{
			Status: "available",
		},
	}})
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
		UUID:          s.Controller.UUID,
		APIAddresses:  s.APIInfo(c).Addrs,
		CACertificate: s.APIInfo(c).CACert,
		CloudTag:      names.NewCloudTag(jemtest.TestCloudName).String(),
		CloudRegion:   jemtest.TestCloudRegionName,
		Username:      s.APIInfo(c).Tag.Id(),
		AgentVersion:  s.Controller.Version.String(),
		Status: jujuparams.EntityStatus{
			Status: "available",
		},
	})

	_, err = client.AddController(&acr)
	c.Check(err, gc.ErrorMatches, `already exists \(already exists\)`)
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
	c.Check(err, gc.ErrorMatches, `cannot delete controller while it is still alive \(still alive\)`)
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
		UUID:          s.Controller.UUID,
		APIAddresses:  s.APIInfo(c).Addrs,
		CACertificate: s.APIInfo(c).CACert,
		CloudTag:      names.NewCloudTag(jemtest.TestCloudName).String(),
		CloudRegion:   jemtest.TestCloudRegionName,
		Username:      s.APIInfo(c).Tag.Id(),
		AgentVersion:  s.Controller.Version.String(),
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
		UUID:          s.Controller.UUID,
		APIAddresses:  s.APIInfo(c).Addrs,
		CACertificate: s.APIInfo(c).CACert,
		CloudTag:      names.NewCloudTag(jemtest.TestCloudName).String(),
		CloudRegion:   jemtest.TestCloudRegionName,
		Username:      s.APIInfo(c).Tag.Id(),
		AgentVersion:  s.Controller.Version.String(),
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
		UUID:          s.Controller.UUID,
		APIAddresses:  s.APIInfo(c).Addrs,
		CACertificate: s.APIInfo(c).CACert,
		CloudTag:      names.NewCloudTag(jemtest.TestCloudName).String(),
		CloudRegion:   jemtest.TestCloudRegionName,
		Username:      s.APIInfo(c).Tag.Id(),
		AgentVersion:  s.Controller.Version.String(),
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
	err = mmclient.DestroyModel(names.NewModelTag(s.Model.UUID), nil, nil, nil, time.Minute)
	c.Assert(err, gc.Equals, nil)

	conn2 := s.open(c, nil, "alice")
	defer conn2.Close()
	client2 := api.NewClient(conn2)

	evs, err := client2.FindAuditEvents(&apiparams.FindAuditEventsRequest{})
	c.Assert(err, gc.Equals, nil)

	c.Check(evs, jc.DeepEquals, apiparams.AuditEvents{
		Events: []apiparams.AuditEvent{{
			Time:    evs.Events[0].Time,
			Tag:     names.NewModelTag(s.Model.UUID).String(),
			UserTag: conv.ToUserTag(s.Model.Path.User).String(),
			Action:  "created",
			Params: map[string]string{
				"owner":           "bob",
				"name":            "bob/model-1",
				"controller-name": "controller-admin/controller-1",
				"cloud":           jemtest.TestCloudName,
				"region":          jemtest.TestCloudRegionName,
			},
		}, {
			Time:    evs.Events[1].Time,
			Tag:     names.NewModelTag(s.Model2.UUID).String(),
			UserTag: conv.ToUserTag(s.Model2.Path.User).String(),
			Action:  "created",
			Params: map[string]string{
				"owner":           "charlie",
				"name":            "charlie/model-2",
				"controller-name": "controller-admin/controller-1",
				"cloud":           jemtest.TestCloudName,
				"region":          jemtest.TestCloudRegionName,
			},
		}, {
			Time:    evs.Events[2].Time,
			Tag:     names.NewModelTag(s.Model3.UUID).String(),
			UserTag: conv.ToUserTag(s.Model3.Path.User).String(),
			Action:  "created",
			Params: map[string]string{
				"owner":           "charlie",
				"name":            "charlie/model-3",
				"controller-name": "controller-admin/controller-1",
				"cloud":           jemtest.TestCloudName,
				"region":          jemtest.TestCloudRegionName,
			},
		}, {
			Time:    evs.Events[3].Time,
			Tag:     names.NewModelTag(s.Model.UUID).String(),
			UserTag: conv.ToUserTag(s.Model.Path.User).String(),
			Action:  "deleted",
		}},
	})
}

func (s *jimmSuite) TestUpdateMigratedModel(c *gc.C) {
	c2 := mongodoc.Controller{Path: params.EntityPath{User: jemtest.ControllerAdmin, Name: "controller-2"}}
	s.AddController(c, &c2)

	// Open the API connection as user "bob".
	conn := s.open(c, nil, "bob")
	defer conn.Close()

	req := apiparams.UpdateMigratedModelRequest{
		ModelTag:         names.NewModelTag(s.Model2.UUID).String(),
		TargetController: "controller-1",
	}
	err := conn.APICall("JIMM", 3, "", "UpdateMigratedModel", &req, nil)
	c.Assert(err, gc.ErrorMatches, `unauthorized \(unauthorized access\)`)

	// Open the API connection as user "alice".
	conn = s.open(c, nil, "alice")
	defer conn.Close()

	req = apiparams.UpdateMigratedModelRequest{
		ModelTag:         names.NewModelTag(s.Model2.UUID).String(),
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
