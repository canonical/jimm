// Copyright 2020 Canonical Ltd.

package jem_test

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/juju/clock/testclock"
	modelmanagerapi "github.com/juju/juju/api/modelmanager"
	jujuparams "github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing/factory"
	"github.com/juju/mgo/v2/bson"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v2"
	gc "gopkg.in/check.v1"
	"gopkg.in/errgo.v1"

	"github.com/canonical/jimm/internal/apiconn"
	"github.com/canonical/jimm/internal/conv"
	"github.com/canonical/jimm/internal/jem"
	"github.com/canonical/jimm/internal/jemtest"
	"github.com/canonical/jimm/internal/mongodoc"
	"github.com/canonical/jimm/params"
)

type modelManagerSuite struct {
	jemtest.BootstrapSuite
}

var _ = gc.Suite(&modelManagerSuite{})

func (s *modelManagerSuite) SetUpTest(c *gc.C) {
	s.BootstrapSuite.SetUpTest(c)
}

func (s *modelManagerSuite) TestValidateModelUpgrade(c *gc.C) {
	now := bson.Now()
	s.PatchValue(jem.WallClock, testclock.NewClock(now))

	err := s.JEM.ValidateModelUpgrade(testContext, jemtest.Charlie, s.Model.UUID, true)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrUnauthorized)

	err = s.JEM.ValidateModelUpgrade(testContext, jemtest.Bob, s.Model.UUID, false)
	c.Assert(err, gc.Equals, nil)

	err = s.JEM.ValidateModelUpgrade(testContext, jemtest.Bob, utils.MustNewUUID().String(), true)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)
}

func (s *modelManagerSuite) TestDestroyModel(c *gc.C) {
	conn, err := s.JEM.OpenAPI(testContext, s.Model.Controller)
	c.Assert(err, gc.Equals, nil)
	defer conn.Close()

	// Check the model exists
	client := modelmanagerapi.NewClient(conn)
	_, err = client.ModelInfo([]names.ModelTag{names.NewModelTag(s.Model.UUID)})
	c.Assert(err, gc.Equals, nil)

	err = s.JEM.DestroyModel(testContext, jemtest.Bob, &s.Model, nil, nil, nil, nil)
	c.Assert(err, gc.Equals, nil)

	// Check the model is dying.
	err = s.JEM.DB.GetModel(testContext, &s.Model)
	c.Assert(err, gc.Equals, nil)
	c.Assert(s.Model.Life(), gc.Equals, "dying")

	// Check that it can be destroyed twice.
	err = s.JEM.DestroyModel(testContext, jemtest.Bob, &s.Model, nil, nil, nil, nil)
	c.Assert(err, gc.Equals, nil)

	// Check the model is still dying.
	err = s.JEM.DB.GetModel(testContext, &s.Model)
	c.Assert(err, gc.Equals, nil)
	c.Assert(s.Model.Life(), gc.Equals, "dying")
}

func (s *modelManagerSuite) TestDestroyModelWithStorage(c *gc.C) {
	conn, err := s.JEM.OpenAPI(testContext, s.Model.Controller)
	c.Assert(err, gc.Equals, nil)
	defer conn.Close()

	// Check the model exists
	tag := names.NewModelTag(s.Model.UUID)
	client := modelmanagerapi.NewClient(conn)
	_, err = client.ModelInfo([]names.ModelTag{tag})
	c.Assert(err, gc.Equals, nil)

	modelState, err := s.StatePool.Get(s.Model.UUID)
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

	err = s.JEM.DestroyModel(testContext, jemtest.Bob, &s.Model, nil, nil, nil, nil)
	c.Assert(err, jc.Satisfies, jujuparams.IsCodeHasPersistentStorage)
}

