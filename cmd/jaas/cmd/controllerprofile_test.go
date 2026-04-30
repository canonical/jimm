// Copyright 2026 Canonical.

package cmd

import (
	"bytes"
	"testing"

	qt "github.com/frankban/quicktest"
	"github.com/juju/cmd/v3/cmdtesting"
	"go.uber.org/mock/gomock"
	"sigs.k8s.io/yaml"

	apiparams "github.com/canonical/jimm/v3/pkg/api/params"
)

func sampleControllerProfileRequest(name string) apiparams.SaveControllerProfileRequest {
	return apiparams.SaveControllerProfileRequest{
		ControllerProfile: apiparams.ControllerProfile{
			Name:        name,
			Description: "Reusable bootstrap settings",
			JujuVersion: "3.6",
			Cloud: apiparams.BootstrapCloud{
				Name: "aws",
				Type: "ec2",
				Region: apiparams.BootstrapCloudRegion{
					Name: "eu-west-1",
				},
			},
			BootstrapOptions: apiparams.BootstrapOptions{
				BootstrapBase: "ubuntu@24.04",
			},
		},
	}
}

func sampleControllerProfileYAML(c *qt.C, name string) string {
	c.Helper()
	data, err := yaml.Marshal(sampleControllerProfileRequest(name))
	c.Assert(err, qt.IsNil)
	return string(data)
}

func TestAddControllerProfileRun(t *testing.T) {
	c := qt.New(t)
	s := setupCmdMocks(c)

	wrapped := &addControllerProfileCommand{}
	wrapped.SetClientStore(s.store)
	wrapped.setJIMMAPI(s.client)

	initCommand(c, wrapped, "aws-prod", "--file", "-")

	ctx := newTestContext(c)
	ctx.Stdin = bytes.NewBufferString(sampleControllerProfileYAML(c, "aws-prod"))

	s.client.EXPECT().SaveControllerProfile(gomock.Any()).DoAndReturn(func(req *apiparams.SaveControllerProfileRequest) (apiparams.SaveControllerProfileResponse, error) {
		c.Check(req.Name, qt.Equals, "aws-prod")
		c.Check(req.JujuVersion, qt.Equals, "3.6")
		return apiparams.SaveControllerProfileResponse{ControllerProfile: req.ControllerProfile}, nil
	}).Times(1)
	s.client.EXPECT().Close().Times(1)

	err := wrapped.Run(ctx)
	c.Assert(err, qt.IsNil)
	c.Assert(cmdtesting.Stdout(ctx), qt.Matches, `(?s).*name: aws-prod.*`)
}

func TestUpdateControllerProfileRunUsesCurrentVersion(t *testing.T) {
	c := qt.New(t)
	s := setupCmdMocks(c)

	wrapped := &updateControllerProfileCommand{}
	wrapped.SetClientStore(s.store)
	wrapped.setJIMMAPI(s.client)

	initCommand(c, wrapped, "aws-prod", "--file", "-")

	ctx := newTestContext(c)
	ctx.Stdin = bytes.NewBufferString(sampleControllerProfileYAML(c, "aws-prod"))

	s.client.EXPECT().GetControllerProfile(&apiparams.GetControllerProfileRequest{Name: "aws-prod"}).Return(
		apiparams.GetControllerProfileResponse{ControllerProfile: apiparams.ControllerProfile{Name: "aws-prod", Version: 7}},
		nil,
	).Times(1)
	s.client.EXPECT().SaveControllerProfile(gomock.Any()).DoAndReturn(func(req *apiparams.SaveControllerProfileRequest) (apiparams.SaveControllerProfileResponse, error) {
		c.Check(req.Name, qt.Equals, "aws-prod")
		c.Check(req.Version, qt.Equals, uint(7))
		return apiparams.SaveControllerProfileResponse{ControllerProfile: req.ControllerProfile}, nil
	}).Times(1)
	s.client.EXPECT().Close().Times(1)

	err := wrapped.Run(ctx)
	c.Assert(err, qt.IsNil)
}

func TestShowControllerProfileRun(t *testing.T) {
	c := qt.New(t)
	s := setupCmdMocks(c)

	showCmd := &showControllerProfileCommand{}
	showCmd.SetClientStore(s.store)
	showCmd.setJIMMAPI(s.client)
	initCommand(c, showCmd, "aws-prod", "--format", "yaml")

	s.client.EXPECT().GetControllerProfile(&apiparams.GetControllerProfileRequest{Name: "aws-prod"}).Return(
		apiparams.GetControllerProfileResponse{ControllerProfile: sampleControllerProfileRequest("aws-prod").ControllerProfile},
		nil,
	).Times(1)
	s.client.EXPECT().Close().Times(1)

	ctx := newTestContext(c)
	err := showCmd.Run(ctx)
	c.Assert(err, qt.IsNil)
	c.Assert(cmdtesting.Stdout(ctx), qt.Matches, `(?s).*name: aws-prod.*juju-version: "3.6".*`)
}

func TestListControllerProfilesRun(t *testing.T) {
	c := qt.New(t)
	s := setupCmdMocks(c)

	listCmd := &listControllerProfilesCommand{}
	listCmd.SetClientStore(s.store)
	listCmd.setJIMMAPI(s.client)
	initCommand(c, listCmd, "--juju-version", "3.6.4")

	s.client.EXPECT().ListControllerProfiles(&apiparams.ListControllerProfilesRequest{JujuVersion: "3.6.4"}).Return([]apiparams.ControllerProfileSummary{{
		Name:        "aws-prod",
		Description: "Reusable bootstrap settings",
	}}, nil).Times(1)
	s.client.EXPECT().Close().Times(1)

	ctx := newTestContext(c)
	err := listCmd.Run(ctx)
	c.Assert(err, qt.IsNil)
	c.Assert(cmdtesting.Stdout(ctx), qt.Matches, `(?s).*- name: aws-prod.*`)
}

func TestRemoveControllerProfileRun(t *testing.T) {
	c := qt.New(t)
	s := setupCmdMocks(c)

	removeCmd := &removeControllerProfileCommand{}
	removeCmd.SetClientStore(s.store)
	removeCmd.setJIMMAPI(s.client)
	initCommand(c, removeCmd, "aws-prod", "--force")

	s.client.EXPECT().RemoveControllerProfile(&apiparams.RemoveControllerProfileRequest{Name: "aws-prod"}).Return(nil).Times(1)
	s.client.EXPECT().Close().Times(1)

	ctx := newTestContext(c)
	err := removeCmd.Run(ctx)
	c.Assert(err, qt.IsNil)
}
