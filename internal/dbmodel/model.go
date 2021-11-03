// Copyright 2020 Canonical Ltd.

package dbmodel

import (
	"database/sql"
	"time"

	jujuparams "github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/status"
	"github.com/juju/names/v4"
	"github.com/juju/version/v2"
	"gorm.io/gorm"

	"github.com/CanonicalLtd/jimm/internal/errors"
)

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

	// Users are the users that can access the model.
	Users []UserModelAccess
}

// Tag returns a names.Tag for the model.
func (m Model) Tag() names.Tag {
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

// UserAccess returns the access level of the given user on the model. If
// the user has no access then an empty string is returned.
func (m *Model) UserAccess(u *User) string {
	for _, mu := range m.Users {
		if u.Username == mu.Username {
			return mu.Access
		}
	}
	return ""
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

	m.Users = make([]UserModelAccess, len(info.Users))
	for i, u := range info.Users {
		m.Users[i].FromJujuModelUserInfo(u)
	}

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

// A UserModelAccess maps the access level of a user on a model.
type UserModelAccess struct {
	gorm.Model

	// User is the User this access is for.
	Username string
	User     User `gorm:"foreignKey:Username;references:Username"`

	// Model is the Model this access is for.
	ModelID uint
	Model_  Model `gorm:"foreignKey:ModelID"`

	// Access is the access level of the user on the model.
	Access string `gorm:"not null"`

	// LastConnection holds the last time the user connected to the model.
	LastConnection sql.NullTime
}

// TableName overrides the table name gorm will use to find
// UserModelAccess records.
func (UserModelAccess) TableName() string {
	return "user_model_access"
}

// FromJujuModelUserInfo converts jujuparams.ModelUserInfo into a User.
func (a *UserModelAccess) FromJujuModelUserInfo(u jujuparams.ModelUserInfo) {
	a.User = User{
		Username:    u.UserName,
		DisplayName: u.DisplayName,
	}
	a.Access = string(u.Access)
	if u.LastConnection != nil {
		a.LastConnection = sql.NullTime{
			Time:  *u.LastConnection,
			Valid: true,
		}
	}
}

// ToJujuModelUserInfo converts a UserModelAccess into a
// jujuparams.ModelUserInfo. The UserModelAccess must have its User
// association loaded.
func (a UserModelAccess) ToJujuModelUserInfo() jujuparams.ModelUserInfo {
	var mui jujuparams.ModelUserInfo
	mui.UserName = a.User.Username
	mui.DisplayName = a.User.DisplayName
	if a.LastConnection.Valid {
		mui.LastConnection = &a.LastConnection.Time
	} else {
		mui.LastConnection = nil
	}
	mui.Access = jujuparams.UserAccessPermission(a.Access)
	return mui
}

// ToUserModel converts a UserModelAccess into a jujuparams.ModelUserInfo.
// The UserModelAccess must have its Model_ association loaded.
func (a UserModelAccess) ToJujuUserModel() jujuparams.UserModel {
	var um jujuparams.UserModel
	um.Model = a.Model_.ToJujuModel()
	if a.LastConnection.Valid {
		um.LastConnection = &a.LastConnection.Time
	} else {
		um.LastConnection = nil
	}
	return um
}

// ToJujuModelSummary converts a UserModelAccess to a
// jujuparams.ModelSummary. The UserModelAccess must have its Model_
// association filled out.
func (a UserModelAccess) ToJujuModelSummary() jujuparams.ModelSummary {
	ms := a.Model_.ToJujuModelSummary()
	ms.UserAccess = jujuparams.UserAccessPermission(a.Access)
	if a.LastConnection.Valid {
		ms.UserLastConnection = &a.LastConnection.Time
	} else {
		ms.UserLastConnection = nil
	}
	return ms
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
