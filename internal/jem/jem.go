// Copyright 2015 Canonical Ltd.

package jem

import (
	"fmt"
	"sync"
	"time"

	"github.com/juju/idmclient"
	"github.com/juju/juju/api"
	cloudapi "github.com/juju/juju/api/cloud"
	modelmanagerapi "github.com/juju/juju/api/modelmanager"
	jujuparams "github.com/juju/juju/apiserver/params"
	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/network"
	"github.com/juju/loggo"
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

// AddController adds a new controller and its associated model
// to the database. It returns an error with a params.ErrAlreadyExists
// cause if there is already a controller with the given name.
// The Id field in ctl will be set from its Path field,
// and the Id, Path and Controller fields in env will also be
// set from ctl.
// Any empty Location attributes will be removed from ctl.Location.
//
// If the provided documents aren't valid, AddController with return
// an error with a params.ErrBadRequest cause.
func (j *JEM) AddController(ctl *mongodoc.Controller) error {
	if err := j.CheckIsUser(ctl.Path.User); err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrUnauthorized))
	}
	if ctl.Public {
		if err := j.CheckIsAdmin(); err != nil {
			if errgo.Cause(err) == params.ErrUnauthorized {
				return errgo.WithCausef(nil, params.ErrUnauthorized, "admin access required to add public controllers")
			}
			return errgo.Mask(err)
		}
	}
	if err := validateController(ctl); err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrBadRequest))
	}
	// connect to the controller.
	conn, err := j.OpenAPI(ctl)
	if err != nil {
		logger.Infof("cannot open API: %v", err)
		return errgo.WithCausef(err, params.ErrBadRequest, "cannot connect to controller")
	}
	defer conn.Close()

	if err := updateControllerInfo(conn, ctl); err != nil {
		return errgo.Mask(err)
	}

	return errgo.Mask(j.DB.AddController(ctl), errgo.Is(params.ErrBadRequest), errgo.Is(params.ErrAlreadyExists))
}

func validateController(ctl *mongodoc.Controller) error {
	if len(ctl.HostPorts) == 0 {
		return errgo.WithCausef(nil, params.ErrBadRequest, "no host-ports in request")
	}
	if ctl.CACert == "" {
		return errgo.WithCausef(nil, params.ErrBadRequest, "no ca-cert in request")
	}
	if ctl.AdminUser == "" {
		return errgo.WithCausef(nil, params.ErrBadRequest, "no user in request")
	}
	if ctl.UUID == "" {
		return errgo.WithCausef(nil, params.ErrBadRequest, "bad model UUID in request")
	}
	return nil
}

func updateControllerInfo(conn *apiconn.Conn, ctl *mongodoc.Controller) error {
	// Find out the cloud information.
	client := cloudapi.NewClient(conn)
	clouds, err := client.Clouds()
	if err != nil {
		return errgo.Notef(err, "cannot get cloud information")
	}
	for tag, cloudInfo := range clouds {
		// Take the cloud info from the first cloud reported. There should only be one.
		ctl.Cloud.Name = params.Cloud(tag.Id())
		ctl.Cloud.ProviderType = cloudInfo.Type
		for _, at := range cloudInfo.AuthTypes {
			ctl.Cloud.AuthTypes = append(ctl.Cloud.AuthTypes, string(at))
		}
		ctl.Cloud.Endpoint = cloudInfo.Endpoint
		ctl.Cloud.IdentityEndpoint = cloudInfo.IdentityEndpoint
		ctl.Cloud.StorageEndpoint = cloudInfo.StorageEndpoint
		for _, reg := range cloudInfo.Regions {
			ctl.Cloud.Regions = append(ctl.Cloud.Regions, mongodoc.Region{
				Name:             reg.Name,
				Endpoint:         reg.Endpoint,
				IdentityEndpoint: reg.IdentityEndpoint,
				StorageEndpoint:  reg.StorageEndpoint,
			})
		}
		break
	}

	// Update addresses from latest known in controller.
	// Note that state.APIHostPorts is always guaranteed
	// to include the actual address we succeeded in
	// connecting to.
	ctl.HostPorts = collapseHostPorts(conn.APIHostPorts())

	return nil
}

