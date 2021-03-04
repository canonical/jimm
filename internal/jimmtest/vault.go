// Copyright 2021 Canonical Ltd.

package jimmtest

import (
	"os"
	"os/exec"
	"sync"
	"testing"

	vault "github.com/hashicorp/vault/api"
)

const vaultRootToken = "jimmtest-vault-root-token"

var vaultOnce sync.Once
var vaultCmd *exec.Cmd
var vaultErr error

// VaultClient returns a new vault client for use in a test. If there is an
// error starting a dev vault server a log will be written to the given TB
// and ok will be false. The started vault server will listen on the
// address specified in VAULT_DEV_LISTEN_ADDRESS if there is one. The path
// will be unique for each test.
func VaultClient(tb testing.TB) (client *vault.Client, path string, ok bool) {
	vaultOnce.Do(func() {
		vaultCmd = exec.Command("vault", "server", "-dev", "-dev-no-store-token", "-dev-root-token-id="+vaultRootToken)
		vaultErr = vaultCmd.Start()
	})
	if vaultErr != nil {
		tb.Logf("error starting vault: %s", vaultErr)
		return nil, "", false
	}

	cfg := vault.DefaultConfig()
	if addr := os.Getenv("VAULT_DEV_LISTEN_ADDRESS"); addr != "" {
		cfg.Address = "http://" + addr
	} else {
		cfg.Address = "http://127.0.0.1:8200"
	}
	client, err := vault.NewClient(cfg)
	if err != nil {
		tb.Logf("error creating vault client: %s", err)
		return nil, "", false
	}
	client.SetToken(vaultRootToken)
	if err := client.Sys().Mount("/"+tb.Name(), &vault.MountInput{Type: "kv"}); err != nil {
		tb.Logf("error creating vault client: %s", err)
		return nil, "", false
	}
	return client, tb.Name(), true
}

// VaultStop stops any running vault server. VaultStop must only be called
// once any tests that might be using the vault server have finished.
func VaultStop() {
	if vaultCmd == nil {
		return
	}
	if err := vaultCmd.Process.Signal(os.Interrupt); err != nil {
		return
	}
	if err := vaultCmd.Wait(); err != nil {
		return
	}
	vaultCmd = nil
	return
}
