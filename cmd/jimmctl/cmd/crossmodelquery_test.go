// Copyright 2024 Canonical.

package cmd_test

import (
	"encoding/json"

	"github.com/juju/cmd/v3/cmdtesting"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/juju/testing/factory"
	"github.com/juju/names/v5"
	gc "gopkg.in/check.v1"

	"github.com/canonical/jimm/v3/cmd/jimmctl/cmd"
	"github.com/canonical/jimm/v3/internal/testutils/cmdtest"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
)

type crossModelQuerySuite struct {
	cmdtest.JimmCmdSuite
}

var _ = gc.Suite(&crossModelQuerySuite{})

func (s *crossModelQuerySuite) TestCrossModelQueryCommand(c *gc.C) {
	bClient := s.SetupCLIAccess(c, "alice")

	s.AddController(c, "controller-2", s.APIInfo(c))
	cct := names.NewCloudCredentialTag(jimmtest.TestCloudName + "/alice@canonical.com/cred")
	s.UpdateCloudCredential(c, cct, jujuparams.CloudCredential{AuthType: "empty"})
	mt := s.AddModel(c, names.NewUserTag("alice@canonical.com"), "stg-o11y", names.NewCloudTag(jimmtest.TestCloudName), jimmtest.TestCloudRegionName, cct)
	state, _ := s.StatePool.Get(mt.Id())
	f := factory.NewFactory(state.State, s.StatePool)
	app := f.MakeApplication(c, &factory.ApplicationParams{
		Name: "test-app",
		Charm: f.MakeCharm(c, &factory.CharmParams{
			Name: "wordpress",
		}),
	})
	f.MakeUnit(c, &factory.UnitParams{
		Application: app,
	})

	// Test.
	cmdCtx, err := cmdtesting.RunCommand(c, cmd.NewCrossModelQueryCommandForTesting(s.ClientStore(), bClient), ".")
	c.Assert(err, gc.IsNil)

	topLevel := make(map[string]any)
	c.Assert(json.Unmarshal([]byte(cmdtesting.Stdout(cmdCtx)), &topLevel), gc.IsNil)

	// Check for no errors.
	c.Assert(topLevel["errors"].(map[string]any), gc.DeepEquals, make(map[string]any))

	// There's only 1 model and 1 "result", so we just loop to check it's as
	// we expect.
	for _, v := range topLevel["results"].(map[string]any) {
		modelStatus := v.([]any)[0].(map[string]any)
		// We test simply for fields to be present in our "test-app".
		testApp := modelStatus["applications"].(map[string]any)["test-app"].(map[string]any)
		c.Assert(len(testApp), gc.Equals, 10)

		testModel := modelStatus["model"].(map[string]any)
		c.Assert(len(testModel), gc.Equals, 8)
	}
}
