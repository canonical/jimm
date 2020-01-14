// Copyright 2016 Canonical Ltd.

package auth

import (
	"context"
	"net/http"
	"strings"

	"github.com/juju/utils"
	"go.uber.org/zap"
	candidclient "gopkg.in/CanonicalLtd/candidclient.v1"
	"gopkg.in/errgo.v1"
	"gopkg.in/macaroon-bakery.v2/bakery"
	"gopkg.in/macaroon-bakery.v2/bakery/checkers"
	"gopkg.in/macaroon-bakery.v2/bakery/identchecker"
	"gopkg.in/macaroon-bakery.v2/bakery/mgorootkeystore"
	"gopkg.in/macaroon-bakery.v2/httpbakery"
	"gopkg.in/macaroon.v2"
	"gopkg.in/mgo.v2"

	"github.com/CanonicalLtd/jimm/internal/mgosession"
	"github.com/CanonicalLtd/jimm/internal/servermon"
	"github.com/CanonicalLtd/jimm/internal/zapctx"
	"github.com/CanonicalLtd/jimm/internal/zaputil"
	"github.com/CanonicalLtd/jimm/params"
)

const usernameAttr = "username"

// An Authenticator can be used to authenticate a connection.
type Authenticator struct {
	bakery *identchecker.Bakery
}

// NewAuthenticator initialises a new Authenticator.
func NewAuthenticator(b *identchecker.Bakery) *Authenticator {
	return &Authenticator{
		bakery: b,
	}
}

// Authenticate checks all macaroons in mss. If any are valid then the
// returned context will have authorization information attached,
// otherwise the original context is returned unchanged. If the returned
// macaroon is non-nil then it should be sent to the client and if
// discharged can be used to gain access.
func (a *Authenticator) Authenticate(ctx context.Context, v bakery.Version, mss []macaroon.Slice) (context.Context, *bakery.Macaroon, error) {

	ai, verr := a.bakery.Checker.Auth(mss...).Allow(ctx, identchecker.LoginOp)
	if verr == nil {
		servermon.AuthenticationSuccessCount.Inc()
		return context.WithValue(ctx, authKey{}, ai.Identity), nil, nil
	}
	if !bakery.IsDischargeRequiredError(errgo.Cause(verr)) {
		servermon.AuthenticationFailCount.Inc()
		return ctx, nil, errgo.Mask(verr, errgo.Is(params.ErrUnauthorized))
	}

	derr := errgo.Cause(verr).(*bakery.DischargeRequiredError)
	// Macaroon verification failed: mint a new macaroon.
	m, err := a.bakery.Oven.NewMacaroon(ctx, v, derr.Caveats, derr.Ops...)
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
	ctx, m, err := a.Authenticate(ctx, httpbakery.RequestVersion(req), httpbakery.RequestMacaroons(req))
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
		zapctx.Error(ctx, "cannot make relative URL", zap.String("request-uri", req.RequestURI), zaputil.Error(err))
	} else {
		cookiePath = p
	}
	dischargeErr := httpbakery.NewDischargeRequiredError(httpbakery.DischargeRequiredErrorParams{
		Macaroon:         m,
		OriginalError:    err,
		CookiePath:       cookiePath,
		CookieNameSuffix: "authn",
		Request:          req,
	})
	return ctx, dischargeErr
}

type authKey struct{}

func fromContext(ctx context.Context) identchecker.ACLIdentity {
	if aid, _ := ctx.Value(authKey{}).(identchecker.ACLIdentity); aid != nil {
		return aid
	}
	return noIdentity{}
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
	aid := fromContext(ctx)
	ok, err := aid.Allow(ctx, acl)
	if err != nil {
		return errgo.Notef(err, "cannot check permissions")
	}
	if !ok {
		zapctx.Debug(ctx, "user not authorized",
			zap.String("user", aid.Id()),
			zap.Strings("acl", acl),
		)
		return params.ErrUnauthorized
	}
	return nil
}

// ACLEntity represents a mongo entity with access permissions.
type ACLEntity interface {
	GetACL() params.ACL
	Owner() params.User
}

// CheckCanRead checks whether the current user is allowed to read the
// given entity. The owner is always allowed to access an entity,
// regardless of its ACL.
func CheckCanRead(ctx context.Context, e ACLEntity) error {
	acl := append([]string{string(e.Owner())}, e.GetACL().Read...)
	return CheckACL(ctx, acl)
}

