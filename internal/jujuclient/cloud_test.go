// Copyright 2025 Canonical.

package jujuclient_test

import (
	"context"
	"sort"

	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
)

type cloudSuite struct {
	jujuclientSuite
}

var _ = gc.Suite(&cloudSuite{})

func (s *cloudSuite) TestSupportsCheckCredentialsModels(c *gc.C) {
	c.Assert(s.API.SupportsCheckCredentialModels(), gc.Equals, true)
}

func (s *cloudSuite) TestCheckCredentialModels(c *gc.C) {
	cred := jujuparams.TaggedCredential{
		Tag: names.NewCloudCredentialTag(jimmtest.TestCloudName + "/admin/pw1").String(),
		Credential: jujuparams.CloudCredential{
			AuthType: "userpass",
			Attributes: map[string]string{
				"username": "alibaba",
				"password": "open sesame",
			},
		},
	}

	models, err := s.API.CheckCredentialModels(context.Background(), cred)
	c.Assert(err, gc.Equals, nil)
	c.Assert(models, gc.HasLen, 0)
}

func (s *cloudSuite) TestCheckCredentialModelsWithModels(c *gc.C) {
	ctx := context.Background()

	cct := names.NewCloudCredentialTag(jimmtest.TestCloudName + "/bob@canonical.com/pw1").String()
	cred := jujuparams.TaggedCredential{
		Tag: cct,
		Credential: jujuparams.CloudCredential{
			AuthType: "userpass",
			Attributes: map[string]string{
				"username": "alibaba",
				"password": "open sesame",
			},
		},
	}

	models, err := s.API.UpdateCredential(ctx, cred)
	c.Assert(err, gc.Equals, nil)
	c.Assert(models, gc.HasLen, 0)

	var info jujuparams.ModelInfo
	err = s.API.CreateModel(ctx, &jujuparams.ModelCreateArgs{
		Name:               "model-1",
		OwnerTag:           names.NewUserTag("bob@canonical.com").String(),
		CloudCredentialTag: cct,
	}, &info)
	c.Assert(err, gc.Equals, nil)
	uuid1 := info.UUID

	err = s.API.CreateModel(ctx, &jujuparams.ModelCreateArgs{
		Name:               "model-2",
		OwnerTag:           names.NewUserTag("bob@canonical.com").String(),
		CloudCredentialTag: cct,
	}, &info)
	c.Assert(err, gc.Equals, nil)
	uuid2 := info.UUID

	expectModels := []jujuparams.UpdateCredentialModelResult{{
		ModelUUID: uuid1,
		ModelName: "model-1",
	}, {
		ModelUUID: uuid2,
		ModelName: "model-2",
	}}

	cred = jujuparams.TaggedCredential{
		Tag: cct,
		Credential: jujuparams.CloudCredential{
			AuthType: "userpass",
			Attributes: map[string]string{
				"username": "alibaba",
				"password": "new password",
			},
		},
	}

	models, err = s.API.CheckCredentialModels(ctx, cred)
	c.Assert(err, gc.Equals, nil)
	c.Assert(models, gc.HasLen, 2)
	sort.Slice(models, func(i, j int) bool {
		return models[i].ModelName < models[j].ModelName
	})
	c.Assert(models, jc.DeepEquals, expectModels)
}

func (s *cloudSuite) TestUpdateCredential(c *gc.C) {
	cred := jujuparams.TaggedCredential{
		Tag: names.NewCloudCredentialTag(jimmtest.TestCloudName + "/admin/pw1").String(),
		Credential: jujuparams.CloudCredential{
			AuthType: "userpass",
			Attributes: map[string]string{
				"username": "alibaba",
				"password": "open sesame",
			},
		},
	}

	models, err := s.API.UpdateCredential(context.Background(), cred)
	c.Assert(err, gc.Equals, nil)
	c.Assert(models, gc.HasLen, 0)

	cred.Credential.AuthType = "bad-type"

	models, err = s.API.UpdateCredential(context.Background(), cred)
	c.Assert(err, gc.ErrorMatches, `updating cloud credentials: validating credential "`+jimmtest.TestCloudName+`/admin/pw1" for cloud "`+jimmtest.TestCloudName+`": supported auth-types \["empty" "userpass"\], "bad-type" not supported`)
	c.Assert(models, gc.HasLen, 0)
}

func (s *cloudSuite) TestRevokeCredential(c *gc.C) {
	tag := names.NewCloudCredentialTag(jimmtest.TestCloudName + "/admin/pw1")
	cred := jujuparams.TaggedCredential{
		Tag: tag.String(),
		Credential: jujuparams.CloudCredential{
			AuthType: "userpass",
			Attributes: map[string]string{
				"username": "alibaba",
				"password": "open sesame",
			},
		},
	}

	models, err := s.API.UpdateCredential(context.Background(), cred)
	c.Assert(err, gc.Equals, nil)
	c.Assert(models, gc.HasLen, 0)

	err = s.API.RevokeCredential(context.Background(), tag)
	c.Assert(err, gc.Equals, nil)
}

func (s *cloudSuite) TestUpdateCredentialWithModels(c *gc.C) {
	ctx := context.Background()

	cct := names.NewCloudCredentialTag(jimmtest.TestCloudName + "/bob@canonical.com/pw1").String()
	cred := jujuparams.TaggedCredential{
		Tag: cct,
		Credential: jujuparams.CloudCredential{
			AuthType: "userpass",
			Attributes: map[string]string{
				"username": "alibaba",
				"password": "open sesame",
			},
		},
	}

	models, err := s.API.UpdateCredential(ctx, cred)
	c.Assert(err, gc.Equals, nil)
	c.Assert(models, gc.HasLen, 0)

	var info jujuparams.ModelInfo
	err = s.API.CreateModel(ctx, &jujuparams.ModelCreateArgs{
		Name:               "model-1",
		OwnerTag:           names.NewUserTag("bob@canonical.com").String(),
		CloudCredentialTag: cct,
	}, &info)
	c.Assert(err, gc.Equals, nil)
	uuid1 := info.UUID

	err = s.API.CreateModel(ctx, &jujuparams.ModelCreateArgs{
		Name:               "model-2",
		OwnerTag:           names.NewUserTag("bob@canonical.com").String(),
		CloudCredentialTag: cct,
	}, &info)
	c.Assert(err, gc.Equals, nil)
	uuid2 := info.UUID

	expectModels := []jujuparams.UpdateCredentialModelResult{{
		ModelUUID: uuid1,
		ModelName: "model-1",
	}, {
		ModelUUID: uuid2,
		ModelName: "model-2",
	}}

	cred = jujuparams.TaggedCredential{
		Tag: cct,
		Credential: jujuparams.CloudCredential{
			AuthType: "userpass",
			Attributes: map[string]string{
				"username": "alibaba",
				"password": "new password",
			},
		},
	}

	models, err = s.API.UpdateCredential(ctx, cred)
	c.Assert(err, gc.Equals, nil)
	c.Assert(models, gc.HasLen, 2)
	sort.Slice(models, func(i, j int) bool {
		return models[i].ModelName < models[j].ModelName
	})
	c.Assert(models, jc.DeepEquals, expectModels)
}
