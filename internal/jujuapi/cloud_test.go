// Copyright 2016 Canonical Ltd.

package jujuapi_test

import (
	"context"
	"fmt"
	"sort"

	"github.com/juju/juju/api/base"
	cloudapi "github.com/juju/juju/api/cloud"
	"github.com/juju/juju/api/modelmanager"
	jujuparams "github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cloud"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"
	errgo "gopkg.in/errgo.v1"

	"github.com/CanonicalLtd/jimm/internal/mongodoc"
	"github.com/CanonicalLtd/jimm/params"
)

type cloudSuite struct {
	websocketSuite
}

var _ = gc.Suite(&cloudSuite{})

func (s *cloudSuite) SetUpTest(c *gc.C) {
	s.ServerParams.CharmstoreLocation = "https://api.jujucharms.com/charmstore"
	s.ServerParams.MeteringLocation = "https://api.jujucharms.com/omnibus"
	s.websocketSuite.SetUpTest(c)
	s.PatchValue(&utils.OutgoingAccessAllowed, true)
}

var defaultCloudTests = []struct {
	about      string
	cloudNames []string
	expect     string
}{{
	about: "no controllers",
}, {
	about:      "one controller",
	cloudNames: []string{"cloudname"},
	expect:     "cloudname",
}, {
	about:      "two controllers, same cloud",
	cloudNames: []string{"cloudname", "cloudname"},
	expect:     "cloudname",
}, {
	about:      "two controllers, different cloud",
	cloudNames: []string{"cloud1", "cloud2"},
}, {
	about:      "three controllers, some same",
	cloudNames: []string{"cloud1", "cloud1", "cloud2"},
}}

func (s *cloudSuite) TestDefaultCloud(c *gc.C) {
	ctx := context.Background()

	conn := s.open(c, nil, "test")
	defer conn.Close()
	for i, test := range defaultCloudTests {
		c.Logf("test %d: %s", i, test.about)
		_, err := s.JEM.DB.Controllers().RemoveAll(nil)
		c.Assert(err, gc.Equals, nil)
		_, err = s.JEM.DB.CloudRegions().RemoveAll(nil)
		c.Assert(err, gc.Equals, nil)
		for j, cloud := range test.cloudNames {
			ctlPath := params.EntityPath{User: "test", Name: params.Name(fmt.Sprintf("controller-%d", j))}
			err := s.JEM.DB.InsertController(ctx, &mongodoc.Controller{
				Path:   ctlPath,
				ACL:    params.ACL{Read: []string{"everyone"}},
				CACert: "cacert",
				UUID:   fmt.Sprintf("uuid%d", j),
				Public: true,
			})
			c.Assert(err, gc.Equals, nil)
			err = s.JEM.DB.UpdateCloudRegions(ctx, []mongodoc.CloudRegion{{
				Cloud:              params.Cloud(cloud),
				PrimaryControllers: []params.EntityPath{ctlPath},
				ACL: params.ACL{
					Read: []string{"everyone"},
				},
			}})
			c.Assert(err, gc.Equals, nil)
		}
		cloud, err := defaultCloud(conn)
		if test.expect == "" {
			c.Check(err, gc.ErrorMatches, `no default cloud \(not found\)`)
			c.Assert(jujuparams.IsCodeNotFound(err), gc.Equals, true)
			c.Check(cloud, gc.Equals, names.CloudTag{})
			continue
		}
		c.Assert(err, gc.Equals, nil)
		c.Assert(cloud, gc.Equals, names.NewCloudTag(test.expect))
	}
}

// defaultCloud implements the old DefaultCloud method that was removed
// from cloud.Client.
func defaultCloud(conn base.APICaller) (names.CloudTag, error) {
	var result jujuparams.StringResult
	if err := conn.APICall("Cloud", 3, "", "DefaultCloud", nil, &result); err != nil {
		return names.CloudTag{}, errgo.Mask(err, errgo.Any)
	}
	if result.Error != nil {
		return names.CloudTag{}, result.Error
	}
	cloudTag, err := names.ParseCloudTag(result.Result)
	if err != nil {
		return names.CloudTag{}, errgo.Mask(err)
	}
	return cloudTag, nil
}

