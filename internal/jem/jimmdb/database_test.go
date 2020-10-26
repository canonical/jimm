// Copyright 2016 Canonical Ltd.

package jimmdb_test

import (
	"context"
	"fmt"

	"github.com/google/go-cmp/cmp/cmpopts"
	jujuparams "github.com/juju/juju/apiserver/params"
	"github.com/juju/names/v4"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"
	"gopkg.in/errgo.v1"
	"gopkg.in/macaroon-bakery.v2/bakery/identchecker"

	"github.com/CanonicalLtd/jimm/internal/jem/jimmdb"
	"github.com/CanonicalLtd/jimm/internal/jemtest"
	"github.com/CanonicalLtd/jimm/internal/mgosession"
	"github.com/CanonicalLtd/jimm/internal/mongodoc"
	"github.com/CanonicalLtd/jimm/params"
)

var testContext = context.Background()

type databaseSuite struct {
	jemtest.IsolatedMgoSuite
	database *jimmdb.Database
}

var _ = gc.Suite(&databaseSuite{})

func (s *databaseSuite) SetUpTest(c *gc.C) {
	s.IsolatedMgoSuite.SetUpTest(c)
	pool := mgosession.NewPool(context.TODO(), s.Session, 1)
	s.database = jimmdb.NewDatabase(context.TODO(), pool, "jem")
	c.Assert(s.database.Session.Ping(), gc.Equals, nil)
	pool.Close()
	c.Assert(s.database.Session.Ping(), gc.Equals, nil)
}

func (s *databaseSuite) TearDownTest(c *gc.C) {
	s.database.Session.Close()
	s.database = nil
	s.IsolatedMgoSuite.TearDownTest(c)
}

func (s *databaseSuite) checkDBOK(c *gc.C) {
	c.Check(s.database.Session.Ping(), gc.Equals, nil)
}

func (s *databaseSuite) TestUpdateMachineInfo(c *gc.C) {
	ctlPath := params.EntityPath{"bob", "x"}
	err := s.database.UpdateMachineInfo(testContext, &mongodoc.Machine{
		Controller: ctlPath.String(),
		Cloud:      "dummy",
		Region:     "dummy-region",
		Info: &jujuparams.MachineInfo{
			ModelUUID: "fake-uuid",
			Id:        "0",
			Series:    "quantal",
		},
	})
	c.Assert(err, gc.Equals, nil)
	err = s.database.UpdateMachineInfo(testContext, &mongodoc.Machine{
		Controller: ctlPath.String(),
		Cloud:      "dummy",
		Region:     "dummy-region",
		Info: &jujuparams.MachineInfo{
			ModelUUID: "another-uuid",
			Id:        "0",
			Series:    "blah",
		},
	})
	c.Assert(err, gc.Equals, nil)
	err = s.database.UpdateMachineInfo(testContext, &mongodoc.Machine{
		Controller: ctlPath.String(),
		Cloud:      "dummy",
		Region:     "dummy-region",
		Info: &jujuparams.MachineInfo{
			ModelUUID: "fake-uuid",
			Id:        "1",
			Series:    "precise",
		},
	})
	c.Assert(err, gc.Equals, nil)

	docs, err := s.database.MachinesForModel(testContext, "fake-uuid")
	c.Assert(err, gc.Equals, nil)
	for i := range docs {
		cleanMachineDoc(&docs[i])
	}
	c.Assert(docs, jc.DeepEquals, []mongodoc.Machine{{
		Id:         ctlPath.String() + " fake-uuid 0",
		Controller: ctlPath.String(),
		Cloud:      "dummy",
		Region:     "dummy-region",
		Info: &jujuparams.MachineInfo{
			ModelUUID: "fake-uuid",
			Id:        "0",
			Series:    "quantal",
			Config:    map[string]interface{}{},
		},
	}, {
		Id:         ctlPath.String() + " fake-uuid 1",
		Controller: ctlPath.String(),
		Cloud:      "dummy",
		Region:     "dummy-region",
		Info: &jujuparams.MachineInfo{
			ModelUUID: "fake-uuid",
			Id:        "1",
			Series:    "precise",
			Config:    map[string]interface{}{},
		},
	}})

	// Check that we can update one of the documents.
	err = s.database.UpdateMachineInfo(testContext, &mongodoc.Machine{
		Controller: ctlPath.String(),
		Cloud:      "dummy",
		Region:     "dummy-region",
		Info: &jujuparams.MachineInfo{
			ModelUUID: "fake-uuid",
			Id:        "0",
			Series:    "foo",
			Life:      "dying",
		},
	})
	c.Assert(err, gc.Equals, nil)

	// Check that setting a machine dead removes it.
	err = s.database.UpdateMachineInfo(testContext, &mongodoc.Machine{
		Controller: ctlPath.String(),
		Cloud:      "dummy",
		Region:     "dummy-region",
		Info: &jujuparams.MachineInfo{
			ModelUUID: "fake-uuid",
			Id:        "1",
			Series:    "foo",
			Life:      "dead",
		},
	})
	c.Assert(err, gc.Equals, nil)

	docs, err = s.database.MachinesForModel(testContext, "fake-uuid")
	c.Assert(err, gc.Equals, nil)
	for i := range docs {
		cleanMachineDoc(&docs[i])
	}
	c.Assert(docs, jc.DeepEquals, []mongodoc.Machine{{
		Id:         ctlPath.String() + " fake-uuid 0",
		Controller: ctlPath.String(),
		Cloud:      "dummy",
		Region:     "dummy-region",
		Info: &jujuparams.MachineInfo{
			ModelUUID: "fake-uuid",
			Id:        "0",
			Series:    "foo",
			Config:    map[string]interface{}{},
			Life:      "dying",
		},
	}})
}

