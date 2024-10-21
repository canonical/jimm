// Copyright 2024 Canonical.

package jujuapi_test

import (
	"context"
	"fmt"
	"sort"

	cloudapi "github.com/juju/juju/api/client/cloud"
	"github.com/juju/juju/api/client/modelmanager"
	"github.com/juju/juju/cloud"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/openfga"
	ofganames "github.com/canonical/jimm/v3/internal/openfga/names"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
)

type cloudSuite struct {
	websocketSuite
}

var _ = gc.Suite(&cloudSuite{})

func (s *cloudSuite) TestCloudCall(c *gc.C) {
	conn := s.open(c, nil, "test")
	defer conn.Close()
	client := cloudapi.NewClient(conn)
	info, err := client.Cloud(names.NewCloudTag(jimmtest.TestCloudName))
	c.Assert(err, gc.Equals, nil)
	c.Assert(info, jc.DeepEquals, cloud.Cloud{
		Name:      jimmtest.TestCloudName,
		Type:      jimmtest.TestProviderType,
		AuthTypes: cloud.AuthTypes{"empty", "userpass"},
		Regions: []cloud.Region{{
			Name:             jimmtest.TestCloudRegionName,
			Endpoint:         jimmtest.TestCloudEndpoint,
			IdentityEndpoint: jimmtest.TestCloudIdentityEndpoint,
			StorageEndpoint:  jimmtest.TestCloudStorageEndpoint,
		}},
		Endpoint:         jimmtest.TestCloudEndpoint,
		IdentityEndpoint: jimmtest.TestCloudIdentityEndpoint,
		StorageEndpoint:  jimmtest.TestCloudStorageEndpoint,
	})
}

func (s *cloudSuite) TestClouds(c *gc.C) {
	conn := s.open(c, nil, "bob")
	defer conn.Close()

	client := cloudapi.NewClient(conn)
	clouds, err := client.Clouds()
	c.Assert(err, gc.Equals, nil)
	c.Assert(clouds, jc.DeepEquals, map[names.CloudTag]cloud.Cloud{
		names.NewCloudTag(jimmtest.TestCloudName): {
			Name:      jimmtest.TestCloudName,
			Type:      jimmtest.TestProviderType,
			AuthTypes: cloud.AuthTypes{"empty", "userpass"},
			Regions: []cloud.Region{{
				Name:             jimmtest.TestCloudRegionName,
				Endpoint:         jimmtest.TestCloudEndpoint,
				IdentityEndpoint: jimmtest.TestCloudIdentityEndpoint,
				StorageEndpoint:  jimmtest.TestCloudStorageEndpoint,
			}},
			Endpoint:         jimmtest.TestCloudEndpoint,
			IdentityEndpoint: jimmtest.TestCloudIdentityEndpoint,
			StorageEndpoint:  jimmtest.TestCloudStorageEndpoint,
		},
	})
}

func (s *cloudSuite) TestUserCredentials(c *gc.C) {
	conn := s.open(c, nil, "bob")
	defer conn.Close()
	client := cloudapi.NewClient(conn)
	creds, err := client.UserCredentials(names.NewUserTag("bob@canonical.com"), names.NewCloudTag(jimmtest.TestCloudName))
	c.Assert(err, gc.Equals, nil)
	c.Assert(creds, jc.DeepEquals, []names.CloudCredentialTag{
		names.NewCloudCredentialTag(jimmtest.TestCloudName + "/bob@canonical.com/cred"),
	})
}

func (s *cloudSuite) TestUserCredentialsWithDomain(c *gc.C) {
	cct := names.NewCloudCredentialTag(jimmtest.TestCloudName + "/test@domain/cred1")
	s.UpdateCloudCredential(c, cct, jujuparams.CloudCredential{
		AuthType: "credtype",
		Attributes: map[string]string{
			"attr1": "val1",
			"attr2": "val2",
		},
	})
	conn := s.open(c, nil, "test@domain")
	defer conn.Close()
	client := cloudapi.NewClient(conn)
	creds, err := client.UserCredentials(names.NewUserTag("test@domain"), names.NewCloudTag(jimmtest.TestCloudName))
	c.Assert(err, gc.Equals, nil)
	c.Assert(creds, jc.DeepEquals, []names.CloudCredentialTag{
		cct,
	})
}

func (s *cloudSuite) TestUserCredentialsErrors(c *gc.C) {
	conn := s.open(c, nil, "test")
	defer conn.Close()
	req := jujuparams.UserClouds{
		UserClouds: []jujuparams.UserCloud{{
			UserTag:  "not-a-user-tag",
			CloudTag: jimmtest.TestCloudName,
		}},
	}
	var resp jujuparams.StringsResults
	err := conn.APICall("Cloud", 7, "", "UserCredentials", req, &resp)
	c.Assert(err, gc.Equals, nil)
	c.Assert(resp.Results[0].Error, gc.ErrorMatches, `"not-a-user-tag" is not a valid tag`)
	c.Assert(resp.Results, gc.HasLen, 1)
}

