// Copyright 2024 Canonical.

package rebac_admin

import (
	"context"

	"github.com/canonical/rebac-admin-ui-handlers/v1/resources"

	openfgastatic "github.com/canonical/jimm/v3/openfga"
)

const ApplicationOffer = "applicationoffer"
const Cloud = "cloud"
const Controller = "controller"
const Group = "group"
const Model = "model"
const ServiceAccount = "serviceaccount"

// For rebac v1 this list is kept manually.
// The reason behind that is we want to decide what relations to expose to rebac admin ui.
var EntitlementsList = []resources.EntitlementSchema{
	// applicationoffer
	{Entitlement: "administrator", ReceiverType: "user", EntityType: ApplicationOffer},
	{Entitlement: "administrator", ReceiverType: "user:*", EntityType: ApplicationOffer},
	{Entitlement: "administrator", ReceiverType: "group#member", EntityType: ApplicationOffer},
	{Entitlement: "consumer", ReceiverType: "user", EntityType: ApplicationOffer},
	{Entitlement: "consumer", ReceiverType: "user:*", EntityType: ApplicationOffer},
	{Entitlement: "consumer", ReceiverType: "group#member", EntityType: ApplicationOffer},
	{Entitlement: "reader", ReceiverType: "user", EntityType: ApplicationOffer},
	{Entitlement: "reader", ReceiverType: "user:*", EntityType: ApplicationOffer},
	{Entitlement: "reader", ReceiverType: "group#member", EntityType: ApplicationOffer},

	// cloud
	{Entitlement: "administrator", ReceiverType: "user", EntityType: Cloud},
	{Entitlement: "administrator", ReceiverType: "user:*", EntityType: Cloud},
	{Entitlement: "administrator", ReceiverType: "group#member", EntityType: Cloud},
	{Entitlement: "can_addmodel", ReceiverType: "user", EntityType: Cloud},
	{Entitlement: "can_addmodel", ReceiverType: "user:*", EntityType: Cloud},
	{Entitlement: "can_addmodel", ReceiverType: "group#member", EntityType: Cloud},

	// controller
	{Entitlement: "administrator", ReceiverType: "user", EntityType: Controller},
	{Entitlement: "administrator", ReceiverType: "user:*", EntityType: Controller},
	{Entitlement: "administrator", ReceiverType: "group#member", EntityType: Controller},
	{Entitlement: "audit_log_viewer", ReceiverType: "user", EntityType: Controller},
	{Entitlement: "audit_log_viewer", ReceiverType: "user:*", EntityType: Controller},
	{Entitlement: "audit_log_viewer", ReceiverType: "group#member", EntityType: Controller},

	// group
	{Entitlement: "member", ReceiverType: "user", EntityType: Group},
	{Entitlement: "member", ReceiverType: "user:*", EntityType: Group},
	{Entitlement: "member", ReceiverType: "group#member", EntityType: Group},

	// model
	{Entitlement: "administrator", ReceiverType: "user", EntityType: Model},
	{Entitlement: "administrator", ReceiverType: "user:*", EntityType: Model},
	{Entitlement: "administrator", ReceiverType: "group#member", EntityType: Model},
	{Entitlement: "reader", ReceiverType: "user", EntityType: Model},
	{Entitlement: "reader", ReceiverType: "user:*", EntityType: Model},
	{Entitlement: "reader", ReceiverType: "group#member", EntityType: Model},
	{Entitlement: "writer", ReceiverType: "user", EntityType: Model},
	{Entitlement: "writer", ReceiverType: "user:*", EntityType: Model},
	{Entitlement: "writer", ReceiverType: "group#member", EntityType: Model},

	// serviceaccount
	{Entitlement: "administrator", ReceiverType: "user", EntityType: ServiceAccount},
	{Entitlement: "administrator", ReceiverType: "user:*", EntityType: ServiceAccount},
	{Entitlement: "administrator", ReceiverType: "group#member", EntityType: ServiceAccount},
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
