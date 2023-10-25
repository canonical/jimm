// Copyright 2015 Canonical Ltd.

package admincmd

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/canonical/jimm/internal/jemtest"
	"github.com/canonical/jimm/params"
)

type internalSuite struct {
	jemtest.JujuConnSuite
}

var _ = gc.Suite(&internalSuite{})

var entityPathValueTests = []struct {
	about            string
	val              string
	expectEntityPath params.EntityPath
	expectError      string
}{{
	about: "success",
	val:   "foo/bar",
	expectEntityPath: params.EntityPath{
		User: "foo",
		Name: "bar",
	},
}, {
	about:       "only one part",
	val:         "a",
	expectError: `invalid entity path "a": need <user>/<name>`,
}, {
	about:       "too many parts",
	val:         "a/b/c",
	expectError: `invalid entity path "a/b/c": need <user>/<name>`,
}, {
	about:       "empty string",
	val:         "",
	expectError: `invalid entity path "": need <user>/<name>`,
}}

func (s *internalSuite) TestEntityPathValue(c *gc.C) {
	for i, test := range entityPathValueTests {
		c.Logf("test %d: %s", i, test.about)
		var p entityPathValue
		err := p.Set(test.val)
		if test.expectError != "" {
			c.Assert(err, gc.ErrorMatches, test.expectError)
			continue
		}
		c.Assert(err, gc.Equals, nil)
		c.Assert(p.EntityPath, gc.Equals, test.expectEntityPath)
	}
}

var entityPathsValueTests = []struct {
	about             string
	val               string
	expectEntityPaths []params.EntityPath
	expectError       string
}{{
	about: "success",
	val:   "foo/bar,baz/arble",
	expectEntityPaths: []params.EntityPath{{
		User: "foo",
		Name: "bar",
	}, {
		User: "baz",
		Name: "arble",
	}},
}, {
	about:       "no paths",
	val:         "",
	expectError: `empty entity paths`,
}, {
	about:       "invalid entry",
	val:         "a/b/c,foo/bar",
	expectError: `invalid entity path "a/b/c": need <user>/<name>`,
}}

func (s *internalSuite) TestEntityPathsValue(c *gc.C) {
	for i, test := range entityPathsValueTests {
		c.Logf("test %d: %s", i, test.about)
		var p entityPathsValue
		err := p.Set(test.val)
		if test.expectError != "" {
			c.Assert(err, gc.ErrorMatches, test.expectError)
			continue
		}
		c.Assert(err, gc.Equals, nil)
		c.Assert(p.paths, jc.DeepEquals, test.expectEntityPaths)
	}
}
