// Copyright 2026 Canonical.

package jimmtest

import "os"

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
