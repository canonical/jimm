// Copyright 2015 Canonical Ltd.

package jem

import (
	"fmt"
	"sync"
	"time"

	"github.com/juju/idmclient"
	"github.com/juju/juju/api"
	"github.com/juju/loggo"
	"github.com/juju/utils"
	"gopkg.in/errgo.v1"
	"gopkg.in/juju/names.v2"
	"gopkg.in/macaroon-bakery.v1/bakery"
	"gopkg.in/macaroon-bakery.v1/bakery/mgostorage"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"

	"github.com/CanonicalLtd/jem/internal/apiconn"
	"github.com/CanonicalLtd/jem/internal/mongodoc"
	"github.com/CanonicalLtd/jem/params"
)

var logger = loggo.GetLogger("jem.internal.jem")

// Params holds parameters for the NewPool function.
type Params struct {
	// DB holds the mongo database that will be used to
	// store the JEM information.
	DB *mgo.Database

	// BakeryParams holds the parameters for creating
	// a new bakery.Service.
	BakeryParams bakery.NewServiceParams

	// IDMClient holds the identity-manager client
	// to use for finding out group membership.
	IDMClient *idmclient.Client

	// ControllerAdmin holds the identity of the user
	// or group that is allowed to create controllers.
	ControllerAdmin params.User

	// IdentityLocation holds the location of the third party identity service.
	IdentityLocation string
}

