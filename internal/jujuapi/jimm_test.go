// Copyright 2016 Canonical Ltd.

package jujuapi_test

import (
	"context"
	"time"

	jujuparams "github.com/juju/juju/apiserver/params"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"

	"github.com/CanonicalLtd/jimm/params"
)

type jimmSuite struct {
	websocketSuite
}

var _ = gc.Suite(&jimmSuite{})

func (s *jimmSuite) SetUpTest(c *gc.C) {
	s.ServerParams.CharmstoreLocation = "https://api.jujucharms.com/charmstore"
	s.ServerParams.MeteringLocation = "https://api.jujucharms.com/omnibus"
	s.websocketSuite.SetUpTest(c)
	s.PatchValue(&utils.OutgoingAccessAllowed, true)
}

func (s *jimmSuite) TestJIMMFacadeVersion(c *gc.C) {
	conn := s.open(c, nil, "test")
	defer conn.Close()
	c.Assert(conn.AllFacadeVersions()["JIMM"], jc.DeepEquals, []int{1, 2})
}

func (s *jimmSuite) TestUserModelStats(c *gc.C) {
	ctx := context.Background()

	ctlPath := s.AssertAddController(ctx, c, params.EntityPath{User: "test", Name: "controller-1"}, true)
	cred := s.AssertUpdateCredential(ctx, c, "test", "dummy", "cred1", "empty")
	cred2 := s.AssertUpdateCredential(ctx, c, "test2", "dummy", "cred1", "empty")
	err := s.JEM.DB.SetACL(ctx, s.JEM.DB.Controllers(), ctlPath, params.ACL{
		Read: []string{"test2"},
	})

	c.Assert(err, gc.Equals, nil)

	model1 := s.assertCreateModel(c, createModelParams{name: "model-1", username: "test", cred: cred})
	model2 := s.assertCreateModel(c, createModelParams{name: "model-2", username: "test2", cred: cred2})
	model3 := s.assertCreateModel(c, createModelParams{name: "model-3", username: "test2", cred: cred2})

	// Update some stats for the models we've just created'
	t0 := time.Unix(0, 0)

	err = s.JEM.DB.UpdateModelCounts(ctx, ctlPath, model1.UUID, map[params.EntityCount]int{
		params.UnitCount: 99,
	}, t0)

	c.Assert(err, gc.Equals, nil)
	err = s.JEM.DB.UpdateModelCounts(ctx, ctlPath, model2.UUID, map[params.EntityCount]int{
		params.MachineCount: 10,
	}, t0)

	c.Assert(err, gc.Equals, nil)
	err = s.JEM.DB.UpdateModelCounts(ctx, ctlPath, model3.UUID, map[params.EntityCount]int{
		params.ApplicationCount: 1,
	}, t0)

	c.Assert(err, gc.Equals, nil)

	// Allow test2/model-3 access to everyone, so that we can be sure we're
	// not seeing models that we have access to but aren't the creator of.
	err = s.JEM.DB.SetACL(ctx, s.JEM.DB.Models(), params.EntityPath{User: "test2", Name: "model-3"}, params.ACL{
		Read: []string{"test"},
	})

	// Open the API connection as user "test". We should only see the one model.
	conn := s.open(c, nil, "test")
	defer conn.Close()
	var resp params.UserModelStatsResponse
	err = conn.APICall("JIMM", 1, "", "UserModelStats", nil, &resp)
	c.Assert(err, gc.Equals, nil)

	c.Assert(resp, jc.DeepEquals, params.UserModelStatsResponse{
		Models: map[string]params.ModelStats{
			model1.UUID: {
				Model: jujuparams.Model{
					Name:     "model-1",
					UUID:     model1.UUID,
					OwnerTag: names.NewUserTag(model1.Owner).String(),
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

	// As test2, we should see the other two models.
	conn = s.open(c, nil, "test2")
	defer conn.Close()
	resp = params.UserModelStatsResponse{}
	err = conn.APICall("JIMM", 1, "", "UserModelStats", nil, &resp)
	c.Assert(err, gc.Equals, nil)

	c.Assert(resp, jc.DeepEquals, params.UserModelStatsResponse{
		Models: map[string]params.ModelStats{
			model2.UUID: {
				Model: jujuparams.Model{
					Name:     "model-2",
					UUID:     model2.UUID,
					OwnerTag: names.NewUserTag(model2.Owner).String(),
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
			model3.UUID: {
				Model: jujuparams.Model{
					Name:     "model-3",
					UUID:     model3.UUID,
					OwnerTag: names.NewUserTag(model3.Owner).String(),
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
	ctx := context.Background()
	ctlId0 := s.AssertAddController(ctx, c, params.EntityPath{User: "test", Name: "controller-0"}, true)
	ctlId1 := s.AssertAddController(ctx, c, params.EntityPath{User: "test", Name: "controller-1"}, true)
	ctlId2 := s.AssertAddController(ctx, c, params.EntityPath{User: "test", Name: "controller-2"}, true)

	c0, err := s.JEM.DB.Controller(context.Background(), ctlId0)
	c.Assert(err, gc.Equals, nil)
	c1, err := s.JEM.DB.Controller(context.Background(), ctlId1)
	c.Assert(err, gc.Equals, nil)
	c2, err := s.JEM.DB.Controller(context.Background(), ctlId2)
	c.Assert(err, gc.Equals, nil)

	// Open the API connection as user "test".
	conn := s.open(c, nil, "test")
	defer conn.Close()
	var resp params.ListControllerResponse
	err = conn.APICall("JIMM", 2, "", "ListControllers", nil, &resp)
	c.Assert(err, gc.Equals, nil)

	c.Assert(resp, jc.DeepEquals, params.ListControllerResponse{
		Controllers: []params.ControllerResponse{{
			Path:     ctlId0,
			Location: map[string]string{"cloud": "dummy", "region": "dummy-region"},
			Public:   true,
			UUID:     c0.UUID,
			Version:  c0.Version.String(),
		}, {
			Path:     ctlId1,
			Location: map[string]string{"cloud": "dummy", "region": "dummy-region"},
			Public:   true,
			UUID:     c1.UUID,
			Version:  c1.Version.String(),
		}, {
			Path:     ctlId2,
			Location: map[string]string{"cloud": "dummy", "region": "dummy-region"},
			Public:   true,
			UUID:     c2.UUID,
			Version:  c2.Version.String(),
		}},
	})
}

func (s *jimmSuite) TestListControllersUnauthorizedUser(c *gc.C) {
	ctx := context.Background()
	s.AssertAddController(ctx, c, params.EntityPath{User: "test", Name: "controller-0"}, true)
	s.AssertAddController(ctx, c, params.EntityPath{User: "test", Name: "controller-1"}, true)
	s.AssertAddController(ctx, c, params.EntityPath{User: "test", Name: "controller-2"}, true)

	// Open the API connection as user "unknown-user".
	conn := s.open(c, nil, "unknown-user")
	defer conn.Close()
	var resp params.ListControllerResponse
	err := conn.APICall("JIMM", 2, "", "ListControllers", nil, &resp)
	c.Assert(err, gc.Equals, nil)

	c.Assert(resp, jc.DeepEquals, params.ListControllerResponse{
		Controllers: []params.ControllerResponse{{
			Path:    params.EntityPath{User: "admin", Name: "jaas"},
			Public:  true,
			UUID:    "914487b5-60e7-42bb-bd63-1adc3fd3a388",
			Version: "0.0.0",
		}},
	})
}