// collapseHostPorts collapses a list of host-port lists
// into a single list suitable for passing to api.Open.
// It preserves ordering because api.State.APIHostPorts
// makes sure to return the first-connected address
// first in the slice.
// See juju.PrepareEndpointsForCaching for a more
// comprehensive version of this function.
func collapseHostPorts(hpss [][]network.HostPort) []string {
	hps := network.CollapseHostPorts(hpss)
	hps = network.FilterUnusableHostPorts(hps)
	hps = network.DropDuplicatedHostPorts(hps)
	return network.HostPortsToStrings(hps)
}

// DeleteController deletes existing controller and all of its
// associated models from the database. It returns an error if
// either deletion fails. If there is no matching controller then the
// error will have the cause params.ErrNotFound.
//
// Note that this operation is not atomic.
func (j *JEM) DeleteController(path params.EntityPath) error {
	if err := j.CheckIsUser(path.User); err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrUnauthorized))
	}
	return errgo.Mask(j.DB.DeleteController(path), errgo.Is(params.ErrNotFound))
}

// Controller returns information on the controller with the given
// path. It returns an error with a params.ErrNotFound cause if the
// controller was not found.
func (j *JEM) Controller(path params.EntityPath) (*mongodoc.Controller, error) {
	ctl, err := j.DB.Controller(path)
	if err != nil {
		if errgo.Cause(err) == params.ErrNotFound {
			if uerr := j.CheckIsUser(path.User); uerr != nil {
				err = uerr
			}
		}
		return nil, errgo.Mask(err, errgo.Is(params.ErrNotFound), errgo.Is(params.ErrUnauthorized))
	}
	if ctl.Public {
		return ctl, nil
	}
	if err := j.CheckCanRead(ctl); err != nil {
		return nil, errgo.Mask(err, errgo.Is(params.ErrUnauthorized))
	}
	return ctl, nil
}

// Model returns information on the model with the given
// path. It returns an error with a params.ErrNotFound cause if the
// controller was not found.
func (j *JEM) Model(path params.EntityPath) (*mongodoc.Model, error) {
	m, err := j.DB.Model(path)
	if err != nil {
		if errgo.Cause(err) == params.ErrNotFound {
			if uerr := j.CheckIsUser(path.User); uerr != nil {
				err = uerr
			}
		}
		return nil, errgo.Mask(err, errgo.Is(params.ErrNotFound), errgo.Is(params.ErrUnauthorized))
	}
	if err := j.CheckCanRead(m); err != nil {
		return nil, errgo.Mask(err, errgo.Is(params.ErrUnauthorized))
	}
	return m, nil
}

// ModelFromUUID returns the document representing the model with the
// given UUID. It returns an error with a params.ErrNotFound cause if the
// controller was not found.
func (j *JEM) ModelFromUUID(uuid string) (*mongodoc.Model, error) {
	m, err := j.DB.ModelFromUUID(uuid)
	if err != nil {
		return nil, errgo.Mask(err, errgo.Is(params.ErrNotFound))
	}
	if err := j.CheckCanRead(m); err != nil {
		return nil, errgo.Mask(err, errgo.Is(params.ErrUnauthorized))
	}
	return m, nil
}

// ErrAPIConnection is returned by OpenAPI and OpenAPIFromDocs
// when the API connection cannot be made.
var ErrAPIConnection = errgo.New("cannot connect to API")

// OpenAPI opens an API connection to the given Controller.
//
// If the model API connection could not be made, the error
// will have a cause of ErrAPIConnection.
//
// The returned connection must be closed when finished with.
func (j *JEM) OpenAPI(ctl *mongodoc.Controller) (*apiconn.Conn, error) {
	return j.pool.connCache.OpenAPI(ctl.UUID, func() (api.Connection, *api.Info, error) {
		apiInfo := apiInfo(ctl)
		logger.Debugf("%#v", apiInfo)
		st, err := api.Open(apiInfo, apiDialOpts())
		if err != nil {
			return nil, nil, errgo.WithCausef(err, ErrAPIConnection, "")
		}
		return st, apiInfo, nil
	})
}