func (s *databaseSuite) TestRemoveControllerMachines(c *gc.C) {
	ctlPath := params.EntityPath{"bob", "x"}
	err := s.database.UpdateMachineInfo(testContext, &mongodoc.Machine{
		Controller: ctlPath.String(),
		Cloud:      "dummy",
		Region:     "dummy-region",
		Info: &jujuparams.MachineInfo{
			ModelUUID: "fake-uuid",
			Id:        "0",
			Series:    "quantal",
		},
	})
	c.Assert(err, gc.Equals, nil)
	docs, err := s.database.MachinesForModel(testContext, "fake-uuid")
	c.Assert(err, gc.Equals, nil)
	for i := range docs {
		cleanMachineDoc(&docs[i])
	}
	c.Assert(docs, jc.DeepEquals, []mongodoc.Machine{{
		Id:         ctlPath.String() + " fake-uuid 0",
		Controller: ctlPath.String(),
		Cloud:      "dummy",
		Region:     "dummy-region",
		Info: &jujuparams.MachineInfo{
			ModelUUID: "fake-uuid",
			Id:        "0",
			Series:    "quantal",
			Config:    map[string]interface{}{},
		},
	}})
	err = s.database.RemoveControllerMachines(testContext, ctlPath)
	c.Assert(err, gc.Equals, nil)
	docs, err = s.database.MachinesForModel(testContext, "fake-uuid")
	c.Assert(err, gc.Equals, nil)
	c.Assert(docs, jc.DeepEquals, []mongodoc.Machine{})
}

func (s *databaseSuite) TestUpdateApplicationInfo(c *gc.C) {
	ctlPath := params.EntityPath{"bob", "x"}
	err := s.database.UpdateApplicationInfo(testContext, &mongodoc.Application{
		Controller: ctlPath.String(),
		Cloud:      "dummy",
		Region:     "dummy-region",
		Info: &mongodoc.ApplicationInfo{
			ModelUUID: "fake-uuid",
			Name:      "0",
		},
	})
	c.Assert(err, gc.Equals, nil)
	err = s.database.UpdateApplicationInfo(testContext, &mongodoc.Application{
		Controller: ctlPath.String(),
		Cloud:      "dummy",
		Region:     "dummy-region",
		Info: &mongodoc.ApplicationInfo{
			ModelUUID: "fake-uuid",
			Name:      "1",
		},
	})

	docs, err := s.database.ApplicationsForModel(testContext, "fake-uuid")
	c.Assert(err, gc.Equals, nil)
	for i := range docs {
		cleanApplicationDoc(&docs[i])
	}
	c.Assert(docs, jc.DeepEquals, []mongodoc.Application{{
		Id:         ctlPath.String() + " fake-uuid 0",
		Controller: ctlPath.String(),
		Cloud:      "dummy",
		Region:     "dummy-region",
		Info: &mongodoc.ApplicationInfo{
			ModelUUID: "fake-uuid",
			Name:      "0",
		},
	}, {
		Id:         ctlPath.String() + " fake-uuid 1",
		Controller: ctlPath.String(),
		Cloud:      "dummy",
		Region:     "dummy-region",
		Info: &mongodoc.ApplicationInfo{
			ModelUUID: "fake-uuid",
			Name:      "1",
		},
	}})

	// Check that we can update one of the documents.
	err = s.database.UpdateApplicationInfo(testContext, &mongodoc.Application{
		Controller: ctlPath.String(),
		Cloud:      "dummy",
		Region:     "dummy-region",
		Info: &mongodoc.ApplicationInfo{
			ModelUUID: "fake-uuid",
			Name:      "0",
			Life:      "dying",
		},
	})
	c.Assert(err, gc.Equals, nil)

	// Check that setting a machine dead removes it.
	err = s.database.UpdateApplicationInfo(testContext, &mongodoc.Application{
		Controller: ctlPath.String(),
		Cloud:      "dummy",
		Region:     "dummy-region",
		Info: &mongodoc.ApplicationInfo{
			ModelUUID: "fake-uuid",
			Name:      "1",
			Life:      "dead",
		},
	})
	c.Assert(err, gc.Equals, nil)

	docs, err = s.database.ApplicationsForModel(testContext, "fake-uuid")
	c.Assert(err, gc.Equals, nil)
	for i := range docs {
		cleanApplicationDoc(&docs[i])
	}
	c.Assert(docs, jc.DeepEquals, []mongodoc.Application{{
		Id:         ctlPath.String() + " fake-uuid 0",
		Controller: ctlPath.String(),
		Cloud:      "dummy",
		Region:     "dummy-region",
		Info: &mongodoc.ApplicationInfo{
			ModelUUID: "fake-uuid",
			Name:      "0",
			Life:      "dying",
		},
	}})
}

func (s *databaseSuite) TestRemoveControllerApplications(c *gc.C) {
	ctlPath := params.EntityPath{"bob", "x"}
	err := s.database.UpdateApplicationInfo(testContext, &mongodoc.Application{
		Controller: ctlPath.String(),
		Cloud:      "dummy",
		Region:     "dummy-region",
		Info: &mongodoc.ApplicationInfo{
			ModelUUID: "fake-uuid",
			Name:      "0",
		},
	})
	c.Assert(err, gc.Equals, nil)
	docs, err := s.database.ApplicationsForModel(testContext, "fake-uuid")
	c.Assert(err, gc.Equals, nil)
	for i := range docs {
		cleanApplicationDoc(&docs[i])
	}
	c.Assert(docs, jc.DeepEquals, []mongodoc.Application{{
		Id:         ctlPath.String() + " fake-uuid 0",
		Controller: ctlPath.String(),
		Cloud:      "dummy",
		Region:     "dummy-region",
		Info: &mongodoc.ApplicationInfo{
			ModelUUID: "fake-uuid",
			Name:      "0",
		},
	}})
	err = s.database.RemoveControllerApplications(testContext, ctlPath)
	c.Assert(err, gc.Equals, nil)
	docs, err = s.database.ApplicationsForModel(testContext, "fake-uuid")
	c.Assert(err, gc.Equals, nil)
	c.Assert(docs, jc.DeepEquals, []mongodoc.Application{})
}

