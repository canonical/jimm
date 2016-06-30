// Copyright 2015 Canonical Ltd.

package jem_test

import (
	"fmt"
	"sort"
	"time"

	"github.com/juju/idmclient"
	"github.com/juju/idmclient/idmtest"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/errgo.v1"
	"gopkg.in/macaroon-bakery.v1/bakery"
	"gopkg.in/mgo.v2/bson"

	"github.com/CanonicalLtd/jem/internal/jem"
	"github.com/CanonicalLtd/jem/internal/mongodoc"
	"github.com/CanonicalLtd/jem/params"
)

type jemSuite struct {
	jujutesting.IsolatedMgoSuite
	idmSrv *idmtest.Server
	pool   *jem.Pool
	store  *jem.JEM
}

var _ = gc.Suite(&jemSuite{})

func (s *jemSuite) SetUpTest(c *gc.C) {
	s.IsolatedMgoSuite.SetUpTest(c)
	s.idmSrv = idmtest.NewServer()
	pool, err := jem.NewPool(jem.Params{
		DB: s.Session.DB("jem"),
		BakeryParams: bakery.NewServiceParams{
			Location: "here",
		},
		IDMClient: idmclient.New(idmclient.NewParams{
			BaseURL: s.idmSrv.URL.String(),
			Client:  s.idmSrv.Client("agent"),
		}),
		ControllerAdmin: "controller-admin",
	})
	c.Assert(err, gc.IsNil)
	s.pool = pool
	s.store = s.pool.JEM()
}

func (s *jemSuite) TearDownTest(c *gc.C) {
	s.store.Close()
	s.pool.Close()
	s.IsolatedMgoSuite.TearDownTest(c)
}

func (s *jemSuite) TestPoolRequiresControllerAdmin(c *gc.C) {
	pool, err := jem.NewPool(jem.Params{
		DB: s.Session.DB("jem"),
		BakeryParams: bakery.NewServiceParams{
			Location: "here",
		},
		IDMClient: idmclient.New(idmclient.NewParams{
			BaseURL: s.idmSrv.URL.String(),
			Client:  s.idmSrv.Client("agent"),
		}),
	})
	c.Assert(err, gc.ErrorMatches, "no controller admin group specified")
	c.Assert(pool, gc.IsNil)
}

func (s *jemSuite) TestAddController(c *gc.C) {
	ctlPath := params.EntityPath{"bob", "x"}
	ctl := &mongodoc.Controller{
		Id:            "ignored",
		Path:          ctlPath,
		CACert:        "certainly",
		HostPorts:     []string{"host1:1234", "host2:9999"},
		AdminUser:     "foo-admin",
		AdminPassword: "foo-password",
		Location: map[string]string{
			"cloud":   "aws",
			"region":  "foo",
			"ignored": "",
		},
	}
	m := &mongodoc.Model{
		Id:   "ignored",
		Path: params.EntityPath{"ignored-user", "ignored-name"},
	}
	err := s.store.AddController(ctl, m)
	c.Assert(err, gc.IsNil)

	// Check that the fields have been mutated as expected.
	c.Assert(ctl, jc.DeepEquals, &mongodoc.Controller{
		Id:            "bob/x",
		Path:          ctlPath,
		CACert:        "certainly",
		HostPorts:     []string{"host1:1234", "host2:9999"},
		AdminUser:     "foo-admin",
		AdminPassword: "foo-password",
		Location: map[string]string{
			"cloud":  "aws",
			"region": "foo",
		},
	})
	c.Assert(m, jc.DeepEquals, &mongodoc.Model{
		Id:         "bob/x",
		Path:       ctlPath,
		Controller: ctlPath,
	})

	ctl1, err := s.store.Controller(ctlPath)
	c.Assert(err, gc.IsNil)
	c.Assert(ctl1, jc.DeepEquals, &mongodoc.Controller{
		Id:            "bob/x",
		Path:          ctlPath,
		CACert:        "certainly",
		HostPorts:     []string{"host1:1234", "host2:9999"},
		AdminUser:     "foo-admin",
		AdminPassword: "foo-password",
		Location: map[string]string{
			"cloud":  "aws",
			"region": "foo",
		},
	})
	m1, err := s.store.Model(ctlPath)
	c.Assert(err, gc.IsNil)
	c.Assert(m1, jc.DeepEquals, m)

	err = s.store.AddController(ctl, m)
	c.Assert(err, gc.ErrorMatches, "already exists")
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrAlreadyExists)

	ctlPath2 := params.EntityPath{"bob", "y"}
	ctl2 := &mongodoc.Controller{
		Id:            "ignored",
		Path:          ctlPath2,
		CACert:        "certainly",
		HostPorts:     []string{"host1:1234", "host2:9999"},
		AdminUser:     "foo-admin",
		AdminPassword: "foo-password",
		Location: map[string]string{
			"foo": "bar",
		},
	}
	m2 := &mongodoc.Model{
		Id:   "bob/noty",
		Path: params.EntityPath{"ignored-user", "ignored-name"},
	}
	err = s.store.AddController(ctl2, m2)
	c.Assert(err, gc.IsNil)
	m3, err := s.store.Model(ctlPath2)
	c.Assert(err, gc.IsNil)
	c.Assert(m3, jc.DeepEquals, m2)
}

