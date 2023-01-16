package openfga

import (
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
