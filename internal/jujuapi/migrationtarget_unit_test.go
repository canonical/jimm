// Copyright 2025 Canonical.

package jujuapi_test

import (
	"context"
	"errors"
	"time"

	"github.com/juju/description/v9"
	"github.com/juju/juju/core/migration"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v5"
	"github.com/juju/version/v2"
	gc "gopkg.in/check.v1"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/jimm"
	"github.com/canonical/jimm/v3/internal/jujuapi"
	"github.com/canonical/jimm/v3/internal/openfga"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest/mocks"
)

type migrationTargetUnitSuite struct {
}

var _ = gc.Suite(&migrationTargetUnitSuite{})

func (s *migrationTargetUnitSuite) TestAbort(c *gc.C) {
	ctx := context.Background()

	abortCalled := false
	jujuManager := mocks.JujuManager{
		MigrationMocks: mocks.MigrationMocks{
			AbortMigration_: func(ctx context.Context, user *openfga.User, modelUUID string) error {
				abortCalled = true
				c.Check(modelUUID, gc.Equals, "00000001-0000-0000-0000-000000000001")
				return nil
			},
		}}
	jimm := &jimmtest.JIMM{
		JujuManager_: func() jimm.JujuManager {
			return &jujuManager
		},
	}

	var u dbmodel.Identity
	u.SetTag(names.NewUserTag("alice@canonical.com"))
	user := openfga.NewUser(&u, nil)

	cr := jujuapi.NewControllerRoot(jimm, jujuapi.Params{})
	jujuapi.SetUser(cr, user)

	args := jujuparams.ModelArgs{
		ModelTag: names.NewModelTag("00000001-0000-0000-0000-000000000001").String(),
	}

	// Validate access denied without JIMM admin permissions.
	err := cr.Abort(ctx, args)
	c.Assert(err, gc.ErrorMatches, `unauthorized`)
	c.Assert(abortCalled, gc.Equals, false)

	// Validate the method is called when the user is a JIMM admin.
	user.JimmAdmin = true
	err = cr.Abort(ctx, args)
	c.Assert(err, gc.IsNil)
	c.Assert(abortCalled, gc.Equals, true)

	// Validate that an invalid model tag is rejected.
	args.ModelTag = "invalid-model-tag"
	err = cr.Abort(ctx, args)
	c.Assert(err, gc.ErrorMatches, `"invalid-model-tag" is not a valid tag`)
}

func (s *migrationTargetUnitSuite) TestCheckMachines(c *gc.C) {
	ctx := context.Background()

	checkMachinesCalled := false
	jujuManager := mocks.JujuManager{
		MigrationMocks: mocks.MigrationMocks{
			CheckMachines_: func(ctx context.Context, user *openfga.User, modelUUID string) ([]error, error) {
				checkMachinesCalled = true
				c.Check(modelUUID, gc.Equals, "00000001-0000-0000-0000-000000000001")
				return []error{errors.New("fake-error")}, nil
			},
		}}
	jimm := &jimmtest.JIMM{
		JujuManager_: func() jimm.JujuManager {
			return &jujuManager
		},
	}

	var u dbmodel.Identity
	u.SetTag(names.NewUserTag("alice@canonical.com"))
	user := openfga.NewUser(&u, nil)

	cr := jujuapi.NewControllerRoot(jimm, jujuapi.Params{})
	jujuapi.SetUser(cr, user)

	args := jujuparams.ModelArgs{
		ModelTag: names.NewModelTag("00000001-0000-0000-0000-000000000001").String(),
	}

	// Validate access denied without JIMM admin permissions.
	_, err := cr.CheckMachines(ctx, args)
	c.Assert(err, gc.ErrorMatches, `unauthorized`)
	c.Assert(checkMachinesCalled, gc.Equals, false)

	// Validate the checkMachines method is called when the user is a JIMM admin.
	user.JimmAdmin = true
	res, err := cr.CheckMachines(ctx, args)
	c.Assert(err, gc.IsNil)
	c.Assert(res.Results, gc.HasLen, 1)
	c.Assert(res.Results[0].Error.Message, gc.Equals, "fake-error")
	c.Assert(res.Results[0].Error.Code, gc.Equals, "")
	c.Assert(res.Results[0].Error.Info, gc.IsNil)
	c.Assert(checkMachinesCalled, gc.Equals, true)

	// Validate that an invalid model tag is rejected.
	args.ModelTag = "invalid-model-tag"
	_, err = cr.CheckMachines(ctx, args)
	c.Assert(err, gc.ErrorMatches, `"invalid-model-tag" is not a valid tag`)
}