func (s *cloudSuite) TestCloudCall(c *gc.C) {
	ctx := context.Background()

	s.AssertAddController(ctx, c, params.EntityPath{User: "test", Name: "controller-1"}, true)
	conn := s.open(c, nil, "test")
	defer conn.Close()
	client := cloudapi.NewClient(conn)
	info, err := client.Cloud(names.NewCloudTag("dummy"))
	c.Assert(err, gc.Equals, nil)
	c.Assert(info, jc.DeepEquals, cloud.Cloud{
		Name:      "dummy",
		Type:      "dummy",
		AuthTypes: cloud.AuthTypes{"empty", "userpass"},
		Regions: []cloud.Region{{
			Name:             "dummy-region",
			Endpoint:         "dummy-endpoint",
			IdentityEndpoint: "dummy-identity-endpoint",
			StorageEndpoint:  "dummy-storage-endpoint",
		}},
		Endpoint:         "dummy-endpoint",
		IdentityEndpoint: "dummy-identity-endpoint",
		StorageEndpoint:  "dummy-storage-endpoint",
	})
}

func (s *cloudSuite) TestClouds(c *gc.C) {
	ctx := context.Background()

	s.AssertAddController(ctx, c, params.EntityPath{User: "test-group", Name: "controller-1"}, true)
	s.IDMSrv.AddUser("test", "test-group")
	conn := s.open(c, nil, "test")
	defer conn.Close()

	client := cloudapi.NewClient(conn)
	clouds, err := client.Clouds()
	c.Assert(err, gc.Equals, nil)
	c.Assert(clouds, jc.DeepEquals, map[names.CloudTag]cloud.Cloud{
		names.NewCloudTag("dummy"): {
			Name:      "dummy",
			Type:      "dummy",
			AuthTypes: cloud.AuthTypes{"empty", "userpass"},
			Regions: []cloud.Region{{
				Name:             "dummy-region",
				Endpoint:         "dummy-endpoint",
				IdentityEndpoint: "dummy-identity-endpoint",
				StorageEndpoint:  "dummy-storage-endpoint",
			}},
			Endpoint:         "dummy-endpoint",
			IdentityEndpoint: "dummy-identity-endpoint",
			StorageEndpoint:  "dummy-storage-endpoint",
		},
	})
}

func (s *cloudSuite) TestUserCredentials(c *gc.C) {
	ctx := context.Background()

	s.AssertAddController(ctx, c, params.EntityPath{User: "test", Name: "controller-1"}, true)
	_, err := s.JEM.UpdateCredential(ctx, &mongodoc.Credential{
		Path: mongodoc.CredentialPath{
			Cloud: "dummy",
			EntityPath: mongodoc.EntityPath{
				User: "test",
				Name: "cred1",
			},
		},
		Type:  "credtype",
		Label: "Credentials 1",
		Attributes: map[string]string{
			"attr1": "val1",
			"attr2": "val2",
		},
	}, 0)
	c.Assert(err, gc.Equals, nil)
	conn := s.open(c, nil, "test")
	defer conn.Close()
	client := cloudapi.NewClient(conn)
	creds, err := client.UserCredentials(names.NewUserTag("test@external"), names.NewCloudTag("dummy"))
	c.Assert(err, gc.Equals, nil)
	c.Assert(creds, jc.DeepEquals, []names.CloudCredentialTag{
		names.NewCloudCredentialTag("dummy/test@external/cred1"),
	})
}

func (s *cloudSuite) TestUserCredentialsWithDomain(c *gc.C) {
	ctx := context.Background()

	s.AssertAddController(ctx, c, params.EntityPath{User: "test", Name: "controller-1"}, true)
	_, err := s.JEM.UpdateCredential(ctx, &mongodoc.Credential{
		Path: mongodoc.CredentialPath{
			Cloud: "dummy",
			EntityPath: mongodoc.EntityPath{
				User: "test@domain",
				Name: "cred1",
			},
		},
		Type:  "credtype",
		Label: "Credentials 1",
		Attributes: map[string]string{
			"attr1": "val1",
			"attr2": "val2",
		},
	}, 0)
	c.Assert(err, gc.Equals, nil)
	conn := s.open(c, nil, "test@domain")
	defer conn.Close()
	client := cloudapi.NewClient(conn)
	creds, err := client.UserCredentials(names.NewUserTag("test@domain"), names.NewCloudTag("dummy"))
	c.Assert(err, gc.Equals, nil)
	c.Assert(creds, jc.DeepEquals, []names.CloudCredentialTag{
		names.NewCloudCredentialTag("dummy/test@domain/cred1"),
	})
}