var createModelTests = []struct {
	about            string
	user             string
	params           jem.CreateModelParams
	expectCredential mongodoc.CredentialPath
	expectError      string
	expectErrorCause error
}{{
	about: "success",
	user:  "bob",
	params: jem.CreateModelParams{
		Path: params.EntityPath{"bob", ""},
		Credential: mongodoc.CredentialPath{
			Cloud: jemtest.TestCloudName,
			EntityPath: mongodoc.EntityPath{
				User: "bob",
				Name: "cred",
			},
		},
		Cloud: jemtest.TestCloudName,
	},
}, {
	about: "success specified controller",
	user:  "bob",
	params: jem.CreateModelParams{
		Path:           params.EntityPath{"bob", ""},
		ControllerPath: params.EntityPath{jemtest.ControllerAdmin, "controller-1"},
		Credential: mongodoc.CredentialPath{
			Cloud: jemtest.TestCloudName,
			EntityPath: mongodoc.EntityPath{
				User: "bob",
				Name: "cred",
			},
		},
		Cloud: jemtest.TestCloudName,
	},
}, {
	about: "success with region",
	user:  "bob",
	params: jem.CreateModelParams{
		Path: params.EntityPath{"bob", ""},
		Credential: mongodoc.CredentialPath{
			Cloud: jemtest.TestCloudName,
			EntityPath: mongodoc.EntityPath{
				User: "bob",
				Name: "cred",
			},
		},
		Cloud:  jemtest.TestCloudName,
		Region: jemtest.TestCloudRegionName,
	},
}, {
	about: "unknown credential",
	user:  "bob",
	params: jem.CreateModelParams{
		Path: params.EntityPath{"bob", ""},
		Credential: mongodoc.CredentialPath{
			Cloud: jemtest.TestCloudName,
			EntityPath: mongodoc.EntityPath{
				User: "bob",
				Name: "cred2",
			},
		},
		Cloud: jemtest.TestCloudName,
	},
	expectError:      `credential not found`,
	expectErrorCause: params.ErrNotFound,
}, {
	about: "model exists",
	user:  "bob",
	params: jem.CreateModelParams{
		Path: params.EntityPath{"bob", "model-1"},
		Credential: mongodoc.CredentialPath{
			Cloud: jemtest.TestCloudName,
			EntityPath: mongodoc.EntityPath{
				User: "bob",
				Name: "cred",
			},
		},
		Cloud: jemtest.TestCloudName,
	},
	expectError:      `already exists`,
	expectErrorCause: params.ErrAlreadyExists,
}, {
	about: "unrecognised region",
	user:  "bob",
	params: jem.CreateModelParams{
		Path: params.EntityPath{"bob", ""},
		Credential: mongodoc.CredentialPath{
			Cloud: jemtest.TestCloudName,
			EntityPath: mongodoc.EntityPath{
				User: "bob",
				Name: "cred",
			},
		},
		Cloud:  jemtest.TestCloudName,
		Region: "not-a-region",
	},
	expectError: `cloudregion not found`,
}, {
	about: "empty cloud credentials selects single choice",
	user:  "bob",
	params: jem.CreateModelParams{
		Path:  params.EntityPath{"bob", ""},
		Cloud: jemtest.TestCloudName,
	},
	expectCredential: mongodoc.CredentialPath{
		Cloud: jemtest.TestCloudName,
		EntityPath: mongodoc.EntityPath{
			User: "bob",
			Name: "cred",
		},
	},
}, {
	about: "empty cloud credentials fails with more than one choice",
	user:  "alice",
	params: jem.CreateModelParams{
		Path:  params.EntityPath{"alice", ""},
		Cloud: jemtest.TestCloudName,
	},
	expectError:      `more than one possible credential to use`,
	expectErrorCause: params.ErrAmbiguousChoice,
}, {
	about: "empty cloud credentials passed through if no credentials found",
	user:  "charlie",
	params: jem.CreateModelParams{
		Path:  params.EntityPath{"charlie", ""},
		Cloud: jemtest.TestCloudName,
	},
}}

func (s *modelManagerSuite) TestCreateModel(c *gc.C) {
	now := bson.Now()
	s.PatchValue(jem.WallClock, testclock.NewClock(now))
	// Add two credentials for alice.
	cred1 := jemtest.EmptyCredential("alice", "cred1")
	err := s.JEM.DB.UpsertCredential(testContext, &cred1)
	c.Assert(err, gc.Equals, nil)
	cred2 := jemtest.EmptyCredential("alice", "cred2")
	err = s.JEM.DB.UpsertCredential(testContext, &cred2)
	c.Assert(err, gc.Equals, nil)

	for i, test := range createModelTests {
		c.Logf("test %d. %s", i, test.about)
		if test.params.Path.Name == "" {
			test.params.Path.Name = params.Name(fmt.Sprintf("test-%d", i))
		}
		var info jujuparams.ModelInfo
		err := s.JEM.CreateModel(testContext, jemtest.NewIdentity(test.user), test.params, &info)
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
		c.Check(info.CloudRegion, gc.Equals, jemtest.TestCloudRegionName)
		c.Check(info.DefaultSeries, gc.Not(gc.Equals), "")
		c.Check(string(info.Life), gc.Equals, "alive")

		m := mongodoc.Model{
			Path: test.params.Path,
		}
		err = s.JEM.DB.GetModel(testContext, &m)
		c.Assert(err, gc.Equals, nil)
		c.Check(m.Creator, gc.Equals, test.user)
		c.Check(m.CreationTime.Equal(now), gc.Equals, true)
		if !test.expectCredential.IsZero() {
			c.Check(m.Credential, jc.DeepEquals, test.expectCredential)
		} else {
			c.Check(m.Credential, jc.DeepEquals, test.params.Credential)
		}
	}
}

