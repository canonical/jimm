// Copyright 2024 Canonical.

package jujuapi_test

import (
	"fmt"
	"strings"

	"github.com/juju/charm/v12"
	"github.com/juju/charm/v12/resource"
	"github.com/juju/juju/api"
	"github.com/juju/juju/api/client/charms"
	"github.com/juju/juju/api/client/resources"
	"github.com/juju/juju/testcharms"
	"github.com/juju/juju/testing/factory"
	"github.com/juju/version/v2"
	gc "gopkg.in/check.v1"
)

// localCharmSuite tests end-to-end deployment of a local charm.
type localCharmSuite struct {
	websocketSuite
}

var _ = gc.Suite(&localCharmSuite{})

func (s *localCharmSuite) TestLocalCharmDeploy(c *gc.C) {
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

func (s *localCharmSuite) TestResourceEndpoint(c *gc.C) {
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
