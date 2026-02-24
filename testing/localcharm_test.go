// Copyright 2026 Canonical.

package testing

import (
	"fmt"
	"strings"
	"testing"

	qt "github.com/frankban/quicktest"
	"github.com/juju/charm/v12"
	"github.com/juju/charm/v12/resource"
	"github.com/juju/juju/api"
	"github.com/juju/juju/api/client/charms"
	"github.com/juju/juju/api/client/resources"
	"github.com/juju/juju/testcharms"
	"github.com/juju/version/v2"

	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
)

func TestLocalCharmDeploy(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupJimmWithControllers(c)
	model := s.CreateModelForBob(c)

	conn := s.Open(c, &api.Info{
		ModelTag:  model.ResourceTag(),
		SkipLogin: false,
	}, s.AdminUser.Name, nil)

	client, err := charms.NewLocalCharmClient(conn)
	c.Assert(err, qt.IsNil)
	charmArchive := testcharms.Repo.CharmArchive(c.TempDir(), "dummy")
	curl := charm.MustParseURL(
		fmt.Sprintf("local:quantal/%s-%d", charmArchive.Meta().Name, charmArchive.Revision()),
	)
	vers := version.MustParse("2.6.6")
	url, err := client.AddLocalCharm(curl, charmArchive, false, vers)
	c.Assert(err, qt.IsNil)
	c.Assert(url.String(), qt.Equals, curl.String())
}

func TestResourceEndpoint(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupJimmWithControllers(c)
	model := s.CreateModelForBob(c)

	// setup: app and pending resource
	conn := s.Open(c, &api.Info{
		ModelTag:  model.ResourceTag(),
		SkipLogin: false,
	}, s.AdminUser.Name, nil)

	charmClient, err := charms.NewLocalCharmClient(conn)
	c.Assert(err, qt.IsNil)

	charmArchive := testcharms.Repo.CharmArchive(c.TempDir(), "dummy")
	curl := charm.MustParseURL(
		fmt.Sprintf("local:quantal/%s-%d", charmArchive.Meta().Name, charmArchive.Revision()),
	)
	url, err := charmClient.AddLocalCharm(curl, charmArchive, false, version.MustParse("2.6.6"))
	c.Assert(err, qt.IsNil)

	appName := "test-app"

	uploadClient, err := resources.NewClient(conn)
	c.Assert(err, qt.IsNil)

	pendingIDs, err := uploadClient.AddPendingResources(resources.AddPendingResourcesArgs{
		ApplicationID: appName,
		CharmID:       resources.CharmID{URL: url.String()},
		Resources: []resource.Resource{
			{
				Meta:   resource.Meta{Name: "test", Type: 1, Path: "file"},
				Origin: resource.OriginStore,
			},
		},
	})
	c.Assert(err, qt.IsNil)
	c.Assert(pendingIDs, qt.HasLen, 1)
	pendingId := pendingIDs[0]

	// act
	err = uploadClient.Upload(appName, "test", "file", pendingId, strings.NewReader("<data>"))
	c.Assert(err, qt.IsNil)
}