// CheckIsAdmin checks whether the current user is an admin on the given
// entity. The owner is always allowed to access an entity, regardless of
// its ACL.
func CheckIsAdmin(ctx context.Context, e ACLEntity) error {
	acl := append([]string{string(e.Owner())}, e.GetACL().Admin...)
	return CheckACL(ctx, acl)
}

// Username returns the name of the user authenticated on the given
// context. If no user is authenticated then an empty string is returned.
func Username(ctx context.Context) string {
	return fromContext(ctx).Id()
}

type testIdentity []string

func (a testIdentity) Allow(_ context.Context, acl []string) (bool, error) {
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

func (a testIdentity) Id() string {
	return a[0]
}

func (a testIdentity) Domain() string {
	return ""
}

// ContextWithUser returns the given context as if it had been returned
// from Authenticate with the given authenticated user
// and as if the user was a member of all the given groups.
func ContextWithUser(ctx context.Context, username string, groups ...string) context.Context {
	groups = append([]string{username}, groups...)
	return context.WithValue(ctx, authKey{}, testIdentity(groups))
}

type noIdentity struct{}

func (noIdentity) Id() string {
	return ""
}

func (noIdentity) Domain() string {
	return ""
}

func (noIdentity) Allow(context.Context, []string) (bool, error) {
	return false, nil
}

type RootKeyStoreParams struct {
	// Pool contains the mgo session pool used by the application.
	Pool *mgosession.Pool

	// RootKeys contains an mgorootkeystore.RootKeys from which
	// stores can be derived.
	RootKeys *mgorootkeystore.RootKeys

	// Policy contains the root key policy used by the application.
	Policy mgorootkeystore.Policy

	// Collection contains the mgo Collection that contains the root keys.
	Collection *mgo.Collection
}

// RootKeyStore implements a bakery.RootKeyStore using an underlying
// mgorootkeysstore.RootKeys.
type RootKeyStore struct {
	p RootKeyStoreParams
}

// NewRootKeyStore creates a new RootKeyStore.
func NewRootKeyStore(p RootKeyStoreParams) *RootKeyStore {
	return &RootKeyStore{p: p}
}

// Get implements bakery.RootKeyStore.Get.
func (s *RootKeyStore) Get(ctx context.Context, id []byte) ([]byte, error) {
	session := s.p.Pool.Session(ctx)
	defer session.Close()

	return s.p.RootKeys.NewStore(s.p.Collection.With(session), s.p.Policy).Get(ctx, id)
}

// RootKey implements bakery.RootKeyStore.RootKey.
func (s *RootKeyStore) RootKey(ctx context.Context) ([]byte, []byte, error) {
	session := s.p.Pool.Session(ctx)
	defer session.Close()

	return s.p.RootKeys.NewStore(s.p.Collection.With(session), s.p.Policy).RootKey(ctx)
}

type IdentityClientParams struct {
	// CandidClient contains the underlying candid client for the
	// identity client.
	CandidClient *candidclient.Client

	// Domain contains a domain users must be a member of to
	// successfully log in.
	Domain string
}

// NewIdentityClient creates a new identity client for the authorizer.
func NewIdentityClient(p IdentityClientParams) *IdentityClient {
	return &IdentityClient{p: p}
}

// An IdentityClient implements identchecker.IdentityClient.
type IdentityClient struct {
	p IdentityClientParams
}

// IdentityFromContext implements IdentityClient.IdentityFromContext.
func (c *IdentityClient) IdentityFromContext(ctx context.Context) (identchecker.Identity, []checkers.Caveat, error) {
	condition := "is-authenticated-user"
	if c.p.Domain != "" {
		condition += " @" + c.p.Domain
	}
	cav := checkers.NeedDeclaredCaveat(
		checkers.Caveat{
			Location:  c.p.CandidClient.Client.BaseURL,
			Condition: condition,
		},
		usernameAttr,
	)

	return nil, []checkers.Caveat{cav}, nil
}

// DeclaredIdentity implements IdentityClient.DeclaredIdentity.
func (c *IdentityClient) DeclaredIdentity(ctx context.Context, declared map[string]string) (identchecker.Identity, error) {
	id, err := c.p.CandidClient.DeclaredIdentity(ctx, declared)
	if err != nil {
		return nil, errgo.Mask(err)
	}
	if c.p.Domain == "" || strings.HasSuffix(id.Id(), "@"+c.p.Domain) {
		return id, nil
	}

	return nil, &bakery.VerificationError{
		Reason: errgo.Newf("user not in %q domain", c.p.Domain),
	}
}
