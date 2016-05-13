// Copyright 2016 Canonical Ltd.

package jemcmd_test

import (
	"strings"

	gc "gopkg.in/check.v1"
)

type locationsSuite struct {
	commonSuite
}

var _ = gc.Suite(&locationsSuite{})

var locationsErrorTests = []struct {
	about        string
	args         []string
	expectStderr string
	expectCode   int
}{{
	about:        "invalid key-value pair argument",
	args:         []string{"bad"},
	expectStderr: `expected "key=value", got "bad"`,
	expectCode:   2,
}}

func (s *locationsSuite) TestError(c *gc.C) {
	for i, test := range locationsErrorTests {
		c.Logf("test %d: %s", i, test.about)
		stdout, stderr, code := run(c, c.MkDir(), "locations", test.args...)
		c.Assert(code, gc.Equals, test.expectCode, gc.Commentf("stderr: %s", stderr))
		c.Assert(stderr, gc.Matches, "(error:|ERROR) "+test.expectStderr+"\n")
		c.Assert(stdout, gc.Equals, "")
	}
}

func (s *locationsSuite) TestSuccess(c *gc.C) {
	s.idmSrv.SetDefaultUser("bob")
	s.idmSrv.AddUser("bob", "admin")

	addController := func(name string, attrs ...string) {
		args := append([]string{name}, attrs...)
		stdout, stderr, code := run(c, c.MkDir(), "add-controller", args...)
		c.Assert(code, gc.Equals, 0, gc.Commentf("stderr: %s", stderr))
		c.Assert(stdout, gc.Equals, "")
		c.Assert(stderr, gc.Equals, "")
	}

	// Add some controllers.
	addController("bob/c1", "--public", "cloud=aws", "region=us-east-1")
	addController("bob/c2", "--public", "cloud=aws", "region=us-east-1")
	addController("bob/c3", "--public", "cloud=aws", "region=us-east-1", "staging=true")
	addController("bob/c4", "--public", "cloud=aws", "region=eu-west-1")
	addController("bob/c5", "--public", "cloud=aws", "region=eu-west-1")
	addController("bob/c6", "--public", "cloud=azure", "region=somewhere")

	stdout, stderr, code := run(c, c.MkDir(), "locations")
	c.Assert(code, gc.Equals, 0, gc.Commentf("stderr: %s", stderr))
	c.Assert(stderr, gc.Equals, "")

	c.Assert(sanitizeTable(stdout), gc.Equals, `
CLOUD REGION STAGING
aws eu-west-1
aws us-east-1
aws us-east-1 true
azure somewhere
`[1:])

	// Check it works with filters.
	stdout, stderr, code = run(c, c.MkDir(), "locations", "cloud=azure")
	c.Assert(code, gc.Equals, 0, gc.Commentf("stderr: %s", stderr))
	c.Assert(stderr, gc.Equals, "")
	c.Assert(sanitizeTable(stdout), gc.Equals, `
CLOUD REGION
azure somewhere
`[1:])

	// Check it's ok with a filter that doesn't match anything.
	stdout, stderr, code = run(c, c.MkDir(), "locations", "cloud=erewhon")
	c.Assert(code, gc.Equals, 0, gc.Commentf("stderr: %s", stderr))
	c.Assert(stderr, gc.Equals, "")
	c.Assert(stdout, gc.Equals, "")
}

// sanitizeTable makes tabular output easier to compare
// by compressing all runs of space characters to a single
// space and deleting trailing space characters on each line.
func sanitizeTable(s string) string {
	// Compress all runs of white-space down to a single space so
	// we don't have to worry too much about tabwriter's output.
	s = strings.Join(strings.FieldsFunc(s, isSpace), " ")
	// Eliminate the trailing space on a line.
	s = strings.Replace(s, " \n", "\n", -1)
	return s
}

func isSpace(r rune) bool {
	return r == ' '
}