func (s *jemSuite) TestAddControllerWithInvalidLocationAttr(c *gc.C) {
	ctlPath := params.EntityPath{"bob", "x"}
	ctl := &mongodoc.Controller{
		Path: ctlPath,
		Location: map[string]string{
			"foo.bar": "aws",
		},
	}
	err := s.store.AddController(ctl, &mongodoc.Model{})
	c.Check(errgo.Cause(err), gc.Equals, params.ErrBadRequest)
	c.Assert(err, gc.ErrorMatches, `bad controller location: invalid attribute "foo.bar"`)
}

func (s *jemSuite) TestSetControllerWithInvalidLocationAttr(c *gc.C) {
	ctlPath := params.EntityPath{"bob", "x"}
	ctl := &mongodoc.Controller{
		Path: ctlPath,
	}
	err := s.store.AddController(ctl, &mongodoc.Model{})
	err = s.store.SetControllerLocation(ctlPath, map[string]string{"foo.bar": "aws"})
	c.Check(errgo.Cause(err), gc.Equals, params.ErrBadRequest)
	c.Assert(err, gc.ErrorMatches, `bad controller location: invalid attribute "foo.bar"`)
}

func (s *jemSuite) TestSetControllerLocation(c *gc.C) {
	ctlPath := params.EntityPath{"bob", "x"}
	ctl := &mongodoc.Controller{
		Path: ctlPath,
	}
	err := s.store.AddController(ctl, &mongodoc.Model{})

	// Set cloud on controller.
	err = s.store.SetControllerLocation(ctlPath, map[string]string{"cloud": "aws"})
	c.Assert(err, gc.IsNil)
	ctl, err = s.store.Controller(ctlPath)
	c.Assert(err, gc.IsNil)
	c.Assert(ctl.Location, gc.DeepEquals, map[string]string{"cloud": "aws"})

	// Add region later.
	err = s.store.SetControllerLocation(ctlPath, map[string]string{"region": "us-east1"})
	c.Assert(err, gc.IsNil)
	ctl, err = s.store.Controller(ctlPath)
	c.Assert(err, gc.IsNil)
	c.Assert(ctl.Location, gc.DeepEquals, map[string]string{"cloud": "aws", "region": "us-east1"})

	// Remove cloud.
	err = s.store.SetControllerLocation(ctlPath, map[string]string{"cloud": ""})
	c.Assert(err, gc.IsNil)
	ctl, err = s.store.Controller(ctlPath)
	c.Assert(err, gc.IsNil)
	c.Assert(ctl.Location, gc.DeepEquals, map[string]string{"region": "us-east1"})
}

