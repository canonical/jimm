// Copyright 2015 Canonical Ltd.

package jem

import (
	"gopkg.in/errgo.v1"
	"gopkg.in/macaroon-bakery.v0/bakery"
	"gopkg.in/macaroon-bakery.v0/bakery/mgostorage"
	"gopkg.in/mgo.v2"
)

type JEM struct {
	// DB holds the mongodb-backed identity store.
	DB Database

	// Bakery holds the JEM bakery service.
	Bakery *bakery.Service
}

func New(db *mgo.Database, bakeryParams *bakery.NewServiceParams) (*JEM, error) {
	j := &JEM{
		DB: Database{db},
	}
	// TODO migrate database
	macStore, err := mgostorage.New(j.DB.Macaroons())
	if err != nil {
		return nil, errgo.Notef(err, "cannot create macaroon store")
	}
	p := *bakeryParams
	p.Store = macStore
	bsvc, err := bakery.NewService(p)
	if err != nil {
		return nil, errgo.Notef(err, "cannot make bakery service")
	}
	j.Bakery = bsvc
	return j, nil
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
