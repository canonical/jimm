// Copyright 2024 Canonical.
package cmd_test

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/juju/cmd/v3/cmdtesting"
	"github.com/juju/juju/cloud"
	"github.com/juju/names/v5"
	gc "gopkg.in/check.v1"

	"github.com/canonical/jimm/v3/cmd/jimmctl/cmd"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/openfga"
	ofganames "github.com/canonical/jimm/v3/internal/openfga/names"
	"github.com/canonical/jimm/v3/internal/testutils/cmdtest"
)

type addCloudToControllerSuite struct {
	cmdtest.JimmCmdSuite
}

var _ = gc.Suite(&addCloudToControllerSuite{})

func (s *addCloudToControllerSuite) SetUpTest(c *gc.C) {
	s.JimmCmdSuite.SetUpTest(c)

	// We add user bob, who is a JIMM administrator.
	i, err := dbmodel.NewIdentity("bob@canonical.com")
	c.Assert(err, gc.IsNil)
	err = s.JIMM.Database.UpdateIdentity(context.Background(), i)
	c.Assert(err, gc.IsNil)

	// We add a test-cloud cloud.
	info := s.APIInfo(c)
	err = s.JIMM.Database.AddCloud(context.Background(), &dbmodel.Cloud{
		Name:    "test-cloud",
		Type:    "kubernetes",
		Regions: []dbmodel.CloudRegion{{Name: "default", CloudName: "test-cloud"}},
	})
	c.Assert(err, gc.IsNil)
	region, err := s.JIMM.Database.FindRegion(context.Background(), "kubernetes", "default")
	c.Assert(err, gc.IsNil)

	// We grant user bob administrator access to JIMM and the added
	// test-cloud.
	i, err = dbmodel.NewIdentity("bob@canonical.com")
	c.Assert(err, gc.IsNil)
	bob := openfga.NewUser(
		i,
		s.JIMM.OpenFGAClient,
	)
	err = bob.SetControllerAccess(context.Background(), s.JIMM.ResourceTag(), ofganames.AdministratorRelation)
	c.Assert(err, gc.IsNil)
	err = bob.SetCloudAccess(context.Background(), names.NewCloudTag("test-cloud"), ofganames.AdministratorRelation)
	c.Assert(err, gc.IsNil)

	err = s.JIMM.Database.AddController(context.Background(), &dbmodel.Controller{
		Name:          "controller-1",
		CACertificate: info.CACert,
		UUID:          info.ControllerUUID,
		PublicAddress: info.Addrs[0],
		CloudName:     "test-cloud",
		CloudRegion:   "default",
		CloudRegions: []dbmodel.CloudRegionControllerPriority{{
			CloudRegion: *region,
			Priority:    1,
		}},
	})
	c.Assert(err, gc.IsNil)

	err = s.JIMM.OpenFGAClient.AddController(context.Background(), s.JIMM.ResourceTag(), names.NewControllerTag(info.ControllerUUID))
	c.Assert(err, gc.IsNil)
}

