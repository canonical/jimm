// Copyright 2015 Canonical Ltd.

package jem_test

import (
	"context"
	"fmt"
	"time"

	"github.com/juju/clock/testclock"
	cloudapi "github.com/juju/juju/api/cloud"
	"github.com/juju/juju/api/controller"
	modelmanagerapi "github.com/juju/juju/api/modelmanager"
	jujuparams "github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/multiwatcher"
	jujujujutesting "github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
	jt "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"
	"gopkg.in/errgo.v1"
	"gopkg.in/juju/names.v2"
	"gopkg.in/mgo.v2/bson"

	"github.com/CanonicalLtd/jimm/internal/apiconn"
	"github.com/CanonicalLtd/jimm/internal/auth"
	"github.com/CanonicalLtd/jimm/internal/jem"
	"github.com/CanonicalLtd/jimm/internal/jemtest"
	"github.com/CanonicalLtd/jimm/internal/kubetest"
	"github.com/CanonicalLtd/jimm/internal/mgosession"
	"github.com/CanonicalLtd/jimm/internal/mongodoc"
	"github.com/CanonicalLtd/jimm/params"
)

type jemSuite struct {
	jemtest.JujuConnSuite
	pool        *jem.Pool
	sessionPool *mgosession.Pool
	jem         *jem.JEM

	suiteCleanups []func()
}

var _ = gc.Suite(&jemSuite{})

func (s *jemSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.sessionPool = mgosession.NewPool(context.TODO(), s.Session, 5)
	pool, err := jem.NewPool(context.TODO(), jem.Params{
		DB:              s.Session.DB("jem"),
		ControllerAdmin: "controller-admin",
		SessionPool:     s.sessionPool,
	})
	c.Assert(err, gc.Equals, nil)
	s.pool = pool
	s.jem = s.pool.JEM(context.TODO())
	s.PatchValue(&utils.OutgoingAccessAllowed, true)
}

func (s *jemSuite) TearDownTest(c *gc.C) {
	s.jem.Close()
	s.pool.Close()
	s.sessionPool.Close()
	s.JujuConnSuite.TearDownTest(c)
}

func (s *jemSuite) TestPoolRequiresControllerAdmin(c *gc.C) {
	pool, err := jem.NewPool(context.TODO(), jem.Params{
		DB: s.Session.DB("jem"),
	})
	c.Assert(err, gc.ErrorMatches, "no controller admin group specified")
	c.Assert(pool, gc.IsNil)
}

func (s *jemSuite) TestPoolDoesNotReuseDeadConnection(c *gc.C) {
	session := jt.NewProxiedSession(c)
	defer session.Close()
	sessionPool := mgosession.NewPool(context.TODO(), session.Session, 3)
	defer sessionPool.Close()
	pool, err := jem.NewPool(context.TODO(), jem.Params{
		DB:              session.DB("jem"),
		ControllerAdmin: "controller-admin",
		SessionPool:     sessionPool,
	})
	c.Assert(err, gc.Equals, nil)
	defer pool.Close()

	assertOK := func(j *jem.JEM) {
		_, err := j.DB.Model(testContext, params.EntityPath{"bob", "x"})
		c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)
	}
	assertBroken := func(j *jem.JEM) {
		_, err := j.DB.Model(testContext, params.EntityPath{"bob", "x"})
		c.Assert(err, gc.ErrorMatches, `cannot get model "bob/x": EOF`)
	}

	// Get a JEM instance and perform a single operation so that the session used by the
	// JEM instance obtains a mongo socket.
	c.Logf("make jem0")
	jem0 := pool.JEM(context.TODO())
	defer jem0.Close()
	assertOK(jem0)

	c.Logf("close connections")
	// Close all current connections to the mongo instance,
	// which should cause subsequent operations on jem1 to fail.
	session.CloseConns()

	// Get another JEM instance, which should be a new session,
	// so operations on it should not fail.
	c.Logf("make jem1")
	jem1 := pool.JEM(context.TODO())
	defer jem1.Close()
	assertOK(jem1)

	// Get another JEM instance which should clone the same session
	// used by jem0 because only two sessions are available.
	c.Logf("make jem2")
	jem2 := pool.JEM(context.TODO())
	defer jem2.Close()

	// Perform another operation on jem0, which should fail and
	// cause its session not to be reused.
	c.Logf("check jem0 is broken")
	assertBroken(jem0)

	// The jem1 connection should still be working because it
	// was created after the connections were broken.
	c.Logf("check jem1 is ok")
	assertOK(jem1)

	c.Logf("check jem2 is ok")
	// The jem2 connection should also be broken because it
	// reused the same sessions as jem0
	assertBroken(jem2)

	// Get another instance, which should reuse the jem3 connection
	// and work OK.
	c.Logf("make jem3")
	jem3 := pool.JEM(context.TODO())
	defer jem3.Close()
	assertOK(jem3)

	// When getting the next instance, we should see that the connection
	// that we would have used is broken and create another one.
	c.Logf("make jem4")
	jem4 := pool.JEM(context.TODO())
	defer jem4.Close()
	assertOK(jem4)
}

func (s *jemSuite) TestClone(c *gc.C) {
	j := s.jem.Clone()
	j.Close()
	_, err := s.jem.DB.Model(testContext, params.EntityPath{"bob", "x"})
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)
}

var createModelTests = []struct {
	about            string
	user             string
	params           jem.CreateModelParams
	expectCredential params.CredentialPath
	expectError      string
	expectErrorCause error
}{{
	about: "success",
	user:  "bob",
	params: jem.CreateModelParams{
		Path: params.EntityPath{"bob", ""},
		Credential: params.CredentialPath{
			Cloud:      "dummy",
			EntityPath: params.EntityPath{"bob", "cred1"},
		},
		Cloud: "dummy",
	},
}, {
	about: "success specified controller",
	user:  "bob",
	params: jem.CreateModelParams{
		Path:           params.EntityPath{"bob", ""},
		ControllerPath: params.EntityPath{"bob", "controller"},
		Credential: params.CredentialPath{
			Cloud:      "dummy",
			EntityPath: params.EntityPath{"bob", "cred1"},
		},
		Cloud: "dummy",
	},
}, {
	about: "success with region",
	user:  "bob",
	params: jem.CreateModelParams{
		Path: params.EntityPath{"bob", ""},
		Credential: params.CredentialPath{
			Cloud:      "dummy",
			EntityPath: params.EntityPath{"bob", "cred1"},
		},
		Cloud:  "dummy",
		Region: "dummy-region",
	},
}, {
	about: "unknown credential",
	user:  "bob",
	params: jem.CreateModelParams{
		Path: params.EntityPath{"bob", ""},
		Credential: params.CredentialPath{
			Cloud:      "dummy",
			EntityPath: params.EntityPath{"bob", "cred2"},
		},
		Cloud: "dummy",
	},
	expectError:      `credential "dummy/bob/cred2" not found`,
	expectErrorCause: params.ErrNotFound,
}, {
	about: "model exists",
	user:  "bob",
	params: jem.CreateModelParams{
		Path: params.EntityPath{"bob", "oldmodel"},
		Credential: params.CredentialPath{
			Cloud:      "dummy",
			EntityPath: params.EntityPath{"bob", "cred1"},
		},
		Cloud: "dummy",
	},
	expectError:      `already exists`,
	expectErrorCause: params.ErrAlreadyExists,
}, {
	about: "unrecognised region",
	user:  "bob",
	params: jem.CreateModelParams{
		Path: params.EntityPath{"bob", ""},
		Credential: params.CredentialPath{
			Cloud:      "dummy",
			EntityPath: params.EntityPath{"bob", "cred1"},
		},
		Cloud:  "dummy",
		Region: "not-a-region",
	},
	expectError: `cloud "dummy" region "not-a-region" not found`,
}, {
	about: "empty cloud credentials selects single choice",
	user:  "bob",
	params: jem.CreateModelParams{
		Path:  params.EntityPath{"bob", ""},
		Cloud: "dummy",
	},
	expectCredential: params.CredentialPath{
		Cloud:      "dummy",
		EntityPath: params.EntityPath{"bob", "cred1"},
	},
}, {
	about: "empty cloud credentials fails with more than one choice",
	user:  "alice",
	params: jem.CreateModelParams{
		Path:  params.EntityPath{"alice", ""},
		Cloud: "dummy",
	},
	expectError:      `more than one possible credential to use`,
	expectErrorCause: params.ErrAmbiguousChoice,
}, {
	about: "empty cloud credentials passed through if no credentials found",
	user:  "charlie",
	params: jem.CreateModelParams{
		Path:  params.EntityPath{"charlie", ""},
		Cloud: "dummy",
	},
}}

