// Copyright 2026 Canonical.

package upgrade_test

import (
	"context"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	qt "github.com/frankban/quicktest"
	jujuerrors "github.com/juju/errors"
	"github.com/juju/juju/api/base"
	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/environs/cloudspec"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v5"
	"github.com/juju/version/v2"
	"github.com/riverqueue/river/rivertype"
	"go.uber.org/mock/gomock"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/jimm/bootstrap"
	"github.com/canonical/jimm/v3/internal/jimm/upgrade"
	"github.com/canonical/jimm/v3/internal/jimm/upgrade/mocks"
	"github.com/canonical/jimm/v3/internal/jujuclient"
	"github.com/canonical/jimm/v3/internal/openfga"
	"github.com/canonical/jimm/v3/internal/rivertypes"
)

type upgradeManagerDeps struct {
	bootstrapManager *mocks.MockBootstrapManager
	jujuManager      *mocks.MockJujuManager
	store            *mocks.MockStore
	enqueuer         *mocks.MockUpgradeEnqueuer
	dialer           *mocks.MockDialer
	api              *mocks.MockAPI
}

func setupTest(t *testing.T) upgradeManagerDeps {
	s := upgradeManagerDeps{}
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	s.bootstrapManager = mocks.NewMockBootstrapManager(ctrl)
	s.jujuManager = mocks.NewMockJujuManager(ctrl)
	s.store = mocks.NewMockStore(ctrl)
	s.enqueuer = mocks.NewMockUpgradeEnqueuer(ctrl)
	s.dialer = mocks.NewMockDialer(ctrl)
	s.api = mocks.NewMockAPI(ctrl)

	return s
}

func TestNewUpgradeManager(t *testing.T) {
	s := setupTest(t)
	c := qt.New(t)

	_, err := upgrade.NewUpgradeManager(s.bootstrapManager, s.jujuManager, s.store, s.dialer, s.enqueuer)
	c.Assert(err, qt.IsNil)
}

func TestNewUpgradeManager_InvalidParams(t *testing.T) {
	c := qt.New(t)

	_, err := upgrade.NewUpgradeManager(nil, nil, nil, nil, nil)
	c.Assert(err, qt.ErrorMatches, "bootstrap manager cannot be nil")
}

func TestPrepareUpgradeTo_RejectsCurrentVersionNewerThanTarget(t *testing.T) {
	s := setupTest(t)
	c := qt.New(t)

	ctx := c.Context()

	upgradeMgr, err := upgrade.NewUpgradeManager(s.bootstrapManager, s.jujuManager, s.store, s.dialer, s.enqueuer)
	c.Assert(err, qt.IsNil)

	modelUUID := "93608db4-f1cb-4da5-9926-8233981aef0a"
	targetVersion, err := version.Parse("2.9.0")
	c.Assert(err, qt.IsNil)

	s.jujuManager.EXPECT().
		GetModel(gomock.Any(), modelUUID).
		Return(dbmodel.Model{
			Controller: dbmodel.Controller{
				AgentVersion: "3.0.0", // Current version is newer than target
			},
		}, nil)

	_, _, _, err = upgradeMgr.PrepareUpgradeTo(ctx, modelUUID, targetVersion)
	c.Assert(err, qt.ErrorMatches, ".*target version must be greater than or equal to current version.*")
}