func apiDialOpts() api.DialOpts {
	return api.DialOpts{
		Timeout:    APIOpenTimeout,
		RetryDelay: 500 * time.Millisecond,
	}
}

func apiInfo(ctl *mongodoc.Controller) *api.Info {
	return &api.Info{
		Addrs:    ctl.HostPorts,
		CACert:   ctl.CACert,
		Tag:      names.NewUserTag(ctl.AdminUser),
		Password: ctl.AdminPassword,
	}
}

func (j *JEM) openAPIPath(path params.EntityPath) (*apiconn.Conn, error) {
	ctl, err := j.Controller(path)
	if err != nil {
		return nil, errgo.Mask(err, errgo.Is(params.ErrNotFound), errgo.Is(params.ErrUnauthorized))
	}
	return j.OpenAPI(ctl)
}

// Credential gets the specified credential. If the credential cannot be
// found the returned error will have a cause of params.ErrNotFound.
func (j *JEM) Credential(user params.User, cloud params.Cloud, name params.Name) (*mongodoc.Credential, error) {
	cred, err := j.DB.Credential(user, cloud, name)
	if err != nil {
		return nil, errgo.Mask(err, errgo.Is(params.ErrNotFound))
	}
	if err := j.CheckCanRead(cred); err != nil {
		return nil, errgo.Mask(err, errgo.Is(params.ErrUnauthorized))
	}
	return cred, nil
}

// DoControllers calls the given function for each controller that
// can be read by the current user that matches the given attributes.
// If the function returns an error, the iteration stops and
// DoControllers returns the error with the same cause.
func (j *JEM) DoControllers(cloud params.Cloud, region string, do func(c *mongodoc.Controller) error) error {
	// Query all the controllers that match the attributes, building
	// up all the possible values.
	q, err := j.DB.controllerLocationQuery(cloud, region, false)
	if err != nil {
		return errgo.WithCausef(err, params.ErrBadRequest, "%s", "")
	}
	// Sort by _id so that we can make easily reproducible tests.
	iter := j.CanReadIter(q.Sort("_id").Iter())
	var ctl mongodoc.Controller
	for iter.Next(&ctl) {
		if err := do(&ctl); err != nil {
			iter.Close()
			return errgo.Mask(err, errgo.Any)
		}
	}
	if err := iter.Err(); err != nil {
		return errgo.Notef(err, "cannot query")
	}
	return nil
}

// selectController chooses a controller that matches the cloud and region criteria, if specified.
func (j *JEM) selectController(cloud params.Cloud, region string) (*mongodoc.Controller, error) {
	var controllers []mongodoc.Controller
	err := j.DoControllers(cloud, region, func(c *mongodoc.Controller) error {
		controllers = append(controllers, *c)
		return nil
	})
	if err != nil {
		return nil, errgo.Mask(err, errgo.Is(params.ErrBadRequest))
	}
	if len(controllers) == 0 {
		return nil, errgo.WithCausef(nil, params.ErrNotFound, "no matching controllers found")
	}
	// Choose a random controller.
	// TODO select a controller more intelligently, for example
	// by choosing the most lightly loaded controller
	n := randIntn(len(controllers))
	return &controllers[n], nil
}

// CreateModelParams specifies the parameters needed to create a new
// model using CreateModel.
type CreateModelParams struct {
	// Path contains the path of the new model.
	Path params.EntityPath

	// ControllerPath contains the path of the owning
	// controller.
	ControllerPath params.EntityPath

	// Credential contains the name of the credential to use to
	// create the model.
	Credential params.Name

	// Cloud contains the name of the cloud in which the
	// model will be created.
	Cloud params.Cloud

	// Region contains the name of the region in which the model will
	// be created. This may be empty if the cloud does not support
	// regions.
	Region string

	// Attributes contains the attributes to assign to the new model.
	Attributes map[string]interface{}
}

