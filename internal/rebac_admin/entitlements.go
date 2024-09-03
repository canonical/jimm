// Copyright 2024 Canonical.

package rebac_admin

import (
	"context"

	"github.com/canonical/rebac-admin-ui-handlers/v1/resources"

	openfgastatic "github.com/canonical/jimm/v3/openfga"
)

// For rebac v1 this list is kept manually.
// The reason behind that is we want to decide what relations to expose to rebac admin ui.
var EntitlementsList = []resources.EntitlementSchema{
	// applicationoffer
	{Entitlement: "administrator", ReceiverType: "user", EntityType: "applicationoffer"},
	{Entitlement: "administrator", ReceiverType: "user:*", EntityType: "applicationoffer"},
	{Entitlement: "administrator", ReceiverType: "group#member", EntityType: "applicationoffer"},
	{Entitlement: "consumer", ReceiverType: "user", EntityType: "applicationoffer"},
	{Entitlement: "consumer", ReceiverType: "user:*", EntityType: "applicationoffer"},
	{Entitlement: "consumer", ReceiverType: "group#member", EntityType: "applicationoffer"},
	{Entitlement: "reader", ReceiverType: "user", EntityType: "applicationoffer"},
	{Entitlement: "reader", ReceiverType: "user:*", EntityType: "applicationoffer"},
	{Entitlement: "reader", ReceiverType: "group#member", EntityType: "applicationoffer"},

	// cloud
	{Entitlement: "administrator", ReceiverType: "user", EntityType: "cloud"},
	{Entitlement: "administrator", ReceiverType: "user:*", EntityType: "cloud"},
	{Entitlement: "administrator", ReceiverType: "group#member", EntityType: "cloud"},
	{Entitlement: "can_addmodel", ReceiverType: "user", EntityType: "cloud"},
	{Entitlement: "can_addmodel", ReceiverType: "user:*", EntityType: "cloud"},
	{Entitlement: "can_addmodel", ReceiverType: "group#member", EntityType: "cloud"},

	// controller
	{Entitlement: "administrator", ReceiverType: "user", EntityType: "controller"},
	{Entitlement: "administrator", ReceiverType: "user:*", EntityType: "controller"},
	{Entitlement: "administrator", ReceiverType: "group#member", EntityType: "controller"},
	{Entitlement: "audit_log_viewer", ReceiverType: "user", EntityType: "controller"},
	{Entitlement: "audit_log_viewer", ReceiverType: "user:*", EntityType: "controller"},
	{Entitlement: "audit_log_viewer", ReceiverType: "group#member", EntityType: "controller"},

	// group
	{Entitlement: "member", ReceiverType: "user", EntityType: "group"},
	{Entitlement: "member", ReceiverType: "user:*", EntityType: "group"},
	{Entitlement: "member", ReceiverType: "group#member", EntityType: "group"},

	// model
	{Entitlement: "administrator", ReceiverType: "user", EntityType: "model"},
	{Entitlement: "administrator", ReceiverType: "user:*", EntityType: "model"},
	{Entitlement: "administrator", ReceiverType: "group#member", EntityType: "model"},
	{Entitlement: "reader", ReceiverType: "user", EntityType: "model"},
	{Entitlement: "reader", ReceiverType: "user:*", EntityType: "model"},
	{Entitlement: "reader", ReceiverType: "group#member", EntityType: "model"},
	{Entitlement: "writer", ReceiverType: "user", EntityType: "model"},
	{Entitlement: "writer", ReceiverType: "user:*", EntityType: "model"},
	{Entitlement: "writer", ReceiverType: "group#member", EntityType: "model"},

	// serviceaccount
	{Entitlement: "administrator", ReceiverType: "user", EntityType: "serviceaccount"},
	{Entitlement: "administrator", ReceiverType: "user:*", EntityType: "serviceaccount"},
	{Entitlement: "administrator", ReceiverType: "group#member", EntityType: "serviceaccount"},
}

// entitlementsService implements the `entitlementsService` interface from rebac-admin-ui-handlers library
type entitlementsService struct{}

func newEntitlementService() *entitlementsService {
	return &entitlementsService{}
}

// ListEntitlements returns the list of entitlements in JSON format.
func (s *entitlementsService) ListEntitlements(ctx context.Context, params *resources.GetEntitlementsParams) ([]resources.EntitlementSchema, error) {
	return EntitlementsList, nil
}

// RawEntitlements returns the list of entitlements as raw text.
func (s *entitlementsService) RawEntitlements(ctx context.Context) (string, error) {
	return string(openfgastatic.AuthModelDSL), nil
}
