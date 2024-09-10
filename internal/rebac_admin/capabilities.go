// Copyright 2024 Canonical.

package rebac_admin

import (
	"context"

	"github.com/canonical/rebac-admin-ui-handlers/v1/interfaces"
	"github.com/canonical/rebac-admin-ui-handlers/v1/resources"
)

// capabilitiesService implements the `CapabilitiesService` interface.
type capabilitiesService struct {
}

// For doc/test sake, to hint that the struct needs to implement a specific interface.
var _ interfaces.CapabilitiesService = &capabilitiesService{}

func newCapabilitiesService() *capabilitiesService {
	return &capabilitiesService{}
}

var capabilities = []resources.Capability{
	{
		Endpoint: "/swagger.json",
		Methods: []resources.CapabilityMethods{
			"GET",
		},
	},
	{
		Endpoint: "/capabilities",
		Methods: []resources.CapabilityMethods{
			"GET",
		},
	},
	{
		Endpoint: "/identities",
		Methods: []resources.CapabilityMethods{
			"GET",
			"POST",
		},
	},
	{
		Endpoint: "/identities/{id}",
		Methods: []resources.CapabilityMethods{
			"GET",
		},
	},
	{
		Endpoint: "/identities/{id}/groups",
		Methods: []resources.CapabilityMethods{
			"GET",
			"PATCH",
		},
	},
	{
		Endpoint: "/identities/{id}/entitlements",
		Methods: []resources.CapabilityMethods{
			"GET",
			"PATCH",
		},
	},
	{
		Endpoint: "/groups",
		Methods: []resources.CapabilityMethods{
			"GET",
			"POST",
		},
	},
	{
		Endpoint: "/groups/{id}",
		Methods: []resources.CapabilityMethods{
			"GET",
			"PUT",
			"DELETE",
		},
	},
	{
		Endpoint: "/groups/{id}/identities",
		Methods: []resources.CapabilityMethods{
			"GET",
			"PATCH",
		},
	},
	{
		Endpoint: "/groups/{id}/entitlements",
		Methods: []resources.CapabilityMethods{
			"GET",
			"PATCH",
		},
	},
	{
		Endpoint: "/entitlements",
		Methods: []resources.CapabilityMethods{
			"GET",
		},
	},
	{
		Endpoint: "/entitlements/raw",
		Methods: []resources.CapabilityMethods{
			"GET",
		},
	},
	{
		Endpoint: "/resources",
		Methods: []resources.CapabilityMethods{
			"GET",
		},
	},
}

// ListCapabilities returns a list of capabilities supported by this service.
func (s *capabilitiesService) ListCapabilities(ctx context.Context) ([]resources.Capability, error) {

	return capabilities, nil
}