func (s *jemSuite) TestSetControllerAvailability(c *gc.C) {
	ctlPath := params.EntityPath{"bob", "x"}
	ctl := &mongodoc.Controller{
		Path: ctlPath,
	}
	err := s.store.AddController(ctl, &mongodoc.Model{})

	// Check that we can mark it as unavailable.
	t0 := time.Now()
	err = s.store.SetControllerUnavailableAt(ctlPath, t0)
	c.Assert(err, gc.IsNil)

	ctl, err = s.store.Controller(ctlPath)
	c.Assert(err, gc.IsNil)
	c.Assert(ctl.UnavailableSince.UTC(), jc.DeepEquals, mongodoc.Time(t0).UTC())

	// Check that if we mark it unavailable again, it doesn't
	// have any affect.
	err = s.store.SetControllerUnavailableAt(ctlPath, t0.Add(time.Second))
	c.Assert(err, gc.IsNil)

	ctl, err = s.store.Controller(ctlPath)
	c.Assert(err, gc.IsNil)
	c.Assert(ctl.UnavailableSince.UTC(), jc.DeepEquals, mongodoc.Time(t0).UTC())

	// Check that we can mark it as available again.
	err = s.store.SetControllerAvailable(ctlPath)
	c.Assert(err, gc.IsNil)

	ctl, err = s.store.Controller(ctlPath)
	c.Assert(err, gc.IsNil)
	c.Assert(ctl.UnavailableSince, jc.Satisfies, time.Time.IsZero)

	t1 := t0.Add(3 * time.Second)
	// ... and that we can mark it as unavailable after that.
	err = s.store.SetControllerUnavailableAt(ctlPath, t1)
	c.Assert(err, gc.IsNil)

	ctl, err = s.store.Controller(ctlPath)
	c.Assert(err, gc.IsNil)
	c.Assert(ctl.UnavailableSince.UTC(), jc.DeepEquals, mongodoc.Time(t1).UTC())
}

func (s *jemSuite) TestSetControllerAvailabilityWithNotFoundController(c *gc.C) {
	ctlPath := params.EntityPath{"bob", "x"}
	err := s.store.SetControllerUnavailableAt(ctlPath, time.Now())
	c.Assert(err, gc.IsNil)
	err = s.store.SetControllerAvailable(ctlPath)
	c.Assert(err, gc.IsNil)
}

func (s *jemSuite) TestControllerLocationQuery(c *gc.C) {
	ut := time.Now().UTC()
	for _, ctl := range []*mongodoc.Controller{{
		Path: params.EntityPath{"bob", "aws-us-east-1"},
		Location: map[string]string{
			"cloud":  "aws",
			"region": "us-east-1",
		},
		Public: true,
	}, {
		Path: params.EntityPath{"bob", "aws-eu-west-1"},
		Location: map[string]string{
			"cloud":  "aws",
			"region": "eu-west-1",
		},
		Public: true,
	}, {
		Path: params.EntityPath{"charlie", "other"},
		Location: map[string]string{
			"other": "something",
		},
		Public: true,
	}, {
		Path:   params.EntityPath{"charlie", "noattrs"},
		Public: true,
	}, {
		Path: params.EntityPath{"bob", "private"},
		Location: map[string]string{
			"cloud":  "aws",
			"region": "eu-west-1",
		},
	}, {
		Path: params.EntityPath{"bob", "down"},
		Location: map[string]string{
			"cloud":  "aws",
			"region": "eu-west-1",
		},
		UnavailableSince: ut,
		Public:           true,
	}} {
		err := s.store.AddController(ctl, &mongodoc.Model{})
		c.Assert(err, gc.IsNil)
	}

	tests := []struct {
		about              string
		location           map[string]string
		includeUnavailable bool
		expect             []string
		expectError        string
	}{{
		about: "single location attribute",
		location: map[string]string{
			"cloud": "aws",
		},
		expect: []string{
			"bob/aws-us-east-1",
			"bob/aws-eu-west-1",
		},
	}, {
		about:    "no location attributes",
		location: nil,
		expect: []string{
			"bob/aws-us-east-1",
			"bob/aws-eu-west-1",
			"charlie/other",
			"charlie/noattrs",
		},
	}, {
		about: "several location attributes",
		location: map[string]string{
			"cloud":  "aws",
			"region": "us-east-1",
		},
		expect: []string{
			"bob/aws-us-east-1",
		},
	}, {
		about: "include unavailable controllers",
		location: map[string]string{
			"cloud": "aws",
		},
		includeUnavailable: true,
		expect: []string{
			"bob/aws-us-east-1",
			"bob/aws-eu-west-1",
			"bob/down",
		},
	}, {
		about: "invalid location attribute",
		location: map[string]string{
			"invalid.attr$": "foo",
		},
		expectError: `bad controller location query: invalid attribute "invalid\.attr\$"`,
	}, {
		about: "empty location attribute",
		location: map[string]string{
			"cloud": "",
		},
		expect: []string{
			"charlie/other",
			"charlie/noattrs",
		},
	}}
	for i, test := range tests {
		c.Logf("test %d: %s", i, test.about)
		q, err := s.store.ControllerLocationQuery(test.location, test.includeUnavailable)
		if test.expectError != "" {
			c.Assert(err, gc.ErrorMatches, test.expectError)
			continue
		}
		var ctls []*mongodoc.Controller
		err = q.All(&ctls)
		c.Assert(err, gc.IsNil)
		var paths []string
		for _, ctl := range ctls {
			paths = append(paths, ctl.Path.String())
		}
		sort.Strings(paths)
		sort.Strings(test.expect)
		c.Assert(paths, jc.DeepEquals, test.expect)
	}
}

