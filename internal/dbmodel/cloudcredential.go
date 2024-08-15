// Copyright 2024 Canonical.

package dbmodel

import (
	"database/sql"
	"fmt"

	"github.com/juju/names/v5"
	"gorm.io/gorm"
)

// A CloudCredential is a credential that is used to access a cloud.
type CloudCredential struct {
	gorm.Model

	// Name is the name of the credential.
	Name string

	// Cloud is the cloud this credential is for.
	CloudName string
	Cloud     Cloud `gorm:"foreignKey:CloudName;references:Name;constraint:OnDelete:CASCADE"`

	// Owner is the identity that owns this credential.
	OwnerIdentityName string
	Owner             Identity `gorm:"foreignKey:OwnerIdentityName;references:Name"`

	// AuthType is the type of the credential.
	AuthType string

	// Label is an optional label for the credential.
	Label string

	// AttributesInVault indicates whether the attributes are stored in
	// the configured vault key-value store, rather than this database.
	AttributesInVault bool

	// Attributes contains the attributes of the credential.
	Attributes StringMap

	// Valid stores whether the cloud-credential is known to be valid.
	Valid sql.NullBool

	// Models contains the models using this credential.
	Models []Model
}

// Tag returns a names.Tag for the cloud-credential.
func (c CloudCredential) Tag() names.Tag {
	return c.ResourceTag()
}

// ResourceTag returns a tag for the cloud-credential.  This method
// is intended to be used in places where we expect to see
// a concrete type names.CloudCredentialTag instead of the
// names.Tag interface.
func (c CloudCredential) ResourceTag() names.CloudCredentialTag {
	return names.NewCloudCredentialTag(fmt.Sprintf("%s/%s/%s", c.CloudName, c.OwnerIdentityName, c.Name))
}

// SetTag sets the Name, CloudName and Username fields from the given tag.
func (c *CloudCredential) SetTag(t names.CloudCredentialTag) {
	c.CloudName = t.Cloud().Id()
	c.Name = t.Name()
	c.OwnerIdentityName = t.Owner().Id()
}

// Path returns a juju style cloud credential path.
func (c CloudCredential) Path() string {
	return fmt.Sprintf("%s/%s/%s", c.CloudName, c.OwnerIdentityName, c.Name)
}
