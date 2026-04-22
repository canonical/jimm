// Copyright 2026 Canonical.

package testing

import (
	"fmt"
	"strings"
	"testing"

	qt "github.com/frankban/quicktest"
	"github.com/juju/juju/api"
	"github.com/juju/juju/api/client/charms"
	"github.com/juju/juju/api/client/resources"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/domain/deployment/charm"
	"github.com/juju/juju/domain/deployment/charm/resource"
	"github.com/juju/juju/testcharms"

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
		// Revision only need be sent for pre-4.0.
		fmt.Sprintf("local:quantal/%s-%d", charmArchive.Meta().Name, charmArchive.Revision()),
	)

	vers := semversion.MustParse("2.6.6")

	url, err := client.AddLocalCharm(curl, charmArchive, false, vers)
	c.Assert(err, qt.IsNil)

	ctrlVers, _ := conn.ServerVersion()

	if ctrlVers.Compare(semversion.MustParse("4.0.0")) == -1 {
		c.Assert(url.String(), qt.Equals, curl.String())
	} else {
		// In 4.0, it is auto-incremented for us from 0.
		c.Assert(url.String(), qt.Equals, "local:dummy-0")
		// As such, we'll upload it twice and verify it is incremented accordingly.
		url, err := client.AddLocalCharm(curl, charmArchive, false, vers)
		c.Assert(err, qt.IsNil)
		c.Assert(url.String(), qt.Equals, "local:dummy-1")
	}
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

	charmArchive := testcharms.Repo.CharmArchive(c.TempDir(), "starsay")
	curl := charm.MustParseURL(
		fmt.Sprintf("local:quantal/%s-%d", charmArchive.Meta().Name, charmArchive.Revision()),
	)

	url, err := charmClient.AddLocalCharm(curl, charmArchive, false, semversion.MustParse("2.6.6"))
	c.Assert(err, qt.IsNil)

	appName := "test-app"

	uploadClient, err := resources.NewClient(conn)
	c.Assert(err, qt.IsNil)

	pendingIDs, err := uploadClient.AddPendingResources(t.Context(), resources.AddPendingResourcesArgs{
		ApplicationID: appName,
		CharmID:       resources.CharmID{URL: url.String()},
		Resources: []resource.Resource{
			{
				Meta:   resource.Meta{Name: "store-resource", Type: 1, Path: "filename.tgz"},
				Origin: resource.OriginStore,
			},
		},
	})
	c.Assert(err, qt.IsNil)
	c.Assert(pendingIDs, qt.HasLen, 1)
	pendingId := pendingIDs[0]

	// act
	err = uploadClient.Upload(t.Context(), appName, "store-resource", "filename.tgz", pendingId, strings.NewReader("<data>"))
	c.Assert(err, qt.IsNil)
}