func (s *jemSuite) TestDeleteController(c *gc.C) {
	ctlPath := params.EntityPath{"dalek", "who"}
	ctl := &mongodoc.Controller{
		Id:            "ignored",
		Path:          ctlPath,
		CACert:        "certainly",
		HostPorts:     []string{"host1:1234", "host2:9999"},
		AdminUser:     "foo-admin",
		AdminPassword: "foo-password",
	}
	m := &mongodoc.Model{
		Id:   "dalek/who",
		Path: params.EntityPath{"ignored", "ignored"},
	}
	err := s.store.AddController(ctl, m)
	c.Assert(err, gc.IsNil)
	err = s.store.DeleteController(ctlPath)
	c.Assert(err, gc.IsNil)

	ctl1, err := s.store.Controller(ctlPath)
	c.Assert(ctl1, gc.IsNil)
	m1, err := s.store.Model(ctlPath)
	c.Assert(m1, gc.IsNil)

	err = s.store.DeleteController(ctlPath)
	c.Assert(err, gc.ErrorMatches, "controller \"dalek/who\" not found")
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)

	// Test with non-existing model.
	ctl2 := &mongodoc.Controller{
		Id:            "dalek/who",
		Path:          ctlPath,
		CACert:        "certainly",
		HostPorts:     []string{"host1:1234", "host2:9999"},
		AdminUser:     "foo-admin",
		AdminPassword: "foo-password",
	}
	m2 := &mongodoc.Model{
		Id:   "dalek/exterminated",
		Path: params.EntityPath{"ignored", "ignored"},
	}
	err = s.store.AddController(ctl2, m2)
	c.Assert(err, gc.IsNil)

	err = s.store.DeleteController(ctlPath)
	c.Assert(err, gc.IsNil)
	ctl3, err := s.store.Controller(ctlPath)
	c.Assert(ctl3, gc.IsNil)
	m3, err := s.store.Model(ctlPath)
	c.Assert(m3, gc.IsNil)
}

func (s *jemSuite) TestDeleteModel(c *gc.C) {
	ctlPath := params.EntityPath{"dalek", "who"}
	ctl := &mongodoc.Controller{
		Id:            "ignored",
		Path:          ctlPath,
		CACert:        "certainly",
		HostPorts:     []string{"host1:1234", "host2:9999"},
		AdminUser:     "foo-admin",
		AdminPassword: "foo-password",
	}
	m := &mongodoc.Model{
		Id:   "dalek/who",
		Path: params.EntityPath{"ignored", "ignored"},
	}
	err := s.store.AddController(ctl, m)
	c.Assert(err, gc.IsNil)

	err = s.store.DeleteModel(m.Path)
	c.Assert(err, gc.ErrorMatches, `cannot remove model "dalek/who" because it is a controller`)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrForbidden)

	modelPath := params.EntityPath{"dalek", "exterminate"}
	m2 := &mongodoc.Model{
		Id:   "dalek/exterminate",
		Path: modelPath,
	}
	err = s.store.AddModel(m2)
	c.Assert(err, gc.IsNil)

	err = s.store.DeleteModel(m2.Path)
	c.Assert(err, gc.IsNil)
	m3, err := s.store.Model(modelPath)
	c.Assert(m3, gc.IsNil)

	err = s.store.DeleteModel(m2.Path)
	c.Assert(err, gc.ErrorMatches, "model \"dalek/exterminate\" not found")
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)
}

func (s *jemSuite) TestAddModel(c *gc.C) {
	ctlPath := params.EntityPath{"bob", "x"}
	m := &mongodoc.Model{
		Id:   "ignored",
		Path: ctlPath,
	}
	err := s.store.AddModel(m)
	c.Assert(err, gc.IsNil)
	c.Assert(m, jc.DeepEquals, &mongodoc.Model{
		Id:   "bob/x",
		Path: ctlPath,
	})

	m1, err := s.store.Model(ctlPath)
	c.Assert(err, gc.IsNil)
	c.Assert(m1, jc.DeepEquals, m)

	err = s.store.AddModel(m)
	c.Assert(err, gc.ErrorMatches, "already exists")
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrAlreadyExists)
}

