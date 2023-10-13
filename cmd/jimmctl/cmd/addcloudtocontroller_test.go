// Copyright 2021 Canonical Ltd.
package cmd_test

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/juju/cmd/v3/cmdtesting"
	"github.com/juju/juju/cloud"

	gc "gopkg.in/check.v1"

	"github.com/canonical/jimm/cmd/jimmctl/cmd"
	"github.com/canonical/jimm/internal/dbmodel"
	"github.com/canonical/jimm/internal/errors"
)

type addCloudToControllerSuite struct {
	jimmSuite
}

var _ = gc.Suite(&addCloudToControllerSuite{})

func (s *addCloudToControllerSuite) SetUpTest(c *gc.C) {
	s.jimmSuite.SetUpTest(c)

	// add some users
	err := s.JIMM.Database.UpdateUser(context.Background(), &dbmodel.User{
		DisplayName:      "Bob",
		Username:         "bob@external",
		ControllerAccess: "superuser",
	})
	c.Assert(err, gc.IsNil)

	// add a cloud
	info := s.APIInfo(c)
	err = s.JIMM.Database.AddCloud(context.Background(), &dbmodel.Cloud{
		Name:    "test-cloud",
		Type:    "kubernetes",
		Regions: []dbmodel.CloudRegion{{Name: "default", CloudName: "test-cloud"}},
		Users:   []dbmodel.UserCloudAccess{{Username: "bob@external", CloudName: "test-cloud", Access: "admin"}},
	})
	c.Assert(err, gc.IsNil)
	region, err := s.JIMM.Database.FindRegion(context.Background(), "kubernetes", "default")
	c.Assert(err, gc.IsNil)

	// add 2 controllers
	err = s.JIMM.Database.AddController(context.Background(), &dbmodel.Controller{
		Name:          "controller-1",
		CACertificate: info.CACert,
		AdminUser:     info.Tag.Id(),
		AdminPassword: info.Password,
		UUID:          "00000001-0000-0000-0000-000000000001",
		PublicAddress: info.Addrs[0],
		CloudName:     "test-cloud",
		CloudRegion:   "default",
		CloudRegions: []dbmodel.CloudRegionControllerPriority{{
			CloudRegion: *region,
			Priority:    1,
		}},
	})
	c.Assert(err, gc.IsNil)
	err = s.JIMM.Database.AddController(context.Background(), &dbmodel.Controller{
		Name:          "controller-2",
		CACertificate: info.CACert,
		AdminUser:     info.Tag.Id(),
		AdminPassword: info.Password,
		UUID:          "00000001-0000-0000-0000-000000000002",
		PublicAddress: info.Addrs[0],
		CloudName:     "test-cloud",
		CloudRegion:   "default",
		CloudRegions: []dbmodel.CloudRegionControllerPriority{{
			CloudRegion: *region,
			Priority:    10,
		}},
	})
	c.Assert(err, gc.IsNil)
}

func (s *addCloudToControllerSuite) TestAddCloudToController(c *gc.C) {
	clouds := `
clouds:
  test-hosted-cloud:
    type: kubernetes
    auth-types: [certificate]
    host-cloud-region: kubernetes/default
`
	tmpfile, cleanupFunc := writeTempFile(c, clouds)
	defer cleanupFunc()

	bClient := s.userBakeryClient("bob@external")

	// Running the command succeeds
	_, err := cmdtesting.RunCommand(c, cmd.NewAddCloudToControllerCommandForTesting(s.ClientStore, bClient, nil), "controller-1", "test-hosted-cloud", "--cloud="+tmpfile)
	c.Assert(err, gc.IsNil)

	// The cloud is there
	cloud := dbmodel.Cloud{Name: "test-hosted-cloud"}
	err = s.JIMM.Database.GetCloud(context.Background(), &cloud)
	c.Assert(err, gc.IsNil)
	controller := dbmodel.Controller{Name: "controller-1"}
	s.JIMM.Database.GetController(context.Background(), &controller)
	c.Assert(controller.CloudRegions, gc.HasLen, 2)
	c.Assert(controller.CloudRegions[1].CloudRegion.CloudName, gc.Equals, "test-hosted-cloud")
}

func (s *addCloudToControllerSuite) TestAddMaasCloudToController(c *gc.C) {
	clouds := `
clouds:
  test-hosted-cloud:
    type: maas
    auth-types: [oauth1]
    regions:
      default: {}
`
	tmpfile, cleanupFunc := writeTempFile(c, clouds)
	defer cleanupFunc()

	bClient := s.userBakeryClient("bob@external")

	// Running the command succeeds
	// Force is required here because the JIMM cloud is provisioned as "dummy" and so doesn't pass the default checks.
	_, err := cmdtesting.RunCommand(c, cmd.NewAddCloudToControllerCommandForTesting(s.ClientStore, bClient, nil), "controller-1", "test-hosted-cloud", "--cloud="+tmpfile, "--force")
	c.Assert(err, gc.IsNil)

	// The cloud is there
	cloud := dbmodel.Cloud{Name: "test-hosted-cloud"}
	err = s.JIMM.Database.GetCloud(context.Background(), &cloud)
	c.Assert(err, gc.IsNil)
	controller := dbmodel.Controller{Name: "controller-1"}
	s.JIMM.Database.GetController(context.Background(), &controller)
	c.Assert(controller.CloudRegions, gc.HasLen, 2)
	c.Assert(controller.CloudRegions[1].CloudRegion.CloudName, gc.Equals, "test-hosted-cloud")
}