func (s *addCloudToControllerSuite) TestAddCloudToController(c *gc.C) {
	tests := []struct {
		about             string
		cloudInfo         string
		force             bool
		cloudByNameFunc   func(cloudName string) (*cloud.Cloud, error)
		expectedCloudName string
		expectedIndex     int
		expectedError     string
	}{
		{
			about: "Add Kubernetes Cloud",
			cloudInfo: `
clouds:
  test-hosted-cloud:
    type: kubernetes
    auth-types: [certificate]
    host-cloud-region: kubernetes/default`,
			force:             false,
			expectedCloudName: "test-hosted-cloud",
			expectedIndex:     1,
			expectedError:     "",
		}, {
			about: "Add MAAS Cloud",
			cloudInfo: `
clouds:
  test-maas-cloud:
    type: maas
    auth-types: [oauth1]
    regions:
      default: {}`,
			force:             true,
			expectedCloudName: "test-maas-cloud",
			expectedIndex:     2,
			expectedError:     "",
		}, {
			about: "Add cloud with unknown provider",
			cloudInfo: `
clouds:
  test-unknown-cloud:
    type: unknown
    auth-types: [oauth1]
    regions:
      default: {}`,
			force:             false,
			expectedCloudName: "test-unknown-cloud",
			expectedError:     ".*no registered provider.*",
		}, {
			about: "Add cloud to controller with wrong name",
			cloudInfo: `
clouds:
  test-hosted-cloud-2:
    type: kubernetes
    auth-types: [certificate]
    host-cloud-region: kubernetes/default`,
			force:             false,
			expectedCloudName: "test-cloud",
			expectedError:     ".* cloud .* not found in file .*",
		}, {
			about: "Add existing cloud to controller",
			cloudByNameFunc: func(cloudName string) (*cloud.Cloud, error) {
				return &cloud.Cloud{
					Name:            "test-hosted-cloud-2",
					Type:            "kubernetes",
					AuthTypes:       []cloud.AuthType{"certificate"},
					HostCloudRegion: "kubernetes/default",
				}, nil
			},
			force:             false,
			expectedCloudName: "test-hosted-cloud-2",
			expectedIndex:     3,
			expectedError:     "",
		}, {
			about: "Add existing cloud to controller where existing cloud is not found",
			cloudByNameFunc: func(cloudName string) (*cloud.Cloud, error) {
				return nil, errors.E("not found")
			},
			force:             false,
			expectedCloudName: "test-cloud",
			expectedError:     "could not find existing cloud, please provide a cloud file",
		},
	}

	for _, test := range tests {
		c.Log(test.about)
		tmpfile, cleanupFunc := writeTempFile(c, test.cloudInfo)

		bClient := s.SetupCLIAccess(c, "bob@canonical.com")
		// Running the command succeeds
		newCmd := cmd.NewAddCloudToControllerCommandForTesting(s.ClientStore(), bClient, test.cloudByNameFunc)
		var err error
		if test.cloudInfo != "" {
			_, err = cmdtesting.RunCommand(c, newCmd, "controller-1", test.expectedCloudName, "--cloud="+tmpfile, "--force="+strconv.FormatBool(test.force))
		} else {
			_, err = cmdtesting.RunCommand(c, newCmd, "controller-1", test.expectedCloudName, "--force="+strconv.FormatBool(test.force))
		}

		if test.expectedError != "" {
			c.Assert(err, gc.NotNil)
			errWithoutBreaks := strings.ReplaceAll(err.Error(), "\n", "")
			c.Assert(errWithoutBreaks, gc.Matches, test.expectedError)
		} else {
			c.Assert(err, gc.IsNil)
			cloud := dbmodel.Cloud{Name: test.expectedCloudName}
			err = s.JIMM.Database.GetCloud(context.Background(), &cloud)
			c.Assert(err, gc.IsNil)
			controller := dbmodel.Controller{Name: "controller-1"}
			err = s.JIMM.Database.GetController(context.Background(), &controller)
			c.Assert(err, gc.IsNil)
			c.Assert(controller.CloudRegions[test.expectedIndex].CloudRegion.CloudName, gc.Equals, test.expectedCloudName)
		}
		cleanupFunc()
		// Needed for JIMM CLI table tests
		s.RefreshControllerAddress(c)
	}
}

func writeTempFile(c *gc.C, content string) (string, func()) {
	dir, err := os.MkdirTemp("", "add-cloud-to-controller-test")
	c.Assert(err, gc.Equals, nil)

	tmpfn := filepath.Join(dir, "tmp.yaml")

	err = os.WriteFile(tmpfn, []byte(content), 0600)
	c.Assert(err, gc.Equals, nil)
	return tmpfn, func() {
		os.RemoveAll(dir)
	}
}