func (s *cloudSuite) TestUpdateCloudCredentials(c *gc.C) {
	conn := s.open(c, nil, "test")
	defer conn.Close()
	client := cloudapi.NewClient(conn)
	credentialTag := names.NewCloudCredentialTag(fmt.Sprintf(jimmtest.TestCloudName + "/test@canonical.com/cred3"))
	reqCreds := map[string]cloud.Credential{
		credentialTag.String(): cloud.NewCredential("credtype", map[string]string{
			"attr1": "val31",
			"attr2": "val32",
		}),
	}
	res, err := client.UpdateCloudsCredentials(reqCreds, false)
	c.Assert(err, gc.Equals, nil)
	c.Assert(res, gc.DeepEquals, []jujuparams.UpdateCredentialResult{{
		CredentialTag: credentialTag.String(),
	}})
	creds, err := client.UserCredentials(names.NewUserTag("test@canonical.com"), names.NewCloudTag(jimmtest.TestCloudName))
	c.Assert(err, gc.Equals, nil)
	c.Assert(creds, jc.DeepEquals, []names.CloudCredentialTag{credentialTag})
	_, err = client.UpdateCredentialsCheckModels(credentialTag, cloud.NewCredential("credtype", map[string]string{"attr1": "val33", "attr2": "val34"}))
	c.Assert(err, gc.Equals, nil)
	creds, err = client.UserCredentials(names.NewUserTag("test@canonical.com"), names.NewCloudTag(jimmtest.TestCloudName))
	c.Assert(err, gc.Equals, nil)
	var _ = creds
}

func (s *cloudSuite) TestUpdateCloudCredentialsErrors(c *gc.C) {
	conn := s.open(c, nil, "test")
	defer conn.Close()
	req := jujuparams.TaggedCredentials{
		Credentials: []jujuparams.TaggedCredential{{
			Tag: "not-a-cloud-credentials-tag",
			Credential: jujuparams.CloudCredential{
				AuthType: "credtype",
				Attributes: map[string]string{
					"attr1": "val1",
				},
			},
		}, {
			Tag: names.NewCloudCredentialTag(jimmtest.TestCloudName + "/test2@canonical.com/cred1").String(),
			Credential: jujuparams.CloudCredential{
				AuthType: "credtype",
				Attributes: map[string]string{
					"attr1": "val1",
				},
			},
		}, {
			Tag: names.NewCloudCredentialTag(jimmtest.TestCloudName + "/test@canonical.com/bad-name-").String(),
			Credential: jujuparams.CloudCredential{
				AuthType: "credtype",
				Attributes: map[string]string{
					"attr1": "val1",
				},
			},
		}},
	}
	var resp jujuparams.ErrorResults
	err := conn.APICall("Cloud", 7, "", "UpdateCredentialsCheckModels", req, &resp)
	c.Assert(err, gc.Equals, nil)
	c.Assert(resp.Results, gc.HasLen, 3)
	c.Assert(resp.Results[0].Error, gc.ErrorMatches, `"not-a-cloud-credentials-tag" is not a valid tag`)
	c.Assert(resp.Results[1].Error, gc.ErrorMatches, `unauthorized`)
	c.Assert(resp.Results[2].Error, gc.IsNil)
}

func (s *cloudSuite) TestUpdateCloudCredentialsForce(c *gc.C) {
	conn := s.open(c, nil, "test")
	defer conn.Close()
	client := cloudapi.NewClient(conn)
	credentialTag := names.NewCloudCredentialTag(fmt.Sprintf(jimmtest.TestCloudName + "/test@canonical.com/cred3"))
	_, err := client.UpdateCredentialsCheckModels(credentialTag, cloud.NewCredential("userpass", map[string]string{"username": "a", "password": "b"}))
	c.Assert(err, gc.Equals, nil)

	mmclient := modelmanager.NewClient(conn)
	_, err = mmclient.CreateModel("model1", "test@canonical.com", jimmtest.TestCloudName, "", credentialTag, nil)
	c.Assert(err, gc.Equals, nil)

	args := jujuparams.UpdateCredentialArgs{
		Credentials: []jujuparams.TaggedCredential{{
			Tag: credentialTag.String(),
			Credential: jujuparams.CloudCredential{
				AuthType: "badauthtype",
				Attributes: map[string]string{
					"bad1attr": "cloud-user2",
					"bad2attr": "cloud-pass2",
				},
			},
		}},
	}
	// First try without Force to check that it fails.
	var resp jujuparams.UpdateCredentialResults
	err = conn.APICall("Cloud", 7, "", "UpdateCredentialsCheckModels", args, &resp)
	c.Assert(err, gc.Equals, nil)
	c.Assert(resp.Results[0].Error, gc.ErrorMatches, `some models are no longer visible`)

	// Check that the credentials have not been updated.
	creds, err := client.Credentials(credentialTag)
	c.Assert(err, gc.Equals, nil)
	c.Assert(creds, jc.DeepEquals, []jujuparams.CloudCredentialResult{{
		Result: &jujuparams.CloudCredential{
			AuthType: "userpass",
			Attributes: map[string]string{
				"username": "a",
			},
			Redacted: []string{
				"password",
			},
		},
	}})

	args.Force = true
	err = conn.APICall("Cloud", 7, "", "UpdateCredentialsCheckModels", args, &resp)
	c.Assert(err, gc.Equals, nil)
	c.Check(resp.Results[0].Error, gc.ErrorMatches, `updating cloud credentials: validating credential "`+jimmtest.TestCloudName+`/test@canonical.com/cred3" for cloud "`+jimmtest.TestCloudName+`": supported auth-types \["empty" "userpass"\], "badauthtype" not supported`)

	// Check that the credentials have been updated even though
	// we got an error.
	creds, err = client.Credentials(credentialTag)
	c.Assert(err, gc.Equals, nil)
	sort.Strings(creds[0].Result.Redacted)
	c.Assert(creds, jc.DeepEquals, []jujuparams.CloudCredentialResult{{
		Result: &jujuparams.CloudCredential{
			AuthType: "badauthtype",
			Redacted: []string{"bad1attr", "bad2attr"},
		},
	}})
}

