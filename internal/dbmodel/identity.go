// Copyright 2020 Canonical Ltd.

package dbmodel

import (
	"database/sql"

	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v5"
	"gorm.io/gorm"
)

// Identity represents a JIMM identity, which can be a user or a service account.
type Identity struct {
	gorm.Model

	// Name is the name of the identity. This is the user name when
	// representing a Juju user (i.e. with an @external suffix), or the client
	// ID for a service account. The Name will have originated at an
	// external identity provider in JAAS deployments.
	Name string `gorm:"not null;uniqueIndex"`

	// DisplayName is the display name of the identity.
	DisplayName string `gorm:"not null"`

	// LastLogin is the time the identity last authenticated to the JIMM
	// server. LastLogin will only be a valid time if the identity has
	// authenticated at least once.
	LastLogin sql.NullTime

	// Disabled records whether the identity has been disabled or not, disabled
	// identities are not allowed to authenticate.
	Disabled bool `gorm:"not null;default:FALSE"`

	// CloudCredentials are the cloud credentials owned by this identity.
	CloudCredentials []CloudCredential `gorm:"foreignKey:OwnerIdentityName;references:Name"`

	// AccessToken is an OAuth2.0 access token for this identity, it may have come
	// from the browser or device flow, and as such is updated on every successful
	// login.
	AccessToken string
}

// Tag returns a names.Tag for the identity.
func (u Identity) Tag() names.Tag {
	return u.ResourceTag()
}

// ResourceTag returns a tag for the user. This method
// is intended to be used in places where we expect to see
// a concrete type names.UserTag instead of the
// names.Tag interface.
func (i Identity) ResourceTag() names.UserTag {
	return names.NewUserTag(i.Name)
}

// SetTag sets the identity name of the identity to the value from the given tag.
func (i *Identity) SetTag(t names.UserTag) {
	i.Name = t.Id()
}

// ToJujuUserInfo converts an Identity into a juju UserInfo value.
func (i Identity) ToJujuUserInfo() jujuparams.UserInfo {
	var ui jujuparams.UserInfo
	ui.Username = i.Name
	ui.DisplayName = i.DisplayName
	ui.Access = "" //TODO(Kian) CSS-6040 Handle merging OpenFGA and Postgres information
	ui.DateCreated = i.CreatedAt
	if i.LastLogin.Valid {
		ui.LastConnection = &i.LastLogin.Time
	}
	return ui
}
