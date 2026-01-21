// Copyright 2025 Canonical.

package cmd

import (
	"os"
	"path/filepath"
	"testing"

	qt "github.com/frankban/quicktest"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/rpc/params"

	jimmjujuapi "github.com/canonical/jimm/v3/internal/jujuapi"
	apiparams "github.com/canonical/jimm/v3/pkg/api/params"
)

func TestAddCloudToControllerRun(t *testing.T) {
	c := qt.New(t)

	force := false

	cmdMocks := setupCmdMocks(t)

	expectedCloud := &cloud.Cloud{
		Name:            "test-hosted-cloud",
		Type:            "kubernetes",
		AuthTypes:       []cloud.AuthType{"certificate"},
		HostCloudRegion: "kubernetes/default",
		Regions:         []cloud.Region{{Name: cloud.DefaultCloudRegion}}, // Verify DefaultCloudRegion is set. It's all this test can do really.
	}

	cmdMocks.client.EXPECT().Close().Times(1)
	cmdMocks.client.EXPECT().AddCloudToController(&apiparams.AddCloudToControllerRequest{
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
		jimmAPIFunc: func() (JIMMAPI, error) {
			return cmdMocks.client, nil
		},
	}

	err := cmd.Run(newTestContext(t))
	c.Assert(err, qt.IsNil)
}

func TestAddCloudToControllerRun_Run_CloudFromFile(t *testing.T) {
	c := qt.New(t)

	writeTempFile := func(c *qt.C, content string) (string, func()) {
		dir, err := os.MkdirTemp("", "add-cloud-to-controller-test")
		c.Assert(err, qt.IsNil)

		tmpfn := filepath.Join(dir, "tmp.yaml")

		err = os.WriteFile(tmpfn, []byte(content), 0600)
		c.Assert(err, qt.IsNil)
		return tmpfn, func() {
			os.RemoveAll(dir)
		}
	}

	cloudFile, cleanup := writeTempFile(c, `
clouds:
  test-maas-cloud:
    type: maas
    auth-types: [oauth1]
    regions:
      default: {}`)

	c.Cleanup(cleanup)

	cmdMocks := setupCmdMocks(t)

	cmdMocks.client.EXPECT().Close().Times(1)

	// Expect args to be the cloud read from file.
	expectedCloud := &cloud.Cloud{
		Name:      "test-maas-cloud",
		Type:      "maas",
		AuthTypes: []cloud.AuthType{"oauth1"},
		Regions:   []cloud.Region{{Name: "default"}},
	}

	force := false

	cmdMocks.client.EXPECT().AddCloudToController(&apiparams.AddCloudToControllerRequest{
		ControllerName: "",
		AddCloudArgs: params.AddCloudArgs{
			Name:  "test-maas-cloud",
			Cloud: jimmjujuapi.CloudToParams(*expectedCloud),
			Force: &force,
		},
	}).Return(nil).Times(1)

	cmd := addCloudToControllerCommand{
		cloudName:           "test-maas-cloud",
		cloudDefinitionFile: cloudFile,
		jimmAPIFunc: func() (JIMMAPI, error) {
			return cmdMocks.client, nil
		},
	}

	err := cmd.Run(newTestContext(t))
	c.Assert(err, qt.IsNil)
}
