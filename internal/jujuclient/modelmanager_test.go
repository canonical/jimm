// Copyright 2020 Canonical Ltd.

package jujuclient_test

import (
	"context"
	"fmt"
	"time"

	"github.com/google/go-cmp/cmp/cmpopts"
	jujuparams "github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/life"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v2"
	gc "gopkg.in/check.v1"

	"github.com/CanonicalLtd/jimm/internal/jemtest"
)

type modelmanagerSuite struct {
	jujuclientSuite
}

var _ = gc.Suite(&modelmanagerSuite{})

func (s *modelmanagerSuite) TestCreateModel(c *gc.C) {
	ctx := context.Background()

	var info jujuparams.ModelInfo
	err := s.API.CreateModel(ctx, &jujuparams.ModelCreateArgs{
		Name:     "test-model",
		OwnerTag: names.NewUserTag("test-user@external").String(),
	}, &info)
	c.Assert(err, gc.Equals, nil)

	c.Check(info.UUID, gc.Not(gc.Equals), "")
	c.Check(info.CloudTag, gc.Equals, names.NewCloudTag(jemtest.TestCloudName).String())
	c.Check(info.CloudRegion, gc.Equals, jemtest.TestCloudRegionName)
	c.Check(info.DefaultSeries, gc.Equals, "focal")
	c.Check(string(info.Life), gc.Equals, "alive")
	c.Check(string(info.Status.Status), gc.Equals, "available")
	c.Check(info.Status.Data, gc.IsNil)
	c.Check(info.Status.Since.After(time.Now().Add(-10*time.Second)), gc.Equals, true)
	c.Check(info.Type, gc.Equals, "iaas")
	c.Check(info.ProviderType, gc.Equals, jemtest.TestProviderType)
}

func (s *modelmanagerSuite) TestCreateModelError(c *gc.C) {
	ctx := context.Background()

	var info jujuparams.ModelInfo
	err := s.API.CreateModel(ctx, &jujuparams.ModelCreateArgs{
		Name:     "test-model",
		OwnerTag: names.NewUserTag("test-user@external").String(),
		CloudTag: names.NewCloudTag("nosuchcloud").String(),
	}, &info)
	c.Check(jujuparams.ErrCode(err), gc.Equals, jujuparams.CodeNotFound)
	c.Check(err, gc.ErrorMatches, `cloud "nosuchcloud" not found, expected one of \["`+jemtest.TestCloudName+`"\] \(not found\)`)
}

func (s *modelmanagerSuite) TestGrantJIMMModelAdmin(c *gc.C) {
	ctx := context.Background()

	var info jujuparams.ModelInfo
	err := s.API.CreateModel(ctx, &jujuparams.ModelCreateArgs{
		Name:     "test-model",
		OwnerTag: names.NewUserTag("test-user@external").String(),
	}, &info)
	c.Assert(err, gc.Equals, nil)

	err = s.API.GrantJIMMModelAdmin(ctx, names.NewModelTag(info.UUID))
	c.Assert(err, gc.Equals, nil)

	err = s.API.ModelInfo(ctx, &info)
	c.Assert(err, gc.Equals, nil)

	var access jujuparams.UserAccessPermission
	for _, u := range info.Users {
		if u.UserName == s.APIInfo(c).Tag.Id() {
			access = u.Access
		}
	}
	c.Check(access, gc.Equals, jujuparams.ModelAdminAccess)
}

func (s *modelmanagerSuite) TestGrantJIMMModelAdminError(c *gc.C) {
	ctx := context.Background()

	err := s.API.GrantJIMMModelAdmin(ctx, names.NewModelTag("00000000-0000-0000-0000-000000000000"))
	c.Check(jujuparams.ErrCode(err), gc.Equals, jujuparams.CodeNotFound)
	c.Check(err, gc.ErrorMatches, `could not lookup model: model "00000000-0000-0000-0000-000000000000" not found`)
}

func (s *modelmanagerSuite) TestModelInfo(c *gc.C) {
	ctx := context.Background()

	var info jujuparams.ModelInfo
	err := s.API.CreateModel(ctx, &jujuparams.ModelCreateArgs{
		Name:     "test-model",
		OwnerTag: names.NewUserTag("test-user@external").String(),
	}, &info)
	c.Assert(err, gc.Equals, nil)

	mi := jujuparams.ModelInfo{
		UUID: info.UUID,
	}
	err = s.API.ModelInfo(ctx, &mi)
	c.Assert(err, gc.Equals, nil)

	c.Check(mi, jc.DeepEquals, info)
}

func (s *modelmanagerSuite) TestModelInfoError(c *gc.C) {
	ctx := context.Background()

	mi := jujuparams.ModelInfo{
		UUID: "00000000-0000-0000-0000-000000000000",
	}
	err := s.API.ModelInfo(ctx, &mi)
	c.Check(jujuparams.ErrCode(err), gc.Equals, jujuparams.CodeUnauthorized)
	c.Check(err, gc.ErrorMatches, `permission denied`)
}

