// Copyright 2026 Canonical.

package testing

import (
	"context"
	"fmt"
	"sort"
	"testing"

	petname "github.com/dustinkirkland/golang-petname"
	qt "github.com/frankban/quicktest"
	"github.com/google/go-cmp/cmp/cmpopts"
	cloudapi "github.com/juju/juju/api/client/cloud"
	"github.com/juju/juju/api/client/modelmanager"
	"github.com/juju/juju/cloud"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v5"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/openfga"
	ofganames "github.com/canonical/jimm/v3/internal/openfga/names"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
)

func addCloud(c *qt.C, s jimmtest.JimmWithControllers, username string, cloud cloud.Cloud, force, cleanup bool) {
	conn := s.Open(c, nil, username, nil)
	defer conn.Close()

	client := cloudapi.NewClient(conn)
	err := client.AddCloud(cloud, force)
	c.Assert(err, qt.Equals, nil)
	if cleanup {
		c.Cleanup(func() {
			s.RemoveCloud(c, cloud.Name)
		})
	}
}

func TestCloudCall(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupJimmWithControllers(c)

	conn := s.Open(c, nil, "test", nil)
	defer conn.Close()
	client := cloudapi.NewClient(conn)
	info, err := client.Cloud(names.NewCloudTag(jimmtest.TestE2ECloudName))
	c.Assert(err, qt.Equals, nil)
	c.Assert(info, qt.CmpEquals(
		cmpopts.IgnoreFields(cloud.Cloud{}, "Endpoint", "Regions"),
	), cloud.Cloud{
		Name:      jimmtest.TestE2ECloudName,
		Type:      jimmtest.TestE2EProviderType,
		AuthTypes: cloud.AuthTypes{"certificate"},
	})
}

func TestClouds(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupJimmWithControllers(c)

	conn := s.Open(c, nil, "bob", nil)
	defer conn.Close()

	client := cloudapi.NewClient(conn)
	clouds, err := client.Clouds()
	c.Assert(err, qt.Equals, nil)
	e2eCloud, ok := clouds[names.NewCloudTag(jimmtest.TestE2ECloudName)]
	c.Assert(ok, qt.Equals, true)
	c.Assert(e2eCloud, qt.CmpEquals(
		cmpopts.IgnoreFields(cloud.Cloud{}, "Endpoint", "Regions"),
	), cloud.Cloud{
		Name:      jimmtest.TestE2ECloudName,
		Type:      jimmtest.TestE2EProviderType,
		AuthTypes: cloud.AuthTypes{"certificate"},
	})
}

func TestUserCredentials(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupJimmWithControllers(c)

	conn := s.Open(c, nil, "bob", nil)
	defer conn.Close()
	client := cloudapi.NewClient(conn)
	creds, err := client.UserCredentials(names.NewUserTag("bob@canonical.com"), names.NewCloudTag(jimmtest.TestE2ECloudName))
	c.Assert(err, qt.Equals, nil)
	c.Assert(creds, qt.HasLen, 1)
	c.Assert(creds[0], qt.Equals, names.NewCloudCredentialTag(jimmtest.TestE2ECloudName+"/bob@canonical.com/cred"))
}

func TestUserCredentialsWithDomain(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupJimmWithControllers(c)

	cct := names.NewCloudCredentialTag(jimmtest.TestE2ECloudName + "/test@domain/cred1")
	s.UpdateCloudCredential(c, cct, jujuparams.CloudCredential{
		AuthType: "credtype",
		Attributes: map[string]string{
			"attr1": "val1",
			"attr2": "val2",
		},
	})
	conn := s.Open(c, nil, "test@domain", nil)
	defer conn.Close()
	client := cloudapi.NewClient(conn)
	creds, err := client.UserCredentials(names.NewUserTag("test@domain"), names.NewCloudTag(jimmtest.TestE2ECloudName))
	c.Assert(err, qt.Equals, nil)
	c.Assert(creds, qt.HasLen, 1)
	c.Assert(creds[0], qt.Equals, cct)
}

func TestUserCredentialsErrors(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupJimmWithControllers(c)

	conn := s.Open(c, nil, "test", nil)
	defer conn.Close()
	req := jujuparams.UserClouds{
		UserClouds: []jujuparams.UserCloud{{
			UserTag:  "not-a-user-tag",
			CloudTag: jimmtest.TestE2ECloudName,
		}},
	}
	var resp jujuparams.StringsResults
	err := conn.APICall("Cloud", 7, "", "UserCredentials", req, &resp)
	c.Assert(err, qt.Equals, nil)
	c.Assert(resp.Results[0].Error, qt.ErrorMatches, `"not-a-user-tag" is not a valid tag`)
	c.Assert(resp.Results, qt.HasLen, 1)
}

