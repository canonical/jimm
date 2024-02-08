// Copyright 2024 Canonical Ltd.

package cmd_test

import (
	"encoding/json"

	"github.com/juju/cmd/v3/cmdtesting"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v4"
	"github.com/riverqueue/river/rivertype"
	gc "gopkg.in/check.v1"

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
	context, err := cmdtesting.RunCommand(c, cmd.NewJobViewerCommandForTesting(s.ClientStore(), bClient), "--getCompleted", "--format", "json")
	c.Assert(err, gc.IsNil)

	cmdOut := cmdtesting.Stdout(context)
	var data map[rivertype.JobState][]rivertype.JobRow
	if err := json.Unmarshal([]byte(cmdOut), &data); err != nil {
		c.Errorf("failed to deserialize command output, err %s", err)
	}
	c.Assert(len(data[rivertype.JobStateCompleted]), gc.Equals, 1)
	jobRow := data[rivertype.JobStateCompleted][0]
	c.Assert(jobRow.Kind, gc.Equals, "AddModel")
	var args jimm.RiverAddModelArgs
	err = json.Unmarshal(jobRow.EncodedArgs, &args)
	if err != nil {
		c.Errorf("Error unmarshalling decoded EncodedArgs, err %s", err)
	}
	config := make(map[string]interface{})
	c.Assert(args, gc.DeepEquals, jimm.RiverAddModelArgs{
		ModelId:   1,
		OwnerName: "charlie@external",
		Config:    config,
	})
	c.Assert(len(data[rivertype.JobStateCancelled]), gc.Equals, 0)
	c.Assert(len(data[rivertype.JobStateDiscarded]), gc.Equals, 0)
}