func (s *cloudSuite) TestCheckCredentialsModels(c *gc.C) {
	conn := s.open(c, nil, "test")
	defer conn.Close()

	credTag := names.NewCloudCredentialTag(jimmtest.TestCloudName + "/test@canonical.com/cred")
	cred1 := cloud.NewCredential("userpass", map[string]string{
		"username": "cloud-user",
		"password": "cloud-pass",
	})

	client := cloudapi.NewClient(conn)
	_, err := client.UpdateCredentialsCheckModels(credTag, cred1)
	c.Assert(err, gc.Equals, nil)

	mmclient := modelmanager.NewClient(conn)
	model1, err := mmclient.CreateModel("model1", "test@canonical.com", jimmtest.TestCloudName, "", credTag, nil)
	c.Assert(err, gc.Equals, nil)

	model2, err := mmclient.CreateModel("model2", "test@canonical.com", jimmtest.TestCloudName, "", credTag, nil)
	c.Assert(err, gc.Equals, nil)

	var resp jujuparams.UpdateCredentialResults
	err = conn.APICall("Cloud", 7, "", "CheckCredentialsModels", jujuparams.TaggedCredentials{
		Credentials: []jujuparams.TaggedCredential{{
			Tag: credTag.String(),
			Credential: jujuparams.CloudCredential{
				AuthType: "userpass",
				Attributes: map[string]string{
					"username": "cloud-user2",
					"password": "cloud-pass2",
				},
			},
		}},
	}, &resp)
	c.Assert(err, gc.Equals, nil)
	modelResults := []jujuparams.UpdateCredentialModelResult{{
		ModelUUID: model1.UUID,
		ModelName: "model1",
	}, {
		ModelUUID: model2.UUID,
		ModelName: "model2",
	}}
	sort.Slice(modelResults, func(i, j int) bool {
		return modelResults[i].ModelUUID < modelResults[j].ModelUUID
	})
	c.Assert(resp, jc.DeepEquals, jujuparams.UpdateCredentialResults{
		Results: []jujuparams.UpdateCredentialResult{{
			CredentialTag: credTag.String(),
			Models:        modelResults,
		}},
	})
}

func (s *cloudSuite) TestCheckCredentialsModelsInvalidCreds(c *gc.C) {
	conn := s.open(c, nil, "test")
	defer conn.Close()

	credTag := names.NewCloudCredentialTag(jimmtest.TestCloudName + "/test@canonical.com/cred")
	cred1 := cloud.NewCredential("userpass", map[string]string{
		"username": "cloud-user",
		"password": "cloud-pass",
	})

	client := cloudapi.NewClient(conn)
	_, err := client.UpdateCredentialsCheckModels(credTag, cred1)
	c.Assert(err, gc.Equals, nil)

	mmclient := modelmanager.NewClient(conn)
	model1, err := mmclient.CreateModel("model1", "test@canonical.com", jimmtest.TestCloudName, "", credTag, nil)
	c.Assert(err, gc.Equals, nil)

	var resp jujuparams.UpdateCredentialResults
	err = conn.APICall("Cloud", 7, "", "CheckCredentialsModels", jujuparams.TaggedCredentials{
		Credentials: []jujuparams.TaggedCredential{{
			Tag: credTag.String(),
			Credential: jujuparams.CloudCredential{
				AuthType: "unknowntype",
				Attributes: map[string]string{
					"x": "y",
				},
			},
		}},
	}, &resp)
	c.Assert(err, gc.Equals, nil)
	c.Assert(resp, jc.DeepEquals, jujuparams.UpdateCredentialResults{
		Results: []jujuparams.UpdateCredentialResult{{
			CredentialTag: "cloudcred-" + jimmtest.TestCloudName + "_test@canonical.com_cred",
			Error:         &jujuparams.Error{Message: "some models are no longer visible"},
			Models: []jujuparams.UpdateCredentialModelResult{{
				ModelUUID: model1.UUID,
				ModelName: "model1",
				Errors: []jujuparams.ErrorResult{{
					Error: &jujuparams.Error{
						Message: `validating credential "` + jimmtest.TestCloudName + `/test@canonical.com/cred" for cloud "` + jimmtest.TestCloudName + `": supported auth-types ["empty" "userpass"], "unknowntype" not supported`,
						Code:    "not supported",
					},
				}},
			}},
		}},
	})
}

func (s *cloudSuite) TestCredential(c *gc.C) {
	conn := s.open(c, nil, "test")
	defer conn.Close()

	cred1Tag := names.NewCloudCredentialTag(jimmtest.TestCloudName + "/test@canonical.com/cred1")
	cred1 := cloud.NewCredential("userpass", map[string]string{
		"username": "cloud-user",
		"password": "cloud-pass",
	})
	cred2Tag := names.NewCloudCredentialTag(jimmtest.TestCloudName + "/test@canonical.com/cred2")
	cred2 := cloud.NewCredential("empty", nil)

	client := cloudapi.NewClient(conn)
	_, err := client.UpdateCredentialsCheckModels(cred1Tag, cred1)
	c.Assert(err, gc.Equals, nil)
	_, err = client.UpdateCredentialsCheckModels(cred2Tag, cred2)
	c.Assert(err, gc.Equals, nil)

	creds, err := client.Credentials(
		cred1Tag,
		cred2Tag,
		names.NewCloudCredentialTag(jimmtest.TestCloudName+"/test@canonical.com/cred3"),
		names.NewCloudCredentialTag(jimmtest.TestCloudName+"/no-test@canonical.com/cred4"),
		names.NewCloudCredentialTag(jimmtest.TestCloudName+"/admin@local/cred6"),
	)
	c.Assert(err, gc.Equals, nil)
	for i := range creds {
		if creds[i].Result == nil {
			continue
		}
		sort.Strings(creds[i].Result.Redacted)
	}
	c.Assert(creds, jc.SameContents, []jujuparams.CloudCredentialResult{{
		Result: &jujuparams.CloudCredential{
			AuthType: "userpass",
			Attributes: map[string]string{
				"username": "cloud-user",
			},
			Redacted: []string{
				"password",
			},
		},
	}, {
		Result: &jujuparams.CloudCredential{
			AuthType: "empty",
		},
	}, {
		Error: &jujuparams.Error{
			Message: `cloudcredential "` + jimmtest.TestCloudName + `/test@canonical.com/cred3" not found`,
			Code:    jujuparams.CodeNotFound,
		},
	}, {
		Error: &jujuparams.Error{
			Message: `unauthorized`,
			Code:    jujuparams.CodeUnauthorized,
		},
	}, {
		Error: &jujuparams.Error{
			Message: `unauthorized`,
			Code:    jujuparams.CodeUnauthorized,
		},
	}})
}

