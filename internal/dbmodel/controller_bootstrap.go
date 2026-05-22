// Copyright 2026 Canonical.

package dbmodel

import (
	"database/sql"
	"time"

	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v5"

	apiparams "github.com/canonical/jimm/v3/pkg/api/params"
)

// ControllerBootstrap reserves a controller name while a bootstrap job is in progress.
type ControllerBootstrap struct {
	ID        uint `gorm:"primarykey"`
	CreatedAt time.Time
	UpdatedAt time.Time

	Name        string `gorm:"not null;uniqueIndex"`
	CloudName   string
	CloudRegion string
	JobID       sql.NullInt64 `gorm:"column:job_id;uniqueIndex"`
}

// ToAPIControllerInfo converts a pending bootstrap entry to controller info for list/show APIs.
func (c ControllerBootstrap) ToAPIControllerInfo() apiparams.ControllerInfo {
	ci := apiparams.ControllerInfo{
		Name:        c.Name,
		CloudRegion: c.CloudRegion,
		Status: jujuparams.EntityStatus{
			Status: "bootstrapping",
		},
	}
	if c.CloudName != "" {
		ci.CloudTag = names.NewCloudTag(c.CloudName).String()
	}
	return ci
}
