// Copyright 2024 canonical.

package rebac_admin

import (
	"context"

	openfgastatic "github.com/canonical/jimm/openfga"
	"github.com/canonical/rebac-admin-ui-handlers/v1/resources"
)

// Since these values has semantic meanings in the API, they'll probably be
// refactored into constants provided by `rebac-admin-ui-handlers` library. So,
// we define them here as constants, rather than repeating them as literals.
const identity = "identity"
const group = "group"

// For rebac v1 this list is kept manually.
// The reason behind that is we want to decide what relations to expose to rebac admin ui.
var EntitlementSchemaList = []resources.EntitlementSchema{
	{EntityType: "applicationoffer", Entitlement: "administrator", ReceiverType: identity},
	{EntityType: "applicationoffer", Entitlement: "administrator", ReceiverType: group},
	{EntityType: "applicationoffer", Entitlement: "consumer", ReceiverType: identity},
	{EntityType: "applicationoffer", Entitlement: "consumer", ReceiverType: group},
	{EntityType: "applicationoffer", Entitlement: "reader", ReceiverType: identity},
	{EntityType: "applicationoffer", Entitlement: "reader", ReceiverType: group},

	{EntityType: "cloud", Entitlement: "administrator", ReceiverType: identity},
	{EntityType: "cloud", Entitlement: "administrator", ReceiverType: group},
	{EntityType: "cloud", Entitlement: "can_addmodel", ReceiverType: identity},
	{EntityType: "cloud", Entitlement: "can_addmodel", ReceiverType: group},

	{EntityType: "controller", Entitlement: "administrator", ReceiverType: identity},
	{EntityType: "controller", Entitlement: "administrator", ReceiverType: group},
	{EntityType: "controller", Entitlement: "audit_log_viewer", ReceiverType: identity},
	{EntityType: "controller", Entitlement: "audit_log_viewer", ReceiverType: group},

	{EntityType: "group", Entitlement: "member", ReceiverType: identity},
	{EntityType: "group", Entitlement: "member", ReceiverType: group},

	{EntityType: "model", Entitlement: "administrator", ReceiverType: identity},
	{EntityType: "model", Entitlement: "administrator", ReceiverType: group},
	{EntityType: "model", Entitlement: "reader", ReceiverType: identity},
	{EntityType: "model", Entitlement: "reader", ReceiverType: group},
	{EntityType: "model", Entitlement: "writer", ReceiverType: identity},
	{EntityType: "model", Entitlement: "writer", ReceiverType: group},

	{EntityType: "serviceaccount", Entitlement: "administrator", ReceiverType: identity},
	{EntityType: "serviceaccount", Entitlement: "administrator", ReceiverType: group},
}

// entitlementsService implements the `entitlementsService` interface from rebac-admin-ui-handlers library
type entitlementsService struct{}

func newEntitlementService() *entitlementsService {
	return &entitlementsService{}
}

// ListEntitlements returns the list of entitlements in JSON format.
func (s *entitlementsService) ListEntitlements(ctx context.Context, params *resources.GetEntitlementsParams) ([]resources.EntitlementSchema, error) {
	return EntitlementSchemaList, nil
}

// RawEntitlements returns the list of entitlements as raw text.
func (s *entitlementsService) RawEntitlements(ctx context.Context) (string, error) {
	return string(openfgastatic.AuthModelDSL), nil
}