func (s *migrationTargetUnitSuite) TestPreChecks(c *gc.C) {
	ctx := context.Background()

	preChecksCalled := false
	jujuManager := mocks.JujuManager{
		MigrationMocks: mocks.MigrationMocks{
			Prechecks_: func(ctx context.Context, user *openfga.User, model migration.ModelInfo) error {
				preChecksCalled = true
				c.Assert(model.UUID, gc.Equals, "00000001-0000-0000-0000-000000000001")
				c.Assert(model.Owner.Id(), gc.Equals, "bob")
				return nil
			},
		}}
	jimm := &jimmtest.JIMM{
		JujuManager_: func() jimm.JujuManager {
			return &jujuManager
		},
	}

	var u dbmodel.Identity
	u.SetTag(names.NewUserTag("alice@canonical.com"))
	user := openfga.NewUser(&u, nil)

	cr := jujuapi.NewControllerRoot(jimm, jujuapi.Params{})
	jujuapi.SetUser(cr, user)

	modelDescription := description.NewModel(description.ModelArgs{
		Type:        description.IAAS,
		Owner:       names.NewUserTag("bob"),
		Cloud:       jimmtest.TestCloudName,
		CloudRegion: jimmtest.TestCloudRegionName,
	})
	modelDescription.SetStatus(description.StatusArgs{Value: "available"})

	serialisedDescription, err := description.Serialize(modelDescription)
	c.Assert(err, gc.IsNil)

	args := jujuparams.MigrationModelInfo{
		UUID:             "00000001-0000-0000-0000-000000000001",
		Name:             "test-model",
		OwnerTag:         names.NewUserTag("bob").String(),
		ModelDescription: serialisedDescription,
	}

	// Validate access denied without JIMM admin permissions.
	err = cr.Prechecks(ctx, args)
	c.Assert(err, gc.ErrorMatches, `unauthorized`)
	c.Assert(preChecksCalled, gc.Equals, false)

	// Validate the precheck method is called when the user is a JIMM admin.
	user.JimmAdmin = true
	err = cr.Prechecks(ctx, args)
	c.Assert(err, gc.IsNil)
	c.Assert(preChecksCalled, gc.Equals, true)

	// Validate that an invalid owner tag is rejected.
	args.OwnerTag = "invalid-owner-tag"
	err = cr.Prechecks(ctx, args)
	c.Assert(err, gc.ErrorMatches, `"invalid-owner-tag" is not a valid tag`)
}

func (s *migrationTargetUnitSuite) TestPreChecks_InvalidModelDescription(c *gc.C) {
	ctx := context.Background()

	jimm := &jimmtest.JIMM{}

	var u dbmodel.Identity
	u.SetTag(names.NewUserTag("alice@canonical.com"))
	user := openfga.NewUser(&u, nil)
	user.JimmAdmin = true

	cr := jujuapi.NewControllerRoot(jimm, jujuapi.Params{})
	jujuapi.SetUser(cr, user)

	args := jujuparams.MigrationModelInfo{
		UUID:             "00000001-0000-0000-0000-000000000001",
		Name:             "test-model",
		OwnerTag:         names.NewUserTag("bob").String(),
		ModelDescription: []byte(`invalid`),
	}

	// Validate access denied without JIMM admin permissions.
	err := cr.Prechecks(ctx, args)
	c.Assert(err, gc.ErrorMatches, `(?s)failed to deserialize model description.*`)
}

