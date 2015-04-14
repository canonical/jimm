package v1_test

import (
	gc "gopkg.in/check.v1"
	"net/http"

	"github.com/CanonicalLtd/jem/internal/jem"
	"github.com/CanonicalLtd/jem/internal/v1"
	"github.com/CanonicalLtd/jem/params"
	jujutesting "github.com/juju/testing"
	"github.com/juju/testing/httptesting"
	"gopkg.in/macaroon-bakery.v0/bakery"
	"gopkg.in/mgo.v2"
)

type APISuite struct {
	jujutesting.IsolatedMgoSuite
	srv http.Handler
	jem *jem.JEM
}

var _ = gc.Suite(&APISuite{})

func (s *APISuite) SetUpTest(c *gc.C) {
	s.IsolatedMgoSuite.SetUpTest(c)
	s.srv, s.jem = newServer(c, s.Session)
}

func newServer(c *gc.C, session *mgo.Session) (http.Handler, *jem.JEM) {
	db := session.DB("jem")
	j, err := jem.New(db, &bakery.NewServiceParams{})
	c.Assert(err, gc.IsNil)
	config := jem.ServerParams{
		DB: db,
	}
	srv, err := jem.NewServer(config, map[string]jem.NewAPIHandlerFunc{"v1": v1.NewAPIHandler})
	c.Assert(err, gc.IsNil)
	return srv, j
}

func (s *APISuite) TestTest(c *gc.C) {
	httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
		Handler:      s.srv,
		URL:          "/v1/test",
		ExpectStatus: http.StatusUnauthorized,
		ExpectBody: &params.Error{
			Message: "go away: testing",
			Code:    params.ErrUnauthorized,
		},
	})
}
