// Copyright 2015 Canonical Ltd.

package jem

import (
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	cloudapi "github.com/juju/juju/api/cloud"
	"github.com/juju/juju/api/modelmanager"
	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/loggo"
	"github.com/juju/utils/clock"
	"golang.org/x/net/context"
	"gopkg.in/errgo.v1"
	"gopkg.in/juju/names.v2"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"

	"github.com/CanonicalLtd/jem/internal/apiconn"
	"github.com/CanonicalLtd/jem/internal/mgosession"
	"github.com/CanonicalLtd/jem/internal/mongodoc"
	"github.com/CanonicalLtd/jem/params"
)

var logger = loggo.GetLogger("jem.internal.jem")

// wallClock provides access to the current time. It is a variable so
// that it can be overridden in tests.
var wallClock clock.Clock = clock.WallClock

// Functions defined as variables so they can be overridden in tests.
var (
	randIntn = rand.Intn
)

// Params holds parameters for the NewPool function.
type Params struct {
	// DB holds the mongo database that will be used to
	// store the JEM information.
	DB *mgo.Database

	// SessionPool holds a pool from which session objects are
	// taken to be used in database operations.
	SessionPool *mgosession.Pool

	// ControllerAdmin holds the identity of the user
	// or group that is allowed to create controllers.
	ControllerAdmin params.User
}

type Pool struct {
	config    Params
	connCache *apiconn.Cache

	// dbName holds the name of the database to use.
	dbName string

	// mu guards the fields below it.
	mu sync.Mutex

	// closed holds whether the Pool has been closed.
	closed bool

	// refCount holds the number of JEM instances that
	// currently refer to the pool. The pool is finally
	// closed when all JEM instances are closed and the
	// pool itself has been closed.
	refCount int
}

var APIOpenTimeout = 15 * time.Second

var notExistsQuery = bson.D{{"$exists", false}}

// NewPool represents a pool of possible JEM instances that use the given
// database as a store, and use the given bakery parameters to create the
// bakery.Service.
func NewPool(p Params) (*Pool, error) {
	// TODO migrate database
	if p.ControllerAdmin == "" {
		return nil, errgo.Newf("no controller admin group specified")
	}
	if p.SessionPool == nil {
		return nil, errgo.Newf("no session pool provided")
	}
	pool := &Pool{
		config:    p,
		dbName:    p.DB.Name,
		connCache: apiconn.NewCache(apiconn.CacheParams{}),
		refCount:  1,
	}
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
	p.refCount++
	return &JEM{
		DB:   newDatabase(p.config.SessionPool.Session(), p.dbName),
		pool: p,
	}
}