// cleanMachineDoc cleans up the machine document so
// that we can use a DeepEqual comparison without worrying
// about non-nil vs nil map comparisons.
func cleanMachineDoc(doc *mongodoc.Machine) {
	if len(doc.Info.AgentStatus.Data) == 0 {
		doc.Info.AgentStatus.Data = nil
	}
	if len(doc.Info.InstanceStatus.Data) == 0 {
		doc.Info.InstanceStatus.Data = nil
	}
}

// cleanApplicationDoc cleans up the application document so
// that we can use a DeepEqual comparison without worrying
// about non-nil vs nil map comparisons.
func cleanApplicationDoc(doc *mongodoc.Application) {
	if len(doc.Info.Status.Data) == 0 {
		doc.Info.Status.Data = nil
	}
}

func (s *databaseSuite) TestSetModelControllerNotFound(c *gc.C) {
	err := s.database.SetModelController(testContext, params.EntityPath{"bob", "foo"}, params.EntityPath{"x", "y"})
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)
}

func (s *databaseSuite) TestSetModelControllerSuccess(c *gc.C) {
	ctlPath := params.EntityPath{"bob", "foo"}
	err := s.database.InsertController(testContext, &mongodoc.Controller{
		Path: ctlPath,
		UUID: "fake-uuid",
	})
	c.Assert(err, gc.Equals, nil)

	modelPath := params.EntityPath{"bob", "foo"}
	err = s.database.InsertModel(testContext, &mongodoc.Model{
		Path:       modelPath,
		UUID:       "fake-uuid",
		Controller: params.EntityPath{"bob", "foo"},
	})
	c.Assert(err, gc.Equals, nil)
	origDoc := mongodoc.Model{Path: modelPath}
	err = s.database.GetModel(testContext, &origDoc)
	c.Assert(err, gc.Equals, nil)

	err = s.database.SetModelController(testContext, params.EntityPath{"bob", "foo"}, params.EntityPath{"x", "y"})
	c.Assert(err, gc.Equals, nil)

	newDoc := mongodoc.Model{Path: modelPath}
	err = s.database.GetModel(testContext, &newDoc)
	c.Assert(err, gc.Equals, nil)

	origDoc.Controller = params.EntityPath{"x", "y"}

	c.Assert(newDoc, gc.DeepEquals, origDoc)
}

type legacyCredentialPath struct {
	Cloud params.Cloud `httprequest:",path"`
	params.EntityPath
}

func (s *databaseSuite) TestLegacyCredentials(c *gc.C) {
	attrs := map[string]string{
		"attr1": "val1",
		"attr2": "val2",
	}

	id := "test-cloud/test-user/test-credentials"
	// insert credentials with the old path
	err := s.database.Credentials().Insert(
		struct {
			Id         string `bson:"_id"`
			Path       legacyCredentialPath
			Type       string
			Label      string
			Attributes map[string]string
			Revoked    bool
		}{
			Id: id,
			Path: legacyCredentialPath{
				Cloud: "test-cloud",
				EntityPath: params.EntityPath{
					User: params.User("test-user"),
					Name: params.Name("test-credentials"),
				},
			},
			Type:       "credtype",
			Label:      "Test Label",
			Attributes: attrs,
			Revoked:    false,
		})
	c.Assert(err, gc.Equals, nil)

	cred := mongodoc.Credential{
		Path: mongodoc.CredentialPath{
			Cloud: "test-cloud",
			EntityPath: mongodoc.EntityPath{
				User: "test-user",
				Name: "test-credentials",
			},
		},
	}
	err = s.database.GetCredential(testContext, &cred)
	c.Assert(err, gc.Equals, nil)
	c.Assert(cred, jc.DeepEquals, mongodoc.Credential{
		Id: id,
		Path: mongodoc.CredentialPath{
			Cloud: "test-cloud",
			EntityPath: mongodoc.EntityPath{
				User: "test-user",
				Name: "test-credentials",
			},
		},
		Type:       "credtype",
		Label:      "Test Label",
		Attributes: attrs,
	})

	s.checkDBOK(c)
}

func (s *databaseSuite) TestSetACL(c *gc.C) {
	ctlPath := params.EntityPath{"bob", "foo"}
	err := s.database.InsertController(testContext, &mongodoc.Controller{
		Path: ctlPath,
		UUID: "fake-uuid",
	})
	c.Assert(err, gc.Equals, nil)

	err = s.database.SetACL(testContext, s.database.Controllers(), ctlPath, params.ACL{
		Read: []string{"t1", "t2"},
	})
	c.Assert(err, gc.Equals, nil)
	var cnt mongodoc.Controller
	err = s.database.Controllers().FindId(ctlPath.String()).One(&cnt)
	c.Assert(err, gc.Equals, nil)
	c.Assert(cnt.ACL, jc.DeepEquals, params.ACL{
		Read: []string{"t1", "t2"},
	})

	err = s.database.SetACL(testContext, s.database.Controllers(), params.EntityPath{"bob", "bar"}, params.ACL{
		Read: []string{"t2", "t1"},
	})
	c.Assert(err, gc.ErrorMatches, `"bob/bar" not found`)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)
	s.checkDBOK(c)
}

