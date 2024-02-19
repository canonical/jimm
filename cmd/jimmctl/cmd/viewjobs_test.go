// Copyright 2024 Canonical Ltd.

package cmd_test

import (
	"encoding/json"

	"github.com/juju/cmd/v3/cmdtesting"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v4"
	gc "gopkg.in/check.v1"

	apiparams "github.com/canonical/jimm/api/params"
	"github.com/canonical/jimm/cmd/jimmctl/cmd"
	"github.com/canonical/jimm/internal/jimm"
	"github.com/canonical/jimm/internal/jimmtest"
)

type jobViewerSuite struct {
	jimmSuite
}

var _ = gc.Suite(&jobViewerSuite{})

func (s *jobViewerSuite) TestJobViewer2success(c *gc.C) {
	s.AddController(c, "controller-1", s.APIInfo(c))
	cct := names.NewCloudCredentialTag(jimmtest.TestCloudName + "/charlie@external/cred")
	s.UpdateCloudCredential(c, cct, jujuparams.CloudCredential{AuthType: "empty"})
	s.AddModel(c, names.NewUserTag("charlie@external"), "model-2", names.NewCloudTag(jimmtest.TestCloudName), jimmtest.TestCloudRegionName, cct)

	bClient := s.userBakeryClient("alice")
	context, err := cmdtesting.RunCommand(c, cmd.NewViewJobsCommandForTesting(s.ClientStore(), bClient), "--inc-completed", "--format", "json")
	c.Assert(err, gc.IsNil)

	cmdOut := cmdtesting.Stdout(context)
	var data apiparams.RiverJobs
	if err := json.Unmarshal([]byte(cmdOut), &data); err != nil {
		c.Errorf("failed to deserialize command output, err %s", err)
	}

	c.Assert(len(data.CompletedJobs), gc.Equals, 1)
	jobRow := data.CompletedJobs[0]
	c.Assert(jobRow.Kind, gc.Equals, "AddModel")
	var args jimm.RiverAddModelArgs
	err = json.Unmarshal(jobRow.EncodedArgs, &args)
	c.Assert(err, gc.IsNil)

	config := make(map[string]interface{})
	c.Assert(args, gc.DeepEquals, jimm.RiverAddModelArgs{
		ModelId:   1,
		OwnerName: "charlie@external",
		Config:    config,
	})
	c.Assert(len(data.CancelledJobs), gc.Equals, 0)
	c.Assert(len(data.FailedJobs), gc.Equals, 0)
}

func (s *jobViewerSuite) TestViewJobsNotAuthorized(c *gc.C) {
	s.AddController(c, "controller-1", s.APIInfo(c))

	cct := names.NewCloudCredentialTag(jimmtest.TestCloudName + "/charlie@external/cred")
	s.UpdateCloudCredential(c, cct, jujuparams.CloudCredential{AuthType: "empty"})
	s.AddModel(c, names.NewUserTag("charlie@external"), "model-2", names.NewCloudTag(jimmtest.TestCloudName), jimmtest.TestCloudRegionName, cct)

	// bob is not superuser
	bClient := s.userBakeryClient("bob")
	_, err := cmdtesting.RunCommand(c, cmd.NewViewJobsCommandForTesting(s.ClientStore(), bClient), "--inc-completed", "--format", "json")
	c.Assert(err, gc.ErrorMatches, `unauthorized \(unauthorized access\)`)
}
