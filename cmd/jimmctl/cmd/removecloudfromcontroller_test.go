// Copyright 2021 Canonical Ltd.
package cmd_test

import (
	"github.com/juju/cmd/v3/cmdtesting"
	jujutesting "github.com/juju/testing"

	gc "gopkg.in/check.v1"

	apiparams "github.com/CanonicalLtd/jimm/api/params"
	"github.com/CanonicalLtd/jimm/cmd/jimmctl/cmd"
)

type removeCloudFromControllerSuite struct {
	jimmSuite

	api *fakeRemoveCloudFromControllerAPI
}

var _ = gc.Suite(&removeCloudFromControllerSuite{})

func (s *removeCloudFromControllerSuite) SetUpTest(c *gc.C) {
	s.jimmSuite.SetUpTest(c)
	s.api = &fakeRemoveCloudFromControllerAPI{}
}

func (s *removeCloudFromControllerSuite) TestRemoveCloudFromController(c *gc.C) {
	bClient := s.userBakeryClient("alice@external")

	command := cmd.NewRemoveCloudFromControllerCommandForTesting(
		s.ClientStore(),
		bClient,
		func() (cmd.RemoveCloudFromControllerAPI, error) {
			return s.api, nil
		})
	ctx, err := cmdtesting.RunCommand(c, command, "controller-1", "test-cloud")
	c.Assert(err, gc.IsNil)
	s.api.CheckCallNames(c, "RemoveCloudFromController")
	s.api.CheckCalls(c, []jujutesting.StubCall{{
		FuncName: "RemoveCloudFromController",
		Args: []interface{}{&apiparams.RemoveCloudFromControllerRequest{
			ControllerName: "controller-1",
			CloudTag:       "cloud-test-cloud",
		}},
	}})
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, "Cloud \"test-cloud\" removed from controller \"controller-1\".\n")
}

func (s *removeCloudFromControllerSuite) TestRemoveCloudFromControllerWrongArguments(c *gc.C) {
	bClient := s.userBakeryClient("alice@external")

	command := cmd.NewRemoveCloudFromControllerCommandForTesting(
		s.ClientStore(),
		bClient,
		func() (cmd.RemoveCloudFromControllerAPI, error) {
			return s.api, nil
		})
	_, err := cmdtesting.RunCommand(c, command, "controller-1")
	c.Assert(err, gc.ErrorMatches, "missing arguments")
	_, err = cmdtesting.RunCommand(c, command, "controller-1", "cloud", "fake-arg")
	c.Assert(err, gc.ErrorMatches, "too many arguments")
}

func (s *removeCloudFromControllerSuite) TestRemoveCloudFromControllerCloudNotFound(c *gc.C) {
	bClient := s.userBakeryClient("alice@external")

	command := cmd.NewRemoveCloudFromControllerCommandForTesting(
		s.ClientStore(),
		bClient,
		nil)
	_, err := cmdtesting.RunCommand(c, command, "controller-1", "test-cloud")
	c.Assert(err, gc.ErrorMatches, ".*cloud \"test-cloud\" not found.*")
}

type fakeRemoveCloudFromControllerAPI struct {
	jujutesting.Stub
}

func (api *fakeRemoveCloudFromControllerAPI) Close() error {
	api.AddCall("Close", nil)
	return api.NextErr()
}

func (api *fakeRemoveCloudFromControllerAPI) RemoveCloudFromController(params *apiparams.RemoveCloudFromControllerRequest) error {
	api.AddCall("RemoveCloudFromController", params)
	return api.NextErr()
}