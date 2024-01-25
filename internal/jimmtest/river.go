package jimmtest

import (
	"context"

	"github.com/canonical/jimm/internal/db"
	"github.com/canonical/jimm/internal/jimm"
	"github.com/canonical/jimm/internal/openfga"
)

// PostgresDB returns a PostgreSQL database instance for tests. To improve
// performance it creates a new database from a template (which has no data but
// is already-migrated).
// In cases where you need an entirely empty database, you should use
// `CreateEmptyDatabase` function in this package.
func NewRiver(t Tester, ofgaConn *openfga.OFGAClient, db db.Database) *jimm.River {
	dsn := getTestDBName(t)

	riverClient, err := jimm.NewRiver(context.Background(), nil, dsn, ofgaConn, db)
	if err != nil {
		t.Fatalf("failed to create river client")
	}
	t.Cleanup(func() {
		riverClient.Cleanup(context.Background())
	})

	return riverClient
}