func (s *databaseSuite) TestGetACL(c *gc.C) {
	m := &mongodoc.Model{
		Path: params.EntityPath{
			User: params.User("bob"),
			Name: "model",
		},
		ACL: params.ACL{
			Read: []string{"fred", "jim"},
		},
	}
	err := s.database.InsertModel(testContext, m)
	c.Assert(err, gc.Equals, nil)
	acl, err := s.database.GetACL(testContext, s.database.Models(), m.Path)
	c.Assert(err, gc.Equals, nil)
	c.Assert(acl, jc.DeepEquals, m.ACL)
}

func (s *databaseSuite) TestGetACLNotFound(c *gc.C) {
	m := &mongodoc.Model{
		Path: params.EntityPath{
			User: params.User("bob"),
			Name: "model",
		},
	}
	acl, err := s.database.GetACL(testContext, s.database.Models(), m.Path)
	c.Assert(err, gc.ErrorMatches, "not found")
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)
	c.Assert(acl, jc.DeepEquals, m.ACL)
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

func (s *databaseSuite) TestCheckReadACL(c *gc.C) {
	for i, test := range checkReadACLTests {
		c.Logf("%d. %s", i, test.about)
		entity := params.EntityPath{
			User: params.User(test.owner),
			Name: params.Name(fmt.Sprintf("test%d", i)),
		}
		if !test.skipCreateEntity {
			err := s.database.InsertModel(testContext, &mongodoc.Model{
				Path: entity,
				ACL: params.ACL{
					Read: test.acl,
				},
			})
			c.Assert(err, gc.Equals, nil)
		}
		err := s.database.CheckReadACL(testContext, jemtest.NewIdentity(test.user, test.groups...), s.database.Models(), entity)
		if test.expectError != "" {
			c.Assert(err, gc.ErrorMatches, test.expectError)
			if test.expectCause != nil {
				c.Assert(errgo.Cause(err), gc.Equals, test.expectCause)
			} else {
				c.Assert(errgo.Cause(err), gc.Equals, err)
			}
		} else {
			c.Assert(err, gc.Equals, nil)
		}
	}
}

func (s *databaseSuite) TestCanReadIter(c *gc.C) {
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
		err := s.database.InsertModel(testContext, &testModels[i])
		c.Assert(err, gc.Equals, nil)
	}
	it := s.database.Models().Find(nil).Sort("_id").Iter()
	crit := s.database.NewCanReadIter(jemtest.NewIdentity("bob", "bob-group"), it)
	var models []mongodoc.Model
	var m mongodoc.Model
	for crit.Next(testContext, &m) {
		models = append(models, m)
	}
	c.Assert(crit.Err(testContext), gc.IsNil)
	c.Assert(models, jemtest.CmpEquals(cmpopts.EquateEmpty()), []mongodoc.Model{
		testModels[0],
		testModels[2],
	})
}

var (
	fakeEntityPath = params.EntityPath{"bob", "foo"}
	fakeCredPath   = mongodoc.CredentialPathFromParams(credentialPath("test-cloud", "test-user", "test-credential"))
)

