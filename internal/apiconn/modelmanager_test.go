// Copyright 2020 Canonical Ltd.

package apiconn_test

import (
	"context"
	"fmt"
	"time"

	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/juju/juju/api"
	jujuparams "github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/life"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v2"
	gc "gopkg.in/check.v1"

	"github.com/CanonicalLtd/jimm/internal/apiconn"
	"github.com/CanonicalLtd/jimm/internal/conv"
	"github.com/CanonicalLtd/jimm/internal/jemtest"
)

type modelmanagerSuite struct {
	jemtest.JujuConnSuite

	cache *apiconn.Cache
	conn  *apiconn.Conn
}

var _ = gc.Suite(&modelmanagerSuite{})

func (s *modelmanagerSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)

	s.cache = apiconn.NewCache(apiconn.CacheParams{})

	var err error
	s.conn, err = s.cache.OpenAPI(context.Background(), s.ControllerConfig.ControllerUUID(), func() (api.Connection, *api.Info, error) {
		apiInfo := s.APIInfo(c)
		return apiOpen(
			&api.Info{
				Addrs:    apiInfo.Addrs,
				CACert:   apiInfo.CACert,
				Tag:      apiInfo.Tag,
				Password: apiInfo.Password,
			},
			api.DialOpts{},
		)
	})
	c.Assert(err, gc.Equals, nil)
}

func (s *modelmanagerSuite) TearDownTest(c *gc.C) {
	if s.conn != nil {
		s.conn.Close()
	}
	if s.cache != nil {
		s.cache.Close()
	}
	s.JujuConnSuite.TearDownTest(c)
}

func (s *modelmanagerSuite) TestCreateModel(c *gc.C) {
	ctx := context.Background()

	var info jujuparams.ModelInfo
	err := s.conn.CreateModel(ctx, &jujuparams.ModelCreateArgs{
		Name:     "test-model",
		OwnerTag: conv.ToUserTag("test-user").String(),
	}, &info)
	c.Assert(err, gc.Equals, nil)

	c.Check(info.UUID, gc.Not(gc.Equals), "")
	c.Check(info.CloudTag, gc.Equals, names.NewCloudTag("dummy").String())
	c.Check(info.CloudRegion, gc.Equals, "dummy-region")
	c.Check(info.DefaultSeries, gc.Equals, "focal")
	c.Check(string(info.Life), gc.Equals, "alive")
	c.Check(string(info.Status.Status), gc.Equals, "available")
	c.Check(info.Status.Data, gc.IsNil)
	c.Check(info.Status.Since.After(time.Now().Add(-10*time.Second)), gc.Equals, true)
	c.Check(info.Type, gc.Equals, "iaas")
	c.Check(info.ProviderType, gc.Equals, "dummy")
}

func (s *modelmanagerSuite) TestCreateModelError(c *gc.C) {
	ctx := context.Background()

	var info jujuparams.ModelInfo
	err := s.conn.CreateModel(ctx, &jujuparams.ModelCreateArgs{
		Name:     "test-model",
		OwnerTag: conv.ToUserTag("test-user").String(),
		CloudTag: conv.ToCloudTag("nosuchcloud").String(),
	}, &info)
	c.Check(apiconn.IsAPIError(err), gc.Equals, true)
	c.Check(jujuparams.ErrCode(err), gc.Equals, jujuparams.CodeNotFound)
	c.Check(err, gc.ErrorMatches, `api error: cloud "nosuchcloud" not found, expected one of \["dummy"\] \(not found\)`)
}

func (s *modelmanagerSuite) TestGrantJIMMModelAdmin(c *gc.C) {
	ctx := context.Background()

	var info jujuparams.ModelInfo
	err := s.conn.CreateModel(ctx, &jujuparams.ModelCreateArgs{
		Name:     "test-model",
		OwnerTag: conv.ToUserTag("test-user").String(),
	}, &info)
	c.Assert(err, gc.Equals, nil)

	err = s.conn.GrantJIMMModelAdmin(ctx, info.UUID)
	c.Assert(err, gc.Equals, nil)

	err = s.conn.ModelInfo(ctx, &info)
	c.Assert(err, gc.Equals, nil)

	var access jujuparams.UserAccessPermission
	for _, u := range info.Users {
		if u.UserName == s.conn.Info.Tag.Id() {
			access = u.Access
		}
	}
	c.Check(access, gc.Equals, jujuparams.ModelAdminAccess)
}

