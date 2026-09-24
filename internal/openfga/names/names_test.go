// Copyright 2024 Canonical.

package names_test

import (
	"testing"

	qt "github.com/frankban/quicktest"
	"github.com/google/uuid"
	"github.com/juju/juju/core/permission"
	"github.com/juju/names/v6"

	ofganames "github.com/canonical/jimm/v3/internal/openfga/names"
	jimmnames "github.com/canonical/jimm/v3/pkg/names"
)

func TestFromResourceTag(t *testing.T) {
	c := qt.New(t)
	id, err := uuid.NewRandom()
	c.Assert(err, qt.IsNil)

	result := ofganames.ConvertTag(names.NewControllerTag(id.String()))
	c.Assert(result, qt.DeepEquals, ofganames.NewTag(id.String(), names.ControllerTagKind, ""))

	result = ofganames.ConvertTag(names.NewModelTag(id.String()))
	c.Assert(result, qt.DeepEquals, ofganames.NewTag(id.String(), names.ModelTagKind, ""))

	result = ofganames.ConvertTag(names.NewUserTag("eve"))
	c.Assert(result, qt.DeepEquals, ofganames.NewTag("eve", names.UserTagKind, ""))

	result = ofganames.ConvertTag(names.NewApplicationOfferTag("test"))
	c.Assert(result, qt.DeepEquals, ofganames.NewTag("test", names.ApplicationOfferTagKind, ""))

	result = ofganames.ConvertTag(names.NewCloudTag("test"))
	c.Assert(result, qt.DeepEquals, ofganames.NewTag("test", names.CloudTagKind, ""))

	result = ofganames.ConvertTag(jimmnames.NewGroupTag(id.String()))
	c.Assert(result, qt.DeepEquals, ofganames.NewTag(id.String(), jimmnames.GroupTagKind, ""))
}

func TestFromGenericResourceTag(t *testing.T) {
	c := qt.New(t)
	id, err := uuid.NewRandom()
	c.Assert(err, qt.IsNil)

	result := ofganames.ConvertGenericTag(names.NewControllerTag(id.String()))
	c.Assert(result, qt.DeepEquals, ofganames.NewTag(id.String(), names.ControllerTagKind, ""))

	result = ofganames.ConvertGenericTag(names.NewModelTag(id.String()))
	c.Assert(result, qt.DeepEquals, ofganames.NewTag(id.String(), names.ModelTagKind, ""))
}

func TestConvertJujuRelation(t *testing.T) {
	c := qt.New(t)
	// unusedAccessLevels are access levels that are not
	// represented in JIMM's OpenFGA model and should return
	// an error.
	unusedAccessLevels := map[permission.Access]struct{}{
		permission.NoAccess:        {},
		permission.SuperuserAccess: {},
		permission.LoginAccess:     {},
	}
	for i, level := range permission.AllAccessLevels {
		c.Logf("running test %d: %s", i, level)
		_, err := ofganames.ConvertJujuRelation(string(level))
		if _, ok := unusedAccessLevels[level]; ok {
			c.Assert(err, qt.IsNotNil)
		} else {
			c.Assert(err, qt.IsNil)
		}
	}
}

func TestParseRelations(t *testing.T) {
	c := qt.New(t)
	for _, relation := range ofganames.AllRelations {
		res, err := ofganames.ParseRelation(relation.String())
		c.Assert(err, qt.IsNil, qt.Commentf("testing relation %s", relation))
		c.Assert(res, qt.Equals, relation, qt.Commentf("testing relation %s", relation))
	}
}
