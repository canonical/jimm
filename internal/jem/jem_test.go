// Copyright 2015 Canonical Ltd.

package jem_test

import (
	"fmt"
	"time"

	cloudapi "github.com/juju/juju/api/cloud"
	"github.com/juju/juju/api/controller"
	modelmanagerapi "github.com/juju/juju/api/modelmanager"
	jujuparams "github.com/juju/juju/apiserver/params"
	corejujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state/multiwatcher"
	jujujujutesting "github.com/juju/juju/testing"
	jt "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"golang.org/x/net/context"
	gc "gopkg.in/check.v1"
	"gopkg.in/errgo.v1"
	"gopkg.in/juju/names.v2"
	"gopkg.in/mgo.v2/bson"

	"github.com/CanonicalLtd/jem/internal/apiconn"
	"github.com/CanonicalLtd/jem/internal/auth"
	"github.com/CanonicalLtd/jem/internal/jem"
	"github.com/CanonicalLtd/jem/internal/mongodoc"
	"github.com/CanonicalLtd/jem/params"
)

type jemSuite struct {
	corejujutesting.JujuConnSuite
	pool *jem.Pool
	jem  *jem.JEM
}

var _ = gc.Suite(&jemSuite{})

func (s *jemSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	pool, err := jem.NewPool(jem.Params{
		DB:              s.Session.DB("jem"),
		MaxDBClones:     1000,
		MaxDBAge:        time.Minute,
		ControllerAdmin: "controller-admin",
	})
	c.Assert(err, gc.IsNil)
	s.pool = pool
	s.jem = s.pool.JEM()
}

func (s *jemSuite) TearDownTest(c *gc.C) {
	s.jem.Close()
	s.pool.Close()
	s.JujuConnSuite.TearDownTest(c)
}

func (s *jemSuite) TestPoolRequiresControllerAdmin(c *gc.C) {
	pool, err := jem.NewPool(jem.Params{
		DB:          s.Session.DB("jem"),
		MaxDBClones: 1000,
		MaxDBAge:    time.Minute,
	})
	c.Assert(err, gc.ErrorMatches, "no controller admin group specified")
	c.Assert(pool, gc.IsNil)
}

func (s *jemSuite) TestPoolCopiesOnSize(c *gc.C) {
	pool, err := jem.NewPool(jem.Params{
		DB:              s.Session.DB("jem"),
		MaxDBClones:     1,
		MaxDBAge:        time.Minute,
		ControllerAdmin: "controller-admin",
	})
	c.Assert(err, jc.ErrorIsNil)
	defer pool.Close()
	j1 := pool.JEM()
	defer j1.Close()
	j2 := pool.JEM()
	defer j2.Close()
	c.Assert(jem.RefCount(j1.DB), gc.Not(gc.Equals), jem.RefCount(j2.DB))
}

func (s *jemSuite) TestPoolCopiesOnAge(c *gc.C) {
	cl := jt.NewClock(time.Now())
	s.PatchValue(jem.WallClock, cl)
	pool, err := jem.NewPool(jem.Params{
		DB:              s.Session.DB("jem"),
		MaxDBClones:     1000,
		MaxDBAge:        time.Minute,
		ControllerAdmin: "controller-admin",
	})
	c.Assert(err, jc.ErrorIsNil)
	defer pool.Close()
	j1 := pool.JEM()
	defer j1.Close()
	cl.Advance(61 * time.Second)
	j2 := pool.JEM()
	defer j2.Close()
	c.Assert(jem.RefCount(j1.DB), gc.Not(gc.Equals), jem.RefCount(j2.DB))
}

func (s *jemSuite) TestClone(c *gc.C) {
	j := s.jem.Clone()
	j.Close()
	_, err := s.jem.DB.Model(params.EntityPath{"bob", "x"})
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)
}