// CreateModel creates a new model as specified by p using conn.
func (j *JEM) CreateModel(p CreateModelParams) (*mongodoc.Model, *jujuparams.ModelInfo, error) {
	if err := j.CheckIsUser(p.Path.User); err != nil {
		return nil, nil, errgo.Mask(err, errgo.Is(params.ErrUnauthorized))
	}
	var ctl *mongodoc.Controller
	if p.ControllerPath.Name == "" {
		var err error
		ctl, err = j.selectController(p.Cloud, p.Region)
		if err != nil {
			return nil, nil, errgo.Mask(err, errgo.Is(params.ErrNotFound), errgo.Is(params.ErrBadRequest))
		}
		p.ControllerPath = ctl.Path
	} else {
		var err error
		ctl, err = j.Controller(p.ControllerPath)
		if err != nil {
			return nil, nil, errgo.Mask(err,
				errgo.Is(params.ErrNotFound),
				errgo.Is(params.ErrBadRequest),
				errgo.Is(params.ErrUnauthorized),
			)
		}
	}
	if p.Cloud == "" {
		p.Cloud = ctl.Cloud.Name
	}
	cred, err := j.Credential(p.Path.User, p.Cloud, p.Credential)
	if err != nil {
		return nil, nil, errgo.Mask(err, errgo.Is(params.ErrNotFound))
	}
	conn, err := j.OpenAPI(ctl)
	if err != nil {
		return nil, nil, errgo.NoteMask(err, "cannot connect to controller", errgo.Is(params.ErrUnauthorized), errgo.Is(params.ErrNotFound))
	}
	defer conn.Close()
	if err := j.updateControllerCredential(conn, cred); err != nil {
		return nil, nil, errgo.Mask(err)
	}
	if err := j.DB.credentialAddController(p.Path.User, p.Cloud, p.Credential, p.ControllerPath); err != nil {
		return nil, nil, errgo.Mask(err)
	}
	// Create the model record in the database before actually
	// creating the model on the controller. It will have an invalid
	// UUID because it doesn't exist but that's better than creating
	// an model that we can't add locally because the name
	// already exists.
	modelDoc := &mongodoc.Model{
		Path:       p.Path,
		Controller: p.ControllerPath,
	}
	if err := j.DB.AddModel(modelDoc); err != nil {
		return nil, nil, errgo.Mask(err, errgo.Is(params.ErrAlreadyExists))
	}
	mmClient := modelmanagerapi.NewClient(conn.Connection)
	m, err := mmClient.CreateModel(
		string(p.Path.Name),
		UserTag(p.Path.User).Id(),
		string(p.Cloud),
		p.Region,
		CloudCredentialTag(p.Cloud, p.Path.User, p.Credential),
		p.Attributes,
	)
	if err != nil {
		// Remove the model that was created, because it's no longer valid.
		if err := j.DB.Models().RemoveId(modelDoc.Id); err != nil {
			logger.Errorf("cannot remove model from database after model creation error: %v", err)
		}
		return nil, nil, errgo.Notef(err, "cannot create model")
	}
	if err := mmClient.GrantModel(conn.Info.Tag.(names.UserTag).Id(), "admin", m.UUID); err != nil {
		// TODO (mhilton) destroy the model?
		return nil, nil, errgo.Notef(err, "cannot grant admin access")
	}
	// Now set the UUID to that of the actually created model.
	if err := j.DB.Models().UpdateId(modelDoc.Id, bson.D{{"$set", bson.D{{"uuid", m.UUID}}}}); err != nil {
		// TODO (mhilton) destroy the model?
		return nil, nil, errgo.Notef(err, "cannot update model UUID in database, leaked model %s", m.UUID)
	}
	modelDoc.UUID = m.UUID
	return modelDoc, &m, nil
}

