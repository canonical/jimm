// Copyright 2020 Canonical Ltd.

package dbmodel

import (
	"time"

	"github.com/juju/names/v4"
	"gorm.io/gorm"
)

// A User represents a JIMM user.
type User struct {
	gorm.Model

	// Username is the username for the user. This is the juju
	// representation of the username (i.e. with an @external suffix). The
	// username will have originated at an external identity provider in
	// JAAS deployments.
	Username string `gorm:"not null;uniqueIndex"`

	// DisplayName is the displayname of the user.
	DisplayName string `gorm:"not null"`

	// LastLogin is the time the user last authenticated to the JIMM
	// server. It will be the zero time if the user has never logged in
	// to JIMM.
	LastLogin time.Time

	// Disabled records whether the user has been disabled or not, disabled
	// users are not allowed to authenticate.
	Disabled bool `gorm:"not null;default:FALSE"`

	// ControllerAccess is the access level this user has on the JIMM
	// controller. By default all users have "add-model" access.
	ControllerAccess string `gorm:"not null;default:'add-model'"`
}

// Tag returns a names.UserTag for the user.
func (u User) Tag() names.UserTag {
	return names.NewUserTag(u.Username)
}