func (s *migrationTargetUnitSuite) TestAdoptResources(c *gc.C) {
	ctx := context.Background()

	adoptResourcesCalled := false
	jujuManager := mocks.JujuManager{
		MigrationMocks: mocks.MigrationMocks{
			AdoptResources_: func(ctx context.Context, user *openfga.User, modelUUID string, controllerVersion version.Number) error {
				adoptResourcesCalled = true
				c.Assert(modelUUID, gc.Equals, "00000001-0000-0000-0000-000000000001")
				c.Assert(controllerVersion, gc.DeepEquals, version.MustParse("3.2.1"))
				return nil
			},
		}}
	jimm := &jimmtest.JIMM{
		JujuManager_: func() jimm.JujuManager {
			return &jujuManager
		},
	}

	var u dbmodel.Identity
	u.SetTag(names.NewUserTag("alice@canonical.com"))
	user := openfga.NewUser(&u, nil)

	cr := jujuapi.NewControllerRoot(jimm, jujuapi.Params{})
	jujuapi.SetUser(cr, user)

	args := jujuparams.AdoptResourcesArgs{
		ModelTag:                names.NewModelTag("00000001-0000-0000-0000-000000000001").String(),
		SourceControllerVersion: version.MustParse("3.2.1"),
	}

	// Validate access denied without JIMM admin permissions.
	err := cr.AdoptResources(ctx, args)
	c.Assert(err, gc.ErrorMatches, `unauthorized`)
	c.Assert(adoptResourcesCalled, gc.Equals, false)

	// Validate the precheck method is called when the user is a JIMM admin.
	user.JimmAdmin = true
	err = cr.AdoptResources(ctx, args)
	c.Assert(err, gc.IsNil)
	c.Assert(adoptResourcesCalled, gc.Equals, true)

	// Validate that an invalid model tag is rejected.
	args.ModelTag = "invalid-model-tag"
	err = cr.AdoptResources(ctx, args)
	c.Assert(err, gc.ErrorMatches, `"invalid-model-tag" is not a valid tag`)
}

func (s *migrationTargetUnitSuite) TestActivateUnauthorized(c *gc.C) {
	ctx := context.Background()

	jujuManager := mocks.JujuManager{}
	jimm := &jimmtest.JIMM{
		JujuManager_: func() jimm.JujuManager {
			return &jujuManager
		},
	}

	var u dbmodel.Identity
	u.SetTag(names.NewUserTag("alice@canonical.com"))
	user := openfga.NewUser(&u, nil)

	cr := jujuapi.NewControllerRoot(jimm, jujuapi.Params{})
	jujuapi.SetUser(cr, user)

	args := jujuparams.ActivateModelArgs{
		ModelTag:        names.NewModelTag("00000001-0000-0000-0000-000000000001").String(),
		ControllerTag:   names.NewControllerTag("00000001-0000-0000-0000-000000000002").String(),
		ControllerAlias: "controller-1",
		CrossModelUUIDs: []string{"related-model-1", "related-model-2"},
	}

	// Validate access denied without JIMM admin permissions.
	err := cr.Activate(ctx, args)
	c.Assert(err, gc.ErrorMatches, `unauthorized`)
}

func (s *migrationTargetUnitSuite) TestActivateValid(c *gc.C) {
	ctx := context.Background()

	activateCalled := false
	jujuManager := mocks.JujuManager{
		MigrationMocks: mocks.MigrationMocks{
			Activate_: func(ctx context.Context, modelTag names.ModelTag, sourceControllerInfo migration.SourceControllerInfo, relatedModels []string) error {
				activateCalled = true
				c.Assert(modelTag.Id(), gc.Equals, "00000001-0000-0000-0000-000000000001")
				c.Assert(sourceControllerInfo.ControllerAlias, gc.Equals, "controller-1")
				c.Assert(relatedModels, gc.DeepEquals, []string{"related-model-1", "related-model-2"})
				return nil
			},
		}}
	jimm := &jimmtest.JIMM{
		JujuManager_: func() jimm.JujuManager {
			return &jujuManager
		},
	}

	var u dbmodel.Identity
	u.SetTag(names.NewUserTag("alice@canonical.com"))
	user := openfga.NewUser(&u, nil)

	cr := jujuapi.NewControllerRoot(jimm, jujuapi.Params{})
	jujuapi.SetUser(cr, user)

	args := jujuparams.ActivateModelArgs{
		ModelTag:        names.NewModelTag("00000001-0000-0000-0000-000000000001").String(),
		ControllerTag:   names.NewControllerTag("00000001-0000-0000-0000-000000000002").String(),
		ControllerAlias: "controller-1",
		CrossModelUUIDs: []string{"related-model-1", "related-model-2"},
	}

	// Validate the activate method is called when the user is a JIMM admin.
	user.JimmAdmin = true
	err := cr.Activate(ctx, args)
	c.Assert(err, gc.IsNil)
	c.Assert(activateCalled, gc.Equals, true)
}

