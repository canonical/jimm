// Copyright 2020 Canonical Ltd.

package dbmodel

import (
	"encoding/json"
	"strings"

	"github.com/juju/names/v4"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// A Cloud represents a cloud service.
type Cloud struct {
	gorm.Model

	// Name is the name of the cloud.
	Name string `gorm:"not null;uniqueIndex"`

	// Type is the provider type of cloud.
	Type string `gorm:"not null"`

	// HostCloudRegion is the "cloud/region" that hosts this cloud, if the
	// cloud is hosted.
	HostCloudRegion string

	// AuthTypes_ is the authentication types supported by this cloud.
	AuthTypes_ []AuthType `gorm:"many2many:cloud_auth_types"`

	// Endpoint is the API endpoint URL for the cloud.
	Endpoint string

	// IdentityEndpoint is the API endpoint URL of the cloud identity
	// service.
	IdentityEndpoint string

	// StorageEndpoint is the API endpoint URL of the cloud storage
	// service.
	StorageEndpoint string

	// Regions contains the regions associated with this cloud.
	Regions []CloudRegion

	// CACertificates_ contains the CA Certificates associated with this
	// cloud. The certificates are stored as a flattened string array.
	CACertificates_ string `gorm:"column:ca_certificates"`

	// Config_ contains the configuration associated with this cloud. The
	// config is stored as a JSON object.
	Config_ datatypes.JSON `gorm:"column:config"`

	// Users contains the users that are authorized on this cloud.
	Users []UserCloudAccess
}

// Tag returns a names.Tag for this cloud.
func (c Cloud) Tag() names.Tag {
	return names.NewCloudTag(c.Name)
}

// AuthTypes returns the cloud's authentication types as a string slice.
func (c Cloud) AuthTypes() []string {
	authTypes := make([]string, len(c.AuthTypes_))
	for i, v := range c.AuthTypes_ {
		authTypes[i] = v.Name
	}
	return authTypes
}

// SetAuthTypes sets the clouds authentication types.
func (c *Cloud) SetAuthTypes(authTypes []string) {
	c.AuthTypes_ = make([]AuthType, len(authTypes))
	for i, v := range authTypes {
		c.AuthTypes_[i].Name = v
	}
}

// CACertificates returns the cloud's CA certificates.
func (c Cloud) CACertificates() []string {
	return strings.Split(c.CACertificates_, "\x1f")
}

// SetCACertificates sets the cloud's CA certificats to the given value.
// The certificates are stored in a single string with each certificate
// separated by a \x1f (unit separator).
func (c *Cloud) SetCACertificates(certs []string) {
	c.CACertificates_ = strings.Join(certs, "\x1f")
}

// Config returns the cloud-specific configuration.
func (c Cloud) Config() map[string]interface{} {
	var config map[string]interface{}
	// Ignore any unmarshal error, if the data is not a valid object then
	// there is no config.
	json.Unmarshal(([]byte)(c.Config_), &config)
	return config
}

// SetConfig sets the cloud-specific configuration.
func (c *Cloud) SetConfig(config map[string]interface{}) {
	buf, err := json.Marshal(config)
	if err != nil {
		// It should be impossible to fail to marshal the config.
		panic(err)
	}
	c.Config_ = datatypes.JSON(buf)
}

// Region returns the cloud region with the given name. If there is no
// such region a zero valued region is returned.
func (c Cloud) Region(name string) CloudRegion {
	for _, r := range c.Regions {
		if r.Name == name {
			return r
		}
	}
	return CloudRegion{}
}

// A CloudRegion is a region of a cloud.
type CloudRegion struct {
	gorm.Model

	// Cloud is the cloud this region belongs to.
	CloudID uint `gorm:"uniqueIndex:idx_cloud_region_cloud_id_name"`
	Cloud   Cloud

	// Name is the name of the region.
	Name string `gorm:"not null;uniqueIndex:idx_cloud_region_cloud_id_name"`

	// Endpoint is the API endpoint URL for the region.
	Endpoint string

	// IdentityEndpoint is the API endpoint URL of the region identity
	// service.
	IdentityEndpoint string

	// StorageEndpoint is the API endpoint URL of the region storage
	// service.
	StorageEndpoint string

	// Config_ contains the configuration associated with this region. The
	// config is stored as a JSON object.
	Config_ datatypes.JSON `gorm:"column:config"`

	// Controllers contains any controllers that can provide service for
	// this cloud-region.
	Controllers []CloudRegionControllerPriority
}

// Config returns the region-specific configuration.
func (r CloudRegion) Config() map[string]interface{} {
	var config map[string]interface{}
	// Ignore any unmarshal error, if the data is not a valid object then
	// there is no config.
	json.Unmarshal(([]byte)(r.Config_), &config)
	return config
}

// SetConfig sets the region-specific configuration.
func (r *CloudRegion) SetConfig(config map[string]interface{}) {
	buf, err := json.Marshal(config)
	if err != nil {
		// It should be impossible to fail to marshal the config.
		panic(err)
	}
	r.Config_ = datatypes.JSON(buf)
}

// AuthType is a type of authentication that can be used to authenticate
// to a cloud.
type AuthType struct {
	gorm.Model

	// Name is the name of the authentication type.
	Name string `gorm:"not null;uniqueIndex"`
}

// A UserCloudAccess maps the access level of a user on a cloud.
type UserCloudAccess struct {
	gorm.Model

	// User is the User this access is for.
	UserID uint
	User   User

	// Cloud is the Cloud this access is for.
	CloudID uint
	Cloud   Cloud

	// Access is the access level of the user on the cloud.
	Access string `gorm:"not null"`
}
