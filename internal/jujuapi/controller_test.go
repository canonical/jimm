// Copyright 2016 Canonical Ltd.

package jujuapi_test

import (
	"context"
	"sort"
	"time"

	"github.com/juju/juju/api/base"
	controllerapi "github.com/juju/juju/api/controller"
	"github.com/juju/juju/api/modelmanager"
	jujuparams "github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/controller"
	jujuversion "github.com/juju/juju/version"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"

	"github.com/CanonicalLtd/jimm/params"
	jimmversion "github.com/CanonicalLtd/jimm/version"
)

type controllerSuite struct {
	websocketSuite
}

var _ = gc.Suite(&controllerSuite{})

func (s *controllerSuite) SetUpTest(c *gc.C) {
	s.ServerParams.CharmstoreLocation = "https://api.jujucharms.com/charmstore"
	s.ServerParams.MeteringLocation = "https://api.jujucharms.com/omnibus"
	s.websocketSuite.SetUpTest(c)
	s.PatchValue(&utils.OutgoingAccessAllowed, true)
}

func (s *controllerSuite) TestControllerConfig(c *gc.C) {
	conn := s.open(c, nil, "test")
	defer conn.Close()
	client := controllerapi.NewClient(conn)
	conf, err := client.ControllerConfig()
	c.Assert(err, gc.Equals, nil)
	c.Assert(conf, jc.DeepEquals, controller.Config(map[string]interface{}{
		"charmstore-url": "https://api.jujucharms.com/charmstore",
		"metering-url":   "https://api.jujucharms.com/omnibus",
	}))
}

func (s *controllerSuite) TestModelConfig(c *gc.C) {
	conn := s.open(c, nil, "test")
	defer conn.Close()
	client := controllerapi.NewClient(conn)
	_, err := client.ModelConfig()
	c.Assert(err, gc.ErrorMatches, `permission denied \(unauthorized access\)`)
	c.Assert(jujuparams.IsCodeUnauthorized(err), gc.Equals, true)
}

func (s *controllerSuite) TestAllModels(c *gc.C) {
	ctx := context.Background()

	ctlPath := s.AssertAddController(ctx, c, params.EntityPath{User: "test", Name: "controller-1"}, true)
	s.AssertUpdateCredential(ctx, c, "test", "dummy", "cred1", "empty")
	s.AssertUpdateCredential(ctx, c, "test2", "dummy", "cred1", "empty")
	err := s.JEM.DB.SetACL(ctx, s.JEM.DB.Controllers(), ctlPath, params.ACL{
		Read: []string{"test2"},
	})

	c.Assert(err, gc.Equals, nil)

	mi := s.assertCreateModel(c, createModelParams{name: "model-1", username: "test", cred: "cred1"})
	modelUUID1 := mi.UUID
	s.assertCreateModel(c, createModelParams{name: "model-2", username: "test2", cred: "cred1"})
	mi = s.assertCreateModel(c, createModelParams{name: "model-3", username: "test2", cred: "cred1"})
	modelUUID3 := mi.UUID

	conn := s.open(c, nil, "test")
	defer conn.Close()
	client := controllerapi.NewClient(conn)

	err = s.JEM.DB.SetACL(ctx, s.JEM.DB.Models(), params.EntityPath{User: "test2", Name: "model-3"}, params.ACL{
		Read: []string{"test"},
	})

	c.Assert(err, gc.Equals, nil)

	models, err := client.AllModels()
	c.Assert(err, gc.Equals, nil)
	c.Assert(models, jc.DeepEquals, []base.UserModel{{
		Name:           "model-1",
		UUID:           modelUUID1,
		Owner:          "test@external",
		LastConnection: nil,
		Type:           "iaas",
	}, {
		Name:           "model-3",
		UUID:           modelUUID3,
		Owner:          "test2@external",
		LastConnection: nil,
		Type:           "iaas",
	}})
}