func (s *jemSuite) TestCreateModel(c *gc.C) {
	now := bson.Now()
	s.PatchValue(jem.WallClock, testclock.NewClock(now))
	ctlId := s.addController(c, params.EntityPath{"bob", "controller"})
	err := s.jem.DB.SetACL(testContext, s.jem.DB.Controllers(), ctlId, params.ACL{
		Read: []string{"everyone"},
	})
	c.Assert(err, gc.Equals, nil)
	// Bob has a single credential.
	err = jem.UpdateCredential(s.jem.DB, testContext, &mongodoc.Credential{
		Path: credentialPath("dummy", "bob", "cred1"),
		Type: "empty",
	})
	c.Assert(err, gc.Equals, nil)
	// Alice has two credentials.
	err = jem.UpdateCredential(s.jem.DB, testContext, &mongodoc.Credential{
		Path: credentialPath("dummy", "alice", "cred1"),
		Type: "empty",
	})
	c.Assert(err, gc.Equals, nil)
	err = jem.UpdateCredential(s.jem.DB, testContext, &mongodoc.Credential{
		Path: credentialPath("dummy", "alice", "cred2"),
		Type: "empty",
	})
	c.Assert(err, gc.Equals, nil)

	ctx := auth.ContextWithUser(testContext, "bob")
	// Create a model so that we can have a test case for an already-existing model
	_, err = s.jem.CreateModel(ctx, jem.CreateModelParams{
		Path:           params.EntityPath{"bob", "oldmodel"},
		ControllerPath: ctlId,
		Credential: params.CredentialPath{
			Cloud:      "dummy",
			EntityPath: params.EntityPath{"bob", "cred1"},
		},
		Cloud: "dummy",
	})
	c.Assert(err, gc.Equals, nil)
	for i, test := range createModelTests {
		c.Logf("test %d. %s", i, test.about)
		if test.params.Path.Name == "" {
			test.params.Path.Name = params.Name(fmt.Sprintf("test-%d", i))
		}
		m, err := s.jem.CreateModel(auth.ContextWithUser(testContext, test.user), test.params)
		if test.expectError != "" {
			c.Assert(err, gc.ErrorMatches, test.expectError)
			if test.expectErrorCause != nil {
				c.Assert(errgo.Cause(err), gc.Equals, test.expectErrorCause)
			}
			continue
		}
		c.Assert(err, gc.Equals, nil)
		c.Assert(m.Path, jc.DeepEquals, test.params.Path)
		c.Assert(m.UUID, gc.Not(gc.Equals), "")
		c.Assert(m.CreationTime.Equal(now), gc.Equals, true)
		c.Assert(m.Creator, gc.Equals, test.user)
		c.Assert(m.Cloud, gc.Equals, test.params.Cloud)
		c.Assert(m.CloudRegion, gc.Equals, "dummy-region")
		if !test.expectCredential.IsZero() {
			c.Assert(m.Credential, jc.DeepEquals, test.expectCredential)
		} else {
			c.Assert(m.Credential, jc.DeepEquals, test.params.Credential)
		}
		c.Assert(m.DefaultSeries, gc.Not(gc.Equals), "")
		c.Assert(m.Life(), gc.Equals, "alive")
	}
}

func (s *jemSuite) TestCreateModelWithPartiallyCreatedModel(c *gc.C) {
	now := bson.Now()
	s.PatchValue(jem.WallClock, testclock.NewClock(now))
	ctlId := s.addController(c, params.EntityPath{"bob", "controller"})
	err := s.jem.DB.SetACL(testContext, s.jem.DB.Controllers(), ctlId, params.ACL{
		Read: []string{"everyone"},
	})
	c.Assert(err, gc.Equals, nil)
	// Bob has a single credential.
	err = jem.UpdateCredential(s.jem.DB, testContext, &mongodoc.Credential{
		Path: credentialPath("dummy", "bob", "cred1"),
		Type: "empty",
	})
	ctx := auth.ContextWithUser(testContext, "bob")
	// Create a partial model in the database.
	err = s.jem.DB.AddModel(ctx, &mongodoc.Model{
		Path:         params.EntityPath{"bob", "oldmodel"},
		Controller:   ctlId,
		CreationTime: now,
		Creator:      "bob",
		Credential:   credentialPath("dummy", "bob", "cred1"),
	})
	c.Assert(err, gc.Equals, nil)
	// Create a new model
	_, err = s.jem.CreateModel(ctx, jem.CreateModelParams{
		Path:           params.EntityPath{"bob", "model"},
		ControllerPath: ctlId,
		Credential: params.CredentialPath{
			Cloud:      "dummy",
			EntityPath: params.EntityPath{"bob", "cred1"},
		},
		Cloud: "dummy",
	})
	c.Assert(err, gc.Equals, nil)
}

func (s *jemSuite) TestCreateModelWithExistingModelInControllerOnly(c *gc.C) {
	// Create a model and then delete its entry in the JIMM database
	// as if the controller model had been created but something
	// had failed in CreateModel after that.
	model := s.bootstrapModel(c, params.EntityPath{User: "bob", Name: "model"})
	ctx := auth.ContextWithUser(testContext, string(model.Path.User))
	err := s.jem.DB.DeleteModel(ctx, model.Path)
	c.Assert(err, gc.Equals, nil)

	// Now try to create the model again.
	_, err = s.jem.CreateModel(ctx, jem.CreateModelParams{
		Path:           model.Path,
		ControllerPath: model.Controller,
		Credential: params.CredentialPath{
			Cloud:      "dummy",
			EntityPath: params.EntityPath{"bob", "cred"},
		},
		Cloud: "dummy",
	})
	c.Assert(err, gc.ErrorMatches, `cannot find suitable controller`)
}

func (s *jemSuite) TestCreateModelWithDeprecatedController(c *gc.C) {
	ctlId := s.addController(c, params.EntityPath{"bob", "controller"})
	err := s.jem.DB.SetACL(testContext, s.jem.DB.Controllers(), ctlId, params.ACL{
		Read: []string{"everyone"},
	})
	c.Assert(err, gc.Equals, nil)
	ctx := auth.ContextWithUser(testContext, "bob")
	// Sanity check that we can create the model while the controller is not deprecated.
	_, err = s.jem.CreateModel(ctx, jem.CreateModelParams{
		Path:   params.EntityPath{"bob", "model1"},
		Cloud:  "dummy",
		Region: "dummy-region",
	})
	c.Assert(err, gc.Equals, nil)

	// Deprecate it and make sure it's not chosen again.
	err = s.jem.DB.SetControllerDeprecated(testContext, ctlId, true)
	c.Assert(err, gc.Equals, nil)

	_, err = s.jem.CreateModel(ctx, jem.CreateModelParams{
		Path:   params.EntityPath{"bob", "model2"},
		Cloud:  "dummy",
		Region: "dummy-region",
	})
	c.Assert(err, gc.ErrorMatches, `cannot find suitable controller`)
}

