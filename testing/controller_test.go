// Copyright 2025 Canonical.

package testing

import (
	"sort"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/client/modelmanager"
	controllerapi "github.com/juju/juju/api/controller/controller"
	jujucontroller "github.com/juju/juju/controller"
	"github.com/juju/juju/core/life"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	"github.com/juju/names/v5"

	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
	jimmversion "github.com/canonical/jimm/v3/version"
)

func TestControllerConfigSetNotSupported(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupWebsocketEnv(c)

	conn := s.Open(c, nil, "test", nil)
	defer conn.Close()
	client := controllerapi.NewClient(conn)
	err := client.ConfigSet(nil)
	c.Assert(jujuparams.IsCodeNotSupported(err), qt.Equals, true)
}

func TestMongoVersion(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupWebsocketEnv(c)

	conn := s.Open(c, nil, "alice", nil)
	defer conn.Close()
	client := controllerapi.NewClient(conn)
	_, err := client.MongoVersion()
	c.Assert(err, qt.ErrorMatches, `not supported \(not supported\)`)
	c.Assert(jujuparams.IsCodeNotSupported(err), qt.Equals, true)
}

func TestAllModels(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupWebsocketEnv(c)

	conn := s.Open(c, nil, "bob", nil)
	defer conn.Close()
	client := controllerapi.NewClient(conn)

	models, err := client.AllModels()
	c.Assert(err, qt.Equals, nil)
	c.Assert(models, qt.ContentEquals, []base.UserModel{{
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

func TestModelStatus(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupWebsocketEnv(c)

	type modelStatuser interface {
		ModelStatus(tags ...names.ModelTag) ([]base.ModelStatus, error)
	}
	doTest := func(client modelStatuser) {
		models, err := client.ModelStatus(s.Model.ResourceTag(), s.Model3.ResourceTag())
		c.Assert(err, qt.Equals, nil)
		c.Assert(models, qt.HasLen, 2)
		c.Check(models[0], qt.DeepEquals, base.ModelStatus{
			Applications:       []base.Application{},
			UUID:               s.Model.UUID.String,
			Life:               life.Value(state.Alive.String()),
			Owner:              "bob@canonical.com",
			TotalMachineCount:  0,
			Volumes:            []base.Volume{},
			Filesystems:        []base.Filesystem{},
			CoreCount:          0,
			HostedMachineCount: 0,
			ApplicationCount:   0,
			Machines:           []base.Machine{},
			ModelType:          "iaas",
		})
		c.Check(models[1].Error, qt.ErrorMatches, `unauthorized`)
		status, err := client.ModelStatus(s.Model2.ResourceTag())
		c.Assert(err, qt.Equals, nil)
		c.Assert(status, qt.HasLen, 1)
		c.Check(status[0].Error, qt.ErrorMatches, "unauthorized")
	}

	conn := s.Open(c, nil, "bob", nil)
	defer conn.Close()
	doTest(controllerapi.NewClient(conn))
	doTest(modelmanager.NewClient(conn))
}

func TestIdentityProviderURL(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupWebsocketEnv(c)

	conn := s.Open(c, nil, "bob", nil)
	defer conn.Close()

	var result jujuparams.StringResult
	err := conn.APICall("Controller", 12, "", "IdentityProviderURL", nil, &result)
	c.Assert(err, qt.IsNil)
	c.Assert(result.Result, qt.Matches, ``)
}

func TestControllerVersion(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupWebsocketEnv(c)

	conn := s.Open(c, nil, "test", nil)
	defer conn.Close()

	var result jujuparams.ControllerVersionResults
	err := conn.APICall("Controller", 12, "", "ControllerVersion", nil, &result)
	c.Assert(err, qt.IsNil)
	c.Assert(result, qt.DeepEquals, jujuparams.ControllerVersionResults{
		Version:   "3.6.14",
		GitCommit: jimmversion.VersionInfo.GitCommit,
	})
}

func TestControllerAccess(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupWebsocketEnv(c)

	conn := s.Open(c, nil, "alice", nil)
	defer conn.Close()

	client := controllerapi.NewClient(conn)
	access, err := client.GetControllerAccess("alice@canonical.com")
	c.Assert(err, qt.Equals, nil)
	c.Check(string(access), qt.Equals, "superuser")

	access, err = client.GetControllerAccess("bob@canonical.com")
	c.Assert(err, qt.Equals, nil)
	c.Check(string(access), qt.Equals, "login")

	conn = s.Open(c, nil, "bob", nil)
	defer conn.Close()

	client = controllerapi.NewClient(conn)
	access, err = client.GetControllerAccess("bob@canonical.com")
	c.Assert(err, qt.Equals, nil)
	c.Check(string(access), qt.Equals, "login")

	_, err = client.GetControllerAccess("alice@canonical.com")
	c.Assert(err, qt.ErrorMatches, `unauthorized`)
}

func TestControllerConfig(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupWebsocketEnv(c)

	conn := s.Open(c, nil, "alice", nil)
	defer conn.Close()

	client := controllerapi.NewClient(conn)
	config, err := client.ControllerConfig()
	c.Assert(err, qt.Equals, nil)

	c.Assert(config[jujucontroller.ControllerUUIDKey], qt.Equals, s.JIMM.ControllerConfig.ControllerUUID)
	c.Assert(config[jujucontroller.PublicDNSAddress], qt.Equals, s.JIMM.ControllerConfig.PublicDNSName)
	c.Assert(config["ssh-host-key"], qt.Equals, s.JIMM.ControllerConfig.SSHPublicHostKey)
	// the reason we need to cast to float64 is because it is a map[string]interface{} and the json marshaller defaults to float64.
	c.Assert(config[jujucontroller.SSHServerPort], qt.Equals, float64(s.JIMM.ControllerConfig.SSHPort))
}

func TestWatchModelSummaries(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupWebsocketEnv(c)

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
	c.Assert(err, qt.IsNil)

	var summaries jujuparams.SummaryWatcherNextResults
	err = conn.APICall("ModelSummaryWatcher", 1, watcherID.WatcherID, "Next", nil, &summaries)
	c.Assert(err, qt.IsNil)
	c.Assert(summaries.Models, qt.DeepEquals, expectedModels)

	err = conn.APICall("ModelSummaryWatcher", 1, watcherID.WatcherID, "Stop", nil, nil)
	c.Assert(err, qt.IsNil)

	err = conn.APICall("ModelSummaryWatcher", 1, "unknown-id", "Next", nil, &summaries)
	c.Assert(err, qt.ErrorMatches, `not found \(not found\)`)
}

func TestWatchAllModelSummaries(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupWebsocketEnv(c)

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
	c.Assert(err, qt.IsNil)

	var summaries jujuparams.SummaryWatcherNextResults
	err = conn.APICall("ModelSummaryWatcher", 1, watcherID.WatcherID, "Next", nil, &summaries)
	c.Assert(err, qt.IsNil)
	c.Assert(summaries.Models, qt.DeepEquals, expectedModels)

	err = conn.APICall("ModelSummaryWatcher", 1, watcherID.WatcherID, "Stop", nil, nil)
	c.Assert(err, qt.IsNil)

	err = conn.APICall("ModelSummaryWatcher", 1, "unknown-id", "Next", nil, &summaries)
	c.Assert(err, qt.ErrorMatches, `not found \(not found\)`)
}
