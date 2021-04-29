// Copyright 2015 Canonical Ltd.

package jem_test

import (
	"context"
	"time"

	jujuapi "github.com/juju/juju/api"
	jujuparams "github.com/juju/juju/apiserver/params"
	jt "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	gc "gopkg.in/check.v1"
	"gopkg.in/errgo.v1"

	"github.com/CanonicalLtd/jimm/internal/conv"
	"github.com/CanonicalLtd/jimm/internal/jem"
	"github.com/CanonicalLtd/jimm/internal/jemtest"
	"github.com/CanonicalLtd/jimm/internal/mgosession"
	"github.com/CanonicalLtd/jimm/internal/mongodoc"
	"github.com/CanonicalLtd/jimm/internal/pubsub"
	"github.com/CanonicalLtd/jimm/params"
)

var testContext = context.Background()

type jemSuite struct {
	jemtest.BootstrapSuite
}

var _ = gc.Suite(&jemSuite{})

func (s *jemSuite) SetUpTest(c *gc.C) {
	s.Params.Pubsub = &pubsub.Hub{MaxConcurrency: 10}
	s.BootstrapSuite.SetUpTest(c)
}

func (s *jemSuite) TestPoolRequiresControllerAdmin(c *gc.C) {
	pool, err := jem.NewPool(testContext, jem.Params{
		DB: s.Session.DB("jem"),
	})
	c.Assert(err, gc.ErrorMatches, "no controller admin group specified")
	c.Assert(pool, gc.IsNil)
}

func (s *jemSuite) TestPoolDoesNotReuseDeadConnection(c *gc.C) {
	session := jt.NewProxiedSession(c)
	defer session.Close()
	sessionPool := mgosession.NewPool(testContext, session.Session, 3)
	defer sessionPool.Close()
	pool, err := jem.NewPool(testContext, jem.Params{
		DB:              session.DB("jem"),
		ControllerAdmin: jemtest.ControllerAdmin,
		SessionPool:     sessionPool,
	})
	c.Assert(err, gc.Equals, nil)
	defer pool.Close()

	assertOK := func(j *jem.JEM) {
		m := mongodoc.Model{Path: params.EntityPath{"bob", "x"}}
		err := j.DB.GetModel(testContext, &m)
		c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)
	}
	assertBroken := func(j *jem.JEM) {
		m := mongodoc.Model{Path: params.EntityPath{"bob", "x"}}
		err = j.DB.GetModel(testContext, &m)
		c.Assert(err, gc.ErrorMatches, `cannot get model: EOF`)
	}

	// Get a JEM instance and perform a single operation so that the session used by the
	// JEM instance obtains a mongo socket.
	c.Logf("make jem0")
	jem0 := pool.JEM(context.TODO())
	defer jem0.Close()
	assertOK(jem0)

	c.Logf("close connections")
	// Close all current connections to the mongo instance,
	// which should cause subsequent operations on jem1 to fail.
	session.CloseConns()

	// Get another JEM instance, which should be a new session,
	// so operations on it should not fail.
	c.Logf("make jem1")
	jem1 := pool.JEM(context.TODO())
	defer jem1.Close()
	assertOK(jem1)

	// Get another JEM instance which should clone the same session
	// used by jem0 because only two sessions are available.
	c.Logf("make jem2")
	jem2 := pool.JEM(context.TODO())
	defer jem2.Close()

	// Perform another operation on jem0, which should fail and
	// cause its session not to be reused.
	c.Logf("check jem0 is broken")
	assertBroken(jem0)

	// The jem1 connection should still be working because it
	// was created after the connections were broken.
	c.Logf("check jem1 is ok")
	assertOK(jem1)

	c.Logf("check jem2 is ok")
	// The jem2 connection should also be broken because it
	// reused the same sessions as jem0
	assertBroken(jem2)

	// Get another instance, which should reuse the jem3 connection
	// and work OK.
	c.Logf("make jem3")
	jem3 := pool.JEM(context.TODO())
	defer jem3.Close()
	assertOK(jem3)

	// When getting the next instance, we should see that the connection
	// that we would have used is broken and create another one.
	c.Logf("make jem4")
	jem4 := pool.JEM(context.TODO())
	defer jem4.Close()
	assertOK(jem4)
}

func (s *jemSuite) TestClone(c *gc.C) {
	j := s.JEM.Clone()
	j.Close()
	m := mongodoc.Model{Path: params.EntityPath{"bob", "x"}}
	err := s.JEM.DB.GetModel(testContext, &m)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)
}

