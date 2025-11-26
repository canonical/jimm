// Copyright 2025 Canonical.

package upgrade_test

import (
	"errors"
	"testing"

	qt "github.com/frankban/quicktest"
	"github.com/frankban/quicktest/qtsuite"
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
}

func (s *upgradeManagerSuite) setupTest(c *qt.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.bootstrapManager = mocks.NewMockBootstrapManager(ctrl)
	s.jujuManager = mocks.NewMockJujuManager(ctrl)
	return ctrl
}

func (s *upgradeManagerSuite) TestNewUpgradeManager(c *qt.C) {
	defer s.setupTest(c)

	_, err := upgrade.NewUpgradeManager(s.bootstrapManager, s.jujuManager)
	c.Assert(err, qt.IsNil)
}

func (s *upgradeManagerSuite) TestNewUpgradeManager_InvalidParams(c *qt.C) {
	ctrl := s.setupTest(c)
	defer ctrl.Finish()

	_, err := upgrade.NewUpgradeManager(nil, nil)
	c.Assert(err, qt.ErrorMatches, "bootstrap manager cannot be nil")
}

func (s *upgradeManagerSuite) TestPrepareUpgradeTo_RejectsCurrentVersionNewerThanTarget(c *qt.C) {
	ctrl := s.setupTest(c)
	defer ctrl.Finish()

	ctx := c.Context()

	upgradeMgr, err := upgrade.NewUpgradeManager(s.bootstrapManager, s.jujuManager)
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

func (s *upgradeManagerSuite) TestCloneController_Success(c *qt.C) {
	ctrl := s.setupTest(c)
	defer ctrl.Finish()

	upgradeMgr, err := upgrade.NewUpgradeManager(s.bootstrapManager, s.jujuManager)
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

	upgradeMgr, err := upgrade.NewUpgradeManager(s.bootstrapManager, s.jujuManager)
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

	upgradeMgr, err := upgrade.NewUpgradeManager(s.bootstrapManager, s.jujuManager)
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
func TestUpgradeManager(t *testing.T) {
	qtsuite.Run(qt.New(t), &upgradeManagerSuite{})
}
