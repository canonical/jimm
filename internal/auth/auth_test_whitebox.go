// Copyright 2026 Canonical.

package auth

import (
	"testing"

	qt "github.com/frankban/quicktest"
)

func TestSplitGroupClaimStringSingleValue(t *testing.T) {
	c := qt.New(t)

	groups := splitGroupClaimString("canonical")
	c.Assert(groups, qt.DeepEquals, []string{"canonical"})
}

func TestSplitGroupClaimStringCommaDelimited(t *testing.T) {
	c := qt.New(t)

	groups := splitGroupClaimString("canonical,platform, devops")
	c.Assert(groups, qt.DeepEquals, []string{"canonical", "platform", "devops"})
}

func TestSplitGroupClaimStringWhitespaceDelimited(t *testing.T) {
	c := qt.New(t)

	groups := splitGroupClaimString("canonical platform\tdevops")
	c.Assert(groups, qt.DeepEquals, []string{"canonical", "platform", "devops"})
}

func TestSplitGroupClaimStringEmpty(t *testing.T) {
	c := qt.New(t)

	groups := splitGroupClaimString("  ")
	c.Assert(groups, qt.IsNil)
}