var createModelTests = []struct {
	about            string
	params           jem.CreateModelParams
	expectError      string
	expectErrorCause error
}{{
	about: "success",
	params: jem.CreateModelParams{
		Path:           params.EntityPath{"bob", ""},
		ControllerPath: params.EntityPath{"bob", "controller"},
		Credential:     "cred1",
		Cloud:          "dummy",
	},
}, {
	about: "unknown credential",
	params: jem.CreateModelParams{
		Path:           params.EntityPath{"bob", ""},
		ControllerPath: params.EntityPath{"bob", "controller"},
		Credential:     "cred2",
		Cloud:          "dummy",
	},
	expectError:      `credential "dummy/bob/cred2" not found`,
	expectErrorCause: params.ErrNotFound,
}, {
	about: "model exists",
	params: jem.CreateModelParams{
		Path:           params.EntityPath{"bob", "controller"},
		ControllerPath: params.EntityPath{"bob", "controller"},
		Credential:     "cred1",
		Cloud:          "dummy",
	},
	expectError:      `already exists`,
	expectErrorCause: params.ErrAlreadyExists,
}, {
	about: "unrecognised region",
	params: jem.CreateModelParams{
		Path:           params.EntityPath{"bob", ""},
		ControllerPath: params.EntityPath{"bob", "controller"},
		Credential:     "cred1",
		Cloud:          "dummy",
		Region:         "not-a-region",
	},
	expectError: `cannot create model: getting cloud region definition: region "not-a-region" not found \(expected one of \["dummy-region"\]\) \(not found\)`,
}, {
	about: "with region",
	params: jem.CreateModelParams{
		Path:           params.EntityPath{"bob", ""},
		ControllerPath: params.EntityPath{"bob", "controller"},
		Credential:     "cred1",
		Cloud:          "dummy",
		Region:         "dummy-region",
	},
}}

func (s *jemSuite) TestCreateModel(c *gc.C) {
	now := bson.Now()
	s.PatchValue(jem.WallClock, jt.NewClock(now))
	ctlId := s.addController(c, params.EntityPath{"bob", "controller"})
	err := jem.UpdateCredential(s.jem.DB, &mongodoc.Credential{
		Path: credentialPath("dummy", "bob", "cred1"),
		Type: "empty",
	})
	conn, err := s.jem.OpenAPI(ctlId)
	c.Assert(err, jc.ErrorIsNil)
	defer conn.Close()
	c.Assert(err, jc.ErrorIsNil)
	_, _, err = s.jem.CreateModel(conn, jem.CreateModelParams{
		Path:           params.EntityPath{"bob", "controller"},
		ControllerPath: params.EntityPath{"bob", "controller"},
		Credential:     "cred1",
		Cloud:          "dummy",
	})
	c.Assert(err, jc.ErrorIsNil)
	for i, test := range createModelTests {
		c.Logf("test %d. %s", i, test.about)
		if test.params.Path.Name == "" {
			test.params.Path.Name = params.Name(fmt.Sprintf("test-%d", i))
		}
		m, _, err := s.jem.CreateModel(conn, test.params)
		if test.expectError != "" {
			c.Assert(err, gc.ErrorMatches, test.expectError)
			if test.expectErrorCause != nil {
				c.Assert(errgo.Cause(err), gc.Equals, test.expectErrorCause)
			}
			continue
		}
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(m.Path, jc.DeepEquals, test.params.Path)
		c.Assert(m.UUID, gc.Not(gc.Equals), "")
		c.Assert(m.CreationTime.Equal(now), gc.Equals, true)
	}
}

func (s *jemSuite) TestGrantModel(c *gc.C) {
	conn, model := s.bootstrapModel(c, params.EntityPath{User: "bob", Name: "model"})
	defer conn.Close()
	err := s.jem.GrantModel(conn, model, "alice", "write")
	c.Assert(err, jc.ErrorIsNil)
	model1, err := s.jem.DB.Model(model.Path)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(model1.ACL, jc.DeepEquals, params.ACL{Read: []string{"alice"}})
}

func (s *jemSuite) TestGrantModelControllerFailure(c *gc.C) {
	conn, model := s.bootstrapModel(c, params.EntityPath{User: "bob", Name: "model"})
	defer conn.Close()
	err := s.jem.GrantModel(conn, model, "alice", "superpowers")
	c.Assert(err, gc.ErrorMatches, `"superpowers" model access not valid`)
	model1, err := s.jem.DB.Model(model.Path)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(model1.ACL, jc.DeepEquals, params.ACL{Read: []string{}})
}

