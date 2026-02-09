// Copyright 2025 Canonical.

package dbmodel

import (
	"database/sql"
	"time"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/core/life"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	"github.com/juju/names/v5"
)

// MigrationMode specifies where the Model is with respect to migration.
// The modes mirror Juju's state values with some JIMM-specific additions.
// We use our own type to avoid introducing a dependency on Juju's state
// package throughout JIMM's codebase.
type MigrationMode string

const (
	// MigrationModeNone is the default mode for a model and reflects
	// that it isn't involved with a model migration.
	MigrationModeNone = MigrationMode(state.MigrationMode(""))

	// MigrationModeExporting reflects a model that is in the process of being
	// exported away from JIMM.
	MigrationModeExporting = MigrationMode(state.MigrationMode("exporting"))

	// MigrationModeImporting reflects a model that is being imported into a
	// controller, but is not yet fully active.
	MigrationModeImporting = MigrationMode(state.MigrationMode("importing"))

	// MigrationModeMovingInternal reflects a model that is being moved internally
	// within JIMM, e.g. from one controller to another.
	MigrationModeMigrateInternal = MigrationMode("migrating-internally")
)

// A Model is a juju model.
type Model struct {
	// Note this cannot use the standard gorm.Model as the soft-delete does
	// not work with the unique constraints.
	ID        uint `gorm:"primarykey"`
	CreatedAt time.Time
	UpdatedAt time.Time

	// Name is the name of the model.
	Name string `gorm:"uniqueIndex:unique_model_names;not null"`

	// UUID is the UUID of the model.
	UUID sql.NullString

	// Owner is identity that owns the model.
	OwnerIdentityName string   `gorm:"uniqueIndex:unique_model_names;not null"`
	Owner             Identity `gorm:"foreignkey:OwnerIdentityName;references:Name"`

	// Controller is the controller that is hosting the model.
	ControllerID uint
	Controller   Controller

	// CloudRegion is the cloud-region hosting the model.
	CloudRegionID uint
	CloudRegion   CloudRegion

	// CloudCredential is the credential used with the model.
	CloudCredentialID uint
	CloudCredential   CloudCredential `gorm:"foreignkey:CloudCredentialID;references:ID"`

	// Life holds the life status of the model.
	Life string

	// Offers are the ApplicationOffers attached to the model.
	Offers []ApplicationOffer

	// MigrationMode is the migration mode of the model.
	MigrationMode MigrationMode `gorm:"default:''"`
}

// Tag returns a names.Tag for the model.
func (m Model) Tag() names.Tag {
	return m.ResourceTag()
}

// ResourceTag returns a tag for the model.  This method
// is intended to be used in places where we expect to see
// a concrete type names.ModelTag instead of the
// names.Tag interface.
func (m Model) ResourceTag() names.ModelTag {
	if m.UUID.Valid {
		return names.NewModelTag(m.UUID.String)
	}
	return names.ModelTag{}
}

// SetTag sets the UUID of the model to the given tag.
func (m *Model) SetTag(t names.ModelTag) {
	m.UUID.String = t.Id()
	m.UUID.Valid = true
}

// SetOwner updates the model owner.
func (m *Model) SetOwner(u *Identity) {
	m.OwnerIdentityName = u.Name
	m.Owner = *u
}

// FromJujuModelInfo converts on a best-effort basis jujuparams.ModelInfo into Model.
//
// Some fields specific to JIMM which aren't present in a jujuparams.ModelInfo type
// will need to be filled in manually by the caller of this function.
func (m *Model) FromJujuModelInfo(info base.ModelInfo) error {
	m.Name = info.Name
	SetNullString(&m.UUID, &info.UUID)
	if info.Owner != "" {
		m.OwnerIdentityName = info.Owner
	}
	m.Life = string(info.Life)

	m.CloudRegion.Name = info.CloudRegion
	if info.Cloud != "" {
		m.CloudRegion.Cloud.Name = info.Cloud
	}
	if info.CloudCredential != "" {
		cct := names.NewCloudCredentialTag(info.CloudCredential)
		m.CloudCredential.Name = cct.Name()
		m.CloudCredential.CloudName = cct.Cloud().Id()
		m.CloudCredential.Owner.Name = cct.Owner().Id()
	}

	return nil
}

// FromModelUpdate updates the model from the given ModelUpdate.
func (m *Model) FromJujuModelUpdate(info jujuparams.ModelUpdate) {
	m.Name = info.Name
	m.Life = string(info.Life)
}

// ToJujuModel converts a model into a jujuparams.Model.
func (m Model) ToJujuModel() jujuparams.Model {
	var jm jujuparams.Model
	jm.Name = m.Name
	jm.UUID = m.UUID.String
	jm.OwnerTag = names.NewUserTag(m.OwnerIdentityName).String()
	return jm
}

// MergeModelSummaryFromController merges a model as received from Juju with info in JIMM.
// It uses the info from the controller and JIMM's stores (database+OpenFGA).
// maskingControllerUUID is used to mask the controllerUUID with the JIMM's.
// access is the user's access level to the model.
func (m Model) MergeModelSummaryFromController(jujuModelSummary base.UserModelSummary, maskingControllerUUID string, access string) base.UserModelSummary {
	jujuModelSummary.Name = m.Name
	jujuModelSummary.UUID = m.UUID.String
	if maskingControllerUUID != "" {
		jujuModelSummary.ControllerUUID = maskingControllerUUID
	} else {
		jujuModelSummary.ControllerUUID = m.Controller.UUID
	}
	jujuModelSummary.ProviderType = m.CloudRegion.Cloud.Type
	jujuModelSummary.Cloud = m.CloudRegion.Cloud.Name
	jujuModelSummary.CloudRegion = m.CloudRegion.Name
	jujuModelSummary.CloudCredential = m.CloudCredential.Tag().Id()
	jujuModelSummary.Owner = m.Owner.Name
	jujuModelSummary.Life = life.Value(m.Life)
	jujuModelSummary.ModelUserAccess = access
	return jujuModelSummary
}

// MigrationFailed processes a failed migration by resetting the
// MigrationMode to MigrationModeNone.
func (m *Model) MigrationFailed() {
	m.MigrationMode = MigrationModeNone
}

// InternalMigrationSuccess processes a successful internal migration
// by setting the MigrationMode to MigrationModeNone and updating the ControllerID.
func (m *Model) InternalMigrationSuccess(controllerID uint) {
	m.MigrationMode = MigrationModeNone
	m.ControllerID = controllerID
	m.Controller = Controller{} // Clear the association to force GORM to use the new ControllerID.
}

// SetInternalMigration sets the model to be in the internal migration mode.
// This is used when the model is being migrated internally within JIMM.
func (m *Model) SetInternalMigration() {
	m.MigrationMode = MigrationModeMigrateInternal
}

// SetExternalMigration sets the model to be in the external migration mode.
// This is used when the model is being exported away from JIMM.
func (m *Model) SetExternalMigration() {
	m.MigrationMode = MigrationModeExporting
}
