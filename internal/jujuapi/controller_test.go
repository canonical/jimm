// Copyright 2016 Canonical Ltd.

package jujuapi_test

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	cloudapi "github.com/juju/juju/api/cloud"
	controllerapi "github.com/juju/juju/api/controller"
	"github.com/juju/juju/api/modelmanager"
	"github.com/juju/juju/api/usermanager"
	jujuparams "github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/controller"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"
	"gopkg.in/macaroon.v2-unstable"

	"github.com/CanonicalLtd/jimm/internal/jujuapi"
	"github.com/CanonicalLtd/jimm/internal/mongodoc"
	"github.com/CanonicalLtd/jimm/params"
)

type controllerSuite struct {
	websocketSuite
}

var _ = gc.Suite(&controllerSuite{})

func (s *controllerSuite) SetUpTest(c *gc.C) {
	s.ServerParams.CharmstoreLocation = "https://api.jujucharms.com/charmstore"
	s.ServerParams.MeteringLocation = "https://api.jujucharms.com/omnibus"
	s.websocketSuite.SetUpTest(c)
	s.PatchValue(&utils.OutgoingAccessAllowed, true)
}

func (s *controllerSuite) TestServerVersion(c *gc.C) {
	ctlPath := params.EntityPath{"test", "controller-1"}
	s.AssertAddController(c, ctlPath, true)
	testVersion := version.MustParse("5.4.3")
	err := s.JEM.DB.SetControllerVersion(testContext, ctlPath, testVersion)
	c.Assert(err, gc.Equals, nil)

	conn := s.open(c, nil, "test")
	defer conn.Close()

	v, ok := conn.ServerVersion()
	c.Assert(ok, gc.Equals, true)
	c.Assert(v, jc.DeepEquals, testVersion)
}

func (s *controllerSuite) TestOldAdminVersionFails(c *gc.C) {
	s.AssertAddController(c, params.EntityPath{User: "test", Name: "controller-1"}, true)
	cred := s.AssertUpdateCredential(c, "test", "dummy", "cred1", "empty")
	mi := s.assertCreateModel(c, createModelParams{name: "model-1", username: "test", cred: cred})
	modelUUID := mi.UUID
	conn := s.open(c, &api.Info{
		ModelTag:  names.NewModelTag(modelUUID),
		SkipLogin: true,
	}, "test")
	defer conn.Close()
	var resp jujuparams.RedirectInfoResult
	err := conn.APICall("Admin", 2, "", "Login", nil, &resp)
	c.Assert(err, gc.ErrorMatches, `JIMM does not support login from old clients \(not supported\)`)
	c.Assert(resp, jc.DeepEquals, jujuparams.RedirectInfoResult{})
}

func (s *controllerSuite) TestAdminIDFails(c *gc.C) {
	s.AssertAddController(c, params.EntityPath{User: "test", Name: "controller-1"}, true)
	cred := s.AssertUpdateCredential(c, "test", "dummy", "cred1", "empty")
	mi := s.assertCreateModel(c, createModelParams{name: "model-1", username: "test", cred: cred})
	modelUUID := mi.UUID
	conn := s.open(c, &api.Info{
		ModelTag:  names.NewModelTag(modelUUID),
		SkipLogin: true,
	}, "test")
	defer conn.Close()
	var resp jujuparams.RedirectInfoResult
	err := conn.APICall("Admin", 3, "Object ID", "Login", nil, &resp)
	c.Assert(err, gc.ErrorMatches, "id not found")
}

func (s *controllerSuite) TestLoginToController(c *gc.C) {
	s.AssertAddController(c, params.EntityPath{User: "test", Name: "controller-1"}, true)
	conn := s.open(c, &api.Info{
		SkipLogin: true,
	}, "test")
	defer conn.Close()
	err := conn.Login(nil, "", "", nil)
	c.Assert(err, gc.Equals, nil)
	var resp jujuparams.RedirectInfoResult
	err = conn.APICall("Admin", 3, "", "RedirectInfo", nil, &resp)
	c.Assert(err, gc.ErrorMatches, `no such request - method Admin.RedirectInfo is not implemented \(not implemented\)`)
}

func (s *controllerSuite) TestLoginToControllerWithInvalidMacaroon(c *gc.C) {
	s.AssertAddController(c, params.EntityPath{User: "test", Name: "controller-1"}, true)
	invalidMacaroon, err := macaroon.New(nil, nil, "")
	c.Assert(err, gc.Equals, nil)
	conn := s.open(c, &api.Info{
		Macaroons: []macaroon.Slice{{invalidMacaroon}},
	}, "test")
	conn.Close()
}