func (s *jemSuite) TestRevokeModel(c *gc.C) {
	conn, model := s.bootstrapModel(c, params.EntityPath{User: "bob", Name: "model"})
	defer conn.Close()
	err := s.jem.GrantModel(conn, model, "alice", "write")
	c.Assert(err, jc.ErrorIsNil)
	model1, err := s.jem.DB.Model(model.Path)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(model1.ACL, jc.DeepEquals, params.ACL{Read: []string{"alice"}})
	err = s.jem.RevokeModel(conn, model, "alice", "write")
	c.Assert(err, jc.ErrorIsNil)
	model1, err = s.jem.DB.Model(model.Path)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(model1.ACL, jc.DeepEquals, params.ACL{Read: []string{}})
}

func (s *jemSuite) TestRevokeModelControllerFailure(c *gc.C) {
	conn, model := s.bootstrapModel(c, params.EntityPath{User: "bob", Name: "model"})
	defer conn.Close()
	err := s.jem.GrantModel(conn, model, "alice", "write")
	c.Assert(err, jc.ErrorIsNil)
	model1, err := s.jem.DB.Model(model.Path)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(model1.ACL, jc.DeepEquals, params.ACL{Read: []string{"alice"}})
	err = s.jem.RevokeModel(conn, model, "alice", "superpowers")
	c.Assert(err, gc.ErrorMatches, `"superpowers" model access not valid`)
	model1, err = s.jem.DB.Model(model.Path)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(model1.ACL, jc.DeepEquals, params.ACL{Read: []string{}})
}

func (s *jemSuite) TestDestroyModel(c *gc.C) {
	conn, model := s.bootstrapModel(c, params.EntityPath{User: "bob", Name: "model"})
	defer conn.Close()

	// Sanity check the model exists
	client := modelmanagerapi.NewClient(conn)
	models, err := client.ListModels("bob@external")
	c.Assert(err, jc.ErrorIsNil)
	var found bool
	for _, m := range models {
		if m.UUID == model.UUID {
			c.Logf("found %#v", m)
			found = true
			break
		}
	}
	c.Assert(found, gc.Equals, true)

	ch := waitForDestruction(conn, c, model.UUID)

	err = s.jem.DestroyModel(conn, model)
	c.Assert(err, jc.ErrorIsNil)

	select {
	case <-ch:
	case <-time.After(jujujujutesting.LongWait):
		c.Fatalf("model not destroyed")
	}

	// Check the model is removed.
	_, err = s.jem.DB.Model(model.Path)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)

	// Check that it cannot be destroyed twice
	err = s.jem.DestroyModel(conn, model)
	c.Assert(err, gc.ErrorMatches, `model "bob/model" not found`)

	// Put the model back in the database
	err = s.jem.DB.AddModel(model)
	c.Assert(err, jc.ErrorIsNil)

	// Check that it can still be removed even if the contoller has no model.
	err = s.jem.DestroyModel(conn, model)
	c.Assert(err, jc.ErrorIsNil)

	// Ensure the model is removed.
	_, err = s.jem.DB.Model(model.Path)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)
}

func waitForDestruction(conn *apiconn.Conn, c *gc.C, uuid string) <-chan struct{} {
	ch := make(chan struct{})
	watcher, err := controller.NewClient(conn).WatchAllModels()
	go func() {
		defer close(ch)
		if !c.Check(err, jc.ErrorIsNil) {
			return
		}
		for {
			deltas, err := watcher.Next()
			if !c.Check(err, jc.ErrorIsNil) {
				return
			}
			for _, d := range deltas {
				d, ok := d.Entity.(*multiwatcher.ModelInfo)
				if ok && d.ModelUUID == uuid && d.Life == "dead" {
					return
				}
			}
		}
	}()
	return ch
}