func (s *controllerSuite) TestModelStatus(c *gc.C) {
	ctx := context.Background()

	ctlPath := s.AssertAddController(ctx, c, params.EntityPath{User: "test", Name: "controller-1"}, true)
	s.AssertUpdateCredential(ctx, c, "test", "dummy", "cred1", "empty")
	s.AssertUpdateCredential(ctx, c, "test2", "dummy", "cred1", "empty")
	err := s.JEM.DB.SetACL(ctx, s.JEM.DB.Controllers(), ctlPath, params.ACL{
		Read: []string{"test2"},
	})

	c.Assert(err, gc.Equals, nil)

	mi := s.assertCreateModel(c, createModelParams{name: "model-1", username: "test", cred: "cred1"})
	modelUUID1 := mi.UUID
	mi = s.assertCreateModel(c, createModelParams{name: "model-2", username: "test2", cred: "cred1"})
	modelUUID2 := mi.UUID
	mi = s.assertCreateModel(c, createModelParams{name: "model-3", username: "test2", cred: "cred1"})
	modelUUID3 := mi.UUID

	err = s.JEM.DB.SetACL(ctx, s.JEM.DB.Models(), params.EntityPath{User: "test2", Name: "model-3"}, params.ACL{
		Read: []string{"test"},
	})

	c.Assert(err, gc.Equals, nil)

	type modelStatuser interface {
		ModelStatus(tags ...names.ModelTag) ([]base.ModelStatus, error)
	}
	doTest := func(client modelStatuser) {
		models, err := client.ModelStatus(names.NewModelTag(modelUUID1), names.NewModelTag(modelUUID3))
		c.Assert(err, gc.Equals, nil)
		c.Assert(models, gc.HasLen, 2)
		c.Check(models[0], jc.DeepEquals, base.ModelStatus{
			UUID:               modelUUID1,
			Life:               "alive",
			Owner:              "test@external",
			TotalMachineCount:  0,
			CoreCount:          0,
			HostedMachineCount: 0,
			ApplicationCount:   0,
			Machines:           []base.Machine{},
			ModelType:          "iaas",
		})
		c.Check(models[1].Error, gc.ErrorMatches, `unauthorized`)
		status, err := client.ModelStatus(names.NewModelTag(modelUUID2))
		c.Assert(err, gc.Equals, nil)
		c.Assert(status, gc.HasLen, 1)
		c.Check(status[0].Error, gc.ErrorMatches, "unauthorized")
	}

	conn := s.open(c, nil, "test")
	defer conn.Close()
	doTest(controllerapi.NewClient(conn))
	doTest(modelmanager.NewClient(conn))
}

func (s *controllerSuite) TestMongoVersion(c *gc.C) {
	ctx := context.Background()

	s.AssertAddController(ctx, c, params.EntityPath{User: "test", Name: "controller-1"}, true)
	s.AssertUpdateCredential(ctx, c, "test", "dummy", "cred1", "empty")

	conn := s.open(c, nil, "test")
	defer conn.Close()

	var version jujuparams.StringResult
	err := conn.APICall("Controller", 6, "", "MongoVersion", nil, &version)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(version.Result, gc.Not(gc.Equals), "")

	err = conn.APICall("Controller", 9, "", "MongoVersion", nil, &version)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(version.Result, gc.Not(gc.Equals), "")
}

func (s *controllerSuite) TestConfigSet(c *gc.C) {
	ctx := context.Background()

	s.AssertAddController(ctx, c, params.EntityPath{User: "test", Name: "controller-1"}, true)
	s.AssertUpdateCredential(ctx, c, "test", "dummy", "cred1", "empty")

	conn := s.open(c, nil, "test")
	defer conn.Close()

	err := conn.APICall("Controller", 5, "", "ConfigSet", jujuparams.ControllerConfigSet{}, nil)
	c.Assert(err, jc.ErrorIsNil)

	err = conn.APICall("Controller", 9, "", "ConfigSet", jujuparams.ControllerConfigSet{}, nil)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *controllerSuite) TestIdentityProviderURL(c *gc.C) {
	ctx := context.Background()

	s.AssertAddController(ctx, c, params.EntityPath{User: "test", Name: "controller-1"}, true)
	s.AssertUpdateCredential(ctx, c, "test", "dummy", "cred1", "empty")

	conn := s.open(c, nil, "test")
	defer conn.Close()

	var result jujuparams.StringResult
	err := conn.APICall("Controller", 7, "", "IdentityProviderURL", nil, &result)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Result, gc.Matches, `https://127\.0\.0\.1.*`)

	err = conn.APICall("Controller", 9, "", "IdentityProviderURL", nil, &result)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Result, gc.Matches, `https://127\.0\.0\.1.*`)
}

