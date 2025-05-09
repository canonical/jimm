// Copyright 2025 Canonical.

package jujuapi_test

import (
	"context"
	"sort"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
	"github.com/google/uuid"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/client/modelmanager"
	controllerapi "github.com/juju/juju/api/controller/controller"
	jujucontroller "github.com/juju/juju/controller"
	"github.com/juju/juju/core/life"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	jujuversion "github.com/juju/juju/version"
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/jimm"
	"github.com/canonical/jimm/v3/internal/jujuapi"
	"github.com/canonical/jimm/v3/internal/openfga"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest/mocks"
	jimmversion "github.com/canonical/jimm/v3/version"
)

type controllerSuite struct {
	websocketSuite
}

var _ = gc.Suite(&controllerSuite{})

func (s *controllerSuite) TestControllerConfigSetNotSupported(c *gc.C) {
	conn := s.open(c, nil, "test")
	defer conn.Close()
	client := controllerapi.NewClient(conn)
	err := client.ConfigSet(nil)
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
	c.Assert(models, jc.SameContents, []base.UserModel{{
		Name:           "model-1",
		UUID:           s.Model.UUID.String,
		Owner:          "bob@canonical.com",
		LastConnection: nil,
		Type:           "iaas",
	}, {
		Name:           "model-3",
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

	conn := s.open(c, nil, "bob")
	defer conn.Close()
	doTest(controllerapi.NewClient(conn))
	doTest(modelmanager.NewClient(conn))
}

func (s *controllerSuite) TestIdentityProviderURL(c *gc.C) {
	conn := s.open(c, nil, "bob")
	defer conn.Close()

	var result jujuparams.StringResult
	err := conn.APICall("Controller", 11, "", "IdentityProviderURL", nil, &result)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Result, gc.Matches, ``)
}

func (s *controllerSuite) TestControllerVersion(c *gc.C) {
	conn := s.open(c, nil, "test")
	defer conn.Close()

	var result jujuparams.ControllerVersionResults
	err := conn.APICall("Controller", 11, "", "ControllerVersion", nil, &result)
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
	access, err := client.GetControllerAccess("alice@canonical.com")
	c.Assert(err, gc.Equals, nil)
	c.Check(string(access), gc.Equals, "superuser")

	access, err = client.GetControllerAccess("bob@canonical.com")
	c.Assert(err, gc.Equals, nil)
	c.Check(string(access), gc.Equals, "login")

	conn = s.open(c, nil, "bob")
	defer conn.Close()

	client = controllerapi.NewClient(conn)
	access, err = client.GetControllerAccess("bob@canonical.com")
	c.Assert(err, gc.Equals, nil)
	c.Check(string(access), gc.Equals, "login")

	_, err = client.GetControllerAccess("alice@canonical.com")
	c.Assert(err, gc.ErrorMatches, `unauthorized`)
}

func (s *controllerSuite) TestControllerConfig(c *gc.C) {
	conn := s.open(c, nil, "alice")
	defer conn.Close()

	client := controllerapi.NewClient(conn)
	config, err := client.ControllerConfig()
	c.Assert(err, gc.Equals, nil)

	c.Assert(config[jujucontroller.ControllerUUIDKey], gc.Equals, s.JIMM.ControllerConfig.ControllerUUID)
	c.Assert(config[jujucontroller.PublicDNSAddress], gc.Equals, s.JIMM.ControllerConfig.PublicDNSName)
	c.Assert(config["ssh-server-hostkey"], gc.Equals, s.JIMM.ControllerConfig.SSHPublicHostKey)
	// the reason we need to cast to float64 is because it is a map[string]interface{} and the json marshaller defaults to float64.
	c.Assert(config[jujucontroller.SSHServerPort], gc.Equals, float64(s.JIMM.ControllerConfig.SSHPort))
	c.Assert(config[jujucontroller.SSHMaxConcurrentConnections], gc.Equals, float64(s.JIMM.ControllerConfig.SSHMaxConcurrentConnections))
	c.Assert(config[jujucontroller.APIPort], gc.Equals, float64(s.JIMM.ControllerConfig.APIPort))
}

type watcherSuite struct {
	websocketSuite
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

	conn := s.open(c, nil, "bob")
	defer conn.Close()

	var watcherID jujuparams.SummaryWatcherID
	err := conn.APICall("Controller", 11, "", "WatchModelSummaries", nil, &watcherID)
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

	conn := s.open(c, nil, "alice")
	defer conn.Close()

	var watcherID jujuparams.SummaryWatcherID
	err := conn.APICall("Controller", 11, "", "WatchAllModelSummaries", nil, &watcherID)
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

func TestInitiateMigration(t *testing.T) {
	c := qt.New(t)

	mt := names.NewModelTag(uuid.New().String())
	migrationID := uuid.New().String()

	tests := []struct {
		about             string
		initiateMigration func(ctx context.Context, user *openfga.User, spec jujuparams.MigrationSpec) (jujuparams.InitiateMigrationResult, error)
		args              jujuparams.InitiateMigrationArgs
		expectedError     string
		expectedResult    jujuparams.InitiateMigrationResults
	}{{
		about: "model migration initiated successfully",
		initiateMigration: func(ctx context.Context, user *openfga.User, spec jujuparams.MigrationSpec) (jujuparams.InitiateMigrationResult, error) {
			return jujuparams.InitiateMigrationResult{
				ModelTag:    mt.String(),
				MigrationId: migrationID,
			}, nil
		},
		args: jujuparams.InitiateMigrationArgs{
			Specs: []jujuparams.MigrationSpec{{
				ModelTag: mt.String(),
			}},
		},
		expectedResult: jujuparams.InitiateMigrationResults{
			Results: []jujuparams.InitiateMigrationResult{{
				ModelTag:    mt.String(),
				MigrationId: migrationID,
			}},
		},
	}, {
		about: "controller returns an error",
		initiateMigration: func(ctx context.Context, user *openfga.User, spec jujuparams.MigrationSpec) (jujuparams.InitiateMigrationResult, error) {
			return jujuparams.InitiateMigrationResult{}, errors.E("a silly error")
		},
		args: jujuparams.InitiateMigrationArgs{
			Specs: []jujuparams.MigrationSpec{{
				ModelTag: mt.String(),
			}},
		},
		expectedResult: jujuparams.InitiateMigrationResults{
			Results: []jujuparams.InitiateMigrationResult{{
				Error: &jujuparams.Error{
					Message: "a silly error",
				},
			}},
		},
	}}

	for _, test := range tests {
		test := test
		c.Run(test.about, func(c *qt.C) {
			jujuManager := mocks.JujuManager{
				InitiateMigration_: test.initiateMigration,
			}
			jimm := &jimmtest.JIMM{
				JujuManager_: func() jimm.JujuManager {
					return &jujuManager
				},
			}
			cr := jujuapi.NewControllerRoot(jimm, jujuapi.Params{})

			result, err := cr.InitiateMigration(context.Background(), test.args)
			if test.expectedError == "" {
				c.Assert(err, qt.IsNil)
				c.Assert(result, qt.DeepEquals, test.expectedResult)
			} else {
				c.Assert(err, qt.ErrorMatches, test.expectedError)
			}
		})
	}
}