func (s *cloudSuite) TestUserCredentialsACL(c *gc.C) {
	ctx := context.Background()

	s.AssertAddController(ctx, c, params.EntityPath{User: "test", Name: "controller-1"}, true)
	_, err := s.JEM.UpdateCredential(ctx, &mongodoc.Credential{
		Path: mongodoc.CredentialPath{
			Cloud: "dummy",
			EntityPath: mongodoc.EntityPath{
				User: "test",
				Name: "cred1",
			},
		},
		Type:  "credtype",
		Label: "Credentials 1",
		Attributes: map[string]string{
			"attr1": "val1",
			"attr2": "val2",
		},
	}, 0)
	c.Assert(err, gc.Equals, nil)
	_, err = s.JEM.UpdateCredential(ctx, &mongodoc.Credential{
		Path: mongodoc.CredentialPath{
			Cloud: "dummy",
			EntityPath: mongodoc.EntityPath{
				User: "test2",
				Name: "cred2",
			},
		},
		ACL: params.ACL{
			Read: []string{"test"},
		},
		Type:  "credtype",
		Label: "Credentials 2",
		Attributes: map[string]string{
			"attr1": "val3",
			"attr2": "val4",
		},
	}, 0)
	c.Assert(err, gc.Equals, nil)
	conn := s.open(c, nil, "test")
	defer conn.Close()
	client := cloudapi.NewClient(conn)
	creds, err := client.UserCredentials(names.NewUserTag("test2@external"), names.NewCloudTag("dummy"))
	c.Assert(err, gc.Equals, nil)
	c.Assert(creds, jc.DeepEquals, []names.CloudCredentialTag{
		names.NewCloudCredentialTag("dummy/test2@external/cred2"),
	})
}

func (s *cloudSuite) TestUserCredentialsErrors(c *gc.C) {
	ctx := context.Background()

	s.AssertAddController(ctx, c, params.EntityPath{User: "test", Name: "controller-1"}, true)
	conn := s.open(c, nil, "test")
	defer conn.Close()
	req := jujuparams.UserClouds{
		UserClouds: []jujuparams.UserCloud{{
			UserTag:  "not-a-user-tag",
			CloudTag: "dummy",
		}},
	}
	var resp jujuparams.StringsResults
	err := conn.APICall("Cloud", 1, "", "UserCredentials", req, &resp)
	c.Assert(err, gc.Equals, nil)
	c.Assert(resp.Results[0].Error, gc.ErrorMatches, `"not-a-user-tag" is not a valid tag`)
	c.Assert(resp.Results, gc.HasLen, 1)
}

func (s *cloudSuite) TestUpdateCloudCredentials(c *gc.C) {
	ctx := context.Background()

	s.AssertAddController(ctx, c, params.EntityPath{User: "test", Name: "controller-1"}, true)
	conn := s.open(c, nil, "test")
	defer conn.Close()
	client := cloudapi.NewClient(conn)
	credentialTag := names.NewCloudCredentialTag(fmt.Sprintf("dummy/test@external/cred3"))
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
	creds, err := client.UserCredentials(names.NewUserTag("test@external"), names.NewCloudTag("dummy"))
	c.Assert(err, gc.Equals, nil)
	c.Assert(creds, jc.DeepEquals, []names.CloudCredentialTag{credentialTag})
	_, err = client.UpdateCredentialsCheckModels(credentialTag, cloud.NewCredential("credtype", map[string]string{"attr1": "val33", "attr2": "val34"}))
	c.Assert(err, gc.Equals, nil)
	creds, err = client.UserCredentials(names.NewUserTag("test@external"), names.NewCloudTag("dummy"))
	c.Assert(err, gc.Equals, nil)
	var _ = creds
}

