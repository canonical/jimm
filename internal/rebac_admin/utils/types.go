package utils

import (
	"github.com/canonical/jimm/v3/internal/openfga"
	"github.com/canonical/rebac-admin-ui-handlers/v1/resources"
)

func ToEntityEntitlement(tuple openfga.Tuple) resources.EntityEntitlement {
	return resources.EntityEntitlement{
		Entitlement: string(tuple.Relation),
		EntityId:    tuple.Target.ID,
		EntityType:  tuple.Target.Kind.String(),
	}
}

func ToEntityEntitlements(tuples []openfga.Tuple) []resources.EntityEntitlement {
	entitlements := make([]resources.EntityEntitlement, 0, len(tuples))
	for _, t := range tuples {
		entitlements = append(entitlements, ToEntityEntitlement(t))
	}
	return entitlements
}