func (s *jemSuite) TestCreateModelWithMultipleControllers(c *gc.C) {
	s.PatchValue(jem.Shuffle, func(int, func(int, int)) {})
	ctlId := s.addController(c, params.EntityPath{"bob", "controller"})
	err := s.jem.DB.SetACL(testContext, s.jem.DB.Controllers(), ctlId, params.ACL{
		Read: []string{"everyone"},
	})
	c.Assert(err, gc.Equals, nil)
	ctl2Id := s.addController(c, params.EntityPath{"bob", "controller2"})
	err = s.jem.DB.SetACL(testContext, s.jem.DB.Controllers(), ctl2Id, params.ACL{
		Read: []string{"everyone"},
	})
	c.Assert(err, gc.Equals, nil)
	ctx := auth.ContextWithUser(testContext, "bob")
	// Deprecate the first controller.
	err = s.jem.DB.SetControllerDeprecated(testContext, ctlId, true)
	c.Assert(err, gc.Equals, nil)

	m, err := s.jem.CreateModel(ctx, jem.CreateModelParams{
		Path:   params.EntityPath{"bob", "model2"},
		Cloud:  "dummy",
		Region: "dummy-region",
	})
	c.Assert(err, gc.Equals, nil)
	c.Assert(m.Controller, jc.DeepEquals, ctl2Id)
}

func (s *jemSuite) TestGrantModelWrite(c *gc.C) {
	model := s.bootstrapModel(c, params.EntityPath{User: "bob", Name: "model"})
	conn, err := s.jem.OpenAPI(testContext, model.Controller)
	c.Assert(err, gc.Equals, nil)
	defer conn.Close()
	err = s.jem.GrantModel(testContext, conn, model, "alice", "write")
	c.Assert(err, gc.Equals, nil)
	model1, err := s.jem.DB.Model(testContext, model.Path)
	c.Assert(err, gc.Equals, nil)
	c.Assert(model1.ACL, jc.DeepEquals, params.ACL{
		Read:  []string{"alice"},
		Write: []string{"alice"},
	})
}

func (s *jemSuite) TestGrantModelRead(c *gc.C) {
	model := s.bootstrapModel(c, params.EntityPath{User: "bob", Name: "model"})
	conn, err := s.jem.OpenAPI(testContext, model.Controller)
	c.Assert(err, gc.Equals, nil)
	defer conn.Close()
	err = s.jem.GrantModel(testContext, conn, model, "alice", "read")
	c.Assert(err, gc.Equals, nil)
	model1, err := s.jem.DB.Model(testContext, model.Path)
	c.Assert(err, gc.Equals, nil)
	c.Assert(model1.ACL, jc.DeepEquals, params.ACL{
		Read: []string{"alice"},
	})
}

func (s *jemSuite) TestGrantModelBadLevel(c *gc.C) {
	model := s.bootstrapModel(c, params.EntityPath{User: "bob", Name: "model"})
	conn, err := s.jem.OpenAPI(testContext, model.Controller)
	c.Assert(err, gc.Equals, nil)
	defer conn.Close()
	err = s.jem.GrantModel(testContext, conn, model, "alice", "superpowers")
	c.Assert(err, gc.ErrorMatches, `"superpowers" model access not valid`)
	model1, err := s.jem.DB.Model(testContext, model.Path)
	c.Assert(err, gc.Equals, nil)
	c.Assert(model1.ACL, jc.DeepEquals, params.ACL{})
}

func (s *jemSuite) TestRevokeModel(c *gc.C) {
	model := s.bootstrapModel(c, params.EntityPath{User: "bob", Name: "model"})
	conn, err := s.jem.OpenAPI(testContext, model.Controller)
	c.Assert(err, gc.Equals, nil)
	defer conn.Close()
	err = s.jem.GrantModel(testContext, conn, model, "alice", "admin")
	c.Assert(err, gc.Equals, nil)
	model1, err := s.jem.DB.Model(testContext, model.Path)
	c.Assert(err, gc.Equals, nil)
	c.Assert(model1.ACL, jc.DeepEquals, params.ACL{
		Read:  []string{"alice"},
		Write: []string{"alice"},
		Admin: []string{"alice"},
	})
	err = s.jem.RevokeModel(testContext, conn, model, "alice", "read")
	c.Assert(err, gc.Equals, nil)
	model1, err = s.jem.DB.Model(testContext, model.Path)
	c.Assert(err, gc.Equals, nil)
	c.Assert(model1.ACL, jc.DeepEquals, params.ACL{})
}

func (s *jemSuite) TestRevokeModelAdmin(c *gc.C) {
	model := s.bootstrapModel(c, params.EntityPath{User: "bob", Name: "model"})
	conn, err := s.jem.OpenAPI(testContext, model.Controller)
	c.Assert(err, gc.Equals, nil)
	defer conn.Close()
	err = s.jem.GrantModel(testContext, conn, model, "alice", "admin")
	c.Assert(err, gc.Equals, nil)
	model1, err := s.jem.DB.Model(testContext, model.Path)
	c.Assert(err, gc.Equals, nil)
	c.Assert(model1.ACL, jc.DeepEquals, params.ACL{
		Read:  []string{"alice"},
		Write: []string{"alice"},
		Admin: []string{"alice"},
	})
	err = s.jem.RevokeModel(testContext, conn, model, "alice", "admin")
	c.Assert(err, gc.Equals, nil)
	model1, err = s.jem.DB.Model(testContext, model.Path)
	c.Assert(err, gc.Equals, nil)
	c.Assert(model1.ACL, jc.DeepEquals, params.ACL{
		Read:  []string{"alice"},
		Write: []string{"alice"},
	})
}

func (s *jemSuite) TestRevokeModelWrite(c *gc.C) {
	model := s.bootstrapModel(c, params.EntityPath{User: "bob", Name: "model"})
	conn, err := s.jem.OpenAPI(testContext, model.Controller)
	c.Assert(err, gc.Equals, nil)
	defer conn.Close()
	err = s.jem.GrantModel(testContext, conn, model, "alice", "admin")
	c.Assert(err, gc.Equals, nil)
	model1, err := s.jem.DB.Model(testContext, model.Path)
	c.Assert(err, gc.Equals, nil)
	c.Assert(model1.ACL, jc.DeepEquals, params.ACL{
		Read:  []string{"alice"},
		Write: []string{"alice"},
		Admin: []string{"alice"},
	})
	err = s.jem.RevokeModel(testContext, conn, model, "alice", "write")
	c.Assert(err, gc.Equals, nil)
	model1, err = s.jem.DB.Model(testContext, model.Path)
	c.Assert(err, gc.Equals, nil)
	c.Assert(model1.ACL, jc.DeepEquals, params.ACL{
		Read: []string{"alice"},
	})
}

func (s *jemSuite) TestRevokeModelControllerFailure(c *gc.C) {
	model := s.bootstrapModel(c, params.EntityPath{User: "bob", Name: "model"})
	conn, err := s.jem.OpenAPI(testContext, model.Controller)
	c.Assert(err, gc.Equals, nil)
	defer conn.Close()
	err = s.jem.GrantModel(testContext, conn, model, "alice", "write")
	c.Assert(err, gc.Equals, nil)
	model1, err := s.jem.DB.Model(testContext, model.Path)
	c.Assert(err, gc.Equals, nil)
	c.Assert(model1.ACL, jc.DeepEquals, params.ACL{
		Read:  []string{"alice"},
		Write: []string{"alice"},
	})
	err = s.jem.RevokeModel(testContext, conn, model, "alice", "superpowers")
	c.Assert(err, gc.ErrorMatches, `"superpowers" model access not valid`)
	model1, err = s.jem.DB.Model(testContext, model.Path)
	c.Assert(err, gc.Equals, nil)
	c.Assert(model1.ACL, jc.DeepEquals, params.ACL{
		Read:  []string{"alice"},
		Write: []string{"alice"},
	})
}