func (s *jemSuite) TestModelFromUUID(c *gc.C) {
	uuid := "99999999-9999-9999-9999-999999999999"
	path := params.EntityPath{"bob", "x"}
	m := &mongodoc.Model{
		Id:   "ignored",
		Path: path,
		UUID: uuid,
	}
	err := s.store.AddModel(m)
	c.Assert(err, gc.IsNil)
	c.Assert(m, jc.DeepEquals, &mongodoc.Model{
		Id:   "bob/x",
		Path: path,
		UUID: uuid,
	})

	m1, err := s.store.ModelFromUUID(uuid)
	c.Assert(err, gc.IsNil)
	c.Assert(m1, jc.DeepEquals, m)

	m2, err := s.store.ModelFromUUID("no-such-uuid")
	c.Assert(err, gc.ErrorMatches, `model "no-such-uuid" not found`)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)
	c.Assert(m2, gc.IsNil)
}

func (s *jemSuite) TestJEMCopiesSession(c *gc.C) {
	session := s.Session.Copy()
	pool, err := jem.NewPool(jem.Params{
		DB: session.DB("jem"),
		BakeryParams: bakery.NewServiceParams{
			Location: "here",
		},
		IDMClient: idmclient.New(idmclient.NewParams{
			BaseURL: s.idmSrv.URL.String(),
			Client:  s.idmSrv.Client("agent"),
		}),
		ControllerAdmin: "controller-admin",
	})
	c.Assert(err, gc.IsNil)

	store := pool.JEM()
	defer store.Close()
	// Check that we get an appropriate error when getting
	// a non-existent model, indicating that database
	// access is going OK.
	_, err = store.Model(params.EntityPath{"bob", "x"})
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)

	// Close the session and check that we still get the
	// same error.
	session.Close()

	_, err = store.Model(params.EntityPath{"bob", "x"})
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)

	// Also check the macaroon storage as that also has its own session reference.
	m, err := store.Bakery.NewMacaroon("", nil, nil)
	c.Assert(err, gc.IsNil)
	c.Assert(m, gc.NotNil)
}

func (s *jemSuite) TestClone(c *gc.C) {
	j := s.store.Clone()
	j.Close()
	_, err := s.store.Model(params.EntityPath{"bob", "x"})
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)
}

func (s *jemSuite) TestEnsureUserSuccess(c *gc.C) {
	n := 0
	s.PatchValue(jem.RandomPassword, func() (string, error) {
		n++
		return fmt.Sprintf("random%d", n), nil
	})
	ctlPath := params.EntityPath{
		User: "bob",
		Name: "bobcontroller",
	}
	err := s.store.AddController(&mongodoc.Controller{
		Id:            "ignored",
		Path:          ctlPath,
		CACert:        "certainly",
		HostPorts:     []string{"host1:1234", "host2:9999"},
		AdminUser:     "foo-admin",
		AdminPassword: "foo-password",
	}, &mongodoc.Model{
		Id:   "ignored",
		Path: params.EntityPath{"ignored-user", "ignored-name"},
	})
	c.Assert(err, gc.IsNil)

	// Calling EnsureUser should populate the initial user entry.
	password, err := s.store.EnsureUser(ctlPath, "jem-bob")
	c.Assert(err, gc.IsNil)
	c.Assert(password, gc.Equals, "random1")

	ctl, err := s.store.Controller(ctlPath)
	c.Assert(err, gc.IsNil)
	c.Assert(ctl.Users, jc.DeepEquals, map[string]mongodoc.UserInfo{
		mongodoc.Sanitize("jem-bob"): {
			Password: password,
		},
	})

	// Calling EnsureUser again should use the existing entry.
	password, err = s.store.EnsureUser(ctlPath, "jem-bob")
	c.Assert(err, gc.IsNil)
	c.Assert(password, gc.Equals, "random1")

	// Make sure the controller entry hasn't been changed.
	ctl, err = s.store.Controller(ctlPath)
	c.Assert(err, gc.IsNil)
	c.Assert(ctl.Users, jc.DeepEquals, map[string]mongodoc.UserInfo{
		mongodoc.Sanitize("jem-bob"): {
			Password: password,
		},
	})

	n = 99
	// Make sure we can add another user OK.
	password, err = s.store.EnsureUser(ctlPath, "jem-alice")
	c.Assert(err, gc.IsNil)
	c.Assert(password, gc.Equals, "random100")

	ctl, err = s.store.Controller(ctlPath)
	c.Assert(err, gc.IsNil)
	c.Assert(ctl.Users, jc.DeepEquals, map[string]mongodoc.UserInfo{
		mongodoc.Sanitize("jem-bob"): {
			Password: "random1",
		},
		mongodoc.Sanitize("jem-alice"): {
			Password: "random100",
		},
	})
}

