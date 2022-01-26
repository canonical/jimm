// Copyright 2020 Canonical Ltd.

package conv_test

import (
	gc "gopkg.in/check.v1"

	"github.com/CanonicalLtd/jimm/internal/conv"
	"github.com/CanonicalLtd/jimm/params"
)

type userSuite struct{}

var _ = gc.Suite(&userSuite{})

func (s *userSuite) TestToUserTag(c *gc.C) {
	c.Assert(conv.ToUserTag(params.User("alice")).String(), gc.Equals, "user-alice@external")
	c.Assert(conv.ToUserTag(params.User("alice@domain")).String(), gc.Equals, "user-alice@domain")
}

func (s *userSuite) TestFromUserID(c *gc.C) {
	user, err := conv.FromUserID("user1@domain")
	c.Assert(err, gc.Equals, nil)
	c.Assert(user, gc.DeepEquals, params.User("user1"))
}