type JEM struct {
	// DB holds the mongodb-backed identity store.
	DB *Database

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

	j.pool.refCount++
	return &JEM{
		DB:   j.DB.clone(),
		pool: j.pool,
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
	j.closed = true
	j.DB.session.Close()
	j.DB = nil
	j.pool.decRef()
}

// ErrAPIConnection is returned by OpenAPI and OpenAPIFromDocs
// when the API connection cannot be made.
var ErrAPIConnection = errgo.New("cannot connect to API")

// OpenAPI opens an API connection to the controller with the given path
// and returns it along with the information used to connect. If the
// controller does not exist, the error will have a cause of
// params.ErrNotFound.
//
// If the controller API connection could not be made, the error will
// have a cause of ErrAPIConnection.
//
// The returned connection must be closed when finished with.
func (j *JEM) OpenAPI(path params.EntityPath) (_ *apiconn.Conn, err error) {
	defer j.DB.checkError(&err)
	ctl, err := j.DB.Controller(path)
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
func (j *JEM) CreateModel(conn *apiconn.Conn, p CreateModelParams) (*mongodoc.Model, *base.ModelInfo, error) {
	credPath := params.CredentialPath{
		Cloud: p.Cloud,
		EntityPath: params.EntityPath{
			User: p.Path.User,
			Name: p.Credential,
		},
	}
	cred, err := j.DB.Credential(credPath)
	if err != nil {
		return nil, nil, errgo.Mask(err, errgo.Is(params.ErrNotFound))
	}
	if err := j.updateControllerCredential(p.ControllerPath, credPath, conn, cred); err != nil {
		return nil, nil, errgo.Mask(err)
	}
	if err := j.DB.credentialAddController(credPath, p.ControllerPath); err != nil {
		return nil, nil, errgo.Mask(err)
	}
	// Create the model record in the database before actually
	// creating the model on the controller. It will have an invalid
	// UUID because it doesn't exist but that's better than creating
	// an model that we can't add locally because the name
	// already exists.
	modelDoc := &mongodoc.Model{
		Path:         p.Path,
		Controller:   p.ControllerPath,
		CreationTime: wallClock.Now(),
	}
	if err := j.DB.AddModel(modelDoc); err != nil {
		return nil, nil, errgo.Mask(err, errgo.Is(params.ErrAlreadyExists))
	}
	mmClient := modelmanager.NewClient(conn.Connection)
	m, err := mmClient.CreateModel(
		string(p.Path.Name),
		UserTag(p.Path.User).Id(),
		string(p.Cloud),
		p.Region,
		CloudCredentialTag(credPath),
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

// UpdateCredential updates the specified credential in the
// local database and then updates it on all controllers to which it is
// deployed.
func (j *JEM) UpdateCredential(ctx context.Context, cred *mongodoc.Credential) (err error) {
	if err := j.DB.updateCredential(cred); err != nil {
		return errgo.Notef(err, "cannot update local database")
	}
	c, err := j.DB.Credential(cred.Path)
	if err != nil {
		return errgo.Mask(err)
	}
	// Mark in the local database that an update is required for all controllers
	if err := j.DB.setCredentialUpdates(cred.Controllers, cred.Path); err != nil {
		// Log the error, but press on hoping to update the controllers anyway.
		logger.Errorf("cannot update controllers with updated credential %q: %s", c.Path, err)
	}
	// Attempt to update all controllers to which the credential is
	// deployed. If these fail they will be updated by the monitor.
	n := len(c.Controllers)
	// Make the channel buffered so we don't leak go-routines
	ch := make(chan struct{}, n)
	for _, ctlPath := range c.Controllers {
		go func(j *JEM, ctlPath params.EntityPath) {
			defer func() {
				ch <- struct{}{}
			}()
			defer j.Close()
			if err := j.updateControllerCredential(ctlPath, cred.Path, nil, c); err != nil {
				logger.Warningf("cannot update credential %q on controller %q: %s", c.Path, ctlPath, err)
				return
			}
		}(j.Clone(), ctlPath)
	}
	// Only wait for as along as the context allows for the updates to finish.
	for n > 0 {
		select {
		case <-ch:
			n--
		case <-ctx.Done():
		}
	}
	return nil
}

// ControllerUpdateCredentials updates the given controller by updating
// all outstanding UpdateCredentials.
func (j *JEM) ControllerUpdateCredentials(ctlPath params.EntityPath) error {
	ctl, err := j.DB.Controller(ctlPath)
	if err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrNotFound))
	}
	conn, err := j.OpenAPIFromDoc(ctl)
	if err != nil {
		return errgo.Notef(err, "cannot connect to controller")
	}
	for _, credPath := range ctl.UpdateCredentials {
		if err := j.updateControllerCredential(ctl.Path, credPath, conn, nil); err != nil {
			logger.Warningf("cannot update credential %q on controller %q: %s", credPath, ctl.Path, err)
		}
	}
	return nil
}

// updateControllerCredential updates the given credential on the given
// controller. If conn is non-nil then it will be used to communicate
// with the controller. If cred is non-nil then those credentials will be
// updated on the controller.
func (j *JEM) updateControllerCredential(
	ctlPath params.EntityPath,
	credPath params.CredentialPath,
	conn *apiconn.Conn,
	cred *mongodoc.Credential,
) error {
	var err error
	if conn == nil {
		conn, err = j.OpenAPI(ctlPath)
		if err != nil {
			return errgo.Mask(err)
		}
		defer conn.Close()
	}
	if cred == nil {
		cred, err = j.DB.Credential(credPath)
		if err != nil {
			return errgo.Mask(err, errgo.Is(params.ErrNotFound))
		}
	}
	cloudCredentialTag := CloudCredentialTag(credPath)
	cloudClient := cloudapi.NewClient(conn)
	if cred.Revoked {
		err = cloudClient.RevokeCredential(cloudCredentialTag)
	} else {
		err = cloudClient.UpdateCredential(
			cloudCredentialTag,
			jujucloud.NewCredential(jujucloud.AuthType(cred.Type), cred.Attributes),
		)
	}
	if err != nil {
		return errgo.Notef(err, "cannot update credentials")
	}
	if err := j.DB.clearCredentialUpdate(ctlPath, credPath); err != nil {
		logger.Warningf("Failed updating controller %q after successfully updating credential %q: %s", ctlPath, credPath, err)
	}
	return nil
}

// GrantModel grants the given access for the given user on the given model and updates the JEM database.
func (j *JEM) GrantModel(conn *apiconn.Conn, model *mongodoc.Model, user params.User, access string) error {
	client := modelmanager.NewClient(conn)
	if err := client.GrantModel(UserTag(user).Id(), access, model.UUID); err != nil {
		return errgo.Mask(err)
	}
	if err := j.DB.Grant(j.DB.Models(), model.Path, user); err != nil {
		// TODO (mhilton) What should be done with the changes already made to the controller.
		return errgo.Mask(err)
	}
	return nil
}

// RevokeModel revokes the given access for the given user on the given model and updates the JEM database.
func (j *JEM) RevokeModel(conn *apiconn.Conn, model *mongodoc.Model, user params.User, access string) error {
	if err := j.DB.Revoke(j.DB.Models(), model.Path, user); err != nil {
		return errgo.Mask(err)
	}
	client := modelmanager.NewClient(conn)
	if err := client.RevokeModel(UserTag(user).Id(), access, model.UUID); err != nil {
		// TODO (mhilton) What should be done with the changes already made to JEM.
		return errgo.Mask(err)
	}
	return nil
}

// DestroyModel destroys the specified model and removes it from the
// database.
//
// Note that if the model is destroyed in its controller but
// j.DeleteModel fails, a subsequent DestroyModel can can still succeed
// because client.DestroyModel will succeed when the model doesn't exist.
func (j *JEM) DestroyModel(conn *apiconn.Conn, model *mongodoc.Model) error {
	client := modelmanager.NewClient(conn)
	if err := client.DestroyModel(names.NewModelTag(model.UUID)); err != nil {
		return errgo.Mask(err)
	}
	if err := j.DB.DeleteModel(model.Path); err != nil {
		return errgo.Mask(err)
	}
	return nil
}

// DoControllers calls the given function for each controller that
// can be read by the current user that matches the given attributes.
// If the function returns an error, the iteration stops and
// DoControllers returns the error with the same cause.
func (j *JEM) DoControllers(ctx context.Context, cloud params.Cloud, region string, do func(c *mongodoc.Controller) error) error {
	// Query all the controllers that match the attributes, building
	// up all the possible values.
	q := j.DB.controllerLocationQuery(cloud, region, false)
	// Sort by _id so that we can make easily reproducible tests.
	iter := j.DB.NewCanReadIter(ctx, q.Sort("_id").Iter())
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

// SelectController chooses a controller that matches the cloud and region criteria, if specified.
func (j *JEM) SelectController(ctx context.Context, cloud params.Cloud, region string) (params.EntityPath, params.Cloud, string, error) {
	var controllers []mongodoc.Controller
	err := j.DoControllers(ctx, cloud, region, func(c *mongodoc.Controller) error {
		controllers = append(controllers, *c)
		return nil
	})
	if err != nil {
		return params.EntityPath{}, "", "", errgo.Mask(err, errgo.Is(params.ErrBadRequest))
	}
	if len(controllers) == 0 {
		return params.EntityPath{}, "", "", errgo.WithCausef(nil, params.ErrNotFound, "no matching controllers found")
	}
	// Choose a random controller.
	// TODO select a controller more intelligently, for example
	// by choosing the most lightly loaded controller
	n := randIntn(len(controllers))
	return controllers[n].Path, params.Cloud(controllers[n].Cloud.Name), region, nil
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
// CredentialPath.
func CloudCredentialTag(p params.CredentialPath) names.CloudCredentialTag {
	return names.NewCloudCredentialTag(fmt.Sprintf("%s/%s@external/%s", p.Cloud, p.User, p.Name))
}