func (s *cloudSuite) TestUpdateCloudCredentialsErrors(c *gc.C) {
	ctx := context.Background()

	s.AssertAddController(ctx, c, params.EntityPath{User: "test", Name: "controller-1"}, true)
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
			Tag: names.NewCloudCredentialTag("dummy/test2@external/cred1").String(),
			Credential: jujuparams.CloudCredential{
				AuthType: "credtype",
				Attributes: map[string]string{
					"attr1": "val1",
				},
			},
		}, {
			Tag: names.NewCloudCredentialTag("dummy/test@external/bad-name-").String(),
			Credential: jujuparams.CloudCredential{
				AuthType: "credtype",
				Attributes: map[string]string{
					"attr1": "val1",
				},
			},
		}},
	}
	var resp jujuparams.ErrorResults
	err := conn.APICall("Cloud", 1, "", "UpdateCredentials", req, &resp)
	c.Assert(err, gc.Equals, nil)
	c.Assert(resp.Results, gc.HasLen, 3)
	c.Assert(resp.Results[0].Error, gc.ErrorMatches, `"not-a-cloud-credentials-tag" is not a valid tag`)
	c.Assert(resp.Results[1].Error, gc.ErrorMatches, `unauthorized`)
	c.Assert(resp.Results[2].Error, gc.ErrorMatches, `invalid name "bad-name-"`)
}

