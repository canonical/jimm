// Copyright 2024 Canonical Ltd.

package utils

import (
	"fmt"
	"time"

	"github.com/canonical/jimm/internal/openfga"
	"github.com/canonical/rebac-admin-ui-handlers/v1/resources"
)

// FromUserToIdentity parses openfga.User into resources.Identity .
func FromUserToIdentity(user openfga.User) resources.Identity {
	id := fmt.Sprintf("%d", user.ID)
	joined := user.CreatedAt.Format(time.RFC3339)
	lastLogin := user.LastLogin.Time.Format(time.RFC3339)
	return resources.Identity{
		Email:     user.Name,
		Id:        &id,
		Joined:    &joined,
		LastLogin: &lastLogin,
		Source:    "",
	}
}
