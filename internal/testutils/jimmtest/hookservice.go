// Copyright 2026 Canonical.

package jimmtest

import (
	"context"
	"os"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"
)

// DefaultHookServiceAddress matches the hook-service compose port mapping
// (see docker-compose.yaml).
const DefaultHookServiceAddress = "localhost:9091"

// HookServiceAddress returns the address of the hook-service to test
// against. It can be overridden with the JIMM_TEST_HOOK_SERVICE_ADDRESS
// environment variable.
func HookServiceAddress() string {
	if envAddr, exists := os.LookupEnv("JIMM_TEST_HOOK_SERVICE_ADDRESS"); exists {
		return envAddr
	}
	return DefaultHookServiceAddress
}

// HookServiceTokenSource returns a token source issuing client credentials
// access tokens for the jimm-device client, which the compose hook-service
// accepts. Tokens are requested via keycloak.localhost so their issuer
// matches the hook-service's AUTHENTICATION_ISSUER.
func HookServiceTokenSource(ctx context.Context) oauth2.TokenSource {
	// #nosec G101 Fake test credentials.
	cfg := clientcredentials.Config{
		ClientID:     "jimm-device",
		ClientSecret: "SwjDofnbDzJDm9iyfUhEp67FfUFMY8L4",
		TokenURL:     "http://keycloak.localhost:8082/realms/jimm/protocol/openid-connect/token",
		AuthStyle:    oauth2.AuthStyleInParams,
	}
	return cfg.TokenSource(ctx)
}