func (s *cloudSuite) TestUpdateCloudCredentialsForce(c *gc.C) {
	ctx := context.Background()

	s.AssertAddController(ctx, c, params.EntityPath{User: "test", Name: "controller-1"}, true)
	conn := s.open(c, nil, "test")
	defer conn.Close()
	client := cloudapi.NewClient(conn)
	credentialTag := names.NewCloudCredentialTag(fmt.Sprintf("dummy/test@external/cred3"))
	_, err := client.UpdateCredentialsCheckModels(credentialTag, cloud.NewCredential("userpass", map[string]string{"username": "a", "password": "b"}))
	c.Assert(err, gc.Equals, nil)

	mmclient := modelmanager.NewClient(conn)
	_, err = mmclient.CreateModel("model1", "test@external", "dummy", "", credentialTag, nil)
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
	// First try without Force to sanity check that it should fail.
	var resp jujuparams.UpdateCredentialResults
	err = conn.APICall("Cloud", 3, "", "UpdateCredentialsCheckModels", args, &resp)
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
	err = conn.APICall("Cloud", 3, "", "UpdateCredentialsCheckModels", args, &resp)
	c.Assert(err, gc.Equals, nil)
	c.Check(resp.Results[0].Error, gc.ErrorMatches, `updating cloud credentials: validating credential "dummy/test@external/cred3" for cloud "dummy": supported auth-types \["empty" "userpass"\], "badauthtype" not supported`)

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
	ctx := context.Background()

	s.AssertAddController(ctx, c, params.EntityPath{User: "test", Name: "controller-1"}, true)

	conn := s.open(c, nil, "test")
	defer conn.Close()

	credTag := names.NewCloudCredentialTag("dummy/test@external/cred")
	cred1 := cloud.NewCredential("userpass", map[string]string{
		"username": "cloud-user",
		"password": "cloud-pass",
	})

	client := cloudapi.NewClient(conn)
	_, err := client.UpdateCredentialsCheckModels(credTag, cred1)
	c.Assert(err, gc.Equals, nil)

	mmclient := modelmanager.NewClient(conn)
	model1, err := mmclient.CreateModel("model1", "test@external", "dummy", "", credTag, nil)
	c.Assert(err, gc.Equals, nil)

	model2, err := mmclient.CreateModel("model2", "test@external", "dummy", "", credTag, nil)
	c.Assert(err, gc.Equals, nil)

	var resp jujuparams.UpdateCredentialResults
	err = conn.APICall("Cloud", 3, "", "CheckCredentialsModels", jujuparams.TaggedCredentials{
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
	ctx := context.Background()

	s.AssertAddController(ctx, c, params.EntityPath{User: "test", Name: "controller-1"}, true)

	conn := s.open(c, nil, "test")
	defer conn.Close()

	credTag := names.NewCloudCredentialTag("dummy/test@external/cred")
	cred1 := cloud.NewCredential("userpass", map[string]string{
		"username": "cloud-user",
		"password": "cloud-pass",
	})

	client := cloudapi.NewClient(conn)
	_, err := client.UpdateCredentialsCheckModels(credTag, cred1)
	c.Assert(err, gc.Equals, nil)

	mmclient := modelmanager.NewClient(conn)
	model1, err := mmclient.CreateModel("model1", "test@external", "dummy", "", credTag, nil)
	c.Assert(err, gc.Equals, nil)

	var resp jujuparams.UpdateCredentialResults
	err = conn.APICall("Cloud", 3, "", "CheckCredentialsModels", jujuparams.TaggedCredentials{
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
	c.Assert(resp, jc.DeepEquals, jujuparams.UpdateCredentialResults{
		Results: []jujuparams.UpdateCredentialResult{{
			CredentialTag: "cloudcred-dummy_test@external_cred",
			Error:         &jujuparams.Error{Message: "some models are no longer visible"},
			Models: []jujuparams.UpdateCredentialModelResult{{
				ModelUUID: model1.UUID,
				ModelName: "model1",
				Errors: []jujuparams.ErrorResult{{
					Error: &jujuparams.Error{
						Message: `validating credential "dummy/test@external/cred" for cloud "dummy": supported auth-types ["empty" "userpass"], "unknowntype" not supported`,
						Code:    "not supported",
					},
				}},
			}},
		}},
	})
}

func (s *cloudSuite) TestCredential(c *gc.C) {
	ctx := context.Background()

	s.AssertAddController(ctx, c, params.EntityPath{User: "test", Name: "controller-1"}, true)
	conn := s.open(c, nil, "test")
	defer conn.Close()

	cred1Tag := names.NewCloudCredentialTag("dummy/test@external/cred1")
	cred1 := cloud.NewCredential("userpass", map[string]string{
		"username": "cloud-user",
		"password": "cloud-pass",
	})
	cred2Tag := names.NewCloudCredentialTag("dummy/test@external/cred2")
	cred2 := cloud.NewCredential("empty", nil)

	cred5Tag := names.NewCloudCredentialTag("no-such-cloud/test@external/cred5")
	cred5 := cloud.NewCredential("userpass", map[string]string{
		"username": "cloud-user",
		"password": "cloud-pass",
	})

	client := cloudapi.NewClient(conn)
	_, err := client.UpdateCredentialsCheckModels(cred1Tag, cred1)
	c.Assert(err, gc.Equals, nil)
	_, err = client.UpdateCredentialsCheckModels(cred2Tag, cred2)
	c.Assert(err, gc.Equals, nil)
	_, err = client.UpdateCredentialsCheckModels(cred5Tag, cred5)
	c.Assert(err, gc.Equals, nil)

	creds, err := client.Credentials(
		cred1Tag,
		cred2Tag,
		names.NewCloudCredentialTag("dummy/test@external/cred3"),
		names.NewCloudCredentialTag("dummy/no-test@external/cred4"),
		cred5Tag,
		names.NewCloudCredentialTag("dummy/admin@local/cred6"),
	)
	c.Assert(err, gc.Equals, nil)
	for i := range creds {
		if creds[i].Result == nil {
			continue
		}
		sort.Strings(creds[i].Result.Redacted)
	}
	c.Assert(creds, jc.DeepEquals, []jujuparams.CloudCredentialResult{{
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
			Message: `credential not found`,
			Code:    jujuparams.CodeNotFound,
		},
	}, {
		Error: &jujuparams.Error{
			Message: `unauthorized`,
			Code:    jujuparams.CodeUnauthorized,
		},
	}, {
		Result: &jujuparams.CloudCredential{
			AuthType: "userpass",
			Redacted: []string{
				"password",
				"username",
			},
		},
	}, {
		Error: &jujuparams.Error{
			Message: `unsupported local user`,
			Code:    jujuparams.CodeUserNotFound,
		},
	}})
}

func (s *cloudSuite) TestRevokeCredential(c *gc.C) {
	ctx := context.Background()

	s.AssertAddController(ctx, c, params.EntityPath{User: "test", Name: "controller"}, true)
	conn := s.open(c, nil, "test")
	defer conn.Close()
	client := cloudapi.NewClient(conn)
	credTag := names.NewCloudCredentialTag("dummy/test@external/cred")
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
			Message: `credential "dummy/test@external/cred" not found`,
		},
	}})

	tags, err = client.UserCredentials(credTag.Owner(), credTag.Cloud())
	c.Assert(err, gc.Equals, nil)
	c.Assert(tags, jc.DeepEquals, []names.CloudCredentialTag{})
}

