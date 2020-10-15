// Copyright 2020 Canonical Ltd.

package jem_test

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/juju/clock/testclock"
	modelmanagerapi "github.com/juju/juju/api/modelmanager"
	jujuparams "github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing/factory"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"
	"gopkg.in/errgo.v1"
	"gopkg.in/macaroon-bakery.v2/httpbakery"
	"gopkg.in/mgo.v2/bson"

	"github.com/CanonicalLtd/jimm/internal/conv"
	"github.com/CanonicalLtd/jimm/internal/jem"
	"github.com/CanonicalLtd/jimm/internal/jemtest"
	"github.com/CanonicalLtd/jimm/internal/mgosession"
	"github.com/CanonicalLtd/jimm/internal/mongodoc"
	"github.com/CanonicalLtd/jimm/internal/pubsub"
	"github.com/CanonicalLtd/jimm/params"
)

type modelManagerSuite struct {
	jemtest.JujuConnSuite
	pool                           *jem.Pool
	sessionPool                    *mgosession.Pool
	jem                            *jem.JEM
	usageSenderAuthorizationClient *testUsageSenderAuthorizationClient
}

var _ = gc.Suite(&modelManagerSuite{})

func (s *modelManagerSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.sessionPool = mgosession.NewPool(context.TODO(), s.Session, 5)
	publicCloudMetadata, _, err := cloud.PublicCloudMetadata()
	c.Assert(err, gc.Equals, nil)
	s.usageSenderAuthorizationClient = &testUsageSenderAuthorizationClient{}
	s.PatchValue(&jem.NewUsageSenderAuthorizationClient, func(_ string, _ *httpbakery.Client) (jem.UsageSenderAuthorizationClient, error) {
		return s.usageSenderAuthorizationClient, nil
	})
	pool, err := jem.NewPool(context.TODO(), jem.Params{
		DB:                  s.Session.DB("jem"),
		ControllerAdmin:     "controller-admin",
		SessionPool:         s.sessionPool,
		PublicCloudMetadata: publicCloudMetadata,
		UsageSenderURL:      "test-usage-sender-url",
		Pubsub: &pubsub.Hub{
			MaxConcurrency: 10,
		},
	})
	c.Assert(err, gc.Equals, nil)
	s.pool = pool
	s.jem = s.pool.JEM(context.TODO())
	s.PatchValue(&utils.OutgoingAccessAllowed, true)
}

func (s *modelManagerSuite) TearDownTest(c *gc.C) {
	s.jem.Close()
	s.pool.Close()
	s.sessionPool.Close()
	s.JujuConnSuite.TearDownTest(c)
}

func (s *modelManagerSuite) TestDestroyModel(c *gc.C) {
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

	err = s.jem.DestroyModel(testContext, jemtest.NewIdentity("bob"), model, nil, nil, nil)
	c.Assert(err, gc.Equals, nil)

	// Check the model is dying.
	m := mongodoc.Model{Path: model.Path}
	err = s.jem.DB.GetModel(testContext, &m)
	c.Assert(err, gc.Equals, nil)
	c.Assert(m.Life(), gc.Equals, "dying")

	// Check that it can be destroyed twice.
	err = s.jem.DestroyModel(testContext, jemtest.NewIdentity("bob"), model, nil, nil, nil)
	c.Assert(err, gc.Equals, nil)

	// Check the model is still dying.
	err = s.jem.DB.GetModel(testContext, &m)
	c.Assert(err, gc.Equals, nil)
	c.Assert(m.Life(), gc.Equals, "dying")
}

func (s *modelManagerSuite) TestDestroyModelWithStorage(c *gc.C) {
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

	err = s.jem.DestroyModel(testContext, jemtest.NewIdentity("bob"), model, nil, nil, nil)
	c.Assert(err, jc.Satisfies, jujuparams.IsCodeHasPersistentStorage)
}