func (s *jemSuite) TestDestroyModel(c *gc.C) {
	model := s.bootstrapModel(c, params.EntityPath{User: "bob", Name: "model"})
	conn, err := s.jem.OpenAPI(testContext, model.Controller)
	c.Assert(err, gc.Equals, nil)
	defer conn.Close()

	// Sanity check the model exists
	client := modelmanagerapi.NewClient(conn)
	_, err = client.ModelInfo([]names.ModelTag{
		names.NewModelTag(model.UUID),
	})
	c.Assert(err, gc.Equals, nil)

	ch := waitForDestruction(conn, c, model.UUID)

	err = s.jem.DestroyModel(testContext, conn, model, nil)
	c.Assert(err, gc.Equals, nil)

	select {
	case <-ch:
	case <-time.After(jujujujutesting.LongWait):
		c.Fatalf("model not destroyed")
	}

	// Check the model is dying.
	_, err = s.jem.DB.Model(testContext, model.Path)
	c.Assert(err, gc.Equals, nil)

	// Check that it can be destroyed twice.
	err = s.jem.DestroyModel(testContext, conn, model, nil)
	c.Assert(err, gc.Equals, nil)

	// Check the model is still dying.
	_, err = s.jem.DB.Model(testContext, model.Path)
	c.Assert(err, gc.Equals, nil)
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

func (s *jemSuite) TestDestroyModelWithStorage(c *gc.C) {
	model := s.bootstrapModel(c, params.EntityPath{User: "bob", Name: "model"})
	conn, err := s.jem.OpenAPI(testContext, model.Controller)
	c.Assert(err, gc.Equals, nil)
	defer conn.Close()

	// Sanity check the model exists
	tag := names.NewModelTag(model.UUID)
	client := modelmanagerapi.NewClient(conn)
	_, err = client.ModelInfo([]names.ModelTag{tag})
	c.Assert(err, gc.Equals, nil)

	modelState, err := s.StatePool.Get(model.UUID)
	c.Assert(err, gc.Equals, nil)
	defer modelState.Release()
	f := factory.NewFactory(modelState.State, s.StatePool)
	f.MakeUnit(c, &factory.UnitParams{
		Application: f.MakeApplication(c, &factory.ApplicationParams{
			Charm: f.MakeCharm(c, &factory.CharmParams{
				Name: "storage-block",
			}),
			Storage: map[string]state.StorageConstraints{
				"data": {Pool: "modelscoped"},
			},
		}),
	})

	err = s.jem.DestroyModel(testContext, conn, model, nil)
	c.Assert(err, jc.Satisfies, jujuparams.IsCodeHasPersistentStorage)
}

func (s *jemSuite) TestUpdateCredential(c *gc.C) {
	ctlPath := s.addController(c, params.EntityPath{User: "bob", Name: "controller"})
	credPath := credentialPath("dummy", "bob", "cred")
	cred := &mongodoc.Credential{
		Path: credPath,
		Type: "empty",
	}
	err := jem.UpdateCredential(s.jem.DB, testContext, cred)
	c.Assert(err, gc.Equals, nil)
	conn, err := s.jem.OpenAPI(testContext, ctlPath)
	c.Assert(err, gc.Equals, nil)
	defer conn.Close()

	err = jem.UpdateControllerCredential(s.jem, testContext, conn, ctlPath, cred, nil)
	c.Assert(err, gc.Equals, nil)
	err = jem.CredentialAddController(s.jem.DB, testContext, credPath, ctlPath)
	c.Assert(err, gc.Equals, nil)

	// Sanity check it was deployed
	client := cloudapi.NewClient(conn)
	credTag := names.NewCloudCredentialTag("dummy/bob@external/cred")
	creds, err := client.Credentials(credTag)
	c.Assert(err, gc.Equals, nil)
	c.Assert(creds, jc.DeepEquals, []jujuparams.CloudCredentialResult{{
		Result: &jujuparams.CloudCredential{
			AuthType: "empty",
		},
	}})

	_, err = s.jem.UpdateCredential(testContext, &mongodoc.Credential{
		Path: credPath,
		Type: "userpass",
		Attributes: map[string]string{
			"username": "cloud-user",
			"password": "cloud-pass",
		},
	}, jem.CredentialUpdate)
	c.Assert(err, gc.Equals, nil)

	// check it was updated on the controller.
	creds, err = client.Credentials(credTag)
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
	}})

	// Revoke the credential
	err = s.jem.RevokeCredential(testContext, credPath, jem.CredentialUpdate)
	c.Assert(err, gc.Equals, nil)

	// check it was removed on the controller.
	creds, err = client.Credentials(credTag)
	c.Assert(err, gc.Equals, nil)
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
	err := jem.UpdateCredential(s.jem.DB, testContext, cred)
	c.Assert(err, gc.Equals, nil)

	err = jem.SetCredentialUpdates(s.jem.DB, testContext, []params.EntityPath{ctlPath}, credPath)
	c.Assert(err, gc.Equals, nil)

	err = s.jem.ControllerUpdateCredentials(testContext, ctlPath)
	c.Assert(err, gc.Equals, nil)

	// check it was updated on the controller.
	conn, err := s.jem.OpenAPI(testContext, ctlPath)
	c.Assert(err, gc.Equals, nil)
	defer conn.Close()

	client := cloudapi.NewClient(conn)
	creds, err := client.Credentials(credTag)
	c.Assert(err, gc.Equals, nil)
	c.Assert(creds, jc.DeepEquals, []jujuparams.CloudCredentialResult{{
		Result: &jujuparams.CloudCredential{
			AuthType:   "empty",
			Attributes: nil,
			Redacted:   nil,
		},
	}})
}