func (s *modelmanagerSuite) TestGrantJIMMModelAdminError(c *gc.C) {
	ctx := context.Background()

	err := s.conn.GrantJIMMModelAdmin(ctx, "00000000-0000-0000-0000-000000000000")
	c.Check(apiconn.IsAPIError(err), gc.Equals, true)
	c.Check(jujuparams.ErrCode(err), gc.Equals, jujuparams.CodeNotFound)
	c.Check(err, gc.ErrorMatches, `api error: could not lookup model: model "00000000-0000-0000-0000-000000000000" not found`)
}

func (s *modelmanagerSuite) TestModelInfo(c *gc.C) {
	ctx := context.Background()

	var info jujuparams.ModelInfo
	err := s.conn.CreateModel(ctx, &jujuparams.ModelCreateArgs{
		Name:     "test-model",
		OwnerTag: conv.ToUserTag("test-user").String(),
	}, &info)
	c.Assert(err, gc.Equals, nil)

	mi := jujuparams.ModelInfo{
		UUID: info.UUID,
	}
	err = s.conn.ModelInfo(ctx, &mi)
	c.Assert(err, gc.Equals, nil)

	c.Check(mi, jc.DeepEquals, info)
}

func (s *modelmanagerSuite) TestModelInfoError(c *gc.C) {
	ctx := context.Background()

	mi := jujuparams.ModelInfo{
		UUID: "00000000-0000-0000-0000-000000000000",
	}
	err := s.conn.ModelInfo(ctx, &mi)
	c.Check(apiconn.IsAPIError(err), gc.Equals, true)
	c.Check(jujuparams.ErrCode(err), gc.Equals, jujuparams.CodeUnauthorized)
	c.Check(err, gc.ErrorMatches, `api error: permission denied`)
}

func (s *modelmanagerSuite) TestGrantRevokeModel(c *gc.C) {
	ctx := context.Background()

	var info jujuparams.ModelInfo
	err := s.conn.CreateModel(ctx, &jujuparams.ModelCreateArgs{
		Name:     "test-model",
		OwnerTag: conv.ToUserTag("test-user").String(),
	}, &info)
	c.Assert(err, gc.Equals, nil)

	err = s.conn.GrantModelAccess(ctx, info.UUID, "test-user-2", jujuparams.ModelReadAccess)
	c.Assert(err, gc.Equals, nil)

	err = s.conn.ModelInfo(ctx, &info)
	c.Assert(err, gc.Equals, nil)

	lessf := func(a, b jujuparams.ModelUserInfo) bool {
		return a.UserName < b.UserName
	}
	c.Check(info.Users, jemtest.CmpEquals(cmpopts.SortSlices(lessf)), []jujuparams.ModelUserInfo{{
		UserName:    "test-user@external",
		DisplayName: "test-user",
		Access:      "admin",
	}, {
		UserName: "test-user-2@external",
		Access:   "read",
	}})

	err = s.conn.RevokeModelAccess(ctx, info.UUID, "test-user-2", jujuparams.ModelReadAccess)
	c.Assert(err, gc.Equals, nil)

	err = s.conn.ModelInfo(ctx, &info)
	c.Assert(err, gc.Equals, nil)

	c.Check(info.Users, jemtest.CmpEquals(cmpopts.SortSlices(lessf)), []jujuparams.ModelUserInfo{{
		UserName:    "test-user@external",
		DisplayName: "test-user",
		Access:      "admin",
	}})
}

