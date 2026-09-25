// Copyright 2026 Canonical.

package jujuapi_test

import (
	"context"
	"testing"

	qt "github.com/frankban/quicktest"
	"github.com/juju/juju/core/semversion"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v6"

	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/jujuapi"
	"github.com/canonical/jimm/v3/internal/openfga"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest/mocks"
)

func TestUpgradeModel_BadModelTag(t *testing.T) {
	c := qt.New(t)

	ctx := c.Context()
	jimm := &jimmtest.JIMM{
		JujuManager_: func() jujuapi.JujuManager {
			return &mocks.JujuManager{}
		},
	}
	root := newTestControllerRoot(jimm, "alice@canonical.com", false)

	result, err := root.UpgradeModel(ctx, jujuparams.UpgradeModelParams{
		ModelTag: "not-a-valid-tag",
	})
	c.Assert(err, qt.ErrorMatches, `.*not-a-valid-tag.*`)
	c.Assert(errors.ErrorCode(err), qt.Equals, errors.CodeBadRequest)
	c.Assert(result, qt.DeepEquals, jujuparams.UpgradeModelResult{})
}

func TestUpgradeModel_Unauthorized(t *testing.T) {
	c := qt.New(t)

	ctx := c.Context()
	modelUUID := "00000000-0000-0000-0000-000000000001"
	jimm := &jimmtest.JIMM{
		JujuManager_: func() jujuapi.JujuManager {
			return &mocks.JujuManager{
				ModelManager: mocks.ModelManager{
					UpgradeModel_: func(_ context.Context, _ *openfga.User, _ names.ModelTag, _ semversion.Number, _ string, _ bool, _ bool) (semversion.Number, error) {
						return semversion.Zero, errors.Codef(errors.CodeUnauthorized, "unauthorized")
					},
				},
			}
		},
	}
	root := newTestControllerRoot(jimm, "alice@canonical.com", false)

	result, err := root.UpgradeModel(ctx, jujuparams.UpgradeModelParams{
		ModelTag: names.NewModelTag(modelUUID).String(),
	})
	c.Assert(errors.ErrorCode(err), qt.Equals, errors.CodeUnauthorized)
	c.Assert(result, qt.DeepEquals, jujuparams.UpgradeModelResult{})
}

func TestUpgradeModel_Success(t *testing.T) {
	c := qt.New(t)

	ctx := c.Context()
	modelUUID := "00000000-0000-0000-0000-000000000001"
	targetVersion, err := semversion.Parse("3.6.12")
	c.Assert(err, qt.IsNil)
	chosenVersion, err := semversion.Parse("3.6.12")
	c.Assert(err, qt.IsNil)

	called := false
	jimm := &jimmtest.JIMM{
		JujuManager_: func() jujuapi.JujuManager {
			return &mocks.JujuManager{
				ModelManager: mocks.ModelManager{
					UpgradeModel_: func(_ context.Context, _ *openfga.User, mt names.ModelTag, tv semversion.Number, stream string, ignoreAgentVersions bool, dryRun bool) (semversion.Number, error) {
						called = true
						c.Assert(mt.Id(), qt.Equals, modelUUID)
						c.Assert(tv, qt.DeepEquals, targetVersion)
						c.Assert(stream, qt.Equals, "")
						c.Assert(ignoreAgentVersions, qt.IsFalse)
						c.Assert(dryRun, qt.IsFalse)
						return chosenVersion, nil
					},
				},
			}
		},
	}
	root := newTestControllerRoot(jimm, "alice@canonical.com", false)

	result, err := root.UpgradeModel(ctx, jujuparams.UpgradeModelParams{
		ModelTag:      names.NewModelTag(modelUUID).String(),
		TargetVersion: targetVersion,
	})
	c.Assert(err, qt.IsNil)
	c.Assert(called, qt.IsTrue)
	c.Assert(result.ChosenVersion, qt.DeepEquals, chosenVersion)
}

func TestUpgradeModel_DryRun(t *testing.T) {
	c := qt.New(t)

	ctx := c.Context()
	modelUUID := "00000000-0000-0000-0000-000000000001"
	targetVersion, err := semversion.Parse("3.6.12")
	c.Assert(err, qt.IsNil)

	jimm := &jimmtest.JIMM{
		JujuManager_: func() jujuapi.JujuManager {
			return &mocks.JujuManager{
				ModelManager: mocks.ModelManager{
					UpgradeModel_: func(_ context.Context, _ *openfga.User, _ names.ModelTag, _ semversion.Number, stream string, ignoreAgentVersions bool, dryRun bool) (semversion.Number, error) {
						c.Assert(stream, qt.Equals, "proposed")
						c.Assert(ignoreAgentVersions, qt.IsTrue)
						c.Assert(dryRun, qt.IsTrue)
						return targetVersion, nil
					},
				},
			}
		},
	}
	root := newTestControllerRoot(jimm, "alice@canonical.com", false)

	result, err := root.UpgradeModel(ctx, jujuparams.UpgradeModelParams{
		ModelTag:            names.NewModelTag(modelUUID).String(),
		TargetVersion:       targetVersion,
		AgentStream:         "proposed",
		IgnoreAgentVersions: true,
		DryRun:              true,
	})
	c.Assert(err, qt.IsNil)
	c.Assert(result.ChosenVersion, qt.DeepEquals, targetVersion)
}

// TestModelUpgraderFacadeVersions verifies that JIMM advertises the
// ModelUpgrader facade at both version 1 (Juju 3.6 clients) and version 2
// (Juju 4.x clients).
func TestModelUpgraderFacadeVersions(t *testing.T) {
	c := qt.New(t)

	supported := jujuapi.SupportedFacades()
	c.Assert(supported["ModelUpgrader"], qt.DeepEquals, []int{1, 2})
}
