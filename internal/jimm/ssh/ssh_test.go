// Copyright 2025 Canonical.

package ssh_test

import (
	"testing"

	qt "github.com/frankban/quicktest"

	"github.com/canonical/jimm/v3/internal/jimm/ssh"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest/mocks"
)

func TestSSHManagerCreation(t *testing.T) {
	c := qt.New(t)
	// TODO(simonedutto): add proper testing when implementing the sshkeymanager VerifyPublicKey method.
	_, err := ssh.NewSSHManager(&mocks.IdentityManager{}, &mocks.ModelManager{}, &mocks.SSHKeyManager{})
	c.Assert(err, qt.IsNil)
}
