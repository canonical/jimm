// Copyright 2025 Canonical.

package cmd

import (
	"bytes"
	"testing"

	qt "github.com/frankban/quicktest"
	jujucmd "github.com/juju/cmd/v3"
	"github.com/juju/juju/api"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/rpc/params"
	"go.uber.org/mock/gomock"

	"github.com/canonical/jimm/v3/cmd/jaas/cmd/mocks"
	jimmjujuapi "github.com/canonical/jimm/v3/internal/jujuapi"
	apiparams "github.com/canonical/jimm/v3/pkg/api/params"
)

// Replace with utils in crossmodelquery pr prior to merging.
func setupMocks(t *testing.T) *mocks.MockJIMMAPI {
	ctrl := gomock.NewController(t)
	jimmAPI := mocks.NewMockJIMMAPI(ctrl)

	t.Cleanup(ctrl.Finish)

	return jimmAPI
}

// Replace with utils in crossmodelquery pr prior to merging.
func newTestContext(t *testing.T) *jujucmd.Context {
	return &jujucmd.Context{
		Context: t.Context(),
		Dir:     t.TempDir(),
		Stdin:   &bytes.Buffer{},
		Stdout:  &bytes.Buffer{},
		Stderr:  &bytes.Buffer{},
	}
}
func TestAddCloudToControllerRun(t *testing.T) {
	c := qt.New(t)

	force := false
	jimmAPIMock := setupMocks(t)

	expectedCloud := &cloud.Cloud{
		Name:            "test-hosted-cloud",
		Type:            "kubernetes",
		AuthTypes:       []cloud.AuthType{"certificate"},
		HostCloudRegion: "kubernetes/default",
		Regions:         []cloud.Region{{Name: cloud.DefaultCloudRegion}}, // Verify DefaultCloudRegion is set. It's all this test can do really.
	}

	jimmAPIMock.EXPECT().Close().Times(1)
	jimmAPIMock.EXPECT().AddCloudToController(&apiparams.AddCloudToControllerRequest{
		ControllerName: "",
		AddCloudArgs: params.AddCloudArgs{
			Name:  "",
			Cloud: jimmjujuapi.CloudToParams(*expectedCloud),
			Force: &force,
		},
	}).Return(nil).Times(1)

	cmd := addCloudToControllerCommand{
		cloudByNameFunc: func(cloudName string) (*cloud.Cloud, error) {
			return expectedCloud, nil
		},
		jimmAPIFunc: func(dialOpts *api.DialOpts) (JIMMAPI, error) {
			return jimmAPIMock, nil
		},
	}

	err := cmd.Run(newTestContext(t))
	c.Assert(err, qt.IsNil)
}
