// Copyright 2024 Canonical.

package dbmodel

import (
	"database/sql"
	"time"

	"github.com/juju/juju/core/life"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v5"

	"github.com/canonical/jimm/v3/internal/errors"
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
func (m *Model) FromJujuModelInfo(info jujuparams.ModelInfo) error {
	m.Name = info.Name
	SetNullString(&m.UUID, &info.UUID)
	if info.OwnerTag != "" {
		ut, err := names.ParseUserTag(info.OwnerTag)
		if err != nil {
			return errors.E(err)
		}
		m.OwnerIdentityName = ut.Id()
	}
	m.Life = string(info.Life)

	m.CloudRegion.Name = info.CloudRegion
	if info.CloudTag != "" {
		ct, err := names.ParseCloudTag(info.CloudTag)
		if err != nil {
			return errors.E(err)
		}
		m.CloudRegion.Cloud.Name = ct.Id()
	}
	if info.CloudCredentialTag != "" {
		cct, err := names.ParseCloudCredentialTag(info.CloudCredentialTag)
		if err != nil {
			return errors.E(err)
		}
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

// MergeModelSummaryFromController converts a model to a jujuparams.ModelSummary.
// It uses the info from the controller and JIMM's db to fill the jujuparams.ModelSummary.
// maskingControllerUUID is used to mask the controllerUUID with the JIMM's one.
// access is the user access level got from JIMM.
func (m Model) MergeModelSummaryFromController(modelSummaryFromController *jujuparams.ModelSummary, maskingControllerUUID string, access jujuparams.UserAccessPermission) jujuparams.ModelSummary {
	if modelSummaryFromController == nil {
		modelSummaryFromController = &jujuparams.ModelSummary{}
	}
	modelSummaryFromController.Name = m.Name
	modelSummaryFromController.UUID = m.UUID.String
	if maskingControllerUUID != "" {
		modelSummaryFromController.ControllerUUID = maskingControllerUUID
	} else {
		modelSummaryFromController.ControllerUUID = m.Controller.UUID
	}
	modelSummaryFromController.ProviderType = m.CloudRegion.Cloud.Type
	modelSummaryFromController.CloudTag = m.CloudRegion.Cloud.Tag().String()
	modelSummaryFromController.CloudRegion = m.CloudRegion.Name
	modelSummaryFromController.CloudCredentialTag = m.CloudCredential.Tag().String()
	modelSummaryFromController.OwnerTag = m.Owner.Tag().String()
	modelSummaryFromController.Life = life.Value(m.Life)
	modelSummaryFromController.UserAccess = access
	return *modelSummaryFromController
}
