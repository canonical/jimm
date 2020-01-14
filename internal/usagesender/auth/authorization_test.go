// Copyright 2018 Canonical Ltd.

package auth_test

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/julienschmidt/httprouter"
	gc "gopkg.in/check.v1"
	"gopkg.in/httprequest.v1"
	"gopkg.in/macaroon-bakery.v2/httpbakery"

	"github.com/CanonicalLtd/jimm/internal/jemerror"
	"github.com/CanonicalLtd/jimm/internal/usagesender/auth"
)

func Test(t *testing.T) { gc.TestingT(t) }

var _ = gc.Suite(&authorizationSuite{})

type authorizationSuite struct {
	handler *testAuthHandler
	server  *httptest.Server
}

func (s *authorizationSuite) SetUpTest(c *gc.C) {

	s.handler = &testAuthHandler{}
	router := httprouter.New()
	srv := httprequest.Server{
		ErrorMapper: jemerror.Mapper,
	}
	handlers := srv.Handlers(func(p httprequest.Params) (*testAuthHandler, context.Context, error) {
		return s.handler, p.Context, nil
	})
	for _, h := range handlers {
		router.Handle(h.Method, h.Path, h.Handle)
	}
	s.server = httptest.NewServer(router)
}

func (s *authorizationSuite) TestGetCredentials(c *gc.C) {
	hclient := httpbakery.NewClient()
	client := auth.NewAuthorizationClient(s.server.URL, hclient)
	creds, err := client.GetCredentials(context.Background(), "someuser")
	c.Assert(err, gc.Equals, nil)
	c.Assert(s.handler.receivedRequest.Tags["user"], gc.Equals, "someuser")
	c.Assert(creds, gc.DeepEquals, []byte("secret stuff"))
}

type getCredentialsRequest struct {
	httprequest.Route `httprequest:"POST /v4/jimm/authorization"`
	Body              credentialsRequest `httprequest:",body"`
}

type credentialsRequest struct {
	Tags map[string]string `json:"tags"`
}

type credentialsResponse struct {
	Credentials []byte `json:"credentials"`
}

type testAuthHandler struct {
	receivedRequest *credentialsRequest
}

// Authorization is a mock implementation of the API endpoint that issues usage sender authorizations.
func (c *testAuthHandler) Authorization(arg *getCredentialsRequest) (*credentialsResponse, error) {
	c.receivedRequest = &arg.Body

	return &credentialsResponse{Credentials: []byte("secret stuff")}, nil
}
