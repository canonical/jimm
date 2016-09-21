// Copyright 2016 Canonical Ltd.

package auth

import (
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/juju/idmclient"
	"github.com/juju/loggo"
	"github.com/juju/utils"
	"github.com/juju/utils/clock"
	"golang.org/x/net/context"
	"gopkg.in/errgo.v1"
	"gopkg.in/macaroon-bakery.v1/bakery"
	"gopkg.in/macaroon-bakery.v1/bakery/checkers"
	"gopkg.in/macaroon-bakery.v1/bakery/mgostorage"
	"gopkg.in/macaroon-bakery.v1/httpbakery"
	"gopkg.in/macaroon.v1"
	"gopkg.in/mgo.v2"

	"github.com/CanonicalLtd/jem/internal/servermon"
	"github.com/CanonicalLtd/jem/params"
)

var logger = loggo.GetLogger("jem.internal.auth")

// wallclock provides access to the current time. It is defined as a variable so that it can be overridden in tests.
var wallclock clock.Clock = clock.WallClock

const (
	usernameAttr = "username"
)

// Params holds parameters for the NewPool function.
type Params struct {
	// Bakery holds the bakery service used to create and verify
	// macaroons.
	Bakery *bakery.Service

	// RootKeys holds a mgostorage.RootKeys instance, used when
	// minting or verifying macaroons.
	RootKeys *mgostorage.RootKeys

	// RootKeysPolicy is the storage policy used with RootKeys.
	RootKeysPolicy mgostorage.Policy

	// MacaroonCollection holds a mgo.Collection which can be used
	// with RootKeys to store and retrive macaroons.
	MacaroonCollection *mgo.Collection

	// MaxCollectionClones holds the maximum number of clones sharing
	// a database connection before a new copy is created.
	MaxCollectionClones int

	// MaxCollectionAge holds the maximum number amount of time for
	// which a database connection will be shared before a new copy
	// is created.
	MaxCollectionAge time.Duration

	// PermChecker holds a PermChecker that will be used to check if
	// the current user is a member of an ACL.
	PermChecker *idmclient.PermChecker

	// IdentityLocation holds the location of the third party identity service.
	IdentityLocation string
}

// A Pool contains a pool of authenticator objects.
type Pool struct {
	params     Params
	pool       sync.Pool
	mu         sync.Mutex
	collection *collection
	closed     bool
}

// NewPool creates a new Pool from which Authenticator objects may be
// retrieved.
func NewPool(p Params) *Pool {
	servermon.DatabaseSessions.Inc()
	pool := &Pool{
		params: p,
		collection: &collection{
			Collection: p.MacaroonCollection.With(p.MacaroonCollection.Database.Session.Copy()),
			created:    wallclock.Now(),
			n:          1,
		},
	}
	pool.pool = sync.Pool{
		New: pool.new,
	}
	return pool
}

// Get retrieves an Authenticator object from the pool.
func (p *Pool) Get() *Authenticator {
	servermon.AuthenticatorPoolGet.Inc()
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		panic("Get called on closed pool")
	}
	p.collection.mu.Lock()
	if p.collection.n > p.params.MaxCollectionClones || p.collection.created.Add(p.params.MaxCollectionAge).Before(wallclock.Now()) {
		c := p.collection
		p.collection = c.Copy()
		p.collection.mu.Lock()
		c.mu.Unlock()
		c.Close()
	}
	p.collection.n++
	p.collection.mu.Unlock()
	return p.pool.Get().(*Authenticator).with(p.collection)
}

// Put returns an Authenticator object to the pool.
func (p *Pool) Put(a *Authenticator) {
	servermon.AuthenticatorPoolPut.Inc()
	a.bakery = nil
	a.collection.Close()
	a.collection = nil
	p.pool.Put(a)
}

// new creates a new authenticator instance.
func (p *Pool) new() interface{} {
	servermon.AuthenticatorPoolNew.Inc()
	return &Authenticator{
		params: p.params,
	}
}

// Close closes the pool and frees it's resources.
func (p *Pool) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closed = true
	p.collection.Close()
	p.collection = nil
}

// An Authenticator can be used to authenticate a connection.
type Authenticator struct {
	params     Params
	bakery     *bakery.Service
	collection *collection
}

// with prepares this authenticator instance for use with the given
// collection.
func (a *Authenticator) with(c *collection) *Authenticator {
	a.collection = c
	a.bakery = a.params.Bakery.WithRootKeyStore(
		a.params.RootKeys.NewStorage(
			a.params.MacaroonCollection.With(c.Database.Session),
			a.params.RootKeysPolicy,
		),
	)
	return a
}

// Authenticate checks all macaroons in mss. If any are valid then the
// returned context will have authorization information attached,
// otherwise the original context is returned unchanged. If the returned
// macaroon is non-nil then it should be sent to the client and if
// discharged can be used to gain access.
func (a *Authenticator) Authenticate(ctx context.Context, mss []macaroon.Slice, checker checkers.Checker) (context.Context, *macaroon.Macaroon, error) {
	attrMap, verr := a.bakery.CheckAny(mss, nil, checkers.New(checker, checkers.TimeBefore))
	if verr == nil {
		servermon.AuthenticationSuccessCount.Inc()
		return context.WithValue(ctx, authKey, authentication{
			username:    attrMap[usernameAttr],
			permChecker: a.params.PermChecker,
		}), nil, nil
	}
	servermon.AuthenticationFailCount.Inc()
	if _, ok := errgo.Cause(verr).(*bakery.VerificationError); !ok {
		return ctx, nil, errgo.Mask(verr, errgo.Is(params.ErrUnauthorized))
	}
	// Macaroon verification failed: mint a new macaroon.
	m, err := a.newMacaroon()
	if err != nil {
		return ctx, nil, errgo.Notef(err, "cannot mint macaroon")
	}
	return ctx, m, verr
}

