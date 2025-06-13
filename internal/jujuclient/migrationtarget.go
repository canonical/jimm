package jujuclient

import (
	"github.com/juju/juju/api/controller/migrationtarget"
	"github.com/juju/juju/core/migration"
)

// PreChecks checks that the target controller is able to accept the
// model being migrated.
func (c Connection) Prechecks(model migration.ModelInfo) error {
	migrationTarget := migrationtarget.NewClient(&c)
	return migrationTarget.Prechecks(model)
}
