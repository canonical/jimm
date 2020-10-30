// Copyright 2016 Canonical Ltd.

package jimmdb

import (
	"context"
	"fmt"

	"gopkg.in/errgo.v1"
	"gopkg.in/mgo.v2"

	"github.com/CanonicalLtd/jimm/internal/mgosession"
	"github.com/CanonicalLtd/jimm/internal/servermon"
	"github.com/CanonicalLtd/jimm/internal/zapctx"
	"github.com/CanonicalLtd/jimm/internal/zaputil"
	"github.com/CanonicalLtd/jimm/params"
)

// Database wraps an mgo.DB ands adds a number of methods for
// manipulating the database.
type Database struct {
	// sessionPool holds the session pool. This will be
	// reset if there's an unexpected mongodb error.
	sessionPool *mgosession.Pool
	*mgo.Database
}

// checkError inspects the value pointed to by err and marks the database
// connection as dead if it looks like the error is probably
// due to a database connection issue. There may be false positives, but
// the worst that can happen is that we do the occasional unnecessary
// Session.Copy which shouldn't be a problem.
//
// TODO if mgo supported it, a better approach would be to check whether
// the mgo.Session is permanently dead.
func (db *Database) checkError(ctx context.Context, err *error) {
	if *err == nil {
		return
	}
	_, ok := errgo.Cause(*err).(params.ErrorCode)
	if ok {
		return
	}
	db.sessionPool.Reset()

	servermon.DatabaseFailCount.Inc()
	zapctx.Warn(ctx, "discarding mongo session", zaputil.Error(*err))
}

// NewDatabase returns a new Database named dbName using
// a session taken from the given pool. The database session
// should be closed after the database is finished with.
func NewDatabase(ctx context.Context, pool *mgosession.Pool, dbName string) *Database {
	return &Database{
		sessionPool: pool,
		Database:    pool.Session(ctx).DB(dbName),
	}
}

func (db *Database) Clone() *Database {
	return &Database{
		sessionPool: db.sessionPool,
		Database:    db.Database.With(db.Database.Session.Clone()),
	}
}

func (db *Database) EnsureIndexes() error {
	indexes := []struct {
		c *mgo.Collection
		i mgo.Index
	}{{
		db.Controllers(),
		mgo.Index{Key: []string{"uuid"}},
	}, {
		db.Machines(),
		mgo.Index{Key: []string{"info.uuid"}},
	}, {
		db.Applications(),
		mgo.Index{Key: []string{"info.uuid"}},
	}, {
		db.Models(),
		mgo.Index{Key: []string{"uuid"}, Unique: true},
	}, {
		db.Models(),
		mgo.Index{Key: []string{"credential"}},
	}, {
		db.Credentials(),
		mgo.Index{Key: []string{"path.entitypath.user", "path.cloud"}},
	}, {
		db.ApplicationOffers(),
		mgo.Index{Key: []string{"offer-url"}, Unique: true},
	}, {
		db.ApplicationOffers(),
		mgo.Index{Key: []string{"owner-name", "model-name", "offer-name"}, Unique: true},
	}}
	for _, idx := range indexes {
		err := idx.c.EnsureIndex(idx.i)
		if err != nil {
			return errgo.Notef(err, "cannot ensure index with keys %v on collection %s", idx.i, idx.c.Name)
		}
	}
	return nil
}

func (db *Database) Collections() []*mgo.Collection {
	return []*mgo.Collection{
		db.Audits(),
		db.Applications(),
		db.CloudRegions(),
		db.Controllers(),
		db.Credentials(),
		db.Macaroons(),
		db.Machines(),
		db.Models(),
		db.ApplicationOffers(),
	}
}

func (db *Database) Applications() *mgo.Collection {
	return db.C("applications")
}

func (db *Database) Audits() *mgo.Collection {
	return db.C("audits")
}

func (db *Database) CloudRegions() *mgo.Collection {
	return db.C("cloudregions")
}

func (db *Database) Controllers() *mgo.Collection {
	return db.C("controllers")
}

func (db *Database) Credentials() *mgo.Collection {
	return db.C("credentials")
}

func (db *Database) Macaroons() *mgo.Collection {
	return db.C("macaroons")
}

func (db *Database) Machines() *mgo.Collection {
	return db.C("machines")
}

func (db *Database) Models() *mgo.Collection {
	return db.C("models")
}

// ApplicationOffers returns the collection holding application offers.
func (db *Database) ApplicationOffers() *mgo.Collection {
	return db.C("application_offers")
}

func (db *Database) C(name string) *mgo.Collection {
	if db.Database == nil {
		panic(fmt.Sprintf("cannot get collection %q because JEM closed", name))
	}
	return db.Database.C(name)
}
