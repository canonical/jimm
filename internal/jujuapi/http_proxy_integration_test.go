// Copyright 2024 Canonical.

package jujuapi_test

import (
	"context"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"

	"github.com/juju/charm/v12"
	"github.com/juju/charm/v12/resource"
	"github.com/juju/juju/api"
	"github.com/juju/juju/api/client/charms"
	"github.com/juju/juju/api/client/resources"
	"github.com/juju/juju/core/network"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/juju/testcharms"
	"github.com/juju/juju/testing/factory"
	"github.com/juju/names/v5"
	"github.com/juju/version/v2"
	gc "gopkg.in/check.v1"

	"github.com/canonical/jimm/v3/internal/jujuapi"
)

type httpProxySuite struct {
	websocketSuite
}

var _ = gc.Suite(&httpProxySuite{})

func (s *httpProxySuite) TestHTTPAuthenticate(c *gc.C) {
	ctx := context.Background()
	httpProxier := jujuapi.HTTPProxier(s.JIMM)

	// good
	req, err := http.NewRequest("POST", fmt.Sprintf("/%s/charms", s.Model.UUID.String), nil)
	c.Assert(err, gc.IsNil)
	token, err := s.JIMM.OAuthAuthenticator.MintSessionToken(s.AdminUser.Name)
	c.Assert(err, gc.IsNil)
	req.SetBasicAuth("", token)
	c.Assert(err, gc.IsNil)
	err = httpProxier.Authenticate(ctx, nil, req)
	c.Assert(err, gc.IsNil)

	// missing auth
	req, err = http.NewRequest("POST", fmt.Sprintf("/%s/charms", s.Model.UUID.String), nil)
	c.Assert(err, gc.IsNil)
	err = httpProxier.Authenticate(ctx, nil, req)
	c.Assert(err, gc.ErrorMatches, "authentication missing")

	// wrong user
	req, err = http.NewRequest("POST", fmt.Sprintf("/%s/charms", s.Model.UUID.String), nil)
	c.Assert(err, gc.IsNil)
	token, err = s.JIMM.OAuthAuthenticator.MintSessionToken("test-user")
	c.Assert(err, gc.IsNil)
	req.SetBasicAuth("", token)
	c.Assert(err, gc.IsNil)
	err = httpProxier.Authenticate(ctx, nil, req)
	c.Assert(err, gc.ErrorMatches, "unauthorized")
}

func (s *httpProxySuite) TestCharmHTTPServe(c *gc.C) {
	ctx := context.Background()
	httpProxier := jujuapi.HTTPProxier(s.JIMM)
	expectU, expectP, err := s.JIMM.GetCredentialStore().GetControllerCredentials(ctx, s.Model.Controller.Name)
	c.Assert(err, gc.IsNil)
	// we expect the controller to respond with TLS
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, p, _ := r.BasicAuth()
		c.Assert(u, gc.Equals, names.NewUserTag(expectU).String())
		c.Assert(p, gc.Equals, expectP)
		w.Write([]byte("OK"))
	}))
	defer ts.Close()
	controller := s.Model.Controller
	pemData := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: ts.Certificate().Raw,
	})
	controller.CACertificate = string(pemData)

	tests := []struct {
		description    string
		setup          func()
		url            string
		statusExpected int
	}{
		{
			description: "good",
			setup: func() {
				newURL, _ := url.Parse(ts.URL)
				controller.PublicAddress = newURL.Host
				err = s.JIMM.Database.UpdateController(ctx, &controller)
				c.Assert(err, gc.IsNil)
			},
			url:            fmt.Sprintf("/model/%s/charms", s.Model.UUID.String),
			statusExpected: http.StatusOK,
		},
		{
			description: "controller no public address, only addresses",
			setup: func() {
				hp, err := network.ParseMachineHostPort(ts.Listener.Addr().String())
				c.Assert(err, gc.Equals, nil)
				controller.Addresses = append(make([][]jujuparams.HostPort, 0), []jujuparams.HostPort{{
					Address: jujuparams.FromMachineAddress(hp.MachineAddress),
					Port:    hp.Port(),
				}})
				controller.Addresses = append(controller.Addresses, []jujuparams.HostPort{})
				controller.PublicAddress = ""
				err = s.JIMM.Database.UpdateController(ctx, &controller)
				c.Assert(err, gc.IsNil)
			},
			url:            fmt.Sprintf("/model/%s/charms", s.Model.UUID.String),
			statusExpected: http.StatusOK,
		},
		{
			description: "controller no public address, only addresses",
			setup: func() {
				hp, err := network.ParseMachineHostPort(ts.Listener.Addr().String())
				c.Assert(err, gc.Equals, nil)
				controller.Addresses = append(make([][]jujuparams.HostPort, 0), []jujuparams.HostPort{{
					Address: jujuparams.FromMachineAddress(hp.MachineAddress),
					Port:    hp.Port(),
				}})
				controller.Addresses = append(controller.Addresses, []jujuparams.HostPort{})
				controller.PublicAddress = ""
				err = s.JIMM.Database.UpdateController(ctx, &controller)
				c.Assert(err, gc.IsNil)
			},
			url:            fmt.Sprintf("/model/%s/charms", s.Model.UUID.String),
			statusExpected: http.StatusOK,
		},
		{
			description: "model not existing",
			setup: func() {
			},
			url:            fmt.Sprintf("/model/%s/charms", "54d9f921-c45a-4825-8253-74e7edc28066"),
			statusExpected: http.StatusNotFound,
		},
		{
			description: "controller not reachable",
			setup: func() {
				controller.Addresses = nil
				controller.PublicAddress = "localhost-not-found:61213"
				err = s.JIMM.Database.UpdateController(ctx, &controller)
				c.Assert(err, gc.IsNil)
			},
			url:            fmt.Sprintf("/model/%s/charms", s.Model.UUID.String),
			statusExpected: http.StatusInternalServerError,
		},
	}

	for _, test := range tests {
		test.setup()
		req, err := http.NewRequest("POST", test.url, nil)
		c.Assert(err, gc.IsNil)
		recorder := httptest.NewRecorder()
		httpProxier.ServeHTTP(ctx, recorder, req)
		c.Assert(recorder.Result().StatusCode, gc.Equals, test.statusExpected)
	}
}

