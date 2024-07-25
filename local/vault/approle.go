// Copyright 2024 Canonical Ltd.

// This package exists to hold files used to authenticate with Vault during tests.
package vault

import (
	_ "embed"
)

//go:embed approle.json
var AppRole []byte
