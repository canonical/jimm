// Copyright 2024 canonical.

package rebac_admin

import (
	"context"

	openfgastatic "github.com/canonical/jimm/openfga"
	"github.com/canonical/rebac-admin-ui-handlers/v1/resources"
)

// For rebac v1 this list is kept manually.
// The reason behind that is we want to decide what relations to expose to rebac admin ui.
var EntitlementsList = []resources.EntityEntitlement{
	// applicationoffer
	{EntitlementType: "administrator", EntityName: "user", EntityType: "applicationoffer"},
	{EntitlementType: "administrator", EntityName: "user:*", EntityType: "applicationoffer"},
	{EntitlementType: "administrator", EntityName: "group#member", EntityType: "applicationoffer"},
	{EntitlementType: "consumer", EntityName: "user", EntityType: "applicationoffer"},
	{EntitlementType: "consumer", EntityName: "user:*", EntityType: "applicationoffer"},
	{EntitlementType: "consumer", EntityName: "group#member", EntityType: "applicationoffer"},
	{EntitlementType: "reader", EntityName: "user", EntityType: "applicationoffer"},
	{EntitlementType: "reader", EntityName: "user:*", EntityType: "applicationoffer"},
	{EntitlementType: "reader", EntityName: "group#member", EntityType: "applicationoffer"},

	// cloud
	{EntitlementType: "administrator", EntityName: "user", EntityType: "cloud"},
	{EntitlementType: "administrator", EntityName: "user:*", EntityType: "cloud"},
	{EntitlementType: "administrator", EntityName: "group#member", EntityType: "cloud"},
	{EntitlementType: "can_addmodel", EntityName: "user", EntityType: "cloud"},
	{EntitlementType: "can_addmodel", EntityName: "user:*", EntityType: "cloud"},
	{EntitlementType: "can_addmodel", EntityName: "group#member", EntityType: "cloud"},

	// controller
	{EntitlementType: "administrator", EntityName: "user", EntityType: "controller"},
	{EntitlementType: "administrator", EntityName: "user:*", EntityType: "controller"},
	{EntitlementType: "administrator", EntityName: "group#member", EntityType: "controller"},
	{EntitlementType: "audit_log_viewer", EntityName: "user", EntityType: "controller"},
	{EntitlementType: "audit_log_viewer", EntityName: "user:*", EntityType: "controller"},
	{EntitlementType: "audit_log_viewer", EntityName: "group#member", EntityType: "controller"},

	// group
	{EntitlementType: "member", EntityName: "user", EntityType: "group"},
	{EntitlementType: "member", EntityName: "user:*", EntityType: "group"},
	{EntitlementType: "member", EntityName: "group#member", EntityType: "group"},

	// model
	{EntitlementType: "administrator", EntityName: "user", EntityType: "model"},
	{EntitlementType: "administrator", EntityName: "user:*", EntityType: "model"},
	{EntitlementType: "administrator", EntityName: "group#member", EntityType: "model"},
	{EntitlementType: "reader", EntityName: "user", EntityType: "model"},
	{EntitlementType: "reader", EntityName: "user:*", EntityType: "model"},
	{EntitlementType: "reader", EntityName: "group#member", EntityType: "model"},
	{EntitlementType: "writer", EntityName: "user", EntityType: "model"},
	{EntitlementType: "writer", EntityName: "user:*", EntityType: "model"},
	{EntitlementType: "writer", EntityName: "group#member", EntityType: "model"},

	// serviceaccount
	{EntitlementType: "administrator", EntityName: "user", EntityType: "serviceaccount"},
	{EntitlementType: "administrator", EntityName: "user:*", EntityType: "serviceaccount"},
	{EntitlementType: "administrator", EntityName: "group#member", EntityType: "serviceaccount"},
}

// entitlementsService implements the `entitlementsService` interface from rebac-admin-ui-handlers library
type entitlementsService struct{}

func newEntitlementService() *entitlementsService {
	return &entitlementsService{}
}

// ListEntitlements returns the list of entitlements in JSON format.
func (s *entitlementsService) ListEntitlements(ctx context.Context, params *resources.GetEntitlementsParams) ([]resources.EntityEntitlement, error) {
	return EntitlementsList, nil
}

// RawEntitlements returns the list of entitlements as raw text.
func (s *entitlementsService) RawEntitlements(ctx context.Context) (string, error) {
	return string(openfgastatic.AuthModelDSL), nil
}
