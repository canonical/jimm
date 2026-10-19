// Copyright 2025 Canonical.

package testing

import (
	"context"
	"sort"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/client/modelmanager"
	controllerapi "github.com/juju/juju/api/controller/controller"
	jujucontroller "github.com/juju/juju/controller"
	"github.com/juju/juju/core/life"
	coremodel "github.com/juju/juju/core/model"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v6"

	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
	jimmversion "github.com/canonical/jimm/v3/version"
)

func TestControllerConfigSetNotSupported(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupJimmWithControllers(c)

	conn := s.Open(c, nil, "test@canonical.com", nil)
	defer conn.Close()
	client := controllerapi.NewClient(conn)
	err := client.ConfigSet(t.Context(), nil)
	c.Assert(jujuparams.IsCodeNotSupported(err), qt.Equals, true)
}

func TestAllModels(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupJimmWithControllers(c)
	model := s.CreateModelForBob(c)
	model3 := s.CreateModelForCharlieWithBobReadAccess(c)

	conn := s.Open(c, nil, "bob@canonical.com", nil)
	defer conn.Close()
	client := controllerapi.NewClient(conn)

	models, err := client.AllModels(t.Context())
	c.Assert(err, qt.Equals, nil)
	c.Assert(models, qt.ContentEquals, []base.UserModel{{
		Name:           model.Name,
		UUID:           model.UUID.String,
		Qualifier:      "bob@canonical.com",
		LastConnection: nil,
	}, {
		Name:           model3.Name,
		UUID:           model3.UUID.String,
		Qualifier:      "charlie@canonical.com",
		LastConnection: nil,
	}})
}

