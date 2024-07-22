// Copyright 2023 canonical.

package rebac_admin

import (
	"context"

	openfgastatic "github.com/canonical/jimm/openfga"
	"github.com/canonical/rebac-admin-ui-handlers/v1/resources"
)

type EntitlementsService struct{}

// ListEntitlements returns the list of entitlements in JSON format.
func (s *EntitlementsService) ListEntitlements(ctx context.Context, params *resources.GetEntitlementsParams) ([]resources.EntityEntitlement, error) {
	return EntitlementsList, nil
}

// RawEntitlements returns the list of entitlements as raw text.
func (s *EntitlementsService) RawEntitlements(ctx context.Context) (string, error) {
	return string(openfgastatic.AuthModelFileDSL), nil
}
