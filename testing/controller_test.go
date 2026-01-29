// Copyright 2025 Canonical.

package testing

import (
	"sort"
	"time"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/client/modelmanager"
	controllerapi "github.com/juju/juju/api/controller/controller"
	jujucontroller "github.com/juju/juju/controller"
	"github.com/juju/juju/core/life"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
	jimmversion "github.com/canonical/jimm/v3/version"
)

type controllerSuite struct {
	jimmtest.WebsocketE2ESuite
}

var _ = gc.Suite(&controllerSuite{})

func (s *controllerSuite) TestControllerConfigSetNotSupported(c *gc.C) {
	conn := s.Open(c, nil, "test", nil)
	defer conn.Close()
	client := controllerapi.NewClient(conn)
	err := client.ConfigSet(nil)
	c.Assert(jujuparams.IsCodeNotSupported(err), gc.Equals, true)
}

func (s *controllerSuite) TestMongoVersion(c *gc.C) {
	conn := s.Open(c, nil, "alice", nil)
	defer conn.Close()
	client := controllerapi.NewClient(conn)
	_, err := client.MongoVersion()
	c.Assert(err, gc.ErrorMatches, `not supported \(not supported\)`)
	c.Assert(jujuparams.IsCodeNotSupported(err), gc.Equals, true)
}

func (s *controllerSuite) TestAllModels(c *gc.C) {
	conn := s.Open(c, nil, "bob", nil)
	defer conn.Close()
	client := controllerapi.NewClient(conn)

	models, err := client.AllModels()
	c.Assert(err, gc.Equals, nil)
	c.Assert(models, jc.SameContents, []base.UserModel{{
		Name:           s.Model.Name,
		UUID:           s.Model.UUID.String,
		Owner:          "bob@canonical.com",
		LastConnection: nil,
		Type:           "iaas",
	}, {
		Name:           s.Model3.Name,
		UUID:           s.Model3.UUID.String,
		Owner:          "charlie@canonical.com",
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
			Life:               life.Value(state.Alive.String()),
			Owner:              "bob@canonical.com",
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

	conn := s.Open(c, nil, "bob", nil)
	defer conn.Close()
	doTest(controllerapi.NewClient(conn))
	doTest(modelmanager.NewClient(conn))
}

func (s *controllerSuite) TestIdentityProviderURL(c *gc.C) {
	conn := s.Open(c, nil, "bob", nil)
	defer conn.Close()

	var result jujuparams.StringResult
	err := conn.APICall("Controller", 12, "", "IdentityProviderURL", nil, &result)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Result, gc.Matches, ``)
}

func (s *controllerSuite) TestControllerVersion(c *gc.C) {
	conn := s.Open(c, nil, "test", nil)
	defer conn.Close()

	var result jujuparams.ControllerVersionResults
	err := conn.APICall("Controller", 12, "", "ControllerVersion", nil, &result)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, jujuparams.ControllerVersionResults{
		Version:   "3.6.14",
		GitCommit: jimmversion.VersionInfo.GitCommit,
	})
}

func (s *controllerSuite) TestControllerAccess(c *gc.C) {
	conn := s.Open(c, nil, "alice", nil)
	defer conn.Close()

	client := controllerapi.NewClient(conn)
	access, err := client.GetControllerAccess("alice@canonical.com")
	c.Assert(err, gc.Equals, nil)
	c.Check(string(access), gc.Equals, "superuser")

	access, err = client.GetControllerAccess("bob@canonical.com")
	c.Assert(err, gc.Equals, nil)
	c.Check(string(access), gc.Equals, "login")

	conn = s.Open(c, nil, "bob", nil)
	defer conn.Close()

	client = controllerapi.NewClient(conn)
	access, err = client.GetControllerAccess("bob@canonical.com")
	c.Assert(err, gc.Equals, nil)
	c.Check(string(access), gc.Equals, "login")

	_, err = client.GetControllerAccess("alice@canonical.com")
	c.Assert(err, gc.ErrorMatches, `unauthorized`)
}

func (s *controllerSuite) TestControllerConfig(c *gc.C) {
	conn := s.Open(c, nil, "alice", nil)
	defer conn.Close()

	client := controllerapi.NewClient(conn)
	config, err := client.ControllerConfig()
	c.Assert(err, gc.Equals, nil)

	c.Assert(config[jujucontroller.ControllerUUIDKey], gc.Equals, s.JIMM.ControllerConfig.ControllerUUID)
	c.Assert(config[jujucontroller.PublicDNSAddress], gc.Equals, s.JIMM.ControllerConfig.PublicDNSName)
	c.Assert(config["ssh-host-key"], gc.Equals, s.JIMM.ControllerConfig.SSHPublicHostKey)
	// the reason we need to cast to float64 is because it is a map[string]interface{} and the json marshaller defaults to float64.
	c.Assert(config[jujucontroller.SSHServerPort], gc.Equals, float64(s.JIMM.ControllerConfig.SSHPort))
}

type watcherSuite struct {
	jimmtest.WebsocketE2ESuite
}

var _ = gc.Suite(&watcherSuite{})

func (s *watcherSuite) TestWatchModelSummaries(c *gc.C) {
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

	conn := s.Open(c, nil, "bob", nil)
	defer conn.Close()

	var watcherID jujuparams.SummaryWatcherID
	err := conn.APICall("Controller", 12, "", "WatchModelSummaries", nil, &watcherID)
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

	conn := s.Open(c, nil, "alice", nil)
	defer conn.Close()

	var watcherID jujuparams.SummaryWatcherID
	err := conn.APICall("Controller", 12, "", "WatchAllModelSummaries", nil, &watcherID)
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
