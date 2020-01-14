// Copyright 2016 Canonical Ltd.

package auth_test

import (
	"context"
	"net/http"
	"time"

	jc "github.com/juju/testing/checkers"
	candidclient "gopkg.in/CanonicalLtd/candidclient.v1"
	"gopkg.in/CanonicalLtd/candidclient.v1/candidtest"
	gc "gopkg.in/check.v1"
	"gopkg.in/errgo.v1"
	"gopkg.in/macaroon-bakery.v2/bakery"
	"gopkg.in/macaroon-bakery.v2/bakery/checkers"
	"gopkg.in/macaroon-bakery.v2/bakery/identchecker"
	"gopkg.in/macaroon-bakery.v2/bakery/mgorootkeystore"
	"gopkg.in/macaroon-bakery.v2/httpbakery"
	"gopkg.in/macaroon.v2"

	"github.com/CanonicalLtd/jimm/internal/auth"
	"github.com/CanonicalLtd/jimm/internal/jemtest"
	"github.com/CanonicalLtd/jimm/internal/mgosession"
	"github.com/CanonicalLtd/jimm/params"
)

type authSuite struct {
	jemtest.IsolatedMgoSuite
	idmSrv        *candidtest.Server
	authenticator *auth.Authenticator
	sessionPool   *mgosession.Pool
}

var _ = gc.Suite(&authSuite{})

func (s *authSuite) SetUpTest(c *gc.C) {
	ctx := context.Background()

	s.IsolatedMgoSuite.SetUpTest(c)
	s.idmSrv = candidtest.NewServer()

	db := s.Session.DB("auth")
	s.sessionPool = mgosession.NewPool(ctx, s.Session, 5)
	rks := auth.NewRootKeyStore(auth.RootKeyStoreParams{
		Pool:     s.sessionPool,
		RootKeys: mgorootkeystore.NewRootKeys(100),
		Policy: mgorootkeystore.Policy{
			ExpiryDuration: 1 * time.Second,
		},
		Collection: db.C("macaroons"),
	})

	key, err := bakery.GenerateKey()
	c.Assert(err, gc.Equals, nil)

	idmClient, err := candidclient.New(candidclient.NewParams{
		BaseURL: s.idmSrv.URL.String(),
		Client:  s.idmSrv.Client("test-user"),
	})
	c.Assert(err, gc.Equals, nil)

	s.authenticator = auth.NewAuthenticator(identchecker.NewBakery(identchecker.BakeryParams{
		RootKeyStore: rks,
		Locator:      s.idmSrv,
		Key:          key,
		IdentityClient: auth.NewIdentityClient(auth.IdentityClientParams{
			CandidClient: idmClient,
		}),
		Authorizer: identchecker.AuthorizerFunc(func(ctx context.Context, id identchecker.Identity, op bakery.Op) (bool, []checkers.Caveat, error) {
			return id != nil && op == identchecker.LoginOp, nil, nil
		}),
		Location: "here",
	}))
}

func (s *authSuite) TearDownTest(c *gc.C) {
	s.sessionPool.Close()
	s.IsolatedMgoSuite.TearDownTest(c)
}

func (s *authSuite) TestAuthenticateNoMacaroon(c *gc.C) {
	ctx := context.Background()
	ctx2, m, err := s.authenticator.Authenticate(ctx, bakery.Version3, nil)
	c.Assert(ctx, jc.DeepEquals, ctx2)
	c.Assert(err, gc.ErrorMatches, `macaroon discharge required: authentication required`)
	c.Assert(m, gc.Not(gc.IsNil))
}

func (s *authSuite) TestAuthenticate(c *gc.C) {
	ctx := context.Background()
	_, m, _ := s.authenticator.Authenticate(ctx, bakery.Version3, nil)
	ms := s.discharge(ctx, c, m, "bob")
	ctx2, m, err := s.authenticator.Authenticate(ctx, bakery.Version3, []macaroon.Slice{ms})
	c.Assert(err, gc.Equals, nil)
	c.Assert(m, gc.IsNil)
	err = auth.CheckIsUser(ctx, "bob")
	c.Assert(err, gc.ErrorMatches, `unauthorized`)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrUnauthorized)
	err = auth.CheckIsUser(ctx2, "bob")
	c.Assert(err, gc.Equals, nil)
}

