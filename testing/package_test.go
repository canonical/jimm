// Copyright 2026 Canonical.

package testing

import (
	"os"
	"testing"

	gc "gopkg.in/check.v1"
)

// Registers Go Check tests into the Go test runner.
func TestPackage(t *testing.T) {
	onlyRunE2ETests(t)
	gc.TestingT(t)
}

func onlyRunE2ETests(t *testing.T) {
	if _, ok := os.LookupEnv("RUN_E2E_TESTS"); !ok {
		t.Skip("Skipping e2e tests. Set RUN_E2E_TESTS=true to run them.")
	}
}
