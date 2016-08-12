// Copyright 2016 Canonical Ltd.

package jujuapi_test

import (
	"bytes"
	"encoding/pem"
	"net/http/httptest"
	"net/url"
	"sync"
	"time"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	cloudapi "github.com/juju/juju/api/cloud"
	"github.com/juju/juju/api/modelmanager"
	jujuparams "github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/network"
	"github.com/juju/juju/status"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/errgo.v1"
	"gopkg.in/juju/names.v2"

	"github.com/CanonicalLtd/jem/internal/apitest"
	"github.com/CanonicalLtd/jem/internal/jujuapi"
	"github.com/CanonicalLtd/jem/internal/mongodoc"
	"github.com/CanonicalLtd/jem/params"
)

type websocketSuite struct {
	apitest.Suite
	wsServer *httptest.Server
}

var _ = gc.Suite(&websocketSuite{})

func (s *websocketSuite) SetUpTest(c *gc.C) {
	s.Suite.SetUpTest(c)
	s.wsServer = httptest.NewTLSServer(s.JEMSrv)
}

func (s *websocketSuite) TearDownTest(c *gc.C) {
	s.wsServer.Close()
	s.Suite.TearDownTest(c)
}

func (s *websocketSuite) TestUnknownModel(c *gc.C) {
	conn := s.open(c, &api.Info{
		ModelTag:  names.NewModelTag("00000000-0000-0000-0000-000000000000"),
		SkipLogin: true,
	}, "bob")
	defer conn.Close()
	err := conn.Login(nil, "", "", nil)
	c.Assert(err, gc.ErrorMatches, `model "00000000-0000-0000-0000-000000000000" not found`)
}

func (s *websocketSuite) TestLoginToModel(c *gc.C) {
	s.AssertAddController(c, params.EntityPath{User: "test", Name: "controller-1"}, true)
	cred := s.AssertUpdateCredential(c, "test", "dummy", "cred1", "empty")
	mi := s.assertCreateModel(c, "model-1", "test", "", string(cred), nil)
	modelUUID := mi.UUID
	conn := s.open(c, &api.Info{
		ModelTag:  names.NewModelTag(modelUUID),
		SkipLogin: true,
	}, "test")
	defer conn.Close()
	nhps, err := network.ParseHostPorts(s.APIInfo(c).Addrs...)
	c.Assert(err, jc.ErrorIsNil)
	err = conn.Login(nil, "", "", nil)
	c.Assert(errgo.Cause(err), jc.DeepEquals, &api.RedirectError{
		Servers: [][]network.HostPort{nhps},
		CACert:  s.APIInfo(c).CACert,
	})
}

func (s *websocketSuite) TestOldAdminVersionFails(c *gc.C) {
	s.AssertAddController(c, params.EntityPath{User: "test", Name: "controller-1"}, true)
	cred := s.AssertUpdateCredential(c, "test", "dummy", "cred1", "empty")
	mi := s.assertCreateModel(c, "model-1", "test", "", string(cred), nil)
	modelUUID := mi.UUID
	conn := s.open(c, &api.Info{
		ModelTag:  names.NewModelTag(modelUUID),
		SkipLogin: true,
	}, "test")
	defer conn.Close()
	var resp jujuparams.RedirectInfoResult
	err := conn.APICall("Admin", 2, "", "Login", nil, &resp)
	c.Assert(err, gc.ErrorMatches, `JAAS does not support login from old clients \(not supported\)`)
	c.Assert(resp, jc.DeepEquals, jujuparams.RedirectInfoResult{})
}

