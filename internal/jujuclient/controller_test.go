package jujuclient_test

import (
	"context"
	"sort"

	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v5"
	gc "gopkg.in/check.v1"
)

type controllerSuite struct {
	jujuclientSuite
}

var _ = gc.Suite(&controllerSuite{})

func (s *controllerSuite) TestAllModels(c *gc.C) {
	ctx := context.Background()

	var info jujuparams.ModelInfo
	err := s.API.CreateModel(ctx, &jujuparams.ModelCreateArgs{
		Name:     "test-model",
		OwnerTag: names.NewUserTag("test-user@canonical.com").String(),
	}, &info)
	c.Assert(err, gc.Equals, nil)

	userModels, err := s.API.AllModels(context.Background())
	c.Assert(err, gc.IsNil)
	sort.Slice(userModels.UserModels, func(i, j int) bool {
		return userModels.UserModels[i].UUID < userModels.UserModels[j].UUID
	})

	c.Assert(userModels.UserModels[0].Name, gc.Equals, "test-model")
	c.Assert(userModels.UserModels[1].Name, gc.Equals, "controller")
}
