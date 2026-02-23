// Copyright 2025 Canonical.

package testing

import (
	"fmt"
	"os"
	"testing"

	qt "github.com/frankban/quicktest"

	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
)

func SetupIntegrationEnv(c *qt.C) (s jimmtest.WebsocketE2ESuite) {
	return jimmtest.SetupWebsocketEnv(c)
}
func SetupIntegrationEnvWithRealAuth(c *qt.C) (s jimmtest.WebsocketE2ESuite) {
	return jimmtest.SetupWebsocketEnv(c, jimmtest.WithRealAuthN())
}

func TestMain(m *testing.M) {
	if _, ok := os.LookupEnv("RUN_E2E_TESTS"); !ok {
		fmt.Fprint(os.Stdout, "Skipping e2e tests. Set RUN_E2E_TESTS=true to run them.")
		os.Exit(0)
	}
	// Run all tests in the package
	code := m.Run()

	os.Exit(code)
}
