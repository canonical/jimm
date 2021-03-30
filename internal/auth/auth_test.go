// Copyright 2016 Canonical Ltd.

package auth_test

import (
	"context"
	"net/http"
	"time"

	"github.com/canonical/candid/candidclient"
	"github.com/canonical/candid/candidtest"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery/mgorootkeystore"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/errgo.v1"
	"gopkg.in/macaroon-bakery.v2/bakery"
	"gopkg.in/macaroon-bakery.v2/bakery/checkers"
	"gopkg.in/macaroon-bakery.v2/bakery/identchecker"
	"gopkg.in/macaroon-bakery.v2/httpbakery"
	"gopkg.in/macaroon.v2"

	"github.com/CanonicalLtd/jimm/internal/auth"
	"github.com/CanonicalLtd/jimm/internal/jemtest"
	"github.com/CanonicalLtd/jimm/params"
)

type authSuite struct {
	jemtest.IsolatedMgoSuite
	idmSrv        *candidtest.Server
	authenticator *auth.Authenticator
}

var _ = gc.Suite(&authSuite{})

func (s *authSuite) SetUpTest(c *gc.C) {
	s.IsolatedMgoSuite.SetUpTest(c)
	s.idmSrv = candidtest.NewServer()

	db := s.Session.DB("auth")

	rks := mgorootkeystore.NewRootKeys(100)
	err := rks.EnsureIndex(db.C("macaroons"))
	c.Assert(err, jc.ErrorIsNil)

	key, err := bakery.GenerateKey()
	c.Assert(err, gc.Equals, nil)

	idmClient, err := candidclient.New(candidclient.NewParams{
		BaseURL: s.idmSrv.URL.String(),
		Client:  s.idmSrv.Client("test-user"),
	})
	c.Assert(err, gc.Equals, nil)

	s.authenticator = auth.NewAuthenticator(identchecker.NewBakery(identchecker.BakeryParams{
		RootKeyStore: rks.NewStore(
			db.C("macaroons"),
			mgorootkeystore.Policy{
				ExpiryDuration: 1 * time.Second,
			},
		),
		Locator: s.idmSrv,
		Key:     key,
		IdentityClient: auth.NewIdentityClient(auth.IdentityClientParams{
			CandidClient: idmClient,
		}),
		Authorizer: identchecker.AuthorizerFunc(func(ctx context.Context, id identchecker.Identity, op bakery.Op) (bool, []checkers.Caveat, error) {
			return id != nil && op == identchecker.LoginOp, nil, nil
		}),
		Location: "here",
	}))
}

func (s *authSuite) TestAuthenticateNoMacaroon(c *gc.C) {
	ctx := context.Background()
	id, m, err := s.authenticator.Authenticate(ctx, bakery.Version3, nil)
	c.Assert(err, gc.ErrorMatches, `macaroon discharge required: authentication required`)
	c.Assert(m, gc.Not(gc.IsNil))
	c.Assert(id, gc.IsNil)
}

func (s *authSuite) TestAuthenticate(c *gc.C) {
	ctx := context.Background()
	_, m, _ := s.authenticator.Authenticate(ctx, bakery.Version3, nil)
	ms := s.discharge(ctx, c, m, "bob")
	id, m, err := s.authenticator.Authenticate(ctx, bakery.Version3, []macaroon.Slice{ms})
	c.Assert(err, gc.Equals, nil)
	c.Assert(m, gc.IsNil)
	err = auth.CheckIsUser(ctx, id, "alice")
	c.Assert(err, gc.ErrorMatches, `unauthorized`)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrUnauthorized)
	err = auth.CheckIsUser(ctx, id, "bob")
	c.Assert(err, gc.Equals, nil)
}

func (s *authSuite) TestAuthenticateWithContextSession(c *gc.C) {
	session := s.Session.Copy()
	defer session.Close()

	ctx := mgorootkeystore.ContextWithMgoSession(context.Background(), session)
	_, m, _ := s.authenticator.Authenticate(ctx, bakery.Version3, nil)
	ms := s.discharge(ctx, c, m, "bob")
	id, m, err := s.authenticator.Authenticate(ctx, bakery.Version3, []macaroon.Slice{ms})
	c.Assert(err, gc.Equals, nil)
	c.Assert(m, gc.IsNil)
	err = auth.CheckIsUser(ctx, id, "alice")
	c.Assert(err, gc.ErrorMatches, `unauthorized`)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrUnauthorized)
	err = auth.CheckIsUser(ctx, id, "bob")
	c.Assert(err, gc.Equals, nil)
}

func (s *authSuite) TestAuthenticateRequest(c *gc.C) {
	ctx := context.Background()
	req, err := http.NewRequest("GET", "/", nil)
	req.RequestURI = "/"
	c.Assert(err, gc.Equals, nil)
	id, err := s.authenticator.AuthenticateRequest(ctx, req)
	c.Assert(id, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, `macaroon discharge required: authentication required`)
	herr, ok := err.(*httpbakery.Error)
	c.Assert(ok, gc.Equals, true)
	ms := s.discharge(ctx, c, herr.Info.Macaroon, "bob")
	cookie, err := httpbakery.NewCookie(nil, ms)
	c.Assert(err, gc.Equals, nil)
	req.AddCookie(cookie)
	id, err = s.authenticator.AuthenticateRequest(ctx, req)
	c.Assert(err, gc.Equals, nil)
	err = auth.CheckIsUser(ctx, id, "alice")
	c.Assert(err, gc.ErrorMatches, `unauthorized`)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrUnauthorized)
	err = auth.CheckIsUser(ctx, id, "bob")
	c.Assert(err, gc.Equals, nil)
}

func (s *authSuite) TestCheckIsUser(c *gc.C) {
	ctx := context.Background()
	id := jemtest.NewIdentity("bob")
	err := auth.CheckIsUser(ctx, id, "bob")
	c.Assert(err, gc.Equals, nil)
	err = auth.CheckIsUser(ctx, id, "alice")
	c.Assert(err, gc.ErrorMatches, `unauthorized`)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrUnauthorized)
}

func (s *authSuite) TestCheckACL(c *gc.C) {
	ctx := context.Background()
	id := jemtest.NewIdentity("bob")
	err := auth.CheckACL(ctx, id, []string{"bob", "charlie"})
	c.Assert(err, gc.Equals, nil)
	err = auth.CheckACL(ctx, id, []string{"alice", "charlie"})
	c.Assert(err, gc.ErrorMatches, `unauthorized`)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrUnauthorized)
	err = auth.CheckACL(ctx, id, []string{"alice", "charlie", "everyone"})
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
	ctx := context.Background()
	id := jemtest.NewIdentity("bob", "bob-group")
	for i, test := range canReadTests {
		c.Logf("%d. %q %#v", i, test.owner, test.readers)
		err := auth.CheckCanRead(ctx, id, testEntity{
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
