// Copyright 2025 Canonical.

package testing

import (
	"context"
	"testing"

	qt "github.com/frankban/quicktest"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/openfga"
	ofganames "github.com/canonical/jimm/v3/internal/openfga/names"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
	"github.com/canonical/jimm/v3/pkg/api"
	apiparams "github.com/canonical/jimm/v3/pkg/api/params"
)

/*
 Role facade related tests
*/

func TestAddRole(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupJimmWithControllers(c)

	conn := s.Open(c, nil, "alice", nil)
	defer conn.Close()

	client := api.NewClient(conn)
	res, err := client.AddRole(&apiparams.AddRoleRequest{Name: "test-role"})
	c.Assert(err, qt.IsNil)
	c.Assert(res.UUID, qt.Not(qt.Equals), "")

	_, err = client.AddRole(&apiparams.AddRoleRequest{Name: "test-role"})
	c.Assert(err, qt.ErrorMatches, ".*already exists.*")
}

func TestGetRole(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupJimmWithControllers(c)

	conn := s.Open(c, nil, "alice", nil)
	defer conn.Close()

	client := api.NewClient(conn)

	created, err := client.AddRole(&apiparams.AddRoleRequest{Name: "test-role"})
	c.Assert(err, qt.IsNil)

	retrievedUuid, err := client.GetRole(&apiparams.GetRoleRequest{UUID: created.UUID})
	c.Assert(err, qt.IsNil)
	c.Assert(retrievedUuid.Role, qt.DeepEquals, created.Role)

	retrievedName, err := client.GetRole(&apiparams.GetRoleRequest{Name: created.Name})
	c.Assert(err, qt.IsNil)
	c.Assert(retrievedName.Role, qt.DeepEquals, created.Role)

	_, err = client.GetRole(&apiparams.GetRoleRequest{UUID: "non-existent"})
	c.Assert(err, qt.ErrorMatches, ".*not found.*")

	_, err = client.GetRole(&apiparams.GetRoleRequest{Name: created.Name, UUID: created.UUID})
	c.Assert(err, qt.ErrorMatches, ".*only one of.*")

	_, err = client.GetRole(&apiparams.GetRoleRequest{
		Name: "#####",
	})
	c.Assert(err, qt.ErrorMatches, ".*invalid role name.*")

}

func TestRemoveRole(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupJimmWithControllers(c)

	conn := s.Open(c, nil, "alice", nil)
	defer conn.Close()

	client := api.NewClient(conn)

	err := client.RemoveRole(&apiparams.RemoveRoleRequest{
		Name: "test-role",
	})
	c.Assert(err, qt.ErrorMatches, ".*not found.*")

	err = client.RemoveRole(&apiparams.RemoveRoleRequest{
		Name: "#####",
	})
	c.Assert(err, qt.ErrorMatches, ".*invalid role name.*")

	_, err = client.AddRole(&apiparams.AddRoleRequest{Name: "test-role"})
	c.Assert(err, qt.IsNil)

	err = client.RemoveRole(&apiparams.RemoveRoleRequest{
		Name: "test-role",
	})
	c.Assert(err, qt.IsNil)
}

