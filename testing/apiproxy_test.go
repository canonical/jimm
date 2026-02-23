// Copyright 2026 Canonical.

package testing

import (
	"bytes"
	"context"
	"net/url"
	"strings"
	"testing"

	qt "github.com/frankban/quicktest"
	"github.com/juju/errors"
	"github.com/juju/juju/api"
	"github.com/juju/juju/api/client/client"
	"github.com/juju/names/v5"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/openfga"
	ofganames "github.com/canonical/jimm/v3/internal/openfga/names"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
)

func TestConnectToModel(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupWebsocketEnv(c)

	conn := s.Open(c, &api.Info{
		ModelTag:  s.Model.ResourceTag(),
		SkipLogin: true,
	}, "test", nil)
	defer conn.Close()
	var resp map[string]interface{}
	err := conn.APICall("Admin", 3, "", "TestMethod", nil, &resp)
	c.Assert(err, qt.ErrorMatches, `(?s).*no such request - method Admin.TestMethod is not implemented \(not implemented\).*`)
}

// TestSessionTokenLoginProvider verifies that the session token login provider works as expected.
// We do this by using a mock authenticator that simulates polling an OIDC server and verifying that
// the user would be prompted with a login URL and fake the user login via the `EnableDeviceFlow` method.
func TestSessionTokenLoginProvider(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupWebsocketEnv(c)

	ctx := context.Background()
	alice := names.NewUserTag("alice@canonical.com")
	aliceUser := openfga.NewUser(&dbmodel.Identity{Name: alice.Id()}, s.JIMM.OpenFGAClient)
	err := aliceUser.SetControllerAccess(ctx, s.Model.Controller.ResourceTag(), ofganames.AdministratorRelation)
	c.Assert(err, qt.IsNil)
	var output bytes.Buffer
	s.EnableDeviceFlow(aliceUser.Name)
	conn, err := s.OpenCustomLoginProvider(c, &api.Info{
		ModelTag:  s.Model.ResourceTag(),
		SkipLogin: false,
	}, "alice", api.NewSessionTokenLoginProvider("", &output, func(s string) {}))
	c.Assert(err, qt.IsNil)
	defer conn.Close()
	c.Check(err, qt.Equals, nil)
	outputNoNewLine := strings.ReplaceAll(output.String(), "\n", "")
	c.Check(outputNoNewLine, qt.Matches, `Please visit .* and enter code.*`)
}

// TestAgentLoginReturnsRedirect verifies that the agent login returns a redirect error
// when trying to connect to the model proxy. We don't use the helper methods
// for opening the connection because we want to test authenticating as an agent.
func TestAgentLoginReturnsRedirect(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupWebsocketEnv(c)

	u, err := url.Parse(s.HTTP.URL)
	c.Assert(err, qt.Equals, nil)

	info := api.Info{
		Tag:      names.NewUnitTag("ubuntu/1"),
		ModelTag: s.Model.ResourceTag(),
		Addrs:    []string{u.Host},
	}
	dialOpts := api.DialOpts{
		InsecureSkipVerify: true,
	}
	_, err = api.Open(&info, dialOpts)
	c.Assert(err, qt.Not(qt.IsNil))
	redirectErr, ok := errors.Cause(err).(*api.RedirectError)
	c.Check(ok, qt.Equals, true)
	c.Assert(redirectErr.Servers, qt.HasLen, 1)
	servers := redirectErr.Servers[0].HostPorts().Strings()
	c.Assert(servers, qt.Not(qt.HasLen), 0)
	c.Check(servers[0], qt.Not(qt.Equals), "")
	c.Check(len(redirectErr.CACert), qt.Not(qt.Equals), 0)
}

func TestAgentLoginModelDoesNotExist(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupWebsocketEnv(c)

	u, err := url.Parse(s.HTTP.URL)
	c.Assert(err, qt.Equals, nil)

	info := api.Info{
		Tag:      names.NewUnitTag("ubuntu/1"),
		ModelTag: names.NewModelTag("00000000-0000-0000-0000-000000000000"),
		Addrs:    []string{u.Host},
	}
	dialOpts := api.DialOpts{
		InsecureSkipVerify: true,
	}
	_, err = api.Open(&info, dialOpts)
	c.Assert(err, qt.Not(qt.IsNil))
	c.Check(err.Error(), qt.Matches, `failed to find model: model not found.*`)
}

type logger struct{}

func (l logger) Errorf(string, ...interface{}) {}

func TestProxyModelStatus(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupWebsocketEnv(c)

	conn := s.Open(c, &api.Info{
		ModelTag:  s.Model.ResourceTag(),
		SkipLogin: false,
	}, "alice@canonical.com", nil)
	defer conn.Close()
	jujuClient := client.NewClient(conn, logger{})
	status, err := jujuClient.Status(nil)
	c.Check(err, qt.IsNil)
	c.Check(status, qt.Not(qt.IsNil))
	c.Check(status.Model.Name, qt.Equals, s.Model.Name)
}

func TestProxyModelStatusWithoutPermission(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupWebsocketEnv(c)

	fooUser := openfga.NewUser(&dbmodel.Identity{Name: "foo@canonical.com"}, s.JIMM.OpenFGAClient)
	var output bytes.Buffer
	s.EnableDeviceFlow(fooUser.Name)
	conn, err := s.OpenCustomLoginProvider(c, &api.Info{
		ModelTag:  s.Model.ResourceTag(),
		SkipLogin: false,
	}, "foo", api.NewSessionTokenLoginProvider("", &output, func(s string) {}))
	c.Check(err, qt.ErrorMatches, "permission denied .*")
	if conn != nil {
		defer conn.Close()
	}
	outputNoNewLine := strings.ReplaceAll(output.String(), "\n", "")
	c.Check(outputNoNewLine, qt.Matches, `Please visit .* and enter code.*`)
}
