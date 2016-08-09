// Copyright 2015 Canonical Ltd.

package jem_test

import (
	"fmt"
	"net/http"

	"github.com/juju/idmclient"
	"github.com/juju/idmclient/idmtest"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/errgo.v1"
	"gopkg.in/macaroon-bakery.v1/bakery"
	"gopkg.in/macaroon-bakery.v1/httpbakery"

	"github.com/CanonicalLtd/jem/internal/jem"
	"github.com/CanonicalLtd/jem/internal/mongodoc"
	"github.com/CanonicalLtd/jem/params"
)

type authSuite struct {
	jujutesting.IsolatedMgoSuite
	idmSrv *idmtest.Server
	pool   *jem.Pool
	jem    *jem.JEM
}

var _ = gc.Suite(&authSuite{})

func (s *authSuite) SetUpTest(c *gc.C) {
	s.IsolatedMgoSuite.SetUpTest(c)
	s.idmSrv = idmtest.NewServer()
	pool, err := jem.NewPool(jem.Params{
		DB:               s.Session.DB("jem"),
		IdentityLocation: s.idmSrv.URL.String(),
		ControllerAdmin:  "controller-admin",
		BakeryParams: bakery.NewServiceParams{
			Location: "here",
			Locator:  s.idmSrv,
		},
		IDMClient: idmclient.New(idmclient.NewParams{
			BaseURL: s.idmSrv.URL.String(),
			Client:  s.idmSrv.Client("agent"),
		}),
	})
	c.Assert(err, gc.IsNil)
	s.pool = pool
	s.jem = s.pool.JEM()
}

func (s *authSuite) TearDownTest(c *gc.C) {
	s.jem.Close()
	s.pool.Close()
	s.IsolatedMgoSuite.TearDownTest(c)
}

func (s *authSuite) TestNewMacaroon(c *gc.C) {
	m, err := s.jem.NewMacaroon()
	c.Assert(err, gc.IsNil)
	c.Assert(m.Location(), gc.Equals, "here")
	c.Assert(m.Id(), gc.Not(gc.Equals), "")
	cavs := m.Caveats()
	c.Assert(cavs, gc.HasLen, 1)
	cav := cavs[0]
	c.Assert(cav.Location, gc.Equals, s.idmSrv.URL.String())
}

func (s *authSuite) TestAuthenticateNoMacaroon(c *gc.C) {
	req, err := http.NewRequest("GET", "/", nil)
	c.Assert(err, gc.IsNil)
	req.RequestURI = "/foo/bar"
	err = s.jem.Authenticate(req)
	c.Assert(err, gc.NotNil)
	berr, ok := err.(*httpbakery.Error)
	c.Assert(ok, gc.Equals, true, gc.Commentf("expected %T, got %T", berr, err))
	c.Assert(berr.Code, gc.Equals, httpbakery.ErrDischargeRequired)
	c.Assert(berr.Info, gc.NotNil)
	c.Assert(berr.Info.Macaroon, gc.NotNil)
	c.Assert(berr.Info.MacaroonPath, gc.Equals, "../")
}

func (s *authSuite) TestAuthenticate(c *gc.C) {
	req := s.newRequestForUser(c, "GET", "/", "bob")
	err := s.jem.Authenticate(req)
	c.Assert(err, gc.IsNil)
	c.Assert(s.jem.Auth.Username, gc.Equals, "bob")
}

func (s *authSuite) TestCheckIsAdmin(c *gc.C) {
	req := s.newRequestForUser(c, "GET", "/", "controller-admin")
	err := s.jem.Authenticate(req)
	c.Assert(err, gc.IsNil)
	c.Assert(s.jem.CheckIsAdmin(), gc.IsNil)
	req = s.newRequestForUser(c, "GET", "/", "bob")
	err = s.jem.Authenticate(req)
	c.Assert(err, gc.IsNil)
	c.Assert(s.jem.CheckIsAdmin(), gc.ErrorMatches, string(params.ErrUnauthorized))
}