func (s *jemSuite) TestUpdateCredential(c *gc.C) {
	ctlPath := s.addController(c, params.EntityPath{User: "bob", Name: "controller"})
	credPath := credentialPath("dummy", "bob", "cred")
	cred := &mongodoc.Credential{
		Path: credPath,
		Type: "empty",
	}
	err := jem.UpdateCredential(s.jem.DB, cred)
	c.Assert(err, jc.ErrorIsNil)
	conn, err := s.jem.OpenAPI(ctlPath)
	c.Assert(err, jc.ErrorIsNil)
	defer conn.Close()

	err = jem.UpdateControllerCredential(s.jem, ctlPath, cred.Path, conn, cred)
	c.Assert(err, jc.ErrorIsNil)
	err = jem.CredentialAddController(s.jem.DB, credPath, ctlPath)
	c.Assert(err, jc.ErrorIsNil)

	// Sanity check it was deployed
	client := cloudapi.NewClient(conn)
	credTag := names.NewCloudCredentialTag("dummy/bob@external/cred")
	creds, err := client.Credentials(credTag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(creds, jc.DeepEquals, []jujuparams.CloudCredentialResult{{
		Result: &jujuparams.CloudCredential{
			AuthType: "empty",
		},
	}})

	err = s.jem.UpdateCredential(context.Background(), &mongodoc.Credential{
		Path: credPath,
		Type: "userpass",
		Attributes: map[string]string{
			"username": "cloud-user",
			"password": "cloud-pass",
		},
	})
	c.Assert(err, jc.ErrorIsNil)

	// check it was updated on the controller.
	creds, err = client.Credentials(credTag)
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
	}})

	// Revoke the credential
	err = s.jem.UpdateCredential(context.Background(), &mongodoc.Credential{
		Path:    credPath,
		Revoked: true,
	})
	c.Assert(err, jc.ErrorIsNil)

	// check it was removed on the controller.
	creds, err = client.Credentials(credTag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(creds, jc.DeepEquals, []jujuparams.CloudCredentialResult{{
		Error: &jujuparams.Error{
			Code:    "not found",
			Message: `credential "cred" not found`,
		},
	}})
}

func (s *jemSuite) TestControllerUpdateCredentials(c *gc.C) {
	ctlPath := s.addController(c, params.EntityPath{User: "bob", Name: "controller"})
	credPath := credentialPath("dummy", "bob", "cred")
	credTag := names.NewCloudCredentialTag("dummy/bob@external/cred")
	cred := &mongodoc.Credential{
		Path: credPath,
		Type: "empty",
	}
	err := jem.UpdateCredential(s.jem.DB, cred)
	c.Assert(err, jc.ErrorIsNil)

	err = jem.SetCredentialUpdates(s.jem.DB, []params.EntityPath{ctlPath}, credPath)
	c.Assert(err, jc.ErrorIsNil)

	err = s.jem.ControllerUpdateCredentials(ctlPath)
	c.Assert(err, jc.ErrorIsNil)

	// check it was updated on the controller.
	conn, err := s.jem.OpenAPI(ctlPath)
	c.Assert(err, jc.ErrorIsNil)
	defer conn.Close()

	client := cloudapi.NewClient(conn)
	creds, err := client.Credentials(credTag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(creds, jc.DeepEquals, []jujuparams.CloudCredentialResult{{
		Result: &jujuparams.CloudCredential{
			AuthType:   "empty",
			Attributes: nil,
			Redacted:   nil,
		},
	}})
}

var checkReadACLTests = []struct {
	about            string
	owner            string
	acl              []string
	user             string
	groups           []string
	skipCreateEntity bool
	expectError      string
	expectCause      error
}{{
	about: "user is owner",
	owner: "bob",
	user:  "bob",
}, {
	about:  "owner is user group",
	owner:  "bobgroup",
	user:   "bob",
	groups: []string{"bobgroup"},
}, {
	about: "acl contains user",
	owner: "fred",
	acl:   []string{"bob"},
	user:  "bob",
}, {
	about:  "acl contains user's group",
	owner:  "fred",
	acl:    []string{"bobgroup"},
	user:   "bob",
	groups: []string{"bobgroup"},
}, {
	about:       "user not in acl",
	owner:       "fred",
	acl:         []string{"fredgroup"},
	user:        "bob",
	expectError: "unauthorized",
	expectCause: params.ErrUnauthorized,
}, {
	about:            "no entity and not owner",
	owner:            "fred",
	user:             "bob",
	skipCreateEntity: true,
	expectError:      "unauthorized",
	expectCause:      params.ErrUnauthorized,
}}

func (s *jemSuite) TestCheckReadACL(c *gc.C) {
	for i, test := range checkReadACLTests {
		c.Logf("%d. %s", i, test.about)
		gs := append(test.groups, test.user)
		ctx := auth.AuthenticateForTest(context.Background(), gs...)
		entity := params.EntityPath{
			User: params.User(test.owner),
			Name: params.Name(fmt.Sprintf("test%d", i)),
		}
		if !test.skipCreateEntity {
			err := s.jem.DB.AddModel(&mongodoc.Model{
				Path: entity,
				ACL: params.ACL{
					Read: test.acl,
				},
			})
			c.Assert(err, gc.IsNil)
		}
		err := jem.CheckReadACL(ctx, s.jem.DB.Models(), entity)
		if test.expectError != "" {
			c.Assert(err, gc.ErrorMatches, test.expectError)
			if test.expectCause != nil {
				c.Assert(errgo.Cause(err), gc.Equals, test.expectCause)
			} else {
				c.Assert(errgo.Cause(err), gc.Equals, err)
			}
		} else {
			c.Assert(err, gc.IsNil)
		}
	}
}

func (s *jemSuite) TestCheckGetACL(c *gc.C) {
	m := &mongodoc.Model{
		Path: params.EntityPath{
			User: params.User("bob"),
			Name: "model",
		},
		ACL: params.ACL{
			Read: []string{"fred", "jim"},
		},
	}
	err := s.jem.DB.AddModel(m)
	c.Assert(err, gc.IsNil)
	acl, err := jem.GetACL(s.jem.DB.Models(), m.Path)
	c.Assert(err, gc.IsNil)
	c.Assert(acl, jc.DeepEquals, m.ACL)
}

func (s *jemSuite) TestCheckGetACLNotFound(c *gc.C) {
	m := &mongodoc.Model{
		Path: params.EntityPath{
			User: params.User("bob"),
			Name: "model",
		},
	}
	acl, err := jem.GetACL(s.jem.DB.Models(), m.Path)
	c.Assert(err, gc.ErrorMatches, "not found")
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)
	c.Assert(acl, jc.DeepEquals, m.ACL)
}

