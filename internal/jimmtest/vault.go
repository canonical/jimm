// Copyright 2021 Canonical Ltd.

package jimmtest

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/hashicorp/vault/api"

	"github.com/CanonicalLtd/jimm/internal/errors"
)

// VaultAuthPath contains the path that is configured automatically to
// allow authentication.
const VaultAuthPath = "/auth/approle/login"

const vaultRootToken = "jimmtest-vault-root-token"

var vaultCmd *exec.Cmd
var vaultRootClient *api.Client

// StartVault starts and initialises a vault service.
func StartVault() error {
	if vaultCmd != nil {
		return errors.E("vault already started")
	}
	vaultCmd = exec.Command("vault", "server", "-dev", "-dev-no-store-token", "-dev-root-token-id="+vaultRootToken)
	if err := vaultCmd.Start(); err != nil {
		return err
	}
	cfg := api.DefaultConfig()
	if addr := os.Getenv("VAULT_DEV_LISTEN_ADDRESS"); addr != "" {
		cfg.Address = "http://" + addr
	} else {
		cfg.Address = "http://127.0.0.1:8200"
	}
	var err error
	vaultRootClient, err = api.NewClient(cfg)
	if err != nil {
		return err
	}
	vaultRootClient.SetToken(vaultRootToken)
	err = vaultRootClient.Sys().EnableAuthWithOptions("approle", &api.EnableAuthOptions{
		Type: "approle",
	})
	if err != nil {
		vaultRootClient = nil
		return err
	}
	err = vaultRootClient.Sys().Mount("/kv", &api.MountInput{
		Type: "kv",
		Config: api.MountConfigInput{
			Options: map[string]string{
				"version": "1",
			},
		},
	})
	if err != nil {
		vaultRootClient = nil
		return err
	}
	return nil
}

const policyTemplate = `
path "kv/%s/*" {
    capabilities = ["create", "read", "update", "delete"]
}
`

type fatalF interface {
	Name() string
	Fatalf(format string, args ...interface{})
}

// VaultClient returns a new vault client for use in a test.
func VaultClient(tb fatalF) (client *api.Client, path string, creds map[string]interface{}, ok bool) {
	if vaultRootClient == nil {
		return nil, "", nil, false
	}
	name := strings.ReplaceAll(tb.Name(), "/", "_")
	err := vaultRootClient.Sys().PutPolicy(name, fmt.Sprintf(policyTemplate, name))
	if err != nil {
		tb.Fatalf("error initialising policy: %s", err)
	}
	_, err = vaultRootClient.Logical().Write("/auth/approle/role/"+name, map[string]interface{}{
		"token_ttl":      "30s",
		"token_max_ttl":  "60s",
		"token_policies": name,
	})
	if err != nil {
		tb.Fatalf("error initialising approle: %s", err)
	}
	s, err := vaultRootClient.Logical().Read("/auth/approle/role/" + name + "/role-id")
	if err != nil {
		tb.Fatalf("error initialising approle: %s", err)
	}
	creds = make(map[string]interface{})
	creds["role_id"] = s.Data["role_id"]
	s, err = vaultRootClient.Logical().Write("/auth/approle/role/"+name+"/secret-id", nil)
	if err != nil {
		tb.Fatalf("error initialising approle: %s", err)
	}
	creds["secret_id"] = s.Data["secret_id"]
	client, err = vaultRootClient.Clone()
	if err != nil {
		tb.Fatalf("error creating client: %s", err)
	}
	client.ClearToken()
	return client, "/kv/" + name, creds, true
}

// StopVault stops any running vault server. VaultStop must only be called
// once any tests that might be using the vault server have finished.
func StopVault() {
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
}