func (s *authSuite) TestCheckIsUser(c *gc.C) {
	req := s.newRequestForUser(c, "GET", "/", "bob")
	err := s.jem.Authenticate(req)
	c.Assert(err, gc.IsNil)
	c.Assert(s.jem.CheckIsUser("fred"), gc.ErrorMatches, string(params.ErrUnauthorized))
	c.Assert(s.jem.CheckIsUser("bob"), gc.IsNil)
}

func (s *authSuite) TestCheckACL(c *gc.C) {
	c.Assert(s.jem.CheckACL([]string{"admin"}), gc.ErrorMatches, `cannot check permissions: cannot fetch groups: missing value for path parameter "username"`)
	req := s.newRequestForUser(c, "GET", "/", "bob", "bob-group")
	err := s.jem.Authenticate(req)
	c.Assert(err, gc.IsNil)
	c.Assert(s.jem.CheckACL([]string{}), gc.ErrorMatches, string(params.ErrUnauthorized))
	c.Assert(s.jem.CheckACL([]string{"bob"}), gc.IsNil)
	c.Assert(s.jem.CheckACL([]string{"bob-group"}), gc.IsNil)
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
	req := s.newRequestForUser(c, "GET", "/", "bob", "bob-group")
	err := s.jem.Authenticate(req)
	c.Assert(err, gc.IsNil)
	for i, test := range canReadTests {
		c.Logf("%d. %q %#v", i, test.owner, test.readers)
		err := s.jem.CheckCanRead(testEntity{
			owner:   test.owner,
			readers: test.readers,
		})
		if test.allowed {
			c.Assert(err, gc.IsNil)
			continue
		}
		c.Assert(err, gc.ErrorMatches, string(params.ErrUnauthorized))
	}
}

var checkReadACLTests = []struct {
	about            string
	owner            string
	acl              []string
	user             string
	groups           []string
	skipCreateEntity bool
	expectError      string
	expectCause      error
}{{
	about: "user is owner",
	owner: "bob",
	user:  "bob",
}, {
	about:  "owner is user group",
	owner:  "bobgroup",
	user:   "bob",
	groups: []string{"bobgroup"},
}, {
	about: "acl contains user",
	owner: "fred",
	acl:   []string{"bob"},
	user:  "bob",
}, {
	about:  "acl contains user's group",
	owner:  "fred",
	acl:    []string{"bobgroup"},
	user:   "bob",
	groups: []string{"bobgroup"},
}, {
	about:       "user not in acl",
	owner:       "fred",
	acl:         []string{"fredgroup"},
	user:        "bob",
	expectError: "unauthorized",
	expectCause: params.ErrUnauthorized,
}, {
	about:            "no entity and not owner",
	owner:            "fred",
	user:             "bob",
	skipCreateEntity: true,
	expectError:      "unauthorized",
	expectCause:      params.ErrUnauthorized,
}}

func (s *authSuite) TestCheckReadACL(c *gc.C) {
	for i, test := range checkReadACLTests {
		c.Logf("%d. %s", i, test.about)
		func() {
			jem := s.pool.JEM()
			defer jem.Close()
			req := s.newRequestForUser(c, "GET", "", test.user, test.groups...)
			err := jem.Authenticate(req)
			c.Assert(err, gc.IsNil)
			entity := params.EntityPath{
				User: params.User(test.owner),
				Name: params.Name(fmt.Sprintf("test%d", i)),
			}
			if !test.skipCreateEntity {
				err := jem.AddModel(&mongodoc.Model{
					Path: entity,
					ACL: params.ACL{
						Read: test.acl,
					},
				})
				c.Assert(err, gc.IsNil)
			}
			err = jem.CheckReadACL(jem.DB.Models(), entity)
			if test.expectError != "" {
				c.Assert(err, gc.ErrorMatches, test.expectError)
				if test.expectCause != nil {
					c.Assert(errgo.Cause(err), gc.Equals, test.expectCause)
				} else {
					c.Assert(errgo.Cause(err), gc.Equals, err)
				}
			} else {
				c.Assert(err, gc.IsNil)
			}
		}()
	}
}

