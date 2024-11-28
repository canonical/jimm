// Copyright 2024 Canonical.
package jujuclient2_test

import (
	"testing"

	jujutesting "github.com/juju/juju/testing"
	"github.com/juju/names/v5"
	gc "gopkg.in/check.v1"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/jujuclient2"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
)

type jujuDialerSuite struct {
	jimmtest.JujuSuite
}

var _ = gc.Suite(&jujuDialerSuite{})

func TestPackage(t *testing.T) {
	jujutesting.MgoTestPackage(t)
}

func (s *jujuDialerSuite) getControllerToDial(c *gc.C) *dbmodel.Controller {
	info := s.APIInfo(c)
	return &dbmodel.Controller{
		UUID:          info.ControllerUUID,
		Name:          s.ControllerConfig.ControllerName(),
		CACertificate: info.CACert,
		PublicAddress: info.Addrs[0],
	}
}

func (s *jujuDialerSuite) TestJujuDialer(c *gc.C) {
	dialer := jujuclient2.JujuDialer{
		JWTService: s.JIMM.JWTService,
	}

	ctl := s.getControllerToDial(c)

	conn, err := dialer.Dial(ctl, names.ModelTag{}, nil)
	c.Assert(err, gc.IsNil)
	defer conn.Close()
}