var setDeadTests = []struct {
	about string
	run   func(db *jimmdb.Database)
}{{
	about: "AppendAudit",
	run: func(db *jimmdb.Database) {
		db.AppendAudit(testContext, jemtest.NewIdentity("bob"), &params.AuditModelCreated{})
	},
}, {
	about: "InsertCloudRegion",
	run: func(db *jimmdb.Database) {
		db.InsertCloudRegion(testContext, &mongodoc.CloudRegion{
			Cloud: "dummy",
		})
	},
}, {
	about: "GetCloudRegion",
	run: func(db *jimmdb.Database) {
		db.GetCloudRegion(testContext, &mongodoc.CloudRegion{
			Cloud: "dummy",
		})
	},
}, {
	about: "ForEachCloudRegion",
	run: func(db *jimmdb.Database) {
		db.ForEachCloudRegion(testContext, nil, nil, func(cr *mongodoc.CloudRegion) error { return nil })
	},
}, {
	about: "UpsertCloudRegion",
	run: func(db *jimmdb.Database) {
		db.UpsertCloudRegion(testContext, &mongodoc.CloudRegion{
			Cloud: "dummy",
		})
	},
}, {
	about: "UpdateCloudRegions",
	run: func(db *jimmdb.Database) {
		db.UpdateCloudRegions(testContext, nil, new(jimmdb.Update))
	},
}, {
	about: "RemoveCloudRegion",
	run: func(db *jimmdb.Database) {
		db.RemoveCloudRegion(testContext, &mongodoc.CloudRegion{
			Cloud: "dummy",
		})
	},
}, {
	about: "RemoveCloudRegions",
	run: func(db *jimmdb.Database) {
		db.RemoveCloudRegions(testContext, nil)
	},
}, {
	about: "InsertController",
	run: func(db *jimmdb.Database) {
		db.InsertController(testContext, &mongodoc.Controller{
			Path: params.EntityPath{"bob", "foo"},
			UUID: "fake-uuid",
		})
	},
}, {
	about: "GetController",
	run: func(db *jimmdb.Database) {
		db.GetController(testContext, &mongodoc.Controller{
			Path: params.EntityPath{"bob", "foo"},
			UUID: "fake-uuid",
		})
	},
}, {
	about: "CountControllers",
	run: func(db *jimmdb.Database) {
		db.CountControllers(testContext, nil)
	},
}, {
	about: "ForEachController",
	run: func(db *jimmdb.Database) {
		db.ForEachController(testContext, nil, nil, func(*mongodoc.Controller) error { return nil })
	},
}, {
	about: "UpdateController",
	run: func(db *jimmdb.Database) {
		db.UpdateController(testContext, &mongodoc.Controller{
			Path: params.EntityPath{"bob", "foo"},
			UUID: "fake-uuid",
		}, new(jimmdb.Update).Set("foo", "bar"), false)
	},
}, {
	about: "UpdateControllerQuery",
	run: func(db *jimmdb.Database) {
		db.UpdateControllerQuery(testContext, nil, nil, new(jimmdb.Update).Set("foo", "bar"), false)
	},
}, {
	about: "RemoveController",
	run: func(db *jimmdb.Database) {
		db.RemoveController(testContext, &mongodoc.Controller{
			Path: params.EntityPath{"bob", "foo"},
			UUID: "fake-uuid",
		})
	},
}, {
	about: "InsertModel",
	run: func(db *jimmdb.Database) {
		db.InsertModel(testContext, &mongodoc.Model{
			Path: fakeEntityPath,
		})
	},
}, {
	about: "CanReadIter",
	run: func(db *jimmdb.Database) {
		it := db.Models().Find(nil).Sort("_id").Iter()
		crit := db.NewCanReadIter(jemtest.NewIdentity("bob", "bob-group"), it)
		crit.Next(testContext, &mongodoc.Model{})
		crit.Err(testContext)
	},
}, {
	about: "CanReadIter with Close",
	run: func(db *jimmdb.Database) {
		it := db.Models().Find(nil).Sort("_id").Iter()
		crit := db.NewCanReadIter(jemtest.NewIdentity("bob", "bob-group"), it)
		crit.Next(testContext, &mongodoc.Model{})
		crit.Close(testContext)
	},
}, {
	about: "GetCredential",
	run: func(db *jimmdb.Database) {
		db.GetCredential(testContext, &mongodoc.Credential{Path: fakeCredPath})
	},
}, {
	about: "RemoveModel",
	run: func(db *jimmdb.Database) {
		db.RemoveModel(testContext, &mongodoc.Model{
			Path: fakeEntityPath,
		})
	},
}, {
	about: "GetACL",
	run: func(db *jimmdb.Database) {
		db.GetACL(testContext, db.Models(), fakeEntityPath)
	},
}, {
	about: "Grant",
	run: func(db *jimmdb.Database) {
		db.Grant(testContext, db.Controllers(), fakeEntityPath, "t1")
	},
}, {
	about: "GetModel",
	run: func(db *jimmdb.Database) {
		db.GetModel(testContext, &mongodoc.Model{Path: fakeEntityPath})
	},
}, {
	about: "MachinesForModel",
	run: func(db *jimmdb.Database) {
		db.MachinesForModel(testContext, "00000000-0000-0000-0000-000000000000")
	},
}, {
	about: "Revoke",
	run: func(db *jimmdb.Database) {
		db.Revoke(testContext, db.Controllers(), fakeEntityPath, "t1")
	},
}, {
	about: "SetACL",
	run: func(db *jimmdb.Database) {
		db.SetACL(testContext, db.Models(), fakeEntityPath, params.ACL{
			Read: []string{"t1", "t2"},
		})
	},
}, {
	about: "SetModelController",
	run: func(db *jimmdb.Database) {
		db.SetModelController(testContext, fakeEntityPath, fakeEntityPath)
	},
}, {
	about: "UpsertCredential",
	run: func(db *jimmdb.Database) {
		db.UpsertCredential(testContext, &mongodoc.Credential{
			Path:  fakeCredPath,
			Type:  "credtype",
			Label: "Test Label",
		})
	},
}, {
	about: "UpdateMachineInfo",
	run: func(db *jimmdb.Database) {
		db.UpdateMachineInfo(testContext, &mongodoc.Machine{
			Controller: "test/test",
			Info: &jujuparams.MachineInfo{
				ModelUUID: "xxx",
				Id:        "yyy",
			},
		})
	},
}, {
	about: "UpdateApplicationInfo",
	run: func(db *jimmdb.Database) {
		db.UpdateApplicationInfo(testContext, &mongodoc.Application{
			Controller: "test/test",
			Info: &mongodoc.ApplicationInfo{
				ModelUUID: "xxx",
				Name:      "yyy",
			},
		})
	},
}}

func (s *databaseSuite) TestSetDead(c *gc.C) {
	session := jujutesting.NewProxiedSession(c)
	defer session.Close()

	pool := mgosession.NewPool(context.TODO(), session.Session, 1)
	defer pool.Close()
	for i, test := range setDeadTests {
		c.Logf("test %d: %s", i, test.about)
		testSetDead(c, session.TCPProxy, pool, test.run)
	}
}

func testSetDead(c *gc.C, proxy *jujutesting.TCPProxy, pool *mgosession.Pool, run func(db *jimmdb.Database)) {
	db := jimmdb.NewDatabase(context.TODO(), pool, "jem")
	defer db.Session.Close()
	// Use the session so that it's bound to the socket.
	err := db.Session.Ping()
	c.Assert(err, gc.Equals, nil)
	proxy.CloseConns() // Close the existing socket so that the connection is broken.

	// Sanity check that getting another session from the pool also
	// gives us a broken session (note that we know that the
	// pool only contains one session).
	s1 := pool.Session(context.TODO())
	defer s1.Close()
	c.Assert(s1.Ping(), gc.NotNil)

	run(db)

	// Check that another session from the pool is OK to use now
	// because the operation has reset the pool.
	s2 := pool.Session(context.TODO())
	defer s2.Close()
	c.Assert(s2.Ping(), gc.Equals, nil)
}

func credentialPath(cloud, user, name string) params.CredentialPath {
	return params.CredentialPath{
		Cloud: params.Cloud(cloud),
		User:  params.User(user),
		Name:  params.CredentialName(name),
	}
}

func mgoCredentialPath(cloud, user, name string) mongodoc.CredentialPath {
	return mongodoc.CredentialPath{
		Cloud: cloud,
		EntityPath: mongodoc.EntityPath{
			User: user,
			Name: name,
		},
	}
}