func (s *authSuite) TestCheckGetACL(c *gc.C) {
	m := &mongodoc.Model{
		Path: params.EntityPath{
			User: params.User("bob"),
			Name: "model",
		},
		ACL: params.ACL{
			Read: []string{"fred", "jim"},
		},
	}
	err := s.jem.AddModel(m)
	c.Assert(err, gc.IsNil)
	acl, err := s.jem.GetACL(s.jem.DB.Models(), m.Path)
	c.Assert(err, gc.IsNil)
	c.Assert(acl, jc.DeepEquals, m.ACL)
}

func (s *authSuite) TestCheckGetACLNotFound(c *gc.C) {
	m := &mongodoc.Model{
		Path: params.EntityPath{
			User: params.User("bob"),
			Name: "model",
		},
	}
	acl, err := s.jem.GetACL(s.jem.DB.Models(), m.Path)
	c.Assert(err, gc.ErrorMatches, "not found")
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)
	c.Assert(acl, jc.DeepEquals, m.ACL)
}

func (s *authSuite) TestCanReadIter(c *gc.C) {
	testModels := []mongodoc.Model{{
		Path: params.EntityPath{
			User: params.User("bob"),
			Name: "m1",
		},
	}, {
		Path: params.EntityPath{
			User: params.User("fred"),
			Name: "m2",
		},
	}, {
		Path: params.EntityPath{
			User: params.User("fred"),
			Name: "m3",
		},
		ACL: params.ACL{
			Read: []string{"bob"},
		},
	}}
	for i := range testModels {
		err := s.jem.AddModel(&testModels[i])
		c.Assert(err, gc.IsNil)
	}
	req := s.newRequestForUser(c, "GET", "/", "bob", "bob-group")
	err := s.jem.Authenticate(req)
	c.Assert(err, gc.IsNil)
	it := s.jem.DB.Models().Find(nil).Sort("_id").Iter()
	crit := s.jem.CanReadIter(it)
	var models []mongodoc.Model
	var m mongodoc.Model
	for crit.Next(&m) {
		models = append(models, m)
	}
	c.Assert(crit.Err(), gc.IsNil)
	c.Assert(models, jc.DeepEquals, []mongodoc.Model{
		testModels[0],
		testModels[2],
	})
	c.Assert(crit.Count(), gc.Equals, 3)
}

var doContollerTests = []struct {
	about             string
	cloud             params.Cloud
	region            string
	expectControllers []params.EntityPath
}{{
	about: "no parameters",
	expectControllers: []params.EntityPath{{
		User: "alice",
		Name: "aws-eu-west-1",
	}, {
		User: "alice",
		Name: "aws-us-east-1",
	}, {
		User: "bob",
		Name: "aws-eu-west-1",
	}, {
		User: "bob",
		Name: "aws-us-east-1",
	}, {
		User: "bob",
		Name: "gce-us-east-1",
	}},
}, {
	about: "aws",
	cloud: "aws",
	expectControllers: []params.EntityPath{{
		User: "alice",
		Name: "aws-eu-west-1",
	}, {
		User: "alice",
		Name: "aws-us-east-1",
	}, {
		User: "bob",
		Name: "aws-eu-west-1",
	}, {
		User: "bob",
		Name: "aws-us-east-1",
	}},
}, {
	about:  "aws-us-east-1",
	cloud:  "aws",
	region: "us-east-1",
	expectControllers: []params.EntityPath{{
		User: "alice",
		Name: "aws-us-east-1",
	}, {
		User: "bob",
		Name: "aws-us-east-1",
	}},
}, {
	about:             "aws-us-east-1",
	cloud:             "aws",
	region:            "us-east-2",
	expectControllers: []params.EntityPath{},
}}

