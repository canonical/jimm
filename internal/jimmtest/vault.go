// Copyright 2021 Canonical Ltd.

package jimmtest

import (
	"encoding/json"

	vault_test "github.com/canonical/jimm/local/vault"
	"github.com/hashicorp/vault/api"
)

type fatalF interface {
	Name() string
	Fatalf(format string, args ...interface{})
}

// VaultClient returns a new vault client for use in a test.
func VaultClient(tb fatalF) (*api.Client, string, string, string, bool) {
	cfg := api.DefaultConfig()
	cfg.Address = "http://localhost:8200"
	vaultClient, _ := api.NewClient(cfg)

	appRole := vault_test.AppRole
	var vaultAPISecret api.Secret
	err := json.Unmarshal(appRole, &vaultAPISecret)
	if err != nil {
		panic("cannot unmarshal vault secret")
	}

	roleID, ok := vaultAPISecret.Data["role_id"]
	if !ok {
		panic("role ID not found")
	}
	roleSecretID, ok := vaultAPISecret.Data["secret_id"]
	if !ok {
		panic("role secret ID not found")
	}
	roleIDString, ok := roleID.(string)
	if !ok {
		panic("failed to convert role ID to string")
	}
	roleSecretIDString, ok := roleSecretID.(string)
	if !ok {
		panic("failed to convert role secret ID to string")
	}
	return vaultClient, "jimm-kv", roleIDString, roleSecretIDString, true
}
