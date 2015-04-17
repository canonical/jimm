// Copyright 2015 Canonical Ltd.

package jem

import (
	"gopkg.in/errgo.v1"
	"gopkg.in/macaroon-bakery.v0/bakery"
	"gopkg.in/macaroon-bakery.v0/bakery/mgostorage"
	"gopkg.in/mgo.v2"
)

type Pool struct {
	db     Database
	bakery *bakery.Service
}

// NewPool represents a pool of possible JEM instances that use the given
// database as a store, and use the given bakery parameters to create the
// bakery.Service.
func NewPool(db *mgo.Database, bakeryParams *bakery.NewServiceParams) (*Pool, error) {
	pool := &Pool{
		db: Database{db},
	}
	// TODO migrate database
	macStore, err := mgostorage.New(pool.db.Macaroons())
	if err != nil {
		return nil, errgo.Notef(err, "cannot create macaroon store")
	}
	p := *bakeryParams
	p.Store = macStore
	bsvc, err := bakery.NewService(p)
	if err != nil {
		return nil, errgo.Notef(err, "cannot make bakery service")
	}
	pool.bakery = bsvc
	return pool, nil
}

// JEM returns a new JEM instance from the pool, suitable
// for using in short-lived requests. The JEM must be
// closed with the Close method after use.
func (p *Pool) JEM() *JEM {
	return &JEM{
		DB:     p.db.Copy(),
		Bakery: p.bakery,
	}
}

type JEM struct {
	// DB holds the mongodb-backed identity store.
	DB Database

	// Bakery holds the JEM bakery service.
	Bakery *bakery.Service
}

func (j *JEM) Close() {
	j.DB.Close()
	j.DB = Database{}
}

// Database wraps an mgo.DB ands adds a few convenience methods.
type Database struct {
	*mgo.Database
}

// Copy copies the Database and its underlying mgo session.
func (s Database) Copy() Database {
	return Database{
		&mgo.Database{
			Name:    s.Name,
			Session: s.Session.Copy(),
		},
	}
}

// Close closes the database's underlying session.
func (db Database) Close() {
	db.Session.Close()
}

func (db Database) Macaroons() *mgo.Collection {
	return db.C("macaroons")
}

func (db Database) StateServers() *mgo.Collection {
	return db.C("stateservers")
}
