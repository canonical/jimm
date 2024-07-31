// Copyright 2016 Canonical Ltd.

package jujuapi_test

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/client/client"
	"github.com/juju/names/v5"
	gc "gopkg.in/check.v1"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/jujuapi"
	"github.com/canonical/jimm/v3/internal/openfga"
	ofganames "github.com/canonical/jimm/v3/internal/openfga/names"
)

type apiProxySuite struct {
	websocketSuite
}

var _ = gc.Suite(&apiProxySuite{})

func (s *apiProxySuite) TestConnectToModel(c *gc.C) {
	conn := s.open(c, &api.Info{
		ModelTag:  s.Model.ResourceTag(),
		SkipLogin: true,
	}, "test")
	defer conn.Close()
	var resp map[string]interface{}
	err := conn.APICall("Admin", 3, "", "TestMethod", nil, &resp)
	c.Assert(err, gc.ErrorMatches, `no such request - method Admin.TestMethod is not implemented \(not implemented\)`)
}

func (s *apiProxySuite) TestSessionTokenLoginProvider(c *gc.C) {
	ctx := context.Background()
	alice := names.NewUserTag("alice@canonical.com")
	aliceUser := openfga.NewUser(&dbmodel.Identity{Name: alice.Id()}, s.JIMM.OpenFGAClient)
	err := aliceUser.SetControllerAccess(ctx, s.Model.Controller.ResourceTag(), ofganames.AdministratorRelation)
	c.Assert(err, gc.IsNil)
	var output bytes.Buffer
	s.JIMMSuite.EnableDeviceFlow(aliceUser.Name)
	conn, err := s.openCustomLoginProvider(c, &api.Info{
		ModelTag:  s.Model.ResourceTag(),
		SkipLogin: false,
	}, "alice", api.NewSessionTokenLoginProvider("", &output, func(s string) error { return nil }))
	c.Assert(err, gc.IsNil)
	defer conn.Close()
	c.Check(err, gc.Equals, nil)
	outputNoNewLine := strings.Replace(output.String(), "\n", "", -1)
	c.Check(outputNoNewLine, gc.Matches, `Please visit .* and enter code.*`)
}

type logger struct{}

func (l logger) Errorf(string, ...interface{}) {}

func (s *apiProxySuite) TestModelStatus(c *gc.C) {
	conn := s.open(c, &api.Info{
		ModelTag:  s.Model.ResourceTag(),
		SkipLogin: false,
	}, "alice@canonical.com")
	defer conn.Close()
	jujuClient := client.NewClient(conn, logger{})
	status, err := jujuClient.Status(nil)
	c.Check(err, gc.IsNil)
	c.Check(status, gc.Not(gc.IsNil))
	c.Check(status.Model.Name, gc.Equals, s.Model.Name)
}

func (s *apiProxySuite) TestModelStatusWithoutPermission(c *gc.C) {
	fooUser := openfga.NewUser(&dbmodel.Identity{Name: "foo@canonical.com"}, s.JIMM.OpenFGAClient)
	var output bytes.Buffer
	s.JIMMSuite.EnableDeviceFlow(fooUser.Name)
	conn, err := s.openCustomLoginProvider(c, &api.Info{
		ModelTag:  s.Model.ResourceTag(),
		SkipLogin: false,
	}, "foo", api.NewSessionTokenLoginProvider("", &output, func(s string) error { return nil }))
	c.Check(err, gc.ErrorMatches, "permission denied .*")
	if conn != nil {
		defer conn.Close()
	}
	outputNoNewLine := strings.Replace(output.String(), "\n", "", -1)
	c.Check(outputNoNewLine, gc.Matches, `Please visit .* and enter code.*`)
}

// TODO(Kian): This test aims to verify that JIMM gracefully handles clients that end their connection
// during the login flow after JIMM starts polling the OIDC server.
// After https://github.com/juju/juju/pull/17606 lands we can begin work on this.
// The API connection's login method should be refactored use the login provider stored on the state struct.
// func (s *apiProxySuite) TestDeviceFlowCancelDuringPolling(c *gc.C) {
// 	ctx := context.Background()
// 	alice := names.NewUserTag("alice@canonical.com")
// 	aliceUser := openfga.NewUser(&dbmodel.Identity{Name: alice.Id()}, s.JIMM.OpenFGAClient)
// 	err := aliceUser.SetControllerAccess(ctx, s.Model.Controller.ResourceTag(), ofganames.AdministratorRelation)
// 	c.Assert(err, gc.IsNil)
// 	var cliOutput string
// 	_ = cliOutput
// 	outputFunc := func(format string, a ...any) error {
// 		cliOutput = fmt.Sprintf(format, a)
// 		return nil
// 	}
// 	var wg sync.WaitGroup
// 	var conn api.Connection
// 	wg.Add(1)
// 	go func() {
// 		defer wg.Done()
// 		conn, err = s.openCustomLP(c, &api.Info{
// 			ModelTag:  s.Model.ResourceTag(),
// 			SkipLogin: true,
// 		}, "alice", api.NewSessionTokenLoginProvider("", outputFunc, func(s string) error { return nil }))
// 		c.Assert(err, gc.IsNil)
// 	}()
// 	conn.Login()
//  // Close the connection after the cliOutput is filled.
// 	c.Assert(err, gc.Equals, nil)
// }

// TODO(CSS-7331) Add more tests for model proxy and new login methods.

type pathTestSuite struct{}

var _ = gc.Suite(&pathTestSuite{})

func (s *pathTestSuite) Test(c *gc.C) {

	testUUID := "059744f6-26d2-4f00-92be-5df97fccbb97"
	tests := []struct {
		path      string
		uuid      string
		finalPath string
		fail      bool
	}{
		{path: fmt.Sprintf("/%s/api", testUUID), uuid: testUUID, finalPath: "api", fail: false},
		{path: fmt.Sprintf("/%s/api/", testUUID), uuid: testUUID, finalPath: "api/", fail: false},
		{path: fmt.Sprintf("/%s/api/foo", testUUID), uuid: testUUID, finalPath: "api/foo", fail: false},
		{path: fmt.Sprintf("/%s/commands", testUUID), uuid: testUUID, finalPath: "commands", fail: false},
		{path: fmt.Sprintf("%s/commands", testUUID), fail: true},
		{path: fmt.Sprintf("/model/%s/commands", testUUID), fail: true},
		{path: "/model/123/commands", fail: true},
		{path: fmt.Sprintf("/controller/%s/commands", testUUID), fail: true},
		{path: fmt.Sprintf("/controller/%s/", testUUID), fail: true},
		{path: "/controller", fail: true},
	}
	for i, test := range tests {
		c.Logf("Running test %d for path %s", i, test.path)
		uuid, finalPath, err := jujuapi.ModelInfoFromPath(test.path)
		if !test.fail {
			c.Assert(err, gc.IsNil)
			c.Assert(uuid, gc.Equals, test.uuid)
			c.Assert(finalPath, gc.Equals, test.finalPath)
		} else {
			c.Assert(err, gc.NotNil)
		}
	}
}
