// Copyright 2016 Canonical Ltd.

package jujuapi_test

import (
	"fmt"
	"sync"
	"time"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	cloudapi "github.com/juju/juju/api/cloud"
	"github.com/juju/juju/api/controller"
	"github.com/juju/juju/api/modelmanager"
	"github.com/juju/juju/api/usermanager"
	jujuparams "github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/multiwatcher"
	"github.com/juju/juju/status"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"golang.org/x/net/context"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"
	"gopkg.in/macaroon.v1"
	"gopkg.in/mgo.v2/bson"

	"github.com/CanonicalLtd/jem/internal/jujuapi"
	"github.com/CanonicalLtd/jem/internal/mongodoc"
	"github.com/CanonicalLtd/jem/params"
)

type controllerSuite struct {
	websocketSuite
}

var _ = gc.Suite(&controllerSuite{})

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
	c.Assert(err, jc.ErrorIsNil)
	var resp jujuparams.RedirectInfoResult
	err = conn.APICall("Admin", 3, "", "RedirectInfo", nil, &resp)
	c.Assert(err, gc.ErrorMatches, `no such request - method Admin.RedirectInfo is not implemented \(not implemented\)`)
}

func (s *controllerSuite) TestLoginToControllerWithInvalidMacaroon(c *gc.C) {
	s.AssertAddController(c, params.EntityPath{User: "test", Name: "controller-1"}, true)
	invalidMacaroon, err := macaroon.New(nil, "", "")
	c.Assert(err, gc.IsNil)
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

func (s *controllerSuite) TestDefaultCloud(c *gc.C) {
	s.AssertAddController(c, params.EntityPath{User: "test", Name: "controller-1"}, true)
	conn := s.open(c, nil, "test")
	defer conn.Close()
	client := cloudapi.NewClient(conn)
	_, err := client.DefaultCloud()
	c.Assert(err, gc.ErrorMatches, "no default cloud")
}

func (s *controllerSuite) TestCloudCall(c *gc.C) {
	s.AssertAddController(c, params.EntityPath{User: "test", Name: "controller-1"}, true)
	conn := s.open(c, nil, "test")
	defer conn.Close()
	client := cloudapi.NewClient(conn)
	info, err := client.Cloud(names.NewCloudTag("dummy"))
	c.Assert(err, jc.ErrorIsNil)
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
		Endpoint: "dummy-storage-endpoint",
	})
}

func (s *controllerSuite) TestClouds(c *gc.C) {
	s.AssertAddController(c, params.EntityPath{User: "test-group", Name: "controller-1"}, true)
	s.IDMSrv.AddUser("test", "test-group")
	conn := s.open(c, nil, "test")
	defer conn.Close()

	client := cloudapi.NewClient(conn)
	clouds, err := client.Clouds()
	c.Assert(err, jc.ErrorIsNil)
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
			Endpoint: "dummy-storage-endpoint",
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
	c.Assert(err, jc.ErrorIsNil)
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
	c.Assert(err, jc.ErrorIsNil)
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
	c.Assert(err, jc.ErrorIsNil)
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
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(resp.Results[0].Error, gc.ErrorMatches, `bad request: "not-a-user-tag" is not a valid tag`)
	c.Assert(resp.Results, gc.HasLen, 1)
}