func (s *controllerSuite) TestControllerVersion(c *gc.C) {
	ctx := context.Background()

	s.AssertAddController(ctx, c, params.EntityPath{User: "test", Name: "controller-1"}, true)
	s.AssertUpdateCredential(ctx, c, "test", "dummy", "cred1", "empty")

	conn := s.open(c, nil, "test")
	defer conn.Close()

	var result jujuparams.ControllerVersionResults
	err := conn.APICall("Controller", 8, "", "ControllerVersion", nil, &result)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, jujuparams.ControllerVersionResults{
		Version:   jujuversion.Current.String(),
		GitCommit: jimmversion.VersionInfo.GitCommit,
	})

	err = conn.APICall("Controller", 9, "", "ControllerVersion", nil, &result)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, jujuparams.ControllerVersionResults{
		Version:   jujuversion.Current.String(),
		GitCommit: jimmversion.VersionInfo.GitCommit,
	})
}

func (s *controllerSuite) TestWatchModelSummaries(c *gc.C) {
	ctx := context.Background()

	ctlPath := s.AssertAddController(ctx, c, params.EntityPath{User: "test", Name: "controller-1"}, true)
	s.AssertUpdateCredential(ctx, c, "test", "dummy", "cred1", "empty")
	s.AssertUpdateCredential(ctx, c, "test2", "dummy", "cred1", "empty")
	err := s.JEM.DB.SetACL(ctx, s.JEM.DB.Controllers(), ctlPath, params.ACL{
		Read: []string{"test2"},
	})

	c.Assert(err, gc.Equals, nil)

	mi := s.assertCreateModel(c, createModelParams{name: "model-1", username: "test", cred: "cred1"})
	modelUUID1 := mi.UUID
	s.assertCreateModel(c, createModelParams{name: "model-2", username: "test2", cred: "cred1"})
	mi = s.assertCreateModel(c, createModelParams{name: "model-3", username: "test2", cred: "cred1"})
	modelUUID3 := mi.UUID
	c.Logf("models: %v %v", modelUUID1, modelUUID3)

	err = s.JEM.DB.SetACL(ctx, s.JEM.DB.Models(), params.EntityPath{User: "test2", Name: "model-3"}, params.ACL{
		Read: []string{"test"},
	})
	c.Assert(err, gc.Equals, nil)

	done := s.Pubsub.Publish(modelUUID1, jujuparams.ModelAbstract{
		UUID:  modelUUID1,
		Cloud: "test-cloud",
		Name:  "test-name-1",
	})
	select {
	case <-done:
	case <-time.After(time.Second):
		c.Fatalf("timed out")
	}
	done = s.Pubsub.Publish(modelUUID3, jujuparams.ModelAbstract{
		UUID:  modelUUID3,
		Cloud: "test-cloud",
		Name:  "test-name-3",
	})
	select {
	case <-done:
	case <-time.After(time.Second):
		c.Fatalf("timed out")
	}

	expectedModels := []jujuparams.ModelAbstract{{
		UUID:  modelUUID1,
		Cloud: "test-cloud",
		Name:  "test-name-1",
	}, {
		UUID:  modelUUID3,
		Cloud: "test-cloud",
		Name:  "test-name-3",
	}}
	sort.Slice(expectedModels, func(i, j int) bool {
		return expectedModels[i].UUID < expectedModels[j].UUID
	})

	conn := s.open(c, nil, "test")
	defer conn.Close()

	var watcherID jujuparams.SummaryWatcherID
	err = conn.APICall("Controller", 9, "", "WatchModelSummaries", nil, &watcherID)
	c.Assert(err, jc.ErrorIsNil)

	var summaries jujuparams.SummaryWatcherNextResults
	err = conn.APICall("ModelSummaryWatcher", 1, watcherID.WatcherID, "Next", nil, &summaries)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(summaries.Models, gc.DeepEquals, expectedModels)

	err = conn.APICall("ModelSummaryWatcher", 1, watcherID.WatcherID, "Stop", nil, nil)
	c.Assert(err, jc.ErrorIsNil)

	err = conn.APICall("ModelSummaryWatcher", 1, "unknown-id", "Next", nil, &summaries)
	c.Assert(err, gc.ErrorMatches, `not found \(not found\)`)
}
