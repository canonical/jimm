// Copyright 2016 Canonical Ltd.

package jujuapi_test

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	controllerapi "github.com/juju/juju/api/controller"
	"github.com/juju/juju/api/modelmanager"
	"github.com/juju/juju/api/usermanager"
	jujuparams "github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/rpc"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"
	errgo "gopkg.in/errgo.v1"
	"gopkg.in/macaroon.v2"

	"github.com/CanonicalLtd/jimm/internal/jujuapi"
	"github.com/CanonicalLtd/jimm/params"
	jimmversion "github.com/CanonicalLtd/jimm/version"
)

type controllerSuite struct {
	websocketSuite
}

var _ = gc.Suite(&controllerSuite{})

func (s *controllerSuite) SetUpTest(c *gc.C) {
	s.ServerParams.CharmstoreLocation = "https://api.jujucharms.com/charmstore"
	s.ServerParams.MeteringLocation = "https://api.jujucharms.com/omnibus"
	s.websocketSuite.SetUpTest(c)
	s.PatchValue(&utils.OutgoingAccessAllowed, true)
}

func (s *controllerSuite) TestServerVersion(c *gc.C) {
	ctx := context.Background()

	ctlPath := params.EntityPath{"test", "controller-1"}
	s.AssertAddController(ctx, c, ctlPath, true)
	testVersion := version.MustParse("5.4.3")
	err := s.JEM.DB.SetControllerVersion(ctx, ctlPath, testVersion)
	c.Assert(err, gc.Equals, nil)

	conn := s.open(c, nil, "test")
	defer conn.Close()

	v, ok := conn.ServerVersion()
	c.Assert(ok, gc.Equals, true)
	c.Assert(v, jc.DeepEquals, testVersion)
}

func (s *controllerSuite) TestOldAdminVersionFails(c *gc.C) {
	ctx := context.Background()

	s.AssertAddController(ctx, c, params.EntityPath{User: "test", Name: "controller-1"}, true)
	cred := s.AssertUpdateCredential(ctx, c, "test", "dummy", "cred1", "empty")
	mi := s.assertCreateModel(c, createModelParams{name: "model-1", username: "test", cred: cred})
	modelUUID := mi.UUID
	conn := s.open(c, &api.Info{
		ModelTag:  names.NewModelTag(modelUUID),
		SkipLogin: true,
	}, "test")
	defer conn.Close()
	var resp jujuparams.RedirectInfoResult
	err := conn.APICall("Admin", 2, "", "Login", nil, &resp)
	c.Assert(err, gc.ErrorMatches, `JIMM does not support login from old clients \(not supported\)`)
	c.Assert(resp, jc.DeepEquals, jujuparams.RedirectInfoResult{})
}

func (s *controllerSuite) TestAdminIDFails(c *gc.C) {
	ctx := context.Background()

	s.AssertAddController(ctx, c, params.EntityPath{User: "test", Name: "controller-1"}, true)
	cred := s.AssertUpdateCredential(ctx, c, "test", "dummy", "cred1", "empty")
	mi := s.assertCreateModel(c, createModelParams{name: "model-1", username: "test", cred: cred})
	modelUUID := mi.UUID
	conn := s.open(c, &api.Info{
		ModelTag:  names.NewModelTag(modelUUID),
		SkipLogin: true,
	}, "test")
	defer conn.Close()
	var resp jujuparams.RedirectInfoResult
	err := conn.APICall("Admin", 3, "Object ID", "Login", nil, &resp)
	c.Assert(err, gc.ErrorMatches, "id not found")
}

func (s *controllerSuite) TestLoginToController(c *gc.C) {
	ctx := context.Background()

	s.AssertAddController(ctx, c, params.EntityPath{User: "test", Name: "controller-1"}, true)
	conn := s.open(c, &api.Info{
		SkipLogin: true,
	}, "test")
	defer conn.Close()
	err := conn.Login(nil, "", "", nil)
	c.Assert(err, gc.Equals, nil)
	var resp jujuparams.RedirectInfoResult
	err = conn.APICall("Admin", 3, "", "RedirectInfo", nil, &resp)
	rerr, ok := errgo.Cause(err).(*rpc.RequestError)
	c.Assert(ok, gc.Equals, true)
	c.Assert(rerr.Code, gc.Equals, jujuparams.CodeNotImplemented)
}

