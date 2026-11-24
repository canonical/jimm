// Copyright 2026 Canonical.

package dbmodel

import "time"

// EntityMetadataTarget identifies a model or an application within a model.
// Entity is the model UUID for models and the application name for applications.
type EntityMetadataTarget struct {
	EntityType string `gorm:"not null"`
	ModelUUID  string `gorm:"not null"`
	Entity     string `gorm:"not null"`
}

// EntityMetadata stores one user-managed metadata key and value.
type EntityMetadata struct {
	ID        uint `gorm:"primarykey"`
	CreatedAt time.Time
	UpdatedAt time.Time

	EntityMetadataTarget `gorm:"embedded"`
	Key                  string `gorm:"not null"`
	Value                string `gorm:"not null"`
}
