// Copyright 2026 Canonical.

package testing

import (
	"testing"

	qt "github.com/frankban/quicktest"
	"github.com/juju/juju/api"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/version/v2"

	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
)

func TestServerVersion(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupJimmWithControllers(c)
	model := s.CreateModelForBob(c)

	model.Controller.AgentVersion = "1.2.3"
	err := s.JIMM.Database.UpdateController(c.Context(), &model.Controller)
	c.Assert(err, qt.Equals, nil)

	conn := s.Open(c, nil, "test", nil)
	defer conn.Close()

	v, ok := conn.ServerVersion()
	c.Assert(ok, qt.Equals, true)
	c.Assert(v, qt.DeepEquals, version.MustParse("1.2.3"))
}

func TestUnimplementedMethodFails(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupJimmWithControllers(c)
	model := s.CreateModelForBob(c)

	conn := s.Open(c, &api.Info{
		ModelTag:  model.ResourceTag(),
		SkipLogin: true,
	}, "test", nil)
	defer conn.Close()
	var resp jujuparams.RedirectInfoResult
	err := conn.APICall("Admin", 3, "", "Logout", nil, &resp)
	c.Assert(err, qt.ErrorMatches, `(?s).*no such request - method Admin.Logout is not implemented \(not implemented\).*`)
}

func TestUnimplementedRootFails(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupJimmWithControllers(c)

	conn := s.Open(c, nil, "test", nil)
	defer conn.Close()
	var resp jujuparams.RedirectInfoResult
	err := conn.APICall("NoSuch", 1, "", "Method", nil, &resp)
	c.Assert(err, qt.ErrorMatches, `(?s).*no such request - method NoSuch\(1\).Method is not implemented \(not implemented\).*`)
}