func TestPrepareUpgradeTo_Success(t *testing.T) {
	s := setupTest(t)
	c := qt.New(t)

	ctx := c.Context()

	upgradeMgr, err := upgrade.NewUpgradeManager(s.bootstrapManager, s.jujuManager, s.store, s.dialer, s.enqueuer)
	c.Assert(err, qt.IsNil)

	modelUUID := "93608db4-f1cb-4da5-9926-8233981aef0a"
	targetVersion, err := version.Parse("4.1.0")
	c.Assert(err, qt.IsNil)

	s.jujuManager.EXPECT().
		GetModel(gomock.Any(), modelUUID).
		Return(dbmodel.Model{
			Controller: dbmodel.Controller{
				AgentVersion: "3.6.9",
			},
		},
			nil,
		)
	s.dialer.EXPECT().
		Dial(ctx, gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(s.api, nil)

	cloudCred := jujucloud.NewCredential(jujucloud.AccessKeyAuthType, map[string]string{
		"access-key": "AKIA...",
	})
	s.api.EXPECT().
		CloudSpec(gomock.Any()).
		DoAndReturn(func(ctx context.Context) (cloudspec.CloudSpec, error) {
			return cloudspec.CloudSpec{
				Name:       "aws",
				Credential: &cloudCred,
				Region:     "us-east-1",
			}, nil
		})

	s.api.EXPECT().
		Cloud(gomock.Any(), gomock.Any()).
		DoAndReturn(func(tag names.CloudTag, cloud *jujucloud.Cloud) error {
			*cloud = jujucloud.Cloud{
				IsControllerCloud: true,
			}
			return nil
		})

	ctrlCloud, ctrlCloudRegion, ctrlCredential, err := upgradeMgr.PrepareUpgradeTo(ctx, modelUUID, targetVersion)
	c.Assert(err, qt.IsNil)
	c.Assert(ctrlCloud.IsControllerCloud, qt.Equals, true)
	c.Assert(ctrlCredential.AuthType(), qt.Equals, jujucloud.AccessKeyAuthType)
	c.Assert(ctrlCredential.Attributes()["access-key"], qt.Equals, "AKIA...")
	c.Assert(ctrlCloudRegion, qt.Equals, "us-east-1")
}

func TestCloneController_Success(t *testing.T) {
	s := setupTest(t)
	c := qt.New(t)

	upgradeMgr, err := upgrade.NewUpgradeManager(s.bootstrapManager, s.jujuManager, s.store, s.dialer, s.enqueuer)
	c.Assert(err, qt.IsNil)

	jobId := int64(1)

	s.bootstrapManager.EXPECT().
		StartBootstrapJob(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(jobId, nil)

	s.bootstrapManager.EXPECT().
		WaitForJobCompletion(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil)

	err = upgradeMgr.CloneController(c.Context(), &openfga.User{}, upgrade.CloneControllerParams{})
	c.Assert(err, qt.IsNil)
}

func TestCloneController_Error(t *testing.T) {
	s := setupTest(t)
	c := qt.New(t)

	upgradeMgr, err := upgrade.NewUpgradeManager(s.bootstrapManager, s.jujuManager, s.store, s.dialer, s.enqueuer)
	c.Assert(err, qt.IsNil)

	errorToReturn := errors.New("bootstrap error")
	s.bootstrapManager.EXPECT().
		StartBootstrapJob(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(0, errorToReturn)

	err = upgradeMgr.CloneController(c.Context(), &openfga.User{}, upgrade.CloneControllerParams{})
	c.Assert(err, qt.ErrorMatches, ".*failed to start bootstrap job.*bootstrap error.*")
}

func TestCloneController_WaitForJobCompletionError(t *testing.T) {
	s := setupTest(t)
	c := qt.New(t)

	upgradeMgr, err := upgrade.NewUpgradeManager(s.bootstrapManager, s.jujuManager, s.store, s.dialer, s.enqueuer)
	c.Assert(err, qt.IsNil)

	jobId := int64(1)
	errorToReturn := errors.New("job failed")

	s.bootstrapManager.EXPECT().
		StartBootstrapJob(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(jobId, nil)

	s.bootstrapManager.EXPECT().
		WaitForJobCompletion(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(errorToReturn)

	err = upgradeMgr.CloneController(c.Context(), &openfga.User{}, upgrade.CloneControllerParams{})
	c.Assert(err, qt.ErrorMatches, ".*bootstrap job failed.*job failed.*")
}

func TestUpgradeTo_Success(t *testing.T) {
	s := setupTest(t)
	c := qt.New(t)

	ctx := c.Context()
	user := &openfga.User{
		Identity: &dbmodel.Identity{
			Name: "alice@canonical.com",
		},
	}
	modelUUID := "93608db4-f1cb-4da5-9926-8233981aef0a"
	targetVersion, err := version.Parse("4.2.0")
	c.Assert(err, qt.IsNil)

	upgradeMgr, err := upgrade.NewUpgradeManager(s.bootstrapManager, s.jujuManager, s.store, s.dialer, s.enqueuer)
	c.Assert(err, qt.IsNil)

	// PrepareUpgradeTo expectations.
	s.jujuManager.EXPECT().
		GetModel(gomock.Any(), modelUUID).
		Return(dbmodel.Model{
			Controller: dbmodel.Controller{
				AgentVersion: "4.1.0",
			},
		}, nil)

	s.dialer.EXPECT().
		Dial(ctx, gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(s.api, nil)

	s.api.EXPECT().
		CloudSpec(gomock.Any()).
		DoAndReturn(func(ctx context.Context) (cloudspec.CloudSpec, error) {
			return cloudspec.CloudSpec{
				Name:       "aws",
				Credential: &jujucloud.Credential{},
				Region:     "us-east-1",
			}, nil
		})

	s.api.EXPECT().
		Cloud(gomock.Any(), gomock.Any()).
		DoAndReturn(func(tag names.CloudTag, cloud *jujucloud.Cloud) error {
			*cloud = jujucloud.Cloud{
				Name:              "aws",
				IsControllerCloud: true,
			}
			return nil
		})

	// CloneController expectations.
	var newControllerName string
	s.bootstrapManager.EXPECT().
		StartBootstrapJob(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, u *openfga.User, params bootstrap.BootstrapParams) (int64, error) {
			newControllerName = params.ControllerName
			c.Assert(params.CLIVersion, qt.Equals, targetVersion.String())
			c.Assert(params.CloudNameAndRegion, qt.Equals, "aws/us-east-1")
			c.Assert(regexp.MustCompile(`^controller-\d+$`).MatchString(newControllerName), qt.IsTrue)
			c.Assert(params.Cloud.Name, qt.Equals, "aws")
			return 1, nil
		})

	s.bootstrapManager.EXPECT().
		WaitForJobCompletion(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil)

	// Migration expectations.
	s.enqueuer.EXPECT().EnqueueUpgradeTo(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, uta rivertypes.UpgradeToArgs) (*rivertype.JobInsertResult, error) {
		c.Check(uta.ModelUUID, qt.Equals, modelUUID)
		c.Check(uta.TargetVersion, qt.Equals, targetVersion)
		c.Check(uta.Username, qt.Equals, user.Name)
		// Don't check target controller name since that is currently generated based on the current time.
		return &rivertype.JobInsertResult{
			Job: &rivertype.JobRow{ID: 1},
		}, nil
	})

	jobID, err := upgradeMgr.UpgradeTo(ctx, user, modelUUID, targetVersion)
	c.Assert(err, qt.IsNil)
	c.Assert(jobID, qt.Equals, int64(1))

	parts := strings.Split(newControllerName, "-")
	c.Assert(len(parts), qt.Equals, 2)
	ts, convErr := strconv.ParseInt(parts[1], 10, 64)
	c.Assert(convErr, qt.IsNil)
	c.Assert(time.Since(time.Unix(ts, 0)) < 2*time.Second, qt.IsTrue)
}

func TestUpgradeTo_ValidatesTargetVersion(t *testing.T) {
	s := setupTest(t)
	c := qt.New(t)

	ctx := c.Context()
	user := &openfga.User{}
	modelUUID := "93608db4-f1cb-4da5-9926-8233981aef0a"
	targetVersion, err := version.Parse("0.0.0")
	c.Assert(err, qt.IsNil)

	upgradeMgr, err := upgrade.NewUpgradeManager(s.bootstrapManager, s.jujuManager, s.store, s.dialer, s.enqueuer)
	c.Assert(err, qt.IsNil)

	_, err = upgradeMgr.UpgradeTo(ctx, user, modelUUID, targetVersion)
	c.Assert(err, qt.ErrorMatches, ".*target version cannot be zero*")
}

func TestMigrateModel_Success(t *testing.T) {
	s := setupTest(t)
	c := qt.New(t)

	ctx := c.Context()

	upgradeMgr, err := upgrade.NewUpgradeManager(s.bootstrapManager, s.jujuManager, s.store, s.dialer, s.enqueuer)
	c.Assert(err, qt.IsNil)

	targetMt := names.NewModelTag("93608db4-f1cb-4da5-9926-8233981aef0a")
	targetController := "4.0controller"
	c.Assert(err, qt.IsNil)

	s.jujuManager.EXPECT().
		ModelInfo(
			gomock.Any(),
			gomock.Any(),
			targetMt,
		).
		Return(jujuclient.ModelInfo{
			ModelInfo: base.ModelInfo{
				UUID: targetMt.Id(),
			},
		}, nil)

	s.jujuManager.EXPECT().
		GetModel(gomock.Any(), targetMt.Id()).
		Return(
			dbmodel.Model{
				Controller: dbmodel.Controller{
					Name: "source controller",
				},
			},
			nil,
		)

	s.jujuManager.EXPECT().
		InitiateInternalMigration(gomock.Any(), gomock.Any(), targetMt.Id(), targetController).
		Return(
			jujuparams.InitiateMigrationResult{
				ModelTag:    targetMt.String(),
				MigrationId: "1",
			},
			nil,
		)

	s.jujuManager.EXPECT().
		ModelInfo(
			gomock.Any(),
			gomock.Any(),
			targetMt,
		).
		Return(jujuclient.ModelInfo{
			ModelInfo: base.ModelInfo{
				UUID: targetMt.Id(),
			},
		}, nil)

	s.jujuManager.EXPECT().
		GetModel(gomock.Any(), targetMt.Id()).
		Return(
			dbmodel.Model{
				Controller: dbmodel.Controller{
					Name: targetController,
				},
			},
			nil,
		)

	err = upgradeMgr.MigrateModel(ctx, &openfga.User{}, targetMt.Id(), targetController)
	c.Assert(err, qt.IsNil)
}

func TestMigrateModel_EndsEarly(t *testing.T) {
	s := setupTest(t)
	c := qt.New(t)

	ctx := c.Context()

	upgradeMgr, err := upgrade.NewUpgradeManager(s.bootstrapManager, s.jujuManager, s.store, s.dialer, s.enqueuer)
	c.Assert(err, qt.IsNil)

	targetMt := names.NewModelTag("93608db4-f1cb-4da5-9926-8233981aef0a")
	targetController := "4.0controller"

	s.jujuManager.EXPECT().InitiateInternalMigration(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(jujuparams.InitiateMigrationResult{}, nil)

	s.jujuManager.EXPECT().
		GetModel(gomock.Any(), targetMt.Id()).
		Return(
			dbmodel.Model{Controller: dbmodel.Controller{Name: "source controller"}},
			nil,
		)

	timeFailed := time.Date(2026, 2, 13, 13, 31, 2, 983062723, time.FixedZone("SAST", 2*60*60))
	expectedEndUTC := timeFailed.UTC().Format(time.RFC3339)
	gomock.InOrder(
		s.jujuManager.EXPECT().
			ModelInfo(gomock.Any(), gomock.Any(), targetMt).
			Return(jujuclient.ModelInfo{}, nil),
		s.jujuManager.EXPECT().
			ModelInfo(gomock.Any(), gomock.Any(), targetMt).
			Return(jujuclient.ModelInfo{
				ModelInfo: base.ModelInfo{
					UUID: targetMt.Id(),
				},
				MigrationStatus: &jujuclient.ModelMigrationStatus{
					Status: "some-status",
					End:    &timeFailed,
				},
			}, nil),
	)

	s.jujuManager.EXPECT().GetModel(gomock.Any(), targetMt.Id()).Return(
		dbmodel.Model{Controller: dbmodel.Controller{
			Name: "source controller",
		}}, nil,
	)

	err = upgradeMgr.MigrateModel(ctx, &openfga.User{}, targetMt.Id(), targetController)
	c.Assert(err, qt.ErrorMatches, ".*model migration failed: migration ended at "+regexp.QuoteMeta(expectedEndUTC)+" with status some-status")
}

func TestMigrateModel_Retries2Times(t *testing.T) {
	s := setupTest(t)
	c := qt.New(t)

	ctx := c.Context()

	upgradeMgr, err := upgrade.NewUpgradeManager(s.bootstrapManager, s.jujuManager, s.store, s.dialer, s.enqueuer)
	c.Assert(err, qt.IsNil)

	targetMt := names.NewModelTag("93608db4-f1cb-4da5-9926-8233981aef0a")
	targetController := "4.0controller"

	s.jujuManager.EXPECT().
		ModelInfo(
			gomock.Any(),
			gomock.Any(),
			targetMt,
		).
		Return(jujuclient.ModelInfo{
			ModelInfo: base.ModelInfo{
				UUID: targetMt.Id(),
			},
		}, nil)

	// Model is a different controller, so continues.
	s.jujuManager.EXPECT().
		GetModel(gomock.Any(), targetMt.Id()).
		Return(
			dbmodel.Model{Controller: dbmodel.Controller{Name: "source controller"}},
			nil,
		)

	s.jujuManager.EXPECT().
		InitiateInternalMigration(gomock.Any(), gomock.Any(), targetMt.Id(), targetController).
		Return(
			jujuparams.InitiateMigrationResult{ModelTag: targetMt.String(), MigrationId: "1"},
			nil,
		)

	// Expect 3 because we're going to retry twice before succeeding.
	s.jujuManager.EXPECT().
		ModelInfo(gomock.Any(), gomock.Any(), targetMt).
		Return(jujuclient.ModelInfo{ModelInfo: base.ModelInfo{UUID: targetMt.Id()}}, nil).
		Times(3)

	// Retry 3 times.
	getModelCalls := 0
	s.jujuManager.EXPECT().
		GetModel(gomock.Any(), targetMt.Id()).
		DoAndReturn(func(ctx context.Context, uuid string) (dbmodel.Model, error) {
			getModelCalls++
			if getModelCalls < 3 {
				return dbmodel.Model{Controller: dbmodel.Controller{Name: "still-source"}}, nil
			}
			return dbmodel.Model{Controller: dbmodel.Controller{Name: targetController}}, nil
		}).
		Times(3)

	synctest.Test(t, func(t *testing.T) {
		err = upgradeMgr.MigrateModel(ctx, &openfga.User{}, targetMt.Id(), targetController)
		c.Assert(err, qt.IsNil)
	})

	// 3 suggests we succeed on the third try.
	c.Assert(getModelCalls, qt.Equals, 3)
}

func TestMigrateModel_IdempotencyWhenModelHasAlreadyBeenMigrated(t *testing.T) {
	s := setupTest(t)
	c := qt.New(t)

	ctx := c.Context()
	upgradeMgr, err := upgrade.NewUpgradeManager(s.bootstrapManager, s.jujuManager, s.store, s.dialer, s.enqueuer)
	c.Assert(err, qt.IsNil)

	targetMt := names.NewModelTag("93608db4-f1cb-4da5-9926-8233981aef0a")
	targetController := "4.0controller"

	s.jujuManager.EXPECT().
		ModelInfo(
			gomock.Any(),
			gomock.Any(),
			targetMt,
		).
		Return(jujuclient.ModelInfo{
			ModelInfo: base.ModelInfo{
				UUID: targetMt.Id(),
			},
		}, nil)

	// Model is already on the target controller, so migration is a no-op.
	s.jujuManager.EXPECT().
		GetModel(gomock.Any(), targetMt.Id()).
		Return(
			dbmodel.Model{Controller: dbmodel.Controller{Name: targetController}},
			nil,
		)

	err = upgradeMgr.MigrateModel(ctx, &openfga.User{}, targetMt.Id(), targetController)
	c.Assert(err, qt.IsNil)
}

func TestUpgradeModel_RejectsZeroTargetVersion(t *testing.T) {
	s := setupTest(t)
	c := qt.New(t)

	upgradeMgr, err := upgrade.NewUpgradeManager(
		s.bootstrapManager,
		s.jujuManager,
		s.store,
		s.dialer,
		s.enqueuer,
	)
	c.Assert(err, qt.IsNil)

	err = upgradeMgr.UpgradeModel(c.Context(), "some-model-uuid", version.Zero)
	c.Assert(err, qt.ErrorMatches, ".*target version cannot be zero.*")
}

func TestUpgradeModel_ModelNotFound(t *testing.T) {
	s := setupTest(t)
	c := qt.New(t)

	ctx := c.Context()
	modelUUID := "93608db4-f1cb-4da5-9926-8233981aef0a"
	targetVersion, err := version.Parse("4.1.0")
	c.Assert(err, qt.IsNil)

	upgradeMgr, err := upgrade.NewUpgradeManager(
		s.bootstrapManager,
		s.jujuManager,
		s.store,
		s.dialer,
		s.enqueuer,
	)
	c.Assert(err, qt.IsNil)

	s.store.EXPECT().GetModel(gomock.Any(), gomock.Any()).Return(errors.New("db error"))

	err = upgradeMgr.UpgradeModel(ctx, modelUUID, targetVersion)
	c.Assert(err, qt.ErrorMatches, ".*model not found.*")
}

func TestUpgradeModel_AlreadyAtTargetDoesNotCallUpgrade(t *testing.T) {
	s := setupTest(t)
	c := qt.New(t)

	ctx := c.Context()
	modelUUID := "93608db4-f1cb-4da5-9926-8233981aef0a"
	targetVersion, err := version.Parse("4.1.0")
	c.Assert(err, qt.IsNil)

	upgradeMgr, err := upgrade.NewUpgradeManager(
		s.bootstrapManager,
		s.jujuManager,
		s.store,
		s.dialer,
		s.enqueuer,
	)
	c.Assert(err, qt.IsNil)

	s.store.EXPECT().GetModel(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, model *dbmodel.Model) error {
		model.Controller = dbmodel.Controller{Name: "ctrl"}
		return nil
	})

	s.dialer.EXPECT().
		Dial(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(s.api, nil)

	s.api.EXPECT().ModelInfo(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, mt names.ModelTag) (jujuclient.ModelInfo, error) {
		return jujuclient.ModelInfo{
			ModelInfo: base.ModelInfo{
				UUID:         modelUUID,
				AgentVersion: &targetVersion,
			},
		}, nil
	})

	err = upgradeMgr.UpgradeModel(ctx, modelUUID, targetVersion)
	c.Assert(err, qt.IsNil)
}

func TestUpgradeModel_RetriesUntilModelReportsTargetVersion(t *testing.T) {
	s := setupTest(t)
	c := qt.New(t)

	ctx := c.Context()
	modelUUID := "93608db4-f1cb-4da5-9926-8233981aef0a"
	targetVersion, err := version.Parse("4.1.0")
	c.Assert(err, qt.IsNil)
	oldVersion, err := version.Parse("4.0.0")
	c.Assert(err, qt.IsNil)

	upgradeMgr, err := upgrade.NewUpgradeManager(
		s.bootstrapManager,
		s.jujuManager,
		s.store,
		s.dialer,
		s.enqueuer,
	)
	c.Assert(err, qt.IsNil)

	s.store.EXPECT().GetModel(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, model *dbmodel.Model) error {
		model.Controller = dbmodel.Controller{Name: "ctrl"}
		return nil
	})

	s.dialer.EXPECT().
		Dial(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(s.api, nil)

	modelInfoCalls := 0
	s.api.EXPECT().ModelInfo(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, mt names.ModelTag) (jujuclient.ModelInfo, error) {
		c.Check(mt.Id(), qt.Equals, modelUUID)
		mi := jujuclient.ModelInfo{}
		modelInfoCalls++
		if modelInfoCalls == 1 {
			v := oldVersion
			mi.AgentVersion = &v
			return mi, nil
		}
		v := targetVersion
		mi.AgentVersion = &v
		return mi, nil
	}).Times(2)

	s.api.EXPECT().UpgradeModel(modelUUID, targetVersion, "", false, false).Return(targetVersion, nil)

	// Wrap the test in a synctest so that time advances instantly.
	synctest.Test(t, (func(t *testing.T) {
		err := upgradeMgr.UpgradeModel(ctx, modelUUID, targetVersion)
		c.Assert(err, qt.IsNil)
	}))
}

// TestUpgradeModel_AlreadyUpgraded tests that if the UpgradeModel API call
// returns an AlreadyExists error, the UpgradeModel method treats this as
// a successful upgrade (idempotency).
func TestUpgradeModel_AlreadyUpgraded(t *testing.T) {
	s := setupTest(t)
	c := qt.New(t)

	ctx := c.Context()
	modelUUID := "93608db4-f1cb-4da5-9926-8233981aef0a"
	targetVersion, err := version.Parse("4.1.0")
	c.Assert(err, qt.IsNil)
	oldVersion, err := version.Parse("4.0.0")
	c.Assert(err, qt.IsNil)

	upgradeMgr, err := upgrade.NewUpgradeManager(
		s.bootstrapManager,
		s.jujuManager,
		s.store,
		s.dialer,
		s.enqueuer,
	)
	c.Assert(err, qt.IsNil)

	s.store.EXPECT().GetModel(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, model *dbmodel.Model) error {
		model.Controller = dbmodel.Controller{Name: "ctrl"}
		return nil
	})

	s.dialer.EXPECT().
		Dial(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(s.api, nil)

	s.api.EXPECT().ModelInfo(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, mt names.ModelTag) (jujuclient.ModelInfo, error) {
		return jujuclient.ModelInfo{
			ModelInfo: base.ModelInfo{
				UUID:         modelUUID,
				AgentVersion: &oldVersion,
			},
		}, nil
	})

	s.api.EXPECT().UpgradeModel(modelUUID, targetVersion, "", false, false).Return(version.Number{}, jujuerrors.AlreadyExists)

	err = upgradeMgr.UpgradeModel(ctx, modelUUID, targetVersion)
	c.Assert(err, qt.IsNil)
}

//go:generate go tool mockgen -typed -destination=./mocks/bootstrapmanager.go -package=mocks . BootstrapManager
//go:generate go tool mockgen -typed -destination=./mocks/jujumanager.go -package=mocks . JujuManager
//go:generate go tool mockgen -typed -destination=./mocks/store.go -package=mocks . Store
//go:generate go tool mockgen -typed -destination=./mocks/enqueuer.go -package=mocks . UpgradeEnqueuer
//go:generate go tool mockgen -typed -destination=./mocks/dialer.go -package=mocks github.com/canonical/jimm/v3/internal/jimm/juju Dialer
//go:generate go tool mockgen -typed -destination=./mocks/api.go -package=mocks github.com/canonical/jimm/v3/internal/jimm/juju API