func (s *databaseSuite) TestLegacyModelCredentials(c *gc.C) {
	ctlPath := params.EntityPath{"bob", "x"}
	m := struct {
		Id            string `bson:"_id"`
		Path          params.EntityPath
		Cloud         params.Cloud
		CloudRegion   string `bson:",omitempty"`
		DefaultSeries string
		Credential    legacyCredentialPath
	}{
		Id:            ctlPath.String(),
		Path:          ctlPath,
		Cloud:         "bob-cloud",
		CloudRegion:   "bob-region",
		DefaultSeries: "trusty",
		Credential: legacyCredentialPath{
			Cloud: "bob-cloud",
			EntityPath: params.EntityPath{
				User: params.User("bob"),
				Name: params.Name("test-credentials"),
			},
		},
	}
	err := s.database.Models().Insert(m)
	c.Assert(err, gc.Equals, nil)

	m1 := mongodoc.Model{Path: ctlPath}
	err = s.database.GetModel(testContext, &m1)
	c.Assert(err, gc.Equals, nil)
	c.Assert(m1, jemtest.CmpEquals(cmpopts.EquateEmpty()), mongodoc.Model{
		Id:            m.Id,
		Path:          m.Path,
		Cloud:         m.Cloud,
		CloudRegion:   m.CloudRegion,
		DefaultSeries: m.DefaultSeries,
		Credential: mongodoc.CredentialPath{
			Cloud: "bob-cloud",
			EntityPath: mongodoc.EntityPath{
				User: "bob",
				Name: "test-credentials",
			},
		},
	})
}

func (s *databaseSuite) TestApplicationOffers(c *gc.C) {
	offer := mongodoc.ApplicationOffer{
		OfferUUID:              "00000000-0000-0000-0000-000000000001",
		OfferURL:               "user1@external/test-model:test-offer1",
		OwnerName:              "user1@external",
		ModelUUID:              "00000000-0000-0000-0000-000000000002",
		ModelName:              "test-model",
		OfferName:              "test-offer1",
		ApplicationName:        "test-application",
		ApplicationDescription: "test description",
		Endpoints: []mongodoc.RemoteEndpoint{{
			Name: "ep1",
		}, {
			Name: "ep2",
		}, {
			Name: "ep3",
		}},
	}

	err := s.database.AddApplicationOffer(context.Background(), &offer)
	c.Assert(err, gc.Equals, nil)

	err = s.database.AddApplicationOffer(context.Background(), &offer)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrAlreadyExists)

	offer1 := mongodoc.ApplicationOffer{
		OfferUUID: offer.OfferUUID,
	}
	err = s.database.GetApplicationOffer(context.Background(), &offer1)
	c.Assert(err, gc.Equals, nil)
	c.Assert(offer1, jemtest.CmpEquals(cmpopts.EquateEmpty()), offer)

	offer2 := mongodoc.ApplicationOffer{
		OfferUUID: "no-such-offer",
	}
	err = s.database.GetApplicationOffer(context.Background(), &offer2)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)

	update := offer
	update.OfferName = "another-test-offer"
	update.ApplicationName = "another-test-application"
	update.Endpoints = []mongodoc.RemoteEndpoint{{
		Name: "ep4",
	}}
	err = s.database.UpdateApplicationOffer(context.Background(), &update)
	c.Assert(err, gc.Equals, nil)

	offer3 := mongodoc.ApplicationOffer{
		OfferUUID: offer.OfferUUID,
	}
	err = s.database.GetApplicationOffer(context.Background(), &offer3)
	c.Assert(err, gc.Equals, nil)
	c.Assert(offer3, jemtest.CmpEquals(cmpopts.EquateEmpty()), update)

	newUpdate := update
	newUpdate.OfferUUID = "no such offer"
	err = s.database.UpdateApplicationOffer(context.Background(), &newUpdate)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)

	offer4 := mongodoc.ApplicationOffer{
		OfferURL: update.OfferURL,
	}
	err = s.database.GetApplicationOffer(context.Background(), &offer4)
	c.Assert(err, gc.Equals, nil)
	c.Assert(offer4, jemtest.CmpEquals(cmpopts.EquateEmpty()), update)

	offer5 := mongodoc.ApplicationOffer{
		OfferURL: "no such offer",
	}
	err = s.database.GetApplicationOffer(context.Background(), &offer5)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)

	err = s.database.RemoveApplicationOffer(context.Background(), update.OfferUUID)
	c.Assert(err, gc.Equals, nil)

	offer6 := mongodoc.ApplicationOffer{
		OfferUUID: update.OfferUUID,
	}
	err = s.database.GetApplicationOffer(context.Background(), &offer6)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)

	err = s.database.RemoveApplicationOffer(context.Background(), update.OfferUUID)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)
}