func newTestModelCreateAPI(ctx context.Context, conn *apiconn.Conn, waitChannel, doneChannel chan bool) *testModelCreateAPI {
	return &testModelCreateAPI{
		Conn:        conn,
		waitChannel: waitChannel,
		doneChannel: doneChannel,
	}
}

type testModelCreateAPI struct {
	*apiconn.Conn
	waitChannel chan bool
	doneChannel chan bool
}

func (t *testModelCreateAPI) CreateModel(ctx context.Context, params *jujuparams.ModelCreateArgs, info *jujuparams.ModelInfo) error {
	defer func() {
		t.doneChannel <- true
	}()
	<-t.waitChannel
	return t.Conn.CreateModel(ctx, params, info)
}

func (s *modelManagerSuite) TestCreateModelWithTimeout(c *gc.C) {
	now := bson.Now()
	s.PatchValue(jem.WallClock, testclock.NewClock(now))

	// Add two credentials for alice.
	cred1 := jemtest.EmptyCredential("alice", "cred1")
	err := s.JEM.DB.UpsertCredential(testContext, &cred1)
	c.Assert(err, gc.Equals, nil)
	cred2 := jemtest.EmptyCredential("alice", "cred2")
	err = s.JEM.DB.UpsertCredential(testContext, &cred2)
	c.Assert(err, gc.Equals, nil)

	params := jem.CreateModelParams{
		Path: params.EntityPath{"bob", "test"},
		Credential: mongodoc.CredentialPath{
			Cloud: jemtest.TestCloudName,
			EntityPath: mongodoc.EntityPath{
				User: "bob",
				Name: "cred",
			},
		},
		Cloud: jemtest.TestCloudName,
	}

	waitChannel := make(chan bool)
	doneChannel := make(chan bool)
	s.PatchValue(&jem.OpenModelCreateAPI, func(ctx context.Context, j *jem.JEM, controller *mongodoc.Controller) (jem.ModelCreateAPI, error) {
		conn, err := j.OpenAPIFromDoc(ctx, controller)
		if err != nil {
			return nil, err
		}
		return newTestModelCreateAPI(ctx, conn, waitChannel, doneChannel), nil
	})

	testContext, cancelContextFunc := context.WithTimeout(testContext, 500*time.Millisecond)

	var info jujuparams.ModelInfo
	// let's call the CreateModel function now, which will try to
	// create a model async. So timing out the context will cause
	// the create model request to JEM to fail, but model creating
	// at juju controller should continue in the background.
	err = s.JEM.CreateModel(testContext, jemtest.NewIdentity("bob"), params, &info)

	// now let's cancel the context - e.g the request timed out.
	cancelContextFunc()

	// and now we can continue - send a bool on the wait channel causing
	// the CreateModel to be called on the controller connection.
	waitChannel <- true
	// and wait for the call to finish.
	<-doneChannel

	c.Assert(err, gc.ErrorMatches, "cannot create model: context deadline exceeded")
	c.Check(info.Name, gc.Equals, string(params.Path.Name))
	c.Check(info.OwnerTag, gc.Equals, conv.ToUserTag(params.Path.User).String())
	c.Check(info.UUID, gc.Not(gc.Equals), "")
	c.Check(info.CloudTag, gc.Equals, conv.ToCloudTag(params.Cloud).String())
	c.Check(info.CloudRegion, gc.Equals, jemtest.TestCloudRegionName)
	c.Check(info.DefaultSeries, gc.Not(gc.Equals), "")
	c.Check(string(info.Life), gc.Equals, "alive")

	m := mongodoc.Model{
		Path: params.Path,
	}
	for i := 0; i < 10; i++ {
		err = s.JEM.DB.GetModel(testContext, &m)
		if err == nil {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	c.Assert(err, gc.Equals, nil)
	c.Check(m.Creator, gc.Equals, "bob")
	c.Check(m.CreationTime.Equal(now), gc.Equals, true)
	c.Check(m.Credential, jc.DeepEquals, params.Credential)
}

func (s *modelManagerSuite) TestCreateModelWithPartiallyCreatedModel(c *gc.C) {
	now := bson.Now()
	s.PatchValue(jem.WallClock, testclock.NewClock(now))

	// Create a partial model in the database.
	err := s.JEM.DB.InsertModel(testContext, &mongodoc.Model{
		Path:         params.EntityPath{"bob", "oldmodel"},
		Controller:   s.Controller.Path,
		CreationTime: now,
		Creator:      "bob",
		Credential:   s.Credential.Path,
	})
	c.Assert(err, gc.Equals, nil)
	// Create a new model
	err = s.JEM.CreateModel(testContext, jemtest.Bob, jem.CreateModelParams{
		Path:           params.EntityPath{"bob", "model"},
		ControllerPath: s.Controller.Path,
		Credential:     s.Credential.Path,
		Cloud:          jemtest.TestCloudName,
	}, nil)
	c.Assert(err, gc.Equals, nil)
}

func (s *modelManagerSuite) TestCreateModelWithExistingModelInControllerOnly(c *gc.C) {
	// Delete the default model from the JIMM database as if the controller
	// model had been created but something had failed in CreateModel after
	// that.
	err := s.JEM.DB.RemoveModel(testContext, &s.Model)
	c.Assert(err, gc.Equals, nil)

	// Now try to create the model again.
	err = s.JEM.CreateModel(testContext, jemtest.Bob, jem.CreateModelParams{
		Path:           s.Model.Path,
		ControllerPath: s.Model.Controller,
		Credential:     s.Model.Credential,
		Cloud:          s.Model.Cloud,
	}, nil)
	c.Assert(err, gc.ErrorMatches, `cannot create model: model name in use: api error: failed to create new model: model "model-1" for bob@external already exists \(already exists\)`)
}

func (s *modelManagerSuite) TestCreateModelWithDeprecatedController(c *gc.C) {
	// Check that we can create the model while the controller is not deprecated.
	err := s.JEM.CreateModel(testContext, jemtest.Bob, jem.CreateModelParams{
		Path:   params.EntityPath{"bob", "model1"},
		Cloud:  jemtest.TestCloudName,
		Region: jemtest.TestCloudRegionName,
	}, nil)
	c.Assert(err, gc.Equals, nil)

	// Deprecate it and make sure it's not chosen again.
	err = s.JEM.SetControllerDeprecated(testContext, jemtest.Alice, s.Controller.Path, true)
	c.Assert(err, gc.Equals, nil)

	err = s.JEM.CreateModel(testContext, jemtest.Bob, jem.CreateModelParams{
		Path:   params.EntityPath{"bob", "model2"},
		Cloud:  jemtest.TestCloudName,
		Region: jemtest.TestCloudRegionName,
	}, nil)
	c.Assert(err, gc.ErrorMatches, `cannot find suitable controller`)
}

func (s *modelManagerSuite) TestCreateModelWithMultipleControllers(c *gc.C) {
	s.PatchValue(jem.Shuffle, func(int, func(int, int)) {})
	ctlPath := params.EntityPath{"alice", "controller-2"}
	s.AddController(c, &mongodoc.Controller{
		Path: ctlPath,
	})
	// Deprecate the first controller.
	err := s.JEM.SetControllerDeprecated(testContext, jemtest.Alice, s.Controller.Path, true)
	c.Assert(err, gc.Equals, nil)

	err = s.JEM.CreateModel(testContext, jemtest.Bob, jem.CreateModelParams{
		Path:   params.EntityPath{"bob", "model2"},
		Cloud:  jemtest.TestCloudName,
		Region: jemtest.TestCloudRegionName,
	}, nil)
	c.Assert(err, gc.Equals, nil)
	m := mongodoc.Model{Path: params.EntityPath{"bob", "model2"}}
	err = s.JEM.DB.GetModel(testContext, &m)
	c.Assert(err, gc.Equals, nil)
	c.Assert(m.Controller, jc.DeepEquals, ctlPath)
}

func (s *modelManagerSuite) TestCreateModelWithUnreachableController(c *gc.C) {
	// Check that we can create the model while the controller is not deprecated.
	err := s.JEM.CreateModel(testContext, jemtest.Bob, jem.CreateModelParams{
		Path:   params.EntityPath{"bob", "model1"},
		Cloud:  jemtest.TestCloudName,
		Region: jemtest.TestCloudRegionName,
	}, nil)
	c.Assert(err, gc.Equals, nil)

	err = s.JEM.DB.Controllers().Update(
		bson.D{{"_id", s.Controller.Path.String()}},
		bson.D{{"$set", bson.D{{"hostports", []bson.D{{
			{"host", "0.1.2.3"},
			{"port", "1234"},
			{"scope", "public"},
		}}}}}})
	c.Assert(err, gc.Equals, nil)

	s.Pool.ClearAPIConnCache()

	err = s.JEM.CreateModel(testContext, jemtest.Bob, jem.CreateModelParams{
		Path:   params.EntityPath{"bob", "model2"},
		Cloud:  jemtest.TestCloudName,
		Region: jemtest.TestCloudRegionName,
	}, nil)
	c.Assert(err, gc.ErrorMatches, `cannot create model: cannot connect to controller: validating info for opening an API connection: missing addresses not valid`)
}

func (s *modelManagerSuite) TestGetModelInfo(c *gc.C) {
	conn, err := s.JEM.OpenAPIFromDoc(testContext, &s.Controller)
	c.Assert(err, gc.Equals, nil)
	defer conn.Close()

	err = s.JEM.GrantModel(testContext, jemtest.Bob, &s.Model, "alice@external", "admin")
	c.Assert(err, gc.Equals, nil)

	info := jujuparams.ModelInfo{
		UUID: s.Model.UUID,
	}
	err = conn.ModelInfo(testContext, &info)
	c.Assert(err, gc.Equals, nil)

	// filter local users.
	users := make([]jujuparams.ModelUserInfo, 0, len(info.Users))
	for _, u := range info.Users {
		if strings.Index(u.UserName, "@") == -1 {
			continue
		}
		users = append(users, u)
	}
	info.Users = users

	info2 := jujuparams.ModelInfo{
		UUID: s.Model.UUID,
	}
	err = s.JEM.GetModelInfo(testContext, jemtest.Bob, &info2, true)
	c.Assert(err, gc.Equals, nil)
	c.Check(info2, jc.DeepEquals, info)
}

func (s *modelManagerSuite) TestGetModelInfoLocalOnly(c *gc.C) {
	conn, err := s.JEM.OpenAPIFromDoc(testContext, &s.Controller)
	c.Assert(err, gc.Equals, nil)
	defer conn.Close()
	info := jujuparams.ModelInfo{
		UUID: s.Model.UUID,
	}
	err = conn.ModelInfo(testContext, &info)
	c.Assert(err, gc.Equals, nil)

	info2 := jujuparams.ModelInfo{
		UUID: info.UUID,
	}
	err = s.JEM.GetModelInfo(testContext, jemtest.Bob, &info2, false)
	c.Assert(err, gc.Equals, nil)

	// filter local users.
	users := make([]jujuparams.ModelUserInfo, 0, len(info.Users))
	for _, u := range info.Users {
		if strings.Index(u.UserName, "@") == -1 {
			continue
		}
		users = append(users, u)
	}
	info.Users = users

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
	info := jujuparams.ModelInfo{
		UUID: s.Model.UUID,
	}
	err := s.JEM.GetModelInfo(testContext, jemtest.Charlie, &info, false)
	c.Check(err, gc.ErrorMatches, "unauthorized")
	c.Check(errgo.Cause(err), gc.Equals, params.ErrUnauthorized)
}

func (s *modelManagerSuite) TestGetModelInfoNotFound(c *gc.C) {
	info := jujuparams.ModelInfo{
		UUID: "no uuid",
	}
	err := s.JEM.GetModelInfo(testContext, jemtest.Bob, &info, true)
	c.Check(err, gc.ErrorMatches, "model not found")
	c.Check(errgo.Cause(err), gc.Equals, params.ErrNotFound)
}

func (s *modelManagerSuite) TestGetModelStatus(c *gc.C) {
	conn, err := s.JEM.OpenAPIFromDoc(testContext, &s.Controller)
	c.Assert(err, gc.Equals, nil)
	defer conn.Close()
	controllerStatus := jujuparams.ModelStatus{
		ModelTag: names.NewModelTag(s.Model.UUID).String(),
	}
	err = conn.ModelStatus(testContext, &controllerStatus)
	c.Assert(err, gc.Equals, nil)

	var status jujuparams.ModelStatus
	err = s.JEM.GetModelStatus(testContext, jemtest.Bob, s.Model.UUID, &status, true)
	c.Assert(err, gc.Equals, nil)
	c.Check(status, jc.DeepEquals, controllerStatus)
}

func (s *modelManagerSuite) TestGetModelStatusLocalOnly(c *gc.C) {
	conn, err := s.JEM.OpenAPIFromDoc(testContext, &s.Controller)
	c.Assert(err, gc.Equals, nil)
	defer conn.Close()
	controllerStatus := jujuparams.ModelStatus{
		ModelTag: names.NewModelTag(s.Model.UUID).String(),
	}
	err = conn.ModelStatus(testContext, &controllerStatus)
	c.Assert(err, gc.Equals, nil)

	var status jujuparams.ModelStatus
	err = s.JEM.GetModelStatus(testContext, jemtest.Bob, s.Model.UUID, &status, true)
	c.Assert(err, gc.Equals, nil)
	c.Check(status, jc.DeepEquals, controllerStatus)
}

func (s *modelManagerSuite) TestGetModelStatusUnauthorized(c *gc.C) {
	var status jujuparams.ModelStatus
	err := s.JEM.GetModelStatus(testContext, jemtest.Charlie, s.Model.UUID, &status, false)
	c.Check(err, gc.ErrorMatches, "unauthorized")
	c.Check(errgo.Cause(err), gc.Equals, params.ErrUnauthorized)
}

func (s *modelManagerSuite) TestGetModelStatusNotFound(c *gc.C) {
	var status jujuparams.ModelStatus
	err := s.JEM.GetModelStatus(testContext, jemtest.Bob, "not a uuid", &status, true)
	c.Check(err, gc.ErrorMatches, "model not found")
	c.Check(errgo.Cause(err), gc.Equals, params.ErrNotFound)
}

func (s *modelManagerSuite) TestGetModelStatuses(c *gc.C) {
	st, err := s.JEM.GetModelStatuses(testContext, jemtest.Alice)
	c.Assert(err, gc.Equals, nil)
	c.Assert(st, jc.DeepEquals, params.ModelStatuses{{
		ID:         s.Model.Id,
		UUID:       s.Model.UUID,
		Controller: s.Model.Controller.String(),
		Created:    s.Model.CreationTime,
		Cloud:      string(s.Model.Cloud),
		Region:     s.Model.CloudRegion,
		Status:     "available",
	}})
}

func (s *modelManagerSuite) TestGrantModelWrite(c *gc.C) {
	err := s.JEM.GrantModel(testContext, jemtest.Bob, &s.Model, "alice", "write")
	c.Assert(err, gc.Equals, nil)
	model1 := mongodoc.Model{Path: s.Model.Path}
	err = s.JEM.DB.GetModel(testContext, &model1)
	c.Assert(err, gc.Equals, nil)
	c.Assert(model1.ACL, jc.DeepEquals, params.ACL{
		Read:  []string{"alice"},
		Write: []string{"alice"},
	})
}

func (s *modelManagerSuite) TestGrantModelRead(c *gc.C) {
	err := s.JEM.GrantModel(testContext, jemtest.Bob, &s.Model, "alice", "read")
	c.Assert(err, gc.Equals, nil)
	model1 := mongodoc.Model{Path: s.Model.Path}
	err = s.JEM.DB.GetModel(testContext, &model1)
	c.Assert(err, gc.Equals, nil)
	c.Assert(model1.ACL, jc.DeepEquals, params.ACL{
		Read: []string{"alice"},
	})
}

func (s *modelManagerSuite) TestGrantModelBadLevel(c *gc.C) {
	err := s.JEM.GrantModel(testContext, jemtest.Bob, &s.Model, "alice", "superpowers")
	c.Assert(err, gc.ErrorMatches, `api error: could not modify model access: "superpowers" model access not valid`)
	model1 := mongodoc.Model{Path: s.Model.Path}
	err = s.JEM.DB.GetModel(testContext, &model1)
	c.Assert(err, gc.Equals, nil)
	c.Assert(model1.ACL, jc.DeepEquals, params.ACL{})
}

func (s *modelManagerSuite) TestRevokeModel(c *gc.C) {
	err := s.JEM.GrantModel(testContext, jemtest.Bob, &s.Model, "alice", "admin")
	c.Assert(err, gc.Equals, nil)
	model1 := mongodoc.Model{Path: s.Model.Path}
	err = s.JEM.DB.GetModel(testContext, &model1)
	c.Assert(err, gc.Equals, nil)
	c.Assert(model1.ACL, jc.DeepEquals, params.ACL{
		Read:  []string{"alice"},
		Write: []string{"alice"},
		Admin: []string{"alice"},
	})
	err = s.JEM.RevokeModel(testContext, jemtest.Bob, &s.Model, "alice", "read")
	c.Assert(err, gc.Equals, nil)
	err = s.JEM.DB.GetModel(testContext, &model1)
	c.Assert(err, gc.Equals, nil)
	c.Assert(model1.ACL, jc.DeepEquals, params.ACL{})
}

func (s *modelManagerSuite) TestRevokeModelAdmin(c *gc.C) {
	err := s.JEM.GrantModel(testContext, jemtest.Bob, &s.Model, "alice", "admin")
	c.Assert(err, gc.Equals, nil)
	model1 := mongodoc.Model{Path: s.Model.Path}
	err = s.JEM.DB.GetModel(testContext, &model1)
	c.Assert(err, gc.Equals, nil)
	c.Assert(model1.ACL, jc.DeepEquals, params.ACL{
		Read:  []string{"alice"},
		Write: []string{"alice"},
		Admin: []string{"alice"},
	})
	err = s.JEM.RevokeModel(testContext, jemtest.Bob, &s.Model, "alice", "admin")
	c.Assert(err, gc.Equals, nil)
	err = s.JEM.DB.GetModel(testContext, &model1)
	c.Assert(err, gc.Equals, nil)
	c.Assert(model1.ACL, jc.DeepEquals, params.ACL{
		Read:  []string{"alice"},
		Write: []string{"alice"},
	})
}

func (s *modelManagerSuite) TestRevokeModelWrite(c *gc.C) {
	err := s.JEM.GrantModel(testContext, jemtest.Bob, &s.Model, "alice", "admin")
	c.Assert(err, gc.Equals, nil)
	model1 := mongodoc.Model{Path: s.Model.Path}
	err = s.JEM.DB.GetModel(testContext, &model1)
	c.Assert(err, gc.Equals, nil)
	c.Assert(model1.ACL, jc.DeepEquals, params.ACL{
		Read:  []string{"alice"},
		Write: []string{"alice"},
		Admin: []string{"alice"},
	})
	err = s.JEM.RevokeModel(testContext, jemtest.Bob, &s.Model, "alice", "write")
	c.Assert(err, gc.Equals, nil)
	err = s.JEM.DB.GetModel(testContext, &model1)
	c.Assert(err, gc.Equals, nil)
	c.Assert(model1.ACL, jc.DeepEquals, params.ACL{
		Read: []string{"alice"},
	})
}

func (s *modelManagerSuite) TestUserCanAlwaysRevokeTheirOwnAccess(c *gc.C) {
	err := s.JEM.GrantModel(testContext, jemtest.Bob, &s.Model, "alice", "read")
	c.Assert(err, gc.Equals, nil)
	model1 := mongodoc.Model{Path: s.Model.Path}
	err = s.JEM.DB.GetModel(testContext, &model1)
	c.Assert(err, gc.Equals, nil)
	c.Assert(model1.ACL, jc.DeepEquals, params.ACL{
		Read:  []string{"alice"},
		Write: []string{},
		Admin: []string{},
	})
	err = s.JEM.RevokeModel(testContext, jemtest.Alice, &s.Model, "alice", "read")
	c.Assert(err, gc.Equals, nil)
	err = s.JEM.DB.GetModel(testContext, &model1)
	c.Assert(err, gc.Equals, nil)
	c.Assert(model1.ACL, jc.DeepEquals, params.ACL{})
}

func (s *modelManagerSuite) TestRevokeModelControllerFailure(c *gc.C) {
	err := s.JEM.GrantModel(testContext, jemtest.Bob, &s.Model, "alice", "write")
	c.Assert(err, gc.Equals, nil)
	model1 := mongodoc.Model{Path: s.Model.Path}
	err = s.JEM.DB.GetModel(testContext, &model1)
	c.Assert(err, gc.Equals, nil)
	c.Assert(model1.ACL, jc.DeepEquals, params.ACL{
		Read:  []string{"alice"},
		Write: []string{"alice"},
	})
	err = s.JEM.RevokeModel(testContext, jemtest.Bob, &s.Model, "alice", "superpowers")
	c.Assert(err, gc.ErrorMatches, `api error: could not modify model access: "superpowers" model access not valid`)
	err = s.JEM.DB.GetModel(testContext, &model1)
	c.Assert(err, gc.Equals, nil)
	c.Assert(model1.ACL, jc.DeepEquals, params.ACL{
		Read:  []string{"alice"},
		Write: []string{"alice"},
	})
}

func (s *modelManagerSuite) TestUpdateModelCredential(c *gc.C) {
	cred := jemtest.EmptyCredential("bob", "cred2")
	err := s.JEM.DB.UpsertCredential(testContext, &cred)

	conn, err := s.JEM.OpenAPIFromDoc(testContext, &s.Controller)
	c.Assert(err, gc.Equals, nil)
	defer conn.Close()

	err = s.JEM.UpdateModelCredential(testContext, conn, &s.Model, &cred)
	c.Assert(err, gc.Equals, nil)
	model1 := mongodoc.Model{Path: s.Model.Path}
	err = s.JEM.DB.GetModel(testContext, &model1)
	c.Assert(err, gc.Equals, nil)
	c.Assert(model1.Credential, jc.DeepEquals, cred.Path)
}

func (s *modelManagerSuite) TestModelDefaults(c *gc.C) {
	ctx := context.Background()

	result, err := s.JEM.ModelDefaultsForCloud(ctx, jemtest.NewIdentity("bob"), "no-such-cloud")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Error, gc.IsNil)
	c.Assert(result.Config, gc.HasLen, 0)

	err = s.JEM.SetModelDefaults(
		ctx,
		jemtest.NewIdentity("bob"),
		"test-cloud",
		"test-region",
		map[string]interface{}{
			"a": 12345,
			"b": "value1",
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	result, err = s.JEM.ModelDefaultsForCloud(ctx, jemtest.Bob, "test-cloud")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Error, gc.IsNil)
	c.Assert(result.Config, jc.DeepEquals, map[string]jujuparams.ModelDefaults{
		"a": jujuparams.ModelDefaults{
			Regions: []jujuparams.RegionDefaults{{
				RegionName: "test-region",
				Value:      12345,
			}},
		},
		"b": jujuparams.ModelDefaults{
			Regions: []jujuparams.RegionDefaults{{
				RegionName: "test-region",
				Value:      "value1",
			}},
		},
	})

	err = s.JEM.SetModelDefaults(
		ctx,
		jemtest.NewIdentity("bob"),
		"test-cloud",
		"test-region",
		map[string]interface{}{
			"a": 12345,
			"b": "value1",
			"c": 17,
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	err = s.JEM.SetModelDefaults(
		ctx,
		jemtest.NewIdentity("bob"),
		"test-cloud",
		"test-another-region",
		map[string]interface{}{
			"a": 1,
			"b": "value2",
			"c": 2,
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	result, err = s.JEM.ModelDefaultsForCloud(ctx, jemtest.NewIdentity("bob"), "test-cloud")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Error, gc.IsNil)
	for k, v := range result.Config {
		sort.Slice(v.Regions, func(i, j int) bool {
			return v.Regions[i].RegionName < v.Regions[j].RegionName
		})
		result.Config[k] = v
	}
	c.Assert(result.Config, jc.DeepEquals, map[string]jujuparams.ModelDefaults{
		"a": jujuparams.ModelDefaults{
			Regions: []jujuparams.RegionDefaults{{
				RegionName: "test-another-region",
				Value:      1,
			}, {
				RegionName: "test-region",
				Value:      12345,
			}},
		},
		"b": jujuparams.ModelDefaults{
			Regions: []jujuparams.RegionDefaults{{
				RegionName: "test-another-region",
				Value:      "value2",
			}, {
				RegionName: "test-region",
				Value:      "value1",
			}},
		},
		"c": jujuparams.ModelDefaults{
			Regions: []jujuparams.RegionDefaults{{
				RegionName: "test-another-region",
				Value:      2,
			}, {
				RegionName: "test-region",
				Value:      17,
			}},
		},
	})

	err = s.JEM.UnsetModelDefaults(
		ctx,
		jemtest.NewIdentity("bob"),
		"test-cloud",
		"test-another-region",
		[]string{"a"},
	)
	c.Assert(err, jc.ErrorIsNil)
	err = s.JEM.UnsetModelDefaults(
		ctx,
		jemtest.NewIdentity("bob"),
		"test-cloud",
		"test-region",
		[]string{"c"},
	)
	c.Assert(err, jc.ErrorIsNil)

	result, err = s.JEM.ModelDefaultsForCloud(ctx, jemtest.NewIdentity("bob"), "test-cloud")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Error, gc.IsNil)
	for k, v := range result.Config {
		sort.Slice(v.Regions, func(i, j int) bool {
			return v.Regions[i].RegionName < v.Regions[j].RegionName
		})
		result.Config[k] = v
	}
	c.Assert(result.Config, jc.DeepEquals, map[string]jujuparams.ModelDefaults{
		"a": jujuparams.ModelDefaults{
			Regions: []jujuparams.RegionDefaults{{
				RegionName: "test-region",
				Value:      12345,
			}},
		},
		"b": jujuparams.ModelDefaults{
			Regions: []jujuparams.RegionDefaults{{
				RegionName: "test-another-region",
				Value:      "value2",
			}, {
				RegionName: "test-region",
				Value:      "value1",
			}},
		},
		"c": jujuparams.ModelDefaults{
			Regions: []jujuparams.RegionDefaults{{
				RegionName: "test-another-region",
				Value:      2,
			}},
		},
	})
}
