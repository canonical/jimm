// Copyright 2026 Canonical.

package river

import (
	"context"
	"database/sql"
	"time"

	qt "github.com/frankban/quicktest"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverdatabasesql"
	"github.com/riverqueue/river/rivertype"
	gomock "go.uber.org/mock/gomock"

	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/openfga"
	"github.com/canonical/jimm/v3/internal/testutils/testdb"
)

func setupTestDB(c *qt.C) (*db.Database, *sql.DB) {
	db := &db.Database{
		DB: testdb.PostgresDB(c, time.Now),
	}
	err := db.Migrate(c.Context())
	c.Assert(err, qt.IsNil)

	err = MigrateRiver(c.Context(), db)
	c.Assert(err, qt.IsNil)

	sqlDB, err := db.SqlDB()
	c.Assert(err, qt.IsNil)
	c.Cleanup(func() {
		c.Check(sqlDB.Close(), qt.IsNil)
	})

	return db, sqlDB
}

type setupWorkerParams struct {
	migrateRetryCount int
	upgradeRetryCount int
	awaitFunc         awaitCompletionFunc
}

type testDeps struct {
	db                   *db.Database
	sqlDB                *sql.DB
	riverClient          *river.Client[*sql.Tx]
	mockUpgradeManager   *MockUpgradeManager
	mockBootstrapManager *MockBootstrapManager
	identity             string
}

func setupIntegrationTest(
	c *qt.C,
	p setupWorkerParams,
) testDeps {
	ctrl := gomock.NewController(c)
	c.Cleanup(ctrl.Finish)

	database, sqlDb := setupTestDB(c)

	upgradeManager := NewMockUpgradeManager(ctrl)
	bootstrapManager := NewMockBootstrapManager(ctrl)

	// Prepare identity needed by migrationWorker.
	u, err := dbmodel.NewIdentity("ash@catchum.com")
	c.Assert(err, qt.IsNil)
	err = database.GetIdentity(c.Context(), u)
	c.Assert(err, qt.IsNil)

	openfgaClient := &openfga.OFGAClient{}
	workerParams := workerParams{
		migrateRetryCount: p.migrateRetryCount,
		upgradeRetryCount: p.upgradeRetryCount,
		awaitFunc:         p.awaitFunc,
		openfgaClient:     openfgaClient,
		store:             database,
		upgradeManager:    upgradeManager,
		bootstrapManager:  bootstrapManager,
	}
	workers, err := newWorkers(workerParams)
	c.Assert(err, qt.IsNil)

	riverClient, err := river.NewClient(riverdatabasesql.New(sqlDb), &river.Config{
		TestOnly: true,
		Queues: map[string]river.QueueConfig{
			river.QueueDefault: {MaxWorkers: 5},
		},
		Workers:     workers,
		RetryPolicy: &testRetryPolicy{},
	})
	c.Assert(err, qt.IsNil)

	c.Assert(riverClient.Start(c.Context()), qt.IsNil)
	c.Cleanup(func() {
		err := riverClient.Stop(context.Background())
		c.Check(err, qt.IsNil)
	})

	return testDeps{
		db:                   database,
		sqlDB:                sqlDb,
		riverClient:          riverClient,
		mockUpgradeManager:   upgradeManager,
		mockBootstrapManager: bootstrapManager,
		identity:             u.Name,
	}
}

type testRetryPolicy struct{}

// NextRetry implements the [river.ClientRetryPolicy] interface.
// It ensures retries happen quickly during tests.
func (p *testRetryPolicy) NextRetry(job *rivertype.JobRow) time.Time {
	return time.Now().Add(1 * time.Millisecond)
}

func waitForFinalisedJob(c *qt.C, ctx context.Context, sub <-chan *river.Event, jobID int64) *rivertype.JobRow {
	for {
		select {
		case event := <-sub:
			c.Logf("received job event for job ID %d", event.Job.ID)
			if event.Job.ID == jobID && event.Job.FinalizedAt != nil {
				return event.Job
			}
		case <-ctx.Done():
			c.Fatal("timed out waiting for job event")
		}
	}
}

func waitForJobState(c *qt.C, ctx context.Context, riverClient *river.Client[*sql.Tx], jobID int64, target rivertype.JobState, kind string) {
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()

	params := river.NewJobListParams().Kinds(kind).First(10)
	for {
		select {
		case <-ticker.C:
			listRes, err := riverClient.JobList(ctx, params)
			c.Assert(err, qt.IsNil)
			for _, job := range listRes.Jobs {
				if job.ID == jobID {
					if job.State == target {
						return
					}
				}
			}
		case <-ctx.Done():
			c.Fatal("timed out waiting for job state")
		}
	}
}
