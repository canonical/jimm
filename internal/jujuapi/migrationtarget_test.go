// Copyright 2025 Canonical.

package jujuapi_test

import (
	"github.com/juju/description/v8"
	"github.com/juju/juju/api/controller/migrationtarget"
	"github.com/juju/juju/core/migration"
	"github.com/juju/names/v5"
	"github.com/juju/version/v2"
	gc "gopkg.in/check.v1"

	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
	"github.com/canonical/jimm/v3/pkg/api"
	"github.com/canonical/jimm/v3/pkg/api/params"
)

type migrationTargetSuite struct {
	websocketSuite
}

var _ = gc.Suite(&migrationTargetSuite{})

func (s *migrationTargetSuite) TestAbort(c *gc.C) {
	conn := s.open(c, nil, "alice")
	defer conn.Close()

	modelUUID := "00000001-0000-0000-0000-000000000001"
	client := migrationtarget.NewClient(conn)
	err := client.Abort(modelUUID)
	c.Assert(err, gc.ErrorMatches, `.*model migration not found`)
}

func (s *migrationTargetSuite) TestPrechecks(c *gc.C) {
	conn := s.open(c, nil, "alice")
	defer conn.Close()

	modelDescriptionArgs := description.ModelArgs{
		Type:        description.IAAS,
		Owner:       names.NewUserTag("alice"),
		Cloud:       jimmtest.TestCloudName,
		CloudRegion: jimmtest.TestCloudRegionName,
	}
	modelUUID := "00000001-0000-0000-0000-000000000001"
	modelDescription := description.NewModel(modelDescriptionArgs)
	model := migration.ModelInfo{
		UUID:                   modelUUID,
		Owner:                  names.NewUserTag("alice"),
		Name:                   "test-model",
		ControllerAgentVersion: version.MustParse("3.5.0"),
		AgentVersion:           version.MustParse("3.5.0"),
		ModelDescription:       modelDescription,
	}
	client := migrationtarget.NewClient(conn)
	err := client.Prechecks(model)
	c.Assert(err, gc.ErrorMatches, `.*model migration not found`)

	prepareModelMigration := params.PrepareModelMigrationRequest{
		ModelTag:             names.NewModelTag(modelUUID).String(),
		TargetControllerName: "controller-1", // Default name of the initial controller added to JIMM.
		UserMapping:          map[string]string{"alice": "alice@canonical.com"},
	}
	jimmClient := api.NewClient(conn)
	err = jimmClient.PrepareModelMigration(&prepareModelMigration)
	c.Assert(err, gc.IsNil)

	err = client.Prechecks(model)
	c.Assert(err, gc.IsNil)
}

func (s *migrationTargetSuite) TestCACert(c *gc.C) {
	conn := s.open(c, nil, "alice")
	defer conn.Close()

	client := migrationtarget.NewClient(conn)
	cert, err := client.CACert()
	c.Assert(err, gc.IsNil)
	c.Assert(cert, gc.Equals, "")
}

func (s *migrationTargetSuite) TestAdoptResources(c *gc.C) {
	conn := s.open(c, nil, "alice")
	defer conn.Close()

	modelUUID := "00000001-0000-0000-0000-000000000001"
	client := migrationtarget.NewClient(conn)
	err := client.AdoptResources(modelUUID)
	c.Assert(err, gc.ErrorMatches, `.*model migration not found`)
}