func (s *jemSuite) TestCanReadIter(c *gc.C) {
	testModels := []mongodoc.Model{{
		Path: params.EntityPath{
			User: params.User("bob"),
			Name: "m1",
		},
	}, {
		Path: params.EntityPath{
			User: params.User("fred"),
			Name: "m2",
		},
	}, {
		Path: params.EntityPath{
			User: params.User("fred"),
			Name: "m3",
		},
		ACL: params.ACL{
			Read: []string{"bob"},
		},
	}}
	for i := range testModels {
		err := s.jem.DB.AddModel(&testModels[i])
		c.Assert(err, gc.IsNil)
	}
	ctx := auth.AuthenticateForTest(context.Background(), "bob", "bob-group")
	it := s.jem.DB.Models().Find(nil).Sort("_id").Iter()
	crit := jem.NewCanReadIter(ctx, it)
	var models []mongodoc.Model
	var m mongodoc.Model
	for crit.Next(&m) {
		models = append(models, m)
	}
	c.Assert(crit.Err(), gc.IsNil)
	c.Assert(models, jc.DeepEquals, []mongodoc.Model{
		testModels[0],
		testModels[2],
	})
	c.Assert(crit.Count(), gc.Equals, 3)
}

var doContollerTests = []struct {
	about             string
	cloud             params.Cloud
	region            string
	expectControllers []params.EntityPath
}{{
	about: "no parameters",
	expectControllers: []params.EntityPath{{
		User: "alice",
		Name: "aws-eu-west-1",
	}, {
		User: "alice",
		Name: "aws-us-east-1",
	}, {
		User: "bob",
		Name: "aws-eu-west-1",
	}, {
		User: "bob",
		Name: "aws-us-east-1",
	}, {
		User: "bob",
		Name: "gce-us-east-1",
	}},
}, {
	about: "aws",
	cloud: "aws",
	expectControllers: []params.EntityPath{{
		User: "alice",
		Name: "aws-eu-west-1",
	}, {
		User: "alice",
		Name: "aws-us-east-1",
	}, {
		User: "bob",
		Name: "aws-eu-west-1",
	}, {
		User: "bob",
		Name: "aws-us-east-1",
	}},
}, {
	about:  "aws-us-east-1",
	cloud:  "aws",
	region: "us-east-1",
	expectControllers: []params.EntityPath{{
		User: "alice",
		Name: "aws-us-east-1",
	}, {
		User: "bob",
		Name: "aws-us-east-1",
	}},
}, {
	about:             "aws-us-east-1",
	cloud:             "aws",
	region:            "us-east-2",
	expectControllers: []params.EntityPath{},
}}