func (s *cloudSuite) TestRevokeCredential(c *gc.C) {
	conn := s.open(c, nil, "test")
	defer conn.Close()
	client := cloudapi.NewClient(conn)
	credTag := names.NewCloudCredentialTag(jimmtest.TestCloudName + "/test@canonical.com/cred")
	_, err := client.UpdateCredentialsCheckModels(
		credTag,
		cloud.NewCredential("empty", nil),
	)
	c.Assert(err, gc.Equals, nil)

	tags, err := client.UserCredentials(credTag.Owner(), credTag.Cloud())
	c.Assert(err, gc.Equals, nil)
	c.Assert(tags, jc.DeepEquals, []names.CloudCredentialTag{
		credTag,
	})

	ccr, err := client.Credentials(credTag)
	c.Assert(err, gc.Equals, nil)
	c.Assert(ccr, jc.DeepEquals, []jujuparams.CloudCredentialResult{{
		Result: &jujuparams.CloudCredential{
			AuthType: "empty",
		},
	}})

	err = client.RevokeCredential(credTag, false)
	c.Assert(err, gc.Equals, nil)

	ccr, err = client.Credentials(credTag)
	c.Assert(err, gc.Equals, nil)
	c.Assert(ccr, jc.DeepEquals, []jujuparams.CloudCredentialResult{{
		Error: &jujuparams.Error{
			Code:    jujuparams.CodeNotFound,
			Message: `cloudcredential "` + jimmtest.TestCloudName + `/test@canonical.com/cred" not found`,
		},
	}})

	tags, err = client.UserCredentials(credTag.Owner(), credTag.Cloud())
	c.Assert(err, gc.Equals, nil)
	c.Assert(tags, jc.DeepEquals, []names.CloudCredentialTag{})
}

func (s *cloudSuite) TestAddCloud(c *gc.C) {
	conn := s.open(c, nil, "test")
	defer conn.Close()
	client := cloudapi.NewClient(conn)
	err := client.AddCloud(cloud.Cloud{
		Name:             "test-cloud",
		Type:             "kubernetes",
		AuthTypes:        cloud.AuthTypes{cloud.CertificateAuthType},
		Endpoint:         "https://0.1.2.3:5678",
		IdentityEndpoint: "https://0.1.2.3:5679",
		StorageEndpoint:  "https://0.1.2.3:5680",
		HostCloudRegion:  jimmtest.TestCloudName + "/" + jimmtest.TestCloudRegionName,
	}, false)
	c.Assert(err, gc.Equals, nil)
	clouds, err := client.Clouds()
	c.Assert(err, gc.Equals, nil)
	c.Assert(clouds[names.NewCloudTag("test-cloud")], jc.DeepEquals, cloud.Cloud{
		Name:             "test-cloud",
		Type:             "kubernetes",
		AuthTypes:        cloud.AuthTypes{"certificate"},
		Endpoint:         "https://0.1.2.3:5678",
		IdentityEndpoint: "https://0.1.2.3:5679",
		StorageEndpoint:  "https://0.1.2.3:5680",
		Regions: []cloud.Region{{
			Name: "default",
		}},
	})
}

func (s *cloudSuite) TestRevokeCredentialsCheckModels(c *gc.C) {
	conn := s.open(c, nil, "test")
	defer conn.Close()
	client := cloudapi.NewClient(conn)
	credTag := names.NewCloudCredentialTag(jimmtest.TestCloudName + "/test@canonical.com/cred")
	_, err := client.UpdateCredentialsCheckModels(
		credTag,
		cloud.NewCredential("empty", nil),
	)
	c.Assert(err, gc.Equals, nil)

	tags, err := client.UserCredentials(credTag.Owner(), credTag.Cloud())
	c.Assert(err, gc.Equals, nil)
	c.Assert(tags, jc.DeepEquals, []names.CloudCredentialTag{
		credTag,
	})

	ccr, err := client.Credentials(credTag)
	c.Assert(err, gc.Equals, nil)
	c.Assert(ccr, jc.DeepEquals, []jujuparams.CloudCredentialResult{{
		Result: &jujuparams.CloudCredential{
			AuthType: "empty",
		},
	}})

	mmclient := modelmanager.NewClient(conn)
	_, err = mmclient.CreateModel("test", "test@canonical.com", jimmtest.TestCloudName, jimmtest.TestCloudRegionName, credTag, nil)
	c.Assert(err, gc.Equals, nil)

	var resp jujuparams.ErrorResults
	err = conn.APICall("Cloud", 7, "", "RevokeCredentialsCheckModels", jujuparams.RevokeCredentialArgs{
		Credentials: []jujuparams.RevokeCredentialArg{{
			Tag:   credTag.String(),
			Force: false,
		}},
	}, &resp)
	c.Assert(err, gc.Equals, nil)
	c.Assert(resp.Results[0].Error, gc.ErrorMatches, `cloud credential still used by 1 model\(s\)`)

	resp.Results = nil
	err = conn.APICall("Cloud", 7, "", "RevokeCredentialsCheckModels", jujuparams.RevokeCredentialArgs{
		Credentials: []jujuparams.RevokeCredentialArg{{
			Tag:   credTag.String(),
			Force: true,
		}},
	}, &resp)
	c.Assert(err, gc.Equals, nil)
	c.Assert(resp.Results[0].Error, gc.IsNil)

	ccr, err = client.Credentials(credTag)
	c.Assert(err, gc.Equals, nil)
	c.Assert(ccr, jc.DeepEquals, []jujuparams.CloudCredentialResult{{
		Error: &jujuparams.Error{
			Code:    jujuparams.CodeNotFound,
			Message: `cloudcredential "` + jimmtest.TestCloudName + `/test@canonical.com/cred" not found`,
		},
	}})

	tags, err = client.UserCredentials(credTag.Owner(), credTag.Cloud())
	c.Assert(err, gc.Equals, nil)
	c.Assert(tags, jc.DeepEquals, []names.CloudCredentialTag{})
}

