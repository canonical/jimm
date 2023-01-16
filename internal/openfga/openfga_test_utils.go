package openfga

import (
	openfga "github.com/openfga/go-sdk"
	"github.com/openfga/go-sdk/credentials"
)

func SetupTestOFGAClient() (openfga.OpenFgaApi, *OFGAClient) {
	cfg, _ := openfga.NewConfiguration(openfga.Configuration{
		ApiScheme: "http",
		ApiHost:   "localhost:8080",
		StoreId:   "01GP1254CHWJC1MNGVB0WDG1T0",
		Credentials: &credentials.Credentials{
			Method: credentials.CredentialsMethodApiToken,
			Config: &credentials.Config{
				ApiToken: "jimm",
			},
		},
	})
	api := openfga.NewAPIClient(cfg).OpenFgaApi
	client := NewOpenFGAClient(api, "01GP1EC038KHGB6JJ2XXXXCXKB")
	return api, client
}