func (s *jemSuite) TestDoControllers(c *gc.C) {
	testControllers := []mongodoc.Controller{{
		Path: params.EntityPath{
			User: params.User("bob"),
			Name: "aws-us-east-1",
		},
		Cloud: mongodoc.Cloud{
			Name: "aws",
			Regions: []mongodoc.Region{{
				Name: "us-east-1",
			}},
		},
		Public: true,
	}, {
		Path: params.EntityPath{
			User: params.User("bob"),
			Name: "aws-eu-west-1",
		},
		Cloud: mongodoc.Cloud{
			Name: "aws",
			Regions: []mongodoc.Region{{
				Name: "eu-west-1",
			}},
		},
		Public: true,
	}, {
		Path: params.EntityPath{
			User: params.User("alice"),
			Name: "aws-us-east-1",
		},
		ACL: params.ACL{
			Read: []string{"bob-group"},
		},
		Cloud: mongodoc.Cloud{
			Name: "aws",
			Regions: []mongodoc.Region{{
				Name: "us-east-1",
			}},
		},
		Public: true,
	}, {
		Path: params.EntityPath{
			User: params.User("alice"),
			Name: "aws-eu-west-1",
		},
		ACL: params.ACL{
			Read: []string{"bob"},
		},
		Cloud: mongodoc.Cloud{
			Name: "aws",
			Regions: []mongodoc.Region{{
				Name: "eu-west-1",
			}},
		},
		Public: true,
	}, {
		Path: params.EntityPath{
			User: params.User("alice"),
			Name: "aws-us-east-2",
		},
		Cloud: mongodoc.Cloud{
			Name: "aws",
			Regions: []mongodoc.Region{{
				Name: "us-east-1",
			}},
		},
		Public: true,
	}, {
		Path: params.EntityPath{
			User: params.User("bob"),
			Name: "gce-us-east-1",
		},
		Cloud: mongodoc.Cloud{
			Name: "gce",
			Regions: []mongodoc.Region{{
				Name: "us-east-1",
			}},
		},
		Public: true,
	}, {
		Path: params.EntityPath{
			User: params.User("alice"),
			Name: "gce-us-east-1",
		},
		Cloud: mongodoc.Cloud{
			Name: "gce",
			Regions: []mongodoc.Region{{
				Name: "us-east-1",
			}},
		},
		Public: true,
	}}
	for i := range testControllers {
		err := s.jem.DB.AddController(&testControllers[i])

		c.Assert(err, gc.IsNil)
	}
	ctx := auth.AuthenticateForTest(context.Background(), "bob", "bob-group")
	for i, test := range doContollerTests {
		c.Logf("test %d. %s", i, test.about)
		var obtainedControllers []params.EntityPath
		err := s.jem.DoControllers(ctx, test.cloud, test.region, func(ctl *mongodoc.Controller) error {
			obtainedControllers = append(obtainedControllers, ctl.Path)
			return nil
		})
		c.Assert(err, gc.IsNil)
		c.Assert(obtainedControllers, jc.DeepEquals, test.expectControllers)
	}
}

