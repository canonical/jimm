// Copyright 2024 Canonical.
package rebac_admin

var (
	NewGroupService       = newGroupService
	NewRoleService        = newRoleService
	NewidentitiesService  = newidentitiesService
	NewResourcesService   = newResourcesService
	NewEntitlementService = newEntitlementService
	EntitlementsList      = entitlementsList
	Capabilities          = capabilities
)

type GroupsService = groupsService
type RolesService = rolesService
