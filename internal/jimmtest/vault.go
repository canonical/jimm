// Copyright 2021 Canonical Ltd.

package jimmtest

import (
	"encoding/json"
	"fmt"
	"os"
	"path"

	"github.com/hashicorp/vault/api"
)

type fatalF interface {
	Name() string
	Fatalf(format string, args ...interface{})
}

// VaultClient returns a new vault client for use in a test.
func VaultClient(tb fatalF, prefix string) (*api.Client, string, map[string]interface{}, bool) {
	cfg := api.DefaultConfig()
	cfg.Address = "http://localhost:8200"
	vaultClient, _ := api.NewClient(cfg)

	b, err := os.ReadFile(path.Join(prefix, "./local/vault/approle.json"))
	if err != nil {
		fmt.Println("we got file?")
	}

	creds := make(map[string]interface{})
	var vaultAPISecret api.Secret
	err = json.Unmarshal(b, &vaultAPISecret)
	if err != nil {
		fmt.Println("error?")
	}
	creds["role_id"] = vaultAPISecret.Data["role_id"]
	creds["secret_id"] = vaultAPISecret.Data["secret_id"]

	return vaultClient, "/jimm-kv/", creds, true
}