func (s *addCloudToControllerSuite) TestAddCloudWithoutProviderToController(c *gc.C) {
	clouds := `
clouds:
    test-hosted-cloud:
      type: unknown
      auth-types: [oauth1]
      regions:
        default: {}
`
	tmpfile, cleanupFunc := writeTempFile(c, clouds)
	defer cleanupFunc()

	bClient := s.userBakeryClient("bob@external")

	// Running the command succeeds
	_, err := cmdtesting.RunCommand(c, cmd.NewAddCloudToControllerCommandForTesting(s.ClientStore, bClient, nil), "controller-1", "test-hosted-cloud", "--cloud="+tmpfile)
	c.Assert(err, gc.ErrorMatches, ".*no registered provider.*")
}

func (s *addCloudToControllerSuite) TestAddIncompatibleCloudToController(c *gc.C) {
	clouds := `
clouds:
    test-hosted-cloud:
      type: maas
      auth-types: [oauth1]
      regions:
        default: {}
`
	tmpfile, cleanupFunc := writeTempFile(c, clouds)
	defer cleanupFunc()

	bClient := s.userBakeryClient("bob@external")

	// Running the command succeeds
	_, err := cmdtesting.RunCommand(c, cmd.NewAddCloudToControllerCommandForTesting(s.ClientStore, bClient, nil), "controller-1", "test-hosted-cloud", "--cloud="+tmpfile)
	c.Assert(err, gc.NotNil)
	errWithoutBreaks := strings.ReplaceAll(err.Error(), "\n", "")
	c.Assert(errWithoutBreaks, gc.Matches, ".*incompatible clouds.*")
}

func (s *addCloudToControllerSuite) TestAddCloudToControllerExisting(c *gc.C) {
	bClient := s.userBakeryClient("bob@external")

	// Running the command with an existing cloud works
	cloudByNameFunc := func(cloudName string) (*cloud.Cloud, error) {
		return &cloud.Cloud{
			Name:            "test-hosted-cloud-2",
			Type:            "kubernetes",
			AuthTypes:       []cloud.AuthType{"certificate"},
			HostCloudRegion: "kubernetes/default",
		}, nil
	}
	_, err := cmdtesting.RunCommand(c, cmd.NewAddCloudToControllerCommandForTesting(s.ClientStore, bClient, cloudByNameFunc), "controller-1", "test-hosted-cloud-2")
	c.Assert(err, gc.IsNil)
}

func (s *addCloudToControllerSuite) TestAddCloudToControllerExistingNotFound(c *gc.C) {
	cloudByNameFunc := func(cloudName string) (*cloud.Cloud, error) {
		return nil, errors.E("not found")
	}
	bClient := s.userBakeryClient("bob")
	_, err := cmdtesting.RunCommand(c, cmd.NewAddCloudToControllerCommandForTesting(s.ClientStore, bClient, cloudByNameFunc), "controller-1", "test-cloud")
	c.Assert(err, gc.ErrorMatches, "could not find existing cloud, please provide a cloud file")
}

func (s *addCloudToControllerSuite) TestAddCloudToControllerWrongName(c *gc.C) {
	clouds := `
clouds:
  test-hosted-cloud-2:
    type: kubernetes
    auth-types: [certificate]
    host-cloud-region: kubernetes/default
`

	tmpfile, cleanupFunc := writeTempFile(c, clouds)
	defer cleanupFunc()

	bClient := s.userBakeryClient("bob")
	_, err := cmdtesting.RunCommand(c, cmd.NewAddCloudToControllerCommandForTesting(s.ClientStore, bClient, nil), "controller-1", "test-cloud", "--cloud="+tmpfile)
	c.Assert(err, gc.ErrorMatches, ".* cloud .* not found in file .*")
}

func writeTempFile(c *gc.C, content string) (string, func()) {
	dir, err := ioutil.TempDir("", "add-cloud-to-controller-test")
	c.Assert(err, gc.Equals, nil)

	tmpfn := filepath.Join(dir, "tmp.yaml")
	err = ioutil.WriteFile(tmpfn, []byte(content), 0666)
	c.Assert(err, gc.Equals, nil)
	return tmpfn, func() {
		os.RemoveAll(dir)
	}
}
