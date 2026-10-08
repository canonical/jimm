// Copyright 2025 Canonical.

package jujuapi_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
	"github.com/juju/description/v11"
	"github.com/juju/juju/core/migration"
	"github.com/juju/juju/core/semversion"
	jujuparams "github.com/juju/juju/rpc/params"
	namesv5 "github.com/juju/names/v5"
	"github.com/juju/names/v6"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/jimm/juju"
	"github.com/canonical/jimm/v3/internal/jujuapi"
	"github.com/canonical/jimm/v3/internal/openfga"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest/mocks"
)

func TestAbort(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	abortCalled := false
	jujuManager := mocks.JujuManager{
		MigrationMocks: mocks.MigrationMocks{
			AbortMigration_: func(ctx context.Context, user *openfga.User, modelUUID string) error {
				abortCalled = true
				c.Check(modelUUID, qt.Equals, "00000001-0000-0000-0000-000000000001")
				return nil
			},
		}}
	jimm := &jimmtest.JIMM{
		JujuManager_: func() jujuapi.JujuManager {
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
	c.Assert(err, qt.ErrorMatches, `unauthorized`)
	c.Assert(abortCalled, qt.Equals, false)

	// Validate the method is called when the user is a JIMM admin.
	user.JimmAdmin = true
	err = cr.Abort(ctx, args)
	c.Assert(err, qt.IsNil)
	c.Assert(abortCalled, qt.Equals, true)

	// Validate that an invalid model tag is rejected.
	args.ModelTag = "invalid-model-tag"
	err = cr.Abort(ctx, args)
	c.Assert(err, qt.ErrorMatches, `"invalid-model-tag" is not a valid tag`)
}

func TestCheckMachines(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	checkMachinesCalled := false
	jujuManager := mocks.JujuManager{
		MigrationMocks: mocks.MigrationMocks{
			CheckMachines_: func(ctx context.Context, user *openfga.User, modelUUID string) ([]error, error) {
				checkMachinesCalled = true
				c.Check(modelUUID, qt.Equals, "00000001-0000-0000-0000-000000000001")
				return []error{errors.New("fake-error")}, nil
			},
		}}
	jimm := &jimmtest.JIMM{
		JujuManager_: func() jujuapi.JujuManager {
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
	c.Assert(err, qt.ErrorMatches, `unauthorized`)
	c.Assert(checkMachinesCalled, qt.Equals, false)

	// Validate the checkMachines method is called when the user is a JIMM admin.
	user.JimmAdmin = true
	res, err := cr.CheckMachines(ctx, args)
	c.Assert(err, qt.IsNil)
	c.Assert(res.Results, qt.HasLen, 1)
	c.Assert(res.Results[0].Error.Message, qt.Equals, "fake-error")
	c.Assert(res.Results[0].Error.Code, qt.Equals, "")
	c.Assert(res.Results[0].Error.Info, qt.IsNil)
	c.Assert(checkMachinesCalled, qt.Equals, true)

	// Validate that an invalid model tag is rejected.
	args.ModelTag = "invalid-model-tag"
	_, err = cr.CheckMachines(ctx, args)
	c.Assert(err, qt.ErrorMatches, `"invalid-model-tag" is not a valid tag`)
}

func TestPreChecks(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	preChecksCalled := false
	jujuManager := mocks.JujuManager{
		MigrationMocks: mocks.MigrationMocks{
			Prechecks_: func(ctx context.Context, user *openfga.User, model juju.MigratingModelInfo) error {
				preChecksCalled = true
				c.Assert(model.UUID, qt.Equals, "00000001-0000-0000-0000-000000000001")
				c.Assert(model.Owner, qt.Equals, "bob")
				return nil
			},
		}}
	jimm := &jimmtest.JIMM{
		JujuManager_: func() jujuapi.JujuManager {
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
		Owner:       namesv5.NewUserTag("bob"),
		Cloud:       jimmtest.TestCloudName,
		CloudRegion: jimmtest.TestCloudRegionName,
	})

	serialisedDescription, err := description.Serialize(modelDescription)
	c.Assert(err, qt.IsNil)

	args := jujuparams.MigrationModelInfoLegacy{
		UUID:                   "00000001-0000-0000-0000-000000000001",
		Name:                   "test-model",
		OwnerTag:               names.NewUserTag("bob").String(),
		ControllerAgentVersion: semversion.MustParse("3.6.9"),
		ModelDescription:       serialisedDescription,
	}

	// Validate access denied without JIMM admin permissions.
	err = cr.Prechecks(ctx, args)
	c.Assert(err, qt.ErrorMatches, `unauthorized`)
	c.Assert(preChecksCalled, qt.Equals, false)

	// Validate the precheck method is called when the user is a JIMM admin.
	user.JimmAdmin = true
	err = cr.Prechecks(ctx, args)
	c.Assert(err, qt.IsNil)
	c.Assert(preChecksCalled, qt.Equals, true)
}

func TestAdoptResources(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	adoptResourcesCalled := false
	jujuManager := mocks.JujuManager{
		MigrationMocks: mocks.MigrationMocks{
			AdoptResources_: func(ctx context.Context, user *openfga.User, modelUUID string, controllerVersion semversion.Number) error {
				adoptResourcesCalled = true
				c.Assert(modelUUID, qt.Equals, "00000001-0000-0000-0000-000000000001")
				c.Assert(controllerVersion, qt.DeepEquals, semversion.MustParse("3.2.1"))
				return nil
			},
		}}
	jimm := &jimmtest.JIMM{
		JujuManager_: func() jujuapi.JujuManager {
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
		SourceControllerVersion: semversion.MustParse("3.2.1"),
	}

	// Validate access denied without JIMM admin permissions.
	err := cr.AdoptResources(ctx, args)
	c.Assert(err, qt.ErrorMatches, `unauthorized`)
	c.Assert(adoptResourcesCalled, qt.Equals, false)

	// Validate the precheck method is called when the user is a JIMM admin.
	user.JimmAdmin = true
	err = cr.AdoptResources(ctx, args)
	c.Assert(err, qt.IsNil)
	c.Assert(adoptResourcesCalled, qt.Equals, true)

	// Validate that an invalid model tag is rejected.
	args.ModelTag = "invalid-model-tag"
	err = cr.AdoptResources(ctx, args)
	c.Assert(err, qt.ErrorMatches, `"invalid-model-tag" is not a valid tag`)
}

func TestActivateUnauthorized(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	jujuManager := mocks.JujuManager{}
	jimm := &jimmtest.JIMM{
		JujuManager_: func() jujuapi.JujuManager {
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
	c.Assert(err, qt.ErrorMatches, `unauthorized`)
}

func TestActivateValid(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	activateCalled := false
	jujuManager := mocks.JujuManager{
		MigrationMocks: mocks.MigrationMocks{
			Activate_: func(ctx context.Context, user *openfga.User, modelTag names.ModelTag, sourceControllerInfo migration.SourceControllerInfo, relatedModels []string) error {
				activateCalled = true
				c.Assert(modelTag.Id(), qt.Equals, "00000001-0000-0000-0000-000000000001")
				c.Assert(sourceControllerInfo.ControllerAlias, qt.Equals, "controller-1")
				c.Assert(relatedModels, qt.DeepEquals, []string{"related-model-1", "related-model-2"})
				return nil
			},
		}}
	jimm := &jimmtest.JIMM{
		JujuManager_: func() jujuapi.JujuManager {
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
	c.Assert(err, qt.IsNil)
	c.Assert(activateCalled, qt.Equals, true)
}

func TestActivateInvalidModelTag(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	jujuManager := mocks.JujuManager{}
	jimm := &jimmtest.JIMM{
		JujuManager_: func() jujuapi.JujuManager {
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
	c.Assert(err, qt.ErrorMatches, `.*"invalid-model-tag" is not a valid tag`)
}

func TestActivateInvalidControllerTag(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	jujuManager := mocks.JujuManager{}
	jimm := &jimmtest.JIMM{
		JujuManager_: func() jujuapi.JujuManager {
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
	c.Assert(err, qt.ErrorMatches, `.*"invalid-controller-tag" is not a valid tag`)
}

func TestActivateMissingControllerTag(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	jujuManager := mocks.JujuManager{
		MigrationMocks: mocks.MigrationMocks{
			Activate_: func(ctx context.Context, user *openfga.User, modelTag names.ModelTag, sourceControllerInfo migration.SourceControllerInfo, relatedModels []string) error {
				// This function should not be called
				return nil
			},
		},
	}
	jimm := &jimmtest.JIMM{
		JujuManager_: func() jujuapi.JujuManager {
			return &jujuManager
		},
	}

	var u dbmodel.Identity
	u.SetTag(names.NewUserTag("alice@canonical.com"))
	user := openfga.NewUser(&u, nil)

	cr := jujuapi.NewControllerRoot(jimm, jujuapi.Params{})
	jujuapi.SetUser(cr, user)

	// The only required field is the model tag.
	args := jujuparams.ActivateModelArgs{
		ModelTag: names.NewModelTag("00000001-0000-0000-0000-000000000001").String(),
	}

	// Validate that an invalid controller tag is rejected.
	user.JimmAdmin = true
	err := cr.Activate(ctx, args)
	c.Assert(err, qt.IsNil)
}

func TestLatestLogTime(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	latestLogTimeCalled := false
	jujuManager := mocks.JujuManager{
		MigrationMocks: mocks.MigrationMocks{
			LatestLogTime_: func(ctx context.Context, user *openfga.User, modelUUID string) (time.Time, error) {
				latestLogTimeCalled = true
				c.Check(modelUUID, qt.Equals, "00000001-0000-0000-0000-000000000001")
				return time.Now(), nil
			},
		}}
	jimm := &jimmtest.JIMM{
		JujuManager_: func() jujuapi.JujuManager {
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
	c.Assert(err, qt.ErrorMatches, `unauthorized`)
	c.Assert(latestLogTimeCalled, qt.Equals, false)

	// Validate the latest log time method is not called with an invalid model tag.
	user.JimmAdmin = true
	args.ModelTag = "invalid-model-tag"
	_, err = cr.LatestLogTime(ctx, args)
	c.Assert(err, qt.ErrorMatches, `"invalid-model-tag" is not a valid tag`)
	c.Assert(latestLogTimeCalled, qt.Equals, false)

	// Validate the latest log time method is called when the user is a JIMM admin
	// with a valid model tag.
	args.ModelTag = names.NewModelTag("00000001-0000-0000-0000-000000000001").String()
	logTime, err := cr.LatestLogTime(ctx, args)
	c.Assert(err, qt.IsNil)
	c.Assert(logTime, qt.Not(qt.IsNil))
	c.Assert(latestLogTimeCalled, qt.Equals, true)
}

func TestImportValid(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	activateCalled := false
	jujuManager := mocks.JujuManager{
		MigrationMocks: mocks.MigrationMocks{
			Import_: func(ctx context.Context, user *openfga.User, serialized jujuparams.SerializedModel) error {
				activateCalled = true
				return nil
			},
		}}
	jimm := &jimmtest.JIMM{
		JujuManager_: func() jujuapi.JujuManager {
			return &jujuManager
		},
	}

	var u dbmodel.Identity
	u.SetTag(names.NewUserTag("alice@canonical.com"))
	user := openfga.NewUser(&u, nil)

	cr := jujuapi.NewControllerRoot(jimm, jujuapi.Params{})
	jujuapi.SetUser(cr, user)

	// Validate the activate method is called when the user is a JIMM admin.
	user.JimmAdmin = true
	err := cr.Import(ctx, jujuparams.SerializedModel{})
	c.Assert(err, qt.IsNil)
	c.Assert(activateCalled, qt.Equals, true)
}

func TestImportUnauthorized(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	jujuManager := mocks.JujuManager{}
	jimm := &jimmtest.JIMM{
		JujuManager_: func() jujuapi.JujuManager {
			return &jujuManager
		},
	}

	var u dbmodel.Identity
	u.SetTag(names.NewUserTag("alice@canonical.com"))
	user := openfga.NewUser(&u, nil)

	cr := jujuapi.NewControllerRoot(jimm, jujuapi.Params{})
	jujuapi.SetUser(cr, user)

	// Validate access denied without JIMM admin permissions.
	err := cr.Import(ctx, jujuparams.SerializedModel{})
	c.Assert(err, qt.ErrorMatches, `unauthorized`)
}

func TestPrechecksV2(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	preChecksCalled := false
	jujuManager := mocks.JujuManager{
		MigrationMocks: mocks.MigrationMocks{
			PrechecksV2_: func(ctx context.Context, user *openfga.User, envelope jujuparams.SerializedModelV2) error {
				preChecksCalled = true
				c.Check(user.Name, qt.Equals, "alice@canonical.com")
				c.Check(envelope.ModelInfo.UUID, qt.Equals, "00000001-0000-0000-0000-000000000001")
				c.Check(envelope.ModelInfo.Qualifier, qt.Equals, "bob")
				return nil
			},
		}}
	jimm := &jimmtest.JIMM{
		JujuManager_: func() jujuapi.JujuManager {
			return &jujuManager
		},
	}

	var u dbmodel.Identity
	u.SetTag(names.NewUserTag("alice@canonical.com"))
	user := openfga.NewUser(&u, nil)

	cr := jujuapi.NewControllerRoot(jimm, jujuapi.Params{})
	jujuapi.SetUser(cr, user)

	envelope := jujuparams.SerializedModelV2{
		ModelInfo: jujuparams.SerializedModelInfo{
			UUID:      "00000001-0000-0000-0000-000000000001",
			Name:      "test-model",
			Qualifier: "bob",
		},
	}

	// Validate access denied without JIMM admin permissions.
	err := cr.PrechecksV2(ctx, envelope)
	c.Assert(err, qt.ErrorMatches, `unauthorized`)
	c.Assert(preChecksCalled, qt.Equals, false)

	// Validate the precheck method is called when the user is a JIMM admin.
	user.JimmAdmin = true
	err = cr.PrechecksV2(ctx, envelope)
	c.Assert(err, qt.IsNil)
	c.Assert(preChecksCalled, qt.Equals, true)
}

func TestPrechecksV2Error(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	jujuManager := mocks.JujuManager{
		MigrationMocks: mocks.MigrationMocks{
			PrechecksV2_: func(ctx context.Context, user *openfga.User, envelope jujuparams.SerializedModelV2) error {
				return errors.New("precheck failed")
			},
		}}
	jimm := &jimmtest.JIMM{
		JujuManager_: func() jujuapi.JujuManager {
			return &jujuManager
		},
	}

	var u dbmodel.Identity
	u.SetTag(names.NewUserTag("alice@canonical.com"))
	user := openfga.NewUser(&u, nil)
	user.JimmAdmin = true

	cr := jujuapi.NewControllerRoot(jimm, jujuapi.Params{})
	jujuapi.SetUser(cr, user)

	// Validate errors from the manager are returned to the caller.
	err := cr.PrechecksV2(ctx, jujuparams.SerializedModelV2{})
	c.Assert(err, qt.ErrorMatches, `precheck failed`)
}

func TestImportV2Valid(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	importCalled := false
	jujuManager := mocks.JujuManager{
		MigrationMocks: mocks.MigrationMocks{
			ImportV2_: func(ctx context.Context, user *openfga.User, envelope jujuparams.SerializedModelV2) error {
				importCalled = true
				c.Check(user.Name, qt.Equals, "alice@canonical.com")
				c.Check(envelope.ModelInfo.UUID, qt.Equals, "00000001-0000-0000-0000-000000000001")
				c.Check(envelope.Payload, qt.DeepEquals, []byte("payload"))
				return nil
			},
		}}
	jimm := &jimmtest.JIMM{
		JujuManager_: func() jujuapi.JujuManager {
			return &jujuManager
		},
	}

	var u dbmodel.Identity
	u.SetTag(names.NewUserTag("alice@canonical.com"))
	user := openfga.NewUser(&u, nil)

	cr := jujuapi.NewControllerRoot(jimm, jujuapi.Params{})
	jujuapi.SetUser(cr, user)

	envelope := jujuparams.SerializedModelV2{
		ModelInfo: jujuparams.SerializedModelInfo{
			UUID: "00000001-0000-0000-0000-000000000001",
		},
		Payload: []byte("payload"),
	}

	// Validate the import method is called when the user is a JIMM admin.
	user.JimmAdmin = true
	err := cr.ImportV2(ctx, envelope)
	c.Assert(err, qt.IsNil)
	c.Assert(importCalled, qt.Equals, true)
}

func TestImportV2Unauthorized(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	importCalled := false
	jujuManager := mocks.JujuManager{
		MigrationMocks: mocks.MigrationMocks{
			ImportV2_: func(ctx context.Context, user *openfga.User, envelope jujuparams.SerializedModelV2) error {
				importCalled = true
				return nil
			},
		}}
	jimm := &jimmtest.JIMM{
		JujuManager_: func() jujuapi.JujuManager {
			return &jujuManager
		},
	}

	var u dbmodel.Identity
	u.SetTag(names.NewUserTag("alice@canonical.com"))
	user := openfga.NewUser(&u, nil)

	cr := jujuapi.NewControllerRoot(jimm, jujuapi.Params{})
	jujuapi.SetUser(cr, user)

	// Validate access denied without JIMM admin permissions.
	err := cr.ImportV2(ctx, jujuparams.SerializedModelV2{})
	c.Assert(err, qt.ErrorMatches, `unauthorized`)
	c.Assert(importCalled, qt.Equals, false)
}

func TestImportV2Error(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	jujuManager := mocks.JujuManager{
		MigrationMocks: mocks.MigrationMocks{
			ImportV2_: func(ctx context.Context, user *openfga.User, envelope jujuparams.SerializedModelV2) error {
				return errors.New("import failed")
			},
		}}
	jimm := &jimmtest.JIMM{
		JujuManager_: func() jujuapi.JujuManager {
			return &jujuManager
		},
	}

	var u dbmodel.Identity
	u.SetTag(names.NewUserTag("alice@canonical.com"))
	user := openfga.NewUser(&u, nil)
	user.JimmAdmin = true

	cr := jujuapi.NewControllerRoot(jimm, jujuapi.Params{})
	jujuapi.SetUser(cr, user)

	// Validate errors from the manager are wrapped and returned to the caller.
	err := cr.ImportV2(ctx, jujuparams.SerializedModelV2{})
	c.Assert(err, qt.ErrorMatches, `failed to import model: import failed`)
}

func TestMigrationTargetFacadeVersions(t *testing.T) {
	c := qt.New(t)

	// JIMM offers v6 and v8. v7 is deliberately not implemented as it is only required
	// for 4.0.0-4.0.11 with was effectively DOA.
	c.Assert(jujuapi.SupportedFacades()["MigrationTarget"], qt.DeepEquals, []int{6, 8})

	cr := jujuapi.NewControllerRoot(&jimmtest.JIMM{}, jujuapi.Params{})
	jujuapi.SetupFacades(cr)

	// Prechecks and Import take a model description in v6 and a
	// SerializedModelV2 envelope in v8.
	paramsTypes := []struct {
		version    int
		method     string
		paramsType reflect.Type
	}{
		{6, "Prechecks", reflect.TypeFor[jujuparams.MigrationModelInfoLegacy]()},
		{6, "Import", reflect.TypeFor[jujuparams.SerializedModel]()},
		{8, "Prechecks", reflect.TypeFor[jujuparams.SerializedModelV2]()},
		{8, "Import", reflect.TypeFor[jujuparams.SerializedModelV2]()},
	}
	for _, test := range paramsTypes {
		m, err := cr.FindMethod("MigrationTarget", test.version, test.method)
		c.Assert(err, qt.IsNil, qt.Commentf("v%d %s", test.version, test.method))
		c.Check(m.ParamsType(), qt.Equals, test.paramsType, qt.Commentf("v%d %s", test.version, test.method))
	}

	// The remaining methods are unchanged between v6 and v8.
	for _, method := range []string{"CACert", "Activate", "AdoptResources", "Abort", "CheckMachines", "LatestLogTime"} {
		v6, err := cr.FindMethod("MigrationTarget", 6, method)
		c.Assert(err, qt.IsNil, qt.Commentf("v6 %s", method))
		v8, err := cr.FindMethod("MigrationTarget", 8, method)
		c.Assert(err, qt.IsNil, qt.Commentf("v8 %s", method))
		c.Check(v8.ParamsType(), qt.Equals, v6.ParamsType(), qt.Commentf("%s", method))
	}

	_, err := cr.FindMethod("MigrationTarget", 7, "Prechecks")
	c.Assert(err, qt.ErrorMatches, `unknown method "Prechecks" at version 7 for facade type "MigrationTarget"`)
}
