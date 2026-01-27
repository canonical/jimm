// Copyright 2025 Canonical.

package cmd

import (
	"testing"

	qt "github.com/frankban/quicktest"
	"github.com/juju/cmd/v3/cmdtesting"
	"github.com/juju/juju/rpc/params"
	"go.uber.org/mock/gomock"
	"gopkg.in/yaml.v3"

	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
)

func TestModelStatusSuperuser(t *testing.T) {
	c := qt.New(t)
	s := setupCmdMocks(c)

	fullStatus := params.FullStatus{
		Model: params.ModelStatusInfo{
			CloudTag:    "cloud-" + jimmtest.TestCloudName,
			CloudRegion: jimmtest.TestCloudRegionName,
			ModelStatus: params.DetailedStatus{
				Data: map[string]any{},
			},
		},
		Machines:           map[string]params.MachineStatus{},
		Applications:       map[string]params.ApplicationStatus{},
		RemoteApplications: map[string]params.RemoteApplicationStatus{},
		Offers:             map[string]params.ApplicationOfferStatus{},
		Branches:           map[string]params.BranchStatus{},
		Storage:            []params.StorageDetails{},
		Filesystems:        []params.FilesystemDetails{},
		Volumes:            []params.VolumeDetails{},
		Relations:          []params.RelationStatus{},
	}

	s.client.EXPECT().FullModelStatus(gomock.Any()).
		Return(fullStatus, nil)
	s.client.EXPECT().Close()

	statusCmd := &modelStatusCommand{
		jimmAPIFunc: func() (JIMMAPI, error) {
			return s.client, nil
		},
	}
	initCommand(c, statusCmd, "2cb433a6-04eb-4ec4-9567-90426d20a004")

	ctx := newTestContext(c)
	err := statusCmd.Run(ctx)
	c.Assert(err, qt.IsNil)

	res := cmdtesting.Stdout(ctx)
	var gotStatus params.FullStatus
	err = yaml.Unmarshal([]byte(res), &gotStatus)
	c.Assert(err, qt.IsNil)

	c.Assert(gotStatus, qt.DeepEquals, fullStatus)
}
