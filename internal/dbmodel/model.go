// Copyright 2020 Canonical Ltd.

package dbmodel

import (
	"database/sql"
	"time"

	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/status"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v4"
	"github.com/juju/version/v2"

	"github.com/canonical/jimm/internal/errors"
)

// Life values specify model life status
type Life int

// Enumerate all possible migration phases.
const (
	UNKNOWN Life = iota
	ALIVE
	DEAD
	DYING
	MIGRATING_INTERNAL
	MIGRATING_AWAY
)

var lifeNames = []string{
	"unknown",
	"alive",
	"dead",
	"dying",
	"migrating-internal",
	"migrating-way",
}

// String returns the name of an model life constant.
func (p Life) String() string {
	i := int(p)
	if i >= 0 && i < len(lifeNames) {
		return lifeNames[i]
	}
	return "UNKNOWN"
}

// Parselife converts a string model life name
// to its constant value.
func ParseLife(target string) (Life, bool) {
	for p, name := range lifeNames {
		if target == name {
			return Life(p), true
		}
	}
	return UNKNOWN, false
}

// A Model is a juju model.
type Model struct {
	// Note this cannot use the standard gorm.Model as the soft-delete does
	// not work with the unique constraints.
	ID        uint `gorm:"primarykey"`
	CreatedAt time.Time
	UpdatedAt time.Time

	// Name is the name of the model.
	Name string

	// UUID is the UUID of the model.
	UUID sql.NullString

	// Owner is user that owns the model.
	OwnerUsername string
	Owner         User `gorm:"foreignkey:OwnerUsername;references:Username"`

	// Controller is the controller that is hosting the model.
	ControllerID uint
	Controller   Controller

	// NewControllerID is the controller that a model is migrating to.
	// This is only filled if the new controller is outside JIMM.
	NewControllerID sql.NullInt32

	// CloudRegion is the cloud-region hosting the model.
	CloudRegionID uint
	CloudRegion   CloudRegion

	// CloudCredential is the credential used with the model.
	CloudCredentialID uint
	CloudCredential   CloudCredential

	// Type is the type of model.
	Type string

	// IsController specifies if the model hosts the controller machines.
	IsController bool

	// DefaultSeries holds the default series for the model.
	DefaultSeries string

	// Life holds the life status of the model.
	Life string

	// Status holds the current status of the model.
	Status Status `gorm:"embedded;embeddedPrefix:status_"`

	// SLA contains the SLA of the model.
	SLA SLA `gorm:"embedded;embeddedPrefix:sla_"`

	// Cores contains the count of cores in the model.
	Cores int64

	// Machines contains the count of machines in the model.
	Machines int64

	// Units contains the count of machines in the model.
	Units int64

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

// FromModelUpdate updates the model from the given ModelUpdate.
func (m *Model) SwitchOwner(u *User) {
	m.OwnerUsername = u.Username
	m.Owner = *u
}

// FromJujuModelInfo converts jujuparams.ModelInfo into Model.
func (m *Model) FromJujuModelInfo(info jujuparams.ModelInfo) error {
	m.Name = info.Name
	m.Type = info.Type
	SetNullString(&m.UUID, &info.UUID)
	m.IsController = info.IsController
	m.DefaultSeries = info.DefaultSeries
	if info.OwnerTag != "" {
		ut, err := names.ParseUserTag(info.OwnerTag)
		if err != nil {
			return errors.E(err)
		}
		m.OwnerUsername = ut.Id()
	}
	m.Life = string(info.Life)
	m.Status.FromJujuEntityStatus(info.Status)

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
		m.CloudCredential.Owner.Username = cct.Owner().Id()
	}

	if info.SLA != nil {
		m.SLA.FromJujuModelSLAInfo(*info.SLA)
	}

	if info.AgentVersion != nil {
		m.Status.Version = info.AgentVersion.String()
	}
	return nil
}

// FromModelUpdate updates the model from the given ModelUpdate.
func (m *Model) FromJujuModelUpdate(info jujuparams.ModelUpdate) {
	m.Name = info.Name
	m.Life = string(info.Life)
	m.Status.FromJujuStatusInfo(info.Status)
	m.SLA.FromJujuModelSLAInfo(info.SLA)
}

// ToJujuModel converts a model into a jujuparams.Model.
func (m Model) ToJujuModel() jujuparams.Model {
	var jm jujuparams.Model
	jm.Name = m.Name
	jm.UUID = m.UUID.String
	jm.Type = m.Type
	jm.OwnerTag = names.NewUserTag(m.OwnerUsername).String()
	return jm
}

