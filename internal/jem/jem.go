// Copyright 2015 Canonical Ltd.

package jem

import (
	"sync"
	"time"

	"github.com/CanonicalLtd/blues-identity/idmclient"
	"github.com/juju/juju/api"
	"github.com/juju/names"
	"gopkg.in/errgo.v1"
	"gopkg.in/macaroon-bakery.v1/bakery"
	"gopkg.in/macaroon-bakery.v1/bakery/mgostorage"
	"gopkg.in/mgo.v2"

	"github.com/CanonicalLtd/jem/internal/apiconn"
	"github.com/CanonicalLtd/jem/internal/mongodoc"
	"github.com/CanonicalLtd/jem/params"
)

type Pool struct {
	db           Database
	bakery       *bakery.Service
	connCache    *apiconn.Cache
	bakeryParams bakery.NewServiceParams
	permChecker  *idmclient.PermChecker

	mu       sync.Mutex
	closed   bool
	refCount int
}

var APIOpenTimeout = 15 * time.Second

const maxPermCacheDuration = 10 * time.Second

// NewPool represents a pool of possible JEM instances that use the given
// database as a store, and use the given bakery parameters to create the
// bakery.Service.
func NewPool(db *mgo.Database, bp bakery.NewServiceParams, idmClient *idmclient.Client) (*Pool, error) {
	// TODO migrate database
	pool := &Pool{
		db:          Database{db},
		connCache:   apiconn.NewCache(apiconn.CacheParams{}),
		permChecker: idmclient.NewPermChecker(idmClient, maxPermCacheDuration),
		refCount:    1,
	}
	// Fill out any bakery parameters explicitly here so
	// that we use the same values when each Store is
	// created. We don't fill out bp.Store field though, as
	// that needs to hold the correct mongo session which we
	// only know when the Store is created from the Pool.
	if bp.Key == nil {
		var err error
		bp.Key, err = bakery.GenerateKey()
		if err != nil {
			return nil, errgo.Notef(err, "cannot generate bakery key")
		}
	}
	if bp.Locator == nil {
		bp.Locator = bakery.PublicKeyLocatorMap(nil)
	}
	pool.bakeryParams = bp
	return pool, nil
}

// Close closes the pool. Its resources will be freed
// when the last JEM instance created from the pool has
// been closed.
func (p *Pool) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return
	}
	p.decRef()
	p.closed = true
}

func (p *Pool) decRef() {
	// called with p.mu held.
	if p.refCount--; p.refCount == 0 {
		p.connCache.Close()
	}
	if p.refCount < 0 {
		panic("negative reference count")
	}
}

// ClearAPIConnCache clears out the API connection cache.
// This is useful for testing purposes.
func (p *Pool) ClearAPIConnCache() {
	p.connCache.EvictAll()
}

// JEM returns a new JEM instance from the pool, suitable
// for using in short-lived requests. The JEM must be
// closed with the Close method after use.
//
// This method will panic if called after the pool has been
// closed.
func (p *Pool) JEM() *JEM {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		panic("JEM call on closed pool")
	}
	db := p.db.Copy()
	p.refCount++
	return &JEM{
		DB:          db,
		Bakery:      newBakery(db, p.bakeryParams),
		PermChecker: p.permChecker,
		pool:        p,
	}
}

func newBakery(db Database, bp bakery.NewServiceParams) *bakery.Service {
	macStore, err := mgostorage.New(db.Macaroons())
	if err != nil {
		// Should never happen.
		panic(errgo.Newf("unexpected error from mgostorage.New: %v", err))
	}
	bp.Store = macStore
	bsvc, err := bakery.NewService(bp)
	if err != nil {
		// This should never happen because the only reason bakery.NewService
		// can fail is if it can't generate a key, and we have already made
		// sure that the key is generated.
		panic(errgo.Notef(err, "cannot make bakery service"))
	}
	return bsvc
}

type JEM struct {
	// DB holds the mongodb-backed identity store.
	DB Database

	// Bakery holds the JEM bakery service.
	Bakery *bakery.Service

	PermChecker *idmclient.PermChecker

	// pool holds the Pool from which the JEM instance
	// was created.
	pool *Pool

	// closed records whether the JEM instance has
	// been closed.
	closed bool
}

// Close closes the JEM instance. This should be called when
// the JEM instance is finished with.
func (j *JEM) Close() {
	j.pool.mu.Lock()
	defer j.pool.mu.Unlock()
	if j.closed {
		return
	}
	j.closed = true
	j.DB.Close()
	j.DB = Database{}
	j.pool.decRef()
}

