// Copyright 2016 Canonical Ltd.

package jujuapi_test

import (
	"bytes"
	"encoding/pem"
	"net/http/httptest"
	"net/url"

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
	"gopkg.in/juju/names.v2"

	"github.com/CanonicalLtd/jem/internal/apitest"
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
	ctlPath := s.AssertAddController(c, params.EntityPath{User: "test", Name: "controller-1"}, nil)
	_, _, modelUUID := s.CreateModel(c, params.EntityPath{User: "test", Name: "model-1"}, ctlPath)
	conn := s.open(c, &api.Info{
		ModelTag:  names.NewModelTag(modelUUID),
		SkipLogin: true,
	}, "test")
	defer conn.Close()
	err := conn.Login(nil, "", "", nil)
	c.Assert(jujuparams.IsRedirect(err), gc.Equals, true)
	var resp jujuparams.RedirectInfoResult
	err = conn.APICall("Admin", 3, "", "RedirectInfo", nil, &resp)
	c.Assert(err, jc.ErrorIsNil)
	nhps, err := network.ParseHostPorts(s.APIInfo(c).Addrs...)
	c.Assert(err, jc.ErrorIsNil)
	hps := jujuparams.FromNetworkHostPorts(nhps)
	c.Assert(resp, jc.DeepEquals, jujuparams.RedirectInfoResult{
		Servers: [][]jujuparams.HostPort{hps},
		CACert:  s.APIInfo(c).CACert,
	})
}

func (s *websocketSuite) TestIncorrectUserFails(c *gc.C) {
	ctlPath := s.AssertAddController(c, params.EntityPath{User: "test", Name: "controller-1"}, nil)
	_, _, modelUUID := s.CreateModel(c, params.EntityPath{User: "test", Name: "model-1"}, ctlPath)
	conn := s.open(c, &api.Info{
		ModelTag:  names.NewModelTag(modelUUID),
		SkipLogin: true,
	}, "bob")
	defer conn.Close()
	err := conn.Login(nil, "", "", nil)
	c.Assert(err, gc.ErrorMatches, "unauthorized")
}

func (s *websocketSuite) TestRedirectInfoFailsWithoutLogin(c *gc.C) {
	ctlPath := s.AssertAddController(c, params.EntityPath{User: "test", Name: "controller-1"}, nil)
	_, _, modelUUID := s.CreateModel(c, params.EntityPath{User: "test", Name: "model-1"}, ctlPath)
	conn := s.open(c, &api.Info{
		ModelTag:  names.NewModelTag(modelUUID),
		SkipLogin: true,
	}, "test")
	defer conn.Close()
	var resp jujuparams.RedirectInfoResult
	err := conn.APICall("Admin", 3, "", "RedirectInfo", nil, &resp)
	c.Assert(err, gc.ErrorMatches, "unauthorized")
}

func (s *websocketSuite) TestOldAdminVersionFails(c *gc.C) {
	ctlPath := s.AssertAddController(c, params.EntityPath{User: "test", Name: "controller-1"}, nil)
	_, _, modelUUID := s.CreateModel(c, params.EntityPath{User: "test", Name: "model-1"}, ctlPath)
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
	ctlPath := s.AssertAddController(c, params.EntityPath{User: "test", Name: "controller-1"}, nil)
	_, _, modelUUID := s.CreateModel(c, params.EntityPath{User: "test", Name: "model-1"}, ctlPath)
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
	s.AssertAddController(c, params.EntityPath{User: "test", Name: "controller-1"}, nil)
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
	ctlPath := s.AssertAddController(c, params.EntityPath{User: "test", Name: "controller-1"}, nil)
	_, _, modelUUID := s.CreateModel(c, params.EntityPath{User: "test", Name: "model-1"}, ctlPath)
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
	s.AssertAddController(c, params.EntityPath{User: "test", Name: "controller-1"}, nil)
	conn := s.open(c, &api.Info{
		ModelTag: s.APIInfo(c).ModelTag,
	}, "test")
	defer conn.Close()
	var resp jujuparams.RedirectInfoResult
	err := conn.APICall("NoSuch", 1, "", "Method", nil, &resp)
	c.Assert(err, gc.ErrorMatches, `unknown version \(1\) of interface "NoSuch" \(not implemented\)`)
}

