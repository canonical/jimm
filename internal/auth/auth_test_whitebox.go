// Copyright 2026 Canonical.

package auth

import (
	"testing"

	qt "github.com/frankban/quicktest"
)

var OIDCGroupsTestGroupName = "canonical"

func TestSplitGroupClaimStringSingleValue(t *testing.T) {
	c := qt.New(t)

	groups := splitGroupClaimString(OIDCGroupsTestGroupName)
	c.Assert(groups, qt.DeepEquals, []string{OIDCGroupsTestGroupName})
}

func TestSplitGroupClaimStringCommaDelimited(t *testing.T) {
	c := qt.New(t)

	groups := splitGroupClaimString(OIDCGroupsTestGroupName + ",platform, devops")
	c.Assert(groups, qt.DeepEquals, []string{OIDCGroupsTestGroupName, "platform", "devops"})
}

func TestSplitGroupClaimStringWhitespaceDelimited(t *testing.T) {
	c := qt.New(t)

	groups := splitGroupClaimString(OIDCGroupsTestGroupName + " platform\tdevops")
	c.Assert(groups, qt.DeepEquals, []string{OIDCGroupsTestGroupName, "platform", "devops"})
}

func TestSplitGroupClaimStringEmpty(t *testing.T) {
	c := qt.New(t)

	groups := splitGroupClaimString("  ")
	c.Assert(groups, qt.IsNil)
}
