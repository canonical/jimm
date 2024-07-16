// Copyright 2024 Canonical Ltd.

package rebac

import (
	"net/http"

	"github.com/canonical/rebac-admin-ui-handlers/v1/interfaces"
)

type Authenticator struct{}

var _ interfaces.Authenticator = &Authenticator{}

// Authenticate extracts the calling user's information from the given HTTP request
func (a *Authenticator) Authenticate(r *http.Request) (any, error) {
	// TODO(CSS-9386): replace with real authentication
	return "joe", nil
}