func (s *migrationTargetUnitSuite) TestActivateInvalidModelTag(c *gc.C) {
	ctx := context.Background()

	jujuManager := mocks.JujuManager{}
	jimm := &jimmtest.JIMM{
		JujuManager_: func() jimm.JujuManager {
			return &jujuManager
		},
	}

	var u dbmodel.Identity
	u.SetTag(names.NewUserTag("alice@canonical.com"))
	user := openfga.NewUser(&u, nil)

	cr := jujuapi.NewControllerRoot(jimm, jujuapi.Params{})
	jujuapi.SetUser(cr, user)

	args := jujuparams.ActivateModelArgs{
		ModelTag:        "invalid-model-tag",
		ControllerTag:   names.NewControllerTag("00000001-0000-0000-0000-000000000002").String(),
		ControllerAlias: "controller-1",
		CrossModelUUIDs: []string{"related-model-1", "related-model-2"},
	}

	// Validate that an invalid model tag is rejected.
	user.JimmAdmin = true
	err := cr.Activate(ctx, args)
	c.Assert(err, gc.ErrorMatches, `"invalid-model-tag" is not a valid tag`)
}

func (s *migrationTargetUnitSuite) TestActivateInvalidControllerTag(c *gc.C) {
	ctx := context.Background()

	jujuManager := mocks.JujuManager{}
	jimm := &jimmtest.JIMM{
		JujuManager_: func() jimm.JujuManager {
			return &jujuManager
		},
	}

	var u dbmodel.Identity
	u.SetTag(names.NewUserTag("alice@canonical.com"))
	user := openfga.NewUser(&u, nil)

	cr := jujuapi.NewControllerRoot(jimm, jujuapi.Params{})
	jujuapi.SetUser(cr, user)

	args := jujuparams.ActivateModelArgs{
		ModelTag:        names.NewModelTag("00000001-0000-0000-0000-000000000001").String(),
		ControllerTag:   "invalid-controller-tag",
		ControllerAlias: "controller-1",
		CrossModelUUIDs: []string{"related-model-1", "related-model-2"},
	}

	// Validate that an invalid controller tag is rejected.
	user.JimmAdmin = true
	err := cr.Activate(ctx, args)
	c.Assert(err, gc.ErrorMatches, `"invalid-controller-tag" is not a valid tag`)
}

func (s *migrationTargetUnitSuite) TestLatestLogTime(c *gc.C) {
	ctx := context.Background()

	latestLogTimeCalled := false
	jujuManager := mocks.JujuManager{
		MigrationMocks: mocks.MigrationMocks{
			LatestLogTime_: func(ctx context.Context, modelUUID string) (time.Time, error) {
				latestLogTimeCalled = true
				c.Check(modelUUID, gc.Equals, "00000001-0000-0000-0000-000000000001")
				return time.Now(), nil
			},
		}}
	jimm := &jimmtest.JIMM{
		JujuManager_: func() jimm.JujuManager {
			return &jujuManager
		},
	}

	var u dbmodel.Identity
	u.SetTag(names.NewUserTag("alice@canonical.com"))
	user := openfga.NewUser(&u, nil)
	cr := jujuapi.NewControllerRoot(jimm, jujuapi.Params{})
	jujuapi.SetUser(cr, user)

	args := jujuparams.ModelArgs{}

	// Validate access denied without JIMM admin permissions.
	_, err := cr.LatestLogTime(ctx, args)
	c.Assert(err, gc.ErrorMatches, `unauthorized`)
	c.Assert(latestLogTimeCalled, gc.Equals, false)

	// Validate the latest log time method is not called with an invalid model tag.
	user.JimmAdmin = true
	args.ModelTag = "invalid-model-tag"
	_, err = cr.LatestLogTime(ctx, args)
	c.Assert(err, gc.ErrorMatches, `"invalid-model-tag" is not a valid tag`)
	c.Assert(latestLogTimeCalled, gc.Equals, false)

	// Validate the latest log time method is called when the user is a JIMM admin
	// with a valid model tag.
	args.ModelTag = names.NewModelTag("00000001-0000-0000-0000-000000000001").String()
	logTime, err := cr.LatestLogTime(ctx, args)
	c.Assert(err, gc.IsNil)
	c.Assert(logTime, gc.Not(gc.IsNil))
	c.Assert(latestLogTimeCalled, gc.Equals, true)
}
