// Copyright 2026 Canonical.

package dbmodel

import (
	"strings"
	"time"
)

// ControllerProfile stores reusable, non-secret controller bootstrap settings.
//
// Version is incremented each time the profile is replaced. Callers must pass
// the last version they read when updating an existing profile. A zero value is
// used when creating a new profile.
type ControllerProfile struct {
	ID        uint `gorm:"primarykey"`
	CreatedAt time.Time
	UpdatedAt time.Time

	Name        string `gorm:"not null;uniqueIndex"`
	Description string
	JujuVersion string `gorm:"column:juju_version;not null"`
	Version     uint   `gorm:"not null"`

	Cloud            ControllerProfileCloud            `gorm:"embedded"`
	BootstrapOptions ControllerProfileBootstrapOptions `gorm:"embedded"`
}

// PartialJujuVersionPrefixes returns the progressive prefixes of a Juju
// version string. Callers are expected to validate the version before use if
// needed.
func PartialJujuVersionPrefixes(version string) []string {
	if version == "" {
		return nil
	}
	parts := strings.Split(version, ".")
	prefixes := make([]string, 0, len(parts))
	for i := 1; i <= len(parts); i++ {
		prefixes = append(prefixes, strings.Join(parts[:i], "."))
	}
	return prefixes
}

// ControllerProfileCloud stores the cloud definition persisted in a controller
// profile.
type ControllerProfileCloud struct {
	Name            string                       `gorm:"column:cloud_name;not null"`
	Type            string                       `gorm:"column:cloud_type"`
	AuthTypes       Strings                      `gorm:"column:cloud_auth_types"`
	CACertificates  Strings                      `gorm:"column:cloud_ca_certificates"`
	Config          Map                          `gorm:"column:cloud_config"`
	Endpoint        string                       `gorm:"column:cloud_endpoint"`
	HostCloudRegion string                       `gorm:"column:cloud_host_cloud_region"`
	Region          ControllerProfileCloudRegion `gorm:"embedded"`
}

// ControllerProfileCloudRegion stores the single bootstrap region definition
// persisted for a controller profile.
type ControllerProfileCloudRegion struct {
	Name             string `gorm:"column:cloud_region_name;not null"`
	Endpoint         string `gorm:"column:cloud_region_endpoint"`
	IdentityEndpoint string `gorm:"column:cloud_region_identity_endpoint"`
	StorageEndpoint  string `gorm:"column:cloud_region_storage_endpoint"`
}

// ControllerProfileBootstrapOptions stores the reusable bootstrap settings
// supported by a controller profile.
type ControllerProfileBootstrapOptions struct {
	BootstrapBase         string                       `gorm:"column:bootstrap_base"`
	BootstrapConstraints  StringMap                    `gorm:"column:bootstrap_constraints"`
	ModelConstraints      StringMap                    `gorm:"column:model_constraints"`
	ModelDefault          StringMap                    `gorm:"column:model_default"`
	StoragePool           ControllerProfileStoragePool `gorm:"embedded"`
	BootstrapConfig       StringMap                    `gorm:"column:bootstrap_config"`
	ControllerConfig      StringMap                    `gorm:"column:controller_config"`
	ControllerModelConfig StringMap                    `gorm:"column:controller_model_config"`
}

// ControllerProfileStoragePool stores the optional storage pool configuration
// persisted for a controller profile.
type ControllerProfileStoragePool struct {
	Name       string    `gorm:"column:storage_pool_name"`
	Type       string    `gorm:"column:storage_pool_type"`
	Attributes StringMap `gorm:"column:storage_pool_attributes"`
}
