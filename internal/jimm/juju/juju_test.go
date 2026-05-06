// Copyright 2025 Canonical.

package juju_test

import (
	"context"
	"encoding/base64"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
	"github.com/google/uuid"
	"github.com/juju/names/v5"

	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/jimm/credentials"
	"github.com/canonical/jimm/v3/internal/jimm/juju"
	"github.com/canonical/jimm/v3/internal/jimm/permissions"
	"github.com/canonical/jimm/v3/internal/jimmjwx"
	"github.com/canonical/jimm/v3/internal/jujuclient"
	"github.com/canonical/jimm/v3/internal/openfga"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest/mocks"
	"github.com/canonical/jimm/v3/internal/testutils/testdb"
)

type parameters struct {
	Dialer                 juju.Dialer
	CredentialStore        credentials.CredentialStore
	CrossModelQueryTimeout time.Duration
	JWTService             jujuclient.JWTMinter
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
	if p.JWTService == nil {
		p.JWTService = mocks.JWTService{}
	}

	jujuManager, err := juju.NewJujuManager(db, ofgaClient,
		p.CredentialStore, p.JWTService, permissionManager,
		jimmResourceTag, []string{},
		p.Dialer, p.CrossModelQueryTimeout,
		&mockMigrationTokenGenerator{})
	c.Assert(err, qt.IsNil)

	return jujuManager
}

type mockMigrationTokenGenerator struct{}

func (m *mockMigrationTokenGenerator) NewMigrationToken(ctx context.Context, username string) (string, error) {
	// Simulate a token generation by returning a simple string.
	// In a real implementation, this would be a JWT or similar token.
	return "test-migration-token", nil
}

func TestControllerSuperuserAuthorizationHeader(t *testing.T) {
	c := qt.New(t)
	const controllerUUID = "00000001-0000-0000-0000-000000000001"
	const modelUUID = "00000002-0000-0000-0000-000000000001"

	user := openfga.NewUser(&dbmodel.Identity{Name: "alice@canonical.com"}, nil)

	tests := []struct {
		name           string
		modelTag       names.ModelTag
		expectedAccess map[string]string
	}{
		{
			name:     "controller and model",
			modelTag: names.NewModelTag(modelUUID),
			expectedAccess: map[string]string{
				names.NewControllerTag(controllerUUID).String(): "superuser",
				names.NewModelTag(modelUUID).String():           "admin",
			},
		},
		{
			name:     "controller only",
			modelTag: names.ModelTag{},
			expectedAccess: map[string]string{
				names.NewControllerTag(controllerUUID).String(): "superuser",
			},
		},
	}

	for _, test := range tests {
		c.Run(test.name, func(c *qt.C) {
			var gotJWTParams jimmjwx.JWTParams
			j := &juju.JujuManager{JWTService: mocks.JWTService{NewJWT_: func(ctx context.Context, params jimmjwx.JWTParams) ([]byte, error) {
				gotJWTParams = params
				return []byte("test-token"), nil
			}}}

			header, err := j.ControllerSuperuserAuthorizationHeader(c.Context(), controllerUUID, test.modelTag, user)
			c.Assert(err, qt.IsNil)
			c.Assert(header.Get("Authorization"), qt.Equals, "Bearer "+base64.StdEncoding.EncodeToString([]byte("test-token")))
			c.Assert(gotJWTParams, qt.DeepEquals, jimmjwx.JWTParams{
				Controller: controllerUUID,
				User:       names.NewUserTag("alice@canonical.com").String(),
				Access:     test.expectedAccess,
			})
		})
	}
}
