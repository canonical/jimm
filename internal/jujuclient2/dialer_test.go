// Copyright 2024 Canonical.
package jujuclient2_test

import (
	"testing"

	jujutesting "github.com/juju/juju/testing"
	"github.com/juju/names/v5"
	gc "gopkg.in/check.v1"

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

func (s *jujuDialerSuite) getDialParams(c *gc.C) jujuclient2.DialParams {
	info := s.APIInfo(c)
	return jujuclient2.DialParams{
		ControllerTag: names.NewControllerTag(info.ControllerUUID),
		Addresses:     info.Addrs,
		CACertificate: info.CACert,
	}
}

func (s *jujuDialerSuite) TestJujuDialer(c *gc.C) {
	dialer := jujuclient2.JujuDialer{
		JWTService: s.JIMM.JWTService,
	}

	p := s.getDialParams(c)

	conn, err := dialer.Dial(p)
	c.Assert(err, gc.IsNil)
	defer conn.Close()
}