func (s *jemSuite) TestDoControllersErrorResponse(c *gc.C) {
	testControllers := []mongodoc.Controller{{
		Path: params.EntityPath{
			User: params.User("bob"),
			Name: "aws-us-east-1",
		},
		Cloud: mongodoc.Cloud{
			Name: "aws",
			Regions: []mongodoc.Region{{
				Name: "us-east-1",
			}},
		},
		Public: true,
	}, {
		Path: params.EntityPath{
			User: params.User("bob"),
			Name: "aws-eu-west-1",
		},
		Cloud: mongodoc.Cloud{
			Name: "aws",
			Regions: []mongodoc.Region{{
				Name: "eu-west-1",
			}},
		},
		Public: true,
	}, {
		Path: params.EntityPath{
			User: params.User("alice"),
			Name: "aws-us-east-1",
		},
		ACL: params.ACL{
			Read: []string{"bob-group"},
		},
		Cloud: mongodoc.Cloud{
			Name: "aws",
			Regions: []mongodoc.Region{{
				Name: "us-east-1",
			}},
		},
		Public: true,
	}, {
		Path: params.EntityPath{
			User: params.User("alice"),
			Name: "aws-eu-west-1",
		},
		ACL: params.ACL{
			Read: []string{"bob"},
		},
		Cloud: mongodoc.Cloud{
			Name: "aws",
			Regions: []mongodoc.Region{{
				Name: "eu-west-1",
			}},
		},
		Public: true,
	}, {
		Path: params.EntityPath{
			User: params.User("alice"),
			Name: "aws-us-east-2",
		},
		Cloud: mongodoc.Cloud{
			Name: "aws",
			Regions: []mongodoc.Region{{
				Name: "us-east-1",
			}},
		},
		Public: true,
	}, {
		Path: params.EntityPath{
			User: params.User("bob"),
			Name: "gce-us-east-1",
		},
		Cloud: mongodoc.Cloud{
			Name: "gce",
			Regions: []mongodoc.Region{{
				Name: "us-east-1",
			}},
		},
		Public: true,
	}, {
		Path: params.EntityPath{
			User: params.User("alice"),
			Name: "gce-us-east-1",
		},
		Cloud: mongodoc.Cloud{
			Name: "gce",
			Regions: []mongodoc.Region{{
				Name: "us-east-1",
			}},
		},
		Public: true,
	}}
	for i := range testControllers {
		err := s.jem.DB.AddController(&testControllers[i])

		c.Assert(err, gc.IsNil)
	}
	ctx := auth.AuthenticateForTest(context.Background(), "bob", "bob-group")
	testCause := errgo.New("test-cause")
	err := s.jem.DoControllers(ctx, "", "", func(ctl *mongodoc.Controller) error {
		return errgo.WithCausef(nil, testCause, "test error")
	})
	c.Assert(errgo.Cause(err), gc.Equals, testCause)
}

var selectContollerTests = []struct {
	about            string
	cloud            params.Cloud
	region           string
	randIntn         func(int) int
	expectController params.EntityPath
	expectCloud      params.Cloud
	expectRegion     string
	expectError      string
	expectErrorCause error
}{{
	about: "no parameters",
	randIntn: func(n int) int {
		return 4
	},
	expectController: params.EntityPath{
		User: "bob",
		Name: "gce-us-east-1",
	},
	expectCloud: "gce",
}, {
	about: "aws",
	cloud: "aws",
	randIntn: func(n int) int {
		return 1
	},
	expectController: params.EntityPath{
		User: "alice",
		Name: "aws-us-east-1",
	},
	expectCloud: "aws",
}, {
	about:  "aws-us-east-1",
	cloud:  "aws",
	region: "us-east-1",
	randIntn: func(n int) int {
		return 1
	},
	expectController: params.EntityPath{
		User: "bob",
		Name: "aws-us-east-1",
	},
	expectCloud:  "aws",
	expectRegion: "us-east-1",
}, {
	about:  "no match",
	cloud:  "aws",
	region: "us-east-2",
	randIntn: func(n int) int {
		return 1
	},
	expectError:      `no matching controllers found`,
	expectErrorCause: params.ErrNotFound,
}}

