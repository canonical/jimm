package jujuclient_test

import (
	"context"

	jujuparams "github.com/juju/juju/apiserver/params"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/canonical/jimm/internal/jemtest"
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
		Tag: names.NewCloudCredentialTag(jemtest.TestCloudName + "/admin/pw1").String(),
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
		Tag: names.NewCloudCredentialTag(jemtest.TestCloudName + "/admin/pw1").String(),
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
	c.Assert(err, gc.ErrorMatches, `updating cloud credentials: validating credential "`+jemtest.TestCloudName+`/admin/pw1" for cloud "`+jemtest.TestCloudName+`": supported auth-types \["empty" "userpass"\], "bad-type" not supported`)
	c.Assert(models, gc.HasLen, 0)
}

func (s *cloudSuite) TestRevokeCredential(c *gc.C) {
	tag := names.NewCloudCredentialTag(jemtest.TestCloudName + "/admin/pw1")
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
		names.NewCloudTag(jemtest.TestCloudName): jujuparams.Cloud{
			Type:             jemtest.TestProviderType,
			AuthTypes:        []string{"empty", "userpass"},
			Endpoint:         jemtest.TestCloudEndpoint,
			IdentityEndpoint: jemtest.TestCloudIdentityEndpoint,
			StorageEndpoint:  jemtest.TestCloudStorageEndpoint,
			Regions: []jujuparams.CloudRegion{{
				Name:             jemtest.TestCloudRegionName,
				Endpoint:         jemtest.TestCloudEndpoint,
				IdentityEndpoint: jemtest.TestCloudIdentityEndpoint,
				StorageEndpoint:  jemtest.TestCloudStorageEndpoint,
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
	err = s.API.Cloud(ctx, names.NewCloudTag(jemtest.TestCloudName), &cloud)
	c.Assert(err, gc.Equals, nil)

	c.Check(cloud, jc.DeepEquals, clouds[names.NewCloudTag(jemtest.TestCloudName)])
}

func (s *cloudSuite) TestAddCloud(c *gc.C) {
	cloud := jujuparams.Cloud{
		Type:      "kubernetes",
		AuthTypes: []string{"empty"},
	}

	ctx := context.Background()

	err := s.API.AddCloud(ctx, names.NewCloudTag(jemtest.TestCloudName), cloud)
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
	err = s.API.GrantCloudAccess(context.Background(), names.NewCloudTag(jemtest.TestCloudName), names.NewUserTag("user@external"), "add-model")
	c.Check(err, gc.Equals, nil)
}

func (s *cloudSuite) TestRevokeCloudAccess(c *gc.C) {
	err := s.API.RevokeCloudAccess(context.Background(), names.NewCloudTag("no-such-cloud"), names.NewUserTag("user@external"), "add-model")
	c.Check(jujuparams.ErrCode(err), gc.Equals, jujuparams.CodeNotFound)
	err = s.API.GrantCloudAccess(context.Background(), names.NewCloudTag(jemtest.TestCloudName), names.NewUserTag("user@external"), "admin")
	c.Assert(err, gc.Equals, nil)
	err = s.API.RevokeCloudAccess(context.Background(), names.NewCloudTag(jemtest.TestCloudName), names.NewUserTag("user@external"), "admin")
	c.Check(err, gc.Equals, nil)
	err = s.API.RevokeCloudAccess(context.Background(), names.NewCloudTag(jemtest.TestCloudName), names.NewUserTag("user@external"), "add-model")
	c.Check(err, gc.Equals, nil)
	err = s.API.RevokeCloudAccess(context.Background(), names.NewCloudTag(jemtest.TestCloudName), names.NewUserTag("user@external"), "add-model")
	c.Check(jujuparams.ErrCode(err), gc.Equals, jujuparams.CodeNotFound)
}

func (s *cloudSuite) TestCloudInfo(c *gc.C) {
	var ci jujuparams.CloudInfo

	err := s.API.CloudInfo(context.Background(), names.NewCloudTag("no-such-cloud"), &ci)
	c.Check(jujuparams.ErrCode(err), gc.Equals, jujuparams.CodeNotFound)

	err = s.API.CloudInfo(context.Background(), names.NewCloudTag(jemtest.TestCloudName), &ci)
	c.Assert(err, gc.Equals, nil)
	c.Assert(ci, jc.DeepEquals, jujuparams.CloudInfo{
		CloudDetails: jujuparams.CloudDetails{
			Type:             jemtest.TestProviderType,
			AuthTypes:        []string{"empty", "userpass"},
			Endpoint:         jemtest.TestCloudEndpoint,
			IdentityEndpoint: jemtest.TestCloudIdentityEndpoint,
			StorageEndpoint:  jemtest.TestCloudStorageEndpoint,
			Regions: []jujuparams.CloudRegion{{
				Name:             jemtest.TestCloudRegionName,
				Endpoint:         jemtest.TestCloudEndpoint,
				IdentityEndpoint: jemtest.TestCloudIdentityEndpoint,
				StorageEndpoint:  jemtest.TestCloudStorageEndpoint,
			}},
		},
		Users: []jujuparams.CloudUserInfo{{
			UserName:    "admin",
			DisplayName: "admin",
			Access:      "admin",
		}},
	})
}

func (s *cloudSuite) TestUpdateCloud(c *gc.C) {
	var cloud jujuparams.Cloud

	err := s.API.Cloud(context.Background(), names.NewCloudTag(jemtest.TestCloudName), &cloud)
	c.Assert(err, gc.Equals, nil)

	err = s.API.UpdateCloud(context.Background(), names.NewCloudTag("no-such-cloud"), cloud)
	c.Check(jujuparams.ErrCode(err), gc.Equals, jujuparams.CodeNotFound)

	cloud.Endpoint = "new-cloud-endpoint"

	err = s.API.UpdateCloud(context.Background(), names.NewCloudTag(jemtest.TestCloudName), cloud)
	c.Assert(err, gc.Equals, nil)

	var cloud2 jujuparams.Cloud
	err = s.API.Cloud(context.Background(), names.NewCloudTag(jemtest.TestCloudName), &cloud2)
	c.Assert(err, gc.Equals, nil)
	c.Check(cloud2, jc.DeepEquals, cloud)
}