var earliestControllerVersionTests = []struct {
	about       string
	controllers []mongodoc.Controller
	expect      version.Number
}{{
	about:  "no controllers",
	expect: version.Number{},
}, {
	about: "one controller",
	controllers: []mongodoc.Controller{{
		Path:    params.EntityPath{"bob", "c1"},
		Public:  true,
		Version: &version.Number{Minor: 1},
		ACL: params.ACL{
			Read: []string{"everyone"},
		},
	}},
	expect: version.Number{Minor: 1},
}, {
	about: "multiple controllers",
	controllers: []mongodoc.Controller{{
		Path:    params.EntityPath{"bob", "c1"},
		Public:  true,
		Version: &version.Number{Minor: 1},
		ACL: params.ACL{
			Read: []string{"everyone"},
		},
	}, {
		Path:    params.EntityPath{"bob", "c2"},
		Public:  true,
		Version: &version.Number{Minor: 2},
		ACL: params.ACL{
			Read: []string{"everyone"},
		},
	}, {
		Path:    params.EntityPath{"bob", "c3"},
		Public:  true,
		Version: &version.Number{Minor: 3},
		ACL: params.ACL{
			Read: []string{"everyone"},
		},
	}},
	expect: version.Number{Minor: 1},
}, {
	about: "non-public controllers ignored",
	controllers: []mongodoc.Controller{{
		Path:    params.EntityPath{"bob", "c1"},
		Version: &version.Number{Minor: 1},
	}, {
		Path:   params.EntityPath{"bob", "c2"},
		Public: true,
		ACL: params.ACL{
			Read: []string{"everyone"},
		},
		Version: &version.Number{Minor: 2},
	}},
	expect: version.Number{Minor: 2},
}}

func (s *jemSuite) TestEarliestControllerVersion(c *gc.C) {
	for i, test := range earliestControllerVersionTests {
		c.Logf("test %d: %v", i, test.about)
		_, err := s.JEM.DB.Controllers().RemoveAll(nil)
		c.Assert(err, gc.Equals, nil)
		for _, ctl := range test.controllers {
			err := s.JEM.DB.InsertController(testContext, &ctl)
			c.Assert(err, gc.Equals, nil)
		}
		v, err := s.JEM.EarliestControllerVersion(testContext, jemtest.NewIdentity("someone"))
		c.Assert(err, gc.Equals, nil)
		c.Assert(v, jc.DeepEquals, test.expect)
	}
}

func (s *jemSuite) TestWatchAllModelSummaries(c *gc.C) {
	pubsub := s.JEM.Pubsub()
	summaryChannel := make(chan interface{}, 1)
	handlerFunction := func(_ string, summary interface{}) {
		select {
		case summaryChannel <- summary:
		default:
		}
	}
	cleanup, err := pubsub.Subscribe(s.Model.UUID, handlerFunction)
	c.Assert(err, jc.ErrorIsNil)
	defer cleanup()

	watcherCleanup, err := s.JEM.WatchAllModelSummaries(context.Background(), s.Controller.Path)
	c.Assert(err, gc.Equals, nil)
	defer func() {
		err := watcherCleanup()
		if err != nil {
			c.Logf("failed to stop all model summaries watcher: %v", err)
		}
	}()

	select {
	case summary := <-summaryChannel:
		c.Check(summary, gc.DeepEquals,
			jujuparams.ModelAbstract{
				UUID:       s.Model.UUID,
				Removed:    false,
				Controller: "",
				Name:       string(s.Model.Path.Name),
				Admins:     []string{conv.ToUserTag(s.Model.Path.User).Id()},
				Cloud:      "dummy",
				Region:     "dummy-region",
				Credential: conv.ToCloudCredentialTag(s.Model.Credential).Id(),
				Size: jujuparams.ModelSummarySize{
					Machines:     0,
					Containers:   0,
					Applications: 0,
					Units:        0,
					Relations:    0,
				},
				Status: "green",
			})
	case <-time.After(time.Second):
		c.Fatal("timed out")
	}
}

func addController(c *gc.C, path params.EntityPath, info *jujuapi.Info, jem *jem.JEM) params.EntityPath {
	hps, err := mongodoc.ParseAddresses(info.Addrs)
	c.Assert(err, gc.Equals, nil)

	ctl := &mongodoc.Controller{
		Path:          path,
		HostPorts:     [][]mongodoc.HostPort{hps},
		CACert:        info.CACert,
		AdminUser:     info.Tag.Id(),
		AdminPassword: info.Password,
		Public:        true,
	}
	err = jem.AddController(testContext, jemtest.NewIdentity(string(path.User), string(jem.ControllerAdmin())), ctl)
	c.Assert(err, gc.Equals, nil)

	return path
}