func (s *authSuite) TestAuthenticateRequest(c *gc.C) {
	ctx := context.Background()
	req, err := http.NewRequest("GET", "/", nil)
	req.RequestURI = "/"
	c.Assert(err, gc.Equals, nil)
	ctx2, err := s.authenticator.AuthenticateRequest(ctx, req)
	c.Assert(ctx2, gc.Equals, ctx)
	c.Assert(err, gc.ErrorMatches, `macaroon discharge required: authentication required`)
	herr, ok := err.(*httpbakery.Error)
	c.Assert(ok, gc.Equals, true)
	ms := s.discharge(ctx, c, herr.Info.Macaroon, "bob")
	cookie, err := httpbakery.NewCookie(nil, ms)
	c.Assert(err, gc.Equals, nil)
	req.AddCookie(cookie)
	ctx3, err := s.authenticator.AuthenticateRequest(ctx, req)
	c.Assert(err, gc.Equals, nil)
	err = auth.CheckIsUser(ctx2, "bob")
	c.Assert(err, gc.ErrorMatches, `unauthorized`)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrUnauthorized)
	err = auth.CheckIsUser(ctx3, "bob")
	c.Assert(err, gc.Equals, nil)
}

func (s *authSuite) TestCheckIsUser(c *gc.C) {
	ctx := auth.ContextWithUser(context.Background(), "bob")
	err := auth.CheckIsUser(ctx, "bob")
	c.Assert(err, gc.Equals, nil)
	err = auth.CheckIsUser(ctx, "alice")
	c.Assert(err, gc.ErrorMatches, `unauthorized`)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrUnauthorized)
}

func (s *authSuite) TestCheckACL(c *gc.C) {
	ctx := auth.ContextWithUser(context.Background(), "bob")
	err := auth.CheckACL(ctx, []string{"bob", "charlie"})
	c.Assert(err, gc.Equals, nil)
	err = auth.CheckACL(ctx, []string{"alice", "charlie"})
	c.Assert(err, gc.ErrorMatches, `unauthorized`)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrUnauthorized)
	err = auth.CheckACL(ctx, []string{"alice", "charlie", "everyone"})
	c.Assert(err, gc.Equals, nil)
}

var canReadTests = []struct {
	owner   string
	readers []string
	allowed bool
}{{
	owner:   "bob",
	allowed: true,
}, {
	owner: "fred",
}, {
	owner:   "fred",
	readers: []string{"bob"},
	allowed: true,
}, {
	owner:   "fred",
	readers: []string{"bob-group"},
	allowed: true,
}, {
	owner:   "bob-group",
	allowed: true,
}, {
	owner:   "fred",
	readers: []string{"everyone"},
	allowed: true,
}, {
	owner:   "fred",
	readers: []string{"harry", "john"},
}, {
	owner:   "fred",
	readers: []string{"harry", "bob-group"},
	allowed: true,
}}

func (s *authSuite) TestCheckCanRead(c *gc.C) {
	ctx := auth.ContextWithUser(context.Background(), "bob", "bob-group")
	for i, test := range canReadTests {
		c.Logf("%d. %q %#v", i, test.owner, test.readers)
		err := auth.CheckCanRead(ctx, testEntity{
			owner:   test.owner,
			readers: test.readers,
		})
		if test.allowed {
			c.Assert(err, gc.Equals, nil)
			continue
		}
		c.Assert(err, gc.ErrorMatches, `unauthorized`)
		c.Assert(errgo.Cause(err), gc.Equals, params.ErrUnauthorized)
	}
}

func (s *authSuite) TestUsername(c *gc.C) {
	ctx := context.Background()

	c.Assert(auth.Username(ctx), gc.Equals, "")
	_, m, _ := s.authenticator.Authenticate(context.Background(), bakery.Version3, nil)
	ms := s.discharge(ctx, c, m, "bob")
	ctx, _, err := s.authenticator.Authenticate(context.Background(), bakery.Version3, []macaroon.Slice{ms})
	c.Assert(err, gc.Equals, nil)
	c.Assert(auth.Username(ctx), gc.Equals, "bob")
}

func (s *authSuite) discharge(ctx context.Context, c *gc.C, m *bakery.Macaroon, username string, groups ...string) macaroon.Slice {
	s.idmSrv.AddUser(username, groups...)
	s.idmSrv.SetDefaultUser(username)
	cl := s.idmSrv.Client(username)
	ms, err := cl.DischargeAll(ctx, m)
	c.Assert(err, gc.Equals, nil)
	return ms
}

type testEntity struct {
	owner   string
	readers []string
}

func (e testEntity) Owner() params.User {
	return params.User(e.owner)
}

func (e testEntity) GetACL() params.ACL {
	return params.ACL{
		Read: e.readers,
	}
}

var _ auth.ACLEntity = testEntity{}