func (s *cloudSuite) TestAddCloudError(c *gc.C) {
	conn := s.open(c, nil, "test")
	defer conn.Close()
	client := cloudapi.NewClient(conn)
	err := client.AddCloud(cloud.Cloud{
		Name:             "test-cloud",
		Type:             "kubernetes",
		Endpoint:         "https://0.1.2.3:5678",
		IdentityEndpoint: "https://0.1.2.3:5679",
		StorageEndpoint:  "https://0.1.2.3:5680",
		HostCloudRegion:  jimmtest.TestCloudName + "/" + jimmtest.TestCloudRegionName,
	}, false)
	c.Assert(err, gc.ErrorMatches, `invalid cloud: empty auth-types not valid.*`)
}

func (s *cloudSuite) TestAddCloudNoHostCloudRegion(c *gc.C) {
	conn := s.open(c, nil, "test")
	defer conn.Close()
	client := cloudapi.NewClient(conn)
	err := client.AddCloud(cloud.Cloud{
		Name:             "test-cloud",
		Type:             "kubernetes",
		Endpoint:         "https://0.1.2.3:5678",
		IdentityEndpoint: "https://0.1.2.3:5679",
		StorageEndpoint:  "https://0.1.2.3:5680",
	}, false)
	c.Assert(err, gc.ErrorMatches, `cloud host region not specified \(cloud region required\)`)
	c.Assert(jujuparams.ErrCode(err), gc.Equals, jujuparams.CodeCloudRegionRequired)
}

func (s *cloudSuite) TestAddCloudBadName(c *gc.C) {
	conn := s.open(c, nil, "test")
	defer conn.Close()
	client := cloudapi.NewClient(conn)
	err := client.AddCloud(cloud.Cloud{
		Name:             "aws",
		Type:             "kubernetes",
		HostCloudRegion:  jimmtest.TestCloudName + "/" + jimmtest.TestCloudRegionName,
		Endpoint:         "https://0.1.2.3:5678",
		IdentityEndpoint: "https://0.1.2.3:5679",
		StorageEndpoint:  "https://0.1.2.3:5680",
	}, false)
	c.Assert(err, gc.ErrorMatches, `cloud "aws" already exists \(already exists\)`)
}

func (s *cloudSuite) TestAddCredential(c *gc.C) {
	conn := s.open(c, nil, "test")
	defer conn.Close()
	client := cloudapi.NewClient(conn)
	credentialTag := names.NewCloudCredentialTag(jimmtest.TestCloudName + "/test@canonical.com/cred3")
	err := client.AddCredential(
		credentialTag.String(),
		cloud.NewCredential(
			"userpass",
			map[string]string{
				"username": "test-user",
				"password": "S3cret",
			},
		),
	)
	c.Assert(err, gc.Equals, nil)
	creds, err := client.CredentialContents(jimmtest.TestCloudName, "cred3", true)
	c.Assert(err, gc.Equals, nil)
	c.Assert(creds, jc.DeepEquals, []jujuparams.CredentialContentResult{{
		Result: &jujuparams.ControllerCredentialInfo{
			Content: jujuparams.CredentialContent{
				Name:     "cred3",
				Cloud:    jimmtest.TestCloudName,
				AuthType: "userpass",
				Attributes: map[string]string{
					"username": "test-user",
					"password": "S3cret",
				},
			},
		},
	}})
	err = client.AddCredential(
		credentialTag.String(),
		cloud.NewCredential(
			"userpass",
			map[string]string{
				"username": "test-user2",
				"password": "S3cret2",
			},
		),
	)
	c.Assert(err, gc.Equals, nil)
	creds, err = client.CredentialContents(jimmtest.TestCloudName, "cred3", true)
	c.Assert(err, gc.Equals, nil)
	c.Assert(creds, jc.DeepEquals, []jujuparams.CredentialContentResult{{
		Result: &jujuparams.ControllerCredentialInfo{
			Content: jujuparams.CredentialContent{
				Name:     "cred3",
				Cloud:    jimmtest.TestCloudName,
				AuthType: "userpass",
				Attributes: map[string]string{
					"username": "test-user2",
					"password": "S3cret2",
				},
			},
		},
	}})
}

