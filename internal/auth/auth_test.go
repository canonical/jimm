// Copyright 2016 Canonical Ltd.

package auth_test

import (
	"net/http"
	"time"

	"github.com/juju/idmclient"
	"github.com/juju/idmclient/idmtest"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"golang.org/x/net/context"
	gc "gopkg.in/check.v1"
	"gopkg.in/errgo.v1"
	"gopkg.in/macaroon-bakery.v1/bakery"
	"gopkg.in/macaroon-bakery.v1/bakery/checkers"
	"gopkg.in/macaroon-bakery.v1/bakery/mgostorage"
	"gopkg.in/macaroon-bakery.v1/httpbakery"
	"gopkg.in/macaroon.v1"

	"github.com/CanonicalLtd/jem/internal/auth"
	"github.com/CanonicalLtd/jem/params"
)

type authSuite struct {
	jujutesting.IsolatedMgoSuite
	idmSrv *idmtest.Server
	pool   *auth.Pool
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
	c.Assert(err, jc.ErrorIsNil)
	s.pool = auth.NewPool(auth.Params{
		Bakery:   bakery,
		RootKeys: mgostorage.NewRootKeys(100),
		RootKeysPolicy: mgostorage.Policy{
			ExpiryDuration: 1 * time.Second,
		},
		MacaroonCollection:  db.C("macaroons"),
		MaxCollectionClones: 1000,
		MaxCollectionAge:    time.Minute,
		PermChecker: idmclient.NewPermChecker(
			idmclient.New(idmclient.NewParams{
				BaseURL: s.idmSrv.URL.String(),
				Client:  s.idmSrv.Client("test-user"),
			}),
			time.Second,
		),
		IdentityLocation: s.idmSrv.URL.String(),
	})
}

func (s *authSuite) TearDownTest(c *gc.C) {
	s.pool.Close()
	s.IsolatedMgoSuite.TearDownTest(c)
}

func (s authSuite) TestGet(c *gc.C) {
	a1 := s.pool.Get()
	c.Assert(a1, gc.Not(gc.IsNil))
	defer s.pool.Put(a1)
	a2 := s.pool.Get()
	c.Assert(a2, gc.Not(gc.IsNil))
	defer s.pool.Put(a2)
	c.Assert(a1, gc.DeepEquals, a2)
}

func (s authSuite) TestGetChangeAfterNumberOfInstances(c *gc.C) {
	session := s.Session.Clone()
	defer session.Close()
	bakery, err := bakery.NewService(bakery.NewServiceParams{
		Location: "here",
		Locator:  s.idmSrv,
	})
	c.Assert(err, jc.ErrorIsNil)
	pool := auth.NewPool(auth.Params{
		Bakery:   bakery,
		RootKeys: mgostorage.NewRootKeys(100),
		RootKeysPolicy: mgostorage.Policy{
			ExpiryDuration: 1 * time.Second,
		},
		MacaroonCollection:  session.DB("auth").C("macaroons"),
		MaxCollectionClones: 1,
		MaxCollectionAge:    time.Minute,
		PermChecker: idmclient.NewPermChecker(
			idmclient.New(idmclient.NewParams{
				BaseURL: s.idmSrv.URL.String(),
				Client:  s.idmSrv.Client("test-user"),
			}),
			time.Second,
		),
		IdentityLocation: s.idmSrv.URL.String(),
	})
	defer pool.Close()
	a1 := pool.Get()
	c.Assert(a1, gc.Not(gc.IsNil))
	defer pool.Put(a1)
	a2 := pool.Get()
	c.Assert(a2, gc.Not(gc.IsNil))
	defer pool.Put(a2)
	c.Assert(a1, gc.Not(gc.DeepEquals), a2)
}

func (s authSuite) TestGetChangeAfterTime(c *gc.C) {
	session := s.Session.Clone()
	defer session.Close()
	bakery, err := bakery.NewService(bakery.NewServiceParams{
		Location: "here",
		Locator:  s.idmSrv,
	})
	c.Assert(err, jc.ErrorIsNil)
	pool := auth.NewPool(auth.Params{
		Bakery:   bakery,
		RootKeys: mgostorage.NewRootKeys(100),
		RootKeysPolicy: mgostorage.Policy{
			ExpiryDuration: 1 * time.Second,
		},
		MacaroonCollection:  session.DB("auth").C("macaroons"),
		MaxCollectionClones: 1,
		MaxCollectionAge:    time.Nanosecond,
		PermChecker: idmclient.NewPermChecker(
			idmclient.New(idmclient.NewParams{
				BaseURL: s.idmSrv.URL.String(),
				Client:  s.idmSrv.Client("test-user"),
			}),
			time.Second,
		),
		IdentityLocation: s.idmSrv.URL.String(),
	})
	defer pool.Close()
	a1 := pool.Get()
	c.Assert(a1, gc.Not(gc.IsNil))
	defer pool.Put(a1)
	a2 := pool.Get()
	c.Assert(a2, gc.Not(gc.IsNil))
	defer pool.Put(a2)
	c.Assert(a1, gc.Not(gc.DeepEquals), a2)
}

