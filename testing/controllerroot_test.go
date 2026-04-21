// Copyright 2026 Canonical.

package testing

import (
	"testing"

	qt "github.com/frankban/quicktest"
	"github.com/juju/juju/api"
	"github.com/juju/juju/core/semversion"
	jujuparams "github.com/juju/juju/rpc/params"

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
	c.Assert(v, qt.DeepEquals, semversion.MustParse("1.2.3"))
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
	err := conn.APICall(t.Context(), "Admin", 3, "", "Logout", nil, &resp)
	// This regex matches both the 3.6 and 4.0 error messages, which have
	// slightly different wording but the same meaning (Admin.Logout vs Logout at version 0 for facade type "Admin").
	c.Assert(err, qt.ErrorMatches, `(?s).*(?:Admin\.Logout|Logout.*Admin).*\(not implemented\).*`)
}

func TestUnimplementedRootFails(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupJimmWithControllers(c)

	conn := s.Open(c, nil, "test", nil)
	defer conn.Close()
	var resp jujuparams.RedirectInfoResult
	err := conn.APICall(t.Context(), "NoSuch", 1, "", "Method", nil, &resp)
	c.Assert(err, qt.ErrorMatches, `(?s).*unknown method "Method" at version 1 for facade type "NoSuch" \(not implemented\).*`)
}