func (s *cloudSuite) TestCredentialContents(c *gc.C) {
	conn := s.open(c, nil, "test")
	defer conn.Close()
	client := cloudapi.NewClient(conn)
	credentialTag := names.NewCloudCredentialTag(jimmtest.TestCloudName + "/test@canonical.com/cred3")
	err := client.AddCredential(
		credentialTag.String(),
		cloud.NewCredential(
			"userpass",
			map[string]string{
				"username": "test-user",
				"password": "S3cret",
			},
		),
	)
	c.Assert(err, gc.Equals, nil)
	creds, err := client.CredentialContents(jimmtest.TestCloudName, "cred3", false)
	c.Assert(err, gc.Equals, nil)
	c.Assert(creds, jc.DeepEquals, []jujuparams.CredentialContentResult{{
		Result: &jujuparams.ControllerCredentialInfo{
			Content: jujuparams.CredentialContent{
				Name:     "cred3",
				Cloud:    jimmtest.TestCloudName,
				AuthType: "userpass",
				Attributes: map[string]string{
					"username": "test-user",
				},
			},
		},
	}})

	mmclient := modelmanager.NewClient(conn)
	_, err = mmclient.CreateModel("model1", "test@canonical.com", jimmtest.TestCloudName, "", credentialTag, nil)
	c.Assert(err, gc.Equals, nil)

	creds, err = client.CredentialContents(jimmtest.TestCloudName, "cred3", true)
	c.Assert(err, gc.Equals, nil)
	c.Assert(creds, jc.DeepEquals, []jujuparams.CredentialContentResult{{
		Result: &jujuparams.ControllerCredentialInfo{
			Content: jujuparams.CredentialContent{
				Name:     "cred3",
				Cloud:    jimmtest.TestCloudName,
				AuthType: "userpass",
				Attributes: map[string]string{
					"username": "test-user",
					"password": "S3cret",
				},
			},
			Models: []jujuparams.ModelAccess{{
				Model:  "model1",
				Access: "admin",
			}},
		},
	}})

	// unspecified credentials return all.
	creds, err = client.CredentialContents("", "", true)
	c.Assert(err, gc.Equals, nil)
	c.Assert(creds, jc.DeepEquals, []jujuparams.CredentialContentResult{{
		Result: &jujuparams.ControllerCredentialInfo{
			Content: jujuparams.CredentialContent{
				Name:     "cred3",
				Cloud:    jimmtest.TestCloudName,
				AuthType: "userpass",
				Attributes: map[string]string{
					"username": "test-user",
					"password": "S3cret",
				},
			},
			Models: []jujuparams.ModelAccess{{
				Model:  "model1",
				Access: "admin",
			}},
		},
	}})
}

func (s *cloudSuite) TestCredentialContentsWithEmptyAttributes(c *gc.C) {
	conn := s.open(c, nil, "test")
	defer conn.Close()
	client := cloudapi.NewClient(conn)
	credentialTag := names.NewCloudCredentialTag(jimmtest.TestCloudName + "/test@canonical.com/cred3")
	err := client.AddCredential(
		credentialTag.String(),
		cloud.NewCredential(
			"certificate",
			nil,
		),
	)
	c.Assert(err, gc.Equals, nil)
	creds, err := client.CredentialContents(jimmtest.TestCloudName, "cred3", false)
	c.Assert(err, gc.Equals, nil)
	c.Assert(creds, jc.DeepEquals, []jujuparams.CredentialContentResult{{
		Result: &jujuparams.ControllerCredentialInfo{
			Content: jujuparams.CredentialContent{
				Name:       "cred3",
				Cloud:      jimmtest.TestCloudName,
				AuthType:   "certificate",
				Attributes: nil,
			},
		},
	}})
}

func (s *cloudSuite) TestRemoveCloud(c *gc.C) {
	conn := s.open(c, nil, "test")
	defer conn.Close()
	client := cloudapi.NewClient(conn)
	err := client.AddCloud(cloud.Cloud{
		Name:             "test-cloud",
		Type:             "kubernetes",
		AuthTypes:        cloud.AuthTypes{cloud.CertificateAuthType},
		Endpoint:         "https://0.1.2.3:5678",
		IdentityEndpoint: "https://0.1.2.3:5679",
		StorageEndpoint:  "https://0.1.2.3:5680",
		HostCloudRegion:  jimmtest.TestCloudName + "/" + jimmtest.TestCloudRegionName,
	}, false)
	c.Assert(err, gc.Equals, nil)
	clouds, err := client.Clouds()
	c.Assert(err, gc.Equals, nil)
	c.Assert(clouds[names.NewCloudTag("test-cloud")], jc.DeepEquals, cloud.Cloud{
		Name:             "test-cloud",
		Type:             "kubernetes",
		AuthTypes:        cloud.AuthTypes{"certificate"},
		Endpoint:         "https://0.1.2.3:5678",
		IdentityEndpoint: "https://0.1.2.3:5679",
		StorageEndpoint:  "https://0.1.2.3:5680",
		Regions: []cloud.Region{{
			Name: "default",
		}},
	})

	err = client.RemoveCloud("test-cloud")
	c.Assert(err, gc.Equals, nil)
	clouds, err = client.Clouds()
	c.Assert(err, gc.Equals, nil)
	c.Assert(clouds[names.NewCloudTag("test-cloud")], jc.DeepEquals, cloud.Cloud{})
}

func (s *cloudSuite) TestRemoveCloudNotFound(c *gc.C) {
	conn := s.open(c, nil, "test")
	defer conn.Close()
	client := cloudapi.NewClient(conn)

	err := client.RemoveCloud("test-cloud")
	c.Assert(err, gc.ErrorMatches, `cloud "test-cloud" not found`)
}

