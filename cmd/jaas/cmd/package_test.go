// Copyright 2025 Canonical.

package cmd_test

import (
	"testing"

	jujutesting "github.com/juju/juju/testing"
)

//go:generate go tool mockgen -package mocks -typed -destination ./mocks/client_mock.go github.com/canonical/jimm/v3/cmd/jaas/cmd JIMMClient
//go:generate go tool mockgen -package mocks -typed -destination ./mocks/io_writer_mock.go io Writer

func TestPackage(t *testing.T) {
	jujutesting.MgoTestPackage(t)
}
