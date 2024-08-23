// Copyright 2024 Canonical.

package jimmtest

import (
	"github.com/hashicorp/vault/api"
)

const (
	testRoleID   = "test-role-id"
	testSecretID = "test-secret-id"
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
	return vaultClient, "jimm-kv", testRoleID, testSecretID, true
}