func (s *controllerSuite) TestLoginToControllerWithInvalidMacaroon(c *gc.C) {
	ctx := context.Background()

	s.AssertAddController(ctx, c, params.EntityPath{User: "test", Name: "controller-1"}, true)
	invalidMacaroon, err := macaroon.New(nil, []byte("invalid"), "", macaroon.V1)
	c.Assert(err, gc.Equals, nil)
	conn := s.open(c, &api.Info{
		Macaroons: []macaroon.Slice{{invalidMacaroon}},
	}, "test")
	conn.Close()
}

func (s *controllerSuite) TestUnimplementedMethodFails(c *gc.C) {
	ctx := context.Background()

	s.AssertAddController(ctx, c, params.EntityPath{User: "test", Name: "controller-1"}, true)
	cred := s.AssertUpdateCredential(ctx, c, "test", "dummy", "cred1", "empty")
	mi := s.assertCreateModel(c, createModelParams{name: "model-1", username: "test", cred: cred})
	modelUUID := mi.UUID
	conn := s.open(c, &api.Info{
		ModelTag:  names.NewModelTag(modelUUID),
		SkipLogin: true,
	}, "test")
	defer conn.Close()
	var resp jujuparams.RedirectInfoResult
	err := conn.APICall("Admin", 3, "", "Logout", nil, &resp)
	c.Assert(err, gc.ErrorMatches, `no such request - method Admin.Logout is not implemented \(not implemented\)`)
}

func (s *controllerSuite) TestUnimplementedRootFails(c *gc.C) {
	ctx := context.Background()

	s.AssertAddController(ctx, c, params.EntityPath{User: "test", Name: "controller-1"}, true)
	conn := s.open(c, nil, "test")
	defer conn.Close()
	var resp jujuparams.RedirectInfoResult
	err := conn.APICall("NoSuch", 1, "", "Method", nil, &resp)
	c.Assert(err, gc.ErrorMatches, `unknown version \(1\) of interface "NoSuch" \(not implemented\)`)
}

func (s *controllerSuite) TestJIMMFacadeVersion(c *gc.C) {
	conn := s.open(c, nil, "test")
	defer conn.Close()
	c.Assert(conn.AllFacadeVersions()["JIMM"], jc.DeepEquals, []int{1, 2})
}