func TestUpdateCloudCredentials(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupJimmWithControllers(c)

	conn := s.Open(c, nil, "test", nil)
	defer conn.Close()
	client := cloudapi.NewClient(conn)
	credentialTag := names.NewCloudCredentialTag(fmt.Sprintf(jimmtest.TestE2ECloudName + "/test@canonical.com/cred3"))
	reqCreds := map[string]cloud.Credential{
		credentialTag.String(): cloud.NewCredential("credtype", map[string]string{
			"attr1": "val31",
			"attr2": "val32",
		}),
	}
	res, err := client.UpdateCloudsCredentials(reqCreds, false)
	c.Assert(err, qt.Equals, nil)
	c.Assert(res, qt.DeepEquals, []jujuparams.UpdateCredentialResult{{
		CredentialTag: credentialTag.String(),
	}})
	creds, err := client.UserCredentials(names.NewUserTag("test@canonical.com"), names.NewCloudTag(jimmtest.TestE2ECloudName))
	c.Assert(err, qt.Equals, nil)
	c.Assert(creds, qt.HasLen, 1)
	c.Assert(creds[0], qt.Equals, credentialTag)
	_, err = client.UpdateCredentialsCheckModels(credentialTag, cloud.NewCredential("credtype", map[string]string{"attr1": "val33", "attr2": "val34"}))
	c.Assert(err, qt.IsNil)
	_, err = client.UserCredentials(names.NewUserTag("test@canonical.com"), names.NewCloudTag(jimmtest.TestE2ECloudName))
	c.Assert(err, qt.IsNil)
}

func TestUpdateCloudCredentialsErrors(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupJimmWithControllers(c)

	conn := s.Open(c, nil, "test", nil)
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
			Tag: names.NewCloudCredentialTag(jimmtest.TestE2ECloudName + "/test2@canonical.com/cred1").String(),
			Credential: jujuparams.CloudCredential{
				AuthType: "credtype",
				Attributes: map[string]string{
					"attr1": "val1",
				},
			},
		}, {
			Tag: names.NewCloudCredentialTag(jimmtest.TestE2ECloudName + "/test@canonical.com/bad-name-").String(),
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
	c.Assert(err, qt.Equals, nil)
	c.Assert(resp.Results, qt.HasLen, 3)
	c.Assert(resp.Results[0].Error, qt.ErrorMatches, `"not-a-cloud-credentials-tag" is not a valid tag`)
	c.Assert(resp.Results[1].Error, qt.ErrorMatches, `unauthorized`)
	c.Assert(resp.Results[2].Error, qt.Equals, (*jujuparams.Error)(nil))
}

