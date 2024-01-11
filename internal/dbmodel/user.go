// Copyright 2020 Canonical Ltd.

package dbmodel

import (
	"database/sql"

	jujuparams "github.com/juju/juju/rpc/params"
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
	// server. LastLogin will only be a valid time if the user has
	// authenticated at least once.
	LastLogin sql.NullTime

	// Disabled records whether the user has been disabled or not, disabled
	// users are not allowed to authenticate.
	Disabled bool `gorm:"not null;default:FALSE"`

	// CloudCredentials are the cloud credentials owned by this user.
	CloudCredentials []CloudCredential `gorm:"foreignKey:OwnerUsername;references:Username"`
}

func (u User) RecentLogin() sql.NullTime {
	return u.LastLogin
}

func (u User) Name() string {
	return u.Username
}

func (u User) NameDisplay() string {
	return u.DisplayName
}

// Tag returns a names.Tag for the user.
func (u User) Tag() names.Tag {
	return u.ResourceTag()
}

// ResourceTag returns a tag for the user.  This method
// is intended to be used in places where we expect to see
// a concrete type names.UserTag instead of the
// names.Tag interface.
func (u User) ResourceTag() names.UserTag {
	return names.NewUserTag(u.Username)
}

// SetTag sets the username of the user to the value from the given tag.
func (u *User) SetTag(t names.Tag) {
	u.Username = t.Id()
}

// ToJujuUserInfo converts a User into a juju UserInfo value.
func (u User) ToJujuUserInfo() jujuparams.UserInfo {
	var ui jujuparams.UserInfo
	ui.Username = u.Username
	ui.DisplayName = u.DisplayName
	ui.Access = "" //TODO(Kian) CSS-6040 Handle merging OpenFGA and Postgres information
	ui.DateCreated = u.CreatedAt
	if u.LastLogin.Valid {
		ui.LastConnection = &u.LastLogin.Time
	}
	return ui
}