var createModelTests = []struct {
	about                          string
	user                           string
	params                         jem.CreateModelParams
	usageSenderAuthorizationErrors []error
	expectCredential               params.CredentialPath
	expectError                    string
	expectErrorCause               error
}{{
	about: "success",
	user:  "bob",
	params: jem.CreateModelParams{
		Path: params.EntityPath{"bob", ""},
		Credential: params.CredentialPath{
			Cloud: "dummy",
			User:  "bob",
			Name:  "cred1",
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
			Cloud: "dummy",
			User:  "bob",
			Name:  "cred1",
		},
		Cloud: "dummy",
	},
}, {
	about: "success with region",
	user:  "bob",
	params: jem.CreateModelParams{
		Path: params.EntityPath{"bob", ""},
		Credential: params.CredentialPath{
			Cloud: "dummy",
			User:  "bob",
			Name:  "cred1",
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
			Cloud: "dummy",
			User:  "bob",
			Name:  "cred2",
		},
		Cloud: "dummy",
	},
	expectError:      `credential not found`,
	expectErrorCause: params.ErrNotFound,
}, {
	about: "model exists",
	user:  "bob",
	params: jem.CreateModelParams{
		Path: params.EntityPath{"bob", "oldmodel"},
		Credential: params.CredentialPath{
			Cloud: "dummy",
			User:  "bob",
			Name:  "cred1",
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
			Cloud: "dummy",
			User:  "bob",
			Name:  "cred1",
		},
		Cloud:  "dummy",
		Region: "not-a-region",
	},
	expectError: `cloudregion not found`,
}, {
	about: "empty cloud credentials selects single choice",
	user:  "bob",
	params: jem.CreateModelParams{
		Path:  params.EntityPath{"bob", ""},
		Cloud: "dummy",
	},
	expectCredential: params.CredentialPath{
		Cloud: "dummy",
		User:  "bob",
		Name:  "cred1",
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
}, {
	about: "success with usage sender authorization client",
	user:  "bob",
	params: jem.CreateModelParams{
		Path: params.EntityPath{"bob", ""},
		Credential: params.CredentialPath{
			Cloud: "dummy",
			User:  "bob",
			Name:  "cred1",
		},
		Cloud: "dummy",
	},
	usageSenderAuthorizationErrors: []error{errors.New("a silly error")},
}}

func (s *modelManagerSuite) TestCreateModel(c *gc.C) {
	now := bson.Now()
	s.PatchValue(jem.WallClock, testclock.NewClock(now))
	ctlId := s.addController(c, params.EntityPath{"bob", "controller"})
	err := s.jem.DB.SetACL(testContext, s.jem.DB.Controllers(), ctlId, params.ACL{
		Read: []string{"everyone"},
	})
	c.Assert(err, gc.Equals, nil)
	// Bob has a single credential.
	err = s.jem.DB.UpsertCredential(testContext, &mongodoc.Credential{
		Path: mgoCredentialPath("dummy", "bob", "cred1"),
		Type: "empty",
	})
	c.Assert(err, gc.Equals, nil)
	// Alice has two credentials.
	err = s.jem.DB.UpsertCredential(testContext, &mongodoc.Credential{
		Path: mgoCredentialPath("dummy", "alice", "cred1"),
		Type: "empty",
	})
	c.Assert(err, gc.Equals, nil)
	err = s.jem.DB.UpsertCredential(testContext, &mongodoc.Credential{
		Path: mgoCredentialPath("dummy", "alice", "cred2"),
		Type: "empty",
	})
	c.Assert(err, gc.Equals, nil)

	// Create a model so that we can have a test case for an already-existing model
	err = s.jem.CreateModel(testContext, jemtest.NewIdentity("bob"), jem.CreateModelParams{
		Path:           params.EntityPath{"bob", "oldmodel"},
		ControllerPath: ctlId,
		Credential: params.CredentialPath{
			Cloud: "dummy",
			User:  "bob",
			Name:  "cred1",
		},
		Cloud: "dummy",
	}, nil)
	c.Assert(err, gc.Equals, nil)
	for i, test := range createModelTests {
		c.Logf("test %d. %s", i, test.about)
		s.usageSenderAuthorizationClient.SetErrors(test.usageSenderAuthorizationErrors)
		if test.params.Path.Name == "" {
			test.params.Path.Name = params.Name(fmt.Sprintf("test-%d", i))
		}
		var info jujuparams.ModelInfo
		err := s.jem.CreateModel(testContext, jemtest.NewIdentity(test.user), test.params, &info)
		if test.expectError != "" {
			c.Assert(err, gc.ErrorMatches, test.expectError)
			if test.expectErrorCause != nil {
				c.Assert(errgo.Cause(err), gc.Equals, test.expectErrorCause)
			}
			continue
		}
		c.Assert(err, gc.Equals, nil)
		c.Check(info.Name, gc.Equals, string(test.params.Path.Name))
		c.Check(info.OwnerTag, gc.Equals, conv.ToUserTag(test.params.Path.User).String())
		c.Check(info.UUID, gc.Not(gc.Equals), "")
		c.Check(info.CloudTag, gc.Equals, conv.ToCloudTag(test.params.Cloud).String())
		c.Check(info.CloudRegion, gc.Equals, "dummy-region")
		c.Check(info.DefaultSeries, gc.Not(gc.Equals), "")
		c.Check(string(info.Life), gc.Equals, "alive")

		m := mongodoc.Model{
			Path: test.params.Path,
		}
		err = s.jem.DB.GetModel(testContext, &m)
		c.Assert(err, gc.Equals, nil)
		c.Check(m.Creator, gc.Equals, test.user)
		c.Check(m.CreationTime.Equal(now), gc.Equals, true)
		if !test.expectCredential.IsZero() {
			c.Check(m.Credential, jc.DeepEquals, mongodoc.CredentialPathFromParams(test.expectCredential))
		} else {
			c.Check(m.Credential, jc.DeepEquals, mongodoc.CredentialPathFromParams(test.params.Credential))
		}
	}
}

func (s *modelManagerSuite) TestCreateModelWithPartiallyCreatedModel(c *gc.C) {
	now := bson.Now()
	s.PatchValue(jem.WallClock, testclock.NewClock(now))
	ctlId := s.addController(c, params.EntityPath{"bob", "controller"})
	err := s.jem.DB.SetACL(testContext, s.jem.DB.Controllers(), ctlId, params.ACL{
		Read: []string{"everyone"},
	})
	c.Assert(err, gc.Equals, nil)
	// Bob has a single credential.
	err = s.jem.DB.UpsertCredential(testContext, &mongodoc.Credential{
		Path: mgoCredentialPath("dummy", "bob", "cred1"),
		Type: "empty",
	})
	// Create a partial model in the database.
	err = s.jem.DB.InsertModel(testContext, &mongodoc.Model{
		Path:         params.EntityPath{"bob", "oldmodel"},
		Controller:   ctlId,
		CreationTime: now,
		Creator:      "bob",
		Credential:   mongodoc.CredentialPathFromParams(credentialPath("dummy", "bob", "cred1")),
	})
	c.Assert(err, gc.Equals, nil)
	// Create a new model
	err = s.jem.CreateModel(testContext, jemtest.NewIdentity("bob"), jem.CreateModelParams{
		Path:           params.EntityPath{"bob", "model"},
		ControllerPath: ctlId,
		Credential: params.CredentialPath{
			Cloud: "dummy",
			User:  "bob",
			Name:  "cred1",
		},
		Cloud: "dummy",
	}, nil)
	c.Assert(err, gc.Equals, nil)
}

func (s *modelManagerSuite) TestCreateModelWithExistingModelInControllerOnly(c *gc.C) {
	// Create a model and then delete its entry in the JIMM database
	// as if the controller model had been created but something
	// had failed in CreateModel after that.
	model := s.bootstrapModel(c, params.EntityPath{User: "bob", Name: "model"})
	err := s.jem.DB.RemoveModel(testContext, model)
	c.Assert(err, gc.Equals, nil)

	// Now try to create the model again.
	err = s.jem.CreateModel(testContext, jemtest.NewIdentity(string(model.Path.User)), jem.CreateModelParams{
		Path:           model.Path,
		ControllerPath: model.Controller,
		Credential: params.CredentialPath{
			Cloud: "dummy",
			User:  "bob",
			Name:  "cred",
		},
		Cloud: "dummy",
	}, nil)
	c.Assert(err, gc.ErrorMatches, `cannot create model: model name in use: api error: failed to create new model: model "model" for bob@external already exists \(already exists\)`)
}

func (s *modelManagerSuite) TestCreateModelWithDeprecatedController(c *gc.C) {
	ctlId := s.addController(c, params.EntityPath{"bob", "controller"})
	err := s.jem.DB.SetACL(testContext, s.jem.DB.Controllers(), ctlId, params.ACL{
		Read: []string{"everyone"},
	})
	c.Assert(err, gc.Equals, nil)
	id := jemtest.NewIdentity("bob")
	// Sanity check that we can create the model while the controller is not deprecated.
	err = s.jem.CreateModel(testContext, id, jem.CreateModelParams{
		Path:   params.EntityPath{"bob", "model1"},
		Cloud:  "dummy",
		Region: "dummy-region",
	}, nil)
	c.Assert(err, gc.Equals, nil)

	// Deprecate it and make sure it's not chosen again.
	err = s.jem.SetControllerDeprecated(testContext, jemtest.NewIdentity("controller-admin"), ctlId, true)
	c.Assert(err, gc.Equals, nil)

	err = s.jem.CreateModel(testContext, id, jem.CreateModelParams{
		Path:   params.EntityPath{"bob", "model2"},
		Cloud:  "dummy",
		Region: "dummy-region",
	}, nil)
	c.Assert(err, gc.ErrorMatches, `cannot find suitable controller`)
}

func (s *modelManagerSuite) TestCreateModelWithMultipleControllers(c *gc.C) {
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
	// Deprecate the first controller.
	err = s.jem.SetControllerDeprecated(testContext, jemtest.NewIdentity("controller-admin"), ctlId, true)
	c.Assert(err, gc.Equals, nil)

	err = s.jem.CreateModel(testContext, jemtest.NewIdentity("bob"), jem.CreateModelParams{
		Path:   params.EntityPath{"bob", "model2"},
		Cloud:  "dummy",
		Region: "dummy-region",
	}, nil)
	c.Assert(err, gc.Equals, nil)
	m := mongodoc.Model{Path: params.EntityPath{"bob", "model2"}}
	err = s.jem.DB.GetModel(testContext, &m)
	c.Assert(err, gc.Equals, nil)
	c.Assert(m.Controller, jc.DeepEquals, ctl2Id)
}

func (s *modelManagerSuite) TestGetModelInfo(c *gc.C) {
	ctlPath := s.addController(c, params.EntityPath{"bob", "test"})
	var info jujuparams.ModelInfo
	id := jemtest.NewIdentity("bob")
	err := s.jem.CreateModel(testContext, id, jem.CreateModelParams{
		Path:           params.EntityPath{"bob", "model"},
		ControllerPath: ctlPath,
	}, &info)
	c.Assert(err, gc.Equals, nil)

	info2 := jujuparams.ModelInfo{
		UUID: info.UUID,
	}
	err = s.jem.GetModelInfo(testContext, id, &info2, true)
	c.Assert(err, gc.Equals, nil)
	c.Check(info2, jc.DeepEquals, info)
}

func (s *modelManagerSuite) TestGetModelInfoLocalOnly(c *gc.C) {
	ctlPath := s.addController(c, params.EntityPath{"bob", "test"})
	var info jujuparams.ModelInfo
	id := jemtest.NewIdentity("bob")
	err := s.jem.CreateModel(testContext, id, jem.CreateModelParams{
		Path:           params.EntityPath{"bob", "model"},
		ControllerPath: ctlPath,
	}, &info)
	c.Assert(err, gc.Equals, nil)

	info2 := jujuparams.ModelInfo{
		UUID: info.UUID,
	}
	err = s.jem.GetModelInfo(testContext, id, &info2, false)
	c.Assert(err, gc.Equals, nil)

	// The local database doesn't have the information to populate these
	// fields.
	info.SLA = nil
	info.CloudCredentialValidity = nil

	// Round times to the resolution in the database.
	roundedTime := info.Status.Since.Truncate(time.Millisecond)
	info.Status.Since = &roundedTime
	c.Check(info2, jc.DeepEquals, info)
}

func (s *modelManagerSuite) TestGetModelInfoUnauthorized(c *gc.C) {
	ctlPath := s.addController(c, params.EntityPath{"bob", "test"})
	var info jujuparams.ModelInfo
	id := jemtest.NewIdentity("bob")
	err := s.jem.CreateModel(testContext, id, jem.CreateModelParams{
		Path:           params.EntityPath{"bob", "model"},
		ControllerPath: ctlPath,
	}, &info)
	c.Assert(err, gc.Equals, nil)

	info2 := jujuparams.ModelInfo{
		UUID: info.UUID,
	}
	err = s.jem.GetModelInfo(testContext, jemtest.NewIdentity("alice"), &info2, false)
	c.Check(err, gc.ErrorMatches, "unauthorized")
	c.Check(errgo.Cause(err), gc.Equals, params.ErrUnauthorized)
}

func (s *modelManagerSuite) TestGetModelInfoNotFound(c *gc.C) {
	info := jujuparams.ModelInfo{
		UUID: "no uuid",
	}
	err := s.jem.GetModelInfo(testContext, jemtest.NewIdentity("alice"), &info, true)
	c.Check(err, gc.ErrorMatches, "model not found")
	c.Check(errgo.Cause(err), gc.Equals, params.ErrNotFound)
}

func (s *modelManagerSuite) TestGetModelStatus(c *gc.C) {
	ctlPath := s.addController(c, params.EntityPath{"bob", "test"})
	var info jujuparams.ModelInfo
	id := jemtest.NewIdentity("bob")
	err := s.jem.CreateModel(testContext, id, jem.CreateModelParams{
		Path:           params.EntityPath{"bob", "model"},
		ControllerPath: ctlPath,
	}, &info)
	c.Assert(err, gc.Equals, nil)

	conn, err := s.jem.OpenAPI(testContext, ctlPath)
	c.Assert(err, gc.Equals, nil)
	controllerStatus := jujuparams.ModelStatus{
		ModelTag: names.NewModelTag(info.UUID).String(),
	}
	err = conn.ModelStatus(testContext, &controllerStatus)
	c.Assert(err, gc.Equals, nil)

	var status jujuparams.ModelStatus
	err = s.jem.GetModelStatus(testContext, id, info.UUID, &status, true)
	c.Assert(err, gc.Equals, nil)
	c.Check(status, jc.DeepEquals, controllerStatus)
}

func (s *modelManagerSuite) TestGetModelStatusLocalOnly(c *gc.C) {
	ctlPath := s.addController(c, params.EntityPath{"bob", "test"})
	var info jujuparams.ModelInfo
	id := jemtest.NewIdentity("bob")
	err := s.jem.CreateModel(testContext, id, jem.CreateModelParams{
		Path:           params.EntityPath{"bob", "model"},
		ControllerPath: ctlPath,
	}, &info)
	c.Assert(err, gc.Equals, nil)

	conn, err := s.jem.OpenAPI(testContext, ctlPath)
	c.Assert(err, gc.Equals, nil)
	controllerStatus := jujuparams.ModelStatus{
		ModelTag: names.NewModelTag(info.UUID).String(),
	}
	err = conn.ModelStatus(testContext, &controllerStatus)
	c.Assert(err, gc.Equals, nil)

	var status jujuparams.ModelStatus
	err = s.jem.GetModelStatus(testContext, id, info.UUID, &status, false)
	c.Assert(err, gc.Equals, nil)
	c.Check(status, jc.DeepEquals, controllerStatus)
}

func (s *modelManagerSuite) TestGetModelStatusUnauthorized(c *gc.C) {
	ctlPath := s.addController(c, params.EntityPath{"bob", "test"})
	var info jujuparams.ModelInfo
	id := jemtest.NewIdentity("bob")
	err := s.jem.CreateModel(testContext, id, jem.CreateModelParams{
		Path:           params.EntityPath{"bob", "model"},
		ControllerPath: ctlPath,
	}, &info)
	c.Assert(err, gc.Equals, nil)

	var status jujuparams.ModelStatus
	err = s.jem.GetModelStatus(testContext, jemtest.NewIdentity("alice"), info.UUID, &status, false)
	c.Check(err, gc.ErrorMatches, "unauthorized")
	c.Check(errgo.Cause(err), gc.Equals, params.ErrUnauthorized)
}

func (s *modelManagerSuite) TestGetModelStatusNotFound(c *gc.C) {
	var status jujuparams.ModelStatus
	err := s.jem.GetModelStatus(testContext, jemtest.NewIdentity("alice"), "not a uuid", &status, true)
	c.Check(err, gc.ErrorMatches, "model not found")
	c.Check(errgo.Cause(err), gc.Equals, params.ErrNotFound)
}

func (s *modelManagerSuite) TestGetModelStatuses(c *gc.C) {
	ctlPath := params.EntityPath{"bob", "x"}
	m := mongodoc.Model{
		Id:   "ignored",
		Path: ctlPath,
	}
	err := s.jem.DB.InsertModel(testContext, &m)
	c.Assert(err, gc.Equals, nil)
	c.Assert(m, jc.DeepEquals, mongodoc.Model{
		Id:   "bob/x",
		Path: ctlPath,
	})

	m1 := mongodoc.Model{Path: ctlPath}
	err = s.jem.DB.GetModel(testContext, &m1)
	c.Assert(err, gc.Equals, nil)
	c.Assert(m1, jemtest.CmpEquals(cmpopts.EquateEmpty()), m)

	st, err := s.jem.GetModelStatuses(testContext, jemtest.NewIdentity("bob", "controller-admin"))
	c.Assert(err, gc.Equals, nil)
	c.Assert(st, gc.DeepEquals, params.ModelStatuses{{
		Status:     "unknown",
		ID:         "bob/x",
		Controller: "/",
	}})
}

func (s *modelManagerSuite) TestGrantModelWrite(c *gc.C) {
	model := s.bootstrapModel(c, params.EntityPath{User: "bob", Name: "model"})
	err := s.jem.GrantModel(testContext, jemtest.NewIdentity("bob"), model, "alice", "write")
	c.Assert(err, gc.Equals, nil)
	model1 := mongodoc.Model{Path: model.Path}
	err = s.jem.DB.GetModel(testContext, &model1)
	c.Assert(err, gc.Equals, nil)
	c.Assert(model1.ACL, jc.DeepEquals, params.ACL{
		Read:  []string{"alice"},
		Write: []string{"alice"},
	})
}

func (s *modelManagerSuite) TestGrantModelRead(c *gc.C) {
	model := s.bootstrapModel(c, params.EntityPath{User: "bob", Name: "model"})
	err := s.jem.GrantModel(testContext, jemtest.NewIdentity("bob"), model, "alice", "read")
	c.Assert(err, gc.Equals, nil)
	model1 := mongodoc.Model{Path: model.Path}
	err = s.jem.DB.GetModel(testContext, &model1)
	c.Assert(err, gc.Equals, nil)
	c.Assert(model1.ACL, jc.DeepEquals, params.ACL{
		Read: []string{"alice"},
	})
}

func (s *modelManagerSuite) TestGrantModelBadLevel(c *gc.C) {
	model := s.bootstrapModel(c, params.EntityPath{User: "bob", Name: "model"})
	err := s.jem.GrantModel(testContext, jemtest.NewIdentity("bob"), model, "alice", "superpowers")
	c.Assert(err, gc.ErrorMatches, `api error: could not modify model access: "superpowers" model access not valid`)
	model1 := mongodoc.Model{Path: model.Path}
	err = s.jem.DB.GetModel(testContext, &model1)
	c.Assert(err, gc.Equals, nil)
	c.Assert(model1.ACL, jc.DeepEquals, params.ACL{})
}

func (s *modelManagerSuite) TestRevokeModel(c *gc.C) {
	model := s.bootstrapModel(c, params.EntityPath{User: "bob", Name: "model"})
	err := s.jem.GrantModel(testContext, jemtest.NewIdentity("bob"), model, "alice", "admin")
	c.Assert(err, gc.Equals, nil)
	model1 := mongodoc.Model{Path: model.Path}
	err = s.jem.DB.GetModel(testContext, &model1)
	c.Assert(err, gc.Equals, nil)
	c.Assert(model1.ACL, jc.DeepEquals, params.ACL{
		Read:  []string{"alice"},
		Write: []string{"alice"},
		Admin: []string{"alice"},
	})
	err = s.jem.RevokeModel(testContext, jemtest.NewIdentity("bob"), model, "alice", "read")
	c.Assert(err, gc.Equals, nil)
	err = s.jem.DB.GetModel(testContext, &model1)
	c.Assert(err, gc.Equals, nil)
	c.Assert(model1.ACL, jc.DeepEquals, params.ACL{})
}

func (s *modelManagerSuite) TestRevokeModelAdmin(c *gc.C) {
	model := s.bootstrapModel(c, params.EntityPath{User: "bob", Name: "model"})
	err := s.jem.GrantModel(testContext, jemtest.NewIdentity("bob"), model, "alice", "admin")
	c.Assert(err, gc.Equals, nil)
	model1 := mongodoc.Model{Path: model.Path}
	err = s.jem.DB.GetModel(testContext, &model1)
	c.Assert(err, gc.Equals, nil)
	c.Assert(model1.ACL, jc.DeepEquals, params.ACL{
		Read:  []string{"alice"},
		Write: []string{"alice"},
		Admin: []string{"alice"},
	})
	err = s.jem.RevokeModel(testContext, jemtest.NewIdentity("bob"), model, "alice", "admin")
	c.Assert(err, gc.Equals, nil)
	err = s.jem.DB.GetModel(testContext, &model1)
	c.Assert(err, gc.Equals, nil)
	c.Assert(model1.ACL, jc.DeepEquals, params.ACL{
		Read:  []string{"alice"},
		Write: []string{"alice"},
	})
}

func (s *modelManagerSuite) TestRevokeModelWrite(c *gc.C) {
	model := s.bootstrapModel(c, params.EntityPath{User: "bob", Name: "model"})
	err := s.jem.GrantModel(testContext, jemtest.NewIdentity("bob"), model, "alice", "admin")
	c.Assert(err, gc.Equals, nil)
	model1 := mongodoc.Model{Path: model.Path}
	err = s.jem.DB.GetModel(testContext, &model1)
	c.Assert(err, gc.Equals, nil)
	c.Assert(model1.ACL, jc.DeepEquals, params.ACL{
		Read:  []string{"alice"},
		Write: []string{"alice"},
		Admin: []string{"alice"},
	})
	err = s.jem.RevokeModel(testContext, jemtest.NewIdentity("bob"), model, "alice", "write")
	c.Assert(err, gc.Equals, nil)
	err = s.jem.DB.GetModel(testContext, &model1)
	c.Assert(err, gc.Equals, nil)
	c.Assert(model1.ACL, jc.DeepEquals, params.ACL{
		Read: []string{"alice"},
	})
}

func (s *modelManagerSuite) TestRevokeModelControllerFailure(c *gc.C) {
	model := s.bootstrapModel(c, params.EntityPath{User: "bob", Name: "model"})
	err := s.jem.GrantModel(testContext, jemtest.NewIdentity("bob"), model, "alice", "write")
	c.Assert(err, gc.Equals, nil)
	model1 := mongodoc.Model{Path: model.Path}
	err = s.jem.DB.GetModel(testContext, &model1)
	c.Assert(err, gc.Equals, nil)
	c.Assert(model1.ACL, jc.DeepEquals, params.ACL{
		Read:  []string{"alice"},
		Write: []string{"alice"},
	})
	err = s.jem.RevokeModel(testContext, jemtest.NewIdentity("bob"), model, "alice", "superpowers")
	c.Assert(err, gc.ErrorMatches, `api error: could not modify model access: "superpowers" model access not valid`)
	err = s.jem.DB.GetModel(testContext, &model1)
	c.Assert(err, gc.Equals, nil)
	c.Assert(model1.ACL, jc.DeepEquals, params.ACL{
		Read:  []string{"alice"},
		Write: []string{"alice"},
	})
}

func (s *modelManagerSuite) TestUpdateModelCredential(c *gc.C) {
	model := s.bootstrapModel(c, params.EntityPath{User: "bob", Name: "model"})

	credPath := credentialPath("dummy", "bob", "cred2")
	err := s.jem.DB.UpsertCredential(testContext, &mongodoc.Credential{
		Path: mongodoc.CredentialPathFromParams(credPath),
		Type: "empty",
	})

	conn, err := s.jem.OpenAPI(testContext, model.Controller)
	c.Assert(err, gc.Equals, nil)
	defer conn.Close()

	err = s.jem.UpdateModelCredential(testContext, conn, model, &mongodoc.Credential{
		Path: mongodoc.CredentialPathFromParams(credPath),
		Type: "empty",
	})
	c.Assert(err, gc.Equals, nil)
	model1 := mongodoc.Model{Path: model.Path}
	err = s.jem.DB.GetModel(testContext, &model1)
	c.Assert(err, gc.Equals, nil)
	c.Assert(model1.Credential, jc.DeepEquals, mongodoc.CredentialPathFromParams(credPath))
}

func (s *modelManagerSuite) addController(c *gc.C, path params.EntityPath) params.EntityPath {
	return addController(c, path, s.APIInfo(c), s.jem)
}

func (s *modelManagerSuite) bootstrapModel(c *gc.C, path params.EntityPath) *mongodoc.Model {
	return bootstrapModel(c, path, s.APIInfo(c), s.jem)
}
