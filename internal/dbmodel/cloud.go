// Copyright 2020 Canonical Ltd.

package dbmodel

import (
	"time"

	jujuparams "github.com/juju/juju/rpc/params"
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
}

// Tag returns a names.Tag for this cloud.
func (c Cloud) Tag() names.Tag {
	return c.ResourceTag()
}

// ResourceTag returns a tag for this cloud.  This method
// is intended to be used in places where we expect to see
// a concrete type names.CloudTag instead of the
// names.Tag interface.
func (c Cloud) ResourceTag() names.CloudTag {
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

// ToJujuCloud converts the  Cloud object into a jujuparams.Cloud. The
// cloud must have its regions association filled out.
func (c Cloud) ToJujuCloud() jujuparams.Cloud {
	var cl jujuparams.Cloud
	cl.Type = c.Type
	cl.HostCloudRegion = c.HostCloudRegion
	cl.AuthTypes = []string(c.AuthTypes)
	cl.Endpoint = c.Endpoint
	cl.IdentityEndpoint = c.IdentityEndpoint
	cl.StorageEndpoint = c.StorageEndpoint
	cl.Regions = make([]jujuparams.CloudRegion, len(c.Regions))
	cl.RegionConfig = make(map[string]map[string]interface{}, len(c.Regions))
	for i, r := range c.Regions {
		cl.Regions[i] = r.ToJujuCloudRegion()
		if r.Config != nil {
			cl.RegionConfig[r.Name] = map[string]interface{}(r.Config)
		}
	}
	cl.CACertificates = []string(c.CACertificates)
	cl.Config = map[string]interface{}(c.Config)
	return cl
}

// FromJujuCloud updates a Cloud object with the details from the given
// jujuparams.Cloud.
func (c *Cloud) FromJujuCloud(cld jujuparams.Cloud) {
	c.Type = cld.Type
	c.HostCloudRegion = cld.HostCloudRegion
	c.AuthTypes = Strings(cld.AuthTypes)
	c.Endpoint = cld.Endpoint
	c.IdentityEndpoint = cld.IdentityEndpoint
	c.StorageEndpoint = cld.StorageEndpoint
	c.CACertificates = Strings(cld.CACertificates)
	c.Config = Map(cld.Config)
	regions := make([]CloudRegion, 0, len(c.Regions))
	for _, r := range cld.Regions {
		reg := c.Region(r.Name)
		reg.FromJujuCloudRegion(r)
		reg.Config = Map(cld.RegionConfig[r.Name])
		regions = append(regions, reg)
	}
	c.Regions = regions
}

// ToJujuCloudDetails converts the Cloud object into a
// jujuparams.CloudDetails. The cloud must have its regions association
// filled out.
func (c Cloud) ToJujuCloudDetails() jujuparams.CloudDetails {
	var cd jujuparams.CloudDetails
	cd.Type = c.Type
	cd.AuthTypes = []string(c.AuthTypes)
	cd.Endpoint = c.Endpoint
	cd.IdentityEndpoint = c.IdentityEndpoint
	cd.StorageEndpoint = c.StorageEndpoint
	cd.Regions = make([]jujuparams.CloudRegion, len(c.Regions))
	for i, r := range c.Regions {
		cd.Regions[i] = r.ToJujuCloudRegion()
	}
	return cd
}

// ToJujuCloudInfo converts the Cloud object into a
// jujuparams.CloudInfo. The cloud must have its regions and users
// associations filled out.
func (c Cloud) ToJujuCloudInfo() jujuparams.CloudInfo {
	var ci jujuparams.CloudInfo
	ci.CloudDetails = c.ToJujuCloudDetails()
	// TODO(Kian) CSS-6040 Determine whether to combine OpenFGA Tuples
	// with Postgres data objects for a consolidated view.
	ci.Users = nil
	return ci
}

// A CloudRegion is a region of a cloud.
type CloudRegion struct {
	gorm.Model

	// Cloud is the cloud this region belongs to.
	CloudName string `gorm:"uniqueIndex:idx_cloud_region_cloud_name_name"`
	Cloud     Cloud  `gorm:"foreignKey:CloudName;references:Name;constraint:OnDelete:CASCADE"`

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

// ToJujuCloudRegion converts a CloudRegion into a jujuparams.CloudRegion.
func (r CloudRegion) ToJujuCloudRegion() jujuparams.CloudRegion {
	var cr jujuparams.CloudRegion
	cr.Name = r.Name
	cr.Endpoint = r.Endpoint
	cr.IdentityEndpoint = r.IdentityEndpoint
	cr.StorageEndpoint = r.StorageEndpoint
	return cr
}

// FromJujuCloudRegion updates a CloudRegion object with the details from
// the given jujuparams.CloudRegion.
func (cr *CloudRegion) FromJujuCloudRegion(r jujuparams.CloudRegion) {
	cr.Name = r.Name
	cr.Endpoint = r.Endpoint
	cr.IdentityEndpoint = r.IdentityEndpoint
	cr.StorageEndpoint = r.StorageEndpoint
}
