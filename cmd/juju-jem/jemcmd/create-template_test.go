// Copyright 2015 Canonical Ltd.

package jemcmd_test

import (
	"fmt"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/errgo.v1"
	"gopkg.in/juju/environschema.v1"
	"gopkg.in/juju/environschema.v1/form"

	"github.com/CanonicalLtd/jem/cmd/juju-jem/jemcmd"
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

	// First add the state server that we're going to use
	// to create the new environment.
	stdout, stderr, code := run(c, c.MkDir(), "add-server", "bob/foo")
	c.Assert(code, gc.Equals, 0, gc.Commentf("stderr: %s", stderr))
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Equals, "")

	// Then verify that the template does not already exist.
	_, err := client.GetTemplate(&params.GetTemplate{
		EntityPath: params.EntityPath{"bob", "foo"},
	})
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)

	stdout, stderr, code = run(c, c.MkDir(), "create-template", "--server", "bob/foo", "bob/mytemplate", "state-server=true", "apt-mirror=0.1.2.3")
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

func (s *createTemplateSuite) TestInteractive(c *gc.C) {
	jemcmd.PatchProviderDefaults(s, map[string]map[string]func() (interface{}, error){
		"dummy": {
			"testattr": func() (interface{}, error) {
				return "testattr-default-value", nil
			},
		},
	})
	s.PatchValue(jemcmd.IOFillerFill, func(filler form.IOFiller, f form.Form) (map[string]interface{}, error) {
		// Verify that the filler values look reasonable without actually
		// invoking the actual IO filler (well tested elsewhere).
		// This saves us from depending on the actual dummy provider
		// attributes.
		c.Assert(filler.In, gc.Equals, emptyReader{})
		fmt.Fprint(filler.Out, "test output")
		c.Assert(filler.ShowDescriptions, gc.Equals, true)

		for name, field := range f.Fields {
			c.Assert(field.Mandatory, gc.Equals, false, gc.Commentf("field %s", name))
		}
		// Ensure that the GetDefault machinery is hooked up
		// by calling GetDefault and ensuring that it fetches
		// the value from the provider defaults.
		attr := form.NamedAttr{
			Name: "testattr",
			Attr: environschema.Attr{
				Type: environschema.Tstring,
			},
		}
		checker, err := attr.Checker()
		c.Assert(err, gc.IsNil)
		val, display, err := filler.GetDefault(attr, checker)
		c.Assert(err, gc.IsNil)
		c.Assert(display, gc.Equals, "")
		c.Assert(val, gc.Equals, "testattr-default-value")
		return map[string]interface{}{
			"state-server": true,
		}, nil
	})
	s.idmSrv.AddUser("bob")
	s.idmSrv.SetDefaultUser("bob")
	client := s.jemClient("bob")

	// First add the state server that we're going to use
	// to create the new environment.
	stdout, stderr, code := run(c, c.MkDir(), "add-server", "bob/foo")
	c.Assert(code, gc.Equals, 0, gc.Commentf("stderr: %s", stderr))
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Equals, "")

	stdout, stderr, code = run(c, c.MkDir(), "create-template", "--server", "bob/foo", "-i", "bob/mytemplate")
	c.Assert(code, gc.Equals, 0, gc.Commentf("stderr: %s", stderr))
	c.Assert(stdout, gc.Equals, "test output")
	c.Assert(stderr, gc.Equals, "")
	tmpl, err := client.GetTemplate(&params.GetTemplate{
		EntityPath: params.EntityPath{"bob", "mytemplate"},
	})
	c.Assert(err, gc.IsNil)
	c.Assert(tmpl.Config, jc.DeepEquals, map[string]interface{}{
		"state-server": true,
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
	about:        "state server not provided",
	args:         []string{"bob/foo"},
	expectStderr: `--server flag required but not provided`,
	expectCode:   2,
}, {
	about:        "duplicate key",
	args:         []string{"bob/foo", "--server", "foo/bar", "x=y", "y=z", "x=p"},
	expectStderr: `key "x" specified more than once`,
	expectCode:   2,
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
