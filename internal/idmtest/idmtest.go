package idmtest

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"

	"github.com/juju/httprequest"
	"github.com/julienschmidt/httprouter"
	"gopkg.in/errgo.v1"
	"gopkg.in/macaroon-bakery.v1/bakery"
	"gopkg.in/macaroon-bakery.v1/bakery/checkers"
	"gopkg.in/macaroon-bakery.v1/httpbakery"
	"gopkg.in/macaroon-bakery.v1/httpbakery/agent"
)

// Server represents a mock identity server.
// It currently serves only the discharge and groups endpoints.
type Server struct {
	// URL holds the URL of the mock identity server.
	// The discharger endpoint is located at URL/v1/discharge.
	URL *url.URL

	// PublicKey holds the public key of the mock identity server.
	PublicKey *bakery.PublicKey

	router *httprouter.Router
	srv    *httptest.Server

	// mu guards the fields below it.
	mu          sync.Mutex
	users       map[string]*user
	defaultUser string
}

type user struct {
	groups []string
	key    *bakery.KeyPair
}

// NewServer runs a mock identity server. It can discharge
// macaroons and return information on user group membership.
// The returned server should be closed after use.
func NewServer() *Server {
	bsrv, err := bakery.NewService(bakery.NewServiceParams{})
	if err != nil {
		panic(err)
	}
	srv := &Server{
		users:     make(map[string]*user),
		PublicKey: bsrv.PublicKey(),
	}
	errorMapper := httprequest.ErrorMapper(httpbakery.ErrorToResponse)
	h := &handler{
		srv: srv,
	}
	router := httprouter.New()
	for _, route := range errorMapper.Handlers(func(httprequest.Params) (*handler, error) {
		return h, nil
	}) {
		router.Handle(route.Method, route.Path, route.Handle)
	}
	mux := http.NewServeMux()
	httpbakery.AddDischargeHandler(mux, "/v1/discharge", bsrv, srv.check)
	router.Handler("POST", "/v1/discharge/*rest", mux)
	router.Handler("GET", "/v1/discharge/*rest", mux)

	srv.srv = httptest.NewServer(router)
	srv.URL, err = url.Parse(srv.srv.URL)
	if err != nil {
		panic(err)
	}
	return srv
}

// Close shuts down the server.
func (srv *Server) Close() {
	srv.srv.Close()
}

// PublicKeyForLocation implements bakery.PublicKeyLocator
// by returning the server's public key for all locations.
func (srv *Server) PublicKeyForLocation(loc string) (*bakery.PublicKey, error) {
	return srv.PublicKey, nil
}

// Client returns a bakery client that will discharge as the given user.
// It panics if the user has not been added.
func (srv *Server) Client(username string) *httpbakery.Client {
	c := httpbakery.NewClient()
	u := srv.user(username)
	if u == nil {
		panic(errgo.Newf("unknown user %q", username))
	}
	c.Key = u.key
	agent.SetUpAuth(c, srv.URL, username)
	return c
}

// SetDefaultUser configures the server so that it will discharge for
// the given user if no agent-login cookie is found. The user does not
// need to have been added.
//
// If the name is empty, there will be no default user.
func (srv *Server) SetDefaultUser(name string) {
	srv.mu.Lock()
	srv.defaultUser = name
	srv.mu.Unlock()
}

// AddUser adds a new user that's in the given set of groups.
func (srv *Server) AddUser(name string, groups ...string) {
	key, err := bakery.GenerateKey()
	if err != nil {
		panic(err)
	}
	srv.mu.Lock()
	defer srv.mu.Unlock()
	srv.users[name] = &user{
		groups: groups,
		key:    key,
	}
}

func (srv *Server) user(name string) *user {
	srv.mu.Lock()
	defer srv.mu.Unlock()
	return srv.users[name]
}

func (srv *Server) check(req *http.Request, cavId, cav string) ([]checkers.Caveat, error) {
	if cav != "is-authenticated-user" {
		return nil, errgo.Newf("unknown third party caveat %q", cav)
	}
	username, key, err := agent.LoginCookie(req)
	if errgo.Cause(err) == agent.ErrNoAgentLoginCookie {
		srv.mu.Lock()
		defer srv.mu.Unlock()
		if srv.defaultUser == "" {
			return nil, errgo.Newf("no default discharge user")
		}
		return []checkers.Caveat{
			checkers.DeclaredCaveat("username", srv.defaultUser),
		}, nil
	}
	if err != nil {
		return nil, errgo.Notef(err, "cannot find login cookie: %v")
	}
	u := srv.user(username)
	if u == nil {
		return nil, errgo.Newf("user not found")
	}
	if *key != u.key.Public {
		return nil, errgo.Newf("pubkey mismatch")
	}
	return []checkers.Caveat{
		checkers.DeclaredCaveat("username", username),
		bakery.LocalThirdPartyCaveat(key),
	}, nil
}

type handler struct {
	srv *Server
}

type groupsRequest struct {
	httprequest.Route `httprequest:"GET /v1/u/:User/groups"`
	User              string `httprequest:",path"`
}

func (h *handler) GetGroups(req *groupsRequest) ([]string, error) {
	if u := h.srv.user(req.User); u != nil {
		return u.groups, nil
	}
	return nil, fmt.Errorf("user %q not found", req.User)
}
