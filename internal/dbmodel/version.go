// Copyright 2020 Canonical Ltd.

// Package dbmodel contains the model objects for the relational storage
// database.
package dbmodel

const (
	// Component is the component name in the version table for th
	Component = "jimmdb"

	// Major is the major version of the model described in the dbmodel
	// package. It should be incremented if the database model is modified
	// in a way that is not backwards-compatible. That is, a column or
	// table is added or changed in such a way that the default behaviour
	// that would occur with a previous version of the package would break
	// the data model. If this is incremented the Minor version should be
	// reset to 0.
	Major = 1

	// Minor is the minor version of the model described in the dbmodel
	// package. It should be incremented for any change made to the
	// database model from database model in a released JIMM.
	Minor = 10
)

type Version struct {
	// Component represents the component that the stored version number
	// is for. Currently there is only one known component "jimmdb" it
	// mostly exists for the purposes of there being a primary key on the
	// database table.
	Component string `gorm:"primaryKey"`

	// Major is the stored major version.
	Major int

	// Minor is the stored minor version.
	Minor int
}
