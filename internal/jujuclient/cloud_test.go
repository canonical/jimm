package jujuclient_test

import (
	"context"

	jujuparams "github.com/juju/juju/apiserver/params"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
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
		Tag: names.NewCloudCredentialTag("dummy/admin/pw1").String(),
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

func (s *cloudSuite) TestUpdateCredential(c *gc.C) {
	cred := jujuparams.TaggedCredential{
		Tag: names.NewCloudCredentialTag("dummy/admin/pw1").String(),
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
	c.Assert(err, gc.ErrorMatches, `updating cloud credentials: validating credential "dummy/admin/pw1" for cloud "dummy": supported auth-types \["empty" "userpass"\], "bad-type" not supported`)
	c.Assert(models, gc.HasLen, 0)
}

func (s *cloudSuite) TestRevokeCredential(c *gc.C) {
	tag := names.NewCloudCredentialTag("dummy/admin/pw1")
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

func (s *cloudSuite) TestClouds(c *gc.C) {
	clouds, err := s.API.Clouds(context.Background())
	c.Assert(err, gc.Equals, nil)
	c.Assert(clouds, jc.DeepEquals, map[names.CloudTag]jujuparams.Cloud{
		names.NewCloudTag("dummy"): jujuparams.Cloud{
			Type:             "dummy",
			AuthTypes:        []string{"empty", "userpass"},
			Endpoint:         "dummy-endpoint",
			IdentityEndpoint: "dummy-identity-endpoint",
			StorageEndpoint:  "dummy-storage-endpoint",
			Regions: []jujuparams.CloudRegion{{
				Name:             "dummy-region",
				Endpoint:         "dummy-endpoint",
				IdentityEndpoint: "dummy-identity-endpoint",
				StorageEndpoint:  "dummy-storage-endpoint",
			}},
			IsControllerCloud: true,
		},
	})
}

func (s *cloudSuite) TestCloud(c *gc.C) {
	ctx := context.Background()

	clouds, err := s.API.Clouds(ctx)
	c.Assert(err, gc.Equals, nil)

	var cloud jujuparams.Cloud
	err = s.API.Cloud(ctx, names.NewCloudTag("dummy"), &cloud)
	c.Assert(err, gc.Equals, nil)

	c.Check(cloud, jc.DeepEquals, clouds[names.NewCloudTag("dummy")])
}

func (s *cloudSuite) TestAddCloud(c *gc.C) {
	cloud := jujuparams.Cloud{
		Type:      "kubernetes",
		AuthTypes: []string{"empty"},
	}

	ctx := context.Background()

	err := s.API.AddCloud(ctx, names.NewCloudTag("dummy"), cloud)
	c.Assert(jujuparams.ErrCode(err), gc.Equals, jujuparams.CodeAlreadyExists)

	err = s.API.AddCloud(ctx, names.NewCloudTag("test-cloud"), cloud)
	c.Assert(err, gc.Equals, nil)

	clouds, err := s.API.Clouds(ctx)
	c.Assert(err, gc.Equals, nil)

	c.Check(clouds[names.NewCloudTag("test-cloud")], jc.DeepEquals, jujuparams.Cloud{
		Type:      "kubernetes",
		AuthTypes: []string{"empty"},
		Regions: []jujuparams.CloudRegion{{
			Name: "default",
		}},
	})
}

func (s *cloudSuite) TestRemoveCloud(c *gc.C) {
	cloud := jujuparams.Cloud{
		Type:      "kubernetes",
		AuthTypes: []string{"empty"},
	}

	ctx := context.Background()

	err := s.API.RemoveCloud(ctx, names.NewCloudTag("test-cloud"))
	c.Check(jujuparams.ErrCode(err), gc.Equals, jujuparams.CodeNotFound)

	err = s.API.AddCloud(ctx, names.NewCloudTag("test-cloud"), cloud)
	c.Assert(err, gc.Equals, nil)

	clouds, err := s.API.Clouds(ctx)
	c.Assert(err, gc.Equals, nil)

	c.Assert(clouds[names.NewCloudTag("test-cloud")], jc.DeepEquals, jujuparams.Cloud{
		Type:      "kubernetes",
		AuthTypes: []string{"empty"},
		Regions: []jujuparams.CloudRegion{{
			Name: "default",
		}},
	})

	err = s.API.RemoveCloud(ctx, names.NewCloudTag("test-cloud"))
	c.Assert(err, gc.Equals, nil)

	clouds, err = s.API.Clouds(ctx)
	c.Assert(err, gc.Equals, nil)

	_, ok := clouds[names.NewCloudTag("test-cloud")]
	c.Assert(ok, gc.Equals, false)
}

func (s *cloudSuite) TestGrantCloudAccess(c *gc.C) {
	err := s.API.GrantCloudAccess(context.Background(), names.NewCloudTag("no-such-cloud"), names.NewUserTag("user@external"), "add-model")
	c.Check(jujuparams.ErrCode(err), gc.Equals, jujuparams.CodeNotFound)
	err = s.API.GrantCloudAccess(context.Background(), names.NewCloudTag("dummy"), names.NewUserTag("user@external"), "add-model")
	c.Check(err, gc.Equals, nil)
}

func (s *cloudSuite) TestRevokeCloudAccess(c *gc.C) {
	err := s.API.RevokeCloudAccess(context.Background(), names.NewCloudTag("no-such-cloud"), names.NewUserTag("user@external"), "add-model")
	c.Check(jujuparams.ErrCode(err), gc.Equals, jujuparams.CodeNotFound)
	err = s.API.GrantCloudAccess(context.Background(), names.NewCloudTag("dummy"), names.NewUserTag("user@external"), "admin")
	c.Assert(err, gc.Equals, nil)
	err = s.API.RevokeCloudAccess(context.Background(), names.NewCloudTag("dummy"), names.NewUserTag("user@external"), "admin")
	c.Check(err, gc.Equals, nil)
	err = s.API.RevokeCloudAccess(context.Background(), names.NewCloudTag("dummy"), names.NewUserTag("user@external"), "add-model")
	c.Check(err, gc.Equals, nil)
	err = s.API.RevokeCloudAccess(context.Background(), names.NewCloudTag("dummy"), names.NewUserTag("user@external"), "add-model")
	c.Check(jujuparams.ErrCode(err), gc.Equals, jujuparams.CodeNotFound)
}