func (s *cloudSuite) TestModifyCloudAccess(c *gc.C) {
	conn := s.open(c, nil, "test")
	defer conn.Close()
	client := cloudapi.NewClient(conn)
	err := client.AddCloud(cloud.Cloud{
		Name:             "test-cloud",
		Type:             "kubernetes",
		AuthTypes:        cloud.AuthTypes{cloud.CertificateAuthType},
		Endpoint:         "https://0.1.2.3:5678",
		IdentityEndpoint: "https://0.1.2.3:5679",
		StorageEndpoint:  "https://0.1.2.3:5680",
		HostCloudRegion:  jimmtest.TestCloudName + "/" + jimmtest.TestCloudRegionName,
	}, false)
	c.Assert(err, gc.Equals, nil)
	clouds, err := client.Clouds()
	c.Assert(err, gc.Equals, nil)
	_, ok := clouds[names.NewCloudTag("test-cloud")]
	c.Assert(ok, jc.IsTrue)

	// Check that bob@canonical.com does not yet have access
	conn2 := s.open(c, nil, "bob")
	defer conn2.Close()
	client2 := cloudapi.NewClient(conn2)
	clouds, err = client2.Clouds()
	c.Assert(err, gc.Equals, nil)
	_, ok = clouds[names.NewCloudTag("test-cloud")]
	c.Assert(ok, gc.Equals, false, gc.Commentf("clouds: %#v", clouds))

	err = client.GrantCloud("bob@canonical.com", "add-model", "test-cloud")
	c.Assert(err, gc.Equals, nil)

	clouds, err = client2.Clouds()
	c.Assert(err, gc.Equals, nil)
	_, ok = clouds[names.NewCloudTag("test-cloud")]
	c.Assert(ok, jc.IsTrue)

	err = client.RevokeCloud("bob@canonical.com", "add-model", "test-cloud")
	c.Assert(err, gc.Equals, nil)
	clouds, err = client2.Clouds()
	c.Assert(err, gc.Equals, nil)
	_, ok = clouds[names.NewCloudTag("test-cloud")]
	c.Assert(ok, gc.Equals, false, gc.Commentf("clouds: %#v", clouds))
}

func (s *cloudSuite) TestModifyCloudAccessUnauthorized(c *gc.C) {
	conn := s.open(c, nil, "test")
	defer conn.Close()
	client := cloudapi.NewClient(conn)
	err := client.AddCloud(cloud.Cloud{
		Name:             "test-cloud",
		Type:             "kubernetes",
		AuthTypes:        cloud.AuthTypes{cloud.CertificateAuthType},
		Endpoint:         "https://0.1.2.3:5678",
		IdentityEndpoint: "https://0.1.2.3:5679",
		StorageEndpoint:  "https://0.1.2.3:5680",
		HostCloudRegion:  jimmtest.TestCloudName + "/" + jimmtest.TestCloudRegionName,
	}, false)
	c.Assert(err, gc.Equals, nil)
	clouds, err := client.Clouds()
	c.Assert(err, gc.Equals, nil)
	_, ok := clouds[names.NewCloudTag("test-cloud")]
	c.Assert(ok, jc.IsTrue)

	// Try granting cloud access as an unauthorized user.
	conn2 := s.open(c, nil, "charlie")
	defer conn2.Close()
	client2 := cloudapi.NewClient(conn2)
	err = client2.GrantCloud("charlie@canonical.com", "add-model", "test-cloud")
	c.Assert(err, gc.ErrorMatches, `unauthorized`)
}

func (s *cloudSuite) TestUpdateCloud(c *gc.C) {
	conn := s.open(c, nil, "test")
	defer conn.Close()
	client := cloudapi.NewClient(conn)
	err := client.UpdateCloud(cloud.Cloud{
		Name:             "test-cloud",
		Type:             "kubernetes",
		AuthTypes:        cloud.AuthTypes{cloud.CertificateAuthType},
		Endpoint:         "https://0.1.2.3:5678",
		IdentityEndpoint: "https://0.1.2.3:5679",
		StorageEndpoint:  "https://0.1.2.3:5680",
	})
	c.Assert(jujuparams.IsCodeForbidden(err), gc.Equals, true, gc.Commentf("%#v", err))
}

func (s *cloudSuite) TestCloudInfo(c *gc.C) {
	conn := s.open(c, nil, "alice")
	defer conn.Close()
	client := cloudapi.NewClient(conn)
	err := client.AddCloud(cloud.Cloud{
		Name:             "test-cloud",
		Type:             "kubernetes",
		AuthTypes:        cloud.AuthTypes{cloud.CertificateAuthType},
		Endpoint:         "https://0.1.2.3:5678",
		IdentityEndpoint: "https://0.1.2.3:5679",
		StorageEndpoint:  "https://0.1.2.3:5680",
		HostCloudRegion:  jimmtest.TestCloudName + "/" + jimmtest.TestCloudRegionName,
	}, false)
	c.Assert(err, gc.Equals, nil)

	conn = s.open(c, nil, "bob")
	defer conn.Close()

	args := jujuparams.Entities{
		Entities: []jujuparams.Entity{{
			Tag: names.NewCloudTag(jimmtest.TestCloudName).String(),
		}, {
			Tag: names.NewCloudTag("no-such-cloud").String(),
		}, {
			Tag: names.NewUserTag("not-a-cloud").String(),
		}, {
			Tag: names.NewCloudTag("test-cloud").String(),
		}},
	}
	var result jujuparams.CloudInfoResults
	err = conn.APICall("Cloud", 7, "", "CloudInfo", args, &result)
	c.Assert(err, gc.Equals, nil)
	c.Assert(result, jc.DeepEquals, jujuparams.CloudInfoResults{
		Results: []jujuparams.CloudInfoResult{{
			Result: &jujuparams.CloudInfo{
				CloudDetails: jujuparams.CloudDetails{
					Type:      jimmtest.TestProviderType,
					AuthTypes: []string{"empty", "userpass"},
					Regions: []jujuparams.CloudRegion{{
						Name:             jimmtest.TestCloudRegionName,
						Endpoint:         jimmtest.TestCloudEndpoint,
						IdentityEndpoint: jimmtest.TestCloudIdentityEndpoint,
						StorageEndpoint:  jimmtest.TestCloudStorageEndpoint,
					}},
					Endpoint:         jimmtest.TestCloudEndpoint,
					IdentityEndpoint: jimmtest.TestCloudIdentityEndpoint,
					StorageEndpoint:  jimmtest.TestCloudStorageEndpoint,
				},
			},
		}, {
			Error: &jujuparams.Error{
				Code:    "not found",
				Message: `cloud "no-such-cloud" not found`,
			},
		}, {
			Error: &jujuparams.Error{
				Code:    "bad request",
				Message: `"user-not-a-cloud" is not a valid cloud tag`,
			},
		}, {
			Error: &jujuparams.Error{
				Code:    "unauthorized access",
				Message: "unauthorized",
			},
		}},
	})
}

