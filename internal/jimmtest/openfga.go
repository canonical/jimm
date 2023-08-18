package jimmtest

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"

	cofga "github.com/canonical/ofga"
	"github.com/jackc/pgx/v4"
	"github.com/oklog/ulid/v2"
	openfga "github.com/openfga/go-sdk"
	"gopkg.in/errgo.v1"

	"github.com/canonical/jimm/internal/errors"
	ofga "github.com/canonical/jimm/internal/openfga"
)

var (
	authTypeDefinitions []openfga.TypeDefinition
	setups              map[string]testSetup
	setupsMu            sync.Mutex
)

func init() {
	setups = make(map[string]testSetup)
}

type testSetup struct {
	client      *ofga.OFGAClient
	cofgaClient *cofga.Client
	cofgaParams *cofga.OpenFGAParams
}

func getAuthModelDefinition() (authDefinitions []openfga.TypeDefinition, schemaVersion string, err error) {
	desiredFolder := "local"
	authPath := ""
	var pwd string
	pwd, err = os.Getwd()
	if err != nil {
		return
	}
	for ok := true; ok; {
		if pwd == "/" {
			break
		}
		var files []fs.FileInfo
		files, err = ioutil.ReadDir(pwd)
		if err != nil {
			return
		}

		for _, f := range files {
			if f.Name() == desiredFolder {
				ok = true
				authPath = pwd
			}
		}
		// Move up a directory
		pwd = filepath.Dir(pwd)
	}
	if authPath == "" {
		err = fmt.Errorf("auth path is empty")
		return
	}

	b, err := os.ReadFile(path.Join(authPath, "/local/openfga/authorisation_model.json"))
	if err != nil {
		return
	}

	wrapper := map[string]interface{}{}
	err = json.Unmarshal(b, &wrapper)
	if err != nil {
		return
	}

	// TODO (babakks): If we can omit schema_version, these should be deleted:
	var ok bool
	schemaVersion, ok = wrapper["schema_version"].(string)
	if !ok {
		err = errors.E("schema_version not found in auth model")
		return
	}

	b, err = json.Marshal(wrapper["type_definitions"])
	if err != nil {
		return
	}

	err = json.Unmarshal(b, &authDefinitions)
	if err != nil {
		return
	}
	return
}

// SetupTestOFGAClient is intended to be used per test, in that it
// creates a store based on the current tests name and is expected to
// be a single use instance (due to the initial removing of a store).
//
// The benefit of not cleaning up the store immediately afterwards,
// enables the debugging of created tuples in test development.
func SetupTestOFGAClient(names ...string) (*ofga.OFGAClient, *cofga.Client, *cofga.OpenFGAParams, error) {
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

	if len(authTypeDefinitions) == 0 {
		authTypeDefinitions, _, err = getAuthModelDefinition()
		if err != nil {
			return nil, nil, nil, errgo.Notef(err, "failed to read authorization model definition")
		}
	}

	authModelID, err := cofgaClient.CreateAuthModel(ctx, authTypeDefinitions)
	if err != nil {
		return nil, nil, nil, errgo.Notef(err, "failed to create authorization model")
	}

	cofgaClient.AuthModelId = authModelID
	cofgaParams.AuthModelID = authModelID

	// TODO (babakks): check if we can omit setting schema version.
	// ar.SetSchemaVersion(schemaVersion)
	// amr, _, err := api.WriteAuthorizationModel(ctx).Body(*ar).Execute()
	// if err != nil {
	// 	return nil, nil, nil, err
	// }

	client := ofga.NewOpenFGAClient(cofgaClient)

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
	conn.Exec(ctx, "TRUNCATE TABLE tuple;")
	conn.Exec(ctx, "TRUNCATE TABLE changelog;")
	return nil
}