func (s *controllerSuite) TestUserModelStats(c *gc.C) {
	ctx := context.Background()

	ctlPath := s.AssertAddController(ctx, c, params.EntityPath{User: "test", Name: "controller-1"}, true)
	cred := s.AssertUpdateCredential(ctx, c, "test", "dummy", "cred1", "empty")
	cred2 := s.AssertUpdateCredential(ctx, c, "test2", "dummy", "cred1", "empty")
	err := s.JEM.DB.SetACL(ctx, s.JEM.DB.Controllers(), ctlPath, params.ACL{
		Read: []string{"test2"},
	})

	c.Assert(err, gc.Equals, nil)

	model1 := s.assertCreateModel(c, createModelParams{name: "model-1", username: "test", cred: cred})
	model2 := s.assertCreateModel(c, createModelParams{name: "model-2", username: "test2", cred: cred2})
	model3 := s.assertCreateModel(c, createModelParams{name: "model-3", username: "test2", cred: cred2})

	// Update some stats for the models we've just created'
	t0 := time.Unix(0, 0)

	err = s.JEM.DB.UpdateModelCounts(ctx, ctlPath, model1.UUID, map[params.EntityCount]int{
		params.UnitCount: 99,
	}, t0)

	c.Assert(err, gc.Equals, nil)
	err = s.JEM.DB.UpdateModelCounts(ctx, ctlPath, model2.UUID, map[params.EntityCount]int{
		params.MachineCount: 10,
	}, t0)

	c.Assert(err, gc.Equals, nil)
	err = s.JEM.DB.UpdateModelCounts(ctx, ctlPath, model3.UUID, map[params.EntityCount]int{
		params.ApplicationCount: 1,
	}, t0)

	c.Assert(err, gc.Equals, nil)

	// Allow test2/model-3 access to everyone, so that we can be sure we're
	// not seeing models that we have access to but aren't the creator of.
	err = s.JEM.DB.SetACL(ctx, s.JEM.DB.Models(), params.EntityPath{User: "test2", Name: "model-3"}, params.ACL{
		Read: []string{"test"},
	})

	// Open the API connection as user "test". We should only see the one model.
	conn := s.open(c, nil, "test")
	defer conn.Close()
	var resp params.UserModelStatsResponse
	err = conn.APICall("JIMM", 1, "", "UserModelStats", nil, &resp)
	c.Assert(err, gc.Equals, nil)

	c.Assert(resp, jc.DeepEquals, params.UserModelStatsResponse{
		Models: map[string]params.ModelStats{
			model1.UUID: {
				Model: jujuparams.Model{
					Name:     "model-1",
					UUID:     model1.UUID,
					OwnerTag: names.NewUserTag(model1.Owner).String(),
				},
				Counts: map[params.EntityCount]params.Count{
					params.UnitCount: {
						Time:    t0,
						Current: 99,
						Max:     99,
						Total:   99,
					},
				},
			},
		},
	})

	// As test2, we should see the other two models.
	conn = s.open(c, nil, "test2")
	defer conn.Close()
	resp = params.UserModelStatsResponse{}
	err = conn.APICall("JIMM", 1, "", "UserModelStats", nil, &resp)
	c.Assert(err, gc.Equals, nil)

	c.Assert(resp, jc.DeepEquals, params.UserModelStatsResponse{
		Models: map[string]params.ModelStats{
			model2.UUID: {
				Model: jujuparams.Model{
					Name:     "model-2",
					UUID:     model2.UUID,
					OwnerTag: names.NewUserTag(model2.Owner).String(),
				},
				Counts: map[params.EntityCount]params.Count{
					params.MachineCount: {
						Time:    t0,
						Current: 10,
						Max:     10,
						Total:   10,
					},
				},
			},
			model3.UUID: {
				Model: jujuparams.Model{
					Name:     "model-3",
					UUID:     model3.UUID,
					OwnerTag: names.NewUserTag(model3.Owner).String(),
				},
				Counts: map[params.EntityCount]params.Count{
					params.ApplicationCount: {
						Time:    t0,
						Current: 1,
						Max:     1,
						Total:   1,
					},
				},
			},
		},
	})
}

func (s *controllerSuite) TestListControllers(c *gc.C) {
	ctx := context.Background()
	ctlId0 := s.AssertAddController(ctx, c, params.EntityPath{User: "test", Name: "controller-0"}, true)
	ctlId1 := s.AssertAddController(ctx, c, params.EntityPath{User: "test", Name: "controller-1"}, true)
	ctlId2 := s.AssertAddController(ctx, c, params.EntityPath{User: "test", Name: "controller-2"}, true)

	c0, err := s.JEM.DB.Controller(context.Background(), ctlId0)
	c.Assert(err, gc.Equals, nil)
	c1, err := s.JEM.DB.Controller(context.Background(), ctlId1)
	c.Assert(err, gc.Equals, nil)
	c2, err := s.JEM.DB.Controller(context.Background(), ctlId2)
	c.Assert(err, gc.Equals, nil)

	// Open the API connection as user "test".
	conn := s.open(c, nil, "test")
	defer conn.Close()
	var resp params.ListControllerResponse
	err = conn.APICall("JIMM", 2, "", "ListControllers", nil, &resp)
	c.Assert(err, gc.Equals, nil)

	c.Assert(resp, jc.DeepEquals, params.ListControllerResponse{
		Controllers: []params.ControllerResponse{{
			Path:     ctlId0,
			Location: map[string]string{"cloud": "dummy", "region": "dummy-region"},
			Public:   true,
			UUID:     c0.UUID,
			Version:  c0.Version.String(),
		}, {
			Path:     ctlId1,
			Location: map[string]string{"cloud": "dummy", "region": "dummy-region"},
			Public:   true,
			UUID:     c1.UUID,
			Version:  c1.Version.String(),
		}, {
			Path:     ctlId2,
			Location: map[string]string{"cloud": "dummy", "region": "dummy-region"},
			Public:   true,
			UUID:     c2.UUID,
			Version:  c2.Version.String(),
		}},
	})
}

