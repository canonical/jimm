// Copyright 2020 Canonical Ltd.

package conv

import (
	"github.com/juju/names/v4"
	"gopkg.in/errgo.v1"

	"github.com/canonical/jimm/params"
)

// ErrLocalUser is the error returned when parsing a juju user that is
// local to the google controller, these are not supported by JIMM.
var ErrLocalUser = errgo.New("unsupported local user")

// UserTag creates a juju user tag from a params.User
func ToUserTag(u params.User) names.UserTag {
	tag := names.NewUserTag(string(u))
	if tag.IsLocal() {
		tag = tag.WithDomain("external")
	}
	return tag
}

// FromUserTag creates a params.User from a juju user tag. If the user tag
// is for a local user then an error with a cause of ErrLocalUser is
// returned.
func FromUserTag(t names.UserTag) (params.User, error) {
	if t.IsLocal() {
		return "", errgo.WithCausef(nil, ErrLocalUser, "")
	}
	if t.Domain() == "external" {
		return params.User(t.Name()), nil
	}
	return params.User(t.Id()), nil
}

// FromUserID parses a string as a juju user ID and converts it to the
// equivalent params.User. If the user ID is for a juju local user then an
// error with a cause of ErrLocalUser is returned.
func FromUserID(s string) (params.User, error) {
	if !names.IsValidUser(s) {
		return "", errgo.Newf("invalid user id %q", s)
	}
	u, err := FromUserTag(names.NewUserTag(s))
	return u, errgo.Mask(err, errgo.Is(ErrLocalUser))
}

// ParseUserTag parses the given string as a user tag and converts it to a
// params.User. If the given string is not a valid user tag then an error
// is returned with a cause of params.ErrBadRequest. If the user tag
// represents a juju local user an error with a cause of ErrLocalUser is
// returned.
func ParseUserTag(s string) (params.User, error) {
	tag, err := names.ParseUserTag(s)
	if err != nil {
		return "", errgo.WithCausef(err, params.ErrBadRequest, "")
	}
	return FromUserTag(tag)
}