func (s *authSuite) TestDoControllers(c *gc.C) {
	testControllers := []mongodoc.Controller{{
		Path: params.EntityPath{
			User: params.User("bob"),
			Name: "aws-us-east-1",
		},
		Cloud: mongodoc.Cloud{
			Name: "aws",
			Regions: []mongodoc.Region{{
				Name: "us-east-1",
			}},
		},
		Public: true,
	}, {
		Path: params.EntityPath{
			User: params.User("bob"),
			Name: "aws-eu-west-1",
		},
		Cloud: mongodoc.Cloud{
			Name: "aws",
			Regions: []mongodoc.Region{{
				Name: "eu-west-1",
			}},
		},
		Public: true,
	}, {
		Path: params.EntityPath{
			User: params.User("alice"),
			Name: "aws-us-east-1",
		},
		ACL: params.ACL{
			Read: []string{"bob-group"},
		},
		Cloud: mongodoc.Cloud{
			Name: "aws",
			Regions: []mongodoc.Region{{
				Name: "us-east-1",
			}},
		},
		Public: true,
	}, {
		Path: params.EntityPath{
			User: params.User("alice"),
			Name: "aws-eu-west-1",
		},
		ACL: params.ACL{
			Read: []string{"bob"},
		},
		Cloud: mongodoc.Cloud{
			Name: "aws",
			Regions: []mongodoc.Region{{
				Name: "eu-west-1",
			}},
		},
		Public: true,
	}, {
		Path: params.EntityPath{
			User: params.User("alice"),
			Name: "aws-us-east-2",
		},
		Cloud: mongodoc.Cloud{
			Name: "aws",
			Regions: []mongodoc.Region{{
				Name: "us-east-1",
			}},
		},
		Public: true,
	}, {
		Path: params.EntityPath{
			User: params.User("bob"),
			Name: "gce-us-east-1",
		},
		Cloud: mongodoc.Cloud{
			Name: "gce",
			Regions: []mongodoc.Region{{
				Name: "us-east-1",
			}},
		},
		Public: true,
	}, {
		Path: params.EntityPath{
			User: params.User("alice"),
			Name: "gce-us-east-1",
		},
		Cloud: mongodoc.Cloud{
			Name: "gce",
			Regions: []mongodoc.Region{{
				Name: "us-east-1",
			}},
		},
		Public: true,
	}}
	for i := range testControllers {
		err := s.jem.AddController(&testControllers[i], &mongodoc.Model{
			Path: testControllers[i].Path,
		})
		c.Assert(err, gc.IsNil)
	}
	req := s.newRequestForUser(c, "GET", "/", "bob", "bob-group")
	err := s.jem.Authenticate(req)
	c.Assert(err, gc.IsNil)
	for i, test := range doContollerTests {
		c.Logf("test %d. %s", i, test.about)
		var obtainedControllers []params.EntityPath
		err := s.jem.DoControllers(test.cloud, test.region, func(ctl *mongodoc.Controller) error {
			obtainedControllers = append(obtainedControllers, ctl.Path)
			return nil
		})
		c.Assert(err, gc.IsNil)
		c.Assert(obtainedControllers, jc.DeepEquals, test.expectControllers)
	}
}

