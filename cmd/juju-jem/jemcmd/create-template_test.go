// Copyright 2015 Canonical Ltd.

package jemcmd_test

import (
	"io/ioutil"
	"path/filepath"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/errgo.v1"
	"gopkg.in/yaml.v2"

	"github.com/CanonicalLtd/jem/params"
)

type createTemplateSuite struct {
	commonSuite
}

var _ = gc.Suite(&createTemplateSuite{})

func (s *createTemplateSuite) TestCreateTemplate(c *gc.C) {
	s.idmSrv.AddUser("bob")
	s.idmSrv.SetDefaultUser("bob")
	client := s.jemClient("bob")

	// First add the controller that we're going to use
	// to create the new environment.
	stdout, stderr, code := run(c, c.MkDir(), "add-controller", "bob/foo")
	c.Assert(code, gc.Equals, 0, gc.Commentf("stderr: %s", stderr))
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Equals, "")

	// Then verify that the template does not already exist.
	_, err := client.GetTemplate(&params.GetTemplate{
		EntityPath: params.EntityPath{"bob", "foo"},
	})
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)

	stdout, stderr, code = run(c, c.MkDir(), "create-template", "--controller", "bob/foo", "bob/mytemplate", "state-server=true", "apt-mirror=0.1.2.3")
	c.Assert(code, gc.Equals, 0, gc.Commentf("stderr: %s", stderr))
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Equals, "")
	tmpl, err := client.GetTemplate(&params.GetTemplate{
		EntityPath: params.EntityPath{"bob", "mytemplate"},
	})
	c.Assert(err, gc.IsNil)
	c.Assert(tmpl.Config, jc.DeepEquals, map[string]interface{}{
		"state-server": true,
		"apt-mirror":   "0.1.2.3",
	})
}

func (s *createTemplateSuite) TestCreateTemplateWithConfigFile(c *gc.C) {
	s.idmSrv.AddUser("bob")
	s.idmSrv.SetDefaultUser("bob")
	client := s.jemClient("bob")

	// First add the controller that we're going to use
	// to create the new environment.
	stdout, stderr, code := run(c, c.MkDir(), "add-controller", "bob/foo")
	c.Assert(code, gc.Equals, 0, gc.Commentf("stderr: %s", stderr))
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Equals, "")

	// Then verify that the template does not already exist.
	_, err := client.GetTemplate(&params.GetTemplate{
		EntityPath: params.EntityPath{"bob", "foo"},
	})
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)

	dir := c.MkDir()
	configFile := filepath.Join(dir, "test.config")

	data, err := yaml.Marshal(map[string]interface{}{
		"state-server": true,
		"apt-mirror":   "0.1.2.3",
	})
	c.Assert(err, gc.IsNil)
	err = ioutil.WriteFile(configFile, data, 0666)
	c.Assert(err, gc.IsNil)

	stdout, stderr, code = run(c, c.MkDir(), "create-template", "--controller", "bob/foo", "bob/mytemplate", "--config", configFile)
	c.Assert(code, gc.Equals, 0, gc.Commentf("stderr: %s", stderr))
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Equals, "")
	tmpl, err := client.GetTemplate(&params.GetTemplate{
		EntityPath: params.EntityPath{"bob", "mytemplate"},
	})
	c.Assert(err, gc.IsNil)
	c.Assert(tmpl.Config, jc.DeepEquals, map[string]interface{}{
		"state-server": true,
		"apt-mirror":   "0.1.2.3",
	})
}

var createTemplateErrorTests = []struct {
	about        string
	args         []string
	expectStderr string
	expectCode   int
}{{
	about:        "too few arguments",
	args:         []string{},
	expectStderr: "got 0 arguments, want at least 1",
	expectCode:   2,
}, {
	about:        "invalid template name",
	args:         []string{"a"},
	expectStderr: `invalid entity path "a": wrong number of parts in entity path`,
	expectCode:   2,
}, {
	about:        "controller not provided",
	args:         []string{"bob/foo"},
	expectStderr: `--controller flag required but not provided`,
	expectCode:   2,
}, {
	about:        "duplicate key",
	args:         []string{"bob/foo", "--controller", "foo/bar", "x=y", "y=z", "x=p"},
	expectStderr: `key "x" specified more than once`,
	expectCode:   2,
}, {
	about:        "config file not founD",
	args:         []string{"bob/foo", "--controller", "foo/bar", "--config", "non-existent"},
	expectStderr: `cannot read configuration file: open non-existent: no such file or directory`,
	expectCode:   1,
}}

func (s *createTemplateSuite) TestCreateTemplateError(c *gc.C) {
	for i, test := range createTemplateErrorTests {
		c.Logf("test %d: %s", i, test.about)
		stdout, stderr, code := run(c, c.MkDir(), "create-template", test.args...)
		c.Assert(code, gc.Equals, test.expectCode, gc.Commentf("stderr: %s", stderr))
		c.Assert(stderr, gc.Matches, "(error:|ERROR) "+test.expectStderr+"\n")
		c.Assert(stdout, gc.Equals, "")
	}
}
