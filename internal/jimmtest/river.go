package jimmtest

import (
	"context"

	"github.com/canonical/jimm/internal/db"
	"github.com/canonical/jimm/internal/jimm"
	"github.com/canonical/jimm/internal/openfga"
	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"
)

// PostgresDB returns a PostgreSQL database instance for tests. To improve
// performance it creates a new database from a template (which has no data but
// is already-migrated).
// In cases where you need an entirely empty database, you should use
// `CreateEmptyDatabase` function in this package.
func NewRiver(t Tester, ofgaConn *openfga.OFGAClient, db *db.Database) *jimm.River {
	dsn := getTestDBName(t)
	riverArgs := jimm.NewRiverArgs{
		Config:      nil,
		Db:          db,
		DbUrl:       dsn,
		MaxAttempts: 1, // because this is a unit test
		OfgaClient:  ofgaConn,
	}
	riverClient, err := jimm.NewRiver(context.Background(), riverArgs)
	if err != nil {
		t.Fatalf("failed to create river client")
	}
	t.Cleanup(func() {
		err := riverClient.Cleanup(context.Background())
		if err != nil {
			zapctx.Error(context.Background(), "failed to cleanup river client", zap.Error(err))
		}
	})

	return riverClient
}