func (s *authSuite) TestAuthenticateNoMacaroon(c *gc.C) {
	a := s.pool.Get()
	defer s.pool.Put(a)
	ctx := context.Background()
	ctx2, m, err := a.Authenticate(ctx, nil, checkers.New())
	c.Assert(ctx, jc.DeepEquals, ctx2)
	c.Assert(err, gc.ErrorMatches, `verification failed: no macaroons`)
	c.Assert(m, gc.Not(gc.IsNil))
}

func (s *authSuite) TestAuthenticate(c *gc.C) {
	a := s.pool.Get()
	defer s.pool.Put(a)
	ctx := context.Background()
	_, m, _ := a.Authenticate(ctx, nil, checkers.New())
	ms := s.discharge(c, m, "bob")
	ctx2, m, err := a.Authenticate(ctx, []macaroon.Slice{ms}, checkers.New())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m, gc.IsNil)
	err = auth.CheckIsUser(ctx, "bob")
	c.Assert(err, gc.ErrorMatches, `unauthorized`)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrUnauthorized)
	err = auth.CheckIsUser(ctx2, "bob")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *authSuite) TestAuthenticateRequest(c *gc.C) {
	a := s.pool.Get()
	defer s.pool.Put(a)
	ctx := context.Background()
	req, err := http.NewRequest("GET", "/", nil)
	req.RequestURI = "/"
	c.Assert(err, jc.ErrorIsNil)
	ctx2, err := a.AuthenticateRequest(ctx, req)
	c.Assert(ctx2, gc.Equals, ctx)
	c.Assert(err, gc.ErrorMatches, `verification failed: no macaroons`)
	herr, ok := err.(*httpbakery.Error)
	c.Assert(ok, gc.Equals, true)
	ms := s.discharge(c, herr.Info.Macaroon, "bob")
	cookie, err := httpbakery.NewCookie(ms)
	c.Assert(err, jc.ErrorIsNil)
	req.AddCookie(cookie)
	ctx3, err := a.AuthenticateRequest(ctx, req)
	c.Assert(err, jc.ErrorIsNil)
	err = auth.CheckIsUser(ctx2, "bob")
	c.Assert(err, gc.ErrorMatches, `unauthorized`)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrUnauthorized)
	err = auth.CheckIsUser(ctx3, "bob")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *authSuite) TestCheckIsUser(c *gc.C) {
	ctx := auth.AuthenticateForTest(context.Background(), "bob")
	err := auth.CheckIsUser(ctx, "bob")
	c.Assert(err, jc.ErrorIsNil)
	err = auth.CheckIsUser(ctx, "alice")
	c.Assert(err, gc.ErrorMatches, `unauthorized`)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrUnauthorized)
}

func (s *authSuite) TestCheckACL(c *gc.C) {
	ctx := auth.AuthenticateForTest(context.Background(), "bob")
	err := auth.CheckACL(ctx, []string{"bob", "charlie"})
	c.Assert(err, jc.ErrorIsNil)
	err = auth.CheckACL(ctx, []string{"alice", "charlie"})
	c.Assert(err, gc.ErrorMatches, `unauthorized`)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrUnauthorized)
	err = auth.CheckACL(ctx, []string{"alice", "charlie", "everyone"})
	c.Assert(err, jc.ErrorIsNil)
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
	ctx := auth.AuthenticateForTest(context.Background(), "bob", "bob-group")
	for i, test := range canReadTests {
		c.Logf("%d. %q %#v", i, test.owner, test.readers)
		err := auth.CheckCanRead(ctx, testEntity{
			owner:   test.owner,
			readers: test.readers,
		})
		if test.allowed {
			c.Assert(err, jc.ErrorIsNil)
			continue
		}
		c.Assert(err, gc.ErrorMatches, `unauthorized`)
		c.Assert(errgo.Cause(err), gc.Equals, params.ErrUnauthorized)
	}
}

func (s *authSuite) TestUsername(c *gc.C) {
	c.Assert(auth.Username(context.Background()), gc.Equals, "")
	a := s.pool.Get()
	defer s.pool.Put(a)
	_, m, _ := a.Authenticate(nil, nil, checkers.New())
	ms := s.discharge(c, m, "bob")
	ctx, _, err := a.Authenticate(context.Background(), []macaroon.Slice{ms}, checkers.New())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(auth.Username(ctx), gc.Equals, "bob")
}

func (s *authSuite) discharge(c *gc.C, m *macaroon.Macaroon, username string, groups ...string) macaroon.Slice {
	s.idmSrv.AddUser(username, groups...)
	s.idmSrv.SetDefaultUser(username)
	cl := s.idmSrv.Client(username)
	ms, err := cl.DischargeAll(m)
	c.Assert(err, jc.ErrorIsNil)
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