func (s *modelmanagerSuite) TestGrantRevokeModel(c *gc.C) {
	ctx := context.Background()

	var info jujuparams.ModelInfo
	err := s.API.CreateModel(ctx, &jujuparams.ModelCreateArgs{
		Name:     "test-model",
		OwnerTag: names.NewUserTag("test-user@external").String(),
	}, &info)
	c.Assert(err, gc.Equals, nil)

	err = s.API.GrantModelAccess(ctx, names.NewModelTag(info.UUID), names.NewUserTag("test-user-2@external"), jujuparams.ModelReadAccess)
	c.Assert(err, gc.Equals, nil)

	err = s.API.ModelInfo(ctx, &info)
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

	err = s.API.RevokeModelAccess(ctx, names.NewModelTag(info.UUID), names.NewUserTag("test-user-2@external"), jujuparams.ModelReadAccess)
	c.Assert(err, gc.Equals, nil)

	err = s.API.ModelInfo(ctx, &info)
	c.Assert(err, gc.Equals, nil)

	c.Check(info.Users, jemtest.CmpEquals(cmpopts.SortSlices(lessf)), []jujuparams.ModelUserInfo{{
		UserName:    "test-user@external",
		DisplayName: "test-user",
		Access:      "admin",
	}})
}

func (s *modelmanagerSuite) TestGrantModelAccessError(c *gc.C) {
	ctx := context.Background()

	err := s.API.GrantModelAccess(ctx, names.NewModelTag("00000000-0000-0000-0000-000000000000"), names.NewUserTag("test-user-2"), jujuparams.ModelReadAccess)

	c.Check(jujuparams.ErrCode(err), gc.Equals, jujuparams.CodeNotFound)
	c.Check(err, gc.ErrorMatches, `could not lookup model: model "00000000-0000-0000-0000-000000000000" not found`)
}

func (s *modelmanagerSuite) TestRevokeModelAccessError(c *gc.C) {
	ctx := context.Background()

	err := s.API.RevokeModelAccess(ctx, names.NewModelTag("00000000-0000-0000-0000-000000000000"), names.NewUserTag("test-user-2"), jujuparams.ModelReadAccess)

	c.Check(jujuparams.ErrCode(err), gc.Equals, jujuparams.CodeNotFound)
	c.Check(err, gc.ErrorMatches, `could not lookup model: model "00000000-0000-0000-0000-000000000000" not found`)
}

func (s *modelmanagerSuite) TestValidateModelUpgrade(c *gc.C) {
	ctx := context.Background()

	args := jujuparams.ModelCreateArgs{
		Name:     "test-model",
		OwnerTag: names.NewUserTag("test-user@external").String(),
	}
	var info jujuparams.ModelInfo

	err := s.API.CreateModel(ctx, &args, &info)
	c.Assert(err, gc.Equals, nil)

	err = s.API.ValidateModelUpgrade(ctx, names.NewModelTag(info.UUID), true)
	c.Assert(err, gc.Equals, nil)

	uuid := utils.MustNewUUID().String()
	err = s.API.ValidateModelUpgrade(ctx, names.NewModelTag(uuid), false)
	c.Assert(err, gc.ErrorMatches, fmt.Sprintf("model %q not found", uuid))
}

func (s *modelmanagerSuite) TestDestroyModel(c *gc.C) {
	ctx := context.Background()

	var info jujuparams.ModelInfo
	err := s.API.CreateModel(ctx, &jujuparams.ModelCreateArgs{
		Name:     "test-model",
		OwnerTag: names.NewUserTag("test-user@external").String(),
	}, &info)
	c.Assert(err, gc.Equals, nil)

	err = s.API.DestroyModel(ctx, names.NewModelTag(info.UUID), nil, nil, nil)
	c.Assert(err, gc.Equals, nil)

	err = s.API.ModelInfo(ctx, &info)
	c.Assert(err, gc.Equals, nil)

	c.Check(info.Life, gc.Equals, life.Dying)
}

func (s *modelmanagerSuite) TestModelStatus(c *gc.C) {
	ctx := context.Background()

	var info jujuparams.ModelInfo
	err := s.API.CreateModel(ctx, &jujuparams.ModelCreateArgs{
		Name:     "test-model",
		OwnerTag: names.NewUserTag("test-user@external").String(),
	}, &info)
	c.Assert(err, gc.Equals, nil)

	status := jujuparams.ModelStatus{
		ModelTag: names.NewModelTag(info.UUID).String(),
	}
	err = s.API.ModelStatus(ctx, &status)
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
	err := s.API.ModelStatus(ctx, &status)
	c.Check(jujuparams.ErrCode(err), gc.Equals, jujuparams.CodeNotFound)
	c.Check(err, gc.ErrorMatches, `model "00000000-0000-0000-0000-000000000000" not found`)
}

func (s *modelmanagerSuite) TestChangeModelCredential(c *gc.C) {
	ctx := context.Background()

	var info jujuparams.ModelInfo
	err := s.API.CreateModel(ctx, &jujuparams.ModelCreateArgs{
		Name:     "test-model",
		OwnerTag: names.NewUserTag("test-user@external").String(),
	}, &info)
	c.Assert(err, gc.Equals, nil)

	ct := names.NewCloudCredentialTag(jemtest.TestCloudName + "/test-owner/test-credential")
	_, err = s.API.UpdateCredential(ctx, jujuparams.TaggedCredential{
		Tag: ct.String(),
		Credential: jujuparams.CloudCredential{
			AuthType: "empty",
		},
	})
	c.Assert(err, gc.Equals, nil)

	err = s.API.ChangeModelCredential(ctx, names.NewModelTag(info.UUID), ct)
	c.Assert(err, gc.Equals, nil)
}