// AuthenticateRequest is used to authenticate and http.Request. If the
// request authenticates then the returned context will have
// authorization information attached, otherwise the original context
// will be returned unchanged. If a discharge is required the returned
// error will be a discharge required error.
func (a *Authenticator) AuthenticateRequest(ctx context.Context, req *http.Request) (context.Context, error) {
	ctx, m, err := a.Authenticate(ctx, httpbakery.RequestMacaroons(req), checkers.New(httpbakery.Checkers(req)))
	if m == nil {
		return ctx, errgo.Mask(err, errgo.Is(params.ErrUnauthorized))
	}
	// Request that this macaroon be supplied for all requests
	// to the whole handler. We use a relative path because
	// the JEM server is conventionally under an external
	// root path, with other services also under the same
	// externally visible host name, and we don't want our
	// cookies to be presented to those services.
	cookiePath := "/"
	if p, err := utils.RelativeURLPath(req.RequestURI, "/"); err != nil {
		// Should never happen, as RequestURI should always be absolute.
		logger.Infof("cannot make relative URL from %v", req.RequestURI)
	} else {
		cookiePath = p
	}
	dischargeErr := httpbakery.NewDischargeRequiredErrorForRequest(m, cookiePath, err, req)
	dischargeErr.(*httpbakery.Error).Info.CookieNameSuffix = "authn"
	return ctx, dischargeErr
}

// newMacaroon returns a macaroon that, when discharged, will allow
// access to JIMM.
func (a *Authenticator) newMacaroon() (*macaroon.Macaroon, error) {
	return a.bakery.NewMacaroon("", nil, []checkers.Caveat{
		checkers.NeedDeclaredCaveat(
			checkers.Caveat{
				Location:  a.params.IdentityLocation,
				Condition: "is-authenticated-user",
			},
			usernameAttr,
		),
	})
}

// A collection represents a mgo.Collection that an Authenticator can use
// to look up macaroon information.
type collection struct {
	*mgo.Collection
	created time.Time
	mu      sync.Mutex
	n       int
}

// Copy creates a new collection with a copied underlaying database
// connection.
func (c *collection) Copy() *collection {
	servermon.DatabaseSessions.Inc()
	return &collection{
		Collection: c.Collection.With(c.Collection.Database.Session.Copy()),
		created:    wallclock.Now(),
		n:          1,
	}
}

// Closes closes this collection.
func (c *collection) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.n--
	if c.n == 0 {
		c.Collection.Database.Session.Close()
		c.Collection = nil
		servermon.DatabaseSessions.Dec()
	}
}

type contextKey int

var authKey contextKey = 0

type allower interface {
	allow(acl []string) (bool, error)
}

type authentication struct {
	username    string
	permChecker *idmclient.PermChecker
}

func (a authentication) allow(acl []string) (bool, error) {
	ok, err := a.permChecker.Allow(a.username, acl)
	if err != nil {
		return false, errgo.Mask(err)
	} else if ok {
		return true, nil
	}
	logger.Infof("%s is not authorized for ACL %s", a.username, acl)
	return false, nil
}

// CheckIsUser checks whether the currently authenticated user can
// act as the given name.
func CheckIsUser(ctx context.Context, user params.User) error {
	return CheckACL(ctx, []string{string(user)})
}

// CheckACL checks whether the currently authenticated user is
// allowed to access an entity with the given ACL.
// It returns params.ErrUnauthorized if not.
func CheckACL(ctx context.Context, acl []string) error {
	v := ctx.Value(authKey)
	if v == nil {
		return params.ErrUnauthorized
	}
	a := v.(allower)

	ok, err := a.allow(acl)
	if err != nil {
		return errgo.Notef(err, "cannot check permissions")
	}
	if !ok {
		return params.ErrUnauthorized
	}
	return nil
}

// ACLEntity represents a mongo entity with access permissions.
type ACLEntity interface {
	GetACL() params.ACL
	Owner() params.User
}

// CheckCanRead checks whether the current user is
// allowed to read the given entity. The owner
// is always allowed to access an entity, regardless
// of its ACL.
func CheckCanRead(ctx context.Context, e ACLEntity) error {
	acl := append([]string{string(e.Owner())}, e.GetACL().Read...)
	return CheckACL(ctx, acl)
}

// Username returns the name of the user authenticated on the given
// context. If no user is authenticated then an empty string is returned.
func Username(ctx context.Context) string {
	v := ctx.Value(authKey)
	if v == nil {
		return ""
	}
	a, ok := v.(authentication)
	if !ok {
		return ""
	}
	return a.username
}

type testAuthentication []string

func (a testAuthentication) allow(acl []string) (bool, error) {
	for _, s := range acl {
		if s == "everyone" {
			return true, nil
		}
		i := sort.SearchStrings(a, s)
		if i == len(a) || a[i] != s {
			continue
		}
		return true, nil
	}
	return false, nil
}

// AuthenticateForTest authenticates the given context as if it was for a
// user that is a member of all the given groups. As the name implies it
// is expected to be used in tests.
func AuthenticateForTest(ctx context.Context, groups ...string) context.Context {
	sort.Strings(groups)
	return context.WithValue(ctx, authKey, testAuthentication(groups))
}
