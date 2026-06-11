// Copyright 2025 Canonical.

package juju_test

import (
	"context"
	"time"

	qt "github.com/frankban/quicktest"
	"github.com/google/uuid"
	"github.com/juju/names/v5"

	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/jimm/credentials"
	"github.com/canonical/jimm/v3/internal/jimm/juju"
	"github.com/canonical/jimm/v3/internal/jimm/permissions"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
	"github.com/canonical/jimm/v3/internal/testutils/testdb"
)

type parameters struct {
	Dialer                 juju.Dialer
	CredentialStore        credentials.CredentialStore
	CrossModelQueryTimeout time.Duration
}

// newTestJujuManager creates a new JujuManager for testing purposes.
//
// TODO: Return a struct that includes the JujuManager and any mock
// resources needed for validation during testing.
func newTestJujuManager(c *qt.C, p *parameters) *juju.JujuManager {
	if p == nil {
		p = &parameters{}
	}

	if p.CrossModelQueryTimeout <= 0 {
		p.CrossModelQueryTimeout = time.Second * 5
	}
	db := &db.Database{
		DB: testdb.PostgresDB(c, func() time.Time { return now }),
	}
	err := db.Migrate(context.Background())
	c.Assert(err, qt.IsNil)

	ofgaClient, _, _, err := jimmtest.SetupTestOFGAClient(c.Name())
	if err != nil {
		c.Fatalf("setting up openfga client: %v", err)
	}

	jimmUUID := uuid.NewString()
	jimmResourceTag := names.NewControllerTag(jimmUUID)

	permissionManager, err := permissions.NewManager(db, ofgaClient, jimmUUID, jimmResourceTag)
	c.Assert(err, qt.IsNil)

	if p.CredentialStore == nil {
		p.CredentialStore = db
	}
	if p.Dialer == nil {
		p.Dialer = &jimmtest.Dialer{}
	}

	jujuManager, err := juju.NewJujuManager(db, ofgaClient,
		p.CredentialStore, permissionManager,
		jimmResourceTag, []string{},
		p.Dialer, p.CrossModelQueryTimeout,
		&mockMigrationTokenGenerator{})
	c.Assert(err, qt.IsNil)

	return jujuManager
}

type mockMigrationTokenGenerator struct{}

func (m *mockMigrationTokenGenerator) NewMigrationToken(ctx context.Context, username string, groups []string) (string, error) {
	// Simulate a token generation by returning a simple string.
	// In a real implementation, this would be a JWT or similar token.
	return "test-migration-token", nil
}
