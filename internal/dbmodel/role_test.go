// Copyright 2024 Canonical.

package dbmodel_test

import (
	"testing"

	qt "github.com/frankban/quicktest"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/pkg/names"
)

func TestRoleEntry_TableName(t *testing.T) {
	c := qt.New(t)
	c.Assert((&dbmodel.RoleEntry{}).TableName(), qt.Equals, "roles")
}

func TestRoleEntry_Tag(t *testing.T) {
	c := qt.New(t)

	rt := (&dbmodel.RoleEntry{
		UUID: "f979b3cd-ed92-442f-a0db-1fa8fc99434e",
	}).Tag().(names.RoleTag)
	c.Assert(rt.Id(), qt.Equals, "f979b3cd-ed92-442f-a0db-1fa8fc99434e")
}

func TestRoleEntry_ResourceTag(t *testing.T) {
	c := qt.New(t)

	rt := (&dbmodel.RoleEntry{
		UUID: "f979b3cd-ed92-442f-a0db-1fa8fc99434e",
	}).ResourceTag()
	c.Assert(rt.Id(), qt.Equals, "f979b3cd-ed92-442f-a0db-1fa8fc99434e")
}
