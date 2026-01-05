// Copyright 2025 Canonical.

package testing

import (
	"github.com/juju/description/v9"
	"github.com/juju/juju/api/controller/migrationtarget"
	"github.com/juju/juju/core/migration"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v5"
	"github.com/juju/version/v2"
	gc "gopkg.in/check.v1"

	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
	"github.com/canonical/jimm/v3/pkg/api"
	"github.com/canonical/jimm/v3/pkg/api/params"
)

type migrationTargetSuite struct {
	jimmtest.WebsocketE2ESuite
}

var _ = gc.Suite(&migrationTargetSuite{})

func (s *migrationTargetSuite) TestAbort(c *gc.C) {
	conn := s.Open(c, nil, "alice", nil)
	defer conn.Close()

	modelUUID := "00000001-0000-0000-0000-000000000001"
	client := migrationtarget.NewClient(conn)
	err := client.Abort(modelUUID)
	c.Assert(err, gc.ErrorMatches, `.*model migration not found.*`)
}

func (s *migrationTargetSuite) TestCheckMachines(c *gc.C) {
	conn := s.Open(c, nil, "alice", nil)
	defer conn.Close()

	modelUUID := "00000001-0000-0000-0000-000000000001"
	client := migrationtarget.NewClient(conn)
	_, err := client.CheckMachines(modelUUID)
	c.Assert(err, gc.ErrorMatches, `.*model migration not found.*`)
}

func (s *migrationTargetSuite) TestPrechecks(c *gc.C) {
	conn := s.Open(c, nil, "alice", nil)
	defer conn.Close()

	cct := names.NewCloudCredentialTag(jimmtest.TestE2ECloudName + "/alice@canonical.com/cred")
	s.UpdateCloudCredential(c, cct, jujuparams.CloudCredential{
		AuthType: "empty",
	})

	modelDescriptionArgs := description.ModelArgs{
		Type:        description.IAAS,
		Owner:       names.NewUserTag("alice"),
		Cloud:       jimmtest.TestE2ECloudName,
		CloudRegion: jimmtest.TestE2ECloudRegionName,
	}
	modelUUID := "00000001-0000-0000-0000-000000000001"
	modelDescription := description.NewModel(modelDescriptionArgs)
	modelDescription.SetStatus(description.StatusArgs{Value: "available"})
	modelDescription.SetCloudCredential(description.CloudCredentialArgs{
		Name:  "cred",
		Cloud: names.NewCloudTag(jimmtest.TestE2ECloudName),
		Owner: names.NewUserTag("alice@canonical.com"),
	})
	model := migration.ModelInfo{
		UUID:                   modelUUID,
		Owner:                  names.NewUserTag("alice"),
		Name:                   "test-model",
		ControllerAgentVersion: version.MustParse("3.6.9"),
		AgentVersion:           version.MustParse("3.6.9"),
		ModelDescription:       modelDescription,
	}
	client := migrationtarget.NewClient(conn)
	err := client.Prechecks(model)
	c.Assert(err, gc.ErrorMatches, `.*model migration not found.*`)

	controllerName, _ := s.GetOneControllerConfig(c)

	prepareModelMigration := params.PrepareModelMigrationRequest{
		ModelTag:              names.NewModelTag(modelUUID).String(),
		BackingControllerName: controllerName,
		UserMapping:           map[string]string{"alice": "alice@canonical.com"},
	}
	jimmClient := api.NewClient(conn)
	migrationToken, err := jimmClient.PrepareModelMigration(&prepareModelMigration)
	c.Assert(err, gc.IsNil)
	c.Assert(migrationToken, gc.Not(gc.Equals), "")

	err = client.Prechecks(model)
	c.Assert(err, gc.IsNil)
}

func (s *migrationTargetSuite) TestCACert(c *gc.C) {
	conn := s.Open(c, nil, "alice", nil)
	defer conn.Close()

	client := migrationtarget.NewClient(conn)
	cert, err := client.CACert()
	c.Assert(err, gc.IsNil)
	c.Assert(cert, gc.Equals, "")
}

func (s *migrationTargetSuite) TestAdoptResources(c *gc.C) {
	conn := s.Open(c, nil, "alice", nil)
	defer conn.Close()

	modelUUID := "00000001-0000-0000-0000-000000000001"
	client := migrationtarget.NewClient(conn)
	err := client.AdoptResources(modelUUID)
	c.Assert(err, gc.ErrorMatches, `.*model not found.*`)
}

func (s *migrationTargetSuite) TestActivate(c *gc.C) {
	conn := s.Open(c, nil, "alice", nil)
	defer conn.Close()

	modelUUID := "00000001-0000-0000-0000-000000000001"
	sourceInfo := migration.SourceControllerInfo{
		ControllerTag: names.NewControllerTag("00000001-0000-0000-0000-000000000002"),
	}
	relatedModels := []string{"related-model-1", "related-model-2"}

	client := migrationtarget.NewClient(conn)
	err := client.Activate(modelUUID, sourceInfo, relatedModels)
	c.Assert(err, gc.ErrorMatches, `.*model migration not found.*`)
}

func (s *migrationTargetSuite) TestLatestLogTime(c *gc.C) {
	conn := s.Open(c, nil, "alice", nil)
	defer conn.Close()

	client := migrationtarget.NewClient(conn)
	_, err := client.LatestLogTime(s.Model.UUID.String)
	c.Assert(err, gc.IsNil)
}

func (s *migrationTargetSuite) TestImport(c *gc.C) {
	conn := s.Open(c, nil, "alice", nil)
	defer conn.Close()

	client := migrationtarget.NewClient(conn)
	err := client.Import([]byte{})
	c.Assert(err, gc.ErrorMatches, `^failed to import model.*`)
}
