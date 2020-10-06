// Copyright 2020 Canonical Ltd.

package apiconn_test

import (
	"context"
	"time"

	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/juju/juju/api"
	jujuparams "github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/environs/config"
	jujuversion "github.com/juju/juju/version"
	gc "gopkg.in/check.v1"

	"github.com/CanonicalLtd/jimm/internal/apiconn"
	"github.com/CanonicalLtd/jimm/internal/conv"
	"github.com/CanonicalLtd/jimm/internal/jemtest"
	"github.com/CanonicalLtd/jimm/internal/mongodoc"
	"github.com/CanonicalLtd/jimm/params"
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

	model := mongodoc.Model{
		Path: params.EntityPath{
			User: "test-user",
			Name: "test-model",
		},
	}

	err := s.conn.CreateModel(ctx, &model)
	c.Assert(err, gc.Equals, nil)

	c.Check(model.UUID, gc.Not(gc.Equals), "")
	c.Check(model.Cloud, gc.Equals, params.Cloud("dummy"))
	c.Check(model.CloudRegion, gc.Equals, "dummy-region")
	c.Check(model.DefaultSeries, gc.Equals, "bionic")
	c.Check(model.Info.Life, gc.Equals, "alive")
	c.Check(model.Info.Status.Status, gc.Equals, "available")
	c.Check(model.Info.Status.Data, gc.IsNil)
	c.Check(model.Info.Status.Since.After(time.Now().Add(-10*time.Second)), gc.Equals, true)
	c.Check(model.Info.Config[config.AgentVersionKey], gc.Equals, jujuversion.Current.String())
	c.Check(model.Type, gc.Equals, "iaas")
	c.Check(model.ProviderType, gc.Equals, "dummy")
}

func (s *modelmanagerSuite) TestCreateModelError(c *gc.C) {
	ctx := context.Background()

	model := mongodoc.Model{
		Path: params.EntityPath{
			User: "test-user",
			Name: "test-model",
		},
		Cloud: params.Cloud("nosuchcloud"),
	}

	err := s.conn.CreateModel(ctx, &model)
	c.Check(apiconn.IsAPIError(err), gc.Equals, true)
	c.Check(jujuparams.ErrCode(err), gc.Equals, jujuparams.CodeNotFound)
	c.Check(err, gc.ErrorMatches, `api error: cloud "nosuchcloud" not found, expected one of \["dummy"\] \(not found\)`)
}

func (s *modelmanagerSuite) TestGrantJIMMModelAdmin(c *gc.C) {
	ctx := context.Background()

	model := mongodoc.Model{
		Path: params.EntityPath{
			User: "test-user",
			Name: "test-model",
		},
	}

	err := s.conn.CreateModel(ctx, &model)
	c.Assert(err, gc.Equals, nil)

	err = s.conn.GrantJIMMModelAdmin(ctx, model.UUID)
	c.Assert(err, gc.Equals, nil)

	mi := jujuparams.ModelInfo{
		UUID: model.UUID,
	}
	err = s.conn.ModelInfo(ctx, &mi)
	c.Assert(err, gc.Equals, nil)

	var access jujuparams.UserAccessPermission
	for _, u := range mi.Users {
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

	model := mongodoc.Model{
		Path: params.EntityPath{
			User: "test-user",
			Name: "test-model",
		},
	}

	err := s.conn.CreateModel(ctx, &model)
	c.Assert(err, gc.Equals, nil)

	mi := jujuparams.ModelInfo{
		UUID: model.UUID,
	}
	err = s.conn.ModelInfo(ctx, &mi)
	c.Assert(err, gc.Equals, nil)

	c.Check(mi.UUID, gc.Equals, model.UUID)
	c.Check(mi.Name, gc.Equals, string(model.Path.Name))
	c.Check(mi.OwnerTag, gc.Equals, conv.ToUserTag(model.Path.User).String())
	c.Check(mi.CloudTag, gc.Equals, conv.ToCloudTag(model.Cloud).String())
	c.Check(mi.CloudRegion, gc.Equals, model.CloudRegion)
	c.Check(mi.Type, gc.Equals, model.Type)
	c.Check(mi.ProviderType, gc.Equals, model.ProviderType)
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

	model := mongodoc.Model{
		Path: params.EntityPath{
			User: "test-user",
			Name: "test-model",
		},
	}

	err := s.conn.CreateModel(ctx, &model)
	c.Assert(err, gc.Equals, nil)

	err = s.conn.GrantModelAccess(ctx, model.UUID, "test-user-2", jujuparams.ModelReadAccess)
	c.Assert(err, gc.Equals, nil)

	mi := jujuparams.ModelInfo{
		UUID: model.UUID,
	}
	err = s.conn.ModelInfo(ctx, &mi)
	c.Assert(err, gc.Equals, nil)

	lessf := func(a, b jujuparams.ModelUserInfo) bool {
		return a.UserName < b.UserName
	}
	c.Check(mi.Users, jemtest.CmpEquals(cmpopts.SortSlices(lessf)), []jujuparams.ModelUserInfo{{
		UserName:    "test-user@external",
		DisplayName: "test-user",
		Access:      "admin",
	}, {
		UserName: "test-user-2@external",
		Access:   "read",
	}})

	err = s.conn.RevokeModelAccess(ctx, model.UUID, "test-user-2", jujuparams.ModelReadAccess)
	c.Assert(err, gc.Equals, nil)

	err = s.conn.ModelInfo(ctx, &mi)
	c.Assert(err, gc.Equals, nil)

	c.Check(mi.Users, jemtest.CmpEquals(cmpopts.SortSlices(lessf)), []jujuparams.ModelUserInfo{{
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

func (s *modelmanagerSuite) TestDestroyModel(c *gc.C) {
	ctx := context.Background()

	model := mongodoc.Model{
		Path: params.EntityPath{
			User: "test-user",
			Name: "test-model",
		},
	}

	err := s.conn.CreateModel(ctx, &model)
	c.Assert(err, gc.Equals, nil)

	err = s.conn.DestroyModel(ctx, model.UUID, nil, nil, nil)
	c.Assert(err, gc.Equals, nil)

	mi := jujuparams.ModelInfo{
		UUID: model.UUID,
	}
	err = s.conn.ModelInfo(ctx, &mi)
	c.Assert(err, gc.Equals, nil)

	c.Check(mi.Life, gc.Equals, life.Dying)
}
