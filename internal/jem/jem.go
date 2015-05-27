// Copyright 2015 Canonical Ltd.

package jem

import (
	"gopkg.in/errgo.v1"
	"gopkg.in/macaroon-bakery.v1/bakery"
	"gopkg.in/macaroon-bakery.v1/bakery/mgostorage"
	"gopkg.in/mgo.v2"

	"github.com/CanonicalLtd/jem/internal/mongodoc"
	"github.com/CanonicalLtd/jem/params"
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

// AddStateServer adds a new state server and its associated environment
// to the database. It returns an error with a params.ErrAlreadyExists
// cause if there is already a state server with the given name.
// The Id field in srv will be set from its User and Name fields,
// and the Id, User, Name and StateServer fields in env will also be
// set from srv.
func (j *JEM) AddStateServer(srv *mongodoc.StateServer, env *mongodoc.Environment) error {
	// Insert the environment before inserting the state server
	// to avoid races with other clients creating non-state-server
	// environments.
	srv.Id = entityPathToId(srv.User, srv.Name)
	env.User = srv.User
	env.Name = srv.Name
	env.StateServer = srv.Id
	err := j.AddEnvironment(env)
	if err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrAlreadyExists))
	}
	err = j.DB.StateServers().Insert(srv)
	if err != nil {
		// Since we always insert an environment of the
		// same name first, this should never happen,
		// so we don't preserve the ErrAlreadyExists
		// error here because failing in that way is
		// really an internal server error.
		return errgo.Notef(err, "cannot insert state server")
	}
	return nil
}

// AddEnvironment adds a new environment to the database.
// It returns an error with a params.ErrAlreadyExists
// cause if there is already an environment with the given name.
// The Id field in env will be set from its User and Name fields
// before insertion.
func (j *JEM) AddEnvironment(env *mongodoc.Environment) error {
	env.Id = entityPathToId(env.User, env.Name)
	err := j.DB.Environments().Insert(env)
	if mgo.IsDup(err) {
		return errgo.WithCausef(nil, params.ErrAlreadyExists, "")
	}
	if err != nil {
		return errgo.Notef(err, "cannot insert state server environment")
	}
	return nil
}

func entityPathToId(user params.User, name params.Name) string {
	return string(user) + "/" + string(name)
}

// StateServer returns state server information for
// the state server with the given id (in the form "$user/$name").
// It returns an error with a params.ErrNotFound cause
// if the state server was not found.
func (j *JEM) StateServer(id string) (*mongodoc.StateServer, error) {
	var srv mongodoc.StateServer
	err := j.DB.StateServers().FindId(id).One(&srv)
	if err == mgo.ErrNotFound {
		return nil, errgo.WithCausef(nil, params.ErrNotFound, "state server %q not found", id)
	}
	if err != nil {
		return nil, errgo.Notef(err, "cannot get state server %q", id)
	}
	return &srv, nil
}

// Environment returns environment information for
// the environment with the given id (in the form "$user/$name").
// It returns an error with a params.ErrNotFound cause
// if the state server was not found.
func (j *JEM) Environment(id string) (*mongodoc.Environment, error) {
	var env mongodoc.Environment
	err := j.DB.Environments().FindId(id).One(&env)
	if err == mgo.ErrNotFound {
		return nil, errgo.WithCausef(nil, params.ErrNotFound, "environment %q not found", id)
	}
	if err != nil {
		return nil, errgo.Notef(err, "cannot get environment %q", id)
	}
	return &env, nil
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

func (db Database) Environments() *mgo.Collection {
	return db.C("environments")
}