func (s *controllerSuite) TestListControllersUnauthorizedUser(c *gc.C) {
	ctx := context.Background()
	s.AssertAddController(ctx, c, params.EntityPath{User: "test", Name: "controller-0"}, true)
	s.AssertAddController(ctx, c, params.EntityPath{User: "test", Name: "controller-1"}, true)
	s.AssertAddController(ctx, c, params.EntityPath{User: "test", Name: "controller-2"}, true)

	// Open the API connection as user "unknown-user".
	conn := s.open(c, nil, "unknown-user")
	defer conn.Close()
	var resp params.ListControllerResponse
	err := conn.APICall("JIMM", 2, "", "ListControllers", nil, &resp)
	c.Assert(err, gc.Equals, nil)

	c.Assert(resp, jc.DeepEquals, params.ListControllerResponse{
		Controllers: []params.ControllerResponse{{
			Path:    params.EntityPath{User: "admin", Name: "jaas"},
			Public:  true,
			UUID:    "914487b5-60e7-42bb-bd63-1adc3fd3a388",
			Version: "0.0.0",
		}},
	})
}

func (s *controllerSuite) TestControllerConfig(c *gc.C) {
	conn := s.open(c, nil, "test")
	defer conn.Close()
	client := controllerapi.NewClient(conn)
	conf, err := client.ControllerConfig()
	c.Assert(err, gc.Equals, nil)
	c.Assert(conf, jc.DeepEquals, controller.Config(map[string]interface{}{
		"charmstore-url": "https://api.jujucharms.com/charmstore",
		"metering-url":   "https://api.jujucharms.com/omnibus",
	}))
}

func (s *controllerSuite) TestModelConfig(c *gc.C) {
	conn := s.open(c, nil, "test")
	defer conn.Close()
	client := controllerapi.NewClient(conn)
	_, err := client.ModelConfig()
	c.Assert(err, gc.ErrorMatches, `permission denied \(unauthorized access\)`)
	c.Assert(jujuparams.IsCodeUnauthorized(err), gc.Equals, true)
}

func (s *controllerSuite) TestAllModels(c *gc.C) {
	ctx := context.Background()

	ctlPath := s.AssertAddController(ctx, c, params.EntityPath{User: "test", Name: "controller-1"}, true)
	s.AssertUpdateCredential(ctx, c, "test", "dummy", "cred1", "empty")
	s.AssertUpdateCredential(ctx, c, "test2", "dummy", "cred1", "empty")
	err := s.JEM.DB.SetACL(ctx, s.JEM.DB.Controllers(), ctlPath, params.ACL{
		Read: []string{"test2"},
	})

	c.Assert(err, gc.Equals, nil)

	mi := s.assertCreateModel(c, createModelParams{name: "model-1", username: "test", cred: "cred1"})
	modelUUID1 := mi.UUID
	s.assertCreateModel(c, createModelParams{name: "model-2", username: "test2", cred: "cred1"})
	mi = s.assertCreateModel(c, createModelParams{name: "model-3", username: "test2", cred: "cred1"})
	modelUUID3 := mi.UUID

	conn := s.open(c, nil, "test")
	defer conn.Close()
	client := controllerapi.NewClient(conn)

	err = s.JEM.DB.SetACL(ctx, s.JEM.DB.Models(), params.EntityPath{User: "test2", Name: "model-3"}, params.ACL{
		Read: []string{"test"},
	})

	c.Assert(err, gc.Equals, nil)

	models, err := client.AllModels()
	c.Assert(err, gc.Equals, nil)
	c.Assert(models, jc.DeepEquals, []base.UserModel{{
		Name:           "model-1",
		UUID:           modelUUID1,
		Owner:          "test@external",
		LastConnection: nil,
		Type:           "iaas",
	}, {
		Name:           "model-3",
		UUID:           modelUUID3,
		Owner:          "test2@external",
		LastConnection: nil,
		Type:           "iaas",
	}})
}