func (s *jemSuite) TestEnsureUserNoController(c *gc.C) {
	ctlPath := params.EntityPath{
		User: "bob",
		Name: "bobcontroller",
	}
	password, err := s.store.EnsureUser(ctlPath, "jem-bob")
	c.Assert(err, gc.ErrorMatches, `cannot get controller: controller "bob/bobcontroller" not found`)
	c.Assert(password, gc.Equals, "")
}

func (s *jemSuite) TestSetModelManagedUser(c *gc.C) {
	modelPath := params.EntityPath{"bob", "x"}
	m := &mongodoc.Model{
		Id:   "ignored",
		Path: modelPath,
	}
	err := s.store.AddModel(m)
	c.Assert(err, gc.IsNil)
	err = s.store.SetModelManagedUser(modelPath, "jem-bob", mongodoc.ModelUserInfo{
		Granted: true,
	})
	c.Assert(err, gc.IsNil)

	m, err = s.store.Model(modelPath)
	c.Assert(err, gc.IsNil)
	c.Assert(m, jc.DeepEquals, &mongodoc.Model{
		Id:   "bob/x",
		Path: modelPath,
		Users: map[string]mongodoc.ModelUserInfo{
			mongodoc.Sanitize("jem-bob"): {
				Granted: true,
			},
		},
	})

	err = s.store.SetModelManagedUser(modelPath, "jem-alice", mongodoc.ModelUserInfo{
		Granted: false,
	})
	c.Assert(err, gc.IsNil)

	m, err = s.store.Model(modelPath)
	c.Assert(err, gc.IsNil)
	c.Assert(m, jc.DeepEquals, &mongodoc.Model{
		Id:   "bob/x",
		Path: modelPath,
		Users: map[string]mongodoc.ModelUserInfo{
			mongodoc.Sanitize("jem-bob"): {
				Granted: true,
			},
			mongodoc.Sanitize("jem-alice"): {
				Granted: false,
			},
		},
	})

	err = s.store.SetModelManagedUser(modelPath, "jem-bob", mongodoc.ModelUserInfo{
		Granted: false,
	})
	c.Assert(err, gc.IsNil)

	m, err = s.store.Model(modelPath)
	c.Assert(err, gc.IsNil)
	c.Assert(m, jc.DeepEquals, &mongodoc.Model{
		Id:   "bob/x",
		Path: modelPath,
		Users: map[string]mongodoc.ModelUserInfo{
			mongodoc.Sanitize("jem-bob"): {
				Granted: false,
			},
			mongodoc.Sanitize("jem-alice"): {
				Granted: false,
			},
		},
	})
}

//invalid key for mongo
//user already exists

var epoch = parseTime("2016-01-01T12:00:00Z")

const leaseExpiryDuration = 15 * time.Second

