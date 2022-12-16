package jujuapi

import (
	"context"

	"github.com/CanonicalLtd/jimm/internal/jujuapi/rpc"
)

func init() {
	// Access Facade, OpenFga - https://warthogs.atlassian.net/browse/CSS-2431
	// Will ultimately replace ./usermanager.go
	facadeInit["Access"] = func(r *controllerRoot) []int {

		r.AddMethod("Access", 1, "AddGroup", rpc.Method(r.AddGroup))
		r.AddMethod("Access", 1, "RemoveGroup", rpc.Method(r.RemoveGroup))
		r.AddMethod("Access", 1, "RenameGroup", rpc.Method(r.RenameGroup))
		r.AddMethod("Access", 1, "ListGroups", rpc.Method(r.ListGroups))

		r.AddMethod("Access", 1, "AddRelation", rpc.Method(r.AddRelation))
		r.AddMethod("Access", 1, "RemoveRelation", rpc.Method(r.RemoveRelation))
		r.AddMethod("Access", 1, "CheckRelation", rpc.Method(r.CheckRelation))
		r.AddMethod("Access", 1, "ListRelations", rpc.Method(r.ListRelations))

		r.AddMethod("Access", 1, "GetAuthorisationModel", rpc.Method(r.GetAuthorisationModel))

		return []int{1}
	}
}

// AddGroup creates a new relational access control group tuple
// within OpenFGA.
func (r *controllerRoot) AddGroup(ctx context.Context) error {
	return nil
}

// RemoveGroup removes a relational access control group tuple
// within OpenFGA.
func (r *controllerRoot) RemoveGroup(ctx context.Context) error {
	return nil
}

// RenameGroup renames a relational access control group tuple
// within OpenFGA.
func (r *controllerRoot) RenameGroup(ctx context.Context) error {
	return nil
}

// ListGroup lists relational access control group tuple(s)
// within OpenFGA.
func (r *controllerRoot) ListGroups(ctx context.Context) error {
	return nil
}

// AddRelation creates a relational tuple between two objects [if applicable]
// within OpenFGA.
func (r *controllerRoot) AddRelation(ctx context.Context) error {
	return nil
}

// RemoveRelation removes a relational tuple between two objects [if applicable]
// within OpenFGA.
func (r *controllerRoot) RemoveRelation(ctx context.Context) error {
	return nil
}

// CheckRelation performs an authorisation check for a particular group/user tuple
// against another tuple [if applicable, i.e., they must have some form of relation existing]
// within OpenFGA.
//
// This corresponds directly to /stores/{store_id}/check.
func (r *controllerRoot) CheckRelation(ctx context.Context) error {
	return nil
}

// ListRelations TODO(ale8k): Confirm validity / need for this when using /expand or [EXPERIMENTAL] /list-objects
//
// See: https://openfga.dev/api/service#/Relationship%20Queries/Expand
func (r *controllerRoot) ListRelations(ctx context.Context) error {
	return nil
}

// GetAuthorisationModel retrieves a GET for an authorisation model in the JIMM store
// by name.
//
// TODO(ale8k): Confirm web team can/is happy to display this.
// TODO(ale8k): Should this be paginated? Probably not?
func (r *controllerRoot) GetAuthorisationModel(ctx context.Context) error {
	return nil
}
