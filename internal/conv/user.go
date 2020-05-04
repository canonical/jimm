// Copyright 2020 Canonical Ltd.

package conv

import (
	"github.com/juju/names/v4"

	"github.com/CanonicalLtd/jimm/params"
)

// UserTag creates a juju user tag from a params.User
func ToUserTag(u params.User) names.UserTag {
	tag := names.NewUserTag(string(u))
	if tag.IsLocal() {
		tag = tag.WithDomain("external")
	}
	return tag
}
