// Copyright 2025 Canonical.

package rpcproxy

import "github.com/canonical/jimm/v3/internal/openfga"

type (
	Message          = message
	KeyManagerFacade = keyManagerFacade
)

func NewKeyManagerFacade(keyManager SSHKeyManager, user *openfga.User) keyManagerFacade {
	return keyManagerFacade{keyManager: keyManager, user: user}
}
