package params

import (
	"regexp"

	"github.com/juju/names"
	"gopkg.in/errgo.v1"
)

var validName = regexp.MustCompile("^[a-zA-Z0-9][a-zA-Z0-9.-]*[a-zA-Z0-9]$")

type User string

func (u *User) UnmarshalText(t []byte) error {
	u0 := string(t)
	if !names.IsValidUserName(u0) {
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
	*n = Name(t)
	return nil
}
