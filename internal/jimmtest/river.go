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
func NewRiver(t Tester, riverConfig *jimm.RiverConfig, ofgaConn *openfga.OFGAClient, db *db.Database) *jimm.River {
	dsn := getTestDBName(t)
	if riverConfig == nil {
		riverConfig = &jimm.RiverConfig{
			Config: nil,
			DbUrl:  dsn,
		}
	}
	riverClient, err := jimm.NewRiver(context.Background(), *riverConfig, ofgaConn, db)
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
