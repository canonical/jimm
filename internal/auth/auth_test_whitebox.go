// Copyright 2026 Canonical.

package auth

import (
	"testing"

	qt "github.com/frankban/quicktest"
	"github.com/lestrrat-go/jwx/v2/jwt"
	"golang.org/x/oauth2"

	"github.com/canonical/jimm/v3/internal/errors"
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

func TestExtractGroupsFromAccessTokenRejectsTooManyGroups(t *testing.T) {
	c := qt.New(t)

	token, err := jwt.NewBuilder().
		Claim("groups", []string{
			"group-01", "group-02", "group-03", "group-04", "group-05",
			"group-06", "group-07", "group-08", "group-09", "group-10",
			"group-11", "group-12", "group-13", "group-14", "group-15",
			"group-16", "group-17", "group-18", "group-19", "group-20",
			"group-21",
		}).
		Build()
	c.Assert(err, qt.IsNil)

	serializedToken, err := jwt.NewSerializer().Serialize(token)
	c.Assert(err, qt.IsNil)

	authSvc := &AuthenticationService{groupClaimKey: "groups"}
	groups, err := authSvc.extractGroupsFromAccessToken(t.Context(), &oauth2.Token{AccessToken: string(serializedToken)})
	c.Assert(groups, qt.IsNil)
	c.Assert(errors.ErrorCode(err), qt.Equals, errors.CodeUnauthorized)
	c.Assert(err, qt.ErrorMatches, "authorization denied: IDP group claim contains 21 groups, maximum supported is 20")
}
