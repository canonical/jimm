// Copyright 2024 Canonical.

package jujuclient_test

import (
	"context"
	"fmt"
	"time"

	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/juju/juju/core/life"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v2"
	gc "gopkg.in/check.v1"

	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
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
		OwnerTag: names.NewUserTag("test-user@canonical.com").String(),
	}, &info)
	c.Assert(err, gc.Equals, nil)

	c.Check(info.UUID, gc.Not(gc.Equals), "")
	c.Check(info.CloudTag, gc.Equals, names.NewCloudTag(jimmtest.TestCloudName).String())
	c.Check(info.CloudRegion, gc.Equals, jimmtest.TestCloudRegionName)
	c.Check(info.DefaultSeries, gc.Equals, "jammy")
	c.Check(string(info.Life), gc.Equals, state.Alive.String())
	c.Check(string(info.Status.Status), gc.Equals, "available")
	c.Check(info.Status.Data, gc.IsNil)
	c.Check(info.Status.Since.After(time.Now().Add(-10*time.Second)), gc.Equals, true)
	c.Check(info.Type, gc.Equals, "iaas")
	c.Check(info.ProviderType, gc.Equals, jimmtest.TestProviderType)
}

func (s *modelmanagerSuite) TestCreateModelError(c *gc.C) {
	ctx := context.Background()

	var info jujuparams.ModelInfo
	err := s.API.CreateModel(ctx, &jujuparams.ModelCreateArgs{
		Name:     "test-model",
		OwnerTag: names.NewUserTag("test-user@canonical.com").String(),
		CloudTag: names.NewCloudTag("nosuchcloud").String(),
	}, &info)
	c.Check(jujuparams.ErrCode(err), gc.Equals, jujuparams.CodeNotFound)
	c.Check(err, gc.ErrorMatches, `cloud "nosuchcloud" not found, expected one of \["`+jimmtest.TestCloudName+`"\] \(not found\)`)
}

func (s *modelmanagerSuite) TestGrantJIMMModelAdmin(c *gc.C) {
	ctx := context.Background()

	var info jujuparams.ModelInfo
	err := s.API.CreateModel(ctx, &jujuparams.ModelCreateArgs{
		Name:     "test-model",
		OwnerTag: names.NewUserTag("test-user@canonical.com").String(),
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
		OwnerTag: names.NewUserTag("test-user@canonical.com").String(),
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
		OwnerTag: names.NewUserTag("test-user@canonical.com").String(),
	}, &info)
	c.Assert(err, gc.Equals, nil)

	err = s.API.GrantModelAccess(ctx, names.NewModelTag(info.UUID), names.NewUserTag("test-user-2@canonical.com"), jujuparams.ModelReadAccess)
	c.Assert(err, gc.Equals, nil)

	err = s.API.ModelInfo(ctx, &info)
	c.Assert(err, gc.Equals, nil)

	lessf := func(a, b jujuparams.ModelUserInfo) bool {
		return a.UserName < b.UserName
	}
	c.Check(info.Users, jimmtest.CmpEquals(cmpopts.SortSlices(lessf)), []jujuparams.ModelUserInfo{{
		UserName:    "test-user@canonical.com",
		DisplayName: "test-user",
		Access:      "admin",
	}, {
		UserName: "test-user-2@canonical.com",
		Access:   "read",
	}})

	err = s.API.RevokeModelAccess(ctx, names.NewModelTag(info.UUID), names.NewUserTag("test-user-2@canonical.com"), jujuparams.ModelReadAccess)
	c.Assert(err, gc.Equals, nil)

	err = s.API.ModelInfo(ctx, &info)
	c.Assert(err, gc.Equals, nil)

	c.Check(info.Users, jimmtest.CmpEquals(cmpopts.SortSlices(lessf)), []jujuparams.ModelUserInfo{{
		UserName:    "test-user@canonical.com",
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
	c.Skip("juju 3.x no longer implements this method")
	ctx := context.Background()

	args := jujuparams.ModelCreateArgs{
		Name:     "test-model",
		OwnerTag: names.NewUserTag("test-user@canonical.com").String(),
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
		OwnerTag: names.NewUserTag("test-user@canonical.com").String(),
	}, &info)
	c.Assert(err, gc.Equals, nil)

	err = s.API.DestroyModel(ctx, names.NewModelTag(info.UUID), nil, nil, nil, nil)
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
		OwnerTag: names.NewUserTag("test-user@canonical.com").String(),
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
		OwnerTag: names.NewUserTag("test-user@canonical.com").String(),
	}, &info)
	c.Assert(err, gc.Equals, nil)

	ct := names.NewCloudCredentialTag(jimmtest.TestCloudName + "/test-owner/test-credential")
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