func (s *modelmanagerSuite) TestGrantModelAccessError(c *gc.C) {
	ctx := context.Background()

	err := s.conn.GrantModelAccess(ctx, "00000000-0000-0000-0000-000000000000", "test-user-2", jujuparams.ModelReadAccess)

	c.Check(apiconn.IsAPIError(err), gc.Equals, true)
	c.Check(jujuparams.ErrCode(err), gc.Equals, jujuparams.CodeNotFound)
	c.Check(err, gc.ErrorMatches, `api error: could not lookup model: model "00000000-0000-0000-0000-000000000000" not found`)
}

func (s *modelmanagerSuite) TestRevokeModelAccessError(c *gc.C) {
	ctx := context.Background()

	err := s.conn.RevokeModelAccess(ctx, "00000000-0000-0000-0000-000000000000", "test-user-2", jujuparams.ModelReadAccess)

	c.Check(apiconn.IsAPIError(err), gc.Equals, true)
	c.Check(jujuparams.ErrCode(err), gc.Equals, jujuparams.CodeNotFound)
	c.Check(err, gc.ErrorMatches, `api error: could not lookup model: model "00000000-0000-0000-0000-000000000000" not found`)
}

func (s *modelmanagerSuite) TestValidateModelUpgrade(c *gc.C) {
	ctx := context.Background()

	args := jujuparams.ModelCreateArgs{
		Name:     "test-model",
		OwnerTag: conv.ToUserTag("test-user").String(),
	}
	var info jujuparams.ModelInfo

	err := s.conn.CreateModel(ctx, &args, &info)
	c.Assert(err, gc.Equals, nil)

	err = s.conn.ValidateModelUpgrade(ctx, names.NewModelTag(info.UUID), true)
	c.Assert(err, gc.Equals, nil)

	uuid := utils.MustNewUUID().String()
	err = s.conn.ValidateModelUpgrade(ctx, names.NewModelTag(uuid), false)
	c.Assert(err, gc.ErrorMatches, fmt.Sprintf("api error: model %q not found", uuid))
}

func (s *modelmanagerSuite) TestDestroyModel(c *gc.C) {
	ctx := context.Background()

	var info jujuparams.ModelInfo
	err := s.conn.CreateModel(ctx, &jujuparams.ModelCreateArgs{
		Name:     "test-model",
		OwnerTag: conv.ToUserTag("test-user").String(),
	}, &info)
	c.Assert(err, gc.Equals, nil)

	err = s.conn.DestroyModel(ctx, info.UUID, nil, nil, nil)
	c.Assert(err, gc.Equals, nil)

	err = s.conn.ModelInfo(ctx, &info)
	c.Assert(err, gc.Equals, nil)

	c.Check(info.Life, gc.Equals, life.Dying)
}

func (s *modelmanagerSuite) TestModelStatus(c *gc.C) {
	ctx := context.Background()

	var info jujuparams.ModelInfo
	err := s.conn.CreateModel(ctx, &jujuparams.ModelCreateArgs{
		Name:     "test-model",
		OwnerTag: conv.ToUserTag("test-user").String(),
	}, &info)
	c.Assert(err, gc.Equals, nil)

	status := jujuparams.ModelStatus{
		ModelTag: names.NewModelTag(info.UUID).String(),
	}
	err = s.conn.ModelStatus(ctx, &status)
	c.Assert(err, gc.Equals, nil)

	c.Check(status, jc.DeepEquals, jujuparams.ModelStatus{
		ModelTag: names.NewModelTag(info.UUID).String(),
		Life:     info.Life,
		Type:     info.Type,
		OwnerTag: info.OwnerTag,
	})
}

func (s *modelmanagerSuite) TestModelStatusError(c *gc.C) {
	ctx := context.Background()

	status := jujuparams.ModelStatus{
		ModelTag: names.NewModelTag("00000000-0000-0000-0000-000000000000").String(),
	}
	err := s.conn.ModelStatus(ctx, &status)
	c.Check(apiconn.IsAPIError(err), gc.Equals, true)
	c.Check(jujuparams.ErrCode(err), gc.Equals, jujuparams.CodeNotFound)
	c.Check(err, gc.ErrorMatches, `api error: model "00000000-0000-0000-0000-000000000000" not found`)
}