func (s *websocketSuite) TestCloudCall(c *gc.C) {
	s.AssertAddController(c, params.EntityPath{User: "test", Name: "controller-1"}, nil)
	conn := s.open(c, &api.Info{
		ModelTag: s.APIInfo(c).ModelTag,
	}, "test")
	defer conn.Close()
	client := cloudapi.NewClient(conn)
	info, err := client.Cloud()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, jc.DeepEquals, cloud.Cloud{
		Type:      "dummy",
		AuthTypes: []cloud.AuthType{"empty"},
	})
}

func (s *websocketSuite) TestCloudCredentials(c *gc.C) {
	s.AssertAddController(c, params.EntityPath{User: "test", Name: "controller-1"}, nil)
	s.JEM.AddCredential(&mongodoc.Credential{
		Path:         params.EntityPath{"test", "cred1"},
		ProviderType: "dummy",
		Type:         "credtype",
		Label:        "Credentials 1",
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
	creds, err := client.Credentials(names.NewUserTag("test"))
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
	s.AssertAddController(c, params.EntityPath{User: "test", Name: "controller-1"}, nil)
	s.JEM.AddCredential(&mongodoc.Credential{
		Path:         params.EntityPath{"test2", "cred1"},
		ProviderType: "dummy",
		Type:         "credtype",
		Label:        "Credentials 1",
		Attributes: map[string]string{
			"attr1": "val1",
			"attr2": "val2",
		},
	})
	s.JEM.AddCredential(&mongodoc.Credential{
		Path: params.EntityPath{"test2", "cred2"},
		ACL: params.ACL{
			Read: []string{"test"},
		},
		ProviderType: "dummy",
		Type:         "credtype",
		Label:        "Credentials 2",
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
	creds, err := client.Credentials(names.NewUserTag("test2"))
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
	s.AssertAddController(c, params.EntityPath{User: "test", Name: "controller-1"}, nil)
	conn := s.open(c, &api.Info{
		ModelTag: s.APIInfo(c).ModelTag,
	}, "test")
	defer conn.Close()
	req := jujuparams.Entities{
		Entities: []jujuparams.Entity{{
			Tag: "not-a-user-tag",
		}},
	}
	var resp jujuparams.CloudCredentialsResults
	err := conn.APICall("Cloud", 1, "", "Credentials", req, &resp)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(resp.Results[0].Error, gc.ErrorMatches, `bad request: "not-a-user-tag" is not a valid tag`)
	c.Assert(resp.Results, gc.HasLen, 1)
}

func (s *websocketSuite) TestUpdateCloudCredentials(c *gc.C) {
	s.AssertAddController(c, params.EntityPath{User: "test", Name: "controller-1"}, nil)
	conn := s.open(c, &api.Info{
		ModelTag: s.APIInfo(c).ModelTag,
	}, "test")
	defer conn.Close()
	client := cloudapi.NewClient(conn)
	credsMap := map[string]cloud.Credential{
		"cred3": cloud.NewCredential("credtype", map[string]string{"attr1": "val31", "attr2": "val32"}),
		"cred4": cloud.NewCredential("credtype2", map[string]string{"attr1": "val41", "attr2": "val42"}),
	}
	err := client.UpdateCredentials(names.NewUserTag("test"), credsMap)
	c.Assert(err, jc.ErrorIsNil)
	creds, err := client.Credentials(names.NewUserTag("test"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(creds, jc.DeepEquals, credsMap)
	updateMap := map[string]cloud.Credential{
		"cred3": cloud.NewCredential("credtype", map[string]string{"attr1": "val33", "attr2": "val34"}),
	}
	err = client.UpdateCredentials(names.NewUserTag("test"), updateMap)
	c.Assert(err, jc.ErrorIsNil)
	credsMap["cred3"] = updateMap["cred3"]
	creds, err = client.Credentials(names.NewUserTag("test"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(creds, jc.DeepEquals, credsMap)
}

func (s *websocketSuite) TestUpdateCloudCredentialsErrors(c *gc.C) {
	s.AssertAddController(c, params.EntityPath{User: "test", Name: "controller-1"}, nil)
	conn := s.open(c, &api.Info{
		ModelTag: s.APIInfo(c).ModelTag,
	}, "test")
	defer conn.Close()
	req := jujuparams.UsersCloudCredentials{
		Users: []jujuparams.UserCloudCredentials{{
			UserTag: "not-a-user-tag",
			Credentials: map[string]jujuparams.CloudCredential{
				"cred1": jujuparams.CloudCredential{
					AuthType: "credtype",
					Attributes: map[string]string{
						"attr1": "val1",
					},
				},
			},
		}, {
			UserTag: names.NewUserTag("invalid--user").String(),
			Credentials: map[string]jujuparams.CloudCredential{
				"cred1": jujuparams.CloudCredential{
					AuthType: "credtype",
					Attributes: map[string]string{
						"attr1": "val1",
					},
				},
			},
		}, {
			UserTag: names.NewUserTag("test2").String(),
			Credentials: map[string]jujuparams.CloudCredential{
				"cred1": jujuparams.CloudCredential{
					AuthType: "credtype",
					Attributes: map[string]string{
						"attr1": "val1",
					},
				},
			},
		}, {
			UserTag: names.NewUserTag("test").String(),
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
	ctlPath := s.AssertAddController(c, params.EntityPath{User: "test", Name: "controller-1"}, nil)
	err := s.JEM.SetACL(s.JEM.DB.Controllers(), ctlPath, params.ACL{
		Read: []string{"test2"},
	})
	c.Assert(err, jc.ErrorIsNil)
	_, _, modelUUID1 := s.CreateModel(c, params.EntityPath{User: "test", Name: "model-1"}, ctlPath)
	s.CreateModel(c, params.EntityPath{User: "test2", Name: "model-2"}, ctlPath)
	_, _, modelUUID3 := s.CreateModel(c, params.EntityPath{User: "test2", Name: "model-3"}, ctlPath)
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
		Owner: "test@local",
	}, {
		Name:  "model-1",
		UUID:  modelUUID1,
		Owner: "test@local",
	}, {
		Name:  "model-3",
		UUID:  modelUUID3,
		Owner: "test2@local",
	}})
}

func (s *websocketSuite) TestModelInfo(c *gc.C) {
	ctlPath := s.AssertAddController(c, params.EntityPath{User: "test", Name: "controller-1"}, nil)
	err := s.JEM.SetACL(s.JEM.DB.Controllers(), ctlPath, params.ACL{
		Read: []string{"test2"},
	})
	c.Assert(err, jc.ErrorIsNil)
	_, _, modelUUID1 := s.CreateModel(c, params.EntityPath{User: "test", Name: "model-1"}, ctlPath)
	_, _, modelUUID2 := s.CreateModel(c, params.EntityPath{User: "test2", Name: "model-2"}, ctlPath)
	_, _, modelUUID3 := s.CreateModel(c, params.EntityPath{User: "test2", Name: "model-3"}, ctlPath)
	err = s.JEM.SetACL(s.JEM.DB.Models(), params.EntityPath{User: "test2", Name: "model-3"}, params.ACL{
		Read: []string{"test"},
	})
	conn := s.open(c, nil, "test")
	defer conn.Close()
	client := modelmanager.NewClient(conn)
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
			ControllerUUID:  "deadbeef-0bad-400d-8000-4b1d0d06f00d",
			ProviderType:    "dummy",
			DefaultSeries:   "xenial",
			CloudCredential: "test-model-1",
			OwnerTag:        names.NewUserTag("test").String(),
			Life:            jujuparams.Alive,
			Status: jujuparams.EntityStatus{
				Status: status.StatusAvailable,
			},
		},
	}, {
		Error: &jujuparams.Error{
			Message: "unauthorized",
		},
	}, {
		Result: &jujuparams.ModelInfo{
			Name:            "model-3",
			UUID:            modelUUID3,
			ControllerUUID:  "deadbeef-0bad-400d-8000-4b1d0d06f00d",
			ProviderType:    "dummy",
			DefaultSeries:   "xenial",
			CloudCredential: "test2-model-3",
			OwnerTag:        names.NewUserTag("test2").String(),
			Life:            jujuparams.Alive,
			Status: jujuparams.EntityStatus{
				Status: status.StatusAvailable,
			},
		},
	}})
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