func TestModelStatus(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupJimmWithControllers(c)
	model := s.CreateModelForBob(c)
	model2 := s.CreateModelForCharlie(c)
	model3 := s.CreateModelForCharlieWithBobReadAccess(c)

	type modelStatuser interface {
		ModelStatus(ctx context.Context, tags ...names.ModelTag) ([]base.ModelStatus, error)
	}
	doTest := func(client modelStatuser) {
		models, err := client.ModelStatus(t.Context(), model.ResourceTag(), model3.ResourceTag())
		c.Assert(err, qt.Equals, nil)
		c.Assert(models, qt.HasLen, 2)
		c.Check(models[0], qt.DeepEquals, base.ModelStatus{
			Applications:       []base.Application{},
			UUID:               model.UUID.String,
			Life:               life.Value(string(life.Alive)),
			Qualifier:          coremodel.Qualifier("bob@canonical.com"),
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
		status, err := client.ModelStatus(t.Context(), model2.ResourceTag())
		c.Assert(err, qt.Equals, nil)
		c.Assert(status, qt.HasLen, 1)
		c.Check(status[0].Error, qt.ErrorMatches, "unauthorized")
	}

	conn := s.Open(c, nil, "bob@canonical.com", nil)
	defer conn.Close()
	doTest(controllerapi.NewClient(conn))
	doTest(modelmanager.NewClient(conn))
}

func TestModelStatus_ModelDeletedOnBackingController(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupJimmWithControllers(c)

	client, model := createAndDestroyBackingModel(c, s)

	statuses, err := client.ModelStatus(t.Context(), model.ResourceTag())
	c.Assert(err, qt.IsNil)
	c.Assert(statuses, qt.HasLen, 1)
	c.Check(statuses[0], qt.DeepEquals, base.ModelStatus{
		UUID:         model.UUID.String,
		Life:         life.Alive,
		Qualifier:    "bob@canonical.com",
		Applications: []base.Application{},
		Machines:     []base.Machine{},
		Volumes:      []base.Volume{},
		Filesystems:  []base.Filesystem{},
	})
	c.Check(statuses[0].Error, qt.IsNil)
}

func TestIdentityProviderURL(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupJimmWithControllers(c)

	conn := s.Open(c, nil, "bob@canonical.com", nil)
	defer conn.Close()

	client := controllerapi.NewClient(conn)
	res, err := client.IdentityProviderURL(t.Context())
	c.Assert(err, qt.IsNil)
	c.Assert(res, qt.Matches, ``)
}

func TestControllerVersion(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupJimmWithControllers(c)

	conn := s.Open(c, nil, "test@canonical.com", nil)
	defer conn.Close()

	client := controllerapi.NewClient(conn)
	result, err := client.ControllerVersion(t.Context())
	c.Assert(err, qt.IsNil)
	c.Assert(result.GitCommit, qt.Equals, jimmversion.VersionInfo.GitCommit)
	c.Assert(result.Version, qt.Not(qt.Equals), "0.0.0")
}

func TestControllerAccess(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupJimmWithControllers(c)

	conn := s.Open(c, nil, "alice@canonical.com", nil)
	defer conn.Close()

	client := controllerapi.NewClient(conn)
	access, err := client.GetControllerAccess(t.Context(), "alice@canonical.com")
	c.Assert(err, qt.Equals, nil)
	c.Check(string(access), qt.Equals, "superuser")

	access, err = client.GetControllerAccess(t.Context(), "bob@canonical.com")
	c.Assert(err, qt.Equals, nil)
	c.Check(string(access), qt.Equals, "login")

	conn = s.Open(c, nil, "bob@canonical.com", nil)
	defer conn.Close()

	client = controllerapi.NewClient(conn)
	access, err = client.GetControllerAccess(t.Context(), "bob@canonical.com")
	c.Assert(err, qt.Equals, nil)
	c.Check(string(access), qt.Equals, "login")

	_, err = client.GetControllerAccess(t.Context(), "alice@canonical.com")
	c.Assert(err, qt.ErrorMatches, `unauthorized`)
}

func TestControllerConfig(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupJimmWithControllers(c)

	conn := s.Open(c, nil, "alice@canonical.com", nil)
	defer conn.Close()

	client := controllerapi.NewClient(conn)
	config, err := client.ControllerConfig(t.Context())
	c.Assert(err, qt.Equals, nil)

	c.Assert(config[jujucontroller.ControllerUUIDKey], qt.Equals, s.JIMM.ControllerConfig.ControllerUUID)
	c.Assert(config[jujucontroller.PublicDNSAddress], qt.Equals, s.JIMM.ControllerConfig.PublicDNSName)
	c.Assert(config["ssh-host-key"], qt.Equals, s.JIMM.ControllerConfig.SSHPublicHostKey)
	// the reason we need to cast to float64 is because it is a map[string]interface{} and the json marshaller defaults to float64.
	c.Assert(config[jujucontroller.SSHServerPort], qt.Equals, float64(s.JIMM.ControllerConfig.SSHPort))

	var sshPort jujuparams.SSHControllerSSHPortResult
	err = base.NewFacadeCaller(conn, "Controller").FacadeCall(t.Context(), "SSHServerPort", nil, &sshPort)
	c.Assert(err, qt.Equals, nil)
	c.Assert(sshPort.Error == nil, qt.IsTrue)
	c.Assert(sshPort.Port, qt.Equals, s.JIMM.ControllerConfig.SSHPort)
}

func TestWatchModelSummaries(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupJimmWithControllers(c)
	model := s.CreateModelForBob(c)
	model3 := s.CreateModelForCharlieWithBobReadAccess(c)

	done := s.JIMM.Pubsub.Publish(model.UUID.String, jujuparams.ModelAbstract{
		UUID:  model.UUID.String,
		Cloud: "test-cloud",
		Name:  "test-name-1",
	})
	select {
	case <-done:
	case <-time.After(time.Second):
		c.Fatalf("timed out")
	}
	done = s.JIMM.Pubsub.Publish(model3.UUID.String, jujuparams.ModelAbstract{
		UUID:  model3.UUID.String,
		Cloud: "test-cloud",
		Name:  "test-name-3",
	})
	select {
	case <-done:
	case <-time.After(time.Second):
		c.Fatalf("timed out")
	}

	expectedModels := []jujuparams.ModelAbstract{{
		UUID:  model.UUID.String,
		Cloud: "test-cloud",
		Name:  "test-name-1",
	}, {
		UUID:  model3.UUID.String,
		Cloud: "test-cloud",
		Name:  "test-name-3",
	}}
	sort.Slice(expectedModels, func(i, j int) bool {
		return expectedModels[i].UUID < expectedModels[j].UUID
	})

	conn := s.Open(c, nil, "bob@canonical.com", nil)
	defer conn.Close()

	client := controllerapi.NewClient(conn)
	w, err := client.WatchModelSummaries(t.Context())
	c.Assert(err, qt.IsNil)

	models, err := w.Next(t.Context())
	c.Assert(err, qt.IsNil)
	c.Assert(models, qt.DeepEquals, expectedModels)

	err = w.Stop(t.Context())
	c.Assert(err, qt.IsNil)
}

func TestWatchAllModelSummaries(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupJimmWithControllers(c)
	model := s.CreateModelForBob(c)
	model3 := s.CreateModelForCharlieWithBobReadAccess(c)

	done := s.JIMM.Pubsub.Publish(model.UUID.String, jujuparams.ModelAbstract{
		UUID:  model.UUID.String,
		Cloud: "test-cloud",
		Name:  "test-name-1",
	})
	select {
	case <-done:
	case <-time.After(time.Second):
		c.Fatalf("timed out")
	}
	done = s.JIMM.Pubsub.Publish(model3.UUID.String, jujuparams.ModelAbstract{
		UUID:  model3.UUID.String,
		Cloud: "test-cloud",
		Name:  "test-name-3",
	})
	select {
	case <-done:
	case <-time.After(time.Second):
		c.Fatalf("timed out")
	}

	expectedModels := []jujuparams.ModelAbstract{{
		UUID:  model.UUID.String,
		Cloud: "test-cloud",
		Name:  "test-name-1",
	}, {
		UUID:  model3.UUID.String,
		Cloud: "test-cloud",
		Name:  "test-name-3",
	}}
	sort.Slice(expectedModels, func(i, j int) bool {
		return expectedModels[i].UUID < expectedModels[j].UUID
	})

	conn := s.Open(c, nil, "alice@canonical.com", nil)
	defer conn.Close()

	client := controllerapi.NewClient(conn)
	w, err := client.WatchAllModelSummaries(t.Context())
	c.Assert(err, qt.IsNil)

	models, err := w.Next(t.Context())
	c.Assert(err, qt.IsNil)
	c.Assert(models, qt.DeepEquals, expectedModels)

	err = w.Stop(t.Context())
	c.Assert(err, qt.IsNil)
}