func (s *cloudSuite) TestListCloudInfo(c *gc.C) {
	conn := s.open(c, nil, "alice")
	defer conn.Close()
	client := cloudapi.NewClient(conn)
	err := client.AddCloud(cloud.Cloud{
		Name:             "test-cloud",
		Type:             "kubernetes",
		AuthTypes:        cloud.AuthTypes{cloud.CertificateAuthType},
		Endpoint:         "https://0.1.2.3:5678",
		IdentityEndpoint: "https://0.1.2.3:5679",
		StorageEndpoint:  "https://0.1.2.3:5680",
		HostCloudRegion:  jimmtest.TestCloudName + "/" + jimmtest.TestCloudRegionName,
	}, false)
	c.Assert(err, gc.Equals, nil)

	bobIdentity, err := dbmodel.NewIdentity("bob@canonical.com")
	c.Assert(err, gc.IsNil)
	bob := openfga.NewUser(bobIdentity, s.OFGAClient)
	err = bob.SetCloudAccess(context.Background(), names.NewCloudTag("test-cloud"), ofganames.CanAddModelRelation)
	c.Assert(err, gc.Equals, nil)
	err = bob.SetCloudAccess(context.Background(), names.NewCloudTag(jimmtest.TestCloudName), ofganames.CanAddModelRelation)
	c.Assert(err, gc.Equals, nil)

	aliceIdentity, err := dbmodel.NewIdentity("alice@canonical.com")
	c.Assert(err, gc.IsNil)
	alice := openfga.NewUser(aliceIdentity, s.OFGAClient)
	err = alice.SetCloudAccess(context.Background(), names.NewCloudTag(jimmtest.TestCloudName), ofganames.CanAddModelRelation)
	c.Assert(err, gc.Equals, nil)

	args := jujuparams.ListCloudsRequest{
		UserTag: names.NewUserTag("alice@canonical.com").String(),
		All:     false,
	}
	var result jujuparams.ListCloudInfoResults
	err = conn.APICall("Cloud", 7, "", "ListCloudInfo", args, &result)
	c.Assert(err, gc.Equals, nil)
	sort.Slice(result.Results, func(i, j int) bool {
		return result.Results[i].Result.Type > result.Results[j].Result.Type
	})
	c.Check(result, jc.DeepEquals, jujuparams.ListCloudInfoResults{
		Results: []jujuparams.ListCloudInfoResult{{
			Result: &jujuparams.ListCloudInfo{
				CloudDetails: jujuparams.CloudDetails{
					Type:             "kubernetes",
					AuthTypes:        []string{"certificate"},
					Endpoint:         "https://0.1.2.3:5678",
					IdentityEndpoint: "https://0.1.2.3:5679",
					StorageEndpoint:  "https://0.1.2.3:5680",
					Regions: []jujuparams.CloudRegion{{
						Name: "default",
					}},
				},
				Access: "admin",
			},
		}, {
			Result: &jujuparams.ListCloudInfo{
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
				Access: "admin",
			},
		}},
	})

	conn = s.open(c, nil, "bob")
	defer conn.Close()

	args = jujuparams.ListCloudsRequest{
		UserTag: names.NewUserTag("bob@canonical.com").String(),
		All:     false,
	}
	result.Results = nil
	err = conn.APICall("Cloud", 7, "", "ListCloudInfo", args, &result)
	c.Assert(err, gc.Equals, nil)

	sort.Slice(result.Results, func(i, j int) bool {
		return result.Results[i].Result.Type > result.Results[j].Result.Type
	})
	c.Check(result, jc.DeepEquals, jujuparams.ListCloudInfoResults{
		Results: []jujuparams.ListCloudInfoResult{{
			Result: &jujuparams.ListCloudInfo{
				CloudDetails: jujuparams.CloudDetails{
					Type:             "kubernetes",
					AuthTypes:        []string{"certificate"},
					Endpoint:         "https://0.1.2.3:5678",
					IdentityEndpoint: "https://0.1.2.3:5679",
					StorageEndpoint:  "https://0.1.2.3:5680",
					Regions: []jujuparams.CloudRegion{{
						Name: "default",
					}},
				},
				Access: "add-model",
			},
		}, {
			Result: &jujuparams.ListCloudInfo{
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
				Access: "add-model",
			},
		}},
	})
}