func (s *controllerSuite) TestModelStatus(c *gc.C) {
	ctx := context.Background()

	ctlPath := s.AssertAddController(ctx, c, params.EntityPath{User: "test", Name: "controller-1"}, true)
	s.AssertUpdateCredential(ctx, c, "test", "dummy", "cred1", "empty")
	s.AssertUpdateCredential(ctx, c, "test2", "dummy", "cred1", "empty")
	err := s.JEM.DB.SetACL(ctx, s.JEM.DB.Controllers(), ctlPath, params.ACL{
		Read: []string{"test2"},
	})

	c.Assert(err, gc.Equals, nil)

	mi := s.assertCreateModel(c, createModelParams{name: "model-1", username: "test", cred: "cred1"})
	modelUUID1 := mi.UUID
	mi = s.assertCreateModel(c, createModelParams{name: "model-2", username: "test2", cred: "cred1"})
	modelUUID2 := mi.UUID
	mi = s.assertCreateModel(c, createModelParams{name: "model-3", username: "test2", cred: "cred1"})
	modelUUID3 := mi.UUID

	err = s.JEM.DB.SetACL(ctx, s.JEM.DB.Models(), params.EntityPath{User: "test2", Name: "model-3"}, params.ACL{
		Read: []string{"test"},
	})

	c.Assert(err, gc.Equals, nil)

	type modelStatuser interface {
		ModelStatus(tags ...names.ModelTag) ([]base.ModelStatus, error)
	}
	doTest := func(client modelStatuser) {
		models, err := client.ModelStatus(names.NewModelTag(modelUUID1), names.NewModelTag(modelUUID3))
		c.Assert(err, gc.Equals, nil)
		c.Assert(models, jc.DeepEquals, []base.ModelStatus{{
			UUID:               modelUUID1,
			Life:               "alive",
			Owner:              "test@external",
			TotalMachineCount:  0,
			CoreCount:          0,
			HostedMachineCount: 0,
			ApplicationCount:   0,
			Machines:           []base.Machine{},
		}, {
			UUID:               modelUUID3,
			Life:               "alive",
			Owner:              "test2@external",
			TotalMachineCount:  0,
			CoreCount:          0,
			HostedMachineCount: 0,
			ApplicationCount:   0,
			Machines:           []base.Machine{},
		}})
		_, err = client.ModelStatus(names.NewModelTag(modelUUID2))
		c.Assert(err, gc.ErrorMatches, `unauthorized`)
	}

	conn := s.open(c, nil, "test")
	defer conn.Close()
	doTest(controllerapi.NewClient(conn))
	doTest(modelmanager.NewClient(conn))
}

type testHeartMonitor struct {
	c         chan time.Time
	firstBeat chan struct{}

	// mu protects the fields below.
	mu              sync.Mutex
	_beats          int
	dead            bool
	firstBeatClosed bool
}

func newTestHeartMonitor() *testHeartMonitor {
	return &testHeartMonitor{
		c:         make(chan time.Time),
		firstBeat: make(chan struct{}),
	}
}

func (m *testHeartMonitor) Heartbeat() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m._beats++
	if !m.firstBeatClosed {
		close(m.firstBeat)
		m.firstBeatClosed = true
	}

}

func (m *testHeartMonitor) Dead() <-chan time.Time {
	return m.c
}

func (m *testHeartMonitor) Stop() bool {
	return m.dead
}

func (m *testHeartMonitor) kill(t time.Time) {
	m.mu.Lock()
	m.dead = true
	m.mu.Unlock()
	m.c <- t
}

func (m *testHeartMonitor) beats() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m._beats
}

func (m *testHeartMonitor) waitForFirstPing(c *gc.C, d time.Duration) {
	select {
	case <-m.firstBeat:
	case <-time.After(d):
		c.Fatalf("timeout waiting for first ping")
	}
}

func (s *controllerSuite) TestConnectionClosesWhenHeartMonitorDies(c *gc.C) {
	hm := newTestHeartMonitor()
	s.PatchValue(jujuapi.NewHeartMonitor, jujuapi.InternalHeartMonitor(func(time.Duration) jujuapi.HeartMonitor {
		return hm
	}))
	conn := s.open(c, nil, "test")
	defer conn.Close()
	hm.kill(time.Now())
	beats := hm.beats()
	var err error
	for beats < 10 {
		time.Sleep(10 * time.Millisecond)
		err = conn.APICall("Pinger", 1, "", "Ping", nil, nil)
		if err != nil {
			break
		}
		beats++
	}
	c.Assert(err, gc.ErrorMatches, `connection is shut down`)
	c.Assert(hm.beats(), gc.Equals, beats)
}