func (s *controllerSuite) TestUnimplementedMethodFails(c *gc.C) {
	s.AssertAddController(c, params.EntityPath{User: "test", Name: "controller-1"}, true)
	cred := s.AssertUpdateCredential(c, "test", "dummy", "cred1", "empty")
	mi := s.assertCreateModel(c, createModelParams{name: "model-1", username: "test", cred: cred})
	modelUUID := mi.UUID
	conn := s.open(c, &api.Info{
		ModelTag:  names.NewModelTag(modelUUID),
		SkipLogin: true,
	}, "test")
	defer conn.Close()
	var resp jujuparams.RedirectInfoResult
	err := conn.APICall("Admin", 3, "", "Logout", nil, &resp)
	c.Assert(err, gc.ErrorMatches, `no such request - method Admin.Logout is not implemented \(not implemented\)`)
}

func (s *controllerSuite) TestUnimplementedRootFails(c *gc.C) {
	s.AssertAddController(c, params.EntityPath{User: "test", Name: "controller-1"}, true)
	conn := s.open(c, nil, "test")
	defer conn.Close()
	var resp jujuparams.RedirectInfoResult
	err := conn.APICall("NoSuch", 1, "", "Method", nil, &resp)
	c.Assert(err, gc.ErrorMatches, `unknown version \(1\) of interface "NoSuch" \(not implemented\)`)
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

func (s *controllerSuite) TestDefaultCloud(c *gc.C) {
	conn := s.open(c, nil, "test")
	defer conn.Close()
	client := cloudapi.NewClient(conn)
	for i, test := range defaultCloudTests {
		c.Logf("test %d: %s", i, test.about)
		_, err := s.JEM.DB.Controllers().RemoveAll(nil)
		c.Assert(err, gc.Equals, nil)
		_, err = s.JEM.DB.CloudRegions().RemoveAll(nil)
		c.Assert(err, gc.Equals, nil)
		for j, cloud := range test.cloudNames {
			ctlPath := params.EntityPath{User: "test", Name: params.Name(fmt.Sprintf("controller-%d", j))}
			err := s.JEM.DB.AddController(testContext, &mongodoc.Controller{
				Path:   ctlPath,
				ACL:    params.ACL{Read: []string{"everyone"}},
				CACert: "cacert",
				UUID:   fmt.Sprintf("uuid%d", j),
				Public: true,
			})
			c.Assert(err, gc.Equals, nil)
			err = s.JEM.DB.UpdateCloudRegions(testContext, []mongodoc.CloudRegion{{
				Cloud:              params.Cloud(cloud),
				PrimaryControllers: []params.EntityPath{ctlPath},
				ACL: params.ACL{
					Read: []string{"everyone"},
				},
			}})
			c.Assert(err, gc.Equals, nil)
		}
		cloud, err := client.DefaultCloud()
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

func (s *controllerSuite) TestCloudCall(c *gc.C) {
	s.AssertAddController(c, params.EntityPath{User: "test", Name: "controller-1"}, true)
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

func (s *controllerSuite) TestClouds(c *gc.C) {
	s.AssertAddController(c, params.EntityPath{User: "test-group", Name: "controller-1"}, true)
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

func (s *controllerSuite) TestUserCredentials(c *gc.C) {
	s.AssertAddController(c, params.EntityPath{User: "test", Name: "controller-1"}, true)
	s.JEM.UpdateCredential(context.Background(), &mongodoc.Credential{
		Path: params.CredentialPath{
			Cloud:      "dummy",
			EntityPath: params.EntityPath{User: "test", Name: "cred1"},
		},
		Type:  "credtype",
		Label: "Credentials 1",
		Attributes: map[string]string{
			"attr1": "val1",
			"attr2": "val2",
		},
	})
	conn := s.open(c, nil, "test")
	defer conn.Close()
	client := cloudapi.NewClient(conn)
	creds, err := client.UserCredentials(names.NewUserTag("test@external"), names.NewCloudTag("dummy"))
	c.Assert(err, gc.Equals, nil)
	c.Assert(creds, jc.DeepEquals, []names.CloudCredentialTag{
		names.NewCloudCredentialTag("dummy/test@external/cred1"),
	})
}

func (s *controllerSuite) TestUserCredentialsWithDomain(c *gc.C) {
	s.AssertAddController(c, params.EntityPath{User: "test", Name: "controller-1"}, true)
	s.JEM.UpdateCredential(context.Background(), &mongodoc.Credential{
		Path: params.CredentialPath{
			Cloud:      "dummy",
			EntityPath: params.EntityPath{User: "test@domain", Name: "cred1"},
		},
		Type:  "credtype",
		Label: "Credentials 1",
		Attributes: map[string]string{
			"attr1": "val1",
			"attr2": "val2",
		},
	})
	conn := s.open(c, nil, "test@domain")
	defer conn.Close()
	client := cloudapi.NewClient(conn)
	creds, err := client.UserCredentials(names.NewUserTag("test@domain"), names.NewCloudTag("dummy"))
	c.Assert(err, gc.Equals, nil)
	c.Assert(creds, jc.DeepEquals, []names.CloudCredentialTag{
		names.NewCloudCredentialTag("dummy/test@domain/cred1"),
	})
}

func (s *controllerSuite) TestUserCredentialsACL(c *gc.C) {
	s.AssertAddController(c, params.EntityPath{User: "test", Name: "controller-1"}, true)
	s.JEM.UpdateCredential(context.Background(), &mongodoc.Credential{
		Path: params.CredentialPath{
			Cloud:      "dummy",
			EntityPath: params.EntityPath{User: "test", Name: "cred1"},
		},
		Type:  "credtype",
		Label: "Credentials 1",
		Attributes: map[string]string{
			"attr1": "val1",
			"attr2": "val2",
		},
	})
	s.JEM.UpdateCredential(context.Background(), &mongodoc.Credential{
		Path: params.CredentialPath{
			Cloud:      "dummy",
			EntityPath: params.EntityPath{User: "test2", Name: "cred2"},
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
	})
	conn := s.open(c, nil, "test")
	defer conn.Close()
	client := cloudapi.NewClient(conn)
	creds, err := client.UserCredentials(names.NewUserTag("test2@external"), names.NewCloudTag("dummy"))
	c.Assert(err, gc.Equals, nil)
	c.Assert(creds, jc.DeepEquals, []names.CloudCredentialTag{
		names.NewCloudCredentialTag("dummy/test2@external/cred2"),
	})
}

func (s *controllerSuite) TestUserCredentialsErrors(c *gc.C) {
	s.AssertAddController(c, params.EntityPath{User: "test", Name: "controller-1"}, true)
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

func (s *controllerSuite) TestUpdateCloudCredentials(c *gc.C) {
	s.AssertAddController(c, params.EntityPath{User: "test", Name: "controller-1"}, true)
	conn := s.open(c, nil, "test")
	defer conn.Close()
	client := cloudapi.NewClient(conn)
	credentialTag := names.NewCloudCredentialTag(fmt.Sprintf("dummy/test@external/cred3"))
	_, err := client.UpdateCredentialsCheckModels(credentialTag, cloud.NewCredential("credtype", map[string]string{"attr1": "val31", "attr2": "val32"}))
	c.Assert(err, gc.Equals, nil)
	creds, err := client.UserCredentials(names.NewUserTag("test@external"), names.NewCloudTag("dummy"))
	c.Assert(err, gc.Equals, nil)
	c.Assert(creds, jc.DeepEquals, []names.CloudCredentialTag{credentialTag})
	_, err = client.UpdateCredentialsCheckModels(credentialTag, cloud.NewCredential("credtype", map[string]string{"attr1": "val33", "attr2": "val34"}))
	c.Assert(err, gc.Equals, nil)
	creds, err = client.UserCredentials(names.NewUserTag("test@external"), names.NewCloudTag("dummy"))
	c.Assert(err, gc.Equals, nil)
	var _ = creds
}

func (s *controllerSuite) TestUpdateCloudCredentialsErrors(c *gc.C) {
	s.AssertAddController(c, params.EntityPath{User: "test", Name: "controller-1"}, true)
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

func (s *controllerSuite) TestCredential(c *gc.C) {
	s.AssertAddController(c, params.EntityPath{User: "test", Name: "controller-1"}, true)
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
	cred5 := cloud.NewCredential("empty", nil)

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
			Message: `credential "dummy/test/cred3" not found`,
			Code:    jujuparams.CodeNotFound,
		},
	}, {
		Error: &jujuparams.Error{
			Message: `unauthorized`,
			Code:    jujuparams.CodeUnauthorized,
		},
	}, {
		Error: &jujuparams.Error{
			Message: `cloud "no-such-cloud" not found`,
			Code:    jujuparams.CodeNotFound,
		},
	}, {
		Error: &jujuparams.Error{
			Message: `unsupported local user`,
			Code:    jujuparams.CodeBadRequest,
		},
	}})
}

func (s *controllerSuite) TestRevokeCredential(c *gc.C) {
	s.AssertAddController(c, params.EntityPath{User: "test", Name: "controller"}, true)
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

	err = client.RevokeCredential(credTag)
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

func (s *controllerSuite) TestAddCloud(c *gc.C) {
	s.AssertAddController(c, params.EntityPath{User: "test", Name: "controller"}, true)
	conn := s.open(c, nil, "test")
	defer conn.Close()
	client := cloudapi.NewClient(conn)
	err := client.AddCloud(cloud.Cloud{
		Name:             "test-cloud",
		Type:             "kubernetes",
		AuthTypes:        cloud.AuthTypes{cloud.CertificateAuthType},
		Endpoint:         "https://1.2.3.4:5678",
		IdentityEndpoint: "https://1.2.3.4:5679",
		StorageEndpoint:  "https://1.2.3.4:5680",
	})
	c.Assert(err, gc.Equals, nil)
	clouds, err := client.Clouds()
	c.Assert(err, gc.Equals, nil)
	c.Assert(clouds[names.NewCloudTag("test-cloud")], jc.DeepEquals, cloud.Cloud{
		Name:             "test-cloud",
		Type:             "kubernetes",
		AuthTypes:        cloud.AuthTypes{"certificate"},
		Endpoint:         "https://1.2.3.4:5678",
		IdentityEndpoint: "https://1.2.3.4:5679",
		StorageEndpoint:  "https://1.2.3.4:5680",
	})
}

func (s *controllerSuite) TestAddCloudError(c *gc.C) {
	s.AssertAddController(c, params.EntityPath{User: "test", Name: "controller"}, true)
	conn := s.open(c, nil, "test")
	defer conn.Close()
	client := cloudapi.NewClient(conn)
	err := client.AddCloud(cloud.Cloud{
		Name:             "test-cloud",
		Type:             "kubernetes",
		Endpoint:         "https://1.2.3.4:5678",
		IdentityEndpoint: "https://1.2.3.4:5679",
		StorageEndpoint:  "https://1.2.3.4:5680",
	})
	c.Assert(err, gc.ErrorMatches, `invalid cloud: empty auth-types not valid`)
}

func (s *controllerSuite) TestAddCredential(c *gc.C) {
	s.AssertAddController(c, params.EntityPath{User: "test", Name: "controller-1"}, true)
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

func (s *controllerSuite) TestCredentialContents(c *gc.C) {
	s.AssertAddController(c, params.EntityPath{User: "test", Name: "controller-1"}, true)
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
}

func (s *controllerSuite) TestRemoveCloud(c *gc.C) {
	s.AssertAddController(c, params.EntityPath{User: "test", Name: "controller"}, true)
	conn := s.open(c, nil, "test")
	defer conn.Close()
	client := cloudapi.NewClient(conn)
	err := client.AddCloud(cloud.Cloud{
		Name:             "test-cloud",
		Type:             "kubernetes",
		AuthTypes:        cloud.AuthTypes{cloud.CertificateAuthType},
		Endpoint:         "https://1.2.3.4:5678",
		IdentityEndpoint: "https://1.2.3.4:5679",
		StorageEndpoint:  "https://1.2.3.4:5680",
	})
	c.Assert(err, gc.Equals, nil)
	clouds, err := client.Clouds()
	c.Assert(err, gc.Equals, nil)
	c.Assert(clouds[names.NewCloudTag("test-cloud")], jc.DeepEquals, cloud.Cloud{
		Name:             "test-cloud",
		Type:             "kubernetes",
		AuthTypes:        cloud.AuthTypes{"certificate"},
		Endpoint:         "https://1.2.3.4:5678",
		IdentityEndpoint: "https://1.2.3.4:5679",
		StorageEndpoint:  "https://1.2.3.4:5680",
	})

	err = client.RemoveCloud("test-cloud")
	c.Assert(err, gc.Equals, nil)
	clouds, err = client.Clouds()
	c.Assert(err, gc.Equals, nil)
	c.Assert(clouds[names.NewCloudTag("test-cloud")], jc.DeepEquals, cloud.Cloud{})
}

func (s *controllerSuite) TestRemoveCloudNotFound(c *gc.C) {
	s.AssertAddController(c, params.EntityPath{User: "test", Name: "controller"}, true)
	conn := s.open(c, nil, "test")
	defer conn.Close()
	client := cloudapi.NewClient(conn)

	err := client.RemoveCloud("test-cloud")
	c.Assert(err, gc.ErrorMatches, `cloud "test-cloud" region "" not found`)
}

func (s *controllerSuite) TestJIMMFacadeVersion(c *gc.C) {
	conn := s.open(c, nil, "test")
	defer conn.Close()
	c.Assert(conn.AllFacadeVersions()["JIMM"], jc.DeepEquals, []int{1})
}

func (s *controllerSuite) TestUserModelStats(c *gc.C) {
	ctlPath := s.AssertAddController(c, params.EntityPath{User: "test", Name: "controller-1"}, true)
	cred := s.AssertUpdateCredential(c, "test", "dummy", "cred1", "empty")
	cred2 := s.AssertUpdateCredential(c, "test2", "dummy", "cred1", "empty")
	err := s.JEM.DB.SetACL(testContext, s.JEM.DB.Controllers(), ctlPath, params.ACL{
		Read: []string{"test2"},
	})

	c.Assert(err, gc.Equals, nil)

	model1 := s.assertCreateModel(c, createModelParams{name: "model-1", username: "test", cred: cred})
	model2 := s.assertCreateModel(c, createModelParams{name: "model-2", username: "test2", cred: cred2})
	model3 := s.assertCreateModel(c, createModelParams{name: "model-3", username: "test2", cred: cred2})

	// Update some stats for the models we've just created'
	t0 := time.Unix(0, 0)

	err = s.JEM.DB.UpdateModelCounts(testContext, ctlPath, model1.UUID, map[params.EntityCount]int{
		params.UnitCount: 99,
	}, t0)

	c.Assert(err, gc.Equals, nil)
	err = s.JEM.DB.UpdateModelCounts(testContext, ctlPath, model2.UUID, map[params.EntityCount]int{
		params.MachineCount: 10,
	}, t0)

	c.Assert(err, gc.Equals, nil)
	err = s.JEM.DB.UpdateModelCounts(testContext, ctlPath, model3.UUID, map[params.EntityCount]int{
		params.ApplicationCount: 1,
	}, t0)

	c.Assert(err, gc.Equals, nil)

	// Allow test2/model-3 access to everyone, so that we can be sure we're
	// not seeing models that we have access to but aren't the creator of.
	err = s.JEM.DB.SetACL(testContext, s.JEM.DB.Models(), params.EntityPath{User: "test2", Name: "model-3"}, params.ACL{
		Read: []string{"test"},
	})

	// Open the API connection as user "test". We should only see the one model.
	conn := s.open(c, nil, "test")
	defer conn.Close()
	var resp params.UserModelStatsResponse
	err = conn.APICall("JIMM", 1, "", "UserModelStats", nil, &resp)
	c.Assert(err, gc.Equals, nil)

	c.Assert(resp, jc.DeepEquals, params.UserModelStatsResponse{
		Models: map[string]params.ModelStats{
			model1.UUID: {
				Model: jujuparams.Model{
					Name:     "model-1",
					UUID:     model1.UUID,
					OwnerTag: names.NewUserTag(model1.Owner).String(),
				},
				Counts: map[params.EntityCount]params.Count{
					params.UnitCount: {
						Time:    t0,
						Current: 99,
						Max:     99,
						Total:   99,
					},
				},
			},
		},
	})

	// As test2, we should see the other two models.
	conn = s.open(c, nil, "test2")
	defer conn.Close()
	resp = params.UserModelStatsResponse{}
	err = conn.APICall("JIMM", 1, "", "UserModelStats", nil, &resp)
	c.Assert(err, gc.Equals, nil)

	c.Assert(resp, jc.DeepEquals, params.UserModelStatsResponse{
		Models: map[string]params.ModelStats{
			model2.UUID: {
				Model: jujuparams.Model{
					Name:     "model-2",
					UUID:     model2.UUID,
					OwnerTag: names.NewUserTag(model2.Owner).String(),
				},
				Counts: map[params.EntityCount]params.Count{
					params.MachineCount: {
						Time:    t0,
						Current: 10,
						Max:     10,
						Total:   10,
					},
				},
			},
			model3.UUID: {
				Model: jujuparams.Model{
					Name:     "model-3",
					UUID:     model3.UUID,
					OwnerTag: names.NewUserTag(model3.Owner).String(),
				},
				Counts: map[params.EntityCount]params.Count{
					params.ApplicationCount: {
						Time:    t0,
						Current: 1,
						Max:     1,
						Total:   1,
					},
				},
			},
		},
	})
}

func (s *controllerSuite) TestControllerConfig(c *gc.C) {
	conn := s.open(c, nil, "test")
	defer conn.Close()
	client := controllerapi.NewClient(conn)
	conf, err := client.ControllerConfig()
	c.Assert(err, gc.Equals, nil)
	c.Assert(conf, jc.DeepEquals, controller.Config(map[string]interface{}{
		"charmstore-url": "https://api.jujucharms.com/charmstore",
		"metering-url":   "https://api.jujucharms.com/omnibus",
	}))
}

func (s *controllerSuite) TestAllModels(c *gc.C) {
	ctlPath := s.AssertAddController(c, params.EntityPath{User: "test", Name: "controller-1"}, true)
	s.AssertUpdateCredential(c, "test", "dummy", "cred1", "empty")
	s.AssertUpdateCredential(c, "test2", "dummy", "cred1", "empty")
	err := s.JEM.DB.SetACL(testContext, s.JEM.DB.Controllers(), ctlPath, params.ACL{
		Read: []string{"test2"},
	})

	c.Assert(err, gc.Equals, nil)

	mi := s.assertCreateModel(c, createModelParams{name: "model-1", username: "test", cred: "cred1"})
	modelUUID1 := mi.UUID
	s.assertCreateModel(c, createModelParams{name: "model-2", username: "test2", cred: "cred1"})
	mi = s.assertCreateModel(c, createModelParams{name: "model-3", username: "test2", cred: "cred1"})
	modelUUID3 := mi.UUID

	conn := s.open(c, nil, "test")
	defer conn.Close()
	client := controllerapi.NewClient(conn)

	err = s.JEM.DB.SetACL(testContext, s.JEM.DB.Models(), params.EntityPath{User: "test2", Name: "model-3"}, params.ACL{
		Read: []string{"test"},
	})

	c.Assert(err, gc.Equals, nil)

	models, err := client.AllModels()
	c.Assert(err, gc.Equals, nil)
	c.Assert(models, jc.DeepEquals, []base.UserModel{{
		Name:           "model-1",
		UUID:           modelUUID1,
		Owner:          "test@external",
		LastConnection: nil,
		Type:           "iaas",
	}, {
		Name:           "model-3",
		UUID:           modelUUID3,
		Owner:          "test2@external",
		LastConnection: nil,
		Type:           "iaas",
	}})
}

func (s *controllerSuite) TestModelStatus(c *gc.C) {
	ctlPath := s.AssertAddController(c, params.EntityPath{User: "test", Name: "controller-1"}, true)
	s.AssertUpdateCredential(c, "test", "dummy", "cred1", "empty")
	s.AssertUpdateCredential(c, "test2", "dummy", "cred1", "empty")
	err := s.JEM.DB.SetACL(testContext, s.JEM.DB.Controllers(), ctlPath, params.ACL{
		Read: []string{"test2"},
	})

	c.Assert(err, gc.Equals, nil)

	mi := s.assertCreateModel(c, createModelParams{name: "model-1", username: "test", cred: "cred1"})
	modelUUID1 := mi.UUID
	mi = s.assertCreateModel(c, createModelParams{name: "model-2", username: "test2", cred: "cred1"})
	modelUUID2 := mi.UUID
	mi = s.assertCreateModel(c, createModelParams{name: "model-3", username: "test2", cred: "cred1"})
	modelUUID3 := mi.UUID

	err = s.JEM.DB.SetACL(testContext, s.JEM.DB.Models(), params.EntityPath{User: "test2", Name: "model-3"}, params.ACL{
		Read: []string{"test"},
	})

	c.Assert(err, gc.Equals, nil)

	type modelStatuser interface {
		ModelStatus(tags ...names.ModelTag) ([]base.ModelStatus, error)
	}
	doTest := func(client modelStatuser) {
		models, err := client.ModelStatus(names.NewModelTag(modelUUID1), names.NewModelTag(modelUUID3))
		c.Assert(err, gc.Equals, nil)
		c.Assert(models, jc.DeepEquals, []base.ModelStatus{{
			UUID:               modelUUID1,
			Life:               "alive",
			Owner:              "test@external",
			TotalMachineCount:  0,
			CoreCount:          0,
			HostedMachineCount: 0,
			ApplicationCount:   0,
			Machines:           []base.Machine{},
		}, {
			UUID:               modelUUID3,
			Life:               "alive",
			Owner:              "test2@external",
			TotalMachineCount:  0,
			CoreCount:          0,
			HostedMachineCount: 0,
			ApplicationCount:   0,
			Machines:           []base.Machine{},
		}})
		_, err = client.ModelStatus(names.NewModelTag(modelUUID2))
		c.Assert(err, gc.ErrorMatches, `unauthorized`)
	}

	conn := s.open(c, nil, "test")
	defer conn.Close()
	doTest(controllerapi.NewClient(conn))
	doTest(modelmanager.NewClient(conn))
}

type testHeartMonitor struct {
	c         chan time.Time
	firstBeat chan struct{}

	// mu protects the fields below.
	mu              sync.Mutex
	_beats          int
	dead            bool
	firstBeatClosed bool
}

func newTestHeartMonitor() *testHeartMonitor {
	return &testHeartMonitor{
		c:         make(chan time.Time),
		firstBeat: make(chan struct{}),
	}
}

func (m *testHeartMonitor) Heartbeat() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m._beats++
	if !m.firstBeatClosed {
		close(m.firstBeat)
		m.firstBeatClosed = true
	}

}

func (m *testHeartMonitor) Dead() <-chan time.Time {
	return m.c
}

func (m *testHeartMonitor) Stop() bool {
	return m.dead
}

func (m *testHeartMonitor) kill(t time.Time) {
	m.mu.Lock()
	m.dead = true
	m.mu.Unlock()
	m.c <- t
}

func (m *testHeartMonitor) beats() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m._beats
}

func (m *testHeartMonitor) waitForFirstPing(c *gc.C, d time.Duration) {
	select {
	case <-m.firstBeat:
	case <-time.After(d):
		c.Fatalf("timeout waiting for first ping")
	}
}

func (s *controllerSuite) TestConnectionClosesWhenHeartMonitorDies(c *gc.C) {
	hm := newTestHeartMonitor()
	s.PatchValue(jujuapi.NewHeartMonitor, jujuapi.InternalHeartMonitor(func(time.Duration) jujuapi.HeartMonitor {
		return hm
	}))
	conn := s.open(c, nil, "test")
	defer conn.Close()
	hm.kill(time.Now())
	beats := hm.beats()
	var err error
	for beats < 10 {
		time.Sleep(10 * time.Millisecond)
		err = conn.APICall("Pinger", 1, "", "Ping", nil, nil)
		if err != nil {
			break
		}
		beats++
	}
	c.Assert(err, gc.ErrorMatches, `connection is shut down`)
	c.Assert(hm.beats(), gc.Equals, beats)
}

func (s *controllerSuite) TestPingerUpdatesHeartMonitor(c *gc.C) {
	hm := newTestHeartMonitor()
	s.PatchValue(jujuapi.NewHeartMonitor, jujuapi.InternalHeartMonitor(func(time.Duration) jujuapi.HeartMonitor {
		return hm
	}))
	conn := s.open(c, nil, "test")
	defer conn.Close()
	beats := hm.beats()
	err := conn.APICall("Pinger", 1, "", "Ping", nil, nil)
	c.Assert(err, gc.Equals, nil)
	c.Assert(hm.beats(), gc.Equals, beats+1)
}

func (s *controllerSuite) TestUnauthenticatedPinger(c *gc.C) {
	hm := newTestHeartMonitor()
	s.PatchValue(jujuapi.NewHeartMonitor, jujuapi.InternalHeartMonitor(func(time.Duration) jujuapi.HeartMonitor {
		return hm
	}))
	conn := s.open(c, &api.Info{SkipLogin: true}, "test")
	defer conn.Close()
	err := conn.APICall("Pinger", 1, "", "Ping", nil, nil)
	c.Assert(err, gc.Equals, nil)
	c.Assert(hm.beats(), gc.Equals, 1)
}

func (s *controllerSuite) TestAddUser(c *gc.C) {
	conn := s.open(c, nil, "alice")
	defer conn.Close()

	client := usermanager.NewClient(conn)
	_, _, err := client.AddUser("bob", "Bob", "bob's super secret password")
	c.Assert(err, gc.ErrorMatches, `unauthorized \(unauthorized access\)`)
}

func (s *controllerSuite) TestRemoveUser(c *gc.C) {
	conn := s.open(c, nil, "alice")
	defer conn.Close()

	client := usermanager.NewClient(conn)
	err := client.RemoveUser("bob")
	c.Assert(err, gc.ErrorMatches, `unauthorized \(unauthorized access\)`)
}

func (s *controllerSuite) TestEnableUser(c *gc.C) {
	conn := s.open(c, nil, "alice")
	defer conn.Close()

	client := usermanager.NewClient(conn)
	err := client.EnableUser("bob")
	c.Assert(err, gc.ErrorMatches, `unauthorized \(unauthorized access\)`)
}

func (s *controllerSuite) TestDisableUser(c *gc.C) {
	conn := s.open(c, nil, "alice")
	defer conn.Close()

	client := usermanager.NewClient(conn)
	err := client.DisableUser("bob")
	c.Assert(err, gc.ErrorMatches, `unauthorized \(unauthorized access\)`)
}

func (s *controllerSuite) TestUserInfoAllUsers(c *gc.C) {
	conn := s.open(c, nil, "alice")
	defer conn.Close()

	client := usermanager.NewClient(conn)
	users, err := client.UserInfo(nil, usermanager.AllUsers)
	c.Assert(err, gc.Equals, nil)
	c.Assert(len(users), gc.Equals, 0)
}

func (s *controllerSuite) TestUserInfoSpecifiedUser(c *gc.C) {
	conn := s.open(c, nil, "alice")
	defer conn.Close()

	client := usermanager.NewClient(conn)
	users, err := client.UserInfo([]string{"alice@external"}, usermanager.AllUsers)
	c.Assert(err, gc.Equals, nil)
	c.Assert(len(users), gc.Equals, 1)
	c.Assert(users[0], jc.DeepEquals, jujuparams.UserInfo{
		Username:    "alice@external",
		DisplayName: "alice@external",
		Access:      "add-model",
	})
}

func (s *controllerSuite) TestUserInfoSpecifiedUsers(c *gc.C) {
	conn := s.open(c, nil, "alice")
	defer conn.Close()

	client := usermanager.NewClient(conn)
	users, err := client.UserInfo([]string{"alice@external", "bob@external"}, usermanager.AllUsers)
	c.Assert(err, gc.ErrorMatches, "bob@external: unauthorized")
	c.Assert(users, gc.HasLen, 0)
}

func (s *controllerSuite) TestUserInfoWithDomain(c *gc.C) {
	conn := s.open(c, nil, "alice@mydomain")
	defer conn.Close()

	client := usermanager.NewClient(conn)
	users, err := client.UserInfo([]string{"alice@mydomain"}, usermanager.AllUsers)
	c.Assert(err, gc.Equals, nil)
	c.Assert(len(users), gc.Equals, 1)
	c.Assert(users[0], jc.DeepEquals, jujuparams.UserInfo{
		Username:    "alice@mydomain",
		DisplayName: "alice@mydomain",
		Access:      "add-model",
	})
}

func (s *controllerSuite) TestUserInfoInvalidUsername(c *gc.C) {
	conn := s.open(c, nil, "alice")
	defer conn.Close()

	client := usermanager.NewClient(conn)
	users, err := client.UserInfo([]string{"alice-@external"}, usermanager.AllUsers)
	c.Assert(err, gc.ErrorMatches, `"alice-@external" is not a valid username`)
	c.Assert(users, gc.HasLen, 0)
}

func (s *controllerSuite) TestUserInfoLocalUsername(c *gc.C) {
	conn := s.open(c, nil, "alice")
	defer conn.Close()

	client := usermanager.NewClient(conn)
	users, err := client.UserInfo([]string{"alice"}, usermanager.AllUsers)
	c.Assert(err, gc.ErrorMatches, `alice: unsupported local user`)
	c.Assert(users, gc.HasLen, 0)
}

func (s *controllerSuite) TestSetPassword(c *gc.C) {
	conn := s.open(c, nil, "alice")
	defer conn.Close()

	client := usermanager.NewClient(conn)
	err := client.SetPassword("bob", "bob's new super secret password")
	c.Assert(err, gc.ErrorMatches, `unauthorized \(unauthorized access\)`)
}

func assertModelInfo(c *gc.C, obtained, expected []jujuparams.ModelInfoResult) {
	for i := range obtained {
		// DefaultSeries changes between juju versions and
		// we don't care about its specific value.
		if obtained[i].Result != nil {
			obtained[i].Result.DefaultSeries = ""
		}
	}
	for i := range obtained {
		if obtained[i].Result == nil {
			continue
		}
		obtained[i].Result.Status.Since = nil
		for j := range obtained[i].Result.Users {
			obtained[i].Result.Users[j].LastConnection = nil
		}
	}
	c.Assert(obtained, jc.DeepEquals, expected)
}

func newBool(b bool) *bool {
	return &b
}
