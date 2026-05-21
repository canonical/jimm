// Copyright 2026 Canonical.

package testing

import (
	"testing"

	petname "github.com/dustinkirkland/golang-petname"
	qt "github.com/frankban/quicktest"
	"github.com/juju/juju/api/client/modelupgrader"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v5"
	"github.com/juju/version/v2"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
	"github.com/canonical/jimm/v3/pkg/api"
	apiparams "github.com/canonical/jimm/v3/pkg/api/params"
)

func TestUpgradeModelDryRun(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupJimmWithControllers(c)
	model := s.CreateModelForBob(c)

	conn := s.Open(c, nil, "bob@canonical.com", nil)
	defer conn.Close()

	client := modelupgrader.NewClient(conn)
	chosenVersion, err := client.UpgradeModel(model.UUID.String, version.Zero, "", false, true)
	c.Assert(err, qt.IsNil)
	c.Assert(chosenVersion, qt.Not(qt.Equals), version.Zero)
}

func TestUpgradeModel(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupJimmWithControllers(c)

	ctrlName, _ := s.GetOneControllerConfig(c)
	controller := dbmodel.Controller{Name: ctrlName}
	err := s.JIMM.Database.GetController(c.Context(), &controller)
	c.Assert(err, qt.IsNil)
	c.Assert(controller.AgentVersion, qt.Not(qt.Equals), "")

	ctrlVersion := version.MustParse(controller.AgentVersion)
	var lowerVersion version.Number
	switch ctrlVersion.Major {
	case 3:
		lowerVersion = version.Number{
			Major: 3,
			Minor: 6,
			Patch: 20,
		}
	case 4:
		lowerVersion = version.Number{
			Major: 4,
			Minor: 0,
			Patch: 6,
		}
	default:
		c.Fatalf("unexpected controller major version %d", ctrlVersion.Major)
	}

	conn := s.Open(c, nil, "bob@canonical.com", nil)
	defer conn.Close()
	jimmClient := api.NewClient(conn)

	// Create a model pinned to a lower agent version so there is something to upgrade.
	mi, err := jimmClient.AddModelToController(&apiparams.AddModelToControllerRequest{
		ModelCreateArgs: jujuparams.ModelCreateArgs{
			Name:               petname.Generate(2, "-"),
			OwnerTag:           names.NewUserTag("bob@canonical.com").String(),
			CloudTag:           names.NewCloudTag(jimmtest.TestE2ECloudName).String(),
			CloudRegion:        jimmtest.TestE2ECloudRegionName,
			CloudCredentialTag: "cloudcred-" + jimmtest.TestE2ECloudName + "_bob@canonical.com_cred",
			Config: map[string]any{
				"agent-version": lowerVersion.String(),
			},
		},
		ControllerName: controller.Name,
	})
	c.Assert(err, qt.IsNil)
	c.Cleanup(func() {
		s.DestroyModelAndDeleteFromDatabase(c, names.NewModelTag(mi.UUID))
	})

	// Upgrade the model to the current controller version.
	upgradeClient := modelupgrader.NewClient(conn)
	result, err := upgradeClient.UpgradeModel(mi.UUID, ctrlVersion, "", false, false)
	c.Assert(err, qt.IsNil)
	c.Assert(result, qt.Equals, ctrlVersion)
}

func TestUpgradeModelCrossMajor(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupJimmWithControllers(c)
	model := s.CreateModelForBob(c)
	ctrlVersion := version.MustParse(model.Controller.AgentVersion)

	nextMajorVersion := version.Number{
		Major: ctrlVersion.Major + 1,
	}

	conn := s.Open(c, nil, "bob@canonical.com", nil)
	defer conn.Close()

	client := modelupgrader.NewClient(conn)
	// Attempting to upgrade to a version beyond the controller's version is rejected
	// by the backing controller with the message below.
	_, err := client.UpgradeModel(model.UUID.String, nextMajorVersion, "", false, false)
	c.Assert(err, qt.ErrorMatches, `.*cannot upgrade to a version .* greater than that of the controller .*`)
}

func TestUpgradeModelUnauthorized(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupJimmWithControllers(c)
	// Charlie owns the model; bob only has read access.
	model := s.CreateModelForCharlieWithBobReadAccess(c)

	conn := s.Open(c, nil, "bob@canonical.com", nil)
	defer conn.Close()

	client := modelupgrader.NewClient(conn)
	_, err := client.UpgradeModel(model.UUID.String, version.Zero, "", false, true)
	c.Assert(err, qt.ErrorMatches, `.*unauthorized.*`)
}

func TestAbortModelUpgrade(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupJimmWithControllers(c)
	model := s.CreateModelForBob(c)

	conn := s.Open(c, nil, "bob@canonical.com", nil)
	defer conn.Close()

	client := modelupgrader.NewClient(conn)
	err := client.AbortModelUpgrade(model.UUID.String)
	c.Assert(err, qt.IsNil)
}

func TestAbortModelUpgradeUnauthorized(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupJimmWithControllers(c)
	// Charlie owns the model; bob only has read access.
	model := s.CreateModelForCharlieWithBobReadAccess(c)

	conn := s.Open(c, nil, "bob@canonical.com", nil)
	defer conn.Close()

	client := modelupgrader.NewClient(conn)
	err := client.AbortModelUpgrade(model.UUID.String)
	c.Assert(err, qt.ErrorMatches, `.*unauthorized.*`)
}