func (s *authSuite) TestDoControllersErrorResponse(c *gc.C) {
	testControllers := []mongodoc.Controller{{
		Path: params.EntityPath{
			User: params.User("bob"),
			Name: "aws-us-east-1",
		},
		Cloud: mongodoc.Cloud{
			Name: "aws",
			Regions: []mongodoc.Region{{
				Name: "us-east-1",
			}},
		},
		Public: true,
	}, {
		Path: params.EntityPath{
			User: params.User("bob"),
			Name: "aws-eu-west-1",
		},
		Cloud: mongodoc.Cloud{
			Name: "aws",
			Regions: []mongodoc.Region{{
				Name: "eu-west-1",
			}},
		},
		Public: true,
	}, {
		Path: params.EntityPath{
			User: params.User("alice"),
			Name: "aws-us-east-1",
		},
		ACL: params.ACL{
			Read: []string{"bob-group"},
		},
		Cloud: mongodoc.Cloud{
			Name: "aws",
			Regions: []mongodoc.Region{{
				Name: "us-east-1",
			}},
		},
		Public: true,
	}, {
		Path: params.EntityPath{
			User: params.User("alice"),
			Name: "aws-eu-west-1",
		},
		ACL: params.ACL{
			Read: []string{"bob"},
		},
		Cloud: mongodoc.Cloud{
			Name: "aws",
			Regions: []mongodoc.Region{{
				Name: "eu-west-1",
			}},
		},
		Public: true,
	}, {
		Path: params.EntityPath{
			User: params.User("alice"),
			Name: "aws-us-east-2",
		},
		Cloud: mongodoc.Cloud{
			Name: "aws",
			Regions: []mongodoc.Region{{
				Name: "us-east-1",
			}},
		},
		Public: true,
	}, {
		Path: params.EntityPath{
			User: params.User("bob"),
			Name: "gce-us-east-1",
		},
		Cloud: mongodoc.Cloud{
			Name: "gce",
			Regions: []mongodoc.Region{{
				Name: "us-east-1",
			}},
		},
		Public: true,
	}, {
		Path: params.EntityPath{
			User: params.User("alice"),
			Name: "gce-us-east-1",
		},
		Cloud: mongodoc.Cloud{
			Name: "gce",
			Regions: []mongodoc.Region{{
				Name: "us-east-1",
			}},
		},
		Public: true,
	}}
	for i := range testControllers {
		err := s.jem.AddController(&testControllers[i], &mongodoc.Model{
			Path: testControllers[i].Path,
		})
		c.Assert(err, gc.IsNil)
	}
	req := s.newRequestForUser(c, "GET", "/", "bob", "bob-group")
	err := s.jem.Authenticate(req)
	c.Assert(err, gc.IsNil)
	testCause := errgo.New("test-cause")
	err = s.jem.DoControllers("", "", func(ctl *mongodoc.Controller) error {
		return errgo.WithCausef(nil, testCause, "test error")
	})
	c.Assert(errgo.Cause(err), gc.Equals, testCause)
}

var selectContollerTests = []struct {
	about            string
	cloud            params.Cloud
	region           string
	randIntn         func(int) int
	expectController params.EntityPath
	expectCloud      params.Cloud
	expectRegion     string
	expectError      string
	expectErrorCause error
}{{
	about: "no parameters",
	randIntn: func(n int) int {
		return 4
	},
	expectController: params.EntityPath{
		User: "bob",
		Name: "gce-us-east-1",
	},
	expectCloud: "gce",
}, {
	about: "aws",
	cloud: "aws",
	randIntn: func(n int) int {
		return 1
	},
	expectController: params.EntityPath{
		User: "alice",
		Name: "aws-us-east-1",
	},
	expectCloud: "aws",
}, {
	about:  "aws-us-east-1",
	cloud:  "aws",
	region: "us-east-1",
	randIntn: func(n int) int {
		return 1
	},
	expectController: params.EntityPath{
		User: "bob",
		Name: "aws-us-east-1",
	},
	expectCloud:  "aws",
	expectRegion: "us-east-1",
}, {
	about:  "no match",
	cloud:  "aws",
	region: "us-east-2",
	randIntn: func(n int) int {
		return 1
	},
	expectError:      `no matching controllers found`,
	expectErrorCause: params.ErrNotFound,
}}