func (s *jemSuite) TestDoControllers(c *gc.C) {
	var testControllers = []mongodoc.Controller{{
		Path: params.EntityPath{
			User: params.User("alice"),
			Name: "aws-eu-west-1",
		},
		ACL: params.ACL{
			Read: []string{"bob"},
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
		Public: true,
	}, {
		Path: params.EntityPath{
			User: params.User("bob"),
			Name: "aws-us-east-1",
		},
		ACL: params.ACL{
			Read: []string{"everyone"},
		},
		Public: true,
	}, {
		Path: params.EntityPath{
			User: params.User("alice"),
			Name: "aws-eu-west-2",
		},
		ACL: params.ACL{
			Read: []string{"someoneelse"},
		},
		Public: true,
	}, {
		Path: params.EntityPath{
			User: params.User("alice"),
			Name: "aws-eu-west-3",
		},
		UnavailableSince: time.Now(),
		ACL: params.ACL{
			Read: []string{"everyone"},
		},
		Public: true,
	}, {
		Path: params.EntityPath{
			User: params.User("alice"),
			Name: "aws-eu-west-4",
		},
		Public: false,
		ACL: params.ACL{
			Read: []string{"everyone"},
		},
	}}
	for i := range testControllers {
		err := s.jem.DB.AddController(testContext, &testControllers[i])
		c.Assert(err, gc.Equals, nil)
	}
	ctx := auth.ContextWithUser(testContext, "bob", "bob-group")
	var controllers []string
	err := s.jem.DoControllers(ctx, func(c *mongodoc.Controller) error {
		controllers = append(controllers, c.Id)
		return nil
	})
	c.Assert(err, gc.Equals, nil)
	c.Assert(controllers, gc.DeepEquals, []string{
		"alice/aws-eu-west-1",
		"alice/aws-us-east-1",
		"bob/aws-us-east-1",
	})
}

func (s *jemSuite) TestSelectController(c *gc.C) {
	var testControllers = []mongodoc.Controller{{
		Path: params.EntityPath{
			User: params.User("alice"),
			Name: "aws-eu-west-1",
		},
		ACL: params.ACL{
			Read: []string{"bob"},
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
		Public: true,
	}, {
		Path: params.EntityPath{
			User: params.User("bob"),
			Name: "aws-us-east-1",
		},
		ACL: params.ACL{
			Read: []string{"everyone"},
		},
		Public: true,
	}, {
		Path: params.EntityPath{
			User: params.User("alice"),
			Name: "aws-eu-west-2",
		},
		ACL: params.ACL{
			Read: []string{"someoneelse"},
		},
		Public: true,
	}, {}}
	for i := range testControllers {
		err := s.jem.DB.AddController(testContext, &testControllers[i])
		c.Assert(err, gc.Equals, nil)
	}
	ctx := auth.ContextWithUser(testContext, "bob", "bob-group")
	called := false
	s.PatchValue(jem.RandIntn, func(n int) int {
		called = true
		c.Assert(n, gc.Equals, 3)
		return 1
	})
	ctl, err := jem.SelectRandomController(s.jem, ctx)
	c.Assert(err, gc.Equals, nil)
	c.Assert(called, gc.Equals, true)
	c.Assert(ctl, jc.DeepEquals, params.EntityPath{"alice", "aws-us-east-1"})
}

var controllerTests = []struct {
	path             params.EntityPath
	expectErrorCause error
}{{
	path: params.EntityPath{"bob", "controller"},
}, {
	path: params.EntityPath{"bob-group", "controller"},
}, {
	path:             params.EntityPath{"alice", "controller"},
	expectErrorCause: params.ErrUnauthorized,
}, {
	path:             params.EntityPath{"bob", "controller2"},
	expectErrorCause: params.ErrNotFound,
}, {
	path:             params.EntityPath{"bob-group", "controller2"},
	expectErrorCause: params.ErrNotFound,
}, {
	path:             params.EntityPath{"alice", "controller2"},
	expectErrorCause: params.ErrUnauthorized,
}}

func (s *jemSuite) TestController(c *gc.C) {
	s.addController(c, params.EntityPath{"alice", "controller"})
	s.addController(c, params.EntityPath{"bob", "controller"})
	s.addController(c, params.EntityPath{"bob-group", "controller"})
	ctx := auth.ContextWithUser(testContext, "bob", "bob-group")

	for i, test := range controllerTests {
		c.Logf("tes %d. %s", i, test.path)
		ctl, err := s.jem.Controller(ctx, test.path)
		if test.expectErrorCause != nil {
			c.Assert(errgo.Cause(err), gc.Equals, test.expectErrorCause)
			continue
		}
		c.Assert(err, gc.Equals, nil)
		c.Assert(ctl.Path, jc.DeepEquals, test.path)
	}
}

var credentialTests = []struct {
	path             params.CredentialPath
	expectErrorCause error
}{{
	path: params.CredentialPath{
		Cloud:      "dummy",
		EntityPath: params.EntityPath{"bob", "credential"},
	},
}, {
	path: params.CredentialPath{
		Cloud:      "dummy",
		EntityPath: params.EntityPath{"bob-group", "credential"},
	},
}, {
	path: params.CredentialPath{
		Cloud:      "dummy",
		EntityPath: params.EntityPath{"alice", "credential"},
	},
	expectErrorCause: params.ErrUnauthorized,
}, {
	path: params.CredentialPath{
		Cloud:      "dummy",
		EntityPath: params.EntityPath{"bob", "credential2"},
	},
	expectErrorCause: params.ErrNotFound,
}, {
	path: params.CredentialPath{
		Cloud:      "dummy",
		EntityPath: params.EntityPath{"bob-group", "credential2"},
	},
	expectErrorCause: params.ErrNotFound,
}, {
	path: params.CredentialPath{
		Cloud:      "dummy",
		EntityPath: params.EntityPath{"alice", "credential2"},
	},
	expectErrorCause: params.ErrUnauthorized,
}}

func (s *jemSuite) TestCredential(c *gc.C) {
	creds := []mongodoc.Credential{{
		Path: params.CredentialPath{
			Cloud:      "dummy",
			EntityPath: params.EntityPath{"alice", "credential"},
		},
	}, {
		Path: params.CredentialPath{
			Cloud:      "dummy",
			EntityPath: params.EntityPath{"bob", "credential"},
		},
	}, {
		Path: params.CredentialPath{
			Cloud:      "dummy",
			EntityPath: params.EntityPath{"bob-group", "credential"},
		},
	}}
	for _, cred := range creds {
		cred.Id = cred.Path.String()
		err := jem.UpdateCredential(s.jem.DB, testContext, &cred)
		c.Assert(err, gc.Equals, nil)
	}
	ctx := auth.ContextWithUser(testContext, "bob", "bob-group")

	for i, test := range credentialTests {
		c.Logf("tes %d. %s", i, test.path)
		ctl, err := s.jem.Credential(ctx, test.path)
		if test.expectErrorCause != nil {
			c.Assert(errgo.Cause(err), gc.Equals, test.expectErrorCause)
			continue
		}
		c.Assert(err, gc.Equals, nil)
		c.Assert(ctl.Path, jc.DeepEquals, test.path)
	}
}

func (s *jemSuite) TestUserTag(c *gc.C) {
	c.Assert(jem.UserTag(params.User("alice")).String(), gc.Equals, "user-alice@external")
	c.Assert(jem.UserTag(params.User("alice@domain")).String(), gc.Equals, "user-alice@domain")
}

var earliestControllerVersionTests = []struct {
	about       string
	controllers []mongodoc.Controller
	expect      version.Number
}{{
	about:  "no controllers",
	expect: version.Number{},
}, {
	about: "one controller",
	controllers: []mongodoc.Controller{{
		Path:    params.EntityPath{"bob", "c1"},
		Public:  true,
		Version: &version.Number{Minor: 1},
		ACL: params.ACL{
			Read: []string{"everyone"},
		},
	}},
	expect: version.Number{Minor: 1},
}, {
	about: "multiple controllers",
	controllers: []mongodoc.Controller{{
		Path:    params.EntityPath{"bob", "c1"},
		Public:  true,
		Version: &version.Number{Minor: 1},
		ACL: params.ACL{
			Read: []string{"everyone"},
		},
	}, {
		Path:    params.EntityPath{"bob", "c2"},
		Public:  true,
		Version: &version.Number{Minor: 2},
		ACL: params.ACL{
			Read: []string{"everyone"},
		},
	}, {
		Path:    params.EntityPath{"bob", "c3"},
		Public:  true,
		Version: &version.Number{Minor: 3},
		ACL: params.ACL{
			Read: []string{"everyone"},
		},
	}},
	expect: version.Number{Minor: 1},
}, {
	about: "non-public controllers ignored",
	controllers: []mongodoc.Controller{{
		Path:    params.EntityPath{"bob", "c1"},
		Version: &version.Number{Minor: 1},
	}, {
		Path:   params.EntityPath{"bob", "c2"},
		Public: true,
		ACL: params.ACL{
			Read: []string{"everyone"},
		},
		Version: &version.Number{Minor: 2},
	}},
	expect: version.Number{Minor: 2},
}}

func (s *jemSuite) TestEarliestControllerVersion(c *gc.C) {
	ctx := auth.ContextWithUser(testContext, "someone")
	for i, test := range earliestControllerVersionTests {
		c.Logf("test %d: %v", i, test.about)
		_, err := s.jem.DB.Controllers().RemoveAll(nil)
		c.Assert(err, gc.Equals, nil)
		for _, ctl := range test.controllers {
			err := s.jem.DB.AddController(ctx, &ctl)
			c.Assert(err, gc.Equals, nil)
		}
		v, err := s.jem.EarliestControllerVersion(ctx)
		c.Assert(err, gc.Equals, nil)
		c.Assert(v, jc.DeepEquals, test.expect)
	}
}

func (s *jemSuite) TestCloudCredentialTag(c *gc.C) {
	cp1 := params.CredentialPath{
		Cloud: "dummy",
		EntityPath: params.EntityPath{
			User: "alice",
			Name: "cred",
		},
	}
	cp2 := params.CredentialPath{
		Cloud: "dummy",
		EntityPath: params.EntityPath{
			User: "alice@domain",
			Name: "cred",
		},
	}
	c.Assert(jem.CloudCredentialTag(cp1).String(), gc.Equals, "cloudcred-dummy_alice@external_cred")
	c.Assert(jem.CloudCredentialTag(cp2).String(), gc.Equals, "cloudcred-dummy_alice@domain_cred")
}

func (s *jemSuite) TestUpdateMachineInfo(c *gc.C) {
	m := s.bootstrapModel(c, params.EntityPath{"bob", "model-1"})
	ctlPath := params.EntityPath{"bob", "controller"}

	err := s.jem.UpdateMachineInfo(testContext, ctlPath, &multiwatcher.MachineInfo{
		ModelUUID: m.UUID,
		Id:        "0",
		Series:    "quantal",
	})
	c.Assert(err, gc.Equals, nil)
	err = s.jem.UpdateMachineInfo(testContext, ctlPath, &multiwatcher.MachineInfo{
		ModelUUID: m.UUID,
		Id:        "1",
		Series:    "precise",
	})
	c.Assert(err, gc.Equals, nil)

	docs, err := s.jem.DB.MachinesForModel(testContext, m.UUID)
	c.Assert(err, gc.Equals, nil)
	for i := range docs {
		cleanMachineDoc(&docs[i])
	}
	c.Assert(docs, jc.DeepEquals, []mongodoc.Machine{{
		Id:         ctlPath.String() + " " + m.UUID + " 0",
		Controller: ctlPath.String(),
		Cloud:      "dummy",
		Region:     "dummy-region",
		Info: &multiwatcher.MachineInfo{
			ModelUUID: m.UUID,
			Id:        "0",
			Series:    "quantal",
			Config:    map[string]interface{}{},
		},
	}, {
		Id:         ctlPath.String() + " " + m.UUID + " 1",
		Controller: ctlPath.String(),
		Cloud:      "dummy",
		Region:     "dummy-region",
		Info: &multiwatcher.MachineInfo{
			ModelUUID: m.UUID,
			Id:        "1",
			Series:    "precise",
			Config:    map[string]interface{}{},
		},
	}})

	// Check that we can update one of the documents.
	err = s.jem.UpdateMachineInfo(testContext, ctlPath, &multiwatcher.MachineInfo{
		ModelUUID: m.UUID,
		Id:        "0",
		Series:    "quantal",
		Life:      "dying",
	})
	c.Assert(err, gc.Equals, nil)

	// Check that setting a machine dead removes it.
	err = s.jem.UpdateMachineInfo(testContext, ctlPath, &multiwatcher.MachineInfo{
		ModelUUID: m.UUID,
		Id:        "1",
		Series:    "precise",
		Life:      "dead",
	})
	c.Assert(err, gc.Equals, nil)

	docs, err = s.jem.DB.MachinesForModel(testContext, m.UUID)
	c.Assert(err, gc.Equals, nil)
	for i := range docs {
		cleanMachineDoc(&docs[i])
	}
	c.Assert(docs, jc.DeepEquals, []mongodoc.Machine{{
		Id:         ctlPath.String() + " " + m.UUID + " 0",
		Controller: ctlPath.String(),
		Cloud:      "dummy",
		Region:     "dummy-region",
		Info: &multiwatcher.MachineInfo{
			ModelUUID: m.UUID,
			Id:        "0",
			Series:    "quantal",
			Config:    map[string]interface{}{},
			Life:      "dying",
		},
	}})
}

func (s *jemSuite) TestUpdateMachineUnknownModel(c *gc.C) {
	ctlPath := params.EntityPath{"bob", "controller"}

	err := s.jem.UpdateMachineInfo(testContext, ctlPath, &multiwatcher.MachineInfo{
		ModelUUID: "no-such-uuid",
		Id:        "1",
		Series:    "precise",
	})
	c.Assert(err, gc.Equals, nil)
}

func (s *jemSuite) TestUpdateMachineIncorrectController(c *gc.C) {
	m := s.bootstrapModel(c, params.EntityPath{"bob", "model-1"})
	ctlPath := params.EntityPath{"bob", "controller2"}

	err := s.jem.UpdateMachineInfo(testContext, ctlPath, &multiwatcher.MachineInfo{
		ModelUUID: m.UUID,
		Id:        "1",
		Series:    "precise",
	})
	c.Assert(err, gc.Equals, nil)
}

func (s *jemSuite) TestUpdateApplicationInfo(c *gc.C) {
	m := s.bootstrapModel(c, params.EntityPath{"bob", "model-1"})
	ctlPath := params.EntityPath{"bob", "controller"}

	err := s.jem.UpdateApplicationInfo(testContext, ctlPath, &multiwatcher.ApplicationInfo{
		ModelUUID: m.UUID,
		Name:      "0",
	})
	c.Assert(err, gc.Equals, nil)
	err = s.jem.UpdateApplicationInfo(testContext, ctlPath, &multiwatcher.ApplicationInfo{
		ModelUUID: m.UUID,
		Name:      "1",
	})
	c.Assert(err, gc.Equals, nil)

	docs, err := s.jem.DB.ApplicationsForModel(testContext, m.UUID)
	c.Assert(err, gc.Equals, nil)
	for i := range docs {
		cleanApplicationDoc(&docs[i])
	}
	c.Assert(docs, jc.DeepEquals, []mongodoc.Application{{
		Id:         ctlPath.String() + " " + m.UUID + " 0",
		Controller: ctlPath.String(),
		Cloud:      "dummy",
		Region:     "dummy-region",
		Info: &mongodoc.ApplicationInfo{
			ModelUUID: m.UUID,
			Name:      "0",
		},
	}, {
		Id:         ctlPath.String() + " " + m.UUID + " 1",
		Controller: ctlPath.String(),
		Cloud:      "dummy",
		Region:     "dummy-region",
		Info: &mongodoc.ApplicationInfo{
			ModelUUID: m.UUID,
			Name:      "1",
		},
	}})

	// Check that we can update one of the documents.
	err = s.jem.UpdateApplicationInfo(testContext, ctlPath, &multiwatcher.ApplicationInfo{
		ModelUUID: m.UUID,
		Name:      "0",
		Life:      "dying",
	})
	c.Assert(err, gc.Equals, nil)

	// Check that setting an application dead removes it.
	err = s.jem.UpdateApplicationInfo(testContext, ctlPath, &multiwatcher.ApplicationInfo{
		ModelUUID: m.UUID,
		Name:      "1",
		Life:      "dead",
	})
	c.Assert(err, gc.Equals, nil)

	docs, err = s.jem.DB.ApplicationsForModel(testContext, m.UUID)
	c.Assert(err, gc.Equals, nil)
	for i := range docs {
		cleanApplicationDoc(&docs[i])
	}
	c.Assert(docs, jc.DeepEquals, []mongodoc.Application{{
		Id:         ctlPath.String() + " " + m.UUID + " 0",
		Controller: ctlPath.String(),
		Cloud:      "dummy",
		Region:     "dummy-region",
		Info: &mongodoc.ApplicationInfo{
			ModelUUID: m.UUID,
			Name:      "0",
			Life:      "dying",
		},
	}})
}

func (s *jemSuite) TestUpdateApplicationUnknownModel(c *gc.C) {
	m := s.bootstrapModel(c, params.EntityPath{"bob", "model-1"})
	ctlPath := params.EntityPath{"bob", "controller"}

	err := s.jem.UpdateApplicationInfo(testContext, ctlPath, &multiwatcher.ApplicationInfo{
		ModelUUID: m.UUID,
		Name:      "1",
	})
	c.Assert(err, gc.Equals, nil)
}

func (s *jemSuite) TestCreateCloud(c *gc.C) {
	ctlPath := params.EntityPath{"bob", "foo"}
	s.addController(c, ctlPath)
	ctx := auth.ContextWithUser(testContext, "bob", "bob-group")
	err := s.jem.CreateCloud(ctx, mongodoc.CloudRegion{
		Cloud:            "test-cloud",
		ProviderType:     "kubernetes",
		AuthTypes:        []string{"certificate"},
		Endpoint:         "https://1.2.3.4:5678",
		IdentityEndpoint: "https://1.2.3.4:5679",
		StorageEndpoint:  "https://1.2.3.4:5680",
		CACertificates:   []string{"This is a CA Certficiate (honest)"},
		ACL: params.ACL{
			Read:  []string{"bob"},
			Write: []string{"bob"},
			Admin: []string{"bob"},
		},
	}, nil)
	c.Assert(err, gc.Equals, nil)

	var docs []mongodoc.CloudRegion
	err = s.jem.DB.CloudRegions().Find(bson.D{{"cloud", "test-cloud"}}).Sort("_id").All(&docs)
	c.Assert(err, gc.Equals, nil)
	c.Assert(docs, jc.DeepEquals, []mongodoc.CloudRegion{{
		Id:                   "test-cloud/",
		Cloud:                "test-cloud",
		ProviderType:         "kubernetes",
		AuthTypes:            []string{"certificate"},
		Endpoint:             "https://1.2.3.4:5678",
		IdentityEndpoint:     "https://1.2.3.4:5679",
		StorageEndpoint:      "https://1.2.3.4:5680",
		CACertificates:       []string{"This is a CA Certficiate (honest)"},
		PrimaryControllers:   []params.EntityPath{{User: "bob", Name: "foo"}},
		SecondaryControllers: []params.EntityPath{},
		ACL: params.ACL{
			Read:  []string{"bob"},
			Write: []string{"bob"},
			Admin: []string{"bob"},
		},
	}})
}

func (s *jemSuite) TestCreateCloudWithRegions(c *gc.C) {
	ctlPath := params.EntityPath{"bob", "foo"}
	s.addController(c, ctlPath)
	ctx := auth.ContextWithUser(testContext, "bob", "bob-group")
	err := s.jem.CreateCloud(ctx, mongodoc.CloudRegion{
		Cloud:            "test-cloud",
		ProviderType:     "kubernetes",
		AuthTypes:        []string{"certificate"},
		Endpoint:         "https://1.2.3.4:5678",
		IdentityEndpoint: "https://1.2.3.4:5679",
		StorageEndpoint:  "https://1.2.3.4:5680",
		CACertificates:   []string{"This is a CA Certficiate (honest)"},
		ACL: params.ACL{
			Read:  []string{"bob"},
			Write: []string{"bob"},
			Admin: []string{"bob"},
		},
	}, []mongodoc.CloudRegion{{
		Cloud:  "test-cloud",
		Region: "test-region",
		ACL: params.ACL{
			Read:  []string{"bob"},
			Write: []string{"bob"},
			Admin: []string{"bob"},
		},
	}})
	c.Assert(err, gc.Equals, nil)

	var docs []mongodoc.CloudRegion
	err = s.jem.DB.CloudRegions().Find(bson.D{{"cloud", "test-cloud"}}).Sort("_id").All(&docs)
	c.Assert(err, gc.Equals, nil)
	c.Assert(docs, jc.DeepEquals, []mongodoc.CloudRegion{{
		Id:                   "test-cloud/",
		Cloud:                "test-cloud",
		ProviderType:         "kubernetes",
		AuthTypes:            []string{"certificate"},
		Endpoint:             "https://1.2.3.4:5678",
		IdentityEndpoint:     "https://1.2.3.4:5679",
		StorageEndpoint:      "https://1.2.3.4:5680",
		CACertificates:       []string{"This is a CA Certficiate (honest)"},
		PrimaryControllers:   []params.EntityPath{{User: "bob", Name: "foo"}},
		SecondaryControllers: []params.EntityPath{},
		ACL: params.ACL{
			Read:  []string{"bob"},
			Write: []string{"bob"},
			Admin: []string{"bob"},
		},
	}, {
		Id:                   "test-cloud/test-region",
		Cloud:                "test-cloud",
		Region:               "test-region",
		PrimaryControllers:   []params.EntityPath{{User: "bob", Name: "foo"}},
		SecondaryControllers: []params.EntityPath{},
		ACL: params.ACL{
			Read:  []string{"bob"},
			Write: []string{"bob"},
			Admin: []string{"bob"},
		},
	}})
}

func (s *jemSuite) TestCreateCloudNameMatch(c *gc.C) {
	ctlPath := params.EntityPath{"bob", "foo"}
	s.addController(c, ctlPath)
	ctx := auth.ContextWithUser(testContext, "bob", "bob-group")
	err := s.jem.CreateCloud(ctx, mongodoc.CloudRegion{
		Cloud:            "dummy",
		ProviderType:     "kubernetes",
		AuthTypes:        []string{"certificate"},
		Endpoint:         "https://1.2.3.4:5678",
		IdentityEndpoint: "https://1.2.3.4:5679",
		StorageEndpoint:  "https://1.2.3.4:5680",
		CACertificates:   []string{"This is a CA Certficiate (honest)"},
		ACL: params.ACL{
			Read:  []string{"bob"},
			Write: []string{"bob"},
			Admin: []string{"bob"},
		},
	}, nil)
	c.Assert(err, gc.ErrorMatches, `cloud "dummy" already exists`)
}

func (s *jemSuite) TestCreateCloudNoControllers(c *gc.C) {
	ctx := auth.ContextWithUser(testContext, "bob", "bob-group")
	err := s.jem.CreateCloud(ctx, mongodoc.CloudRegion{
		Cloud:            "test-cloud",
		ProviderType:     "kubernetes",
		AuthTypes:        []string{"certificate"},
		Endpoint:         "https://1.2.3.4:5678",
		IdentityEndpoint: "https://1.2.3.4:5679",
		StorageEndpoint:  "https://1.2.3.4:5680",
		CACertificates:   []string{"This is a CA Certficiate (honest)"},
		ACL: params.ACL{
			Read:  []string{"bob"},
			Write: []string{"bob"},
			Admin: []string{"bob"},
		},
	}, nil)
	c.Assert(err, gc.ErrorMatches, `cannot find a suitable controller`)

	var docs []mongodoc.CloudRegion
	err = s.jem.DB.CloudRegions().Find(bson.D{{"cloud", "test-cloud"}}).Sort("_id").All(&docs)
	c.Assert(err, gc.Equals, nil)
	c.Assert(docs, jc.DeepEquals, []mongodoc.CloudRegion{})
}

func (s *jemSuite) TestCreateCloudAddCloudError(c *gc.C) {
	ctlPath := params.EntityPath{"bob", "foo"}
	s.addController(c, ctlPath)
	ctx := auth.ContextWithUser(testContext, "bob", "bob-group")
	err := s.jem.CreateCloud(ctx, mongodoc.CloudRegion{
		Cloud:            "test-cloud",
		ProviderType:     "kubernetes",
		Endpoint:         "https://1.2.3.4:5678",
		IdentityEndpoint: "https://1.2.3.4:5679",
		StorageEndpoint:  "https://1.2.3.4:5680",
		CACertificates:   []string{"This is a CA Certficiate (honest)"},
		ACL: params.ACL{
			Read:  []string{"bob"},
			Write: []string{"bob"},
			Admin: []string{"bob"},
		},
	}, nil)
	c.Assert(err, gc.ErrorMatches, `invalid cloud: empty auth-types not valid`)

	var docs []mongodoc.CloudRegion
	err = s.jem.DB.CloudRegions().Find(bson.D{{"cloud", "test-cloud"}}).Sort("_id").All(&docs)
	c.Assert(err, gc.Equals, nil)
	c.Assert(docs, jc.DeepEquals, []mongodoc.CloudRegion{})
}

func (s *jemSuite) TestRemoveCloud(c *gc.C) {
	ctlPath := params.EntityPath{"bob", "foo"}
	s.addController(c, ctlPath)
	ctx := auth.ContextWithUser(testContext, "bob", "bob-group")
	err := s.jem.CreateCloud(ctx, mongodoc.CloudRegion{
		Cloud:            "test-cloud",
		ProviderType:     "kubernetes",
		AuthTypes:        []string{"certificate"},
		Endpoint:         "https://1.2.3.4:5678",
		IdentityEndpoint: "https://1.2.3.4:5679",
		StorageEndpoint:  "https://1.2.3.4:5680",
		CACertificates:   []string{"This is a CA Certficiate (honest)"},
		ACL: params.ACL{
			Read:  []string{"bob"},
			Write: []string{"bob"},
			Admin: []string{"bob"},
		},
	}, nil)
	c.Assert(err, gc.Equals, nil)

	var docs []mongodoc.CloudRegion
	err = s.jem.DB.CloudRegions().Find(bson.D{{"cloud", "test-cloud"}}).Sort("_id").All(&docs)
	c.Assert(err, gc.Equals, nil)
	c.Assert(docs, jc.DeepEquals, []mongodoc.CloudRegion{{
		Id:                   "test-cloud/",
		Cloud:                "test-cloud",
		ProviderType:         "kubernetes",
		AuthTypes:            []string{"certificate"},
		Endpoint:             "https://1.2.3.4:5678",
		IdentityEndpoint:     "https://1.2.3.4:5679",
		StorageEndpoint:      "https://1.2.3.4:5680",
		CACertificates:       []string{"This is a CA Certficiate (honest)"},
		PrimaryControllers:   []params.EntityPath{{User: "bob", Name: "foo"}},
		SecondaryControllers: []params.EntityPath{},
		ACL: params.ACL{
			Read:  []string{"bob"},
			Write: []string{"bob"},
			Admin: []string{"bob"},
		},
	}})

	err = s.jem.RemoveCloud(ctx, "test-cloud")
	c.Assert(err, gc.Equals, nil)

	err = s.jem.DB.CloudRegions().Find(bson.D{{"cloud", "test-cloud"}}).Sort("_id").All(&docs)
	c.Assert(err, gc.Equals, nil)
	c.Assert(docs, jc.DeepEquals, []mongodoc.CloudRegion{})
}

func (s *jemSuite) TestRemoveCloudUnauthorized(c *gc.C) {
	ctlPath := params.EntityPath{"bob", "foo"}
	s.addController(c, ctlPath)
	ctx := auth.ContextWithUser(testContext, "bob", "bob-group")
	err := s.jem.CreateCloud(ctx, mongodoc.CloudRegion{
		Cloud:            "test-cloud",
		ProviderType:     "kubernetes",
		AuthTypes:        []string{"certificate"},
		Endpoint:         "https://1.2.3.4:5678",
		IdentityEndpoint: "https://1.2.3.4:5679",
		StorageEndpoint:  "https://1.2.3.4:5680",
		CACertificates:   []string{"This is a CA Certficiate (honest)"},
		ACL: params.ACL{
			Read:  []string{"bob"},
			Write: []string{"bob"},
			Admin: []string{"alice"},
		},
	}, nil)
	c.Assert(err, gc.Equals, nil)

	err = s.jem.RemoveCloud(ctx, "test-cloud")
	c.Assert(err, gc.ErrorMatches, `unauthorized`)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrUnauthorized)
}

func (s *jemSuite) TestRemoveCloudNotFound(c *gc.C) {
	ctlPath := params.EntityPath{"bob", "foo"}
	s.addController(c, ctlPath)
	ctx := auth.ContextWithUser(testContext, "bob", "bob-group")

	err := s.jem.RemoveCloud(ctx, "test-cloud")
	c.Assert(err, gc.ErrorMatches, `cloud "test-cloud" region "" not found`)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)
}

