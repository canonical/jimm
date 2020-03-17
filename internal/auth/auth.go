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
	"gopkg.in/macaroon-bakery.v2/httpbakery"
	"gopkg.in/macaroon.v2"

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
func (a *Authenticator) Authenticate(ctx context.Context, v bakery.Version, mss []macaroon.Slice) (identchecker.ACLIdentity, *bakery.Macaroon, error) {
	ai, verr := a.bakery.Checker.Auth(mss...).Allow(ctx, identchecker.LoginOp)
	if verr == nil {
		servermon.AuthenticationSuccessCount.Inc()
		return ai.Identity.(identchecker.ACLIdentity), nil, nil
	}
	if !bakery.IsDischargeRequiredError(errgo.Cause(verr)) {
		servermon.AuthenticationFailCount.Inc()
		return nil, nil, errgo.Mask(verr, errgo.Is(params.ErrUnauthorized))
	}

	derr := errgo.Cause(verr).(*bakery.DischargeRequiredError)
	// Macaroon verification failed: mint a new macaroon.
	m, err := a.bakery.Oven.NewMacaroon(ctx, v, derr.Caveats, derr.Ops...)
	if err != nil {
		return nil, nil, errgo.Notef(err, "cannot mint macaroon")
	}
	return nil, m, verr
}

// AuthenticateRequest is used to authenticate and http.Request. If the
// request authenticates then the returned context will have
// authorization information attached, otherwise the original context
// will be returned unchanged. If a discharge is required the returned
// error will be a discharge required error.
func (a *Authenticator) AuthenticateRequest(ctx context.Context, req *http.Request) (identchecker.ACLIdentity, error) {
	id, m, err := a.Authenticate(ctx, httpbakery.RequestVersion(req), httpbakery.RequestMacaroons(req))
	if m == nil {
		return id, errgo.Mask(err, errgo.Is(params.ErrUnauthorized))
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
	return nil, dischargeErr
}

// CheckIsUser checks whether the given identity can act as the given
// user. It returns params.ErrUnauthorized if not.
func CheckIsUser(ctx context.Context, id identchecker.ACLIdentity, user params.User) error {
	return CheckACL(ctx, id, []string{string(user)})
}

// CheckACL checks whether the the given identity is allowed to access an
// entity with the given ACL. It returns params.ErrUnauthorized if not.
func CheckACL(ctx context.Context, id identchecker.ACLIdentity, acl []string) error {
	ok, err := id.Allow(ctx, acl)
	if err != nil {
		return errgo.Notef(err, "cannot check permissions")
	}
	if !ok {
		zapctx.Debug(ctx, "user not authorized",
			zap.String("user", id.Id()),
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

// CheckCanRead checks whether the given identity is allowed to read the
// given entity. The owner is always allowed to access an entity,
// regardless of its ACL.
func CheckCanRead(ctx context.Context, id identchecker.ACLIdentity, e ACLEntity) error {
	acl := append([]string{string(e.Owner())}, e.GetACL().Read...)
	return CheckACL(ctx, id, acl)
}

// CheckIsAdmin checks whether the current user is an admin on the given
// entity. The owner is always allowed to access an entity, regardless of
// its ACL.
func CheckIsAdmin(ctx context.Context, id identchecker.ACLIdentity, e ACLEntity) error {
	acl := append([]string{string(e.Owner())}, e.GetACL().Admin...)
	return CheckACL(ctx, id, acl)
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

type authKey struct{}

// ContextWithIdentity returns the given context with the given identity
// attached.
func ContextWithIdentity(ctx context.Context, id identchecker.ACLIdentity) context.Context {
	return context.WithValue(ctx, authKey{}, id)
}

// IdentityFromContext returns the identity that was previously attached
// to the context using ContextWithIdentity. If no identity has been
// attached then an Identity with no authority is returned.
func IdentityFromContext(ctx context.Context) identchecker.ACLIdentity {
	if aid, _ := ctx.Value(authKey{}).(identchecker.ACLIdentity); aid != nil {
		return aid
	}
	return noIdentity{}
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
