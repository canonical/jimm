// Copyright 2020 Canonical Ltd.

package dbmodel

import (
	"database/sql"
	"time"

	jujuparams "github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/status"
	"github.com/juju/names/v4"
	"github.com/juju/version/v2"
	"gorm.io/gorm"

	"github.com/canonical/jimm/internal/errors"
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

	// Applications are the applications attached to the model.
	Applications []Application

	// Machines are the machines attached to the model.
	Machines []Machine

	// Units are the units attached to the model.
	Units []Unit

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

	m.Machines = make([]Machine, len(info.Machines))
	for i, machine := range info.Machines {
		m.Machines[i].FromJujuModelMachineInfo(machine)
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

// ToJujuModelInfo converts a model into a jujuparams.ModelInfo. The model
// must have its Applications, CloudRegion, CloudCredential, Controller,
// Machines, Owner, and Users associations fetched. The ModelInfo is
// created with admin-level data, it is the caller's responsibility to
// filter any data that should not be returned.
func (m Model) ToJujuModelInfo() jujuparams.ModelInfo {
	var mi jujuparams.ModelInfo
	mi.Name = m.Name
	mi.Type = m.Type
	mi.UUID = m.UUID.String
	mi.ControllerUUID = m.Controller.UUID
	mi.IsController = m.IsController
	mi.ProviderType = m.CloudRegion.Cloud.Type
	mi.DefaultSeries = m.DefaultSeries
	mi.CloudTag = m.CloudRegion.Cloud.Tag().String()
	mi.CloudRegion = m.CloudRegion.Name
	mi.CloudCredentialTag = m.CloudCredential.Tag().String()
	if m.CloudCredential.Valid.Valid {
		mi.CloudCredentialValidity = &m.CloudCredential.Valid.Bool
	}
	mi.OwnerTag = m.Owner.Tag().String()
	mi.Life = life.Value(m.Life)
	mi.Status = m.Status.ToJujuEntityStatus()
	mi.Users = make([]jujuparams.ModelUserInfo, len(m.Users))
	for i, u := range m.Users {
		mi.Users[i] = u.ToJujuModelUserInfo()
	}
	mi.Machines = make([]jujuparams.ModelMachineInfo, len(m.Machines))
	for i, machine := range m.Machines {
		mi.Machines[i] = machine.ToJujuModelMachineInfo()
	}
	// JIMM doesn't store information about Migrations so this is omitted.
	mi.SLA = new(jujuparams.ModelSLAInfo)
	*mi.SLA = m.SLA.ToJujuModelSLAInfo()

	v, err := version.Parse(m.Status.Version)
	if err == nil {
		// If there is an error parsing the version it is considered
		// unavailable and therefore is not set.
		mi.AgentVersion = &v
	}
	return mi
}

// ToJujuModelSummary converts a model to a jujuparams.ModelSummary. The
// model must have its Applications, CloudRegion, CloudCredential,
// Controller, Machines, and Owner, associations fetched. The ModelSummary
// will not include the UserAccess or UserLastConnection fields, it is the
// caller's responsibility to complete these fields appropriately.
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
	var machines, cores, units int64
	for _, mach := range m.Machines {
		machines += 1
		if mach.Hardware.CPUCores.Valid {
			cores += int64(mach.Hardware.CPUCores.Uint64)
		}
		units += int64(len(mach.Units))
	}
	ms.Counts = []jujuparams.ModelEntityCount{{
		Entity: jujuparams.Machines,
		Count:  machines,
	}, {
		Entity: jujuparams.Cores,
		Count:  cores,
	}, {
		Entity: jujuparams.Units,
		Count:  units,
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

// ToJujuModelUserInfo covnerts a UserModelAccess into a
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

// A Machine is a machine in a model.
type Machine struct {
	ID        uint `gorm:"primaryKey"`
	CreatedAt time.Time
	UpdatedAt time.Time

	// ModelID is the ID of the owning model
	ModelID uint  `gorm:"not null;uniqueIndex:idx_machine_model_id_machine_id"`
	Model   Model `gorm:"constraint:OnDelete:CASCADE"`

	// MachineID is the ID of the machine within the model.
	MachineID string `gorm:"not null;uniqueIndex:idx_machine_model_id_machine_id"`

	// Hardware contains the hardware characteristics of the machine.
	Hardware Hardware `gorm:"embedded;embeddedPrefix:hw_"`

	// InstanceID is the instance ID of the machine.
	InstanceID string

	// DisplayName is the display name of the machine.
	DisplayName string

	// AgentStatus is the status of the machine agent.
	AgentStatus Status `gorm:"embedded;embeddedPrefix:agent_status_"`

	// InstanceStatus is the status of the machine instance.
	InstanceStatus Status `gorm:"embedded;embeddedPrefix:instance_status_"`

	// Life contains the life status of the machine.
	Life string

	// HasVote indicates whether the machine has a vote.
	HasVote bool

	// WantsVote indicates whether the machine wants a vote.
	WantsVote bool

	// Series contains the machine series.
	Series string

	// Units are the units deployed to this machine.
	Units []Unit `gorm:"foreignKey:ModelID,MachineID;references:ModelID,MachineID"`
}

// FromJujuMachineInfo converts jujuparams.MachineInfo into a Machine.
func (m *Machine) FromJujuMachineInfo(info jujuparams.MachineInfo) {
	m.MachineID = info.Id
	m.InstanceID = info.InstanceId
	m.AgentStatus.FromJujuStatusInfo(info.AgentStatus)
	m.InstanceStatus.FromJujuStatusInfo(info.InstanceStatus)
	m.Life = string(info.Life)
	m.Series = info.Series
	if info.HardwareCharacteristics != nil {
		m.Hardware.FromJujuInstanceHardwareCharacteristics(*info.HardwareCharacteristics)
	}
	m.HasVote = info.HasVote
	m.WantsVote = info.WantsVote
}

// FromJujuModelMachineInfo converts jujuparams.ModelMachineInfo into a Machine.
func (m *Machine) FromJujuModelMachineInfo(mi jujuparams.ModelMachineInfo) {
	m.MachineID = mi.Id
	if mi.Hardware != nil {
		m.Hardware.FromJujuMachineHardware(*mi.Hardware)
	}
	m.InstanceID = mi.InstanceId
	m.DisplayName = mi.DisplayName
	m.InstanceStatus.Status = mi.Status
	m.InstanceStatus.Info = mi.Message
	m.HasVote = mi.HasVote
	m.WantsVote = mi.WantsVote
}

// ToJujuModelMachineInfo converts a Machine into a
// jujuparams.ModelMachineInfo.
func (m Machine) ToJujuModelMachineInfo() jujuparams.ModelMachineInfo {
	var mmi jujuparams.ModelMachineInfo
	mmi.Id = m.MachineID
	mmi.Hardware = new(jujuparams.MachineHardware)
	*mmi.Hardware = m.Hardware.ToJujuMachineHardware()
	mmi.InstanceId = m.InstanceID
	mmi.DisplayName = m.DisplayName
	mmi.Status = m.InstanceStatus.Status
	mmi.Message = m.InstanceStatus.Info
	mmi.HasVote = m.HasVote
	mmi.WantsVote = m.WantsVote
	// HAPrimary status is not known in jimm so it is always
	// omitted.
	return mmi
}

// A Hardware structure contains the known details of a hardware
// definition. This is the superset of the various hardware and constraints
// structures in the juju API.
type Hardware struct {
	// Arch contains the architecture of the machine.
	Arch sql.NullString

	// Container contains any container-type.
	Container sql.NullString

	// Mem contains the amount of memory attached to the machine.
	Mem NullUint64

	// RootDisk contains the size of the root-disk attached to the machine.
	RootDisk NullUint64

	// RootDiskSource contains any root-disk-source constraint.
	RootDiskSource sql.NullString

	// CPUCores contains the number of cores attached to the machine.
	CPUCores NullUint64

	// CPUPower contains the cpu-power of the machine.
	CPUPower NullUint64

	// Tags contains the hardware tags of the machine.
	Tags Strings

	// AvailabilityZone contains the availability zone of the machine.
	AvailabilityZone sql.NullString

	// Zones contains any zones constraint.
	Zones Strings

	// InstanceType contains any instance-type constraint.
	InstanceType sql.NullString

	// Spaces contains any spaces constraint.
	Spaces Strings

	// VirtType contains any virt-type constraint.
	VirtType sql.NullString

	// AllocatePublicIP contains any allocate-public-ip constraint.
	AllocatePublicIP sql.NullBool
}

// FromJujuConstraintsValue updates the Hardware entry with the values from
// a juju constraints.Value structure.
func (h *Hardware) FromJujuConstraintsValue(v constraints.Value) {
	SetNullString(&h.Arch, v.Arch)
	h.Container.Valid = v.Container != nil
	if h.Container.Valid {
		h.Container.String = string(*v.Container)
	} else {
		h.Container.String = ""
	}
	h.CPUCores.FromValue(v.CpuCores)
	h.CPUPower.FromValue(v.CpuPower)
	h.Mem.FromValue(v.Mem)
	h.RootDisk.FromValue(v.RootDisk)
	h.Tags.FromPointer(v.Tags)
	SetNullString(&h.InstanceType, v.InstanceType)
	h.Spaces.FromPointer(v.Spaces)
	SetNullString(&h.VirtType, v.VirtType)
	h.Zones.FromPointer(v.Zones)
	SetNullBool(&h.AllocatePublicIP, v.AllocatePublicIP)
}

// FromJujuInstanceHardwareCharacteristics converts
// instance.HardwareCharacteristics into a MachineHardware.
func (h *Hardware) FromJujuInstanceHardwareCharacteristics(hwc instance.HardwareCharacteristics) {
	SetNullString(&h.Arch, hwc.Arch)
	h.Mem.FromValue(hwc.Mem)
	h.RootDisk.FromValue(hwc.RootDisk)
	SetNullString(&h.RootDiskSource, hwc.RootDiskSource)
	h.CPUCores.FromValue(hwc.CpuCores)
	h.CPUPower.FromValue(hwc.CpuPower)
	h.Tags.FromPointer(hwc.Tags)
	SetNullString(&h.AvailabilityZone, hwc.AvailabilityZone)
}

// FromJujuMachineHardware converts jujuparams.MachineHardware into a Hardware.
func (h *Hardware) FromJujuMachineHardware(mh jujuparams.MachineHardware) {
	SetNullString(&h.Arch, mh.Arch)
	h.Mem.FromValue(mh.Mem)
	h.RootDisk.FromValue(mh.RootDisk)
	h.CPUCores.FromValue(mh.Cores)
	h.CPUPower.FromValue(mh.CpuPower)
	h.Tags.FromPointer(mh.Tags)
	SetNullString(&h.AvailabilityZone, mh.AvailabilityZone)
}

// ToJujuMachineHardware converts a MachineHardware into a
// jujuparams.MachineHardware.
func (h Hardware) ToJujuMachineHardware() jujuparams.MachineHardware {
	var mh jujuparams.MachineHardware
	if h.Arch.Valid {
		mh.Arch = &h.Arch.String
	} else {
		mh.Arch = nil
	}
	if h.Mem.Valid {
		mh.Mem = &h.Mem.Uint64
	} else {
		mh.Mem = nil
	}
	if h.RootDisk.Valid {
		mh.RootDisk = &h.RootDisk.Uint64
	} else {
		mh.RootDisk = nil
	}
	if h.CPUCores.Valid {
		mh.Cores = &h.CPUCores.Uint64
	} else {
		mh.Cores = nil
	}
	if h.CPUPower.Valid {
		mh.CpuPower = &h.CPUPower.Uint64
	} else {
		mh.CpuPower = nil
	}
	if h.Tags == nil {
		mh.Tags = nil
	} else {
		mh.Tags = (*[]string)(&h.Tags)
	}
	if h.AvailabilityZone.Valid {
		mh.AvailabilityZone = &h.AvailabilityZone.String
	} else {
		mh.AvailabilityZone = nil
	}
	return mh
}

// An Application is an application in a model.
type Application struct {
	ID        uint `gorm:"primaryKey"`
	CreatedAt time.Time
	UpdatedAt time.Time

	// Model is the model that contains this application.
	ModelID uint  `gorm:"not null;uniqueIndex:idx_application_model_id_name"`
	Model   Model `gorm:"constraint:OnDelete:CASCADE"`

	// Name is the name of the application.
	Name string `gorm:"not null;uniqueIndex:idx_application_model_id_name"`

	// Exposed is the exposed status of the application.
	Exposed bool

	// CharmURL contains the URL of the charm that supplies the
	CharmURL string

	// Life contains the life status of the application.
	Life string

	// MinUnits contains the minimum number of units required for the
	// application.
	MinUnits uint

	// Constraints contains the application constraints.
	Constraints Hardware `gorm:"embedded;embeddedPrefix:constraint_"`

	// Config contains the application config.
	Config Map

	// Subordinate contains whether this application is a subordinate.
	Subordinate bool

	// Status contains the application status.
	Status Status `gorm:"embedded;embeddedPrefix:status_"`

	// WorkloadVersion contains the application's workload-version.
	WorkloadVersion string

	// Units are units of this application.
	Units []Unit `gorm:"foreignKey:ModelID,ApplicationName;references:ModelID,Name"`

	// Offers are offers for this application.
	Offers []ApplicationOffer `gorm:"foreignKey:ModelID,ApplicationName;references:ModelID,Name"`
}

// FromJujuApplicationInfo sets the values of the Application from a juju
// ApplicationInfo structure.
func (a *Application) FromJujuApplicationInfo(info jujuparams.ApplicationInfo) {
	a.Name = info.Name
	a.Exposed = info.Exposed
	a.CharmURL = info.CharmURL
	a.Life = string(info.Life)
	a.MinUnits = uint(info.MinUnits)
	a.Constraints.FromJujuConstraintsValue(info.Constraints)
	a.Config = Map(info.Config)
	a.Subordinate = info.Subordinate
	a.Status.FromJujuStatusInfo(info.Status)
	a.WorkloadVersion = info.WorkloadVersion
}

// A Unit represents a unit of an application in a model.
type Unit struct {
	ID        uint `gorm:"primaryKey"`
	CreatedAt time.Time
	UpdatedAt time.Time

	// Model is the model this unit belongs to.
	ModelID uint
	Model   Model

	// Application contains the application this unit belongs to.
	ApplicationName string
	Application     Application `gorm:"foreignKey:ModelID,ApplicationName;references:ModelID,Name"`

	// Machine contains the machine this unit is deployed to.
	MachineID string
	Machine   Machine `gorm:"foreignKey:ModelID,MachineID;references:ModelID,MachineID"`

	// Name contains the unit name.
	Name string

	// Life contains the life status of the unit.
	Life string

	// PublicAddress contains the public address of the unit.
	PublicAddress string

	// PrivateAddress contains the private address of the unit.
	PrivateAddress string

	// Ports contains the ports opened on this unit.
	Ports Ports

	// PortRanges contains the port ranges opened on this unit.
	PortRanges PortRanges

	// Principal contains the principal name of the unit.
	Principal string

	// WorkloadStatus is the workload status of the unit.
	WorkloadStatus Status `gorm:"embedded;embeddedPrefix:workload_status_"`

	// AgentStatus is the agent status of the unit.
	AgentStatus Status `gorm:"embedded;embeddedPrefix:agent_status_"`
}

// FromJujuUnitInfo populates the values of the Unit structure from the
// given jujuparams.UnitInfo.
func (u *Unit) FromJujuUnitInfo(info jujuparams.UnitInfo) {
	u.Name = info.Name
	u.MachineID = info.MachineId
	u.ApplicationName = info.Application
	u.Life = string(info.Life)
	u.PublicAddress = info.PublicAddress
	u.PrivateAddress = info.PrivateAddress
	u.Ports = Ports(info.Ports)
	u.PortRanges = PortRanges(info.PortRanges)
	u.Principal = info.Principal
	u.WorkloadStatus.FromJujuStatusInfo(info.WorkloadStatus)
	u.AgentStatus.FromJujuStatusInfo(info.AgentStatus)
}