func (s *jemSuite) TestRemoveCloudWithModel(c *gc.C) {
	kubeconfig, err := kubetest.LoadConfig()
	if errgo.Cause(err) == kubetest.ErrDisabled {
		c.Skip("kubernetes testing disabled")
	}
	c.Assert(err, gc.Equals, nil, gc.Commentf("error loading kubernetes config: %v", err))

	ctlPath := params.EntityPath{"bob", "foo"}
	s.addController(c, ctlPath)
	ctx := auth.ContextWithUser(testContext, "bob", "bob-group")
	var cacerts []string
	if cert := kubetest.CACertificate(kubeconfig); cert != "" {
		cacerts = append(cacerts, cert)
	}
	err = s.jem.CreateCloud(ctx, mongodoc.CloudRegion{
		Cloud:          "test-cloud",
		ProviderType:   "kubernetes",
		AuthTypes:      []string{string(cloud.UserPassAuthType)},
		Endpoint:       kubetest.ServerURL(kubeconfig),
		CACertificates: cacerts,
		ACL: params.ACL{
			Read:  []string{"bob"},
			Write: []string{"bob"},
			Admin: []string{"bob"},
		},
	}, nil)
	c.Assert(err, gc.Equals, nil)

	credpath := params.CredentialPath{
		Cloud: "test-cloud",
		EntityPath: params.EntityPath{
			User: "bob",
			Name: "kubernetes",
		},
	}
	_, err = s.jem.UpdateCredential(ctx, &mongodoc.Credential{
		Path: credpath,
		Type: string(cloud.UserPassAuthType),
		Attributes: map[string]string{
			"username": kubetest.Username(kubeconfig),
			"password": kubetest.Password(kubeconfig),
		},
	}, jem.CredentialUpdate)
	c.Assert(err, gc.Equals, nil)

	_, err = s.jem.CreateModel(ctx, jem.CreateModelParams{
		Path:       params.EntityPath{"bob", "test-model"},
		Cloud:      "test-cloud",
		Credential: credpath,
	})
	c.Assert(err, gc.IsNil)

	err = s.jem.RemoveCloud(ctx, "test-cloud")
	c.Assert(err, gc.ErrorMatches, `cloud is used by 1 model`)
}

