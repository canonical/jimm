package openfga

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/jackc/pgx/v4"
	"github.com/oklog/ulid/v2"
	openfga "github.com/openfga/go-sdk"
	"github.com/openfga/go-sdk/credentials"
	gc "gopkg.in/check.v1"
)

// SetupTestOFGAClient is intended to be used per test, in that it
// creates a store based on the current tests name and is expected to
// be a single use instance (due to the initial removing of a store).
//
// The benefit of not cleaning up the store immediately afterwards,
// enables the debugging of created tuples in test development.
func SetupTestOFGAClient(c *gc.C) (openfga.OpenFgaApi, *OFGAClient) {
	ctx := context.Background()

	openFGATestConfig := openfga.Configuration{
		ApiScheme: "http",
		ApiHost:   "localhost:8080",
		Credentials: &credentials.Credentials{
			Method: credentials.CredentialsMethodApiToken,
			Config: &credentials.Config{
				ApiToken: "jimm",
			},
		},
	}
	err := RemoveStore(ctx, c.TestName())
	c.Assert(err, gc.IsNil)

	uuid := ulid.Make().String()
	err = CreateStore(ctx, c.TestName(), uuid)
	c.Assert(err, gc.IsNil)

	cfg, err := openfga.NewConfiguration(openFGATestConfig)
	c.Assert(err, gc.IsNil)

	client := openfga.NewAPIClient(cfg)

	client.SetStoreId(uuid)
	api := client.OpenFgaApi

	b, err := os.ReadFile("../../local/openfga/authorisation_model.json")
	c.Assert(err, gc.IsNil)
	wrapper := make(map[string]interface{})
	err = json.Unmarshal(b, &wrapper)
	c.Assert(err, gc.IsNil)

	b, err = json.Marshal(wrapper["type_definitions"])
	c.Assert(err, gc.IsNil)

	definitions := []openfga.TypeDefinition{}
	err = json.Unmarshal(b, &definitions)
	c.Assert(err, gc.IsNil)

	ar := openfga.NewWriteAuthorizationModelRequest()
	ar.SetTypeDefinitions(definitions)

	amr, _, err := api.WriteAuthorizationModel(ctx).Body(*ar).Execute()
	c.Assert(err, gc.IsNil)

	wrapperClient := NewOpenFGAClient(client.OpenFgaApi, amr.GetAuthorizationModelId())
	return client.OpenFgaApi, wrapperClient
}

// RemoveStore removes an OpenFGA store (via db) by NAME.
// Currently, OpenFGA does not support this as it is expected to remove a store by ID.
//
// However, in a testing scenario, we want a simple and quick solution to cleanup
// a store per test,
func RemoveStore(ctx context.Context, name string) error {
	conn, err := pgx.Connect(context.Background(), "postgresql://jimm:jimm@localhost/jimm")
	if err != nil {
		return err
	}
	defer conn.Close(ctx)
	_, err = conn.Exec(ctx, fmt.Sprintf("DELETE FROM store WHERE name = '%s';", name))
	if err != nil {
		return err
	}
	return nil
}

// CreateStore adds a store to OpenFGA (via db), circumventing the superficial rules
// set in their server around character limit (64).
func CreateStore(ctx context.Context, name string, id string) error {
	conn, err := pgx.Connect(context.Background(), "postgresql://jimm:jimm@localhost/jimm")
	if err != nil {
		return err
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
		return err
	}
	return nil
}

func TruncateOpenFgaTuples(ctx context.Context) error {
	conn, err := pgx.Connect(context.Background(), "postgresql://jimm:jimm@localhost/jimm")
	if err != nil {
		return err
	}
	defer conn.Close(ctx)
	conn.Exec(ctx, "TRUNCATE TABLE tuple;")
	conn.Exec(ctx, "TRUNCATE TABLE changelog;")
	return nil
}

// DeleteAllTuples removes all existing tuples.
func (o *OFGAClient) DeleteAllTuples(ctx context.Context) error {
	var allTuples []openfga.TupleKey
	rr := openfga.NewReadRequest()
	rr.SetPageSize(25)
	rr.SetAuthorizationModelId(o.AuthModelId)
	readResponse, _, err := o.api.Read(ctx).Body(*rr).Execute()
	if err != nil {
		return err
	}
	var continuationToken string
	for ok := true; ok; ok = (*readResponse.ContinuationToken != continuationToken) {
		for _, tuple := range *readResponse.Tuples {
			allTuples = append(allTuples, *tuple.Key)
		}
		if len(allTuples) > 0 {
			err = o.deleteRelation(ctx, allTuples...)
			if err != nil {
				return err
			}
		}
		allTuples = []openfga.TupleKey{}
		continuationToken = *readResponse.ContinuationToken
		rr.SetContinuationToken(continuationToken)
		readResponse, _, err = o.api.Read(ctx).Body(*rr).Execute()
		if err != nil {
			return err
		}

	}
	return nil
}
