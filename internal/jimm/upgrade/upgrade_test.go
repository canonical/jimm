// Copyright 2025 Canonical.

package upgrade_test

import (
	"context"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
	"github.com/frankban/quicktest/qtsuite"
	jujucloud "github.com/juju/juju/cloud"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v5"
	"github.com/juju/version/v2"
	"go.uber.org/mock/gomock"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/jimm/bootstrap"
	"github.com/canonical/jimm/v3/internal/jimm/upgrade"
	"github.com/canonical/jimm/v3/internal/jimm/upgrade/mocks"
	"github.com/canonical/jimm/v3/internal/openfga"
)

type upgradeManagerSuite struct {
	bootstrapManager *mocks.MockBootstrapManager
	jujuManager      *mocks.MockJujuManager
	store            *mocks.MockStore
	dialer           *mocks.MockDialer
	api              *mocks.MockAPI
}

func (s *upgradeManagerSuite) setupTest(c *qt.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.bootstrapManager = mocks.NewMockBootstrapManager(ctrl)
	s.jujuManager = mocks.NewMockJujuManager(ctrl)
	s.store = mocks.NewMockStore(ctrl)
	s.dialer = mocks.NewMockDialer(ctrl)
	s.api = mocks.NewMockAPI(ctrl)

	return ctrl
}

func (s *upgradeManagerSuite) TestNewUpgradeManager(c *qt.C) {
	defer s.setupTest(c)

	_, err := upgrade.NewUpgradeManager(s.bootstrapManager, s.jujuManager, s.store, s.dialer)
	c.Assert(err, qt.IsNil)
}

func (s *upgradeManagerSuite) TestNewUpgradeManager_InvalidParams(c *qt.C) {
	ctrl := s.setupTest(c)
	defer ctrl.Finish()

	_, err := upgrade.NewUpgradeManager(nil, nil, nil, nil)
	c.Assert(err, qt.ErrorMatches, "bootstrap manager cannot be nil")
}

