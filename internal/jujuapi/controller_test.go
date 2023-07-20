// Copyright 2016 Canonical Ltd.

package jujuapi_test

import (
	"sort"
	"time"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/client/modelmanager"
	controllerapi "github.com/juju/juju/api/controller/controller"
	"github.com/juju/juju/controller"
	jujuparams "github.com/juju/juju/rpc/params"
	jujuversion "github.com/juju/juju/version"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/canonical/jimm/internal/pubsub"
	jimmversion "github.com/canonical/jimm/version"
)

type controllerSuite struct {
	websocketSuite
}

var _ = gc.Suite(&controllerSuite{})

func (s *controllerSuite) TestControllerConfig(c *gc.C) {
	conn := s.open(c, nil, "test")
	defer conn.Close()
	client := controllerapi.NewClient(conn)
	conf, err := client.ControllerConfig()
	c.Assert(err, gc.Equals, nil)
	c.Assert(conf, jc.DeepEquals, controller.Config(map[string]interface{}{}))

	adminConn := s.open(c, nil, "alice")
	defer adminConn.Close()
	err = adminConn.APICall("Controller", 9, "", "ConfigSet", jujuparams.ControllerConfigSet{
		Config: map[string]interface{}{
			"key1":           "value1",
			"key2":           "value2",
			"charmstore-url": "https://api.jujucharms.com/charmstore",
			"metering-url":   "https://api.jujucharms.com/omnibus",
		},
	}, nil)
	c.Assert(err, jc.ErrorIsNil)

	client = controllerapi.NewClient(adminConn)
	conf, err = client.ControllerConfig()
	c.Assert(err, gc.Equals, nil)
	c.Assert(conf, jc.DeepEquals, controller.Config(map[string]interface{}{
		"key1":           "value1",
		"key2":           "value2",
		"charmstore-url": "https://api.jujucharms.com/charmstore",
		"metering-url":   "https://api.jujucharms.com/omnibus",
	}))
}

func (s *controllerSuite) TestModelConfig(c *gc.C) {
	conn := s.open(c, nil, "test")
	defer conn.Close()
	client := controllerapi.NewClient(conn)
	_, err := client.ModelConfig()
	c.Assert(err, gc.ErrorMatches, `not supported \(not supported\)`)
	c.Assert(jujuparams.IsCodeNotSupported(err), gc.Equals, true)

	conn = s.open(c, nil, "alice")
	defer conn.Close()
	client = controllerapi.NewClient(conn)
	_, err = client.ModelConfig()
	c.Assert(err, gc.ErrorMatches, `not supported \(not supported\)`)
	c.Assert(jujuparams.IsCodeNotSupported(err), gc.Equals, true)
}

func (s *controllerSuite) TestMongoVersion(c *gc.C) {
	conn := s.open(c, nil, "alice")
	defer conn.Close()
	client := controllerapi.NewClient(conn)
	_, err := client.MongoVersion()
	c.Assert(err, gc.ErrorMatches, `not supported \(not supported\)`)
	c.Assert(jujuparams.IsCodeNotSupported(err), gc.Equals, true)
}

func (s *controllerSuite) TestAllModels(c *gc.C) {
	conn := s.open(c, nil, "bob")
	defer conn.Close()
	client := controllerapi.NewClient(conn)

	models, err := client.AllModels()
	c.Assert(err, gc.Equals, nil)
	c.Assert(models, jc.DeepEquals, []base.UserModel{{
		Name:           "model-1",
		UUID:           s.Model.UUID.String,
		Owner:          "bob@external",
		LastConnection: nil,
		Type:           "iaas",
	}, {
		Name:           "model-3",
		UUID:           s.Model3.UUID.String,
		Owner:          "charlie@external",
		LastConnection: nil,
		Type:           "iaas",
	}})
}

func (s *controllerSuite) TestModelStatus(c *gc.C) {
	type modelStatuser interface {
		ModelStatus(tags ...names.ModelTag) ([]base.ModelStatus, error)
	}
	doTest := func(client modelStatuser) {
		models, err := client.ModelStatus(s.Model.ResourceTag(), s.Model3.ResourceTag())
		c.Assert(err, gc.Equals, nil)
		c.Assert(models, gc.HasLen, 2)
		c.Check(models[0], jc.DeepEquals, base.ModelStatus{
			UUID:               s.Model.UUID.String,
			Life:               "alive",
			Owner:              "bob@external",
			TotalMachineCount:  0,
			CoreCount:          0,
			HostedMachineCount: 0,
			ApplicationCount:   0,
			Machines:           []base.Machine{},
			ModelType:          "iaas",
		})
		c.Check(models[1].Error, gc.ErrorMatches, `unauthorized`)
		status, err := client.ModelStatus(s.Model2.ResourceTag())
		c.Assert(err, gc.Equals, nil)
		c.Assert(status, gc.HasLen, 1)
		c.Check(status[0].Error, gc.ErrorMatches, "unauthorized")
	}

	conn := s.open(c, nil, "bob")
	defer conn.Close()
	doTest(controllerapi.NewClient(conn))
	doTest(modelmanager.NewClient(conn))
}