func (s *controllerSuite) TestPingerUpdatesHeartMonitor(c *gc.C) {
	hm := newTestHeartMonitor()
	s.PatchValue(jujuapi.NewHeartMonitor, jujuapi.InternalHeartMonitor(func(time.Duration) jujuapi.HeartMonitor {
		return hm
	}))
	conn := s.open(c, nil, "test")
	defer conn.Close()
	beats := hm.beats()
	err := conn.APICall("Pinger", 1, "", "Ping", nil, nil)
	c.Assert(err, gc.Equals, nil)
	c.Assert(hm.beats(), gc.Equals, beats+1)
}

func (s *controllerSuite) TestUnauthenticatedPinger(c *gc.C) {
	hm := newTestHeartMonitor()
	s.PatchValue(jujuapi.NewHeartMonitor, jujuapi.InternalHeartMonitor(func(time.Duration) jujuapi.HeartMonitor {
		return hm
	}))
	conn := s.open(c, &api.Info{SkipLogin: true}, "test")
	defer conn.Close()
	err := conn.APICall("Pinger", 1, "", "Ping", nil, nil)
	c.Assert(err, gc.Equals, nil)
	c.Assert(hm.beats(), gc.Equals, 1)
}

func (s *controllerSuite) TestAddUser(c *gc.C) {
	conn := s.open(c, nil, "alice")
	defer conn.Close()

	client := usermanager.NewClient(conn)
	_, _, err := client.AddUser("bob", "Bob", "bob's super secret password")
	c.Assert(err, gc.ErrorMatches, `unauthorized \(unauthorized access\)`)
}

func (s *controllerSuite) TestRemoveUser(c *gc.C) {
	conn := s.open(c, nil, "alice")
	defer conn.Close()

	client := usermanager.NewClient(conn)
	err := client.RemoveUser("bob")
	c.Assert(err, gc.ErrorMatches, `unauthorized \(unauthorized access\)`)
}

func (s *controllerSuite) TestEnableUser(c *gc.C) {
	conn := s.open(c, nil, "alice")
	defer conn.Close()

	client := usermanager.NewClient(conn)
	err := client.EnableUser("bob")
	c.Assert(err, gc.ErrorMatches, `unauthorized \(unauthorized access\)`)
}

func (s *controllerSuite) TestDisableUser(c *gc.C) {
	conn := s.open(c, nil, "alice")
	defer conn.Close()

	client := usermanager.NewClient(conn)
	err := client.DisableUser("bob")
	c.Assert(err, gc.ErrorMatches, `unauthorized \(unauthorized access\)`)
}

func (s *controllerSuite) TestUserInfoAllUsers(c *gc.C) {
	conn := s.open(c, nil, "alice")
	defer conn.Close()

	client := usermanager.NewClient(conn)
	users, err := client.UserInfo(nil, usermanager.AllUsers)
	c.Assert(err, gc.Equals, nil)
	c.Assert(len(users), gc.Equals, 0)
}

func (s *controllerSuite) TestUserInfoSpecifiedUser(c *gc.C) {
	conn := s.open(c, nil, "alice")
	defer conn.Close()

	client := usermanager.NewClient(conn)
	users, err := client.UserInfo([]string{"alice@external"}, usermanager.AllUsers)
	c.Assert(err, gc.Equals, nil)
	c.Assert(len(users), gc.Equals, 1)
	c.Assert(users[0], jc.DeepEquals, jujuparams.UserInfo{
		Username:    "alice@external",
		DisplayName: "alice@external",
		Access:      "add-model",
	})
}

func (s *controllerSuite) TestUserInfoSpecifiedUsers(c *gc.C) {
	conn := s.open(c, nil, "alice")
	defer conn.Close()

	client := usermanager.NewClient(conn)
	users, err := client.UserInfo([]string{"alice@external", "bob@external"}, usermanager.AllUsers)
	c.Assert(err, gc.ErrorMatches, "bob@external: unauthorized")
	c.Assert(users, gc.HasLen, 0)
}

func (s *controllerSuite) TestUserInfoWithDomain(c *gc.C) {
	conn := s.open(c, nil, "alice@mydomain")
	defer conn.Close()

	client := usermanager.NewClient(conn)
	users, err := client.UserInfo([]string{"alice@mydomain"}, usermanager.AllUsers)
	c.Assert(err, gc.Equals, nil)
	c.Assert(len(users), gc.Equals, 1)
	c.Assert(users[0], jc.DeepEquals, jujuparams.UserInfo{
		Username:    "alice@mydomain",
		DisplayName: "alice@mydomain",
		Access:      "add-model",
	})
}