// AddStateServer adds a new state server and its associated environment
// to the database. It returns an error with a params.ErrAlreadyExists
// cause if there is already a state server with the given name.
// The Id field in srv will be set from its Path field,
// and the Id, Path and StateServer fields in env will also be
// set from srv.
func (j *JEM) AddStateServer(srv *mongodoc.StateServer, env *mongodoc.Environment) error {
	// Insert the environment before inserting the state server
	// to avoid races with other clients creating non-state-server
	// environments.
	srv.Id = srv.Path.String()
	env.Path = srv.Path
	env.StateServer = srv.Path
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
	env.Id = env.Path.String()
	err := j.DB.Environments().Insert(env)
	if mgo.IsDup(err) {
		return errgo.WithCausef(nil, params.ErrAlreadyExists, "")
	}
	if err != nil {
		return errgo.Notef(err, "cannot insert state server environment")
	}
	return nil
}

// StateServer returns information on the state server with the given
// path. It returns an error with a params.ErrNotFound cause if the
// state server was not found.
func (j *JEM) StateServer(path params.EntityPath) (*mongodoc.StateServer, error) {
	var srv mongodoc.StateServer
	id := path.String()
	err := j.DB.StateServers().FindId(id).One(&srv)
	if err == mgo.ErrNotFound {
		return nil, errgo.WithCausef(nil, params.ErrNotFound, "state server %q not found", id)
	}
	if err != nil {
		return nil, errgo.Notef(err, "cannot get state server %q", id)
	}
	return &srv, nil
}

// Environment returns information on the environment with the given
// path. It returns an error with a params.ErrNotFound cause if the
// state server was not found.
func (j *JEM) Environment(path params.EntityPath) (*mongodoc.Environment, error) {
	id := path.String()
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

// OpenAPI opens an API connection to the environment with the given path
// and returns it along with the information used to connect.
// If the environment does not exist, the error will have a cause
// of params.ErrNotFound.
//
// The returned connection must be closed when finished with.
func (j *JEM) OpenAPI(path params.EntityPath) (*apiconn.Conn, error) {
	env, err := j.Environment(path)
	if err != nil {
		return nil, errgo.NoteMask(err, "cannot get environment", errgo.Is(params.ErrNotFound))
	}
	return j.pool.connCache.OpenAPI(env.UUID, func() (api.Connection, *api.Info, error) {
		srv, err := j.StateServer(env.StateServer)
		if err != nil {
			return nil, nil, errgo.Notef(err, "cannot get state server for environment %q", env.UUID)
		}
		apiInfo := apiInfoFromDocs(env, srv)
		st, err := api.Open(apiInfo, apiDialOpts())
		if err != nil {
			return nil, nil, errgo.Mask(err)
		}
		return st, apiInfo, nil
	})
}

// OpenAPIFromDocs returns an API connection to the environment
// and state server held in the given documents. This can
// be useful when we want to connect to an environment
// before it's added to the database (for example when adding
// a new state server). Note that a successful return from this
// function does not necessarily mean that the credentials or
// API addresses in the docs actually work, as it's possible
// that there's already a cached connection for the given environment.
//
// The returned connection must be closed when finished with.
func (j *JEM) OpenAPIFromDocs(env *mongodoc.Environment, srv *mongodoc.StateServer) (*apiconn.Conn, error) {
	return j.pool.connCache.OpenAPI(env.UUID, func() (api.Connection, *api.Info, error) {
		stInfo := apiInfoFromDocs(env, srv)
		st, err := api.Open(stInfo, apiDialOpts())
		if err != nil {
			return nil, nil, errgo.Mask(err)
		}
		return st, stInfo, nil
	})
}

// AddTemplate adds the given template to the database.
// If there is already an existing template with the same
// name, it is overwritten. The Id field in template will be set from its
// Path field. It is the responsibility of the caller to
// ensure that the template attributes are compatible
// with the template schema.
func (j *JEM) AddTemplate(tmpl *mongodoc.Template) error {
	tmpl.Id = tmpl.Path.String()
	_, err := j.DB.Templates().UpsertId(tmpl.Id, tmpl)
	if err != nil {
		return errgo.Notef(err, "cannot add template doc")
	}
	return nil
}

// Template returns information on the template with the given path.
// It returns an error with a params.ErrNotFound cause
// if the template was not found.
func (j *JEM) Template(path params.EntityPath) (*mongodoc.Template, error) {
	var tmpl mongodoc.Template
	err := j.DB.Templates().FindId(path.String()).One(&tmpl)
	if err == mgo.ErrNotFound {
		return nil, errgo.WithCausef(nil, params.ErrNotFound, "template %q not found", path)
	}
	if err != nil {
		return nil, errgo.Notef(err, "cannot get template %q", path)
	}
	return &tmpl, nil
}

func apiDialOpts() api.DialOpts {
	return api.DialOpts{
		Timeout:    APIOpenTimeout,
		RetryDelay: 500 * time.Millisecond,
	}
}

func apiInfoFromDocs(env *mongodoc.Environment, srv *mongodoc.StateServer) *api.Info {
	return &api.Info{
		Addrs:      srv.HostPorts,
		CACert:     srv.CACert,
		Tag:        names.NewUserTag(env.AdminUser),
		Password:   env.AdminPassword,
		EnvironTag: names.NewEnvironTag(env.UUID),
	}
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

func (db Database) Templates() *mgo.Collection {
	return db.C("templates")
}