// ToJujuModelSummary converts a model to a jujuparams.ModelSummary. The
// model must have its CloudRegion, CloudCredential, Controller, Machines,
// and Owner, associations fetched. The ModelSummary will not include the
// UserAccess or UserLastConnection fields, it is the caller's
// responsibility to complete these fields appropriately.
func (m Model) ToJujuModelSummary() jujuparams.ModelSummary {
	var ms jujuparams.ModelSummary
	ms.Name = m.Name
	ms.Type = m.Type
	ms.UUID = m.UUID.String
	ms.ControllerUUID = m.Controller.UUID
	ms.IsController = m.IsController
	ms.ProviderType = m.CloudRegion.Cloud.Type
	ms.DefaultSeries = m.DefaultSeries
	ms.CloudTag = m.CloudRegion.Cloud.Tag().String()
	ms.CloudRegion = m.CloudRegion.Name
	ms.CloudCredentialTag = m.CloudCredential.Tag().String()
	ms.OwnerTag = m.Owner.Tag().String()
	ms.Life = life.Value(m.Life)
	ms.Status = m.Status.ToJujuEntityStatus()
	ms.Counts = []jujuparams.ModelEntityCount{{
		Entity: jujuparams.Machines,
		Count:  m.Machines,
	}, {
		Entity: jujuparams.Cores,
		Count:  m.Cores,
	}, {
		Entity: jujuparams.Units,
		Count:  m.Units,
	}}

	// JIMM doesn't store information about Migrations so this is omitted.
	ms.SLA = new(jujuparams.ModelSLAInfo)
	*ms.SLA = m.SLA.ToJujuModelSLAInfo()

	v, err := version.Parse(m.Status.Version)
	if err == nil {
		// If there is an error parsing the version it is considered
		// unavailable and therefore is not set.
		ms.AgentVersion = &v
	}
	return ms
}

// An SLA contains the details of the SLA associated with the model.
type SLA struct {
	// Level contains the SLA level.
	Level string

	// Owner contains the SLA owner.
	Owner string
}

// FromJujuModelSLAInfo convers jujuparams.ModelSLAInfo into SLA.
func (s *SLA) FromJujuModelSLAInfo(js jujuparams.ModelSLAInfo) {
	s.Level = js.Level
	s.Owner = js.Owner
}

// ToJujuModelSLAInfo converts a SLA into a jujuparams.ModelSLAInfo.
func (s SLA) ToJujuModelSLAInfo() jujuparams.ModelSLAInfo {
	var msi jujuparams.ModelSLAInfo
	msi.Level = s.Level
	msi.Owner = s.Owner
	return msi
}

// A Status holds the entity status of an object.
type Status struct {
	Status  string
	Info    string
	Data    Map
	Since   sql.NullTime
	Version string
}

// FromJujuEntityStatus converts jujuparams.EntityStatus into Status.
func (s *Status) FromJujuEntityStatus(js jujuparams.EntityStatus) {
	s.Status = string(js.Status)
	s.Info = js.Info
	s.Data = Map(js.Data)
	if js.Since == nil {
		s.Since = sql.NullTime{Valid: false}
	} else {
		s.Since = sql.NullTime{
			Time:  js.Since.UTC().Truncate(time.Millisecond),
			Valid: true,
		}
	}
}

// FromJujuStatusInfo updates the Status from the given
// jujuparams.StatusInfo.
func (s *Status) FromJujuStatusInfo(info jujuparams.StatusInfo) {
	s.Status = string(info.Current)
	s.Info = info.Message
	s.Version = info.Version
	if info.Since == nil {
		s.Since = sql.NullTime{Valid: false}
	} else {
		s.Since = sql.NullTime{
			Time:  info.Since.UTC().Truncate(time.Millisecond),
			Valid: true,
		}
	}
	s.Data = Map(info.Data)
}

// ToJujuEntityStatus converts the status into a jujuparams.EntityStatus.
func (s Status) ToJujuEntityStatus() jujuparams.EntityStatus {
	var es jujuparams.EntityStatus
	es.Status = status.Status(s.Status)
	es.Info = s.Info
	es.Data = map[string]interface{}(s.Data)
	if s.Since.Valid {
		es.Since = &s.Since.Time
	} else {
		es.Since = nil
	}
	return es
}
