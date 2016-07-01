package params

import (
	"regexp"
	"strings"

	"gopkg.in/errgo.v1"
	"gopkg.in/juju/names.v2"
)

var validName = regexp.MustCompile("^[a-zA-Z0-9]([a-zA-Z0-9.-]*[a-zA-Z0-9])?$")

type User string

func (u *User) UnmarshalText(t []byte) error {
	u0 := string(t)
	if !names.IsValidUserName(u0) {
		return errgo.WithCausef(nil, ErrBadRequest, "invalid user name %q", u0)
	}
	// Forbid double-hyphen because we use it as a separator.
	if strings.Contains(u0, "--") {
		return errgo.WithCausef(nil, ErrBadRequest, "invalid user name %q", u0)
	}
	*u = User(u0)
	return nil
}

type Name string

func (n *Name) UnmarshalText(t []byte) error {
	if !validName.Match(t) {
		return errgo.WithCausef(nil, ErrBadRequest, "invalid name %q", t)
	}
	// Forbid double-hyphen because we use it as a separator.
	t0 := string(t)
	if strings.Contains(t0, "--") {
		return errgo.WithCausef(nil, ErrBadRequest, "invalid name %q", t0)
	}
	*n = Name(t0)
	return nil
}

var validLocationPat = regexp.MustCompile("^[a-z]+(-[a-z0-9]+)*$")

// IsValidLocationAttr reports whether the given value is valid for
// use as a controller location attribute.
func IsValidLocationAttr(attr string) bool {
	return validLocationPat.MatchString(attr)
}