func (s *cloudSuite) TestAddCloud(c *gc.C) {
	ctx := context.Background()

	s.AssertAddController(ctx, c, params.EntityPath{User: "test", Name: "controller"}, true)
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
		HostCloudRegion:  "dummy/dummy-region",
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
	ctx := context.Background()

	s.AssertAddController(ctx, c, params.EntityPath{User: "test", Name: "controller"}, true)
	conn := s.open(c, nil, "test")
	defer conn.Close()
	client := cloudapi.NewClient(conn)
	credTag := names.NewCloudCredentialTag("dummy/test@external/cred")
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
	_, err = mmclient.CreateModel("test", "test@external", "dummy", "dummy-region", credTag, nil)
	c.Assert(err, gc.Equals, nil)

	var resp jujuparams.ErrorResults
	err = conn.APICall("Cloud", 3, "", "RevokeCredentialsCheckModels", jujuparams.RevokeCredentialArgs{
		Credentials: []jujuparams.RevokeCredentialArg{{
			Tag:   credTag.String(),
			Force: false,
		}},
	}, &resp)
	c.Assert(err, gc.Equals, nil)
	c.Assert(resp.Results[0].Error, gc.ErrorMatches, `cannot revoke because credential is in use on at least one model`)

	resp.Results = nil
	err = conn.APICall("Cloud", 3, "", "RevokeCredentialsCheckModels", jujuparams.RevokeCredentialArgs{
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
			Message: `credential "dummy/test@external/cred" not found`,
		},
	}})

	tags, err = client.UserCredentials(credTag.Owner(), credTag.Cloud())
	c.Assert(err, gc.Equals, nil)
	c.Assert(tags, jc.DeepEquals, []names.CloudCredentialTag{})
}

func (s *cloudSuite) TestAddCloudError(c *gc.C) {
	ctx := context.Background()

	s.AssertAddController(ctx, c, params.EntityPath{User: "test", Name: "controller"}, true)
	conn := s.open(c, nil, "test")
	defer conn.Close()
	client := cloudapi.NewClient(conn)
	err := client.AddCloud(cloud.Cloud{
		Name:             "test-cloud",
		Type:             "kubernetes",
		Endpoint:         "https://0.1.2.3:5678",
		IdentityEndpoint: "https://0.1.2.3:5679",
		StorageEndpoint:  "https://0.1.2.3:5680",
		HostCloudRegion:  "dummy/dummy-region",
	}, false)
	c.Assert(err, gc.ErrorMatches, `invalid cloud: empty auth-types not valid`)
}

func (s *cloudSuite) TestAddCloudNoHostCloudRegion(c *gc.C) {
	ctx := context.Background()

	s.AssertAddController(ctx, c, params.EntityPath{User: "test", Name: "controller"}, true)
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
	c.Assert(err, gc.ErrorMatches, `cloud region required \(cloud region required\)`)
	c.Assert(jujuparams.IsCodeCloudRegionRequired(err), gc.Equals, true)
}

func (s *cloudSuite) TestAddCloudBadName(c *gc.C) {
	ctx := context.Background()

	s.AssertAddController(ctx, c, params.EntityPath{User: "test", Name: "controller"}, true)
	conn := s.open(c, nil, "test")
	defer conn.Close()
	client := cloudapi.NewClient(conn)
	err := client.AddCloud(cloud.Cloud{
		Name:             "aws",
		Type:             "kubernetes",
		HostCloudRegion:  "dummy/dummy-region",
		Endpoint:         "https://0.1.2.3:5678",
		IdentityEndpoint: "https://0.1.2.3:5679",
		StorageEndpoint:  "https://0.1.2.3:5680",
	}, false)
	c.Assert(err, gc.ErrorMatches, `cloud "aws" already exists \(already exists\)`)
}