func (s *databaseSuite) TestIterApplicationOffers(c *gc.C) {
	m1 := utils.MustNewUUID().String()
	m2 := utils.MustNewUUID().String()
	offer1 := mongodoc.ApplicationOffer{
		OfferUUID:              "00000000-0000-0000-0000-000000000002",
		OwnerName:              "bob@external",
		ModelUUID:              m1,
		ModelName:              "test-model1",
		OfferName:              "test offer 1",
		ApplicationName:        "test application",
		ApplicationDescription: "test description",
		Endpoints: []mongodoc.RemoteEndpoint{{
			Name: "ep1",
		}, {
			Name: "ep2",
		}, {
			Name: "ep3",
		}},
		Users: []mongodoc.OfferUserDetails{{
			User:   identchecker.Everyone,
			Access: mongodoc.ApplicationOfferReadAccess,
		}, {
			User:   "alice",
			Access: mongodoc.ApplicationOfferConsumeAccess,
		}, {
			User:   "bob",
			Access: mongodoc.ApplicationOfferAdminAccess,
		}},
	}
	offer2 := mongodoc.ApplicationOffer{
		OfferUUID:              "00000000-0000-0000-0000-000000000003",
		OwnerName:              "bob@external",
		ModelUUID:              m1,
		ModelName:              "test-model1",
		OfferName:              "test offer 2",
		ApplicationName:        "test application 1",
		ApplicationDescription: "test description 1",
		Endpoints: []mongodoc.RemoteEndpoint{{
			Name: "ep1",
		}, {
			Name: "ep2",
		}, {
			Name: "ep3",
		}},
		Users: []mongodoc.OfferUserDetails{{
			User:   identchecker.Everyone,
			Access: mongodoc.ApplicationOfferReadAccess,
		}, {
			User:   "alice",
			Access: mongodoc.ApplicationOfferConsumeAccess,
		}, {
			User:   "bob",
			Access: mongodoc.ApplicationOfferAdminAccess,
		}},
	}
	offer3 := mongodoc.ApplicationOffer{
		OfferUUID:              "00000000-0000-0000-0000-000000000004",
		OwnerName:              "bob@external",
		ModelUUID:              m2,
		ModelName:              "test-model2",
		OfferName:              "test offer 1",
		ApplicationName:        "test application 2",
		ApplicationDescription: "test description 2",
		Endpoints: []mongodoc.RemoteEndpoint{{
			Name: "ep1",
		}, {
			Name: "ep2",
		}, {
			Name: "ep3",
		}},
		Users: []mongodoc.OfferUserDetails{{
			User:   identchecker.Everyone,
			Access: mongodoc.ApplicationOfferReadAccess,
		}, {
			User:   "alice",
			Access: mongodoc.ApplicationOfferConsumeAccess,
		}, {
			User:   "bob",
			Access: mongodoc.ApplicationOfferAdminAccess,
		}},
	}
	offer4 := mongodoc.ApplicationOffer{
		OfferUUID:              "00000000-0000-0000-0000-000000000005",
		OwnerName:              "bob@external",
		ModelUUID:              m2,
		ModelName:              "test-model2",
		OfferName:              "test offer 2",
		ApplicationName:        "test application 3",
		ApplicationDescription: "test description 3",
		Endpoints: []mongodoc.RemoteEndpoint{{
			Name: "ep1",
		}, {
			Name: "ep2",
		}, {
			Name: "ep3",
		}},
		Users: []mongodoc.OfferUserDetails{{
			User:   identchecker.Everyone,
			Access: mongodoc.ApplicationOfferReadAccess,
		}, {
			User:   "alice",
			Access: mongodoc.ApplicationOfferConsumeAccess,
		}, {
			User:   "bob",
			Access: mongodoc.ApplicationOfferAdminAccess,
		}},
	}
	for _, offer := range []mongodoc.ApplicationOffer{offer1, offer2, offer3, offer4} {
		err := s.database.AddApplicationOffer(context.Background(), &offer)
		c.Assert(err, gc.Equals, nil)
	}

	listApplicationOffers := func(
		ctx context.Context,
		user params.User,
		access mongodoc.ApplicationOfferAccessPermission,
		filters []jujuparams.OfferFilter,
	) ([]mongodoc.ApplicationOffer, error) {
		it := s.database.IterApplicationOffers(ctx, user, access, filters)
		defer it.Close()
		var offers []mongodoc.ApplicationOffer
		var doc mongodoc.ApplicationOffer
		for it.Next(&doc) {
			offers = append(offers, doc)
		}
		return offers, errgo.Mask(it.Err())
	}

	offers, err := listApplicationOffers(context.Background(), "bob", mongodoc.ApplicationOfferReadAccess, []jujuparams.OfferFilter{{
		OwnerName: "no such user",
		ModelName: "no such model",
	}})
	c.Assert(err, gc.Equals, nil)
	c.Assert(offers, gc.HasLen, 0)

	offers, err = listApplicationOffers(context.Background(), "bob", mongodoc.ApplicationOfferReadAccess, []jujuparams.OfferFilter{{
		OwnerName: "bob@external",
		ModelName: "test-model1",
	}})
	c.Assert(err, gc.Equals, nil)
	c.Assert(offers, jemtest.CmpEquals(cmpopts.EquateEmpty()), []mongodoc.ApplicationOffer{offer1, offer2})

	offers, err = listApplicationOffers(context.Background(), "bob", mongodoc.ApplicationOfferReadAccess, []jujuparams.OfferFilter{{
		OwnerName: "bob@external",
		ModelName: "test-model2",
	}})
	c.Assert(err, gc.Equals, nil)
	c.Assert(offers, jemtest.CmpEquals(cmpopts.EquateEmpty()), []mongodoc.ApplicationOffer{offer3, offer4})

	offers, err = listApplicationOffers(
		context.Background(),
		"bob",
		mongodoc.ApplicationOfferReadAccess,
		[]jujuparams.OfferFilter{{
			OwnerName:           "bob@external",
			ModelName:           "test-model1",
			AllowedConsumerTags: []string{"user1"},
		}},
	)
	c.Assert(err, gc.Equals, nil)
	c.Assert(offers, gc.DeepEquals, []mongodoc.ApplicationOffer(nil))

	err = s.database.SetApplicationOfferAccess(context.Background(), "user1", offer1.OfferUUID, mongodoc.ApplicationOfferAdminAccess)
	c.Assert(err, gc.Equals, nil)

	offers, err = listApplicationOffers(
		context.Background(),
		"bob",
		mongodoc.ApplicationOfferReadAccess,
		[]jujuparams.OfferFilter{{
			OwnerName:           "bob@external",
			ModelName:           "test-model1",
			AllowedConsumerTags: []string{names.NewUserTag("user1@external").String()},
		}},
	)
	c.Assert(err, gc.Equals, nil)
	c.Assert(offers, gc.HasLen, 1)
	c.Assert(offers[0].Users, cmpUsers, []mongodoc.OfferUserDetails{{
		User:   identchecker.Everyone,
		Access: mongodoc.ApplicationOfferReadAccess,
	}, {
		User:   "alice",
		Access: mongodoc.ApplicationOfferConsumeAccess,
	}, {
		User:   "bob",
		Access: mongodoc.ApplicationOfferAdminAccess,
	}, {
		User:   "user1",
		Access: mongodoc.ApplicationOfferAdminAccess,
	}})
	offers[0].Users = offer1.Users
	c.Assert(offers, jemtest.CmpEquals(cmpopts.EquateEmpty()), []mongodoc.ApplicationOffer{offer1})

	err = s.database.SetApplicationOfferAccess(context.Background(), "user1", offer2.OfferUUID, mongodoc.ApplicationOfferAdminAccess)
	c.Assert(err, gc.Equals, nil)

	offers, err = listApplicationOffers(
		context.Background(),
		"bob",
		mongodoc.ApplicationOfferReadAccess,
		[]jujuparams.OfferFilter{{
			OwnerName:           "bob@external",
			ModelName:           "test-model1",
			AllowedConsumerTags: []string{names.NewUserTag("user1@external").String()},
			ApplicationName:     offer2.ApplicationName,
		}},
	)
	c.Assert(err, gc.Equals, nil)
	c.Assert(offers, gc.HasLen, 1)
	c.Assert(offers[0].Users, cmpUsers, []mongodoc.OfferUserDetails{{
		User:   identchecker.Everyone,
		Access: mongodoc.ApplicationOfferReadAccess,
	}, {
		User:   "alice",
		Access: mongodoc.ApplicationOfferConsumeAccess,
	}, {
		User:   "bob",
		Access: mongodoc.ApplicationOfferAdminAccess,
	}, {
		User:   "user1",
		Access: mongodoc.ApplicationOfferAdminAccess,
	}})
	offers[0].Users = offer2.Users
	c.Assert(offers, jemtest.CmpEquals(cmpopts.EquateEmpty()), []mongodoc.ApplicationOffer{offer2})
}