type e2eProxySuite struct {
	websocketSuite
}

var _ = gc.Suite(&e2eProxySuite{})

func (s *e2eProxySuite) TestLocalCharmDeploy(c *gc.C) {
	conn := s.open(c, &api.Info{
		ModelTag:  s.Model.ResourceTag(),
		SkipLogin: false,
	}, s.AdminUser.Name)

	client, err := charms.NewLocalCharmClient(conn)
	c.Assert(err, gc.IsNil)
	charmArchive := testcharms.Repo.CharmArchive(c.MkDir(), "dummy")
	curl := charm.MustParseURL(
		fmt.Sprintf("local:quantal/%s-%d", charmArchive.Meta().Name, charmArchive.Revision()),
	)
	vers := version.MustParse("2.6.6")
	url, err := client.AddLocalCharm(curl, charmArchive, false, vers)
	c.Assert(err, gc.IsNil)
	c.Assert(url.String(), gc.Equals, curl.String())

}

func (s *e2eProxySuite) TestResourceEndpoint(c *gc.C) {
	// setup: to upload resource we first need to create the application and the pending resource
	modelState, err := s.StatePool.Get(s.Model.UUID.String)
	c.Assert(err, gc.Equals, nil)
	defer modelState.Release()
	f := factory.NewFactory(modelState.State, s.StatePool)
	charmArchive := testcharms.Repo.CharmArchive(c.MkDir(), "dummy")
	curl := charm.MustParseURL(
		fmt.Sprintf("local:quantal/%s-%d", charmArchive.Meta().Name, charmArchive.Revision()),
	)
	app := f.MakeApplication(c, &factory.ApplicationParams{
		Name: "test-app",
		Charm: f.MakeCharm(c, &factory.CharmParams{
			Name: charmArchive.Meta().Name,
			URL:  curl.String(),
		}),
	})
	pendingId, err := modelState.Resources().AddPendingResource(app.Name(), s.Model.OwnerIdentityName, resource.Resource{
		Meta:   resource.Meta{Name: "test", Type: 1, Path: "file"},
		Origin: resource.OriginStore,
	})
	c.Assert(err, gc.Equals, nil)
	conn := s.open(c, &api.Info{
		ModelTag:  s.Model.ResourceTag(),
		SkipLogin: false,
	}, s.AdminUser.Name)
	uploadClient, err := resources.NewClient(conn)
	c.Assert(err, gc.IsNil)

	// test
	err = uploadClient.Upload(app.Name(), "test", "file", pendingId, strings.NewReader("<data>"))
	c.Assert(err, gc.IsNil)
}