func (s *jemSuite) TestSelectController(c *gc.C) {
	var randIntn *func(int) int
	s.PatchValue(jem.RandIntn, func(n int) int {
		return (*randIntn)(n)
	})
	testControllers := []mongodoc.Controller{{
		Path: params.EntityPath{
			User: params.User("bob"),
			Name: "aws-us-east-1",
		},
		Cloud: mongodoc.Cloud{
			Name: "aws",
			Regions: []mongodoc.Region{{
				Name: "us-east-1",
			}},
		},
		Public: true,
	}, {
		Path: params.EntityPath{
			User: params.User("bob"),
			Name: "aws-eu-west-1",
		},
		Cloud: mongodoc.Cloud{
			Name: "aws",
			Regions: []mongodoc.Region{{
				Name: "eu-west-1",
			}},
		},
		Public: true,
	}, {
		Path: params.EntityPath{
			User: params.User("alice"),
			Name: "aws-us-east-1",
		},
		ACL: params.ACL{
			Read: []string{"bob-group"},
		},
		Cloud: mongodoc.Cloud{
			Name: "aws",
			Regions: []mongodoc.Region{{
				Name: "us-east-1",
			}},
		},
		Public: true,
	}, {
		Path: params.EntityPath{
			User: params.User("alice"),
			Name: "aws-eu-west-1",
		},
		ACL: params.ACL{
			Read: []string{"bob"},
		},
		Cloud: mongodoc.Cloud{
			Name: "aws",
			Regions: []mongodoc.Region{{
				Name: "eu-west-1",
			}},
		},
		Public: true,
	}, {
		Path: params.EntityPath{
			User: params.User("alice"),
			Name: "aws-us-east-2",
		},
		Cloud: mongodoc.Cloud{
			Name: "aws",
			Regions: []mongodoc.Region{{
				Name: "us-east-1",
			}},
		},
		Public: true,
	}, {
		Path: params.EntityPath{
			User: params.User("bob"),
			Name: "gce-us-east-1",
		},
		Cloud: mongodoc.Cloud{
			Name: "gce",
			Regions: []mongodoc.Region{{
				Name: "us-east-1",
			}},
		},
		Public: true,
	}, {
		Path: params.EntityPath{
			User: params.User("alice"),
			Name: "gce-us-east-1",
		},
		Cloud: mongodoc.Cloud{
			Name: "gce",
			Regions: []mongodoc.Region{{
				Name: "us-east-1",
			}},
		},
		Public: true,
	}}
	for i := range testControllers {
		err := s.jem.DB.AddController(&testControllers[i])

		c.Assert(err, gc.IsNil)
	}
	ctx := auth.AuthenticateForTest(context.Background(), "bob", "bob-group")
	for i, test := range selectContollerTests {
		c.Logf("test %d. %s", i, test.about)
		randIntn = &test.randIntn
		ctl, cloud, region, err := s.jem.SelectController(ctx, test.cloud, test.region)
		if test.expectError != "" {
			c.Assert(err, gc.ErrorMatches, test.expectError)
			if test.expectErrorCause != nil {
				c.Assert(errgo.Cause(err), gc.Equals, test.expectErrorCause)
			}
			continue
		}
		c.Assert(err, gc.IsNil)
		c.Assert(ctl, jc.DeepEquals, test.expectController)
		c.Assert(cloud, gc.Equals, test.expectCloud)
		c.Assert(region, gc.Equals, test.expectRegion)
	}
}

func (s *jemSuite) addController(c *gc.C, path params.EntityPath) params.EntityPath {
	info := s.APIInfo(c)
	ctl := &mongodoc.Controller{
		Path:          path,
		HostPorts:     info.Addrs,
		CACert:        info.CACert,
		AdminUser:     info.Tag.Id(),
		AdminPassword: info.Password,
	}
	err := s.jem.DB.AddController(ctl)
	c.Assert(err, jc.ErrorIsNil)
	return path
}

func (s *jemSuite) bootstrapModel(c *gc.C, path params.EntityPath) (*apiconn.Conn, *mongodoc.Model) {
	ctlPath := s.addController(c, params.EntityPath{User: path.User, Name: "controller"})
	credPath := credentialPath("dummy", string(path.User), "cred")
	err := jem.UpdateCredential(s.jem.DB, &mongodoc.Credential{
		Path: credPath,
		Type: "empty",
	})
	c.Assert(err, jc.ErrorIsNil)
	conn, err := s.jem.OpenAPI(ctlPath)
	c.Assert(err, jc.ErrorIsNil)
	model, _, err := s.jem.CreateModel(conn, jem.CreateModelParams{
		Path:           path,
		ControllerPath: ctlPath,
		Credential:     "cred",
		Cloud:          "dummy",
	})
	c.Assert(err, jc.ErrorIsNil)
	return conn, model
}