var acquireLeaseTests = []struct {
	about           string
	now             time.Time
	ctlPath         params.EntityPath
	oldExpiry       time.Time
	oldOwner        string
	newExpiry       time.Time
	newOwner        string
	actualOldExpiry time.Time
	actualOldOwner  string
	expectExpiry    time.Time
	expectError     string
	expectCause     error
}{{
	about:           "initial lease acquisition",
	ctlPath:         params.EntityPath{"bob", "foo"},
	oldExpiry:       time.Time{},
	newExpiry:       epoch.Add(leaseExpiryDuration),
	oldOwner:        "",
	newOwner:        "jem1",
	actualOldExpiry: time.Time{},
	actualOldOwner:  "",
	expectExpiry:    epoch.Add(leaseExpiryDuration),
}, {
	about:           "renewal",
	ctlPath:         params.EntityPath{"bob", "foo"},
	oldExpiry:       epoch.Add(leaseExpiryDuration),
	oldOwner:        "jem1",
	newExpiry:       epoch.Add(leaseExpiryDuration/2 + leaseExpiryDuration),
	newOwner:        "jem1",
	actualOldExpiry: epoch.Add(leaseExpiryDuration),
	actualOldOwner:  "jem1",
	expectExpiry:    epoch.Add(leaseExpiryDuration/2 + leaseExpiryDuration),
}, {
	about:           "renewal with time mismatch",
	ctlPath:         params.EntityPath{"bob", "foo"},
	oldExpiry:       epoch.Add(leaseExpiryDuration),
	oldOwner:        "jem1",
	newExpiry:       epoch.Add(leaseExpiryDuration * 3),
	newOwner:        "jem1",
	actualOldExpiry: epoch.Add(leaseExpiryDuration * 2),
	actualOldOwner:  "jem1",
	expectError:     `controller has lease taken out by "jem1" expiring at 2016-01-01 12:00:30 \+0000 UTC`,
	expectCause:     jem.ErrLeaseUnavailable,
}, {
	about:           "renewal with owner mismatch",
	ctlPath:         params.EntityPath{"bob", "foo"},
	oldExpiry:       epoch.Add(leaseExpiryDuration),
	oldOwner:        "jem1",
	newOwner:        "jem1",
	actualOldExpiry: epoch.Add(leaseExpiryDuration),
	actualOldOwner:  "jem0",
	expectError:     `controller has lease taken out by "jem0" expiring at 2016-01-01 12:00:15 \+0000 UTC`,
	expectCause:     jem.ErrLeaseUnavailable,
}, {
	about:           "drop lease",
	now:             epoch.Add(leaseExpiryDuration / 2),
	ctlPath:         params.EntityPath{"bob", "foo"},
	oldExpiry:       epoch.Add(leaseExpiryDuration),
	oldOwner:        "jem1",
	newOwner:        "",
	actualOldExpiry: epoch.Add(leaseExpiryDuration),
	actualOldOwner:  "jem1",
	expectExpiry:    time.Time{},
}, {
	about:           "drop never-acquired lease",
	now:             epoch,
	ctlPath:         params.EntityPath{"bob", "foo"},
	oldOwner:        "",
	newOwner:        "",
	actualOldExpiry: time.Time{},
	actualOldOwner:  "",
	expectExpiry:    time.Time{},
}}

func (s *jemSuite) TestAcquireLease(c *gc.C) {
	for i, test := range acquireLeaseTests {
		c.Logf("test %d: %v", i, test.about)
		_, err := s.store.DB.Controllers().RemoveAll(bson.D{{"path", test.ctlPath}})
		c.Assert(err, gc.IsNil)
		_, err = s.store.DB.Models().RemoveAll(bson.D{{"path", test.ctlPath}})
		c.Assert(err, gc.IsNil)
		err = s.store.AddController(&mongodoc.Controller{
			Path:               test.ctlPath,
			UUID:               "fake-uuid",
			MonitorLeaseOwner:  test.actualOldOwner,
			MonitorLeaseExpiry: test.actualOldExpiry,
		}, &mongodoc.Model{})
		c.Assert(err, gc.IsNil)
		t, err := s.store.AcquireMonitorLease(test.ctlPath, test.oldExpiry, test.oldOwner, test.newExpiry, test.newOwner)
		if test.expectError != "" {
			if test.expectCause != nil {
				c.Check(errgo.Cause(err), gc.Equals, test.expectCause)
			}
			c.Assert(err, gc.ErrorMatches, test.expectError)
			c.Assert(t, jc.Satisfies, time.Time.IsZero)
			continue
		}
		c.Assert(err, gc.IsNil)
		c.Assert(t.UTC(), gc.DeepEquals, test.expectExpiry.UTC())
		ctl, err := s.store.Controller(test.ctlPath)
		c.Assert(err, gc.IsNil)
		c.Assert(ctl.MonitorLeaseExpiry.UTC(), gc.DeepEquals, test.expectExpiry.UTC())
		c.Assert(ctl.MonitorLeaseOwner, gc.Equals, test.newOwner)
	}
}

func (s *jemSuite) TestSetControllerStatsNotFound(c *gc.C) {
	err := s.store.SetControllerStats(params.EntityPath{"bob", "foo"}, &mongodoc.ControllerStats{})
	c.Assert(err, gc.ErrorMatches, "controller not found")
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)
}