func (s *controllerSuite) TestUserInfoInvalidUsername(c *gc.C) {
	conn := s.open(c, nil, "alice")
	defer conn.Close()

	client := usermanager.NewClient(conn)
	users, err := client.UserInfo([]string{"alice-@external"}, usermanager.AllUsers)
	c.Assert(err, gc.ErrorMatches, `"alice-@external" is not a valid username`)
	c.Assert(users, gc.HasLen, 0)
}

func (s *controllerSuite) TestUserInfoLocalUsername(c *gc.C) {
	conn := s.open(c, nil, "alice")
	defer conn.Close()

	client := usermanager.NewClient(conn)
	users, err := client.UserInfo([]string{"alice"}, usermanager.AllUsers)
	c.Assert(err, gc.ErrorMatches, `alice: unsupported local user`)
	c.Assert(users, gc.HasLen, 0)
}

func (s *controllerSuite) TestSetPassword(c *gc.C) {
	conn := s.open(c, nil, "alice")
	defer conn.Close()

	client := usermanager.NewClient(conn)
	err := client.SetPassword("bob", "bob's new super secret password")
	c.Assert(err, gc.ErrorMatches, `unauthorized \(unauthorized access\)`)
}

func (s *controllerSuite) TestWatchModelSummaries(c *gc.C) {
	ctx := context.Background()

	ctlPath := s.AssertAddController(ctx, c, params.EntityPath{User: "test", Name: "controller-1"}, true)
	s.AssertUpdateCredential(ctx, c, "test", "dummy", "cred1", "empty")
	s.AssertUpdateCredential(ctx, c, "test2", "dummy", "cred1", "empty")
	err := s.JEM.DB.SetACL(ctx, s.JEM.DB.Controllers(), ctlPath, params.ACL{
		Read: []string{"test2"},
	})

	c.Assert(err, gc.Equals, nil)

	mi := s.assertCreateModel(c, createModelParams{name: "model-1", username: "test", cred: "cred1"})
	modelUUID1 := mi.UUID
	s.assertCreateModel(c, createModelParams{name: "model-2", username: "test2", cred: "cred1"})
	mi = s.assertCreateModel(c, createModelParams{name: "model-3", username: "test2", cred: "cred1"})
	modelUUID3 := mi.UUID
	c.Logf("models: %v %v", modelUUID1, modelUUID3)

	err = s.JEM.DB.SetACL(ctx, s.JEM.DB.Models(), params.EntityPath{User: "test2", Name: "model-3"}, params.ACL{
		Read: []string{"test"},
	})
	c.Assert(err, gc.Equals, nil)

	done := s.Pubsub.Publish(modelUUID1, jujuparams.ModelAbstract{
		UUID:  modelUUID1,
		Cloud: "test-cloud",
		Name:  "test-name-1",
	})
	select {
	case <-done:
	case <-time.After(time.Second):
		c.Fatalf("timed out")
	}
	done = s.Pubsub.Publish(modelUUID3, jujuparams.ModelAbstract{
		UUID:  modelUUID3,
		Cloud: "test-cloud",
		Name:  "test-name-3",
	})
	select {
	case <-done:
	case <-time.After(time.Second):
		c.Fatalf("timed out")
	}

	expectedModels := []jujuparams.ModelAbstract{{
		UUID:  modelUUID1,
		Cloud: "test-cloud",
		Name:  "test-name-1",
	}, {
		UUID:  modelUUID3,
		Cloud: "test-cloud",
		Name:  "test-name-3",
	}}
	sort.Slice(expectedModels, func(i, j int) bool {
		return expectedModels[i].UUID < expectedModels[j].UUID
	})

	conn := s.open(c, nil, "test")
	defer conn.Close()

	var watcherID jujuparams.SummaryWatcherID
	err = conn.APICall("Controller", 9, "", "WatchModelSummaries", nil, &watcherID)
	c.Assert(err, jc.ErrorIsNil)

	var summaries jujuparams.SummaryWatcherNextResults
	err = conn.APICall("ModelSummaryWatcher", 1, watcherID.WatcherID, "Next", nil, &summaries)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(summaries.Models, gc.DeepEquals, expectedModels)

	err = conn.APICall("ModelSummaryWatcher", 1, watcherID.WatcherID, "Stop", nil, nil)
	c.Assert(err, jc.ErrorIsNil)

	err = conn.APICall("ModelSummaryWatcher", 1, "unknown-id", "Next", nil, &summaries)
	c.Assert(err, gc.ErrorMatches, `not found \(not found\)`)
}

