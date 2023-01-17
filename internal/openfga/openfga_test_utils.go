package openfga

import (
	"context"

	"github.com/jackc/pgx/v4"
	openfga "github.com/openfga/go-sdk"
	"github.com/openfga/go-sdk/credentials"
)

var OpenFGATestAuthModel = "01GP1EC038KHGB6JJ2XXXXCXKB"

var OpenFGATestConfig = openfga.Configuration{
	ApiScheme: "http",
	ApiHost:   "localhost:8080",
	StoreId:   "01GP1254CHWJC1MNGVB0WDG1T0",
	Credentials: &credentials.Credentials{
		Method: credentials.CredentialsMethodApiToken,
		Config: &credentials.Config{
			ApiToken: "jimm",
		},
	},
}

func SetupTestOFGAClient() (openfga.OpenFgaApi, *OFGAClient) {
	cfg, _ := openfga.NewConfiguration(OpenFGATestConfig)
	api := openfga.NewAPIClient(cfg).OpenFgaApi
	client := NewOpenFGAClient(api, OpenFGATestAuthModel)
	return api, client
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
