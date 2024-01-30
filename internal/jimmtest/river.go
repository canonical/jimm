package jimmtest

import (
	"context"

	"github.com/canonical/jimm/internal/db"
	"github.com/canonical/jimm/internal/jimm"
	"github.com/canonical/jimm/internal/openfga"
	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"
)

// NewRiver returns a River instance for tests.
func NewRiver(t Tester, ofgaConn *openfga.OFGAClient, db *db.Database) *jimm.River {
	dsn := getTestDBName(t)
	riverConfig := jimm.RiverConfig{
		Config:      nil,
		Db:          db,
		DbUrl:       dsn,
		MaxAttempts: 1, // because this is a unit test
		OfgaClient:  ofgaConn,
	}
	riverClient, err := jimm.NewRiver(context.Background(), riverConfig)
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