func TestRemoveRoleRemovesTuples(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupJimmWithControllers(c)

	ctx := context.Background()
	db := s.JIMM.Database

	user, _, controller, model, _, _, _, client, closeClient := createTestControllerEnvironment(c, s)
	defer closeClient()

	_, err := db.AddRole(ctx, "test-role2")
	c.Assert(err, qt.IsNil)

	role := &dbmodel.RoleEntry{
		Name: "test-role2",
	}
	err = db.GetRole(ctx, role)
	c.Assert(err, qt.IsNil)

	tuples := []openfga.Tuple{{
		Object:   ofganames.ConvertTag(user.ResourceTag()),
		Relation: ofganames.AssigneeRelation,
		Target:   ofganames.ConvertTag(role.ResourceTag()),
	}, {
		Object:   ofganames.ConvertTagWithRelation(role.ResourceTag(), ofganames.AssigneeRelation),
		Relation: "administrator",
		Target:   ofganames.ConvertTag(controller.ResourceTag()),
	}, {
		Object:   ofganames.ConvertTagWithRelation(role.ResourceTag(), ofganames.AssigneeRelation),
		Relation: "writer",
		Target:   ofganames.ConvertTag(model.ResourceTag()),
	},
	}

	u := user.Tag().String()

	checkAccessTupleController := apiparams.RelationshipTuple{Object: u, Relation: "administrator", TargetObject: "controller-" + controller.UUID}
	checkAccessTupleModel := apiparams.RelationshipTuple{Object: u, Relation: "writer", TargetObject: "model-" + model.UUID.String}

	err = s.JIMM.OpenFGAClient.AddRelation(context.Background(), tuples...)
	c.Assert(err, qt.IsNil)
	// Check user has access to model and controller through role2
	checkResp, err := client.CheckRelation(&apiparams.CheckRelationRequest{Tuple: checkAccessTupleController})
	c.Assert(err, qt.IsNil)
	c.Assert(checkResp.Allowed, qt.Equals, true)
	checkResp, err = client.CheckRelation(&apiparams.CheckRelationRequest{Tuple: checkAccessTupleModel})
	c.Assert(err, qt.IsNil)
	c.Assert(checkResp.Allowed, qt.Equals, true)

	err = client.RemoveRole(&apiparams.RemoveRoleRequest{Name: role.Name})
	c.Assert(err, qt.IsNil)

	// Check user access has been revoked.
	checkResp, err = client.CheckRelation(&apiparams.CheckRelationRequest{Tuple: checkAccessTupleController})
	c.Assert(err, qt.IsNil)
	c.Assert(checkResp.Allowed, qt.Equals, false)
	checkResp, err = client.CheckRelation(&apiparams.CheckRelationRequest{Tuple: checkAccessTupleModel})
	c.Assert(err, qt.IsNil)
	c.Assert(checkResp.Allowed, qt.Equals, false)
}

func TestRenameRole(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupJimmWithControllers(c)

	conn := s.Open(c, nil, "alice", nil)
	defer conn.Close()

	client := api.NewClient(conn)

	err := client.RenameRole(&apiparams.RenameRoleRequest{
		Name:    "test-role",
		NewName: "renamed-role",
	})
	c.Assert(err, qt.ErrorMatches, ".*not found.*")

	_, err = client.AddRole(&apiparams.AddRoleRequest{Name: "test-role"})
	c.Assert(err, qt.IsNil)

	err = client.RenameRole(&apiparams.RenameRoleRequest{
		Name:    "test-role",
		NewName: "renamed-role",
	})
	c.Assert(err, qt.IsNil)
}

func TestListRoles(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupJimmWithControllers(c)

	conn := s.Open(c, nil, "alice", nil)
	defer conn.Close()

	client := api.NewClient(conn)

	roleNames := []string{
		"test-role0",
		"test-role1",
		"test-role2",
		"aaaFinalRole",
	}

	for _, name := range roleNames {
		_, err := client.AddRole(&apiparams.AddRoleRequest{Name: name})
		c.Assert(err, qt.IsNil)
	}
	req := apiparams.ListRolesRequest{Limit: 10, Offset: 0}
	roles, err := client.ListRoles(&req)
	c.Assert(err, qt.IsNil)
	c.Assert(roles, qt.HasLen, 4)
	// Verify the UUID is not empty.
	c.Assert(roles[0].UUID, qt.Not(qt.Equals), "")
	// roles should be returned in ascending order of name
	c.Assert(roles[0].Name, qt.Equals, "aaaFinalRole")
	c.Assert(roles[1].Name, qt.Equals, "test-role0")
	c.Assert(roles[2].Name, qt.Equals, "test-role1")
	c.Assert(roles[3].Name, qt.Equals, "test-role2")
}

func TestUnauthorizedUserForRoleManagerment(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupJimmWithControllers(c)

	conn := s.Open(c, nil, "not-authorized-user", nil)
	defer conn.Close()
	client := api.NewClient(conn)

	_, err := client.GetRole(&apiparams.GetRoleRequest{Name: "name"})
	c.Assert(err, qt.ErrorMatches, ".*unauthorized.*")
	err = client.RemoveRole(&apiparams.RemoveRoleRequest{Name: "name"})
	c.Assert(err, qt.ErrorMatches, ".*unauthorized.*")
	_, err = client.AddRole(&apiparams.AddRoleRequest{Name: "name"})
	c.Assert(err, qt.ErrorMatches, ".*unauthorized.*")
	err = client.RenameRole(&apiparams.RenameRoleRequest{Name: "name", NewName: "rename"})
	c.Assert(err, qt.ErrorMatches, ".*unauthorized.*")
	_, err = client.ListRoles(&apiparams.ListRolesRequest{})
	c.Assert(err, qt.ErrorMatches, ".*unauthorized.*")
}