func (s *upgradeManagerSuite) TestPrepareUpgradeTo_RejectsCurrentVersionNewerThanTarget(c *qt.C) {
	ctrl := s.setupTest(c)
	defer ctrl.Finish()

	ctx := c.Context()

	upgradeMgr, err := upgrade.NewUpgradeManager(s.bootstrapManager, s.jujuManager, s.store, s.dialer)
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

func (s *upgradeManagerSuite) TestPrepareUpgradeTo_Success(c *qt.C) {
	ctrl := s.setupTest(c)
	defer ctrl.Finish()

	ctx := c.Context()

	upgradeMgr, err := upgrade.NewUpgradeManager(s.bootstrapManager, s.jujuManager, s.store, s.dialer)
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

	s.api.EXPECT().
		ControllerModelSummary(ctx, gomock.Any()).
		DoAndReturn(func(ctx context.Context, modelSummary *jujuparams.ModelSummary) error {
			// Mutate the pointer argument to simulate controller response
			*modelSummary = jujuparams.ModelSummary{
				CloudTag:           "cloud-aws",
				CloudCredentialTag: "cloudcred-aws_alice_mycredential",
				CloudRegion:        "us-east-1",
			}
			return nil
		})

	s.api.EXPECT().
		CredentialContents("aws", "mycredential", true).
		DoAndReturn(func(cloud string, credential string, withSecrets bool) ([]jujuparams.CredentialContentResult, error) {
			return []jujuparams.CredentialContentResult{
				{
					Result: &jujuparams.ControllerCredentialInfo{
						Content: jujuparams.CredentialContent{
							Name:     "mycredential",
							Cloud:    "aws",
							AuthType: string(jujucloud.AccessKeyAuthType),
							Attributes: map[string]string{
								"access-key": "AKIA...",
							},
						},
					},
				},
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

func (s *upgradeManagerSuite) TestCloneController_Success(c *qt.C) {
	ctrl := s.setupTest(c)
	defer ctrl.Finish()

	upgradeMgr, err := upgrade.NewUpgradeManager(s.bootstrapManager, s.jujuManager, s.store, s.dialer)
	c.Assert(err, qt.IsNil)

	jobId := "550e8400-e29b-41d4-a716-446655440000"

	s.bootstrapManager.EXPECT().
		StartBootstrapJob(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(jobId, nil)

	s.bootstrapManager.EXPECT().
		WaitForJobCompletion(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil)

	err = upgradeMgr.CloneController(c.Context(), &openfga.User{}, upgrade.CloneControllerParams{})
	c.Assert(err, qt.IsNil)
}

func (s *upgradeManagerSuite) TestCloneController_Error(c *qt.C) {
	ctrl := s.setupTest(c)
	defer ctrl.Finish()

	upgradeMgr, err := upgrade.NewUpgradeManager(s.bootstrapManager, s.jujuManager, s.store, s.dialer)
	c.Assert(err, qt.IsNil)

	errorToReturn := errors.New("bootstrap error")
	s.bootstrapManager.EXPECT().
		StartBootstrapJob(gomock.Any(), gomock.Any(), gomock.Any()).
		Return("", errorToReturn)

	err = upgradeMgr.CloneController(c.Context(), &openfga.User{}, upgrade.CloneControllerParams{})
	c.Assert(err, qt.ErrorMatches, ".*failed to start bootstrap job.*bootstrap error.*")
}

func (s *upgradeManagerSuite) TestCloneController_WaitForJobCompletionError(c *qt.C) {
	ctrl := s.setupTest(c)
	defer ctrl.Finish()

	upgradeMgr, err := upgrade.NewUpgradeManager(s.bootstrapManager, s.jujuManager, s.store, s.dialer)
	c.Assert(err, qt.IsNil)

	jobId := "550e8400-e29b-41d4-a716-446655440000"
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

func (s *upgradeManagerSuite) TestMigrateAndUpgradeModel_Success(c *qt.C) {
	ctrl := s.setupTest(c)
	defer ctrl.Finish()

	ctx := c.Context()

	upgradeMgr, err := upgrade.NewUpgradeManager(s.bootstrapManager, s.jujuManager, s.store, s.dialer)
	c.Assert(err, qt.IsNil)

	mt := names.NewModelTag("93608db4-f1cb-4da5-9926-8233981aef0a")
	targetController := "4.0controller"
	targetModelVersion, err := version.Parse("4.1.0")
	c.Assert(err, qt.IsNil)

	s.jujuManager.EXPECT().
		InitiateInternalMigration(ctx, gomock.Any(), mt.Id(), targetController).
		Return(
			jujuparams.InitiateMigrationResult{
				ModelTag:    mt.String(),
				MigrationId: "1",
			},
			nil,
		)

	s.jujuManager.EXPECT().
		ModelInfo(
			gomock.Any(),
			gomock.Any(),
			mt,
		).
		Return(&jujuparams.ModelInfo{
			UUID: mt.Id(),
		}, nil)

	s.jujuManager.EXPECT().
		GetModel(gomock.Any(), mt.Id()).
		Return(
			dbmodel.Model{
				Controller: dbmodel.Controller{
					Name: targetController,
				},
			},
			nil,
		)

	s.store.EXPECT().
		GetController(ctx, gomock.Any()).
		DoAndReturn(func(ctx context.Context, ctrl *dbmodel.Controller) error {
			*ctrl = dbmodel.Controller{
				Name:         targetController,
				AgentVersion: "4.1.0",
			}
			return nil
		})

	s.dialer.EXPECT().Dial(ctx, gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(s.api, nil)

	s.api.EXPECT().
		UpgradeModel(mt.Id(), targetModelVersion, "", false, false).
		Return(targetModelVersion, nil)

	controllerChosenVersion, err := upgradeMgr.MigrateAndUpgradeModel(ctx, &openfga.User{}, mt.Id(), targetController, targetModelVersion)
	c.Assert(err, qt.IsNil)
	c.Assert(controllerChosenVersion, qt.Equals, targetModelVersion)
}

func (s *upgradeManagerSuite) TestUpgradeTo_Success(c *qt.C) {
	ctrl := s.setupTest(c)
	defer ctrl.Finish()

	ctx := c.Context()
	user := &openfga.User{}
	modelUUID := "93608db4-f1cb-4da5-9926-8233981aef0a"
	targetVersion, err := version.Parse("4.2.0")
	c.Assert(err, qt.IsNil)

	upgradeMgr, err := upgrade.NewUpgradeManager(s.bootstrapManager, s.jujuManager, s.store, s.dialer)
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
		ControllerModelSummary(ctx, gomock.Any()).
		DoAndReturn(func(ctx context.Context, ms *jujuparams.ModelSummary) error {
			*ms = jujuparams.ModelSummary{
				CloudTag:           "cloud-aws",
				CloudCredentialTag: "cloudcred-aws_alice_mycredential",
				CloudRegion:        "us-east-1",
			}
			return nil
		})

	s.api.EXPECT().
		CredentialContents("aws", "mycredential", true).
		DoAndReturn(func(cloud, credential string, withSecrets bool) ([]jujuparams.CredentialContentResult, error) {
			return []jujuparams.CredentialContentResult{
				{
					Result: &jujuparams.ControllerCredentialInfo{
						Content: jujuparams.CredentialContent{
							Name:     "mycredential",
							Cloud:    "aws",
							AuthType: string(jujucloud.AccessKeyAuthType),
							Attributes: map[string]string{
								"access-key": "AKIA...",
							},
						},
					},
				},
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
		DoAndReturn(func(ctx context.Context, u *openfga.User, params bootstrap.BootstrapParams) (string, error) {
			newControllerName = params.ControllerName
			c.Assert(params.CLIVersion, qt.Equals, targetVersion.String())
			c.Assert(params.CloudNameAndRegion, qt.Equals, "aws/us-east-1")
			c.Assert(regexp.MustCompile(`^controller-\d+$`).MatchString(newControllerName), qt.IsTrue)
			c.Assert(params.PersonalCloud.Name, qt.Equals, "aws")
			return "550e8400-e29b-41d4-a716-446655440000", nil
		})

	s.bootstrapManager.EXPECT().
		WaitForJobCompletion(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil)

	// Migration expectations.
	s.jujuManager.EXPECT().
		InitiateInternalMigration(ctx, user, modelUUID, gomock.Any()).
		DoAndReturn(func(ctx context.Context, u *openfga.User, modelNameOrUUID string, targetController string) (jujuparams.InitiateMigrationResult, error) {
			c.Assert(targetController, qt.Equals, newControllerName)
			mt := names.NewModelTag(modelUUID)
			return jujuparams.InitiateMigrationResult{
				ModelTag:    mt.String(),
				MigrationId: "1",
			}, nil
		})

	s.jujuManager.EXPECT().
		ModelInfo(gomock.Any(), gomock.Any(), gomock.Any()).
		AnyTimes().
		DoAndReturn(func(ctx context.Context, u *openfga.User, mt names.ModelTag) (*jujuparams.ModelInfo, error) {
			c.Assert(mt.Id(), qt.Equals, modelUUID)
			return &jujuparams.ModelInfo{UUID: modelUUID}, nil
		})

	s.jujuManager.EXPECT().
		GetModel(gomock.Any(), modelUUID).
		AnyTimes().
		DoAndReturn(func(ctx context.Context, uuid string) (dbmodel.Model, error) {
			// Return the current value of newControllerName so that once it is set
			// by the StartBootstrapJob expectation, subsequent migration retry
			// loop iterations see the migrated controller name and succeed.
			return dbmodel.Model{
				Controller: dbmodel.Controller{
					Name: newControllerName,
				},
			}, nil
		})

	s.store.EXPECT().
		GetController(ctx, gomock.Any()).
		DoAndReturn(func(ctx context.Context, ctrl *dbmodel.Controller) error {
			c.Assert(ctrl.Name, qt.Equals, newControllerName)
			ctrl.AgentVersion = targetVersion.String()
			return nil
		})

	s.dialer.EXPECT().
		Dial(ctx, gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(s.api, nil)

	s.api.EXPECT().
		UpgradeModel(modelUUID, targetVersion, "", false, false).
		Return(targetVersion, nil)

	chosenVersion, err := upgradeMgr.UpgradeTo(ctx, user, modelUUID, targetVersion)
	c.Assert(err, qt.IsNil)
	c.Assert(chosenVersion, qt.Equals, targetVersion)

	parts := strings.Split(newControllerName, "-")
	c.Assert(len(parts), qt.Equals, 2)
	ts, convErr := strconv.ParseInt(parts[1], 10, 64)
	c.Assert(convErr, qt.IsNil)
	c.Assert(time.Since(time.Unix(ts, 0)) < 2*time.Second, qt.IsTrue)
}

//go:generate go tool mockgen -typed -destination=./mocks/bootstrapmanager.go -package=mocks . BootstrapManager
//go:generate go tool mockgen -typed -destination=./mocks/jujumanager.go -package=mocks . JujuManager
//go:generate go tool mockgen -typed -destination=./mocks/store.go -package=mocks . Store
//go:generate go tool mockgen -typed -destination=./mocks/dialer.go -package=mocks github.com/canonical/jimm/v3/internal/jimm/juju Dialer
//go:generate go tool mockgen -typed -destination=./mocks/api.go -package=mocks github.com/canonical/jimm/v3/internal/jimm/juju API
func TestUpgradeManager(t *testing.T) {
	qtsuite.Run(qt.New(t), &upgradeManagerSuite{})
}
