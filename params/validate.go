package params

import (
	"regexp"

	"github.com/juju/names/v4"
	"gopkg.in/errgo.v1"
)

var (
	validName = regexp.MustCompile("^[a-zA-Z0-9]([a-zA-Z0-9.-]*[a-zA-Z0-9])?$")
)

type User string

func (u *User) UnmarshalText(t []byte) error {
	u0 := string(t)
	if !names.IsValidUser(u0) {
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
	*n = Name(string(t))
	return nil
}

// Cloud represents the name of a cloud.
type Cloud string

// UnmarshaText implements encoding.TextUnmarshaler.
func (c *Cloud) UnmarshalText(t []byte) error {
	c0 := string(t)
	if !names.IsValidCloud(c0) {
		return errgo.WithCausef(nil, ErrBadRequest, "invalid cloud %q", t)
	}
	*c = Cloud(c0)
	return nil
}

// CredentialName represents the credential name.
type CredentialName string

// UnmarshaText implements encoding.TextUnmarshaler.
func (c *CredentialName) UnmarshalText(t []byte) error {
	c0 := string(t)
	if !names.IsValidCloudCredentialName(c0) {
		return errgo.WithCausef(nil, ErrBadRequest, "invalid name %q", t)
	}
	*c = CredentialName(string(t))
	return nil
}

var validLocationPat = regexp.MustCompile("^[a-z]+(-[a-z0-9]+)*$")

// IsValidLocationAttr reports whether the given value is valid for
// use as a controller location attribute.
func IsValidLocationAttr(attr string) bool {
	return validLocationPat.MatchString(attr)
}
