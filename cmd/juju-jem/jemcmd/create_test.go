// Copyright 2015 Canonical Ltd.

package jemcmd_test

import (
	"io/ioutil"
	"path/filepath"

	"github.com/juju/juju/juju"
	gc "gopkg.in/check.v1"
	"gopkg.in/macaroon-bakery.v1/httpbakery"
	"gopkg.in/yaml.v1"
)

type createSuite struct {
	commonSuite
}

var _ = gc.Suite(&createSuite{})

func (s *createSuite) TestCreate(c *gc.C) {
	s.idmSrv.SetDefaultUser("bob")

	// First add the controller that we're going to use
	// to create the new model.
	stdout, stderr, code := run(c, c.MkDir(), "add-controller", "bob/foo")
	c.Assert(code, gc.Equals, 0, gc.Commentf("stderr: %s", stderr))
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Equals, "")

	config := map[string]interface{}{
		"authorized-keys": fakeSSHKey,
		"state-server":    true,
	}
	data, err := yaml.Marshal(config)
	c.Assert(err, gc.IsNil)
	configPath := filepath.Join(c.MkDir(), "config.yaml")
	err = ioutil.WriteFile(configPath, data, 0666)
	c.Assert(err, gc.IsNil)
	stdout, stderr, code = run(c, c.MkDir(),
		"create",
		"-c", "bob/foo",
		"--config", configPath,
		"bob/newmodel",
	)
	c.Assert(code, gc.Equals, 0, gc.Commentf("stderr: %s", stderr))
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Equals, "")

	// Check that we can attach to the new model
	// through the usual juju connection mechanism.
	client, err := juju.NewAPIClientFromName("newmodel", httpbakery.NewClient())
	c.Assert(err, gc.IsNil)
	client.Close()
}

func (s *createSuite) TestCreateWithTemplate(c *gc.C) {
	s.idmSrv.SetDefaultUser("bob")

	// First add the controller that we're going to use
	// to create the new model.
	stdout, stderr, code := run(c, c.MkDir(), "add-controller", "bob/foo")
	c.Assert(code, gc.Equals, 0, gc.Commentf("stderr: %s", stderr))
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Equals, "")

	// Then add a template containing the mandatory controller parameter.
	stdout, stderr, code = run(c, c.MkDir(), "create-template", "bob/template", "-c", "bob/foo", "state-server=true")
	c.Assert(code, gc.Equals, 0, gc.Commentf("stderr: %s", stderr))
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Equals, "")

	// Then create an model that uses the template as additional config.
	// Note that because the controller attribute is mandatory, this
	// will fail if the template logic is not working correctly.
	config := map[string]interface{}{
		"authorized-keys": fakeSSHKey,
	}
	data, err := yaml.Marshal(config)
	c.Assert(err, gc.IsNil)
	configPath := filepath.Join(c.MkDir(), "config.yaml")
	err = ioutil.WriteFile(configPath, data, 0666)
	c.Assert(err, gc.IsNil)
	stdout, stderr, code = run(c, c.MkDir(),
		"create",
		"-c", "bob/foo",
		"--config", configPath,
		"-t", "bob/template",
		"bob/newmodel",
	)
	c.Assert(code, gc.Equals, 0, gc.Commentf("stderr: %s", stderr))
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Equals, "")

	// Check that we can attach to the new model
	// through the usual juju connection mechanism.
	client, err := juju.NewAPIClientFromName("newmodel", httpbakery.NewClient())
	c.Assert(err, gc.IsNil)
	client.Close()
}

var createErrorTests = []struct {
	about        string
	args         []string
	expectStderr string
	expectCode   int
}{{
	about:        "too few arguments",
	args:         []string{},
	expectStderr: "got 0 arguments, want 1",
	expectCode:   2,
}, {
	about:        "too many arguments",
	args:         []string{"a", "b", "c"},
	expectStderr: "got 3 arguments, want 1",
	expectCode:   2,
}, {
	about:        "only one part in model id",
	args:         []string{"a"},
	expectStderr: `invalid entity path "a": wrong number of parts in entity path`,
	expectCode:   2,
}, {
	about:        "controller must be specified",
	args:         []string{"foo/bar"},
	expectStderr: `controller must be specified`,
	expectCode:   2,
}}

func (s *createSuite) TestCreateError(c *gc.C) {
	for i, test := range createErrorTests {
		c.Logf("test %d: %s", i, test.about)
		stdout, stderr, code := run(c, c.MkDir(), "create", test.args...)
		c.Assert(code, gc.Equals, test.expectCode, gc.Commentf("stderr: %s", stderr))
		c.Assert(stderr, gc.Matches, "(error:|ERROR) "+test.expectStderr+"\n")
		c.Assert(stdout, gc.Equals, "")
	}
}