func (s *controllerSuite) TestConfigSet(c *gc.C) {
	conn := s.open(c, nil, "alice")
	defer conn.Close()

	err := conn.APICall("Controller", 5, "", "ConfigSet", jujuparams.ControllerConfigSet{}, nil)
	c.Assert(err, jc.ErrorIsNil)

	err = conn.APICall("Controller", 9, "", "ConfigSet", jujuparams.ControllerConfigSet{}, nil)
	c.Assert(err, jc.ErrorIsNil)

	conn1 := s.open(c, nil, "bob")
	defer conn1.Close()

	err = conn1.APICall("Controller", 5, "", "ConfigSet", jujuparams.ControllerConfigSet{}, nil)
	c.Assert(err, gc.ErrorMatches, `unauthorized \(unauthorized access\)`)

	err = conn1.APICall("Controller", 9, "", "ConfigSet", jujuparams.ControllerConfigSet{}, nil)
	c.Assert(err, gc.ErrorMatches, `unauthorized \(unauthorized access\)`)
}

func (s *controllerSuite) TestIdentityProviderURL(c *gc.C) {
	conn := s.open(c, nil, "bob")
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

func (s *controllerSuite) TestControllerAccess(c *gc.C) {
	conn := s.open(c, nil, "alice")
	defer conn.Close()

	client := controllerapi.NewClient(conn)
	access, err := client.GetControllerAccess("alice@external")
	c.Assert(err, gc.Equals, nil)
	c.Check(string(access), gc.Equals, "superuser")

	access, err = client.GetControllerAccess("bob@external")
	c.Assert(err, gc.Equals, nil)
	c.Check(string(access), gc.Equals, "login")

	conn = s.open(c, nil, "bob")
	defer conn.Close()

	client = controllerapi.NewClient(conn)
	access, err = client.GetControllerAccess("bob@external")
	c.Assert(err, gc.Equals, nil)
	c.Check(string(access), gc.Equals, "login")

	_, err = client.GetControllerAccess("alice@external")
	c.Assert(err, gc.ErrorMatches, `unauthorized`)
}

type watcherSuite struct {
	websocketSuite
}

var _ = gc.Suite(&watcherSuite{})

func (s *watcherSuite) SetUpTest(c *gc.C) {
	s.websocketSuite.SetUpTest(c)
	s.JIMM.Pubsub = &pubsub.Hub{MaxConcurrency: 10}
}

func (s *watcherSuite) TestWatchModelSummaries(c *gc.C) {
	c.Logf("models: %v %v", s.Model.UUID.String, s.Model3.UUID.String)

	done := s.JIMM.Pubsub.Publish(s.Model.UUID.String, jujuparams.ModelAbstract{
		UUID:  s.Model.UUID.String,
		Cloud: "test-cloud",
		Name:  "test-name-1",
	})
	select {
	case <-done:
	case <-time.After(time.Second):
		c.Fatalf("timed out")
	}
	done = s.JIMM.Pubsub.Publish(s.Model3.UUID.String, jujuparams.ModelAbstract{
		UUID:  s.Model3.UUID.String,
		Cloud: "test-cloud",
		Name:  "test-name-3",
	})
	select {
	case <-done:
	case <-time.After(time.Second):
		c.Fatalf("timed out")
	}

	expectedModels := []jujuparams.ModelAbstract{{
		UUID:  s.Model.UUID.String,
		Cloud: "test-cloud",
		Name:  "test-name-1",
	}, {
		UUID:  s.Model3.UUID.String,
		Cloud: "test-cloud",
		Name:  "test-name-3",
	}}
	sort.Slice(expectedModels, func(i, j int) bool {
		return expectedModels[i].UUID < expectedModels[j].UUID
	})

	conn := s.open(c, nil, "bob")
	defer conn.Close()

	var watcherID jujuparams.SummaryWatcherID
	err := conn.APICall("Controller", 9, "", "WatchModelSummaries", nil, &watcherID)
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

func (s *watcherSuite) TestWatchAllModelSummaries(c *gc.C) {
	c.Logf("models: %v %v", s.Model.UUID.String, s.Model3.UUID.String)

	done := s.JIMM.Pubsub.Publish(s.Model.UUID.String, jujuparams.ModelAbstract{
		UUID:  s.Model.UUID.String,
		Cloud: "test-cloud",
		Name:  "test-name-1",
	})
	select {
	case <-done:
	case <-time.After(time.Second):
		c.Fatalf("timed out")
	}
	done = s.JIMM.Pubsub.Publish(s.Model3.UUID.String, jujuparams.ModelAbstract{
		UUID:  s.Model3.UUID.String,
		Cloud: "test-cloud",
		Name:  "test-name-3",
	})
	select {
	case <-done:
	case <-time.After(time.Second):
		c.Fatalf("timed out")
	}

	expectedModels := []jujuparams.ModelAbstract{{
		UUID:  s.Model.UUID.String,
		Cloud: "test-cloud",
		Name:  "test-name-1",
	}, {
		UUID:  s.Model3.UUID.String,
		Cloud: "test-cloud",
		Name:  "test-name-3",
	}}
	sort.Slice(expectedModels, func(i, j int) bool {
		return expectedModels[i].UUID < expectedModels[j].UUID
	})

	conn := s.open(c, nil, "alice")
	defer conn.Close()

	var watcherID jujuparams.SummaryWatcherID
	err := conn.APICall("Controller", 9, "", "WatchAllModelSummaries", nil, &watcherID)
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