// DestroyModel destroys the specified model and removes it from the
// database.
//
// Note that if the model is destroyed in its controller but
// j.DeleteModel fails, a subsequent DestroyModel can can still succeed
// because client.DestroyModel will succeed when the model doesn't exist.
func (j *JEM) DestroyModel(path params.EntityPath) error {
	if err := j.CheckIsUser(path.User); err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrUnauthorized))
	}
	model, err := j.Model(path)
	if err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrNotFound))
	}
	return errgo.Mask(j.destroyModel(model))
}

// DestroyModelFromUUID destroys the specified model and removes it from
// the database.
//
// Note that if the model is destroyed in its controller but
// j.DeleteModel fails, a subsequent DestroyModel can can still succeed
// because client.DestroyModel will succeed when the model doesn't exist.
func (j *JEM) DestroyModelFromUUID(uuid string) error {
	model, err := j.DB.ModelFromUUID(uuid)
	if err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrNotFound), errgo.Is(params.ErrUnauthorized))
	}
	if err := j.CheckIsUser(model.Path.User); err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrUnauthorized))
	}
	return errgo.Mask(j.destroyModel(model))
}

func (j *JEM) destroyModel(model *mongodoc.Model) error {
	conn, err := j.openAPIPath(model.Controller)
	if err != nil {
		return errgo.Mask(err)
	}
	defer conn.Close()
	client := modelmanagerapi.NewClient(conn)
	if err := client.DestroyModel(names.NewModelTag(model.UUID)); err != nil {
		return errgo.Mask(err)
	}
	if err := j.DB.DeleteModel(model.Path); err != nil {
		return errgo.Mask(err)
	}
	return nil
}

// UpdateCredential updates the specified credential in the
// local database and then updates it on all controllers to which it is
// deployed.
func (j *JEM) UpdateCredential(cred *mongodoc.Credential) error {
	if err := j.CheckIsUser(cred.User); err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrUnauthorized))
	}
	if err := j.DB.updateCredential(cred); err != nil {
		return errgo.Notef(err, "cannot update local database")
	}
	c, err := j.Credential(cred.User, cred.Cloud, cred.Name)
	if err != nil {
		return errgo.Mask(err)
	}
	// TODO(mhilton) consider how to handle and recover from errors
	// updating credentials in controllers better.
	var firstError error
	for _, ctlPath := range c.Controllers {
		conn, err := j.openAPIPath(ctlPath)
		if err != nil {
			logger.Errorf("cannot open controller connection to %s: %s", ctlPath, err)
			if firstError != nil {
				firstError = err
			}
			continue
		}
		if err := j.updateControllerCredential(conn, cred); err != nil {
			logger.Errorf("cannot update credential %s on %s: %s", cred.Id, ctlPath, err)
			if firstError != nil {
				firstError = err
			}
		}
		conn.Close()
	}
	return errgo.Mask(firstError)
}

// updateControllerCredential uploads the given credential to conn.
func (j *JEM) updateControllerCredential(conn *apiconn.Conn, cred *mongodoc.Credential) error {
	cloudCredentialTag := CloudCredentialTag(cred.Cloud, cred.User, cred.Name)
	cloudClient := cloudapi.NewClient(conn)
	err := cloudClient.UpdateCredential(
		cloudCredentialTag,
		jujucloud.NewCredential(jujucloud.AuthType(cred.Type), cred.Attributes),
	)
	if err != nil {
		return errgo.Notef(err, "cannot upload credentials")
	}
	return nil
}

// GrantModel grants the given access for the given user on the given model and updates the JEM database.
func (j *JEM) GrantModel(path params.EntityPath, user params.User, access string) error {
	if err := j.CheckIsUser(path.User); err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrUnauthorized))
	}
	model, err := j.Model(path)
	if err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrNotFound))
	}
	return errgo.Mask(j.grantModel(model, user, access))
}