func TestUpdateCloudCredentialsForce(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupJimmWithControllers(c)

	conn := s.Open(c, nil, "bob", nil)
	defer conn.Close()
	client := cloudapi.NewClient(conn)

	existingCloudCred := s.GetExistingClientCredentialsForCloud(c, jimmtest.TestE2ECloudName)
	_, err := client.UpdateCredentialsCheckModels(s.BobCredential.ResourceTag(),
		cloud.NewCredential("certificate", existingCloudCred.Attributes),
	)
	c.Assert(err, qt.Equals, nil)

	s.CreateModelForBob(c)

	args := jujuparams.UpdateCredentialArgs{
		Credentials: []jujuparams.TaggedCredential{{
			Tag: s.BobCredential.ResourceTag().String(),
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
	c.Assert(err, qt.Equals, nil)
	c.Assert(resp.Results[0].Error, qt.ErrorMatches, `some models are no longer visible`)

	// Check that the credentials have not been updated.
	creds, err := client.Credentials(s.BobCredential.ResourceTag())
	c.Assert(err, qt.Equals, nil)
	c.Assert(creds, qt.DeepEquals, []jujuparams.CloudCredentialResult{{
		Result: &jujuparams.CloudCredential{
			AuthType: "certificate",
			Redacted: []string{"client-cert", "client-key", "server-cert"},
		},
	}})

	args.Force = true
	err = conn.APICall("Cloud", 7, "", "UpdateCredentialsCheckModels", args, &resp)
	c.Assert(err, qt.Equals, nil)
	c.Check(resp.Results[0].Error, qt.ErrorMatches, `updating cloud credentials: validating credential "`+s.BobCredential.ResourceTag().Id()+`" for cloud "`+jimmtest.TestE2ECloudName+`": supported auth-types \["certificate"\], "badauthtype" not supported`)
	// Check that the credentials have been updated even though
	// we got an error.
	creds, err = client.Credentials(s.BobCredential.ResourceTag())
	c.Assert(err, qt.Equals, nil)
	sort.Strings(creds[0].Result.Redacted)
	c.Assert(creds, qt.DeepEquals, []jujuparams.CloudCredentialResult{{
		Result: &jujuparams.CloudCredential{
			AuthType: "badauthtype",
			Redacted: []string{"bad1attr", "bad2attr"},
		},
	}})
}

func TestCheckCredentialsModels(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupJimmWithControllers(c)

	model1 := s.CreateModelForBob(c)
	model2 := s.CreateModelForBob(c)

	conn := s.Open(c, nil, "bob@canonical.com", nil)
	defer conn.Close()

	existingCloudCred := s.GetExistingClientCredentialsForCloud(c, jimmtest.TestE2ECloudName)

	var resp jujuparams.UpdateCredentialResults
	err := conn.APICall("Cloud", 7, "", "CheckCredentialsModels", jujuparams.TaggedCredentials{
		Credentials: []jujuparams.TaggedCredential{{
			Tag: s.BobCredential.ResourceTag().String(),
			Credential: jujuparams.CloudCredential{
				AuthType:   "certificate",
				Attributes: existingCloudCred.Attributes,
			},
		}},
	}, &resp)
	c.Assert(err, qt.Equals, nil)
	modelResults := []jujuparams.UpdateCredentialModelResult{{
		ModelUUID: model1.UUID.String,
		ModelName: model1.Name,
	}, {
		ModelUUID: model2.UUID.String,
		ModelName: model2.Name,
	}}
	sort.Slice(modelResults, func(i, j int) bool {
		return modelResults[i].ModelUUID < modelResults[j].ModelUUID
	})
	sort.Slice(resp.Results[0].Models, func(i, j int) bool {
		return resp.Results[0].Models[i].ModelUUID < resp.Results[0].Models[j].ModelUUID
	})
	c.Assert(resp, qt.DeepEquals, jujuparams.UpdateCredentialResults{
		Results: []jujuparams.UpdateCredentialResult{{
			CredentialTag: s.BobCredential.ResourceTag().String(),
			Models:        modelResults,
		}},
	})
}

func TestCheckCredentialsModelsInvalidCreds(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupJimmWithControllers(c)

	conn := s.Open(c, nil, "bob@canonical.com", nil)
	defer conn.Close()

	existingCloudCred := s.GetExistingClientCredentialsForCloud(c, jimmtest.TestE2ECloudName)
	cred1 := cloud.NewCredential("certificate", existingCloudCred.Attributes)

	client := cloudapi.NewClient(conn)
	_, err := client.UpdateCredentialsCheckModels(s.BobCredential.ResourceTag(), cred1)
	c.Assert(err, qt.Equals, nil)

	model1 := s.CreateModelForBob(c)

	var resp jujuparams.UpdateCredentialResults
	err = conn.APICall("Cloud", 7, "", "CheckCredentialsModels", jujuparams.TaggedCredentials{
		Credentials: []jujuparams.TaggedCredential{{
			Tag: s.BobCredential.ResourceTag().String(),
			Credential: jujuparams.CloudCredential{
				AuthType: "unknowntype",
				Attributes: map[string]string{
					"x": "y",
				},
			},
		}},
	}, &resp)
	c.Assert(err, qt.Equals, nil)
	c.Assert(resp, qt.DeepEquals, jujuparams.UpdateCredentialResults{
		Results: []jujuparams.UpdateCredentialResult{{
			CredentialTag: s.BobCredential.ResourceTag().String(),
			Error:         &jujuparams.Error{Message: "some models are no longer visible"},
			Models: []jujuparams.UpdateCredentialModelResult{{
				ModelUUID: model1.UUID.String,
				ModelName: model1.Name,
				Errors: []jujuparams.ErrorResult{{
					Error: &jujuparams.Error{
						Message: `validating credential "` + s.BobCredential.ResourceTag().Id() + `" for cloud "` + jimmtest.TestE2ECloudName + `": supported auth-types ["certificate"], "unknowntype" not supported`,
						Code:    "not supported",
					},
				}},
			}},
		}},
	})
}

func TestCredential(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupJimmWithControllers(c)

	conn := s.Open(c, nil, "test", nil)
	defer conn.Close()
	cred1Name := petname.Generate(2, "-")
	cred1Tag := names.NewCloudCredentialTag(jimmtest.TestE2ECloudName + "/test@canonical.com/" + cred1Name)
	cred1 := cloud.NewCredential("userpass", map[string]string{
		"username": "cloud-user",
		"password": "cloud-pass",
	})
	cred2Name := petname.Generate(2, "-")
	cred2Tag := names.NewCloudCredentialTag(jimmtest.TestE2ECloudName + "/test@canonical.com/" + cred2Name)
	cred2 := cloud.NewCredential("empty", nil)

	client := cloudapi.NewClient(conn)
	_, err := client.UpdateCredentialsCheckModels(cred1Tag, cred1)
	c.Assert(err, qt.Equals, nil)
	_, err = client.UpdateCredentialsCheckModels(cred2Tag, cred2)
	c.Assert(err, qt.Equals, nil)

	creds, err := client.Credentials(
		cred1Tag,
		cred2Tag,
		names.NewCloudCredentialTag(jimmtest.TestE2ECloudName+"/test@canonical.com/cred3"),
		names.NewCloudCredentialTag(jimmtest.TestE2ECloudName+"/no-test@canonical.com/cred4"),
		names.NewCloudCredentialTag(jimmtest.TestE2ECloudName+"/admin@local/cred6"),
	)
	c.Assert(err, qt.Equals, nil)
	for i := range creds {
		if creds[i].Result == nil {
			continue
		}
		sort.Strings(creds[i].Result.Redacted)
	}
	c.Assert(creds, qt.ContentEquals, []jujuparams.CloudCredentialResult{{
		Result: &jujuparams.CloudCredential{
			AuthType: "userpass",
			Redacted: []string{
				"password",
				"username",
			},
		},
	}, {
		Result: &jujuparams.CloudCredential{
			AuthType: "empty",
		},
	}, {
		Error: &jujuparams.Error{
			Message: `cloudcredential "` + jimmtest.TestE2ECloudName + `/test@canonical.com/cred3" not found`,
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

func TestRevokeCredential(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupJimmWithControllers(c)

	conn := s.Open(c, nil, "test", nil)
	defer conn.Close()
	client := cloudapi.NewClient(conn)
	credName := petname.Generate(2, "-")
	credTag := names.NewCloudCredentialTag(jimmtest.TestE2ECloudName + "/test@canonical.com/" + credName)
	_, err := client.UpdateCredentialsCheckModels(
		credTag,
		cloud.NewCredential("empty", nil),
	)
	c.Assert(err, qt.Equals, nil)

	tags, err := client.UserCredentials(credTag.Owner(), credTag.Cloud())
	c.Assert(err, qt.Equals, nil)
	c.Assert(tags, qt.HasLen, 1)
	c.Assert(tags[0], qt.Equals, credTag)

	ccr, err := client.Credentials(credTag)
	c.Assert(err, qt.Equals, nil)
	c.Assert(ccr, qt.DeepEquals, []jujuparams.CloudCredentialResult{{
		Result: &jujuparams.CloudCredential{
			AuthType: "empty",
		},
	}})

	err = client.RevokeCredential(credTag, false)
	c.Assert(err, qt.Equals, nil)

	ccr, err = client.Credentials(credTag)
	c.Assert(err, qt.Equals, nil)
	c.Assert(ccr, qt.DeepEquals, []jujuparams.CloudCredentialResult{{
		Error: &jujuparams.Error{
			Code:    jujuparams.CodeNotFound,
			Message: `cloudcredential "` + jimmtest.TestE2ECloudName + `/test@canonical.com/` + credName + `" not found`,
		},
	}})

	tags, err = client.UserCredentials(credTag.Owner(), credTag.Cloud())
	c.Assert(err, qt.Equals, nil)
	c.Assert(tags, qt.HasLen, 0)
}

func TestAddCloud(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupJimmWithControllers(c)

	cloudName := petname.Generate(2, "-")
	addCloud(c, s, "test", cloud.Cloud{
		Name:             cloudName,
		Type:             "kubernetes",
		AuthTypes:        cloud.AuthTypes{cloud.CertificateAuthType},
		Endpoint:         "https://0.1.2.3:5678",
		IdentityEndpoint: "https://0.1.2.3:5679",
		StorageEndpoint:  "https://0.1.2.3:5680",
		HostCloudRegion:  jimmtest.TestE2ECloudName + "/" + jimmtest.TestE2ECloudRegionName,
	}, false, true)

	conn := s.Open(c, nil, "test", nil)
	defer conn.Close()
	client := cloudapi.NewClient(conn)
	clouds, err := client.Clouds()
	c.Assert(err, qt.Equals, nil)
	c.Assert(clouds[names.NewCloudTag(cloudName)], qt.CmpEquals(
		cmpopts.IgnoreFields(cloud.Cloud{}, "Endpoint", "IdentityEndpoint", "StorageEndpoint"),
		cmpopts.IgnoreFields(cloud.Region{}, "Endpoint", "IdentityEndpoint", "StorageEndpoint"),
	), cloud.Cloud{
		Name:      cloudName,
		Type:      "kubernetes",
		AuthTypes: cloud.AuthTypes{"certificate"},
		Regions: []cloud.Region{{
			Name: "default",
		}},
	})
}

func TestRevokeCredentialsCheckModels(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupJimmWithControllers(c)

	s.AddAdminUser(c, "test@canonical.com")
	conn := s.Open(c, nil, "test", nil)
	defer conn.Close()
	client := cloudapi.NewClient(conn)
	credentialName := petname.Generate(2, "-")
	credTag := names.NewCloudCredentialTag(jimmtest.TestE2ECloudName + "/test@canonical.com/" + credentialName)
	existingCloudCred := s.GetExistingClientCredentialsForCloud(c, jimmtest.TestE2ECloudName)
	_, err := client.UpdateCredentialsCheckModels(
		credTag,
		cloud.NewCredential("certificate", existingCloudCred.Attributes),
	)
	c.Assert(err, qt.Equals, nil)

	tags, err := client.UserCredentials(credTag.Owner(), credTag.Cloud())
	c.Assert(err, qt.Equals, nil)
	c.Assert(tags, qt.HasLen, 1)
	c.Assert(tags[0], qt.Equals, credTag)

	ccr, err := client.Credentials(credTag)
	c.Assert(err, qt.Equals, nil)
	c.Assert(ccr, qt.DeepEquals, []jujuparams.CloudCredentialResult{{
		Result: &jujuparams.CloudCredential{
			AuthType: "certificate",
			Redacted: []string{"client-cert", "client-key", "server-cert"},
		},
	}})

	mmclient := modelmanager.NewClient(conn)
	modelName := petname.Generate(2, "-")
	modelInfo, err := mmclient.CreateModel(modelName, "test@canonical.com", jimmtest.TestE2ECloudName, jimmtest.TestE2ECloudRegionName, credTag, nil)
	c.Assert(err, qt.Equals, nil)

	var resp jujuparams.ErrorResults
	err = conn.APICall("Cloud", 7, "", "RevokeCredentialsCheckModels", jujuparams.RevokeCredentialArgs{
		Credentials: []jujuparams.RevokeCredentialArg{{
			Tag:   credTag.String(),
			Force: false,
		}},
	}, &resp)
	c.Assert(err, qt.Equals, nil)
	c.Assert(resp.Results[0].Error, qt.ErrorMatches, `cloud credential still used by 1 model\(s\)`)

	resp.Results = nil
	// we don't support the force flag, so the test should fail again.
	err = conn.APICall("Cloud", 7, "", "RevokeCredentialsCheckModels", jujuparams.RevokeCredentialArgs{
		Credentials: []jujuparams.RevokeCredentialArg{{
			Tag:   credTag.String(),
			Force: true,
		}},
	}, &resp)
	c.Assert(err, qt.Equals, nil)
	c.Assert(resp.Results[0].Error, qt.ErrorMatches, `cloud credential still used by 1 model\(s\)`)

	s.DestroyModelAndDeleteFromDatabase(c, names.NewModelTag(modelInfo.UUID))

	resp.Results = nil
	err = conn.APICall("Cloud", 7, "", "RevokeCredentialsCheckModels", jujuparams.RevokeCredentialArgs{
		Credentials: []jujuparams.RevokeCredentialArg{{
			Tag:   credTag.String(),
			Force: false,
		}},
	}, &resp)
	c.Assert(err, qt.Equals, nil)
	c.Assert(resp.Results[0].Error, qt.Equals, (*jujuparams.Error)(nil))

	ccr, err = client.Credentials(credTag)
	c.Assert(err, qt.Equals, nil)
	c.Assert(ccr, qt.DeepEquals, []jujuparams.CloudCredentialResult{{
		Error: &jujuparams.Error{
			Code:    jujuparams.CodeNotFound,
			Message: `cloudcredential "` + jimmtest.TestE2ECloudName + `/test@canonical.com/` + credentialName + `" not found`,
		},
	}})

	tags, err = client.UserCredentials(credTag.Owner(), credTag.Cloud())
	c.Assert(err, qt.Equals, nil)
	c.Assert(tags, qt.HasLen, 0)
}

func TestAddCloudError(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupJimmWithControllers(c)

	conn := s.Open(c, nil, "test", nil)
	defer conn.Close()
	client := cloudapi.NewClient(conn)
	cloudName := petname.Generate(2, "-")
	err := client.AddCloud(cloud.Cloud{
		Name:             cloudName,
		Type:             "kubernetes",
		Endpoint:         "https://0.1.2.3:5678",
		IdentityEndpoint: "https://0.1.2.3:5679",
		StorageEndpoint:  "https://0.1.2.3:5680",
		HostCloudRegion:  jimmtest.TestE2ECloudName + "/" + jimmtest.TestE2ECloudRegionName,
	}, false)
	c.Assert(err, qt.ErrorMatches, `invalid cloud: empty auth-types not valid.*`)
}

func TestAddCloudNoHostCloudRegion(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupJimmWithControllers(c)

	conn := s.Open(c, nil, "test", nil)
	defer conn.Close()
	client := cloudapi.NewClient(conn)
	cloudName := petname.Generate(2, "-")
	err := client.AddCloud(cloud.Cloud{
		Name:             cloudName,
		Type:             "kubernetes",
		Endpoint:         "https://0.1.2.3:5678",
		IdentityEndpoint: "https://0.1.2.3:5679",
		StorageEndpoint:  "https://0.1.2.3:5680",
	}, false)
	c.Assert(err, qt.ErrorMatches, `cloud host region not specified \(cloud region required\)`)
	c.Assert(jujuparams.ErrCode(err), qt.Equals, jujuparams.CodeCloudRegionRequired)
}

func TestAddCloudBadName(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupJimmWithControllers(c)

	conn := s.Open(c, nil, "test", nil)
	defer conn.Close()
	client := cloudapi.NewClient(conn)
	err := client.AddCloud(cloud.Cloud{
		Name:             "aws",
		Type:             "kubernetes",
		HostCloudRegion:  jimmtest.TestE2ECloudName + "/" + jimmtest.TestE2ECloudRegionName,
		Endpoint:         "https://0.1.2.3:5678",
		IdentityEndpoint: "https://0.1.2.3:5679",
		StorageEndpoint:  "https://0.1.2.3:5680",
	}, false)
	c.Assert(err, qt.ErrorMatches, `cloud "aws" already exists \(already exists\)`)
}

func TestAddCredential(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupJimmWithControllers(c)

	conn := s.Open(c, nil, "test", nil)
	defer conn.Close()
	client := cloudapi.NewClient(conn)
	credentialTag := names.NewCloudCredentialTag(jimmtest.TestE2ECloudName + "/test@canonical.com/cred3")
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
	c.Assert(err, qt.Equals, nil)
	creds, err := client.CredentialContents(jimmtest.TestE2ECloudName, "cred3", true)
	c.Assert(err, qt.Equals, nil)
	c.Assert(creds, qt.DeepEquals, []jujuparams.CredentialContentResult{{
		Result: &jujuparams.ControllerCredentialInfo{
			Content: jujuparams.CredentialContent{
				Name:     "cred3",
				Cloud:    jimmtest.TestE2ECloudName,
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
	c.Assert(err, qt.Equals, nil)
	creds, err = client.CredentialContents(jimmtest.TestE2ECloudName, "cred3", true)
	c.Assert(err, qt.Equals, nil)
	c.Assert(creds, qt.DeepEquals, []jujuparams.CredentialContentResult{{
		Result: &jujuparams.ControllerCredentialInfo{
			Content: jujuparams.CredentialContent{
				Name:     "cred3",
				Cloud:    jimmtest.TestE2ECloudName,
				AuthType: "userpass",
				Attributes: map[string]string{
					"username": "test-user2",
					"password": "S3cret2",
				},
			},
		},
	}})
}

func TestCredentialContents(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupJimmWithControllers(c)

	conn := s.Open(c, nil, "bob@canonical.com", nil)
	defer conn.Close()
	client := cloudapi.NewClient(conn)

	existingCloudCred := s.GetExistingClientCredentialsForCloud(c, jimmtest.TestE2ECloudName)
	cred1 := cloud.NewCredential("certificate", existingCloudCred.Attributes)

	creds, err := client.CredentialContents(jimmtest.TestE2ECloudName, s.BobCredential.Name, false)
	c.Assert(err, qt.Equals, nil)
	c.Assert(creds, qt.DeepEquals, []jujuparams.CredentialContentResult{{
		Result: &jujuparams.ControllerCredentialInfo{
			Content: jujuparams.CredentialContent{
				Name:     s.BobCredential.Name,
				Cloud:    jimmtest.TestE2ECloudName,
				AuthType: "certificate",
			},
		},
	}})

	model := s.CreateModelForBob(c)

	creds, err = client.CredentialContents(jimmtest.TestE2ECloudName, s.BobCredential.Name, true)
	c.Assert(err, qt.Equals, nil)
	c.Assert(creds, qt.DeepEquals, []jujuparams.CredentialContentResult{{
		Result: &jujuparams.ControllerCredentialInfo{
			Content: jujuparams.CredentialContent{
				Name:       s.BobCredential.Name,
				Cloud:      jimmtest.TestE2ECloudName,
				AuthType:   "certificate",
				Attributes: cred1.Attributes(),
			},
			Models: []jujuparams.ModelAccess{{
				Model:  model.Name,
				Access: "admin",
			}},
		},
	}})

	// unspecified credentials return all.
	creds, err = client.CredentialContents("", "", true)
	c.Assert(err, qt.Equals, nil)
	c.Assert(creds, qt.DeepEquals, []jujuparams.CredentialContentResult{{
		Result: &jujuparams.ControllerCredentialInfo{
			Content: jujuparams.CredentialContent{
				Name:       s.BobCredential.Name,
				Cloud:      jimmtest.TestE2ECloudName,
				AuthType:   "certificate",
				Attributes: cred1.Attributes(),
			},
			Models: []jujuparams.ModelAccess{{
				Model:  model.Name,
				Access: "admin",
			}},
		},
	},
	})
}

func TestCredentialContentsWithEmptyAttributes(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupJimmWithControllers(c)

	conn := s.Open(c, nil, "test", nil)
	defer conn.Close()
	client := cloudapi.NewClient(conn)
	credentialName := petname.Generate(2, "-")
	credentialTag := names.NewCloudCredentialTag(jimmtest.TestE2ECloudName + "/test@canonical.com/" + credentialName)
	err := client.AddCredential(
		credentialTag.String(),
		cloud.NewCredential(
			"certificate",
			nil,
		),
	)
	c.Assert(err, qt.Equals, nil)
	creds, err := client.CredentialContents(jimmtest.TestE2ECloudName, credentialName, false)
	c.Assert(err, qt.Equals, nil)
	c.Assert(creds, qt.DeepEquals, []jujuparams.CredentialContentResult{{
		Result: &jujuparams.ControllerCredentialInfo{
			Content: jujuparams.CredentialContent{
				Name:       credentialName,
				Cloud:      jimmtest.TestE2ECloudName,
				AuthType:   "certificate",
				Attributes: nil,
			},
		},
	}})
}

func TestRemoveCloud(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupJimmWithControllers(c)

	cloudName := petname.Generate(2, "-")
	addCloud(c, s, "test", cloud.Cloud{
		Name:             cloudName,
		Type:             "kubernetes",
		AuthTypes:        cloud.AuthTypes{cloud.CertificateAuthType},
		Endpoint:         "https://0.1.2.3:5678",
		IdentityEndpoint: "https://0.1.2.3:5679",
		StorageEndpoint:  "https://0.1.2.3:5680",
		HostCloudRegion:  jimmtest.TestE2ECloudName + "/" + jimmtest.TestE2ECloudRegionName,
	}, false, false)

	conn := s.Open(c, nil, "test", nil)
	defer conn.Close()
	client := cloudapi.NewClient(conn)
	clouds, err := client.Clouds()
	c.Assert(err, qt.Equals, nil)
	c.Assert(clouds[names.NewCloudTag(cloudName)], qt.CmpEquals(
		cmpopts.IgnoreFields(cloud.Cloud{}, "Endpoint", "IdentityEndpoint", "StorageEndpoint"),
		cmpopts.IgnoreFields(cloud.Region{}, "Endpoint", "IdentityEndpoint", "StorageEndpoint"),
	), cloud.Cloud{
		Name:      cloudName,
		Type:      "kubernetes",
		AuthTypes: cloud.AuthTypes{"certificate"},
		Regions: []cloud.Region{{
			Name: "default",
		}},
	})

	err = client.RemoveCloud(cloudName)
	c.Assert(err, qt.Equals, nil)
	clouds, err = client.Clouds()
	c.Assert(err, qt.Equals, nil)
	c.Assert(clouds[names.NewCloudTag(cloudName)], qt.DeepEquals, cloud.Cloud{})
}

func TestRemoveCloudNotFound(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupJimmWithControllers(c)

	conn := s.Open(c, nil, "test", nil)
	defer conn.Close()
	client := cloudapi.NewClient(conn)

	cloudName := petname.Generate(2, "-")
	err := client.RemoveCloud(cloudName)
	c.Assert(err, qt.ErrorMatches, `cloud "`+cloudName+`" not found`)
}

func TestModifyCloudAccess(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupJimmWithControllers(c)

	cloudName := petname.Generate(2, "-")
	addCloud(c, s, "test", cloud.Cloud{
		Name:             cloudName,
		Type:             "kubernetes",
		AuthTypes:        cloud.AuthTypes{cloud.CertificateAuthType},
		Endpoint:         "https://0.1.2.3:5678",
		IdentityEndpoint: "https://0.1.2.3:5679",
		StorageEndpoint:  "https://0.1.2.3:5680",
		HostCloudRegion:  jimmtest.TestE2ECloudName + "/" + jimmtest.TestE2ECloudRegionName,
	}, false, true)

	conn := s.Open(c, nil, "test", nil)
	defer conn.Close()
	client := cloudapi.NewClient(conn)
	clouds, err := client.Clouds()
	c.Assert(err, qt.Equals, nil)
	_, ok := clouds[names.NewCloudTag(cloudName)]
	c.Assert(ok, qt.IsTrue)

	// Check that bob@canonical.com does not yet have access
	conn2 := s.Open(c, nil, "bob", nil)
	defer conn2.Close()
	client2 := cloudapi.NewClient(conn2)
	clouds, err = client2.Clouds()
	c.Assert(err, qt.Equals, nil)
	_, ok = clouds[names.NewCloudTag(cloudName)]
	c.Assert(ok, qt.Equals, false, qt.Commentf("clouds: %#v", clouds))

	err = client.GrantCloud("bob@canonical.com", "add-model", cloudName)
	c.Assert(err, qt.Equals, nil)

	clouds, err = client2.Clouds()
	c.Assert(err, qt.Equals, nil)
	_, ok = clouds[names.NewCloudTag(cloudName)]
	c.Assert(ok, qt.IsTrue)

	err = client.RevokeCloud("bob@canonical.com", "add-model", cloudName)
	c.Assert(err, qt.Equals, nil)
	clouds, err = client2.Clouds()
	c.Assert(err, qt.Equals, nil)
	_, ok = clouds[names.NewCloudTag(cloudName)]
	c.Assert(ok, qt.Equals, false, qt.Commentf("clouds: %#v", clouds))
}

func TestModifyCloudAccessUnauthorized(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupJimmWithControllers(c)

	cloudName := petname.Generate(2, "-")
	addCloud(c, s, "test", cloud.Cloud{
		Name:             cloudName,
		Type:             "kubernetes",
		AuthTypes:        cloud.AuthTypes{cloud.CertificateAuthType},
		Endpoint:         "https://0.1.2.3:5678",
		IdentityEndpoint: "https://0.1.2.3:5679",
		StorageEndpoint:  "https://0.1.2.3:5680",
		HostCloudRegion:  jimmtest.TestE2ECloudName + "/" + jimmtest.TestE2ECloudRegionName,
	}, false, true)

	conn := s.Open(c, nil, "test", nil)
	defer conn.Close()
	client := cloudapi.NewClient(conn)
	clouds, err := client.Clouds()
	c.Assert(err, qt.Equals, nil)
	_, ok := clouds[names.NewCloudTag(cloudName)]
	c.Assert(ok, qt.IsTrue)

	// Try granting cloud access as an unauthorized user.
	conn2 := s.Open(c, nil, "charlie", nil)
	defer conn2.Close()
	client2 := cloudapi.NewClient(conn2)
	err = client2.GrantCloud("charlie@canonical.com", "add-model", "test-cloud")
	c.Assert(err, qt.ErrorMatches, `unauthorized`)
}

func TestUpdateCloud(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupJimmWithControllers(c)

	conn := s.Open(c, nil, "test", nil)
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
	c.Assert(jujuparams.IsCodeForbidden(err), qt.Equals, true, qt.Commentf("%#v", err))
}

func TestCloudInfo(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupJimmWithControllers(c)

	cloudName := petname.Generate(2, "-")
	addCloud(c, s, "alice", cloud.Cloud{
		Name:             cloudName,
		Type:             "kubernetes",
		AuthTypes:        cloud.AuthTypes{cloud.CertificateAuthType},
		Endpoint:         "https://0.1.2.3:5678",
		IdentityEndpoint: "https://0.1.2.3:5679",
		StorageEndpoint:  "https://0.1.2.3:5680",
		HostCloudRegion:  jimmtest.TestE2ECloudName + "/" + jimmtest.TestE2ECloudRegionName,
	}, false, true)

	conn := s.Open(c, nil, "bob", nil)
	defer conn.Close()

	args := jujuparams.Entities{
		Entities: []jujuparams.Entity{{
			Tag: names.NewCloudTag(jimmtest.TestE2ECloudName).String(),
		}, {
			Tag: names.NewCloudTag("no-such-cloud").String(),
		}, {
			Tag: names.NewUserTag("not-a-cloud").String(),
		}, {
			Tag: names.NewCloudTag(cloudName).String(),
		}},
	}
	var result jujuparams.CloudInfoResults
	err := conn.APICall("Cloud", 7, "", "CloudInfo", args, &result)
	c.Assert(err, qt.Equals, nil)
	c.Assert(result, qt.CmpEquals(
		cmpopts.IgnoreFields(jujuparams.CloudDetails{}, "Endpoint", "IdentityEndpoint", "StorageEndpoint"),
		cmpopts.IgnoreFields(jujuparams.CloudRegion{}, "Endpoint", "IdentityEndpoint", "StorageEndpoint"),
	), jujuparams.CloudInfoResults{
		Results: []jujuparams.CloudInfoResult{{
			Result: &jujuparams.CloudInfo{
				CloudDetails: jujuparams.CloudDetails{
					Type:      jimmtest.TestE2EProviderType,
					AuthTypes: []string{"certificate"},
					Regions: []jujuparams.CloudRegion{{
						Name: jimmtest.TestE2ECloudRegionName,
					}},
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

func TestListCloudInfo(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupJimmWithControllers(c)

	cloudName := petname.Generate(2, "-")
	addCloud(c, s, "alice", cloud.Cloud{
		Name:             cloudName,
		Type:             "kubernetes",
		AuthTypes:        cloud.AuthTypes{cloud.CertificateAuthType},
		Endpoint:         "https://0.1.2.3:5678",
		IdentityEndpoint: "https://0.1.2.3:5679",
		StorageEndpoint:  "https://0.1.2.3:5680",
		HostCloudRegion:  jimmtest.TestE2ECloudName + "/" + jimmtest.TestE2ECloudRegionName,
	}, false, true)

	bobIdentity, err := dbmodel.NewIdentity("bob@canonical.com")
	c.Assert(err, qt.IsNil)
	bob := openfga.NewUser(bobIdentity, s.OFGAClient)
	err = bob.SetCloudAccess(context.Background(), names.NewCloudTag(cloudName), ofganames.CanAddModelRelation)
	c.Assert(err, qt.Equals, nil)
	err = bob.SetCloudAccess(context.Background(), names.NewCloudTag(jimmtest.TestE2ECloudName), ofganames.CanAddModelRelation)
	c.Assert(err, qt.Equals, nil)

	aliceIdentity, err := dbmodel.NewIdentity("alice@canonical.com")
	c.Assert(err, qt.IsNil)
	alice := openfga.NewUser(aliceIdentity, s.OFGAClient)
	err = alice.SetCloudAccess(context.Background(), names.NewCloudTag(jimmtest.TestE2ECloudName), ofganames.CanAddModelRelation)
	c.Assert(err, qt.Equals, nil)

	args := jujuparams.ListCloudsRequest{
		UserTag: names.NewUserTag("alice@canonical.com").String(),
		All:     false,
	}
	var result jujuparams.ListCloudInfoResults
	conn := s.Open(c, nil, "alice", nil)
	defer conn.Close()
	err = conn.APICall("Cloud", 7, "", "ListCloudInfo", args, &result)
	c.Assert(err, qt.Equals, nil)
	sort.Slice(result.Results, func(i, j int) bool {
		return result.Results[i].Result.Type > result.Results[j].Result.Type
	})
	c.Check(result, qt.CmpEquals(
		cmpopts.IgnoreFields(jujuparams.CloudDetails{}, "Endpoint", "IdentityEndpoint", "StorageEndpoint"),
		cmpopts.IgnoreFields(jujuparams.CloudRegion{}, "Endpoint", "IdentityEndpoint", "StorageEndpoint"),
	), jujuparams.ListCloudInfoResults{
		Results: []jujuparams.ListCloudInfoResult{
			{
				Result: &jujuparams.ListCloudInfo{
					CloudDetails: jujuparams.CloudDetails{
						Type:      jimmtest.TestE2EProviderType,
						AuthTypes: []string{"certificate"},
						Regions: []jujuparams.CloudRegion{{
							Name: jimmtest.TestE2ECloudRegionName,
						}},
					},
					Access: "admin",
				},
			}, {
				Result: &jujuparams.ListCloudInfo{
					CloudDetails: jujuparams.CloudDetails{
						Type:      "kubernetes",
						AuthTypes: []string{"certificate"},
						Regions: []jujuparams.CloudRegion{{
							Name: "default",
						}},
					},
					Access: "admin",
				},
			}},
	})

	conn = s.Open(c, nil, "bob", nil)
	defer conn.Close()

	args = jujuparams.ListCloudsRequest{
		UserTag: names.NewUserTag("bob@canonical.com").String(),
		All:     false,
	}
	result.Results = nil
	err = conn.APICall("Cloud", 7, "", "ListCloudInfo", args, &result)
	c.Assert(err, qt.Equals, nil)

	sort.Slice(result.Results, func(i, j int) bool {
		return result.Results[i].Result.Type > result.Results[j].Result.Type
	})
	c.Check(result, qt.CmpEquals(
		cmpopts.IgnoreFields(jujuparams.CloudDetails{}, "Endpoint", "IdentityEndpoint", "StorageEndpoint"),
		cmpopts.IgnoreFields(jujuparams.CloudRegion{}, "Endpoint", "IdentityEndpoint", "StorageEndpoint"),
	), jujuparams.ListCloudInfoResults{
		Results: []jujuparams.ListCloudInfoResult{
			{
				Result: &jujuparams.ListCloudInfo{
					CloudDetails: jujuparams.CloudDetails{
						Type:      jimmtest.TestE2EProviderType,
						AuthTypes: []string{"certificate"},
						Regions: []jujuparams.CloudRegion{{
							Name: jimmtest.TestE2ECloudRegionName,
						}},
					},
					Access: "add-model",
				},
			}, {
				Result: &jujuparams.ListCloudInfo{
					CloudDetails: jujuparams.CloudDetails{
						Type:      "kubernetes",
						AuthTypes: []string{"certificate"},
						Regions: []jujuparams.CloudRegion{{
							Name: "default",
						}},
					},
					Access: "add-model",
				},
			}},
	})
}