func (s *authSuite) TestSelectController(c *gc.C) {
	var randIntn *func(int) int
	s.PatchValue(jem.RandIntn, func(n int) int {
		return (*randIntn)(n)
	})
	testControllers := []mongodoc.Controller{{
		Path: params.EntityPath{
			User: params.User("bob"),
			Name: "aws-us-east-1",
		},
		Cloud: mongodoc.Cloud{
			Name: "aws",
			Regions: []mongodoc.Region{{
				Name: "us-east-1",
			}},
		},
		Public: true,
	}, {
		Path: params.EntityPath{
			User: params.User("bob"),
			Name: "aws-eu-west-1",
		},
		Cloud: mongodoc.Cloud{
			Name: "aws",
			Regions: []mongodoc.Region{{
				Name: "eu-west-1",
			}},
		},
		Public: true,
	}, {
		Path: params.EntityPath{
			User: params.User("alice"),
			Name: "aws-us-east-1",
		},
		ACL: params.ACL{
			Read: []string{"bob-group"},
		},
		Cloud: mongodoc.Cloud{
			Name: "aws",
			Regions: []mongodoc.Region{{
				Name: "us-east-1",
			}},
		},
		Public: true,
	}, {
		Path: params.EntityPath{
			User: params.User("alice"),
			Name: "aws-eu-west-1",
		},
		ACL: params.ACL{
			Read: []string{"bob"},
		},
		Cloud: mongodoc.Cloud{
			Name: "aws",
			Regions: []mongodoc.Region{{
				Name: "eu-west-1",
			}},
		},
		Public: true,
	}, {
		Path: params.EntityPath{
			User: params.User("alice"),
			Name: "aws-us-east-2",
		},
		Cloud: mongodoc.Cloud{
			Name: "aws",
			Regions: []mongodoc.Region{{
				Name: "us-east-1",
			}},
		},
		Public: true,
	}, {
		Path: params.EntityPath{
			User: params.User("bob"),
			Name: "gce-us-east-1",
		},
		Cloud: mongodoc.Cloud{
			Name: "gce",
			Regions: []mongodoc.Region{{
				Name: "us-east-1",
			}},
		},
		Public: true,
	}, {
		Path: params.EntityPath{
			User: params.User("alice"),
			Name: "gce-us-east-1",
		},
		Cloud: mongodoc.Cloud{
			Name: "gce",
			Regions: []mongodoc.Region{{
				Name: "us-east-1",
			}},
		},
		Public: true,
	}}
	for i := range testControllers {
		err := s.jem.AddController(&testControllers[i], &mongodoc.Model{
			Path: testControllers[i].Path,
		})
		c.Assert(err, gc.IsNil)
	}
	req := s.newRequestForUser(c, "GET", "/", "bob", "bob-group")
	err := s.jem.Authenticate(req)
	c.Assert(err, gc.IsNil)
	for i, test := range selectContollerTests {
		c.Logf("test %d. %s", i, test.about)
		randIntn = &test.randIntn
		ctl, cloud, region, err := s.jem.SelectController(test.cloud, test.region)
		if test.expectError != "" {
			c.Assert(err, gc.ErrorMatches, test.expectError)
			if test.expectErrorCause != nil {
				c.Assert(errgo.Cause(err), gc.Equals, test.expectErrorCause)
			}
			continue
		}
		c.Assert(err, gc.IsNil)
		c.Assert(ctl, jc.DeepEquals, test.expectController)
		c.Assert(cloud, gc.Equals, test.expectCloud)
		c.Assert(region, gc.Equals, test.expectRegion)
	}
}

// newRequestForUser builds a new *http.Request for method at path which
// includes a macaroon authenticating username who will be placed in the
// specified groups.
func (s *authSuite) newRequestForUser(c *gc.C, method, path, username string, groups ...string) *http.Request {
	s.idmSrv.AddUser(username, groups...)
	s.idmSrv.SetDefaultUser(username)
	cl := s.idmSrv.Client(username)
	m, err := s.jem.NewMacaroon()
	c.Assert(err, gc.IsNil)
	ms, err := cl.DischargeAll(m)
	c.Assert(err, gc.IsNil)
	cookie, err := httpbakery.NewCookie(ms)
	c.Assert(err, gc.IsNil)
	req, err := http.NewRequest(method, path, nil)
	c.Assert(err, gc.IsNil)
	req.AddCookie(cookie)
	return req
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

var _ jem.ACLEntity = testEntity{}