func (s *databaseSuite) TestApplicationOfferAccess(c *gc.C) {
	ctx := context.Background()
	err := s.database.AddApplicationOffer(ctx, &mongodoc.ApplicationOffer{
		OfferUUID:              "00000000-0000-0000-0000-000000000010",
		OwnerName:              "bob@external",
		ModelUUID:              "00000000-0000-0000-0000-000000000001",
		ModelName:              "test-model1",
		OfferName:              "test offer 1",
		ApplicationName:        "test application",
		ApplicationDescription: "test description",
		Endpoints: []mongodoc.RemoteEndpoint{{
			Name: "ep1",
		}, {
			Name: "ep2",
		}, {
			Name: "ep3",
		}},
	})
	c.Assert(err, gc.Equals, nil)

	err = s.database.SetApplicationOfferAccess(ctx, "user1", "00000000-0000-0000-0000-000000000010", mongodoc.ApplicationOfferReadAccess)
	c.Assert(err, gc.Equals, nil)

	err = s.database.SetApplicationOfferAccess(ctx, "user2", "00000000-0000-0000-0000-000000000010", mongodoc.ApplicationOfferConsumeAccess)
	c.Assert(err, gc.Equals, nil)

	err = s.database.SetApplicationOfferAccess(ctx, "user3", "00000000-0000-0000-0000-000000000010", mongodoc.ApplicationOfferAdminAccess)

	access, err := s.database.GetApplicationOfferAccess(ctx, "user1", "00000000-0000-0000-0000-000000000010")
	c.Assert(err, gc.Equals, nil)
	c.Assert(access, gc.Equals, mongodoc.ApplicationOfferReadAccess)
}

func (s *databaseSuite) TestApplicationOfferAccessMultipleTimes(c *gc.C) {
	ctx := context.Background()
	err := s.database.AddApplicationOffer(ctx, &mongodoc.ApplicationOffer{
		OfferUUID:              "00000000-0000-0000-0000-000000000010",
		OwnerName:              "bob@external",
		ModelUUID:              "00000000-0000-0000-0000-000000000001",
		ModelName:              "test-model1",
		OfferName:              "test offer 1",
		ApplicationName:        "test application",
		ApplicationDescription: "test description",
		Endpoints: []mongodoc.RemoteEndpoint{{
			Name: "ep1",
		}, {
			Name: "ep2",
		}, {
			Name: "ep3",
		}},
	})
	c.Assert(err, gc.Equals, nil)

	err = s.database.SetApplicationOfferAccess(ctx, "user1", "00000000-0000-0000-0000-000000000010", mongodoc.ApplicationOfferReadAccess)
	c.Assert(err, gc.Equals, nil)
	err = s.database.SetApplicationOfferAccess(ctx, "user1", "00000000-0000-0000-0000-000000000010", mongodoc.ApplicationOfferReadAccess)
	c.Assert(err, gc.Equals, nil)
	err = s.database.SetApplicationOfferAccess(ctx, "user1", "00000000-0000-0000-0000-000000000010", mongodoc.ApplicationOfferReadAccess)
	c.Assert(err, gc.Equals, nil)

	offer := mongodoc.ApplicationOffer{
		OfferUUID: "00000000-0000-0000-0000-000000000010",
	}
	err = s.database.GetApplicationOffer(ctx, &offer)
	c.Assert(err, gc.Equals, nil)

	c.Check(offer.Users, gc.HasLen, 1)
	c.Check(offer.Users[0], jc.DeepEquals, mongodoc.OfferUserDetails{
		User:   "user1",
		Access: mongodoc.ApplicationOfferReadAccess,
	})
}

var cmpUsers = jemtest.CmpEquals(cmpopts.SortSlices(func(a, b mongodoc.OfferUserDetails) bool {
	return a.User < b.User
}))
