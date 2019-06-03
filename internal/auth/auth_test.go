// Copyright 2016 Canonical Ltd.

package auth_test

import (
	"context"
	"net/http"
	"time"

	"github.com/juju/idmclient"
	"github.com/juju/idmclient/idmtest"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/errgo.v1"
	"gopkg.in/macaroon-bakery.v2-unstable/bakery"
	"gopkg.in/macaroon-bakery.v2-unstable/bakery/checkers"
	"gopkg.in/macaroon-bakery.v2-unstable/bakery/mgostorage"
	"gopkg.in/macaroon-bakery.v2-unstable/httpbakery"
	"gopkg.in/macaroon.v2-unstable"

	"github.com/CanonicalLtd/jimm/internal/auth"
	"github.com/CanonicalLtd/jimm/internal/jemtest"
	"github.com/CanonicalLtd/jimm/internal/mgosession"
	"github.com/CanonicalLtd/jimm/params"
)

type authSuite struct {
	jemtest.IsolatedMgoSuite
	idmSrv      *idmtest.Server
	pool        *auth.Pool
	sessionPool *mgosession.Pool
}

var _ = gc.Suite(&authSuite{})

func (s *authSuite) SetUpTest(c *gc.C) {
	s.IsolatedMgoSuite.SetUpTest(c)
	s.idmSrv = idmtest.NewServer()
	db := s.Session.DB("auth")
	bakery, err := bakery.NewService(bakery.NewServiceParams{
		Location: "here",
		Locator:  s.idmSrv,
	})
	c.Assert(err, gc.Equals, nil)
	s.sessionPool = mgosession.NewPool(context.TODO(), s.Session, 5)
	idmClient, err := idmclient.New(idmclient.NewParams{
		BaseURL: s.idmSrv.URL.String(),
		Client:  s.idmSrv.Client("test-user"),
	})
	c.Assert(err, gc.Equals, nil)
	s.pool, err = auth.NewPool(context.TODO(), auth.Params{
		Bakery:   bakery,
		RootKeys: mgostorage.NewRootKeys(100),
		RootKeysPolicy: mgostorage.Policy{
			ExpiryDuration: 1 * time.Second,
		},
		MacaroonCollection: db.C("macaroons"),
		SessionPool:        s.sessionPool,
		PermChecker:        idmclient.NewPermChecker(idmClient, time.Second),
		IdentityLocation:   s.idmSrv.URL.String(),
	})
	c.Assert(err, gc.Equals, nil)
}

func (s *authSuite) TearDownTest(c *gc.C) {
	s.sessionPool.Close()
	s.IsolatedMgoSuite.TearDownTest(c)
}

func (s *authSuite) TestAuthenticateNoMacaroon(c *gc.C) {
	a := s.pool.Authenticator(context.TODO())
	defer a.Close()
	ctx := context.Background()
	ctx2, m, err := a.Authenticate(ctx, nil, checkers.New())
	c.Assert(ctx, jc.DeepEquals, ctx2)
	c.Assert(err, gc.ErrorMatches, `verification failed: no macaroons`)
	c.Assert(m, gc.Not(gc.IsNil))
}

func (s *authSuite) TestAuthenticate(c *gc.C) {
	a := s.pool.Authenticator(context.TODO())
	defer a.Close()
	ctx := context.Background()
	_, m, _ := a.Authenticate(ctx, nil, checkers.New())
	ms := s.discharge(c, m, "bob")
	ctx2, m, err := a.Authenticate(ctx, []macaroon.Slice{ms}, checkers.New())
	c.Assert(err, gc.Equals, nil)
	c.Assert(m, gc.IsNil)
	err = auth.CheckIsUser(ctx, "bob")
	c.Assert(err, gc.ErrorMatches, `unauthorized`)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrUnauthorized)
	err = auth.CheckIsUser(ctx2, "bob")
	c.Assert(err, gc.Equals, nil)
}

func (s *authSuite) TestAuthenticateRequest(c *gc.C) {
	a := s.pool.Authenticator(context.TODO())
	defer a.Close()
	ctx := context.Background()
	req, err := http.NewRequest("GET", "/", nil)
	req.RequestURI = "/"
	c.Assert(err, gc.Equals, nil)
	ctx2, err := a.AuthenticateRequest(ctx, req)
	c.Assert(ctx2, gc.Equals, ctx)
	c.Assert(err, gc.ErrorMatches, `verification failed: no macaroons`)
	herr, ok := err.(*httpbakery.Error)
	c.Assert(ok, gc.Equals, true)
	ms := s.discharge(c, herr.Info.Macaroon, "bob")
	cookie, err := httpbakery.NewCookie(ms)
	c.Assert(err, gc.Equals, nil)
	req.AddCookie(cookie)
	ctx3, err := a.AuthenticateRequest(ctx, req)
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
	c.Assert(auth.Username(context.Background()), gc.Equals, "")
	a := s.pool.Authenticator(context.TODO())
	defer a.Close()
	_, m, _ := a.Authenticate(nil, nil, checkers.New())
	ms := s.discharge(c, m, "bob")
	ctx, _, err := a.Authenticate(context.Background(), []macaroon.Slice{ms}, checkers.New())
	c.Assert(err, gc.Equals, nil)
	c.Assert(auth.Username(ctx), gc.Equals, "bob")
}

func (s *authSuite) discharge(c *gc.C, m *macaroon.Macaroon, username string, groups ...string) macaroon.Slice {
	s.idmSrv.AddUser(username, groups...)
	s.idmSrv.SetDefaultUser(username)
	cl := s.idmSrv.Client(username)
	ms, err := cl.DischargeAll(m)
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
