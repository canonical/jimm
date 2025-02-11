// Copyright 2025 Canonical.

package ssh

import (
	"context"

	"github.com/canonical/jimm/v3/internal/openfga"
)

// SSHManager is a type alias to export sshManager for use in tests.
type SSHManager = sshManager

func (s *sshManager) ControllerInfoFromModelUUID(ctx context.Context, modelUUID string, user *openfga.User) (ControllerInfo, error) {
	return s.controllerInfoFromModelUUID(ctx, modelUUID, user)
}