// GrantModelFromUUID grants the given access for the given user on the given model and updates the JEM database.
func (j *JEM) GrantModelFromUUID(uuid string, user params.User, access string) error {
	model, err := j.ModelFromUUID(uuid)
	if err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrNotFound))
	}
	if err := j.CheckIsUser(model.Path.User); err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrUnauthorized))
	}
	return errgo.Mask(j.grantModel(model, user, access))
}

func (j *JEM) grantModel(model *mongodoc.Model, user params.User, access string) error {
	conn, err := j.openAPIPath(model.Controller)
	if err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrNotFound))
	}
	defer conn.Close()
	client := modelmanagerapi.NewClient(conn)
	if err := client.GrantModel(UserTag(user).Id(), access, model.UUID); err != nil {
		return errgo.Mask(err)
	}
	if err := Grant(j.DB.Models(), model.Path, user); err != nil {
		// TODO (mhilton) What should be done with the changes already made to the controller.
		return errgo.Mask(err)
	}
	return nil
}

// RevokeModel revokes the given access for the given user on the given model and updates the JEM database.
func (j *JEM) RevokeModel(path params.EntityPath, user params.User, access string) error {
	if err := j.CheckIsUser(path.User); err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrUnauthorized))
	}
	model, err := j.Model(path)
	if err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrNotFound))
	}
	return errgo.Mask(j.revokeModel(model, user, access))
}

// RevokeModelFromUUID revokes the given access for the given user on the given model and updates the JEM database.
func (j *JEM) RevokeModelFromUUID(uuid string, user params.User, access string) error {
	model, err := j.ModelFromUUID(uuid)
	if err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrNotFound))
	}
	if err := j.CheckIsUser(model.Path.User); err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrUnauthorized))
	}
	return errgo.Mask(j.revokeModel(model, user, access))
}

// RevokeModel revokes the given access for the given user on the given model and updates the JEM database.
func (j *JEM) revokeModel(model *mongodoc.Model, user params.User, access string) error {
	conn, err := j.openAPIPath(model.Controller)
	if err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrNotFound))
	}
	defer conn.Close()
	if err := Revoke(j.DB.Models(), model.Path, user); err != nil {
		return errgo.Mask(err)
	}
	client := modelmanagerapi.NewClient(conn)
	if err := client.RevokeModel(UserTag(user).Id(), access, model.UUID); err != nil {
		// TODO (mhilton) What should be done with the changes already made to JEM.
		return errgo.Mask(err)
	}
	return nil
}

// SetModelACL sets the ACL for the given model to the given value.
func (j *JEM) SetModelACL(path params.EntityPath, acl params.ACL) error {
	if err := j.CheckIsUser(path.User); err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrUnauthorized))
	}
	model, err := j.Model(path)
	if err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrNotFound))
	}
	grant := make(map[string]bool, len(acl.Read))
	for _, n := range acl.Read {
		grant[n] = true
	}
	revoke := make([]string, 0, len(model.ACL.Read))
	for _, n := range model.ACL.Read {
		if !grant[n] && n != string(model.Path.User) {
			revoke = append(revoke, n)
			continue
		}
		grant[n] = false
	}
	for _, n := range revoke {
		if err := j.revokeModel(model, params.User(n), "write"); err != nil {
			return errgo.Mask(err)
		}
	}
	for n, g := range grant {
		if !g {
			continue
		}
		if err := j.grantModel(model, params.User(n), "write"); err != nil {
			return errgo.Mask(err)
		}
	}
	return nil
}

// UserTag creates a juju user tag from a params.User
func UserTag(u params.User) names.UserTag {
	return names.NewUserTag(string(u) + "@external")
}

// CloudTag creates a juju cloud tag from a params.Cloud
func CloudTag(c params.Cloud) names.CloudTag {
	return names.NewCloudTag(string(c))
}

// CloudCredentialTag creates a juju cloud credential tag from the given
// cloud, user and name.
func CloudCredentialTag(cloud params.Cloud, user params.User, name params.Name) names.CloudCredentialTag {
	return names.NewCloudCredentialTag(fmt.Sprintf("%s/%s@external/%s", cloud, user, name))
}