func (s *controllerSuite) TestMongoVersion(c *gc.C) {
	ctx := context.Background()

	s.AssertAddController(ctx, c, params.EntityPath{User: "test", Name: "controller-1"}, true)
	s.AssertUpdateCredential(ctx, c, "test", "dummy", "cred1", "empty")

	conn := s.open(c, nil, "test")
	defer conn.Close()

	var version jujuparams.StringResult
	err := conn.APICall("Controller", 6, "", "MongoVersion", nil, &version)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(version.Result, gc.Not(gc.Equals), "")

	err = conn.APICall("Controller", 9, "", "MongoVersion", nil, &version)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(version.Result, gc.Not(gc.Equals), "")
}

func (s *controllerSuite) TestConfigSet(c *gc.C) {
	ctx := context.Background()

	s.AssertAddController(ctx, c, params.EntityPath{User: "test", Name: "controller-1"}, true)
	s.AssertUpdateCredential(ctx, c, "test", "dummy", "cred1", "empty")

	conn := s.open(c, nil, "test")
	defer conn.Close()

	err := conn.APICall("Controller", 5, "", "ConfigSet", jujuparams.ControllerConfigSet{}, nil)
	c.Assert(err, jc.ErrorIsNil)

	err = conn.APICall("Controller", 9, "", "ConfigSet", jujuparams.ControllerConfigSet{}, nil)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *controllerSuite) TestIdentityProviderURL(c *gc.C) {
	ctx := context.Background()

	s.AssertAddController(ctx, c, params.EntityPath{User: "test", Name: "controller-1"}, true)
	s.AssertUpdateCredential(ctx, c, "test", "dummy", "cred1", "empty")

	conn := s.open(c, nil, "test")
	defer conn.Close()

	var result jujuparams.StringResult
	err := conn.APICall("Controller", 7, "", "IdentityProviderURL", nil, &result)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Result, gc.Matches, `https://127\.0\.0\.1.*`)

	err = conn.APICall("Controller", 9, "", "IdentityProviderURL", nil, &result)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Result, gc.Matches, `https://127\.0\.0\.1.*`)
}

func (s *controllerSuite) TestControllerVersion(c *gc.C) {
	ctx := context.Background()

	s.AssertAddController(ctx, c, params.EntityPath{User: "test", Name: "controller-1"}, true)
	s.AssertUpdateCredential(ctx, c, "test", "dummy", "cred1", "empty")

	conn := s.open(c, nil, "test")
	defer conn.Close()

	var result jujuparams.ControllerVersionResults
	err := conn.APICall("Controller", 8, "", "ControllerVersion", nil, &result)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, jujuparams.ControllerVersionResults{
		Version:   "0.0.0",
		GitCommit: jimmversion.VersionInfo.GitCommit,
	})

	err = conn.APICall("Controller", 9, "", "ControllerVersion", nil, &result)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, jujuparams.ControllerVersionResults{
		Version:   "0.0.0",
		GitCommit: jimmversion.VersionInfo.GitCommit,
	})
}

func assertModelInfo(c *gc.C, obtained, expected []jujuparams.ModelInfoResult) {
	for i := range obtained {
		// DefaultSeries changes between juju versions and
		// we don't care about its specific value.
		if obtained[i].Result != nil {
			obtained[i].Result.DefaultSeries = ""
		}
	}
	for i := range obtained {
		if obtained[i].Result == nil {
			continue
		}
		obtained[i].Result.Status.Since = nil
		for j := range obtained[i].Result.Users {
			obtained[i].Result.Users[j].LastConnection = nil
		}
	}
	c.Assert(obtained, jc.DeepEquals, expected)
}

func newBool(b bool) *bool {
	return &b
}
