// Copyright 2026 Canonical Ltd.

package testing

import (
	"bytes"
	"context"
	"net/url"
	"strings"

	gc "gopkg.in/check.v1"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/openfga"
	ofganames "github.com/canonical/jimm/v3/internal/openfga/names"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
	"github.com/juju/errors"
	"github.com/juju/juju/api"
	"github.com/juju/juju/api/client/client"
	"github.com/juju/names/v5"
)

type apiProxySuite struct {
	jimmtest.WebsocketE2ESuite
}

var _ = gc.Suite(&apiProxySuite{})

func (s *apiProxySuite) openCustomLoginProvider(c *gc.C, info *api.Info, username string, lp api.LoginProvider) (api.Connection, error) {
	ld := jimmtest.LoginDetails{Info: info, Username: username, Lp: lp}
	return s.OpenNoAssert(c, ld, nil)
}

func (s *apiProxySuite) TestConnectToModel(c *gc.C) {
	conn := s.Open(c, &api.Info{
		ModelTag:  s.Model.ResourceTag(),
		SkipLogin: true,
	}, "test", nil)
	defer conn.Close()
	var resp map[string]interface{}
	err := conn.APICall("Admin", 3, "", "TestMethod", nil, &resp)
	c.Assert(err, gc.ErrorMatches, `(?s).*no such request - method Admin.TestMethod is not implemented \(not implemented\).*`)
}

// TestSessionTokenLoginProvider verifies that the session token login provider works as expected.
// We do this by using a mock authenticator that simulates polling an OIDC server and verifying that
// the user would be prompted with a login URL and fake the user login via the `EnableDeviceFlow` method.
func (s *apiProxySuite) TestSessionTokenLoginProvider(c *gc.C) {
	ctx := context.Background()
	alice := names.NewUserTag("alice@canonical.com")
	aliceUser := openfga.NewUser(&dbmodel.Identity{Name: alice.Id()}, s.JIMM.OpenFGAClient)
	err := aliceUser.SetControllerAccess(ctx, s.Model.Controller.ResourceTag(), ofganames.AdministratorRelation)
	c.Assert(err, gc.IsNil)
	var output bytes.Buffer
	s.EnableDeviceFlow(aliceUser.Name)
	conn, err := s.openCustomLoginProvider(c, &api.Info{
		ModelTag:  s.Model.ResourceTag(),
		SkipLogin: false,
	}, "alice", api.NewSessionTokenLoginProvider("", &output, func(s string) {}))
	c.Assert(err, gc.IsNil)
	defer conn.Close()
	c.Check(err, gc.Equals, nil)
	outputNoNewLine := strings.ReplaceAll(output.String(), "\n", "")
	c.Check(outputNoNewLine, gc.Matches, `Please visit .* and enter code.*`)
}

// TestAgentLoginReturnsRedirect verifies that the agent login returns a redirect error
// when trying to connect to the model proxy. We don't use the helper methods
// for opening the connection because we want to test authenticating as an agent.
func (s *apiProxySuite) TestAgentLoginReturnsRedirect(c *gc.C) {
	u, err := url.Parse(s.HTTP.URL)
	c.Assert(err, gc.Equals, nil)

	info := api.Info{
		Tag:      names.NewUnitTag("ubuntu/1"),
		ModelTag: s.Model.ResourceTag(),
		Addrs:    []string{u.Host},
	}
	dialOpts := api.DialOpts{
		InsecureSkipVerify: true,
	}
	_, err = api.Open(&info, dialOpts)
	c.Assert(err, gc.NotNil)
	redirectErr, ok := errors.Cause(err).(*api.RedirectError)
	c.Check(ok, gc.Equals, true)
	c.Assert(redirectErr.Servers, gc.HasLen, 1)
	servers := redirectErr.Servers[0].HostPorts().Strings()
	c.Assert(servers, gc.Not(gc.HasLen), 0)
	c.Check(servers[0], gc.Not(gc.Equals), "")
	c.Check(len(redirectErr.CACert), gc.Not(gc.Equals), 0)
}

func (s *apiProxySuite) TestAgentLoginModelDoesNotExist(c *gc.C) {
	u, err := url.Parse(s.HTTP.URL)
	c.Assert(err, gc.Equals, nil)

	info := api.Info{
		Tag:      names.NewUnitTag("ubuntu/1"),
		ModelTag: names.NewModelTag("00000000-0000-0000-0000-000000000000"),
		Addrs:    []string{u.Host},
	}
	dialOpts := api.DialOpts{
		InsecureSkipVerify: true,
	}
	_, err = api.Open(&info, dialOpts)
	c.Assert(err, gc.NotNil)
	c.Check(err.Error(), gc.Matches, `failed to find model: model not found.*`)
}

type logger struct{}

func (l logger) Errorf(string, ...interface{}) {}

func (s *apiProxySuite) TestModelStatus(c *gc.C) {
	conn := s.Open(c, &api.Info{
		ModelTag:  s.Model.ResourceTag(),
		SkipLogin: false,
	}, "alice@canonical.com", nil)
	defer conn.Close()
	jujuClient := client.NewClient(conn, logger{})
	status, err := jujuClient.Status(nil)
	c.Check(err, gc.IsNil)
	c.Check(status, gc.Not(gc.IsNil))
	c.Check(status.Model.Name, gc.Equals, s.Model.Name)
}

func (s *apiProxySuite) TestModelStatusWithoutPermission(c *gc.C) {
	fooUser := openfga.NewUser(&dbmodel.Identity{Name: "foo@canonical.com"}, s.JIMM.OpenFGAClient)
	var output bytes.Buffer
	s.EnableDeviceFlow(fooUser.Name)
	conn, err := s.openCustomLoginProvider(c, &api.Info{
		ModelTag:  s.Model.ResourceTag(),
		SkipLogin: false,
	}, "foo", api.NewSessionTokenLoginProvider("", &output, func(s string) {}))
	c.Check(err, gc.ErrorMatches, "permission denied .*")
	if conn != nil {
		defer conn.Close()
	}
	outputNoNewLine := strings.ReplaceAll(output.String(), "\n", "")
	c.Check(outputNoNewLine, gc.Matches, `Please visit .* and enter code.*`)
}
