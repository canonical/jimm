// Copyright 2024 Canonical.
package jimmtest

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	cofga "github.com/canonical/ofga"
	"github.com/jackc/pgx/v4"
	"github.com/oklog/ulid/v2"
	sdk "github.com/openfga/go-sdk"
	"gopkg.in/errgo.v1"

	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/openfga"
	auth_model "github.com/canonical/jimm/v3/openfga"
)

var (
	authModel *sdk.AuthorizationModel
	setups    map[string]testSetup
	setupsMu  sync.Mutex
)

func init() {
	setups = make(map[string]testSetup)
}

type testSetup struct {
	client      *openfga.OFGAClient
	cofgaClient *cofga.Client
	cofgaParams *cofga.OpenFGAParams
}

func getAuthModelDefinition() (*sdk.AuthorizationModel, error) {
	authModel := sdk.AuthorizationModel{}
	err := json.Unmarshal(auth_model.AuthModelJSON, &authModel)
	if err != nil {
		return nil, err
	}
	return &authModel, nil
}

// SetupTestOFGAClient is intended to be used per test, in that it
// creates a store based on the current tests name and is expected to
// be a single use instance (due to the initial removing of a store).
//
// The benefit of not cleaning up the store immediately afterwards,
// enables the debugging of created tuples in test development.
func SetupTestOFGAClient(names ...string) (*openfga.OFGAClient, *cofga.Client, *cofga.OpenFGAParams, error) {
	ctx := context.Background()

	testName := strings.NewReplacer(" ", "_", "'", "_").Replace(strings.Join(names, "_"))

	setupsMu.Lock()
	defer setupsMu.Unlock()
	setup, ok := setups[testName]
	if ok {
		return setup.client, setup.cofgaClient, setup.cofgaParams, nil
	}

	err := RemoveStore(ctx, testName)
	if err != nil {
		return nil, nil, nil, err
	}

	storeID := ulid.Make().String()
	err = CreateStore(ctx, testName, storeID)
	if err != nil {
		return nil, nil, nil, err
	}

	cofgaParams := cofga.OpenFGAParams{
		Scheme:  "http",
		Host:    "localhost",
		Token:   "jimm",
		Port:    "8080",
		StoreID: storeID,
	}
	cofgaClient, err := cofga.NewClient(ctx, cofgaParams)
	if err != nil {
		return nil, nil, nil, errgo.Notef(err, "failed to create ofga client")
	}

	if authModel == nil {
		authModel, err = getAuthModelDefinition()
		if err != nil {
			return nil, nil, nil, errgo.Notef(err, "failed to read authorization model definition")
		}
	}

	authModelID, err := cofgaClient.CreateAuthModel(ctx, authModel)
	if err != nil {
		return nil, nil, nil, errgo.Notef(err, "failed to create authorization model")
	}
	cofgaClient.SetAuthModelID(authModelID)

	cofgaParams.AuthModelID = authModelID

	client := openfga.NewOpenFGAClient(cofgaClient)

	setups[testName] = testSetup{
		client:      client,
		cofgaClient: cofgaClient,
		cofgaParams: &cofgaParams,
	}
	return client, cofgaClient, &cofgaParams, nil
}

// RemoveStore removes an OpenFGA store (via db) by NAME.
// Currently, OpenFGA does not support this as it is expected to remove a store by ID.
//
// However, in a testing scenario, we want a simple and quick solution to cleanup
// a store per test,
func RemoveStore(ctx context.Context, name string) error {
	conn, err := pgx.Connect(context.Background(), "postgresql://jimm:jimm@localhost/jimm")
	if err != nil {
		return errors.E(err)
	}
	defer conn.Close(ctx)
	_, err = conn.Exec(ctx, fmt.Sprintf("DELETE FROM authorization_model WHERE store = (SELECT id FROM store WHERE name = '%s')", name))
	if err != nil {
		return errors.E(err)
	}
	_, err = conn.Exec(ctx, fmt.Sprintf("DELETE FROM store WHERE name = '%s';", name))
	if err != nil {
		return errors.E(err)
	}
	return nil
}

// CreateStore adds a store to OpenFGA (via db), circumventing the superficial rules
// set in their server around character limit (64).
func CreateStore(ctx context.Context, name string, id string) error {
	conn, err := pgx.Connect(context.Background(), "postgresql://jimm:jimm@localhost/jimm")
	if err != nil {
		return errors.E(err)
	}
	defer conn.Close(ctx)
	_, err = conn.Exec(
		ctx,
		fmt.Sprintf("INSERT INTO store(id, name, created_at, updated_at) VALUES('%s', '%s', '%s', '%s');",
			id,
			name,
			"2023-01-18 12:14:45.048376+00",
			"2023-01-18 12:14:45.048376+00",
		),
	)
	if err != nil {
		return errors.E(err)
	}
	return nil
}

// TruncateOpenFgaTuples truncates the tuple and changelog tables used by openFGA.
func TruncateOpenFgaTuples(ctx context.Context) error {
	conn, err := pgx.Connect(context.Background(), "postgresql://jimm:jimm@localhost/jimm")
	if err != nil {
		return errors.E(err)
	}
	defer conn.Close(ctx)

	if _, err := conn.Exec(ctx, "TRUNCATE TABLE tuple;"); err != nil {
		return err
	}

	if _, err := conn.Exec(ctx, "TRUNCATE TABLE changelog;"); err != nil {
		return err
	}

	return nil
}
