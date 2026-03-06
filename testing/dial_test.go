// Copyright 2025 Canonical.

package testing

import (
	"context"
	"fmt"
	"testing"

	qt "github.com/frankban/quicktest"
	"github.com/google/uuid"
	"github.com/juju/names/v5"
	"github.com/juju/version/v2"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/jujuclient"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
)

func TestDial(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupJimmWithControllers(c)

	name, config := s.GetOneControllerConfig(c)
	ctl := dbmodel.Controller{
		UUID:          config.UUID,
		Name:          name,
		CACertificate: config.CACert,
		PublicAddress: config.Addrs[0],
		TLSHostname:   "juju-apiserver",
	}

	api, err := s.JIMM.Dialer.Dial(context.Background(), &ctl, names.ModelTag{}, nil, nil)
	c.Assert(err, qt.Equals, nil)
	defer api.Close()

	c.Check(ctl.UUID, qt.Equals, config.UUID)
	ctrlVersion := version.MustParse(ctl.AgentVersion)
	minVersion := version.MustParse("3.6.13")
	c.Check(ctrlVersion.Compare(minVersion), qt.Equals, 1)

	addrs := make([]string, len(ctl.Addresses))
	for i, addr := range ctl.Addresses {
		addrs[i] = fmt.Sprintf("%s:%d", addr[0].Value, addr[0].Port)
	}
	c.Check(addrs, qt.ContentEquals, config.Addrs)
}

func TestDialWithJWT(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupJimmWithControllers(c)
	model := s.CreateModelForBob(c)

	ctx := context.Background()

	config := s.GetControllerConfig(c, model.Controller.Name)
	ctl := dbmodel.Controller{
		UUID:          config.UUID,
		Name:          model.Controller.Name,
		CACertificate: config.CACert,
		PublicAddress: config.Addrs[0],
		TLSHostname:   "juju-apiserver",
	}

	dialer := &jujuclient.Dialer{
		JWTService: s.JIMM.JWTService,
	}

	// Check dial is OK
	api, err := dialer.Dial(ctx, &ctl, names.ModelTag{}, nil, nil)
	c.Assert(err, qt.Equals, nil)
	defer api.Close()

	c.Check(ctl.UUID, qt.Equals, config.UUID)
	ctrlVersion := version.MustParse(ctl.AgentVersion)
	minVersion := version.MustParse("3.6.13")
	c.Check(ctrlVersion.Compare(minVersion), qt.Equals, 1)

	addrs := make([]string, len(ctl.Addresses))
	for i, addr := range ctl.Addresses {
		addrs[i] = fmt.Sprintf("%s:%d", addr[0].Value, addr[0].Port)
	}
	c.Check(addrs, qt.ContentEquals, config.Addrs)
}

func TestDialModelStatusMissingModel(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupJimmWithControllers(c)

	name, config := s.GetOneControllerConfig(c)
	ctl := dbmodel.Controller{
		UUID:          config.UUID,
		Name:          name,
		CACertificate: config.CACert,
		PublicAddress: config.Addrs[0],
		TLSHostname:   "juju-apiserver",
	}

	api, err := s.JIMM.Dialer.Dial(context.Background(), &ctl, names.ModelTag{}, nil, nil)
	c.Assert(err, qt.Equals, nil)
	defer api.Close()

	_, err = api.ModelStatus(context.Background(), names.NewModelTag(uuid.NewString()))
	c.Assert(err, qt.Not(qt.IsNil))
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeNotFound)
	c.Check(err, qt.ErrorMatches, `model .* not found.*`)
}

// TestConnectStreams tests the ConnectStream and ConnectControllerStream methods
// of our Juju dialer. It verifies that we can connect to valid endpoints
// on a Juju controller, and that invalid endpoints return errors.
func TestConnectStreams(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupJimmWithControllers(c)
	model := s.CreateModelForBob(c)

	config := s.GetControllerConfig(c, model.Controller.Name)
	ctl := dbmodel.Controller{
		UUID:          config.UUID,
		Name:          model.Controller.Name,
		CACertificate: config.CACert,
		PublicAddress: config.Addrs[0],
		TLSHostname:   "juju-apiserver",
	}

	api, err := s.JIMM.Dialer.Dial(context.Background(), &ctl, model.ResourceTag(), nil, nil)
	c.Assert(err, qt.Equals, nil)
	defer api.Close()

	// Connect to the model stream for a valid endpoint
	modelStream, err := api.ConnectStream("/log", nil)
	c.Assert(err, qt.IsNil)
	defer modelStream.Close()

	// Connect to the model stream for an invalid endpoint
	_, err = api.ConnectStream("/log2", nil)
	c.Assert(err, qt.Not(qt.IsNil))

	// Connect to the controller stream for a valid endpoint
	controllerStream, err := api.ConnectControllerStream("/migrate/logtransfer", nil, nil)
	c.Assert(err, qt.IsNil)
	defer controllerStream.Close()

	// Connect to the controller stream for an invalid endpoint
	_, err = api.ConnectControllerStream("/migrate/logtransfer2", nil, nil)
	c.Assert(err, qt.Not(qt.IsNil))
}
