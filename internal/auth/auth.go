// Copyright 2016 Canonical Ltd.

package auth

import (
	"net/http"

	"github.com/juju/idmclient"
	"github.com/juju/loggo"
	"github.com/juju/utils"
	"golang.org/x/net/context"
	"gopkg.in/errgo.v1"
	"gopkg.in/macaroon-bakery.v1/bakery"
	"gopkg.in/macaroon-bakery.v1/bakery/checkers"
	"gopkg.in/macaroon-bakery.v1/bakery/mgostorage"
	"gopkg.in/macaroon-bakery.v1/httpbakery"
	"gopkg.in/macaroon.v1"
	"gopkg.in/mgo.v2"

	"github.com/CanonicalLtd/jem/internal/mgosession"
	"github.com/CanonicalLtd/jem/internal/servermon"
	"github.com/CanonicalLtd/jem/params"
)

var logger = loggo.GetLogger("jem.internal.auth")

const usernameAttr = "username"

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
	// with RootKeys to store and retrieve macaroons.
	MacaroonCollection *mgo.Collection

	// PermChecker holds a PermChecker that will be used to check if
	// the current user is a member of an ACL.
	PermChecker *idmclient.PermChecker

	// IdentityLocation holds the location of the third party identity service.
	IdentityLocation string

	// SessionPool holds a pool from which session objects are
	// taken to be used in database operations.
	SessionPool *mgosession.Pool
}

// A Pool contains a pool of authenticator objects.
type Pool struct {
	params Params
}

// NewPool creates a new Pool from which Authenticator objects may be
// retrieved.
func NewPool(params Params) (*Pool, error) {
	p := &Pool{
		params: params,
	}
	auth := p.Authenticator()
	defer auth.Close()
	if err := params.RootKeys.EnsureIndex(p.rootKeyCollection(auth.session)); err != nil {
		return nil, errgo.Notef(err, "cannot ensure index on root key store")
	}
	return p, nil
}

// Authenticator retrieves an Authenticator object from the pool, which
// must be closed after use.
func (p *Pool) Authenticator() *Authenticator {
	servermon.AuthenticatorPoolGet.Inc()
	session := p.params.SessionPool.Session()
	return &Authenticator{
		pool: p,
		bakery: p.params.Bakery.WithRootKeyStore(p.params.RootKeys.NewStorage(
			p.rootKeyCollection(session),
			p.params.RootKeysPolicy,
		)),
		session: session,
	}
}

func (p *Pool) rootKeyCollection(session *mgosession.Session) *mgo.Collection {
	return p.params.MacaroonCollection.With(session.Session)
}

// An Authenticator can be used to authenticate a connection.
type Authenticator struct {
	closed  bool
	pool    *Pool
	bakery  *bakery.Service
	session *mgosession.Session
}

// Close closes the authenticator instance.
func (a *Authenticator) Close() {
	if a.closed {
		return
	}
	a.closed = true
	servermon.AuthenticatorPoolPut.Inc()
	a.bakery = nil
	a.session.Close()
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
		return context.WithValue(ctx, authKey{}, authentication{
			username_:   attrMap[usernameAttr],
			permChecker: a.pool.params.PermChecker,
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
				Location:  a.pool.params.IdentityLocation,
				Condition: "is-authenticated-user",
			},
			usernameAttr,
		),
	})
}

type authKey struct{}

func fromContext(ctx context.Context) authInfo {
	if info, _ := ctx.Value(authKey{}).(authInfo); info != nil {
		return info
	}
	return noAuth{}
}

type authInfo interface {
	allow(acl []string) (bool, error)
	username() string
}

type authentication struct {
	username_   string
	permChecker *idmclient.PermChecker
}

func (a authentication) allow(acl []string) (bool, error) {
	ok, err := a.permChecker.Allow(a.username_, acl)
	if err != nil {
		return false, errgo.Mask(err)
	} else if ok {
		return true, nil
	}
	logger.Infof("%s is not authorized for ACL %s", a.username_, acl)
	return false, nil
}

func (a authentication) username() string {
	return a.username_
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
	ok, err := fromContext(ctx).allow(acl)
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
	return fromContext(ctx).username()
}

type testAuthentication []string

func (a testAuthentication) allow(acl []string) (bool, error) {
	for _, g := range acl {
		if g == "everyone" {
			return true, nil
		}
		for _, allowg := range a {
			if allowg == g {
				return true, nil
			}
		}
	}
	return false, nil
}

func (a testAuthentication) username() string {
	return a[0]
}

// ContextWithUser returns the given context as if it had been returned
// from Authenticate with the given authenticated user
// and as if the user was a member of all the given groups.
func ContextWithUser(ctx context.Context, username string, groups ...string) context.Context {
	groups = append([]string{username}, groups...)
	return context.WithValue(ctx, authKey{}, testAuthentication(groups))
}

type noAuth struct{}

func (noAuth) username() string {
	return ""
}

func (noAuth) allow([]string) (bool, error) {
	return false, nil
}
