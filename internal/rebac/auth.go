// Copyright 2024 Canonical Ltd.

package rebac

import (
	"net/http"

	"github.com/canonical/rebac-admin-ui-handlers/v1/interfaces"
)

type Authenticator struct{}

var _ interfaces.Authenticator = &Authenticator{}

// Authenticate for now lets everything through to fulfill the requirement of admin rebac backend to have an authenticator
func (a *Authenticator) Authenticate(r *http.Request) (any, error) {
	return "joe", nil
}
