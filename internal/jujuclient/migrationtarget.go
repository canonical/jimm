// Copyright 2025 Canonical.

package jujuclient

import (
	"context"
	"time"

	"github.com/juju/juju/api/controller/migrationtarget"
	coremigration "github.com/juju/juju/core/migration"
	"github.com/juju/juju/core/semversion"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v6"
)

// PreChecks checks that the target controller is able to accept the
// model being migrated.
//
// This method uses a raw facade call because Juju's API client accepts
// a `github.com/juju/description` type rather than a byte slice for
// the model description, and we need to be able to accept different version
// of the description depending on the target controller version.
func (c Connection) Prechecks(ctx context.Context, model jujuparams.MigrationModelInfo) error {

	args := jujuparams.MigrationModelInfo{
		UUID:                   model.UUID,
		Name:                   model.Name,
		Qualifier:              model.Qualifier,
		AgentVersion:           model.AgentVersion,
		ControllerAgentVersion: model.ControllerAgentVersion,
		FacadeVersions:         model.FacadeVersions,
		ModelDescription:       model.ModelDescription,
	}
	if err := c.CallHighestFacadeVersion(ctx, "MigrationTarget", []int{7, 6}, "", "Prechecks", &args, nil); err != nil {
		return err
	}
	return nil
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
func (c Connection) AdoptResources(ctx context.Context, modelUUID string, controllerVersion semversion.Number) error {
	args := jujuparams.AdoptResourcesArgs{
		ModelTag:                names.NewModelTag(modelUUID).String(),
		SourceControllerVersion: controllerVersion,
	}
	if err := c.CallHighestFacadeVersion(ctx, "MigrationTarget", []int{7, 6}, "", "AdoptResources", &args, nil); err != nil {
		return err
	}
	return nil
}

// Abort aborts a model migration.
func (c Connection) Abort(ctx context.Context, modelUUID string) error {
	migrationTarget := migrationtarget.NewClient(&c)
	return migrationTarget.Abort(ctx, modelUUID)
}

// CheckMachines compares the machines in state with the ones reported by the provider and reports any discrepancies.
func (c Connection) CheckMachines(ctx context.Context, modelUUID string) ([]error, error) {
	migrationTarget := migrationtarget.NewClient(&c)
	return migrationTarget.CheckMachines(ctx, modelUUID)
}

// Activate activates a model on the controller.
func (c Connection) Activate(ctx context.Context, modelUUID string, sourceInfo coremigration.SourceControllerInfo, relatedModels []string) error {
	return migrationtarget.NewClient(&c).Activate(ctx, modelUUID, sourceInfo, relatedModels)
}

// LatestLogTime asks the target controller for the time of the latest
// log record it has seen. This can be used to make the log transfer
// restartable.
func (c Connection) LatestLogTime(ctx context.Context, modelUUID string) (time.Time, error) {
	migrationTarget := migrationtarget.NewClient(&c)
	return migrationTarget.LatestLogTime(ctx, modelUUID)
}

// Import imports a model migration from the given bytes of the
// serialized model description.
//
// This method uses a raw facade call because Juju 4.1+ registers
// MigrationTarget v8, whose Import method takes a SerializedModelV2
// envelope rather than the legacy SerializedModel. The legacy import
// path is still served by facade versions 6 and 7, so we call those
// explicitly to avoid the client negotiating v8 against newer
// controllers.
func (c Connection) Import(ctx context.Context, bytes []byte) error {
	args := jujuparams.SerializedModel{Bytes: bytes}
	if err := c.CallHighestFacadeVersion(ctx, "MigrationTarget", []int{7, 6}, "", "Import", &args, nil); err != nil {
		return err
	}
	return nil
}
