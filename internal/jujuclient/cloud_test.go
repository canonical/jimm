// Copyright 2024 Canonical.
package jujuclient_test

import (
	"context"
	"sort"
	"strings"

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

func (s *cloudSuite) TestClouds(c *gc.C) {
	clouds, err := s.API.Clouds(context.Background())
	c.Assert(err, gc.Equals, nil)
	c.Assert(clouds, jc.DeepEquals, map[names.CloudTag]jujuparams.Cloud{
		names.NewCloudTag(jimmtest.TestCloudName): {
			Type:             jimmtest.TestProviderType,
			AuthTypes:        []string{"empty", "userpass"},
			Endpoint:         jimmtest.TestCloudEndpoint,
			IdentityEndpoint: jimmtest.TestCloudIdentityEndpoint,
			StorageEndpoint:  jimmtest.TestCloudStorageEndpoint,
			Regions: []jujuparams.CloudRegion{{
				Name:             jimmtest.TestCloudRegionName,
				Endpoint:         jimmtest.TestCloudEndpoint,
				IdentityEndpoint: jimmtest.TestCloudIdentityEndpoint,
				StorageEndpoint:  jimmtest.TestCloudStorageEndpoint,
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
	err = s.API.Cloud(ctx, names.NewCloudTag(jimmtest.TestCloudName), &cloud)
	c.Assert(err, gc.Equals, nil)

	c.Check(cloud, jc.DeepEquals, clouds[names.NewCloudTag(jimmtest.TestCloudName)])
}

func (s *cloudSuite) TestAddCloud(c *gc.C) {
	tests := []struct {
		about             string
		cloudInfo         jujuparams.Cloud
		force             bool
		cloudName         string
		expectedError     string
		expectedCloudInfo jujuparams.Cloud
	}{
		{
			about: "Add cloud",
			cloudInfo: jujuparams.Cloud{
				Type:      "kubernetes",
				AuthTypes: []string{"empty"},
			},
			force:     false,
			cloudName: "test-cloud",
			expectedCloudInfo: jujuparams.Cloud{
				Type:      "kubernetes",
				AuthTypes: []string{"empty"},
				Regions: []jujuparams.CloudRegion{{
					Name: "default",
				}},
			},
		}, {
			about: "Add existing cloud",
			cloudInfo: jujuparams.Cloud{
				Type:      "kubernetes",
				AuthTypes: []string{"empty"},
			},
			force:         false,
			cloudName:     jimmtest.TestCloudName,
			expectedError: ".*already exists.*",
		}, {
			about: "Add incompatible cloud",
			cloudInfo: jujuparams.Cloud{
				Type:      "fake-cloud",
				AuthTypes: []string{"empty"},
			},
			force:         false,
			cloudName:     "fake-cloud",
			expectedError: ".*incompatible clouds.*",
		}, {
			about: "Add incompatible cloud by force",
			cloudInfo: jujuparams.Cloud{
				Type:      "fake-cloud",
				AuthTypes: []string{"empty"},
			},
			force:     true,
			cloudName: "fake-cloud-2",
			expectedCloudInfo: jujuparams.Cloud{
				Type:      "fake-cloud",
				AuthTypes: []string{"empty"},
				Regions: []jujuparams.CloudRegion{{
					Name: "default",
				}},
			},
		},
	}

	for _, test := range tests {
		c.Log(test.about)
		ctx := context.Background()
		err := s.API.AddCloud(ctx, names.NewCloudTag(test.cloudName), test.cloudInfo, test.force)
		if test.expectedError != "" {
			c.Assert(err, gc.NotNil)
			errWithoutBreaks := strings.ReplaceAll(err.Error(), "\n", "")
			c.Assert(errWithoutBreaks, gc.Matches, test.expectedError)
			c.Assert(errWithoutBreaks, gc.Matches, test.expectedError)
		} else {
			c.Assert(err, gc.IsNil)
			clouds, err := s.API.Clouds(ctx)
			c.Assert(err, gc.Equals, nil)
			c.Check(clouds[names.NewCloudTag(test.cloudName)], jc.DeepEquals, test.expectedCloudInfo)
		}
	}

}

func (s *cloudSuite) TestAddCloudFailsWithIncompatibleClouds(c *gc.C) {
	cloud := jujuparams.Cloud{
		Type:      "fake-cloud",
		AuthTypes: []string{"empty"},
	}

	ctx := context.Background()

	err := s.API.AddCloud(ctx, names.NewCloudTag("test-cloud"), cloud, false)
	c.Assert(jujuparams.ErrCode(err), gc.Equals, jujuparams.CodeIncompatibleClouds)
}

func (s *cloudSuite) TestAddIncompatibleCloudByForce(c *gc.C) {
	cloud := jujuparams.Cloud{
		Type:      "fake-cloud",
		AuthTypes: []string{"empty"},
	}

	ctx := context.Background()

	err := s.API.AddCloud(ctx, names.NewCloudTag("test-cloud"), cloud, true)
	c.Assert(err, gc.Equals, nil)

	clouds, err := s.API.Clouds(ctx)
	c.Assert(err, gc.Equals, nil)

	c.Check(clouds[names.NewCloudTag("test-cloud")], jc.DeepEquals, jujuparams.Cloud{
		Type:      "fake-cloud",
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

	err = s.API.AddCloud(ctx, names.NewCloudTag("test-cloud"), cloud, false)
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
	err := s.API.GrantCloudAccess(context.Background(), names.NewCloudTag("no-such-cloud"), names.NewUserTag("user@canonical.com"), "add-model")
	c.Check(jujuparams.ErrCode(err), gc.Equals, jujuparams.CodeNotFound)
	err = s.API.GrantCloudAccess(context.Background(), names.NewCloudTag(jimmtest.TestCloudName), names.NewUserTag("user@canonical.com"), "add-model")
	c.Check(err, gc.Equals, nil)
}

func (s *cloudSuite) TestRevokeCloudAccess(c *gc.C) {
	err := s.API.RevokeCloudAccess(context.Background(), names.NewCloudTag("no-such-cloud"), names.NewUserTag("user@canonical.com"), "add-model")
	c.Check(jujuparams.ErrCode(err), gc.Equals, jujuparams.CodeNotFound)
	err = s.API.GrantCloudAccess(context.Background(), names.NewCloudTag(jimmtest.TestCloudName), names.NewUserTag("user@canonical.com"), "admin")
	c.Assert(err, gc.Equals, nil)
	err = s.API.RevokeCloudAccess(context.Background(), names.NewCloudTag(jimmtest.TestCloudName), names.NewUserTag("user@canonical.com"), "admin")
	c.Check(err, gc.Equals, nil)
	err = s.API.RevokeCloudAccess(context.Background(), names.NewCloudTag(jimmtest.TestCloudName), names.NewUserTag("user@canonical.com"), "add-model")
	c.Check(err, gc.Equals, nil)
	err = s.API.RevokeCloudAccess(context.Background(), names.NewCloudTag(jimmtest.TestCloudName), names.NewUserTag("user@canonical.com"), "add-model")
	c.Check(jujuparams.ErrCode(err), gc.Equals, jujuparams.CodeNotFound)
}

func (s *cloudSuite) TestCloudInfo(c *gc.C) {
	var ci jujuparams.CloudInfo

	err := s.API.CloudInfo(context.Background(), names.NewCloudTag("no-such-cloud"), &ci)
	c.Check(jujuparams.ErrCode(err), gc.Equals, jujuparams.CodeNotFound)

	err = s.API.CloudInfo(context.Background(), names.NewCloudTag(jimmtest.TestCloudName), &ci)
	c.Assert(err, gc.Equals, nil)
	c.Assert(ci, jc.DeepEquals, jujuparams.CloudInfo{
		CloudDetails: jujuparams.CloudDetails{
			Type:             jimmtest.TestProviderType,
			AuthTypes:        []string{"empty", "userpass"},
			Endpoint:         jimmtest.TestCloudEndpoint,
			IdentityEndpoint: jimmtest.TestCloudIdentityEndpoint,
			StorageEndpoint:  jimmtest.TestCloudStorageEndpoint,
			Regions: []jujuparams.CloudRegion{{
				Name:             jimmtest.TestCloudRegionName,
				Endpoint:         jimmtest.TestCloudEndpoint,
				IdentityEndpoint: jimmtest.TestCloudIdentityEndpoint,
				StorageEndpoint:  jimmtest.TestCloudStorageEndpoint,
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

	err := s.API.Cloud(context.Background(), names.NewCloudTag(jimmtest.TestCloudName), &cloud)
	c.Assert(err, gc.Equals, nil)

	err = s.API.UpdateCloud(context.Background(), names.NewCloudTag("no-such-cloud"), cloud)
	c.Check(jujuparams.ErrCode(err), gc.Equals, jujuparams.CodeNotFound)

	cloud.Endpoint = "new-endpoint"

	err = s.API.UpdateCloud(context.Background(), names.NewCloudTag(jimmtest.TestCloudName), cloud)
	c.Assert(err, gc.Equals, nil)

	var cloud2 jujuparams.Cloud
	err = s.API.Cloud(context.Background(), names.NewCloudTag(jimmtest.TestCloudName), &cloud2)
	c.Assert(err, gc.Equals, nil)
	c.Check(cloud2, jc.DeepEquals, cloud)
}
