// Copyright 2020 Canonical Ltd.

package dbmodel

import (
	"database/sql"
	"time"

	"github.com/juju/names/v4"
	"gorm.io/gorm"
)

// A Model is a juju model.
type Model struct {
	// Note this cannot use the standard gorm.Model as the soft-delete does
	// not work with the unique constraints.
	ID        uint `gorm:"primarykey"`
	CreatedAt time.Time
	UpdatedAt time.Time

	// Name is the name of the model.
	Name string `gorm:"not null;uniqueIndex:idx_model_name_owner_id"`

	// UUID is the UUID of the model.
	UUID sql.NullString `gorm:"uniqueIndex"`

	// Owner is user that owns the model.
	OwnerID string `gorm:"uniqueIndex:idx_model_name_owner_id"`
	Owner   User   `gorm:"foreignkey:OwnerID;references:Username"`

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
	Status Status `gorm:"embedded;embeddedPrefix:status"`

	// SLA contains the SLA of the model.
	SLA SLA `gorm:"embedded"`

	// Applications are the applications attached to the model.
	Applications []Application

	// Machines are the machines attached to the model.
	Machines []Machine

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

// An SLA contains the details of the SLA associated with the model.
type SLA struct {
	// Level contains the SLA level.
	Level string

	// Owner contains the SLA owner.
	Owner string
}

// A UserModelAccess maps the access level of a user on a model.
type UserModelAccess struct {
	gorm.Model

	// User is the User this access is for.
	UserID uint `gorm:"not null;unitIndex:idx_user_model_access_user_id_model_id"`
	User   User

	// Model is the Model this access is for.
	ModelID uint  `gorm:"not null;unitIndex:idx_user_model_access_user_id_model_id"`
	Model_  Model `gorm:"foreignkey:ModelID;constraint:OnDelete:CASCADE"`

	// Access is the access level of the user on the model.
	Access string `gorm:"not null"`

	// LastConnection holds the last time the user connected to the model.
	LastConnection sql.NullTime
}

// A Status holds the entity status of an object.
type Status struct {
	Status  string
	Info    string
	Data    Map
	Since   sql.NullTime
	Version string
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
	Hardware MachineHardware `gorm:"embedded"`

	// InstanceID is the instance ID of the machine.
	InstanceID string

	// DisplayName is the display name of the machine.
	DisplayName string

	// AgentStatus is the status of the machine agent.
	AgentStatus Status `gorm:"embedded;embeddedPrefix:agent_status"`

	// InstanceStatus is the status of the machine instance.
	InstanceStatus Status `gorm:"embedded;embeddedPrefix:instance_status"`

	// HasVote indicates whether the machine has a vote.
	HasVote bool

	// WantsVote indicates whether the machine wants a vote.
	WantsVote bool

	// Series contains the machine series.
	Series string

	// Units are the units deployed to this machine.
	Units []Unit
}

// A MachineHardware contains the known details of the machine's hardware.
type MachineHardware struct {
	// Arch contains the architecture of the machine.
	Arch sql.NullString

	// Mem contains the amount of memory attached to the machine.
	Mem NullUint64

	// RootDisk contains the size of the root-disk attached to the machine.
	RootDisk NullUint64

	// Cores contains the number of cores attached to the machine.
	Cores NullUint64

	// CPUPower contains the cpu-power of the machine.
	CPUPower NullUint64

	// Tags contains the hardware tags of the machine.
	Tags Strings

	// AvailabilityZone contains the availability zone of the machine.
	AvailabilityZone sql.NullString
}

// An Application is an application in a model.
type Application struct {
	ID        uint `gorm:"primaryKey"`
	CreatedAt time.Time
	UpdatedAt time.Time

	// Model_ is the model that contains this application.
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
	Constraints Constraints `gorm:"embedded"`

	// Config contains the application config.
	Config Map

	// Subordinate contains whether this application is a subordinate.
	Subordinate bool

	// Status contains the application status.
	Status Status `gorm:"embedded;embeddedPrefix:status"`

	// WorkloadVersion contains the application's workload-version.
	WorkloadVersion string

	// Units are units of this application.
	Units []Unit

	// Offers are offers for this application.
	Offers []ApplicationOffer
}

// A Constraints object holds constraints for an application.
type Constraints struct {
	// Arch contains any arch constraint.
	Arch sql.NullString

	// Container contains any container-type.
	Container sql.NullString

	// CpuCores contains any cpu-cores.
	CpuCores NullUint64

	// CpuPower contains any cpu-power constraint.
	CpuPower NullUint64

	// Mem contains any mem constraint.
	Mem NullUint64

	// RootDisk contains any root-disk constraint.
	RootDisk NullUint64

	// RootDiskSource contains any root-disk-source constraint.
	RootDiskSource sql.NullString

	// Tags contains any tags constraint.
	Tags Strings

	// InstanceType contains any instance-type constraint.
	InstanceType sql.NullString

	// Spaces contains any spaces constraint.
	Spaces Strings

	// VirtType contains any virt-type constraint.
	VirtType sql.NullString

	// Zones contains any zones constraint.
	Zones Strings

	// AllocatePublicIP contains any allocate-public-ip constraint.
	AllocatePublicIP sql.NullBool
}

// A Unit represents a unit of an application in a model.
type Unit struct {
	ID        uint `gorm:"primaryKey"`
	CreatedAt time.Time
	UpdatedAt time.Time

	// Application contains the application this unit belongs to.
	ApplicationID uint        `gorm:"not null;uniqueIndex:idx_unit_application_id_name"`
	Application   Application `constraint:OnDelete:CASCADE"`

	// Machine contains the machine this unit is deployed to.
	MachineID uint
	Machine   Machine `constraint:OnDelete:CASCADE"`

	// Name contains the unit name.
	Name string `gorm:"not null;uniqueIndex:idx_unit_application_id_name"`

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
	WorkloadStatus Status `gorm:"embedded;embeddedPrefix:workload_status"`

	// AgentStatus is the agent status of the unit.
	AgentStatus Status `gorm:"embedded;embeddedPrefix:agent_status"`
}

// An ApplicationOffer is an offer for an application.
type ApplicationOffer struct {
	ID        uint `gorm:"primaryKey"`
	CreatedAt time.Time
	UpdatedAt time.Time

	// Application is the application that this offer is for.
	ApplicationID uint        `gorm:"not null;uniqueIndex:idx_application_offer_application_id_name"`
	Application   Application `gorm:"constraint:OnDelete:CASCADE"`

	// Name is the name of the offer.
	Name string `gorm:"not null;uniqueIndex:idx_application_offer_application_id_name"`

	// UUID is the unique ID of the offer.
	UUID string `gorm:"not null;uniqueIndex"`

	// TotalConnectedCount is the count of the total connections to the
	// application offer.
	TotalConnectedCount uint

	// ActiveConnectedCount is the count of the acrtive connections to the
	// application offer.
	ActiveConnectedCount uint

	// Users contains the users with access to the application offer.
	Users []UserApplicationOfferAccess
}

// Tag returns a names.Tag for the application-offer.
func (o ApplicationOffer) Tag() names.Tag {
	return names.NewApplicationOfferTag(o.UUID)
}

// SetTag sets the application-offer's UUID from the given tag.
func (o *ApplicationOffer) SetTag(t names.ApplicationOfferTag) {
	o.UUID = t.Id()
}

// A UserApplicationOfferAccess maps the access level for a user on an
// application-offer.
type UserApplicationOfferAccess struct {
	ID        uint `gorm:"primarykey"`
	CreatedAt time.Time
	UpdatedAt time.Time

	// User is the user associated with this access.
	UserID uint
	User   User

	// ApplicationOffer is the appliction-offer associated with this
	// access.
	ApplicationOfferID uint
	ApplicationOffer   ApplicationOffer `gorm:"constraint:OnDelete:CASCADE"`

	// Access is the access level for to the application offer to the user.
	Access string `gorm:"not null"`
}
