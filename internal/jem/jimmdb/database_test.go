// Copyright 2016 Canonical Ltd.

package jimmdb_test

import (
	"context"
	"time"

	jujuparams "github.com/juju/juju/apiserver/params"
	jujutesting "github.com/juju/testing"
	gc "gopkg.in/check.v1"

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

var (
	fakeEntityPath = params.EntityPath{"bob", "foo"}
	fakeCredPath   = mongodoc.CredentialPathFromParams(credentialPath("test-cloud", "test-user", "test-credential"))
	fakeUpdate     = new(jimmdb.Update).Set("x", "y")
)

var setDeadTests = []struct {
	about string
	run   func(db *jimmdb.Database)
}{{
	about: "UpsertApplication",
	run: func(db *jimmdb.Database) {
		db.UpsertApplication(testContext, &mongodoc.Application{
			Controller: "alice/dummy-1",
			Info: &mongodoc.ApplicationInfo{
				ModelUUID: "00000000-0000-0000-0000-000000000000",
				Name:      "app",
			},
		})
	},
}, {
	about: "ForEachApplication",
	run: func(db *jimmdb.Database) {
		db.ForEachApplication(testContext, nil, nil, func(*mongodoc.Application) error {
			return nil
		})
	},
}, {
	about: "RemoveApplication",
	run: func(db *jimmdb.Database) {
		db.RemoveApplication(testContext, &mongodoc.Application{
			Controller: "alice/dummy-1",
			Info: &mongodoc.ApplicationInfo{
				ModelUUID: "00000000-0000-0000-0000-000000000000",
				Name:      "app",
			},
		})
	},
}, {
	about: "RemoveApplications",
	run: func(db *jimmdb.Database) {
		db.RemoveApplications(testContext, nil)
	},
}, {
	about: "InsertApplicationOffer",
	run: func(db *jimmdb.Database) {
		db.InsertApplicationOffer(testContext, &mongodoc.ApplicationOffer{
			OfferUUID: "dummy",
		})
	},
}, {
	about: "GetApplicationOffer",
	run: func(db *jimmdb.Database) {
		db.GetApplicationOffer(testContext, &mongodoc.ApplicationOffer{
			OfferUUID: "dummy",
		})
	},
}, {
	about: "ForEachApplicationOffer",
	run: func(db *jimmdb.Database) {
		db.ForEachApplicationOffer(testContext, nil, nil, func(*mongodoc.ApplicationOffer) error { return nil })
	},
}, {
	about: "UpdateApplicationOffer",
	run: func(db *jimmdb.Database) {
		db.UpdateApplicationOffer(testContext, &mongodoc.ApplicationOffer{
			OfferUUID: "dummy",
		}, fakeUpdate, true)
	},
}, {
	about: "RemoveApplicationOffer",
	run: func(db *jimmdb.Database) {
		db.RemoveApplicationOffer(testContext, &mongodoc.ApplicationOffer{
			OfferUUID: "dummy",
		})
	},
}, {
	about: "AppendAudit",
	run: func(db *jimmdb.Database) {
		db.AppendAudit(testContext, jemtest.NewIdentity("bob"), &params.AuditModelCreated{})
	},
}, {
	about: "GetAuditEntries",
	run: func(db *jimmdb.Database) {
		db.GetAuditEntries(testContext, time.Time{}, time.Time{}, "")
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
		db.UpdateCloudRegions(testContext, nil, fakeUpdate)
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
		}, fakeUpdate, false)
	},
}, {
	about: "UpdateControllerQuery",
	run: func(db *jimmdb.Database) {
		db.UpdateControllerQuery(testContext, nil, nil, fakeUpdate, false)
	},
}, {
	about: "UpdateControllers",
	run: func(db *jimmdb.Database) {
		db.UpdateControllers(testContext, nil, fakeUpdate)
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
	about: "UpsertMachine",
	run: func(db *jimmdb.Database) {
		db.UpsertMachine(testContext, &mongodoc.Machine{
			Controller: "alice/dummy-1",
			Info: &jujuparams.MachineInfo{
				ModelUUID: "00000000-0000-0000-0000-000000000000",
				Id:        "0",
			},
		})
	},
}, {
	about: "ForEachMachine",
	run: func(db *jimmdb.Database) {
		db.ForEachMachine(testContext, nil, nil, func(*mongodoc.Machine) error {
			return nil
		})
	},
}, {
	about: "RemoveMachine",
	run: func(db *jimmdb.Database) {
		db.RemoveMachine(testContext, &mongodoc.Machine{
			Controller: "alice/dummy-1",
			Info: &jujuparams.MachineInfo{
				ModelUUID: "00000000-0000-0000-0000-000000000000",
				Id:        "0",
			},
		})
	},
}, {
	about: "RemoveMachines",
	run: func(db *jimmdb.Database) {
		db.RemoveMachines(testContext, nil)
	},
}, {
	about: "InsertModel",
	run: func(db *jimmdb.Database) {
		db.InsertModel(testContext, &mongodoc.Model{
			Path: fakeEntityPath,
		})
	},
}, {
	about: "GetModel",
	run: func(db *jimmdb.Database) {
		db.GetModel(testContext, &mongodoc.Model{Path: fakeEntityPath})
	},
}, {
	about: "CountModels",
	run: func(db *jimmdb.Database) {
		db.CountModels(testContext, nil)
	},
}, {
	about: "ForEachModel",
	run: func(db *jimmdb.Database) {
		db.ForEachModel(testContext, nil, nil, nil)
	},
}, {
	about: "UpdateModel",
	run: func(db *jimmdb.Database) {
		db.UpdateModel(testContext, &mongodoc.Model{Path: fakeEntityPath}, fakeUpdate, false)
	},
}, {
	about: "RemoveModel",
	run: func(db *jimmdb.Database) {
		db.RemoveModel(testContext, &mongodoc.Model{
			Path: fakeEntityPath,
		})
	},
}, {
	about: "RemoveModels",
	run: func(db *jimmdb.Database) {
		db.RemoveModels(testContext, nil)
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
