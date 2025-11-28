// Copyright 2025 Canonical.

package upgrade_test

import (
	"context"
	"errors"
	"testing"

	qt "github.com/frankban/quicktest"
	"github.com/frankban/quicktest/qtsuite"
	jujucloud "github.com/juju/juju/cloud"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v5"
	"github.com/juju/version/v2"
	"go.uber.org/mock/gomock"

	"github.com/canonical/jimm/v3/internal/dbmodel"
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

	_, _, err = upgradeMgr.PrepareUpgradeTo(ctx, modelUUID, targetVersion)
	c.Assert(err, qt.ErrorMatches, ".*target version must be greater than current version.*")
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
			}
			return nil
		})

	s.api.EXPECT().
		CredentialContents("aws", "aws/alice/mycredential", true).
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

	ctrlCloud, ctrlCredential, err := upgradeMgr.PrepareUpgradeTo(ctx, modelUUID, targetVersion)
	c.Assert(err, qt.IsNil)
	c.Assert(ctrlCloud.IsControllerCloud, qt.Equals, true)
	c.Assert(ctrlCredential.AuthType(), qt.Equals, jujucloud.AccessKeyAuthType)
	c.Assert(ctrlCredential.Attributes()["access-key"], qt.Equals, "AKIA...")
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

//go:generate mockgen -typed -destination=./mocks/bootstrapmanager.go -package=mocks . BootstrapManager
//go:generate mockgen -typed -destination=./mocks/jujumanager.go -package=mocks . JujuManager
//go:generate mockgen -typed -destination=./mocks/store.go -package=mocks . Store
//go:generate mockgen -typed -destination=./mocks/dialer.go -package=mocks github.com/canonical/jimm/v3/internal/jimm/juju Dialer
//go:generate mockgen -typed -destination=./mocks/api.go -package=mocks github.com/canonical/jimm/v3/internal/jimm/juju API
func TestUpgradeManager(t *testing.T) {
	qtsuite.Run(qt.New(t), &upgradeManagerSuite{})
}
