// Copyright 2024 Canonical.

package jujuapi_test

import (
	"testing"

	jujutesting "github.com/juju/juju/testing"
)

// Registers Go Check tests into the Go test runner.
func TestPackage(t *testing.T) {
	jujutesting.MgoTestPackage(t)
}