func (s *websocketSuite) TestAdminIDFails(c *gc.C) {
	s.AssertAddController(c, params.EntityPath{User: "test", Name: "controller-1"}, true)
	cred := s.AssertUpdateCredential(c, "test", "dummy", "cred1", "empty")
	mi := s.assertCreateModel(c, "model-1", "test", "", string(cred), nil)
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

func (s *websocketSuite) TestLoginToController(c *gc.C) {
	s.AssertAddController(c, params.EntityPath{User: "test", Name: "controller-1"}, true)
	conn := s.open(c, &api.Info{
		ModelTag:  s.APIInfo(c).ModelTag,
		SkipLogin: true,
	}, "test")
	defer conn.Close()
	err := conn.Login(nil, "", "", nil)
	c.Assert(err, jc.ErrorIsNil)
	var resp jujuparams.RedirectInfoResult
	err = conn.APICall("Admin", 3, "", "RedirectInfo", nil, &resp)
	c.Assert(err, gc.ErrorMatches, "not redirected")
}

func (s *websocketSuite) TestUnimplementedMethodFails(c *gc.C) {
	s.AssertAddController(c, params.EntityPath{User: "test", Name: "controller-1"}, true)
	cred := s.AssertUpdateCredential(c, "test", "dummy", "cred1", "empty")
	mi := s.assertCreateModel(c, "model-1", "test", "", string(cred), nil)
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

func (s *websocketSuite) TestUnimplementedRootFails(c *gc.C) {
	s.AssertAddController(c, params.EntityPath{User: "test", Name: "controller-1"}, true)
	conn := s.open(c, &api.Info{
		ModelTag: s.APIInfo(c).ModelTag,
	}, "test")
	defer conn.Close()
	var resp jujuparams.RedirectInfoResult
	err := conn.APICall("NoSuch", 1, "", "Method", nil, &resp)
	c.Assert(err, gc.ErrorMatches, `unknown version \(1\) of interface "NoSuch" \(not implemented\)`)
}

func (s *websocketSuite) TestCloudCall(c *gc.C) {
	s.AssertAddController(c, params.EntityPath{User: "test", Name: "controller-1"}, true)
	conn := s.open(c, &api.Info{
		ModelTag: s.APIInfo(c).ModelTag,
	}, "test")
	defer conn.Close()
	client := cloudapi.NewClient(conn)
	info, err := client.Cloud(names.NewCloudTag("dummy"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, jc.DeepEquals, cloud.Cloud{
		Type:      "dummy",
		AuthTypes: []cloud.AuthType{"empty"},
	})
}

func (s *websocketSuite) TestCloudCredentials(c *gc.C) {
	s.AssertAddController(c, params.EntityPath{User: "test", Name: "controller-1"}, true)
	s.JEM.UpdateCredential(&mongodoc.Credential{
		User:  "test",
		Cloud: "dummy",
		Name:  "cred1",
		Type:  "credtype",
		Label: "Credentials 1",
		Attributes: map[string]string{
			"attr1": "val1",
			"attr2": "val2",
		},
	})
	conn := s.open(c, &api.Info{
		ModelTag: s.APIInfo(c).ModelTag,
	}, "test")
	defer conn.Close()
	client := cloudapi.NewClient(conn)
	creds, err := client.Credentials(names.NewUserTag("test@external"), names.NewCloudTag("dummy"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(creds, jc.DeepEquals, map[string]cloud.Credential{
		"cred1": cloud.NewCredential(
			"credtype", map[string]string{
				"attr1": "val1",
				"attr2": "val2",
			},
		),
	})
}

func (s *websocketSuite) TestCloudCredentialsACL(c *gc.C) {
	s.AssertAddController(c, params.EntityPath{User: "test", Name: "controller-1"}, true)
	s.JEM.UpdateCredential(&mongodoc.Credential{
		User:  "test",
		Cloud: "dummy",
		Name:  "cred1",
		Type:  "credtype",
		Label: "Credentials 1",
		Attributes: map[string]string{
			"attr1": "val1",
			"attr2": "val2",
		},
	})
	s.JEM.UpdateCredential(&mongodoc.Credential{
		User:  "test2",
		Cloud: "dummy",
		Name:  "cred2",
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
	conn := s.open(c, &api.Info{
		ModelTag: s.APIInfo(c).ModelTag,
	}, "test")
	defer conn.Close()
	client := cloudapi.NewClient(conn)
	creds, err := client.Credentials(names.NewUserTag("test2@external"), names.NewCloudTag("dummy"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(creds, jc.DeepEquals, map[string]cloud.Credential{
		"cred2": cloud.NewCredential(
			"credtype", map[string]string{
				"attr1": "val3",
				"attr2": "val4",
			},
		),
	})
}

func (s *websocketSuite) TestCloudCredentialsErrors(c *gc.C) {
	s.AssertAddController(c, params.EntityPath{User: "test", Name: "controller-1"}, true)
	conn := s.open(c, &api.Info{
		ModelTag: s.APIInfo(c).ModelTag,
	}, "test")
	defer conn.Close()
	req := jujuparams.UserClouds{
		UserClouds: []jujuparams.UserCloud{{
			UserTag:  "not-a-user-tag",
			CloudTag: "dummy",
		}},
	}
	var resp jujuparams.CloudCredentialsResults
	err := conn.APICall("Cloud", 1, "", "Credentials", req, &resp)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(resp.Results[0].Error, gc.ErrorMatches, `bad request: "not-a-user-tag" is not a valid tag`)
	c.Assert(resp.Results, gc.HasLen, 1)
}

func (s *websocketSuite) TestUpdateCloudCredentials(c *gc.C) {
	s.AssertAddController(c, params.EntityPath{User: "test", Name: "controller-1"}, true)
	conn := s.open(c, &api.Info{
		ModelTag: s.APIInfo(c).ModelTag,
	}, "test")
	defer conn.Close()
	client := cloudapi.NewClient(conn)
	credsMap := map[string]cloud.Credential{
		"cred3": cloud.NewCredential("credtype", map[string]string{"attr1": "val31", "attr2": "val32"}),
		"cred4": cloud.NewCredential("credtype2", map[string]string{"attr1": "val41", "attr2": "val42"}),
	}
	err := client.UpdateCredentials(names.NewUserTag("test@external"), names.NewCloudTag("dummy"), credsMap)
	c.Assert(err, jc.ErrorIsNil)
	creds, err := client.Credentials(names.NewUserTag("test@external"), names.NewCloudTag("dummy"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(creds, jc.DeepEquals, credsMap)
	updateMap := map[string]cloud.Credential{
		"cred3": cloud.NewCredential("credtype", map[string]string{"attr1": "val33", "attr2": "val34"}),
	}
	err = client.UpdateCredentials(names.NewUserTag("test@external"), names.NewCloudTag("dummy"), updateMap)
	c.Assert(err, jc.ErrorIsNil)
	credsMap["cred3"] = updateMap["cred3"]
	creds, err = client.Credentials(names.NewUserTag("test@external"), names.NewCloudTag("dummy"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(creds, jc.DeepEquals, credsMap)
}

func (s *websocketSuite) TestUpdateCloudCredentialsErrors(c *gc.C) {
	s.AssertAddController(c, params.EntityPath{User: "test", Name: "controller-1"}, true)
	conn := s.open(c, &api.Info{
		ModelTag: s.APIInfo(c).ModelTag,
	}, "test")
	defer conn.Close()
	req := jujuparams.UsersCloudCredentials{
		Users: []jujuparams.UserCloudCredentials{{
			UserTag:  "not-a-user-tag",
			CloudTag: names.NewCloudTag("dummy").String(),
			Credentials: map[string]jujuparams.CloudCredential{
				"cred1": jujuparams.CloudCredential{
					AuthType: "credtype",
					Attributes: map[string]string{
						"attr1": "val1",
					},
				},
			},
		}, {
			UserTag:  names.NewUserTag("invalid--user@external").String(),
			CloudTag: names.NewCloudTag("dummy").String(),
			Credentials: map[string]jujuparams.CloudCredential{
				"cred1": jujuparams.CloudCredential{
					AuthType: "credtype",
					Attributes: map[string]string{
						"attr1": "val1",
					},
				},
			},
		}, {
			UserTag:  names.NewUserTag("test2@external").String(),
			CloudTag: names.NewCloudTag("dummy").String(),
			Credentials: map[string]jujuparams.CloudCredential{
				"cred1": jujuparams.CloudCredential{
					AuthType: "credtype",
					Attributes: map[string]string{
						"attr1": "val1",
					},
				},
			},
		}, {
			UserTag:  names.NewUserTag("test@external").String(),
			CloudTag: names.NewCloudTag("dummy").String(),
			Credentials: map[string]jujuparams.CloudCredential{
				"bad--name": jujuparams.CloudCredential{
					AuthType: "credtype",
					Attributes: map[string]string{
						"attr1": "val1",
					},
				},
			},
		}},
	}
	var resp jujuparams.ErrorResults
	err := conn.APICall("Cloud", 1, "", "UpdateCredentials", req, &resp)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(resp.Results[0].Error, gc.ErrorMatches, `bad request: "not-a-user-tag" is not a valid tag`)
	c.Assert(resp.Results[1].Error, gc.ErrorMatches, `bad request: invalid user name "invalid--user"`)
	c.Assert(resp.Results[2].Error, gc.ErrorMatches, `unauthorized`)
	c.Assert(resp.Results[3].Error, gc.ErrorMatches, `bad request: invalid name "bad--name"`)
	c.Assert(resp.Results, gc.HasLen, 4)
}

func (s *websocketSuite) TestLoginToRoot(c *gc.C) {
	conn := s.open(c, &api.Info{
		SkipLogin: true,
	}, "test")
	defer conn.Close()
	err := conn.Login(nil, "", "", nil)
	c.Assert(err, jc.ErrorIsNil)
	var resp jujuparams.RedirectInfoResult
	err = conn.APICall("Admin", 3, "", "RedirectInfo", nil, &resp)
	c.Assert(err, gc.ErrorMatches, "not redirected")
}

func (s *websocketSuite) TestListModels(c *gc.C) {
	ctlPath := s.AssertAddController(c, params.EntityPath{User: "test", Name: "controller-1"}, true)
	cred := s.AssertUpdateCredential(c, "test", "dummy", "cred1", "empty")
	cred2 := s.AssertUpdateCredential(c, "test2", "dummy", "cred1", "empty")
	err := s.JEM.SetACL(s.JEM.DB.Controllers(), ctlPath, params.ACL{
		Read: []string{"test2"},
	})
	c.Assert(err, jc.ErrorIsNil)
	mi := s.assertCreateModel(c, "model-1", "test", "", string(cred), nil)
	modelUUID1 := mi.UUID
	s.assertCreateModel(c, "model-2", "test2", "", string(cred2), nil)
	mi = s.assertCreateModel(c, "model-3", "test2", "", string(cred2), nil)
	modelUUID3 := mi.UUID
	err = s.JEM.SetACL(s.JEM.DB.Models(), params.EntityPath{User: "test2", Name: "model-3"}, params.ACL{
		Read: []string{"test"},
	})
	c.Assert(err, jc.ErrorIsNil)
	conn := s.open(c, nil, "test")
	defer conn.Close()
	client := modelmanager.NewClient(conn)
	models, err := client.ListModels("test")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(models, jc.DeepEquals, []base.UserModel{{
		Name:  "controller-1",
		UUID:  "deadbeef-0bad-400d-8000-4b1d0d06f00d",
		Owner: "test@external",
	}, {
		Name:  "model-1",
		UUID:  modelUUID1,
		Owner: "test@external",
	}, {
		Name:  "model-3",
		UUID:  modelUUID3,
		Owner: "test2@external",
	}})
}

func (s *websocketSuite) TestModelInfo(c *gc.C) {
	ctlPath := s.AssertAddController(c, params.EntityPath{User: "test", Name: "controller-1"}, true)
	cred1 := s.AssertUpdateCredential(c, "test", "dummy", "cred1", "empty")
	cred2 := s.AssertUpdateCredential(c, "test2", "dummy", "cred1", "empty")
	err := s.JEM.SetACL(s.JEM.DB.Controllers(), ctlPath, params.ACL{
		Read: []string{"test2"},
	})
	c.Assert(err, jc.ErrorIsNil)

	mi := s.assertCreateModel(c, "model-1", "test", "", "cred1", nil)
	modelUUID1 := mi.UUID
	mi = s.assertCreateModel(c, "model-2", "test2", "", "cred1", nil)
	modelUUID2 := mi.UUID
	mi = s.assertCreateModel(c, "model-3", "test2", "", "cred1", nil)
	modelUUID3 := mi.UUID

	conn := s.open(c, nil, "test")
	defer conn.Close()
	client := modelmanager.NewClient(conn)

	err = s.JEM.SetACL(s.JEM.DB.Models(), params.EntityPath{User: "test2", Name: "model-3"}, params.ACL{
		Read: []string{"test"},
	})
	c.Assert(err, jc.ErrorIsNil)

	models, err := client.ModelInfo([]names.ModelTag{
		names.NewModelTag(modelUUID1),
		names.NewModelTag(modelUUID2),
		names.NewModelTag(modelUUID3),
	})
	c.Assert(err, jc.ErrorIsNil)
	for i := range models {
		if models[i].Result == nil {
			continue
		}
		models[i].Result.Status.Since = nil
	}
	c.Assert(models, jc.DeepEquals, []jujuparams.ModelInfoResult{{
		Result: &jujuparams.ModelInfo{
			Name:            "model-1",
			UUID:            modelUUID1,
			ControllerUUID:  "controller-uuid",
			ProviderType:    "dummy",
			DefaultSeries:   "xenial",
			Cloud:           "dummy",
			CloudCredential: string(cred1),
			OwnerTag:        names.NewUserTag("test@external").String(),
			Life:            jujuparams.Alive,
			Status: jujuparams.EntityStatus{
				Status: status.StatusAvailable,
			},
			Users: []jujuparams.ModelUserInfo{{
				UserName:    "test@external",
				DisplayName: "test",
				Access:      "admin",
			}},
		},
	}, {
		Error: &jujuparams.Error{
			Message: "unauthorized",
		},
	}, {
		Result: &jujuparams.ModelInfo{
			Name:            "model-3",
			UUID:            modelUUID3,
			ControllerUUID:  "controller-uuid",
			ProviderType:    "dummy",
			DefaultSeries:   "xenial",
			Cloud:           "dummy",
			CloudCredential: string(cred2),
			OwnerTag:        names.NewUserTag("test2@external").String(),
			Life:            jujuparams.Alive,
			Status: jujuparams.EntityStatus{
				Status: status.StatusAvailable,
			},
			Users: []jujuparams.ModelUserInfo{{
				UserName:    "test2@external",
				DisplayName: "test2",
				Access:      "admin",
			}},
		},
	}})
}

var createModelTests = []struct {
	about       string
	name        string
	owner       string
	region      string
	credential  string
	config      map[string]interface{}
	expectError string
}{{
	about:      "success",
	name:       "model",
	owner:      "test@external",
	credential: "cred1",
}, {
	about:       "unauthorized user",
	name:        "model-2",
	owner:       "not-test@external",
	credential:  "cred1",
	expectError: "unauthorized",
}, {
	about:       "existing model name",
	name:        "existing-model",
	owner:       "test@external",
	credential:  "cred1",
	expectError: "already exists",
}, {
	about:       "no controller",
	name:        "model-3",
	owner:       "test@external",
	region:      "no-such-region",
	credential:  "cred1",
	expectError: `no matching controllers found \(not found\)`,
}, {
	about:       "local user",
	name:        "model-4",
	owner:       "test@local",
	credential:  "cred1",
	expectError: `unauthorized`,
}, {
	about:       "invalid user",
	name:        "model-5",
	owner:       "test/test@external",
	credential:  "cred1",
	expectError: `invalid owner tag: "user-test/test@external" is not a valid user tag`,
}}

func (s *websocketSuite) TestCreateModel(c *gc.C) {
	s.AssertAddController(c, params.EntityPath{User: "test", Name: "controller-1"}, true)
	s.AssertUpdateCredential(c, "test", "dummy", "cred1", "empty")
	s.assertCreateModel(c, "existing-model", "test", "", "cred1", nil)

	conn := s.open(c, nil, "test")
	defer conn.Close()

	for i, test := range createModelTests {
		c.Logf("test %d. %s", i, test.about)
		var mi jujuparams.ModelInfo
		err := conn.APICall("ModelManager", 2, "", "CreateModel", jujuparams.ModelCreateArgs{
			Name:            test.name,
			OwnerTag:        "user-" + test.owner,
			Config:          test.config,
			CloudRegion:     test.region,
			CloudCredential: test.credential,
		}, &mi)
		if test.expectError != "" {
			c.Assert(err, gc.ErrorMatches, test.expectError)
			continue
		}
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(mi.Name, gc.Equals, test.name)
		c.Assert(mi.UUID, gc.Not(gc.Equals), "")
		ownerTag := names.NewUserTag(test.owner)
		c.Assert(mi.OwnerTag, gc.Equals, ownerTag.String())
		c.Assert(mi.ControllerUUID, gc.Equals, "controller-uuid")
		c.Assert(mi.Users, jc.DeepEquals, []jujuparams.ModelUserInfo{{
			UserName:    test.owner,
			DisplayName: ownerTag.Name(),
			Access:      "admin",
		}})
	}
}

func (s *websocketSuite) TestGrantAndRevokeModel(c *gc.C) {
	s.AssertAddController(c, params.EntityPath{User: "test", Name: "controller-1"}, true)
	s.AssertUpdateCredential(c, "test", "dummy", "cred1", "empty")
	mi := s.assertCreateModel(c, "test-model", "test", "", "cred1", nil)

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

	err = client.RevokeModel("bob@external", "write", mi.UUID)
	c.Assert(err, jc.ErrorIsNil)

	res, err = client2.ModelInfo([]names.ModelTag{names.NewModelTag(mi.UUID)})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res, gc.HasLen, 1)
	c.Assert(res[0].Error, gc.ErrorMatches, "unauthorized")
}

func (s *websocketSuite) TestModifyModelErrors(c *gc.C) {
	s.AssertAddController(c, params.EntityPath{User: "alice", Name: "controller-1"}, true)
	s.AssertAddController(c, params.EntityPath{User: "bob", Name: "controller-1"}, true)
	s.AssertUpdateCredential(c, "alice", "dummy", "cred1", "empty")
	s.AssertUpdateCredential(c, "bob", "dummy", "cred1", "empty")
	mi := s.assertCreateModel(c, "test-model", "alice", "", "cred1", nil)
	mi2 := s.assertCreateModel(c, "test-model", "bob", "", "cred1", nil)

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
		expectError: "unauthorized",
	}, {
		about: "bad user domain",
		modifyModelAccess: jujuparams.ModifyModelAccess{
			UserTag:  names.NewUserTag("eve@local").String(),
			Action:   jujuparams.GrantModelAccess,
			Access:   jujuparams.ModelReadAccess,
			ModelTag: names.NewModelTag(mi.UUID).String(),
		},
		expectError: `unsupported domain "local"`,
	}, {
		about: "no such model",
		modifyModelAccess: jujuparams.ModifyModelAccess{
			UserTag:  names.NewUserTag("eve@external").String(),
			Action:   jujuparams.GrantModelAccess,
			Access:   jujuparams.ModelReadAccess,
			ModelTag: names.NewModelTag("00000000-0000-0000-0000-000000000000").String(),
		},
		expectError: `model "00000000-0000-0000-0000-000000000000" not found`,
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
		expectError: `invalid model access permission "not-an-access"`,
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

func (s *websocketSuite) TestConnectionClosesWhenHeartMonitorDies(c *gc.C) {
	hm := newTestHeartMonitor()
	s.PatchValue(jujuapi.NewHeartMonitor, jujuapi.InternalHeartMonitor(func(time.Duration) jujuapi.HeartMonitor {
		return hm
	}))
	conn := s.open(c, nil, "test")
	defer conn.Close()
	hm.waitForFirstPing(c, time.Second)
	hm.kill(time.Now())
	beats := 1
	var err error
	for beats < 10 {
		time.Sleep(10 * time.Millisecond)
		err = conn.Ping()
		if err != nil {
			break
		}
		beats++
	}
	c.Assert(err, gc.ErrorMatches, `connection is shut down`)
	c.Assert(hm.beats(), gc.Equals, 1)
}

func (s *websocketSuite) TestPingerupdatesHeartMonitor(c *gc.C) {
	hm := newTestHeartMonitor()
	s.PatchValue(jujuapi.NewHeartMonitor, jujuapi.InternalHeartMonitor(func(time.Duration) jujuapi.HeartMonitor {
		return hm
	}))
	conn := s.open(c, nil, "test")
	defer conn.Close()
	hm.waitForFirstPing(c, time.Second)
	err := conn.Ping()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(hm.beats(), gc.Equals, 2)
}

// open creates a new websockec connection to the test server, using the
// connection info specified in info. If info is nil then default values
// will be used.
func (s *websocketSuite) open(c *gc.C, info *api.Info, username string) api.Connection {
	var inf api.Info
	if info != nil {
		inf = *info
	}
	u, err := url.Parse(s.wsServer.URL)
	c.Assert(err, jc.ErrorIsNil)
	inf.Addrs = []string{
		u.Host,
	}
	w := new(bytes.Buffer)
	err = pem.Encode(w, &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: s.wsServer.TLS.Certificates[0].Certificate[0],
	})
	c.Assert(err, jc.ErrorIsNil)
	inf.CACert = w.String()
	conn, err := api.Open(&inf, api.DialOpts{
		InsecureSkipVerify: true,
		BakeryClient:       s.IDMSrv.Client(username),
	})
	c.Assert(err, jc.ErrorIsNil)
	return conn
}

// assertCreateModel creates a model for use in tests. The model info for the newly created model is returned.
func (s *websocketSuite) assertCreateModel(c *gc.C, name, username, region, credential string, config map[string]interface{}) jujuparams.ModelInfo {
	conn := s.open(c, nil, username)
	defer conn.Close()
	client := modelmanager.NewClient(conn)
	mi, err := client.CreateModel(name, username+"@external", region, credential, config)
	c.Assert(err, jc.ErrorIsNil)
	return mi
}
