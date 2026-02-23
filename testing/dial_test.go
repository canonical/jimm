// Copyright 2025 Canonical.

package testing

import (
	"context"
	"fmt"

	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	gc "gopkg.in/check.v1"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/jujuclient"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
)

type dialSuite struct {
	jimmtest.WebsocketE2ESuite
}

var _ = gc.Suite(&dialSuite{})

func (s *dialSuite) TestDial(c *gc.C) {
	name, config := s.GetOneControllerConfig(c)
	ctl := dbmodel.Controller{
		UUID:          config.UUID,
		Name:          name,
		CACertificate: config.CACert,
		PublicAddress: config.Addrs[0],
		TLSHostname:   "juju-apiserver",
	}

	api, err := s.JIMM.Dialer.Dial(context.Background(), &ctl, names.ModelTag{}, nil, nil)
	c.Assert(err, gc.Equals, nil)
	defer api.Close()

	c.Check(ctl.UUID, gc.Equals, config.UUID)
	ctrlVersion := version.MustParse(ctl.AgentVersion)
	minVersion := version.MustParse("3.6.13")
	c.Check(ctrlVersion.Compare(minVersion), gc.Equals, 1)

	addrs := make([]string, len(ctl.Addresses))
	for i, addr := range ctl.Addresses {
		addrs[i] = fmt.Sprintf("%s:%d", addr[0].Value, addr[0].Port)
	}
	c.Check(addrs, jc.SameContents, config.Addrs)
}

func (s *dialSuite) TestDialWithJWT(c *gc.C) {
	ctx := context.Background()

	name, config := s.GetOneControllerConfig(c)
	ctl := dbmodel.Controller{
		UUID:          config.UUID,
		Name:          name,
		CACertificate: config.CACert,
		PublicAddress: config.Addrs[0],
		TLSHostname:   "juju-apiserver",
	}

	dialer := &jujuclient.Dialer{
		JWTService: s.JIMM.JWTService,
	}

	// Check dial is OK
	api, err := dialer.Dial(ctx, &ctl, names.ModelTag{}, nil, nil)
	c.Assert(err, gc.Equals, nil)
	defer api.Close()

	c.Check(ctl.UUID, gc.Equals, config.UUID)
	ctrlVersion := version.MustParse(ctl.AgentVersion)
	minVersion := version.MustParse("3.6.13")
	c.Check(ctrlVersion.Compare(minVersion), gc.Equals, 1)

	addrs := make([]string, len(ctl.Addresses))
	for i, addr := range ctl.Addresses {
		addrs[i] = fmt.Sprintf("%s:%d", addr[0].Value, addr[0].Port)
	}
	c.Check(addrs, jc.SameContents, config.Addrs)
}

// TestConnectStreams tests the ConnectStream and ConnectControllerStream methods
// of our Juju dialer. It verifies that we can connect to valid endpoints
// on a Juju controller, and that invalid endpoints return errors.
func (s *dialSuite) TestConnectStreams(c *gc.C) {
	name, config := s.GetOneControllerConfig(c)
	ctl := dbmodel.Controller{
		UUID:          config.UUID,
		Name:          name,
		CACertificate: config.CACert,
		PublicAddress: config.Addrs[0],
		TLSHostname:   "juju-apiserver",
	}

	api, err := s.JIMM.Dialer.Dial(context.Background(), &ctl, s.Model.ResourceTag(), nil, nil)
	c.Assert(err, gc.Equals, nil)
	defer api.Close()

	// Connect to the model stream for a valid endpoint
	modelStream, err := api.ConnectStream("/log", nil)
	c.Assert(err, gc.IsNil)
	defer modelStream.Close()

	// Connect to the model stream for an invalid endpoint
	_, err = api.ConnectStream("/log2", nil)
	c.Assert(err, gc.NotNil)

	// Connect to the controller stream for a valid endpoint
	controllerStream, err := api.ConnectControllerStream("/migrate/logtransfer", nil, nil)
	c.Assert(err, gc.IsNil)
	defer controllerStream.Close()

	// Connect to the controller stream for an invalid endpoint
	_, err = api.ConnectControllerStream("/migrate/logtransfer2", nil, nil)
	c.Assert(err, gc.NotNil)
}