func (s *cloudSuite) TestAddCredential(c *gc.C) {
	ctx := context.Background()

	s.AssertAddController(ctx, c, params.EntityPath{User: "test", Name: "controller-1"}, true)
	conn := s.open(c, nil, "test")
	defer conn.Close()
	client := cloudapi.NewClient(conn)
	credentialTag := names.NewCloudCredentialTag("dummy/test@external/cred3")
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
	creds, err := client.CredentialContents("dummy", "cred3", true)
	c.Assert(err, gc.Equals, nil)
	c.Assert(creds, jc.DeepEquals, []jujuparams.CredentialContentResult{{
		Result: &jujuparams.ControllerCredentialInfo{
			Content: jujuparams.CredentialContent{
				Name:     "cred3",
				Cloud:    "dummy",
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
	creds, err = client.CredentialContents("dummy", "cred3", true)
	c.Assert(err, gc.Equals, nil)
	c.Assert(creds, jc.DeepEquals, []jujuparams.CredentialContentResult{{
		Result: &jujuparams.ControllerCredentialInfo{
			Content: jujuparams.CredentialContent{
				Name:     "cred3",
				Cloud:    "dummy",
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
	ctx := context.Background()

	s.AssertAddController(ctx, c, params.EntityPath{User: "test", Name: "controller-1"}, true)
	conn := s.open(c, nil, "test")
	defer conn.Close()
	client := cloudapi.NewClient(conn)
	credentialTag := names.NewCloudCredentialTag("dummy/test@external/cred3")
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
	creds, err := client.CredentialContents("dummy", "cred3", false)
	c.Assert(err, gc.Equals, nil)
	c.Assert(creds, jc.DeepEquals, []jujuparams.CredentialContentResult{{
		Result: &jujuparams.ControllerCredentialInfo{
			Content: jujuparams.CredentialContent{
				Name:     "cred3",
				Cloud:    "dummy",
				AuthType: "userpass",
				Attributes: map[string]string{
					"username": "test-user",
				},
			},
		},
	}})

	mmclient := modelmanager.NewClient(conn)
	_, err = mmclient.CreateModel("model1", "test@external", "dummy", "", credentialTag, nil)
	c.Assert(err, gc.Equals, nil)

	creds, err = client.CredentialContents("dummy", "cred3", true)
	c.Assert(err, gc.Equals, nil)
	c.Assert(creds, jc.DeepEquals, []jujuparams.CredentialContentResult{{
		Result: &jujuparams.ControllerCredentialInfo{
			Content: jujuparams.CredentialContent{
				Name:     "cred3",
				Cloud:    "dummy",
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
				Cloud:    "dummy",
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

func (s *cloudSuite) TestRemoveCloud(c *gc.C) {
	ctx := context.Background()

	s.AssertAddController(ctx, c, params.EntityPath{User: "test", Name: "controller"}, true)
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
		HostCloudRegion:  "dummy/dummy-region",
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
	ctx := context.Background()

	s.AssertAddController(ctx, c, params.EntityPath{User: "test", Name: "controller"}, true)
	conn := s.open(c, nil, "test")
	defer conn.Close()
	client := cloudapi.NewClient(conn)

	err := client.RemoveCloud("test-cloud")
	c.Assert(err, gc.ErrorMatches, `cloudregion not found`)
}

func (s *cloudSuite) TestModifyCloudAccess(c *gc.C) {
	ctx := context.Background()

	s.AssertAddController(ctx, c, params.EntityPath{User: "test", Name: "controller"}, true)
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
		HostCloudRegion:  "dummy/dummy-region",
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

	// Check that alice@external does not yet have access
	conn2 := s.open(c, nil, "alice")
	defer conn2.Close()
	client2 := cloudapi.NewClient(conn2)
	clouds, err = client2.Clouds()
	c.Assert(err, gc.Equals, nil)
	_, ok := clouds[names.NewCloudTag("test-cloud")]
	c.Assert(ok, gc.Equals, false, gc.Commentf("clouds: %#v", clouds))

	err = client.GrantCloud("alice@external", "add-model", "test-cloud")
	c.Assert(err, gc.Equals, nil)

	clouds, err = client2.Clouds()
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

	err = client.RevokeCloud("alice@external", "add-model", "test-cloud")
	c.Assert(err, gc.Equals, nil)
	clouds, err = client2.Clouds()
	c.Assert(err, gc.Equals, nil)
	_, ok = clouds[names.NewCloudTag("test-cloud")]
	c.Assert(ok, gc.Equals, false, gc.Commentf("clouds: %#v", clouds))
}

func (s *cloudSuite) TestModifyCloudAccessUnauthorized(c *gc.C) {
	ctx := context.Background()

	s.AssertAddController(ctx, c, params.EntityPath{User: "test", Name: "controller"}, true)
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
		HostCloudRegion:  "dummy/dummy-region",
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

	// Check that alice@external does not yet have access
	conn2 := s.open(c, nil, "alice")
	defer conn2.Close()
	client2 := cloudapi.NewClient(conn2)
	clouds, err = client2.Clouds()
	c.Assert(err, gc.Equals, nil)
	_, ok := clouds[names.NewCloudTag("test-cloud")]
	c.Assert(ok, gc.Equals, false, gc.Commentf("clouds: %#v", clouds))

	err = client2.GrantCloud("alice@external", "add-model", "test-cloud")
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
