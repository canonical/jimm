// Copyright 2025 Canonical.

package jujuclient

import (
	"time"

	"github.com/juju/juju/api/controller/migrationtarget"
	"github.com/juju/juju/core/migration"
	coremigration "github.com/juju/juju/core/migration"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v5"
	"github.com/juju/version/v2"
)

// PreChecks checks that the target controller is able to accept the
// model being migrated.
func (c Connection) Prechecks(model migration.ModelInfo) error {
	migrationTarget := migrationtarget.NewClient(&c)
	return migrationTarget.Prechecks(model)
}

// AdoptResources asks the cloud provider to update the controller
// tags for a model's resources. This prevents the resources from
// being destroyed if the source controller is destroyed after the
// model is migrated away.
//
// Note that we can't use the migrationTarget client here because it
// fetches the SourceControllerVersion from a global var based on the
// Juju version we are using, which doesn't work for JIMM since we
// want to use the controller version that was passed to us.
func (c Connection) AdoptResources(modelUUID string, controllerVersion version.Number) error {
	args := jujuparams.AdoptResourcesArgs{
		ModelTag:                names.NewModelTag(modelUUID).String(),
		SourceControllerVersion: controllerVersion,
	}
	if err := c.CallHighestFacadeVersion(c.Context(), "MigrationTarget", []int{1, 2, 3, 4}, "", "AdoptResources", &args, nil); err != nil {
		return err
	}
	return nil
}

// Abort aborts a model migration.
func (c Connection) Abort(modelUUID string) error {
	migrationTarget := migrationtarget.NewClient(&c)
	return migrationTarget.Abort(modelUUID)
}

// CheckMachines compares the machines in state with the ones reported by the provider and reports any discrepancies.
func (c Connection) CheckMachines(modelUUID string) ([]error, error) {
	migrationTarget := migrationtarget.NewClient(&c)
	return migrationTarget.CheckMachines(modelUUID)
}

// Activate activates a model on the controller.
func (c Connection) Activate(modelUUID string, sourceInfo coremigration.SourceControllerInfo, relatedModels []string) error {
	return migrationtarget.NewClient(&c).Activate(modelUUID, sourceInfo, relatedModels)
}

// LatestLogTime asks the target controller for the time of the latest
// log record it has seen. This can be used to make the log transfer
// restartable.
func (c Connection) LatestLogTime(modelUUID string) (time.Time, error) {
	migrationTarget := migrationtarget.NewClient(&c)
	return migrationTarget.LatestLogTime(modelUUID)
}
