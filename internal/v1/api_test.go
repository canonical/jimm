package v1_test

import (
	"encoding/json"
	"net/http"
	"strings"

	gc "gopkg.in/check.v1"
	"gopkg.in/errgo.v1"

	"github.com/CanonicalLtd/jem/internal/jem"
	"github.com/CanonicalLtd/jem/internal/v1"
	"github.com/CanonicalLtd/jem/params"
	jujutesting "github.com/juju/testing"
	"github.com/juju/testing/httptesting"
	"gopkg.in/macaroon-bakery.v0/bakery"
	"gopkg.in/macaroon-bakery.v0/bakery/checkers"
	"gopkg.in/macaroon-bakery.v0/bakerytest"
	"gopkg.in/macaroon-bakery.v0/httpbakery"
	"gopkg.in/mgo.v2"
)

type APISuite struct {
	jujutesting.IsolatedMgoSuite
	srv        http.Handler
	jem        *jem.Pool
	discharger *bakerytest.Discharger
	username   string
	groups     []string
	client     *httpbakery.Client
}

var _ = gc.Suite(&APISuite{})

func (s *APISuite) SetUpTest(c *gc.C) {
	s.IsolatedMgoSuite.SetUpTest(c)
	s.srv, s.jem, s.discharger = s.newServer(c, s.Session)
	s.username = "testuser"
	s.client = httpbakery.NewClient()
}

func (s *APISuite) TearDownTest(c *gc.C) {
	s.discharger.Close()
	s.IsolatedMgoSuite.TearDownTest(c)
}

func (s *APISuite) newServer(c *gc.C, session *mgo.Session) (http.Handler, *jem.Pool, *bakerytest.Discharger) {
	discharger := bakerytest.NewDischarger(nil, func(_ *http.Request, cond string, arg string) ([]checkers.Caveat, error) {
		if s.username == "" {
			return nil, errgo.Newf("no specified username for discharge macaroon")
		}
		return []checkers.Caveat{
			checkers.DeclaredCaveat(v1.UsernameAttr, s.username),
			checkers.DeclaredCaveat(v1.GroupsAttr, strings.Join(s.groups, " ")),
		}, nil
	})
	db := session.DB("jem")
	j, err := jem.NewPool(db, &bakery.NewServiceParams{})
	c.Assert(err, gc.IsNil)
	config := jem.ServerParams{
		DB:               db,
		IdentityLocation: discharger.Location(),
		PublicKeyLocator: discharger,
	}
	srv, err := jem.NewServer(config, map[string]jem.NewAPIHandlerFunc{"v1": v1.NewAPIHandler})
	c.Assert(err, gc.IsNil)
	return srv, j, discharger
}

func (s *APISuite) TestTest(c *gc.C) {
	httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
		Handler:      s.srv,
		URL:          "/v1/test",
		Do:           s.client.Do,
		ExpectStatus: http.StatusUnauthorized,
		ExpectBody: &params.Error{
			Message: "go away: testing",
			Code:    params.ErrUnauthorized,
		},
	})
}

func (s *APISuite) TestTestUnauthorized(c *gc.C) {
	httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
		Handler: s.srv,
		URL:     "/v1/test",
		ExpectBody: httptesting.BodyAsserter(func(c *gc.C, m json.RawMessage) {
			// Allow any body - the next check will check that it's a valid macaroon.
		}),
		ExpectStatus: http.StatusProxyAuthRequired,
	})
}