func (s *jemSuite) TestUpdateModelCredential(c *gc.C) {
	model := s.bootstrapModel(c, params.EntityPath{User: "bob", Name: "model"})

	credPath := credentialPath("dummy", "bob", "cred2")
	err := jem.UpdateCredential(s.jem.DB, testContext, &mongodoc.Credential{
		Path: credPath,
		Type: "empty",
	})

	conn, err := s.jem.OpenAPI(testContext, model.Controller)
	c.Assert(err, gc.Equals, nil)
	defer conn.Close()

	err = s.jem.UpdateModelCredential(testContext, conn, model, &mongodoc.Credential{
		Path: credPath,
		Type: "empty",
	})
	c.Assert(err, gc.Equals, nil)
	model1, err := s.jem.DB.Model(testContext, model.Path)
	c.Assert(err, gc.Equals, nil)
	c.Assert(model1.Credential, jc.DeepEquals, credPath)
}

func (s *jemSuite) addController(c *gc.C, path params.EntityPath) params.EntityPath {
	info := s.APIInfo(c)

	hps, err := mongodoc.ParseAddresses(info.Addrs)
	c.Assert(err, gc.Equals, nil)

	ctl := &mongodoc.Controller{
		Path:          path,
		HostPorts:     [][]mongodoc.HostPort{hps},
		CACert:        info.CACert,
		AdminUser:     info.Tag.Id(),
		AdminPassword: info.Password,
		Location: map[string]string{
			"cloud":  "dummy",
			"region": "dummy-region",
		},
		Public: true,
	}
	err = s.jem.DB.AddController(testContext, ctl)
	c.Assert(err, gc.Equals, nil)
	err = s.jem.DB.UpdateCloudRegions(testContext, []mongodoc.CloudRegion{{
		Cloud:              "dummy",
		PrimaryControllers: []params.EntityPath{path},
		ACL: params.ACL{
			Read: []string{"everyone"},
		},
	}, {
		Cloud:              "dummy",
		Region:             "dummy-region",
		PrimaryControllers: []params.EntityPath{path},
		ACL: params.ACL{
			Read: []string{"everyone"},
		},
	}})
	c.Assert(err, gc.Equals, nil)
	return path
}

func (s *jemSuite) bootstrapModel(c *gc.C, path params.EntityPath) *mongodoc.Model {
	ctlPath := s.addController(c, params.EntityPath{User: path.User, Name: "controller"})
	credPath := credentialPath("dummy", string(path.User), "cred")
	err := jem.UpdateCredential(s.jem.DB, testContext, &mongodoc.Credential{
		Path: credPath,
		Type: "empty",
	})
	c.Assert(err, gc.Equals, nil)
	ctx := auth.ContextWithUser(testContext, string(path.User))
	model, err := s.jem.CreateModel(ctx, jem.CreateModelParams{
		Path:           path,
		ControllerPath: ctlPath,
		Credential: params.CredentialPath{
			Cloud:      "dummy",
			EntityPath: params.EntityPath{"bob", "cred"},
		},
		Cloud: "dummy",
	})
	c.Assert(err, gc.Equals, nil)
	return model
}