func (s *controllerSuite) TestUpdateCloudCredentials(c *gc.C) {
	s.AssertAddController(c, params.EntityPath{User: "test", Name: "controller-1"}, true)
	conn := s.open(c, nil, "test")
	defer conn.Close()
	client := cloudapi.NewClient(conn)
	credentialTag := names.NewCloudCredentialTag(fmt.Sprintf("dummy/test@external/cred3"))
	err := client.UpdateCredential(credentialTag, cloud.NewCredential("credtype", map[string]string{"attr1": "val31", "attr2": "val32"}))
	c.Assert(err, jc.ErrorIsNil)
	creds, err := client.UserCredentials(names.NewUserTag("test@external"), names.NewCloudTag("dummy"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(creds, jc.DeepEquals, []names.CloudCredentialTag{credentialTag})
	err = client.UpdateCredential(credentialTag, cloud.NewCredential("credtype", map[string]string{"attr1": "val33", "attr2": "val34"}))
	c.Assert(err, jc.ErrorIsNil)
	creds, err = client.UserCredentials(names.NewUserTag("test@external"), names.NewCloudTag("dummy"))
	c.Assert(err, jc.ErrorIsNil)
	var _ = creds
}

func (s *controllerSuite) TestUpdateCloudCredentialsErrors(c *gc.C) {
	s.AssertAddController(c, params.EntityPath{User: "test", Name: "controller-1"}, true)
	conn := s.open(c, nil, "test")
	defer conn.Close()
	req := jujuparams.UpdateCloudCredentials{
		Credentials: []jujuparams.UpdateCloudCredential{{
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
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(resp.Results, gc.HasLen, 3)
	c.Assert(resp.Results[0].Error, gc.ErrorMatches, `bad request: "not-a-cloud-credentials-tag" is not a valid tag`)
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
	err := client.UpdateCredential(cred1Tag, cred1)
	c.Assert(err, jc.ErrorIsNil)
	err = client.UpdateCredential(cred2Tag, cred2)
	c.Assert(err, jc.ErrorIsNil)
	err = client.UpdateCredential(cred5Tag, cred5)
	c.Assert(err, jc.ErrorIsNil)

	creds, err := client.Credentials(
		cred1Tag,
		cred2Tag,
		names.NewCloudCredentialTag("dummy/test@external/cred3"),
		names.NewCloudCredentialTag("dummy/no-test@external/cred4"),
		cred5Tag,
		names.NewCloudCredentialTag("dummy/admin@local/cred6"),
	)
	c.Assert(err, jc.ErrorIsNil)
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
	err := client.UpdateCredential(
		credTag,
		cloud.NewCredential("empty", nil),
	)
	c.Assert(err, jc.ErrorIsNil)

	tags, err := client.UserCredentials(credTag.Owner(), credTag.Cloud())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(tags, jc.DeepEquals, []names.CloudCredentialTag{
		credTag,
	})

	ccr, err := client.Credentials(credTag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ccr, jc.DeepEquals, []jujuparams.CloudCredentialResult{{
		Result: &jujuparams.CloudCredential{
			AuthType: "empty",
		},
	}})

	err = client.RevokeCredential(credTag)
	c.Assert(err, jc.ErrorIsNil)

	ccr, err = client.Credentials(credTag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ccr, jc.DeepEquals, []jujuparams.CloudCredentialResult{{
		Error: &jujuparams.Error{
			Code:    jujuparams.CodeNotFound,
			Message: `credential "dummy/test@external/cred" not found`,
		},
	}})

	tags, err = client.UserCredentials(credTag.Owner(), credTag.Cloud())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(tags, jc.DeepEquals, []names.CloudCredentialTag{})
}

func (s *controllerSuite) TestLoginToRoot(c *gc.C) {
	conn := s.open(c, &api.Info{
		SkipLogin: true,
	}, "test")
	defer conn.Close()
	err := conn.Login(nil, "", "", nil)
	c.Assert(err, jc.ErrorIsNil)
	var resp jujuparams.RedirectInfoResult
	err = conn.APICall("Admin", 3, "", "RedirectInfo", nil, &resp)
	c.Assert(err, gc.ErrorMatches, `no such request - method Admin.RedirectInfo is not implemented \(not implemented\)`)
}

func (s *controllerSuite) TestListModels(c *gc.C) {
	ctlPath := s.AssertAddController(c, params.EntityPath{User: "test", Name: "controller-1"}, true)
	cred := s.AssertUpdateCredential(c, "test", "dummy", "cred1", "empty")
	cred2 := s.AssertUpdateCredential(c, "test2", "dummy", "cred1", "empty")
	err := s.JEM.DB.SetACL(testContext, s.JEM.DB.Controllers(), ctlPath, params.ACL{
		Read: []string{"test2"},
	})

	c.Assert(err, jc.ErrorIsNil)
	mi := s.assertCreateModel(c, createModelParams{name: "model-1", username: "test", cred: cred})
	modelUUID1 := mi.UUID
	s.assertCreateModel(c, createModelParams{name: "model-2", username: "test2", cred: cred2})
	mi = s.assertCreateModel(c, createModelParams{name: "model-3", username: "test2", cred: cred2})
	modelUUID3 := mi.UUID
	err = s.JEM.DB.SetACL(testContext, s.JEM.DB.Models(), params.EntityPath{User: "test2", Name: "model-3"}, params.ACL{
		Read: []string{"test"},
	})

	c.Assert(err, jc.ErrorIsNil)
	conn := s.open(c, nil, "test")
	defer conn.Close()
	client := modelmanager.NewClient(conn)
	models, err := client.ListModels("test")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(models, jc.DeepEquals, []base.UserModel{{
		Name:  "model-1",
		UUID:  modelUUID1,
		Owner: "test@external",
	}, {
		Name:  "model-3",
		UUID:  modelUUID3,
		Owner: "test2@external",
	}})
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

	c.Assert(err, jc.ErrorIsNil)

	model1 := s.assertCreateModel(c, createModelParams{name: "model-1", username: "test", cred: cred})
	model2 := s.assertCreateModel(c, createModelParams{name: "model-2", username: "test2", cred: cred2})
	model3 := s.assertCreateModel(c, createModelParams{name: "model-3", username: "test2", cred: cred2})

	// Update some stats for the models we've just created'
	t0 := time.Unix(0, 0)

	err = s.JEM.DB.UpdateModelCounts(testContext, model1.UUID, map[params.EntityCount]int{
		params.UnitCount: 99,
	}, t0)

	c.Assert(err, gc.IsNil)
	err = s.JEM.DB.UpdateModelCounts(testContext, model2.UUID, map[params.EntityCount]int{
		params.MachineCount: 10,
	}, t0)

	c.Assert(err, gc.IsNil)
	err = s.JEM.DB.UpdateModelCounts(testContext, model3.UUID, map[params.EntityCount]int{
		params.ApplicationCount: 1,
	}, t0)

	c.Assert(err, gc.IsNil)

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
	c.Assert(err, gc.IsNil)

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
	c.Assert(err, gc.IsNil)

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

func (s *controllerSuite) TestModelInfo(c *gc.C) {
	ctlPath := s.AssertAddController(c, params.EntityPath{User: "test", Name: "controller-1"}, true)
	s.AssertUpdateCredential(c, "test", "dummy", "cred1", "empty")
	s.AssertUpdateCredential(c, "test2", "dummy", "cred1", "empty")
	err := s.JEM.DB.SetACL(testContext, s.JEM.DB.Controllers(), ctlPath, params.ACL{
		Read: []string{"test2"},
	})

	c.Assert(err, jc.ErrorIsNil)

	mi := s.assertCreateModel(c, createModelParams{name: "model-1", username: "test", cred: "cred1"})
	modelUUID1 := mi.UUID
	mi = s.assertCreateModel(c, createModelParams{name: "model-2", username: "test2", cred: "cred1"})
	modelUUID2 := mi.UUID
	mi = s.assertCreateModel(c, createModelParams{name: "model-3", username: "test2", cred: "cred1"})
	modelUUID3 := mi.UUID
	mi = s.assertCreateModel(c, createModelParams{name: "model-4", username: "test2", cred: "cred1"})
	modelUUID4 := mi.UUID
	mi = s.assertCreateModel(c, createModelParams{name: "model-5", username: "test2", cred: "cred1"})
	modelUUID5 := mi.UUID

	s.grant(c, params.EntityPath{User: "test2", Name: "model-3"}, params.User("test"), "read")
	s.grant(c, params.EntityPath{User: "test2", Name: "model-4"}, params.User("test"), "write")
	s.grant(c, params.EntityPath{User: "test2", Name: "model-5"}, params.User("test"), "admin")

	// Add some machines to one of the models
	err = s.JEM.DB.UpdateMachineInfo(testContext, &multiwatcher.MachineInfo{
		ModelUUID: modelUUID3,
		Id:        "machine-0",
	})
	c.Assert(err, jc.ErrorIsNil)
	machineArch := "bbc-micro"
	err = s.JEM.DB.UpdateMachineInfo(testContext, &multiwatcher.MachineInfo{
		ModelUUID: modelUUID3,
		Id:        "machine-1",
		HardwareCharacteristics: &instance.HardwareCharacteristics{
			Arch: &machineArch,
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	err = s.JEM.DB.UpdateMachineInfo(testContext, &multiwatcher.MachineInfo{
		ModelUUID: modelUUID3,
		Id:        "machine-2",
		Life:      "dead",
	})
	c.Assert(err, jc.ErrorIsNil)

	conn := s.open(c, nil, "test")
	defer conn.Close()
	client := modelmanager.NewClient(conn)

	models, err := client.ModelInfo([]names.ModelTag{
		names.NewModelTag(modelUUID1),
		names.NewModelTag(modelUUID2),
		names.NewModelTag(modelUUID3),
		names.NewModelTag(modelUUID4),
		names.NewModelTag(modelUUID5),
		names.NewModelTag("00000000-0000-0000-0000-000000000007"),
	})
	c.Assert(err, jc.ErrorIsNil)

	assertModelInfo(c, models, []jujuparams.ModelInfoResult{{
		Result: &jujuparams.ModelInfo{
			Name:               "model-1",
			UUID:               modelUUID1,
			ControllerUUID:     "914487b5-60e7-42bb-bd63-1adc3fd3a388",
			ProviderType:       "dummy",
			DefaultSeries:      "xenial",
			CloudTag:           "cloud-dummy",
			CloudRegion:        "dummy-region",
			CloudCredentialTag: names.NewCloudCredentialTag("dummy/test@external/cred1").String(),
			OwnerTag:           names.NewUserTag("test@external").String(),
			Life:               jujuparams.Alive,
			Status: jujuparams.EntityStatus{
				Status: status.Available,
			},
			Users: []jujuparams.ModelUserInfo{{
				UserName:    "test@external",
				DisplayName: "test",
				Access:      jujuparams.ModelAdminAccess,
			}},
		},
	}, {
		Error: &jujuparams.Error{
			Message: "unauthorized",
			Code:    jujuparams.CodeUnauthorized,
		},
	}, {
		Result: &jujuparams.ModelInfo{
			Name:               "model-3",
			UUID:               modelUUID3,
			ControllerUUID:     "914487b5-60e7-42bb-bd63-1adc3fd3a388",
			ProviderType:       "dummy",
			DefaultSeries:      "xenial",
			CloudTag:           "cloud-dummy",
			CloudRegion:        "dummy-region",
			CloudCredentialTag: names.NewCloudCredentialTag("dummy/test2@external/cred1").String(),
			OwnerTag:           names.NewUserTag("test2@external").String(),
			Life:               jujuparams.Alive,
			Status: jujuparams.EntityStatus{
				Status: status.Available,
			},
			Users: []jujuparams.ModelUserInfo{{
				UserName: "test@external",
				Access:   jujuparams.ModelReadAccess,
			}},
			Machines: []jujuparams.ModelMachineInfo{{
				Id: "machine-0",
			}, {
				Id: "machine-1",
				Hardware: &jujuparams.MachineHardware{
					Arch: &machineArch,
				},
			}},
		},
	}, {
		Result: &jujuparams.ModelInfo{
			Name:               "model-4",
			UUID:               modelUUID4,
			ControllerUUID:     "914487b5-60e7-42bb-bd63-1adc3fd3a388",
			ProviderType:       "dummy",
			DefaultSeries:      "xenial",
			CloudTag:           "cloud-dummy",
			CloudRegion:        "dummy-region",
			CloudCredentialTag: names.NewCloudCredentialTag("dummy/test2@external/cred1").String(),
			OwnerTag:           names.NewUserTag("test2@external").String(),
			Life:               jujuparams.Alive,
			Status: jujuparams.EntityStatus{
				Status: status.Available,
			},
			Users: []jujuparams.ModelUserInfo{{
				UserName: "test@external",
				Access:   jujuparams.ModelWriteAccess,
			}},
		},
	}, {
		Result: &jujuparams.ModelInfo{
			Name:               "model-5",
			UUID:               modelUUID5,
			ControllerUUID:     "914487b5-60e7-42bb-bd63-1adc3fd3a388",
			ProviderType:       "dummy",
			DefaultSeries:      "xenial",
			CloudTag:           "cloud-dummy",
			CloudRegion:        "dummy-region",
			CloudCredentialTag: names.NewCloudCredentialTag("dummy/test2@external/cred1").String(),
			OwnerTag:           names.NewUserTag("test2@external").String(),
			Life:               jujuparams.Alive,
			Status: jujuparams.EntityStatus{
				Status: status.Available,
			},
			Users: []jujuparams.ModelUserInfo{{
				UserName:    "test2@external",
				DisplayName: "test2",
				Access:      jujuparams.ModelAdminAccess,
			}, {
				UserName: "test@external",
				Access:   jujuparams.ModelAdminAccess,
			}},
		},
	}, {
		Error: &jujuparams.Error{
			Message: `model "00000000-0000-0000-0000-000000000007" not found`,
			Code:    jujuparams.CodeNotFound,
		},
	}})
}

func (s *controllerSuite) TestModelInfoForLegacyModel(c *gc.C) {
	ctx := context.Background()
	s.AssertAddController(c, params.EntityPath{User: "test", Name: "controller-1"}, true)
	s.AssertUpdateCredential(c, "test", "dummy", "cred1", "empty")
	mi := s.assertCreateModel(c, createModelParams{name: "model-1", username: "test", cred: "cred1"})
	modelUUID1 := mi.UUID

	err := s.JEM.DB.Models().UpdateId("test/model-1", bson.D{{
		"$unset",
		bson.D{{
			"cloud", 1,
		}, {
			"cloudregion", 1,
		}, {
			"credential", 1,
		}, {
			"defaultseries", 1,
		}},
	}})
	c.Assert(err, jc.ErrorIsNil)

	// Sanity check the required fields aren't present.
	model, err := s.JEM.DB.Model(ctx, params.EntityPath{"test", "model-1"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(model.Cloud, gc.Equals, params.Cloud(""))

	conn := s.open(c, nil, "test")
	defer conn.Close()
	client := modelmanager.NewClient(conn)
	models, err := client.ModelInfo([]names.ModelTag{names.NewModelTag(modelUUID1)})
	c.Assert(err, jc.ErrorIsNil)
	assertModelInfo(c, models, []jujuparams.ModelInfoResult{{
		Result: &jujuparams.ModelInfo{
			Name:               "model-1",
			UUID:               modelUUID1,
			ControllerUUID:     "914487b5-60e7-42bb-bd63-1adc3fd3a388",
			ProviderType:       "dummy",
			DefaultSeries:      "xenial",
			CloudTag:           "cloud-dummy",
			CloudRegion:        "dummy-region",
			CloudCredentialTag: names.NewCloudCredentialTag("dummy/test@external/cred1").String(),
			OwnerTag:           names.NewUserTag("test@external").String(),
			Life:               jujuparams.Alive,
			Status: jujuparams.EntityStatus{
				Status: status.Available,
			},
			Users: []jujuparams.ModelUserInfo{{
				UserName:    "test@external",
				DisplayName: "test",
				Access:      jujuparams.ModelAdminAccess,
			}},
		},
	}})

	// Ensure the values in the database have been updated.
	model, err = s.JEM.DB.Model(ctx, params.EntityPath{"test", "model-1"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(model.Cloud, gc.Equals, params.Cloud("dummy"))
	c.Assert(model.CloudRegion, gc.Equals, "dummy-region")
	c.Assert(model.Credential.String(), gc.Equals, "dummy/test/cred1")
	c.Assert(model.DefaultSeries, gc.Equals, "xenial")
}

func (s *controllerSuite) TestModelInfoRequestTimeout(c *gc.C) {
	info := s.APIInfo(c)
	proxy := testing.NewTCPProxy(c, info.Addrs[0])
	p := &params.AddController{
		EntityPath: params.EntityPath{User: "test", Name: "controller-1"},
		Info: params.ControllerInfo{
			HostPorts:      []string{proxy.Addr()},
			CACert:         info.CACert,
			User:           info.Tag.Id(),
			Password:       info.Password,
			ControllerUUID: s.ControllerConfig.ControllerUUID(),
			Public:         true,
		},
	}
	s.IDMSrv.AddUser("test", "controller-admin")
	err := s.NewClient("test").AddController(p)
	c.Assert(err, jc.ErrorIsNil)
	s.AssertUpdateCredential(c, "test", "dummy", "cred1", "empty")

	mi := s.assertCreateModel(c, createModelParams{name: "model-1", username: "test", cred: "cred1"})

	conn := s.open(c, nil, "test")
	defer conn.Close()
	client := modelmanager.NewClient(conn)

	models, err := client.ModelInfo([]names.ModelTag{
		names.NewModelTag(mi.UUID),
	})
	c.Assert(err, jc.ErrorIsNil)

	assertModelInfo(c, models, []jujuparams.ModelInfoResult{{
		Result: &jujuparams.ModelInfo{
			Name:               "model-1",
			UUID:               mi.UUID,
			ControllerUUID:     "914487b5-60e7-42bb-bd63-1adc3fd3a388",
			ProviderType:       "dummy",
			DefaultSeries:      "xenial",
			CloudTag:           "cloud-dummy",
			CloudRegion:        "dummy-region",
			CloudCredentialTag: names.NewCloudCredentialTag("dummy/test@external/cred1").String(),
			OwnerTag:           names.NewUserTag("test@external").String(),
			Life:               jujuparams.Alive,
			Status: jujuparams.EntityStatus{
				Status: status.Available,
			},
			Users: []jujuparams.ModelUserInfo{{
				UserName:    "test@external",
				DisplayName: "test",
				Access:      jujuparams.ModelAdminAccess,
			}},
		},
	}})

	proxy.PauseConns()
	models, err = client.ModelInfo([]names.ModelTag{
		names.NewModelTag(mi.UUID),
	})
	c.Assert(err, jc.ErrorIsNil)

	assertModelInfo(c, models, []jujuparams.ModelInfoResult{{
		Result: &jujuparams.ModelInfo{
			Name:               "model-1",
			UUID:               mi.UUID,
			ControllerUUID:     "914487b5-60e7-42bb-bd63-1adc3fd3a388",
			ProviderType:       "dummy",
			DefaultSeries:      "xenial",
			CloudTag:           "cloud-dummy",
			CloudRegion:        "dummy-region",
			CloudCredentialTag: names.NewCloudCredentialTag("dummy/test@external/cred1").String(),
			OwnerTag:           names.NewUserTag("test@external").String(),
			Life:               jujuparams.Alive,
			Status: jujuparams.EntityStatus{
				Status: status.Available,
			},
		},
	}})

	proxy.ResumeConns()

	models, err = client.ModelInfo([]names.ModelTag{
		names.NewModelTag(mi.UUID),
	})
	c.Assert(err, jc.ErrorIsNil)

	assertModelInfo(c, models, []jujuparams.ModelInfoResult{{
		Result: &jujuparams.ModelInfo{
			Name:               "model-1",
			UUID:               mi.UUID,
			ControllerUUID:     "914487b5-60e7-42bb-bd63-1adc3fd3a388",
			ProviderType:       "dummy",
			DefaultSeries:      "xenial",
			CloudTag:           "cloud-dummy",
			CloudRegion:        "dummy-region",
			CloudCredentialTag: names.NewCloudCredentialTag("dummy/test@external/cred1").String(),
			OwnerTag:           names.NewUserTag("test@external").String(),
			Life:               jujuparams.Alive,
			Status: jujuparams.EntityStatus{
				Status: status.Available,
			},
			Users: []jujuparams.ModelUserInfo{{
				UserName:    "test@external",
				DisplayName: "test",
				Access:      jujuparams.ModelAdminAccess,
			}},
		},
	}})
}

func (s *controllerSuite) TestAllModels(c *gc.C) {
	ctlPath := s.AssertAddController(c, params.EntityPath{User: "test", Name: "controller-1"}, true)
	s.AssertUpdateCredential(c, "test", "dummy", "cred1", "empty")
	s.AssertUpdateCredential(c, "test2", "dummy", "cred1", "empty")
	err := s.JEM.DB.SetACL(testContext, s.JEM.DB.Controllers(), ctlPath, params.ACL{
		Read: []string{"test2"},
	})

	c.Assert(err, jc.ErrorIsNil)

	mi := s.assertCreateModel(c, createModelParams{name: "model-1", username: "test", cred: "cred1"})
	modelUUID1 := mi.UUID
	s.assertCreateModel(c, createModelParams{name: "model-2", username: "test2", cred: "cred1"})
	mi = s.assertCreateModel(c, createModelParams{name: "model-3", username: "test2", cred: "cred1"})
	modelUUID3 := mi.UUID

	conn := s.open(c, nil, "test")
	defer conn.Close()
	client := controller.NewClient(conn)

	err = s.JEM.DB.SetACL(testContext, s.JEM.DB.Models(), params.EntityPath{User: "test2", Name: "model-3"}, params.ACL{
		Read: []string{"test"},
	})

	c.Assert(err, jc.ErrorIsNil)

	models, err := client.AllModels()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(models, jc.DeepEquals, []base.UserModel{{
		Name:           "model-1",
		UUID:           modelUUID1,
		Owner:          "test@external",
		LastConnection: nil,
	}, {
		Name:           "model-3",
		UUID:           modelUUID3,
		Owner:          "test2@external",
		LastConnection: nil,
	}})
}

func (s *controllerSuite) TestModelStatus(c *gc.C) {
	ctlPath := s.AssertAddController(c, params.EntityPath{User: "test", Name: "controller-1"}, true)
	s.AssertUpdateCredential(c, "test", "dummy", "cred1", "empty")
	s.AssertUpdateCredential(c, "test2", "dummy", "cred1", "empty")
	err := s.JEM.DB.SetACL(testContext, s.JEM.DB.Controllers(), ctlPath, params.ACL{
		Read: []string{"test2"},
	})

	c.Assert(err, jc.ErrorIsNil)

	mi := s.assertCreateModel(c, createModelParams{name: "model-1", username: "test", cred: "cred1"})
	modelUUID1 := mi.UUID
	mi = s.assertCreateModel(c, createModelParams{name: "model-2", username: "test2", cred: "cred1"})
	modelUUID2 := mi.UUID
	mi = s.assertCreateModel(c, createModelParams{name: "model-3", username: "test2", cred: "cred1"})
	modelUUID3 := mi.UUID

	err = s.JEM.DB.SetACL(testContext, s.JEM.DB.Models(), params.EntityPath{User: "test2", Name: "model-3"}, params.ACL{
		Read: []string{"test"},
	})

	c.Assert(err, jc.ErrorIsNil)

	type modelStatuser interface {
		ModelStatus(tags ...names.ModelTag) ([]base.ModelStatus, error)
	}
	doTest := func(client modelStatuser) {
		models, err := client.ModelStatus(names.NewModelTag(modelUUID1), names.NewModelTag(modelUUID3))
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(models, jc.DeepEquals, []base.ModelStatus{{
			UUID:               modelUUID1,
			Life:               "alive",
			Owner:              "test@external",
			TotalMachineCount:  0,
			CoreCount:          0,
			HostedMachineCount: 0,
			ServiceCount:       0,
			Machines:           []base.Machine{},
		}, {
			UUID:               modelUUID3,
			Life:               "alive",
			Owner:              "test2@external",
			TotalMachineCount:  0,
			CoreCount:          0,
			HostedMachineCount: 0,
			ServiceCount:       0,
			Machines:           []base.Machine{},
		}})
		_, err = client.ModelStatus(names.NewModelTag(modelUUID2))
		c.Assert(err, gc.ErrorMatches, `unauthorized`)
	}

	conn := s.open(c, nil, "test")
	defer conn.Close()
	doTest(controller.NewClient(conn))
	doTest(modelmanager.NewClient(conn))
}

func (s *controllerSuite) TestModelStatusNotFound(c *gc.C) {
	conn := s.open(c, nil, "test")
	defer conn.Close()
	cclient := controller.NewClient(conn)
	mmclient := modelmanager.NewClient(conn)
	_, err := cclient.ModelStatus(names.NewModelTag("11111111-1111-1111-1111-111111111111"))
	c.Assert(jujuparams.ErrCode(err), gc.Equals, jujuparams.CodeNotFound)
	_, err = mmclient.ModelStatus(names.NewModelTag("11111111-1111-1111-1111-111111111111"))
	c.Assert(jujuparams.ErrCode(err), gc.Equals, jujuparams.CodeNotFound)
}

var createModelTests = []struct {
	about         string
	name          string
	ownerTag      string
	region        string
	cloudTag      string
	credentialTag string
	config        map[string]interface{}
	expectError   string
}{{
	about:         "success",
	name:          "model",
	ownerTag:      "user-test@external",
	cloudTag:      names.NewCloudTag("dummy").String(),
	credentialTag: "cloudcred-dummy_test@external_cred1",
}, {
	about:         "unauthorized user",
	name:          "model-2",
	ownerTag:      "user-not-test@external",
	cloudTag:      names.NewCloudTag("dummy").String(),
	credentialTag: "cloudcred-dummy_test@external_cred1",
	expectError:   `unauthorized \(unauthorized access\)`,
}, {
	about:         "existing model name",
	name:          "existing-model",
	ownerTag:      "user-test@external",
	cloudTag:      names.NewCloudTag("dummy").String(),
	credentialTag: "cloudcred-dummy_test@external_cred1",
	expectError:   "already exists",
}, {
	about:         "no controller",
	name:          "model-3",
	ownerTag:      "user-test@external",
	region:        "no-such-region",
	cloudTag:      names.NewCloudTag("dummy").String(),
	credentialTag: "cloudcred-dummy_test@external_cred1",
	expectError:   `cannot select controller: no matching controllers found \(not found\)`,
}, {
	about:         "local user",
	name:          "model-4",
	ownerTag:      "user-test@local",
	cloudTag:      names.NewCloudTag("dummy").String(),
	credentialTag: "cloudcred-dummy_test@external_cred1",
	expectError:   `unsupported local user \(bad request\)`,
}, {
	about:         "invalid user",
	name:          "model-5",
	ownerTag:      "user-test/test@external",
	cloudTag:      names.NewCloudTag("dummy").String(),
	credentialTag: "cloudcred-dummy_test@external_cred1",
	expectError:   `invalid owner tag: "user-test/test@external" is not a valid user tag \(bad request\)`,
}, {
	about:         "specific cloud",
	name:          "model-6",
	ownerTag:      "user-test@external",
	cloudTag:      names.NewCloudTag("dummy").String(),
	credentialTag: "cloudcred-dummy_test@external_cred1",
}, {
	about:         "specific cloud and region",
	name:          "model-7",
	ownerTag:      "user-test@external",
	cloudTag:      names.NewCloudTag("dummy").String(),
	region:        "dummy-region",
	credentialTag: "cloudcred-dummy_test@external_cred1",
}, {
	about:         "bad cloud tag",
	name:          "model-8",
	ownerTag:      "user-test@external",
	cloudTag:      "not-a-cloud-tag",
	credentialTag: "cloudcred-dummy_test@external_cred1",
	expectError:   `invalid cloud tag: "not-a-cloud-tag" is not a valid tag \(bad request\)`,
}, {
	about:         "no cloud tag",
	name:          "model-8",
	ownerTag:      "user-test@external",
	cloudTag:      "",
	credentialTag: "cloudcred-dummy_test@external_cred1",
	expectError:   `no cloud specified for model; please specify one`,
}, {
	about:         "no credential tag selects unambigous creds",
	name:          "model-8",
	ownerTag:      "user-test@external",
	cloudTag:      names.NewCloudTag("dummy").String(),
	region:        "dummy-region",
	credentialTag: "cloudcred-dummy_test@external_cred1",
}}

func (s *controllerSuite) TestCreateModel(c *gc.C) {
	s.AssertAddController(c, params.EntityPath{User: "test", Name: "controller-1"}, true)
	s.AssertUpdateCredential(c, "test", "dummy", "cred1", "empty")
	s.assertCreateModel(c, createModelParams{name: "existing-model", username: "test", cred: "cred1"})

	conn := s.open(c, nil, "test")
	defer conn.Close()

	for i, test := range createModelTests {
		c.Logf("test %d. %s", i, test.about)
		var mi jujuparams.ModelInfo
		err := conn.APICall("ModelManager", 2, "", "CreateModel", jujuparams.ModelCreateArgs{
			Name:               test.name,
			OwnerTag:           test.ownerTag,
			Config:             test.config,
			CloudTag:           test.cloudTag,
			CloudRegion:        test.region,
			CloudCredentialTag: test.credentialTag,
		}, &mi)
		if test.expectError != "" {
			c.Assert(err, gc.ErrorMatches, test.expectError)
			continue
		}
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(mi.Name, gc.Equals, test.name)
		c.Assert(mi.UUID, gc.Not(gc.Equals), "")
		c.Assert(mi.OwnerTag, gc.Equals, test.ownerTag)
		c.Assert(mi.ControllerUUID, gc.Equals, "914487b5-60e7-42bb-bd63-1adc3fd3a388")
		c.Assert(mi.Users, gc.Not(gc.HasLen), 0)
		if test.credentialTag == "" {
			c.Assert(mi.CloudCredentialTag, gc.Equals, "")
		} else {
			tag, err := names.ParseCloudCredentialTag(mi.CloudCredentialTag)
			c.Assert(err, gc.IsNil)
			c.Assert(tag.String(), gc.Equals, test.credentialTag)
		}
		if test.cloudTag == "" {
			c.Assert(mi.CloudTag, gc.Equals, "cloud-dummy")
		} else {
			ct, err := names.ParseCloudTag(test.cloudTag)
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(mi.CloudTag, gc.Equals, names.NewCloudTag(ct.Id()).String())
		}
	}
}

func (s *controllerSuite) TestGrantAndRevokeModel(c *gc.C) {
	s.AssertAddController(c, params.EntityPath{User: "test", Name: "controller-1"}, true)
	s.AssertUpdateCredential(c, "test", "dummy", "cred1", "empty")
	mi := s.assertCreateModel(c, createModelParams{name: "test-model", username: "test", cred: "cred1"})

	conn := s.open(c, nil, "test")
	defer conn.Close()
	client := modelmanager.NewClient(conn)

	conn2 := s.open(c, nil, "bob")
	defer conn2.Close()
	client2 := modelmanager.NewClient(conn2)

	res, err := client2.ModelInfo([]names.ModelTag{names.NewModelTag(mi.UUID)})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res, gc.HasLen, 1)
	c.Assert(res[0].Error, gc.ErrorMatches, "unauthorized")

	err = client.GrantModel("bob@external", "write", mi.UUID)
	c.Assert(err, jc.ErrorIsNil)

	res, err = client2.ModelInfo([]names.ModelTag{names.NewModelTag(mi.UUID)})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res, gc.HasLen, 1)
	c.Assert(res[0].Error, gc.IsNil)
	c.Assert(res[0].Result.UUID, gc.Equals, mi.UUID)

	err = client.RevokeModel("bob@external", "read", mi.UUID)
	c.Assert(err, jc.ErrorIsNil)

	res, err = client2.ModelInfo([]names.ModelTag{names.NewModelTag(mi.UUID)})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res, gc.HasLen, 1)
	c.Assert(res[0].Error, gc.Not(gc.IsNil))
	c.Assert(res[0].Error, gc.ErrorMatches, "unauthorized")
}

func (s *controllerSuite) TestModifyModelAccessErrors(c *gc.C) {
	s.AssertAddController(c, params.EntityPath{User: "alice", Name: "controller-1"}, true)
	s.AssertAddController(c, params.EntityPath{User: "bob", Name: "controller-1"}, true)
	s.AssertUpdateCredential(c, "alice", "dummy", "cred1", "empty")
	s.AssertUpdateCredential(c, "bob", "dummy", "cred1", "empty")
	mi := s.assertCreateModel(c, createModelParams{name: "test-model", username: "alice", cred: "cred1"})
	mi2 := s.assertCreateModel(c, createModelParams{name: "test-model", username: "bob", cred: "cred1"})

	conn := s.open(c, nil, "alice")
	defer conn.Close()

	modifyModelAccessErrorTests := []struct {
		about             string
		modifyModelAccess jujuparams.ModifyModelAccess
		expectError       string
	}{{
		about: "unauthorized",
		modifyModelAccess: jujuparams.ModifyModelAccess{
			UserTag:  names.NewUserTag("eve@external").String(),
			Action:   jujuparams.GrantModelAccess,
			Access:   jujuparams.ModelReadAccess,
			ModelTag: names.NewModelTag(mi2.UUID).String(),
		},
		expectError: `unauthorized`,
	}, {
		about: "bad user domain",
		modifyModelAccess: jujuparams.ModifyModelAccess{
			UserTag:  names.NewUserTag("eve@local").String(),
			Action:   jujuparams.GrantModelAccess,
			Access:   jujuparams.ModelReadAccess,
			ModelTag: names.NewModelTag(mi.UUID).String(),
		},
		expectError: `unsupported local user`,
	}, {
		about: "no such model",
		modifyModelAccess: jujuparams.ModifyModelAccess{
			UserTag:  names.NewUserTag("eve@external").String(),
			Action:   jujuparams.GrantModelAccess,
			Access:   jujuparams.ModelReadAccess,
			ModelTag: names.NewModelTag("00000000-0000-0000-0000-000000000000").String(),
		},
		expectError: `unauthorized`,
	}, {
		about: "invalid model tag",
		modifyModelAccess: jujuparams.ModifyModelAccess{
			UserTag:  names.NewUserTag("eve@external").String(),
			Action:   jujuparams.GrantModelAccess,
			Access:   jujuparams.ModelReadAccess,
			ModelTag: "not-a-model-tag",
		},
		expectError: `invalid model tag: "not-a-model-tag" is not a valid tag`,
	}, {
		about: "invalid user tag",
		modifyModelAccess: jujuparams.ModifyModelAccess{
			UserTag:  "not-a-user-tag",
			Action:   jujuparams.GrantModelAccess,
			Access:   jujuparams.ModelReadAccess,
			ModelTag: names.NewModelTag(mi.UUID).String(),
		},
		expectError: `invalid user tag: "not-a-user-tag" is not a valid tag`,
	}, {
		about: "unknown action",
		modifyModelAccess: jujuparams.ModifyModelAccess{
			UserTag:  names.NewUserTag("eve@external").String(),
			Action:   "not-an-action",
			Access:   jujuparams.ModelReadAccess,
			ModelTag: names.NewModelTag(mi.UUID).String(),
		},
		expectError: `invalid action "not-an-action"`,
	}, {
		about: "invalid access",
		modifyModelAccess: jujuparams.ModifyModelAccess{
			UserTag:  names.NewUserTag("eve@external").String(),
			Action:   jujuparams.GrantModelAccess,
			Access:   "not-an-access",
			ModelTag: names.NewModelTag(mi.UUID).String(),
		},
		expectError: `"not-an-access" model access not valid`,
	}}

	for i, test := range modifyModelAccessErrorTests {
		c.Logf("%d. %s", i, test.about)
		var res jujuparams.ErrorResults
		req := jujuparams.ModifyModelAccessRequest{
			Changes: []jujuparams.ModifyModelAccess{
				test.modifyModelAccess,
			},
		}
		err := conn.APICall("ModelManager", 2, "", "ModifyModelAccess", req, &res)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(res.Results, gc.HasLen, 1)
		c.Assert(res.Results[0].Error, gc.ErrorMatches, test.expectError)
	}
}

func (s *controllerSuite) TestDestroyModel(c *gc.C) {
	ctlPath := params.EntityPath{User: "alice", Name: "controller-1"}
	s.AssertAddController(c, ctlPath, true)
	s.AssertUpdateCredential(c, "alice", "dummy", "cred1", "empty")
	mi := s.assertCreateModel(c, createModelParams{name: "test-model", username: "alice", cred: "cred1"})

	conn := s.open(c, nil, "alice")
	defer conn.Close()

	client := modelmanager.NewClient(conn)
	tag := names.NewModelTag(mi.UUID)
	err := client.DestroyModel(tag)
	c.Assert(err, jc.ErrorIsNil)

	// Check the model is now dying.
	mis, err := client.ModelInfo([]names.ModelTag{tag})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(mis, gc.HasLen, 1)
	c.Assert(mis[0].Error, gc.Equals, (*jujuparams.Error)(nil))
	c.Assert(mis[0].Result.Life, gc.Equals, jujuparams.Dying)

	// Kill the model.
	err = s.JEM.DB.SetModelLife(testContext, ctlPath, mi.UUID, "dead")
	c.Assert(err, jc.ErrorIsNil)

	// Make sure it's not an error if you destroy a model that't not there.
	err = client.DestroyModel(names.NewModelTag(mi.UUID))
	c.Assert(err, jc.ErrorIsNil)
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
	c.Assert(err, jc.ErrorIsNil)
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
	c.Assert(err, jc.ErrorIsNil)
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

func machineInfo(c *gc.C, m *state.Machine) jujuparams.ModelMachineInfo {
	mi := jujuparams.ModelMachineInfo{
		Id:        m.Id(),
		HasVote:   m.HasVote(),
		WantsVote: m.WantsVote(),
	}
	hc, err := m.HardwareCharacteristics()
	c.Assert(err, jc.ErrorIsNil)
	mi.Hardware = &jujuparams.MachineHardware{
		Arch:             hc.Arch,
		Mem:              hc.Mem,
		RootDisk:         hc.RootDisk,
		Cores:            hc.CpuCores,
		CpuPower:         hc.CpuPower,
		Tags:             hc.Tags,
		AvailabilityZone: hc.AvailabilityZone,
	}
	id, err := m.InstanceId()
	c.Assert(err, jc.ErrorIsNil)
	mi.InstanceId = string(id)
	st, err := m.Status()
	c.Assert(err, jc.ErrorIsNil)
	mi.Status = string(st.Status)
	return mi
}
