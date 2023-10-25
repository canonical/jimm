// Copyright 2020 Canonical Ltd.

package conv_test

import (
	gc "gopkg.in/check.v1"

	"github.com/juju/names/v4"

	"github.com/canonical/jimm/internal/conv"
	"github.com/canonical/jimm/params"
)

type cloudSuite struct{}

var _ = gc.Suite(&cloudSuite{})

func (s *cloudSuite) TestToCloudTag(c *gc.C) {
	ct := conv.ToCloudTag("aws")
	c.Assert(ct.Id(), gc.Equals, "aws")
}

func (s *cloudSuite) TestFromCloudTag(c *gc.C) {
	cloud := conv.FromCloudTag(names.NewCloudTag("aws"))
	c.Assert(cloud, gc.Equals, params.Cloud("aws"))
}