func (s *jemSuite) TestSetControllerStats(c *gc.C) {
	ctlPath := params.EntityPath{"bob", "foo"}
	err := s.store.AddController(&mongodoc.Controller{
		Path: ctlPath,
		UUID: "fake-uuid",
	}, &mongodoc.Model{})
	c.Assert(err, gc.IsNil)

	stats := &mongodoc.ControllerStats{
		UnitCount:    1,
		ModelCount:   2,
		ServiceCount: 3,
		MachineCount: 4,
	}
	err = s.store.SetControllerStats(ctlPath, stats)
	c.Assert(err, gc.IsNil)
	ctl, err := s.store.Controller(ctlPath)
	c.Assert(err, gc.IsNil)
	c.Assert(ctl.Stats, jc.DeepEquals, *stats)
}

func (s *jemSuite) TestSetModelLifeNotFound(c *gc.C) {
	err := s.store.SetModelLife(params.EntityPath{"bob", "foo"}, "fake-uuid", "alive")
	c.Assert(err, gc.IsNil)
}

func (s *jemSuite) TestSetModelLifeSuccess(c *gc.C) {
	ctlPath := params.EntityPath{"bob", "foo"}
	err := s.store.AddController(&mongodoc.Controller{
		Path: ctlPath,
		UUID: "fake-uuid",
	}, &mongodoc.Model{
		UUID: "fake-uuid",
	})
	c.Assert(err, gc.IsNil)

	// Add another model with the same UUID but a different controller.
	err = s.store.AddModel(&mongodoc.Model{
		Path:       params.EntityPath{"bar", "baz"},
		UUID:       "fake-uuid",
		Controller: params.EntityPath{"bar", "zzz"},
	})
	c.Assert(err, gc.IsNil)

	// Add another model with the same controller but a different UUID.
	err = s.store.AddModel(&mongodoc.Model{
		Path:       params.EntityPath{"alice", "baz"},
		UUID:       "another-uuid",
		Controller: ctlPath,
	})
	c.Assert(err, gc.IsNil)

	err = s.store.SetModelLife(ctlPath, "fake-uuid", "alive")
	c.Assert(err, gc.IsNil)

	m, err := s.store.Model(ctlPath)
	c.Assert(err, gc.IsNil)
	c.Assert(m.Life, gc.Equals, "alive")

	m, err = s.store.Model(params.EntityPath{"bar", "baz"})
	c.Assert(err, gc.IsNil)
	c.Assert(m.Life, gc.Equals, "")

	m, err = s.store.Model(params.EntityPath{"alice", "baz"})
	c.Assert(err, gc.IsNil)
	c.Assert(m.Life, gc.Equals, "")
}

func (s *jemSuite) TestAcquireLeaseControllerNotFound(c *gc.C) {
	_, err := s.store.AcquireMonitorLease(params.EntityPath{"bob", "foo"}, time.Time{}, "", time.Now(), "jem1")
	c.Assert(err, gc.ErrorMatches, `controller removed`)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)
}

func (s *jemSuite) TestAddAndGetCredential(c *gc.C) {
	path := params.EntityPath{"test-user", "test-credential"}
	cred, err := s.store.Credential(path)
	c.Assert(cred, gc.IsNil)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)
	c.Assert(err, gc.ErrorMatches, `credential "test-user/test-credential" not found`)

	attrs := map[string]string{
		"attr1": "val1",
		"attr2": "val2",
	}
	err = s.store.AddCredential(&mongodoc.Credential{
		Path:         path,
		ProviderType: "provtype",
		Type:         "credtype",
		Label:        "Test Label",
		Attributes:   attrs,
	})
	c.Assert(err, gc.IsNil)

	cred, err = s.store.Credential(path)
	c.Assert(err, gc.IsNil)
	c.Assert(cred, jc.DeepEquals, &mongodoc.Credential{
		Id:           path.String(),
		Path:         path,
		ProviderType: "provtype",
		Type:         "credtype",
		Label:        "Test Label",
		Attributes:   attrs,
	})

	err = s.store.AddCredential(&mongodoc.Credential{
		Path:         path,
		ProviderType: "provtype",
		Type:         "credtype",
		Label:        "Test Label 2",
		Attributes:   attrs,
	})
	c.Assert(err, gc.IsNil)

	cred, err = s.store.Credential(path)
	c.Assert(err, gc.IsNil)
	c.Assert(cred, jc.DeepEquals, &mongodoc.Credential{
		Id:           path.String(),
		Path:         path,
		ProviderType: "provtype",
		Type:         "credtype",
		Label:        "Test Label 2",
		Attributes:   attrs,
	})
}

func parseTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}
