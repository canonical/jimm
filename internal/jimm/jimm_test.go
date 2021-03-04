// Copyright 2021 Canonical Ltd.

package jimm_test

import (
	"os"
	"testing"

	"github.com/CanonicalLtd/jimm/internal/jimmtest"
)

func TestMain(m *testing.M) {
	code := m.Run()
	jimmtest.VaultStop()
	os.Exit(code)
}
