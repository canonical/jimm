// Copyright 2025 Canonical.

package testing

import (
	"fmt"
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	if _, ok := os.LookupEnv("RUN_E2E_TESTS"); !ok {
		fmt.Fprint(os.Stdout, "Skipping e2e tests. Set RUN_E2E_TESTS=true to run them.")
		os.Exit(0)
	}
	// Run all tests in the package
	code := m.Run()

	os.Exit(code)
}
