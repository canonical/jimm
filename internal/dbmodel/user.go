// Copyright 2020 Canonical Ltd.

package dbmodel

import (
	"database/sql"

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
	// server. It will be not be valid if the user has never logged in
	// to JIMM.
	LastLogin sql.NullTime

	// Disabled records whether the user has been disabled or not, disabled
	// users are not allowed to authenticate.
	Disabled bool `gorm:"not null;default:FALSE"`

	// ControllerAccess is the access level this user has on the JIMM
	// controller. By default all users have "add-model" access.
	ControllerAccess string `gorm:"not null;default:'add-model'"`

	// Clouds are the clouds accessible to this user.
	Clouds []UserCloudAccess

	// CloudCredentials are the cloud credentials owned by this user.
	CloudCredentials []CloudCredential `gorm:"foreignKey:OwnerID;references:Username"`

	// Models are the models accessible to this user.
	Models []UserModelAccess

	// ApplicationOffers are the application-offers accessible to this
	// user.
	ApplicationOffers []UserApplicationOfferAccess
}

// Tag returns a names.Tag for the user.
func (u User) Tag() names.Tag {
	return names.NewUserTag(u.Username)
}

// SetTag sets the username of the user to the value from the given tag.
func (u *User) SetTag(t names.UserTag) {
	u.Username = t.Id()
}