type Pool struct {
	db           Database
	config       Params
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

var notExistsQuery = bson.D{{"$exists", false}}

// NewPool represents a pool of possible JEM instances that use the given
// database as a store, and use the given bakery parameters to create the
// bakery.Service.
func NewPool(p Params) (*Pool, error) {
	// TODO migrate database
	if p.ControllerAdmin == "" {
		return nil, errgo.Newf("no controller admin group specified")
	}
	pool := &Pool{
		config:      p,
		db:          Database{p.DB},
		connCache:   apiconn.NewCache(apiconn.CacheParams{}),
		permChecker: idmclient.NewPermChecker(p.IDMClient, maxPermCacheDuration),
		refCount:    1,
	}
	bp := p.BakeryParams
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

	// Auth holds any authorization credentials as set by
	// JEM.Authenticate. If Authenticate has not been called, this
	// will be zero.
	Auth Authorization

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

// Clone returns an independent copy of the receiver
// that uses a cloned database connection. The
// returned value must be closed after use.
func (j *JEM) Clone() *JEM {
	j.pool.mu.Lock()
	defer j.pool.mu.Unlock()
	db := j.DB.Clone()
	j.pool.refCount++
	return &JEM{
		DB:          db,
		Bakery:      newBakery(db, j.pool.bakeryParams),
		PermChecker: j.pool.permChecker,
		pool:        j.pool,
	}
}

func (j *JEM) ControllerAdmin() params.User {
	return j.pool.config.ControllerAdmin
}

// Close closes the JEM instance. This should be called when
// the JEM instance is finished with.
func (j *JEM) Close() {
	j.pool.mu.Lock()
	defer j.pool.mu.Unlock()
	if j.closed {
		return
	}
	j.Auth = Authorization{}
	j.closed = true
	j.DB.Close()
	j.DB = Database{}
	j.pool.decRef()
}

// AddController adds a new controller to the database. It returns an
// error with a params.ErrAlreadyExists cause if there is already a
// controller with the given name. The Id field in ctl will be set from
// its Path field, and the Id, Path and Controller fields in env will
// also be set from ctl. Any empty Location attributes will be removed
// from ctl.Location.
//
// If the provided document isn't valid, AddController with return an
// error with a params.ErrBadRequest cause.
func (j *JEM) AddController(ctl *mongodoc.Controller) error {
	ctl.Id = ctl.Path.String()
	if err := j.DB.Controllers().Insert(ctl); err != nil {
		if mgo.IsDup(errgo.Cause(err)) {
			return params.ErrAlreadyExists
		}
		return errgo.NoteMask(err, "cannot insert controller")
	}
	return nil
}

// randomPassword is defined as a variable so it can be overridden
// for testing purposes.
var randomPassword = utils.RandomPassword

// AddUser ensures that the user exists in the controller with the given
// name. It returns the password for the user, generating
// a new one if necessary.
func (j *JEM) EnsureUser(ctlName params.EntityPath, user string) (string, error) {
	password, err := randomPassword()
	if err != nil {
		return "", errgo.Notef(err, "cannot generate password")
	}
	userKey := mongodoc.Sanitize(user)
	field := "users." + userKey
	err = j.DB.Controllers().Update(bson.D{{
		"_id", ctlName.String(),
	}, {
		field, notExistsQuery,
	}}, bson.D{{
		"$set", bson.D{{
			field, mongodoc.UserInfo{
				Password: password,
			},
		}},
	}})
	if err == nil {
		return password, nil
	}
	if err != mgo.ErrNotFound {
		return "", errgo.Notef(err, "cannot update user entry")
	}
	// The entry wasn't found. This was probably
	// because the user entry already exists.
	ctl, err := j.Controller(ctlName)
	if err != nil {
		return "", errgo.Notef(err, "cannot get controller")
	}
	if info, ok := ctl.Users[userKey]; ok {
		return info.Password, nil
	}
	return "", errgo.Newf("controller exists but password couldn't be updated")
}

func (j *JEM) SetModelManagedUser(modelName params.EntityPath, user string, info mongodoc.ModelUserInfo) error {
	userKey := mongodoc.Sanitize(user)
	field := "users." + userKey
	err := j.DB.Models().UpdateId(modelName.String(),
		bson.D{{
			"$set", bson.D{{
				field, info,
			}},
		}},
	)
	if err != nil {
		return errgo.Mask(err)
	}
	return nil
}

// DeleteController deletes existing controller and all of its
// associated models from the database. It returns an error if
// either deletion fails. If there is no matching controller then the
// error will have the cause params.ErrNotFound.
//
// Note that this operation is not atomic.
func (j *JEM) DeleteController(path params.EntityPath) error {
	// TODO (urosj) make this operation atomic.
	// Delete its models first.
	info, err := j.DB.Models().RemoveAll(bson.D{{"controller", path}})
	if err != nil {
		return errgo.Notef(err, "error deleting controller models")
	}
	// Then delete the controller.
	err = j.DB.Controllers().RemoveId(path.String())
	if err == mgo.ErrNotFound {
		return errgo.WithCausef(nil, params.ErrNotFound, "controller %q not found", path)
	}
	if err != nil {
		logger.Errorf("deleted %d controller models for model but could not delete controller: %v", info.Removed, err)
		return errgo.Notef(err, "cannot delete controller")
	}
	logger.Infof("deleted controller %v and %d associated models", path, info.Removed)
	return nil
}

// AddModel adds a new model to the database.
// It returns an error with a params.ErrAlreadyExists
// cause if there is already an model with the given name.
// If ignores m.Id and sets it from m.Path.
func (j *JEM) AddModel(m *mongodoc.Model) error {
	m.Id = m.Path.String()
	err := j.DB.Models().Insert(m)
	if mgo.IsDup(err) {
		return errgo.WithCausef(nil, params.ErrAlreadyExists, "")
	}
	if err != nil {
		return errgo.Notef(err, "cannot insert controller model")
	}
	return nil
}

// DeleteModel deletes an model from the database. If an
// model is also a controller it will not be deleted and an error
// with a cause of params.ErrForbidden will be returned. If the
// model cannot be found then an error with a cause of
// params.ErrNotFound is returned.
func (j *JEM) DeleteModel(path params.EntityPath) error {
	// TODO when we monitor model health, prohibit this method
	// and delete the model automatically when it is destroyed.
	// Check if model is also a controller.
	err := j.DB.Models().RemoveId(path.String())
	if err == mgo.ErrNotFound {
		return errgo.WithCausef(nil, params.ErrNotFound, "model %q not found", path)
	}
	if err != nil {
		return errgo.Notef(err, "could not delete model")
	}
	logger.Infof("deleted model %s", path)
	return nil
}

// Controller returns information on the controller with the given
// path. It returns an error with a params.ErrNotFound cause if the
// controller was not found.
func (j *JEM) Controller(path params.EntityPath) (*mongodoc.Controller, error) {
	var ctl mongodoc.Controller
	id := path.String()
	err := j.DB.Controllers().FindId(id).One(&ctl)
	if err == mgo.ErrNotFound {
		return nil, errgo.WithCausef(nil, params.ErrNotFound, "controller %q not found", id)
	}
	if err != nil {
		return nil, errgo.Notef(err, "cannot get controller %q", id)
	}
	return &ctl, nil
}

// Model returns information on the model with the given
// path. It returns an error with a params.ErrNotFound cause if the
// controller was not found.
func (j *JEM) Model(path params.EntityPath) (*mongodoc.Model, error) {
	id := path.String()
	var m mongodoc.Model
	err := j.DB.Models().FindId(id).One(&m)
	if err == mgo.ErrNotFound {
		return nil, errgo.WithCausef(nil, params.ErrNotFound, "model %q not found", id)
	}
	if err != nil {
		return nil, errgo.Notef(err, "cannot get model %q", id)
	}
	return &m, nil
}

// ModelFromUUID returns the document representing the model with the
// given UUID. It returns an error with a params.ErrNotFound cause if the
// controller was not found.
func (j *JEM) ModelFromUUID(uuid string) (*mongodoc.Model, error) {
	var m mongodoc.Model
	err := j.DB.Models().Find(bson.D{{"uuid", uuid}}).One(&m)
	if err == mgo.ErrNotFound {
		return nil, errgo.WithCausef(nil, params.ErrNotFound, "model %q not found", uuid)
	}
	if err != nil {
		return nil, errgo.Notef(err, "cannot get model %q", uuid)
	}
	return &m, nil
}

// ErrAPIConnection is returned by OpenAPI and OpenAPIFromDocs
// when the API connection cannot be made.
var ErrAPIConnection = errgo.New("cannot connect to API")

// OpenAPI opens an API connection to the model with the given path
// and returns it along with the information used to connect.
// If the model does not exist, the error will have a cause
// of params.ErrNotFound.
//
// If the model API connection could not be made, the error
// will have a cause of ErrAPIConnection.
//
// The returned connection must be closed when finished with.
func (j *JEM) OpenAPI(path params.EntityPath) (*apiconn.Conn, error) {
	ctl, err := j.Controller(path)
	if err != nil {
		return nil, errgo.NoteMask(err, "cannot get controller", errgo.Is(params.ErrNotFound))
	}
	return j.pool.connCache.OpenAPI(ctl.UUID, func() (api.Connection, *api.Info, error) {
		apiInfo := apiInfoFromDoc(ctl)
		logger.Debugf("%#v", apiInfo)
		st, err := api.Open(apiInfo, apiDialOpts())
		if err != nil {
			return nil, nil, errgo.WithCausef(err, ErrAPIConnection, "")
		}
		return st, apiInfo, nil
	})
}

// OpenAPIFromDoc returns an API connection to the controller held in the
// given document. This can be useful when we want to connect to a
// controller before it's added to the database. Note that a successful
// return from this function does not necessarily mean that the
// credentials or API addresses in the docs actually work, as it's
// possible that there's already a cached connection for the given
// controller.
//
// The returned connection must be closed when finished with.
func (j *JEM) OpenAPIFromDoc(ctl *mongodoc.Controller) (*apiconn.Conn, error) {
	return j.pool.connCache.OpenAPI(ctl.UUID, func() (api.Connection, *api.Info, error) {
		stInfo := apiInfoFromDoc(ctl)
		st, err := api.Open(stInfo, apiDialOpts())
		if err != nil {
			return nil, nil, errgo.WithCausef(err, ErrAPIConnection, "")
		}
		return st, stInfo, nil
	})
}

func apiDialOpts() api.DialOpts {
	return api.DialOpts{
		Timeout:    APIOpenTimeout,
		RetryDelay: 500 * time.Millisecond,
	}
}

func apiInfoFromDoc(ctl *mongodoc.Controller) *api.Info {
	return &api.Info{
		Addrs:    ctl.HostPorts,
		CACert:   ctl.CACert,
		Tag:      names.NewUserTag(ctl.AdminUser),
		Password: ctl.AdminPassword,
	}
}

// ControllerLocationQuery returns a mongo query that iterates through
// all the public controllers matching the given location attributes,
// including unavailable controllers only if includeUnavailable is true.
// It returns an error if the location attribute keys aren't valid.
func (j *JEM) ControllerLocationQuery(cloud params.Cloud, region string, includeUnavailable bool) (*mgo.Query, error) {
	q := make(bson.D, 0, 4)
	if cloud != "" {
		q = append(q, bson.DocElem{"cloud.name", cloud})
	}
	if region != "" {
		q = append(q, bson.DocElem{"cloud.regions", bson.D{{"$elemMatch", bson.D{{"name", region}}}}})
	}
	q = append(q, bson.DocElem{"public", true})
	if !includeUnavailable {
		q = append(q, bson.DocElem{"unavailablesince", notExistsQuery})
	}
	return j.DB.Controllers().Find(q), nil
}

// SetControllerAvailable marks the given controller as available.
// This method does not return an error when the controller doesn't exist.
func (j *JEM) SetControllerAvailable(ctlPath params.EntityPath) error {
	if err := j.DB.Controllers().UpdateId(ctlPath.String(), bson.D{{
		"$unset", bson.D{{"unavailablesince", nil}},
	}}); err != nil {
		if err == mgo.ErrNotFound {
			// For symmetry with SetControllerUnavailableAt.
			return nil
		}
		return errgo.Notef(err, "cannot update %v", ctlPath)
	}
	return nil
}

// SetControllerUnavailableAt marks the controller as having been unavailable
// since at least the given time. If the controller was already marked
// as unavailable, its time isn't changed.
// This method does not return an error when the controller doesn't exist.
func (j *JEM) SetControllerUnavailableAt(ctlPath params.EntityPath, t time.Time) error {
	err := j.DB.Controllers().Update(
		bson.D{
			{"_id", ctlPath.String()},
			{"unavailablesince", notExistsQuery},
		},
		bson.D{
			{"$set", bson.D{{"unavailablesince", t}}},
		},
	)
	if err == nil {
		return nil
	}
	if err == mgo.ErrNotFound {
		// We don't know whether the not-found error is because there
		// are no controllers with the given name (in which case we want
		// to return a params.ErrNotFound error) or because there was
		// one but it is already unavailable.
		// We could fetch the controller to decide whether it's actually there
		// or not, but because in practice we don't care if we're setting
		// controller-unavailable on a non-existent controller, we'll
		// save the round trip.
		return nil
	}
	return errgo.Notef(err, "cannot update controller")
}

// ErrLeaseUnavailable is the error cause returned by AcquireMonitorLease
// when it cannot acquire the lease because it is unavailable.
var ErrLeaseUnavailable = errgo.Newf("cannot acquire lease")

// AcquireMonitorLease acquires or renews the lease on a controller.
// The lease will only be changed if the lease in the database
// has the given old expiry time and owner.
// When acquired, the lease will have the given new owner
// and expiration time.
//
// If newOwner is empty, the lease will be dropped, the
// returned time will be zero and newExpiry will be ignored.
//
// If the controller has been removed, an error with a params.ErrNotFound
// cause will be returned. If the lease has been obtained by someone else
// an error with a ErrLeaseUnavailable cause will be returned.
func (j *JEM) AcquireMonitorLease(ctlPath params.EntityPath, oldExpiry time.Time, oldOwner string, newExpiry time.Time, newOwner string) (time.Time, error) {
	var update bson.D
	if newOwner != "" {
		newExpiry = mongodoc.Time(newExpiry)
		update = bson.D{{"$set", bson.D{
			{"monitorleaseexpiry", newExpiry},
			{"monitorleaseowner", newOwner},
		}}}
	} else {
		newExpiry = time.Time{}
		update = bson.D{{"$unset", bson.D{
			{"monitorleaseexpiry", nil},
			{"monitorleaseowner", nil},
		}}}
	}
	var oldOwnerQuery interface{}
	var oldExpiryQuery interface{}
	if oldOwner == "" {
		oldOwnerQuery = notExistsQuery
	} else {
		oldOwnerQuery = oldOwner
	}
	if oldExpiry.IsZero() {
		oldExpiryQuery = notExistsQuery
	} else {
		oldExpiryQuery = oldExpiry
	}
	err := j.DB.Controllers().Update(bson.D{
		{"path", ctlPath},
		{"monitorleaseexpiry", oldExpiryQuery},
		{"monitorleaseowner", oldOwnerQuery},
	}, update)
	if err == mgo.ErrNotFound {
		// Someone else got there first, or the document has been
		// removed. Technically don't need to distinguish between the
		// two cases, but it's useful to see the different error messages.
		ctl, err := j.Controller(ctlPath)
		if errgo.Cause(err) == params.ErrNotFound {
			return time.Time{}, errgo.WithCausef(nil, params.ErrNotFound, "controller removed")
		}
		if err != nil {
			return time.Time{}, errgo.Mask(err)
		}
		return time.Time{}, errgo.WithCausef(nil, ErrLeaseUnavailable, "controller has lease taken out by %q expiring at %v", ctl.MonitorLeaseOwner, ctl.MonitorLeaseExpiry.UTC())
	}
	if err != nil {
		return time.Time{}, errgo.Notef(err, "cannot acquire lease")
	}
	return newExpiry, nil
}

// SetControllerStats sets the stats associated with the controller
// with the given path. It returns an error with a params.ErrNotFound
// cause if the controller does not exist.
func (j *JEM) SetControllerStats(ctlPath params.EntityPath, stats *mongodoc.ControllerStats) error {
	err := j.DB.Controllers().UpdateId(
		ctlPath.String(),
		bson.D{{"$set", bson.D{{"stats", stats}}}},
	)
	if err == mgo.ErrNotFound {
		return errgo.WithCausef(nil, params.ErrNotFound, "controller not found")
	}
	return errgo.Mask(err)
}

// SetModelLife sets the Life field of all models controlled
// by the given controller that have the given UUID.
// It does not return an error if there are no such models.
func (j *JEM) SetModelLife(ctlPath params.EntityPath, uuid string, life string) error {
	_, err := j.DB.Models().UpdateAll(
		bson.D{{"uuid", uuid}, {"controller", ctlPath}},
		bson.D{{"$set", bson.D{{"life", life}}}},
	)
	if err != nil {
		return errgo.Notef(err, "cannot update model")
	}
	return nil
}

// updateCredential stores the given credential in the database. If a
// credential with the same name exists it is overwritten.
func (j *JEM) updateCredential(cred *mongodoc.Credential) error {
	update := bson.D{{
		"type", cred.Type,
	}, {
		"label", cred.Label,
	}, {
		"attributes", cred.Attributes,
	}}
	if len(cred.ACL.Read) > 0 {
		update = append(update, bson.DocElem{"acl", cred.ACL})
	}
	id := credentialId(cred.User, cred.Cloud, cred.Name)
	_, err := j.DB.Credentials().UpsertId(id, bson.D{{
		"$set", update,
	}, {
		"$setOnInsert", bson.D{{
			"user", cred.User,
		}, {
			"cloud", cred.Cloud,
		}, {
			"name", cred.Name,
		}},
	}})
	if err != nil {
		return errgo.Mask(err)
	}
	return nil
}

// Credential gets the specified credential. If the credential cannot be
// found the returned error will have a cause of params.ErrNotFound.
func (j *JEM) Credential(user params.User, cloud params.Cloud, name params.Name) (*mongodoc.Credential, error) {
	var cred mongodoc.Credential
	id := credentialId(user, cloud, name)
	err := j.DB.Credentials().FindId(id).One(&cred)
	if err == mgo.ErrNotFound {
		return nil, errgo.WithCausef(nil, params.ErrNotFound, "credential %q not found", id)
	}
	if err != nil {
		return nil, errgo.Notef(err, "cannot get credential %q", id)
	}
	return &cred, nil
}

// credentialId calculates the id for a credential with the specified
// user, cloud and name.
func credentialId(user params.User, cloud params.Cloud, name params.Name) string {
	return fmt.Sprintf("%s/%s/%s", user, cloud, name)
}

// credentialAddController stores the fact that the credential with the
// given user, cloud and name is present on the given controller.
func (j *JEM) credentialAddController(user params.User, cloud params.Cloud, name params.Name, controller params.EntityPath) error {
	id := credentialId(user, cloud, name)
	err := j.DB.Credentials().UpdateId(id, bson.D{{
		"$addToSet", bson.D{{"controllers", controller}},
	}})
	if err != nil {
		if err == mgo.ErrNotFound {
			return errgo.WithCausef(nil, params.ErrNotFound, "credential %q not found", id)
		}
		return errgo.Notef(err, "cannot update credential %q", id)
	}
	return nil
}

// credentialRemoveController stores the fact that the credential with
// the given user, cloud and name is not present on the given controller.
func (j *JEM) credentialRemoveController(user params.User, cloud params.Cloud, name params.Name, controller params.EntityPath) error {
	id := credentialId(user, cloud, name)
	err := j.DB.Credentials().UpdateId(id, bson.D{{
		"$pull", bson.D{{"controllers", controller}},
	}})
	if err != nil {
		if err == mgo.ErrNotFound {
			return errgo.WithCausef(nil, params.ErrNotFound, "credential %q not found", id)
		}
		return errgo.Notef(err, "cannot update credential %q", id)
	}
	return nil
}

// Cloud gets the details of the given cloud.
//
// Note that there may be many controllers with the given cloud name. We
// return an arbitrary choice, assuming that cloud definitions are the
// same across all possible controllers.
func (j *JEM) Cloud(cloud params.Cloud) (*mongodoc.Cloud, error) {
	var ctl mongodoc.Controller
	err := j.DB.Controllers().Find(bson.D{{"cloud.name", cloud}}).One(&ctl)
	if err == mgo.ErrNotFound {
		return nil, errgo.WithCausef(nil, params.ErrNotFound, "cloud %q not found", cloud)
	}
	if err != nil {
		return nil, errgo.Notef(err, "cannot get cloud %q", cloud)
	}
	return &ctl.Cloud, nil
}

// SetACL sets the ACL for the path document in c to be equal to acl.
func (j *JEM) SetACL(c *mgo.Collection, path params.EntityPath, acl params.ACL) error {
	err := c.UpdateId(path.String(), bson.D{{"$set", bson.D{{"acl", acl}}}})
	if err == nil {
		return nil
	}
	if err == mgo.ErrNotFound {
		return errgo.WithCausef(nil, params.ErrNotFound, "%q not found", path)
	}
	return errgo.Notef(err, "cannot update ACL on %q", path)
}

// Grant updates the ACL for the path document in c to include user.
func (j *JEM) Grant(c *mgo.Collection, path params.EntityPath, user params.User) error {
	err := c.UpdateId(path.String(), bson.D{{"$addToSet", bson.D{{"acl.read", user}}}})
	if err == nil {
		return nil
	}
	if err == mgo.ErrNotFound {
		return errgo.WithCausef(nil, params.ErrNotFound, "%q not found", path)
	}
	return errgo.Notef(err, "cannot update ACL on %q", path)
}

// Revoke updates the ACL for the path document in c to not include user.
func (j *JEM) Revoke(c *mgo.Collection, path params.EntityPath, user params.User) error {
	err := c.UpdateId(path.String(), bson.D{{"$pull", bson.D{{"acl.read", user}}}})
	if err == nil {
		return nil
	}
	if err == mgo.ErrNotFound {
		return errgo.WithCausef(nil, params.ErrNotFound, "%q not found", path)
	}
	return errgo.Notef(err, "cannot update ACL on %q", path)
}

type NewModelParams struct {
	Path            params.EntityPath
	ControllerPath  params.EntityPath
	CredentialsPath params.EntityPath
	Cloud           string
	CloudRegion     string
}

// NewModel creates a new model based on params.
func (j *JEM) NewModel(params NewModelParams) (*mongodoc.Model, error) {
	return nil, nil
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

// Clone copies the Database and clones its underlying
// mgo session. See mgo.Session.Clone and mgo.Session.Copy
// for information on the distinction between Clone and Copy.
func (s Database) Clone() Database {
	if s.Session == nil {
		panic("nil session in clone!")
	}
	return Database{
		&mgo.Database{
			Name:    s.Name,
			Session: s.Session.Clone(),
		},
	}
}

func (db Database) Collections() []*mgo.Collection {
	return []*mgo.Collection{
		db.Macaroons(),
		db.Controllers(),
		db.Models(),
		db.Credentials(),
	}
}

// Close closes the database's underlying session.
func (db Database) Close() {
	db.Session.Close()
}

func (db Database) Macaroons() *mgo.Collection {
	return db.C("macaroons")
}

func (db Database) Controllers() *mgo.Collection {
	return db.C("controllers")
}

func (db Database) Models() *mgo.Collection {
	return db.C("models")
}

func (db Database) Credentials() *mgo.Collection {
	return db.C("credentials")
}

func (db Database) C(name string) *mgo.Collection {
	if db.Database == nil {
		panic(fmt.Sprintf("cannot get collection %q because JEM closed", name))
	}
	return db.Database.C(name)
}

func validateLocationAttrs(attrs map[string]string) error {
	for attr := range attrs {
		if !params.IsValidLocationAttr(attr) {
			return errgo.Newf("invalid attribute %q", attr)
		}
	}
	return nil
}

// ModelName creates a valid model name for the model specified by path.
func ModelName(path params.EntityPath) string {
	return fmt.Sprintf("%s--%s", path.User, path.Name)
}
