// Copyright 2020 Canonical Ltd.

package dbmodel

import (
	"time"

	jujuparams "github.com/juju/juju/apiserver/params"
	"github.com/juju/names/v4"
	"gorm.io/gorm"
)

// A Cloud represents a cloud service.
type Cloud struct {
	ID        uint `gorm:"primarykey"`
	CreatedAt time.Time
	UpdatedAt time.Time

	// Name is the name of the cloud.
	Name string `gorm:"not null;uniqueIndex"`

	// Type is the provider type of cloud.
	Type string `gorm:"not null"`

	// HostCloudRegion is the "cloud/region" that hosts this cloud, if the
	// cloud is hosted.
	HostCloudRegion string

	// AuthTypes is the authentication types supported by this cloud.
	AuthTypes Strings

	// Endpoint is the API endpoint URL for the cloud.
	Endpoint string

	// IdentityEndpoint is the API endpoint URL of the cloud identity
	// service.
	IdentityEndpoint string

	// StorageEndpoint is the API endpoint URL of the cloud storage
	// service.
	StorageEndpoint string

	// Regions contains the regions associated with this cloud.
	Regions []CloudRegion `gorm:"foreignKey:CloudName;references:Name"`

	// CACertificates contains the CA Certificates associated with this
	// cloud.
	CACertificates Strings

	// Config contains the configuration associated with this cloud.
	Config Map

	// Users contains the users that are authorized on this cloud.
	Users []UserCloudAccess `gorm:"foreignkey:CloudName;references:Name"`
}

// Tag returns a names.Tag for this cloud.
func (c Cloud) Tag() names.Tag {
	return names.NewCloudTag(c.Name)
}

// SetTag sets the name of the cloud to the value from the given cloud tag.
func (c *Cloud) SetTag(t names.CloudTag) {
	c.Name = t.Id()
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

// WriteCloud writes the Cloud object into a jujuparams.Cloud. The cloud
// must have its regions association filled out.
func (c Cloud) WriteCloud(cl *jujuparams.Cloud) {
	cl.Type = c.Type
	cl.HostCloudRegion = c.HostCloudRegion
	cl.AuthTypes = []string(c.AuthTypes)
	cl.Endpoint = c.Endpoint
	cl.IdentityEndpoint = c.IdentityEndpoint
	cl.StorageEndpoint = c.StorageEndpoint
	cl.Regions = make([]jujuparams.CloudRegion, len(c.Regions))
	cl.RegionConfig = make(map[string]map[string]interface{}, len(c.Regions))
	for i, r := range c.Regions {
		r.WriteCloudRegion(&cl.Regions[i])
		r.WriteCloudRegionConfig(cl.RegionConfig)
	}
	cl.CACertificates = []string(c.CACertificates)
	cl.Config = map[string]interface{}(c.Config)
}

// WriteCloudDetails writes the Cloud object into a
// jujuparams.CloudDetails. The cloud must have its regions association
// filled out.
func (c Cloud) WriteCloudDetails(cd *jujuparams.CloudDetails) {
	cd.Type = c.Type
	cd.AuthTypes = []string(c.AuthTypes)
	cd.Endpoint = c.Endpoint
	cd.IdentityEndpoint = c.IdentityEndpoint
	cd.StorageEndpoint = c.StorageEndpoint
	cd.Regions = make([]jujuparams.CloudRegion, len(c.Regions))
	for i, r := range c.Regions {
		r.WriteCloudRegion(&cd.Regions[i])
	}
}

// WriteCloudInfo writes the Cloud object into a
// jujuparams.CloudInfo. The cloud must have its regions and users
// associations filled out.
func (c Cloud) WriteCloudInfo(ci *jujuparams.CloudInfo) {
	c.WriteCloudDetails(&ci.CloudDetails)
	ci.Users = make([]jujuparams.CloudUserInfo, len(c.Users))
	for i, u := range c.Users {
		u.WriteCloudUserInfo(&ci.Users[i])
	}
}

// A CloudRegion is a region of a cloud.
type CloudRegion struct {
	gorm.Model

	// Cloud is the cloud this region belongs to.
	CloudName string `gorm:"uniqueIndex:idx_cloud_region_cloud_name_name"`
	Cloud     Cloud  `gorm:"foreignKey:CloudName;references:Name"`

	// Name is the name of the region.
	Name string `gorm:"not null;uniqueIndex:idx_cloud_region_cloud_name_name"`

	// Endpoint is the API endpoint URL for the region.
	Endpoint string

	// IdentityEndpoint is the API endpoint URL of the region identity
	// service.
	IdentityEndpoint string

	// StorageEndpoint is the API endpoint URL of the region storage
	// service.
	StorageEndpoint string

	// Config contains the configuration associated with this region.
	Config Map

	// Controllers contains any controllers that can provide service for
	// this cloud-region.
	Controllers []CloudRegionControllerPriority
}

// WriteCloudRegion writes a CloudRegion into a jujuparams.CloudRegion.
func (r CloudRegion) WriteCloudRegion(cr *jujuparams.CloudRegion) {
	cr.Name = r.Name
	cr.Endpoint = r.Endpoint
	cr.IdentityEndpoint = r.IdentityEndpoint
	cr.StorageEndpoint = r.StorageEndpoint
}

// WriteCloudRegionConfig writes the configuration from the CloudRegion
// into the given config map.
func (r CloudRegion) WriteCloudRegionConfig(cfg map[string]map[string]interface{}) {
	cfg[r.Name] = map[string]interface{}(r.Config)
}

// A UserCloudAccess maps the access level of a user on a cloud.
type UserCloudAccess struct {
	gorm.Model

	// User is the User this access is for.
	Username string `gorm:"uniqueIndex:idx_user_cloud_accesses_username_cloud_name"`
	User     User   `gorm:"foreignKey:Username;references:Username"`

	// Cloud is the Cloud this access is for.
	CloudName string `gorm:"uniqueIndex:idx_user_cloud_accesses_username_cloud_name"`
	Cloud     Cloud  `gorm:"foreignKey:CloudName;references:Name;constraint:OnDelete:CASCADE"`

	// Access is the access level of the user on the cloud.
	Access string `gorm:"not null"`
}

func (a UserCloudAccess) WriteCloudUserInfo(cui *jujuparams.CloudUserInfo) {
	cui.UserName = a.User.Username
	cui.DisplayName = a.User.DisplayName
	cui.Access = a.Access
}
