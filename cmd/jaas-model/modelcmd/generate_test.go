// Copyright 2015 Canonical Ltd.

package modelcmd_test

import (
	"io/ioutil"
	"path/filepath"

	gc "gopkg.in/check.v1"
	"gopkg.in/juju/environschema.v1"

	"github.com/CanonicalLtd/jem/params"
)

type generateSuite struct {
	commonSuite
}

var _ = gc.Suite(&generateSuite{})

func (s *generateSuite) TestGenerate(c *gc.C) {
	s.idmSrv.SetDefaultUser("bob")

	// First add the controller that we're going to use
	// to generate the config data.
	stdout, stderr, code := run(c, c.MkDir(), "add-controller", "bob/foo")
	c.Assert(code, gc.Equals, 0, gc.Commentf("stderr: %s", stderr))
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Equals, "")

	stdout, stderr, code = run(c, c.MkDir(), "generate", "-c", "bob/foo")
	c.Assert(code, gc.Equals, 0, gc.Commentf("stderr: %s", stderr))
	c.Assert(stderr, gc.Equals, "")

	// Just smoke-test to see that the output looks reasonable, without
	// relying on the entire 250 line output.
	c.Assert(stdout, gc.Matches,
		`(.|\n)*# Whitespace-separated Environ methods that should return an error when called
#
# broken: ""
(.|\n)*
# Whether the model should start a controller
#
# controller: false
(.|\n)*`)
	configData := stdout

	// It should have omitted all juju-group attributes,
	// so check one to be sure.
	client := s.jemClient("bob")
	controllerInfo, err := client.GetController(&params.GetController{
		EntityPath: params.EntityPath{
			User: "bob",
			Name: "foo",
		},
	})
	c.Assert(err, gc.IsNil)

	field, ok := controllerInfo.Schema["agent-version"]
	c.Assert(ok, gc.Equals, true)
	c.Assert(field.Group, gc.Equals, environschema.JujuGroup)

	c.Assert(stdout, gc.Not(gc.Matches), `(.|\n)*agent-version(.|\n)*`)

	// Check that generating to a file generates the same thing.
	file := filepath.Join(c.MkDir(), "config.yaml")
	stdout, stderr, code = run(c, c.MkDir(), "generate", "-c", "bob/foo", "-o", file)
	c.Assert(code, gc.Equals, 0, gc.Commentf("stderr: %s", stderr))
	c.Assert(stderr, gc.Equals, "")
	c.Assert(stdout, gc.Equals, "")

	data, err := ioutil.ReadFile(file)
	c.Assert(err, gc.IsNil)
	c.Assert(string(data), gc.Equals, configData)
}

func (s *generateSuite) TestGenerateWithTemplates(c *gc.C) {
	s.idmSrv.SetDefaultUser("bob")

	// First add the controller that we're going to use
	// to generate the config data.
	stdout, stderr, code := run(c, c.MkDir(), "add-controller", "bob/foo")
	c.Assert(code, gc.Equals, 0, gc.Commentf("stderr: %s", stderr))
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Equals, "")

	stdout, stderr, code = run(c, c.MkDir(), "create-template", "--controller", "bob/foo", "bob/template", "controller=false")
	c.Assert(code, gc.Equals, 0, gc.Commentf("stderr: %s", stderr))
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Equals, "")

	stdout, stderr, code = run(c, c.MkDir(), "generate", "-c", "bob/foo", "-t", "bob/template")
	c.Assert(code, gc.Equals, 0, gc.Commentf("stderr: %s", stderr))
	c.Assert(stderr, gc.Equals, "")

	// The controller attribute should be omitted.
	c.Assert(stdout, gc.Not(gc.Matches), `(.|\n)*controller:(.|\n)*`)

	// But the other attributes should still be there.
	c.Assert(stdout, gc.Matches, `(.|\n)*broken:(.|\n)*`)
}

var generateErrorTests = []struct {
	about        string
	args         []string
	expectStderr string
	expectCode   int
}{{
	about:        "too many arguments",
	args:         []string{"a"},
	expectStderr: "arguments provided but none expected",
	expectCode:   2,
}, {
	about:        "controller must be specified",
	expectStderr: `controller must be specified`,
	expectCode:   2,
}}

func (s *generateSuite) TestGenerateError(c *gc.C) {
	for i, test := range generateErrorTests {
		c.Logf("test %d: %s", i, test.about)
		stdout, stderr, code := run(c, c.MkDir(), "generate", test.args...)
		c.Assert(code, gc.Equals, test.expectCode, gc.Commentf("stderr: %s", stderr))
		c.Assert(stderr, gc.Matches, "(error:|ERROR) "+test.expectStderr+"\n")
		c.Assert(stdout, gc.Equals, "")
	}
}
