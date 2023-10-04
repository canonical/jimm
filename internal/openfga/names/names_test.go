// Copyright 2023 canonical.

package names_test

import (
	"testing"

	"github.com/juju/names/v4"
	gc "gopkg.in/check.v1"

	ofganames "github.com/canonical/jimm/internal/openfga/names"
	jimmnames "github.com/canonical/jimm/pkg/names"
	"github.com/google/uuid"
	"github.com/juju/juju/core/permission"
)

func Test(t *testing.T) {
	gc.TestingT(t)
}

var _ = gc.Suite(&namesSuite{})

type namesSuite struct {
}

func (s *namesSuite) TestFromResourceTag(c *gc.C) {
	id, err := uuid.NewRandom()
	c.Assert(err, gc.IsNil)

	result := ofganames.ConvertTag(names.NewControllerTag(id.String()))
	c.Assert(result, gc.DeepEquals, ofganames.NewTag(id.String(), names.ControllerTagKind, ""))

	result = ofganames.ConvertTag(names.NewModelTag(id.String()))
	c.Assert(result, gc.DeepEquals, ofganames.NewTag(id.String(), names.ModelTagKind, ""))

	result = ofganames.ConvertTag(names.NewUserTag("eve"))
	c.Assert(result, gc.DeepEquals, ofganames.NewTag("eve", names.UserTagKind, ""))

	result = ofganames.ConvertTag(names.NewApplicationOfferTag("test"))
	c.Assert(result, gc.DeepEquals, ofganames.NewTag("test", names.ApplicationOfferTagKind, ""))

	result = ofganames.ConvertTag(names.NewCloudTag("test"))
	c.Assert(result, gc.DeepEquals, ofganames.NewTag("test", names.CloudTagKind, ""))

	result = ofganames.ConvertTag(jimmnames.NewGroupTag("1"))
	c.Assert(result, gc.DeepEquals, ofganames.NewTag("1", jimmnames.GroupTagKind, ""))
}

func (s *namesSuite) TestFromGenericResourceTag(c *gc.C) {
	id, err := uuid.NewRandom()
	c.Assert(err, gc.IsNil)

	result := ofganames.ConvertGenericTag(names.NewControllerTag(id.String()))
	c.Assert(result, gc.DeepEquals, ofganames.NewTag(id.String(), names.ControllerTagKind, ""))

	result = ofganames.ConvertGenericTag(names.NewModelTag(id.String()))
	c.Assert(result, gc.DeepEquals, ofganames.NewTag(id.String(), names.ModelTagKind, ""))
}

func (s *namesSuite) TestConvertJujuRelation(c *gc.C) {
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
			c.Assert(err, gc.NotNil)
		} else {
			c.Assert(err, gc.IsNil)
		}
	}
}

func (s *namesSuite) TestParseRelations(c *gc.C) {
	for _, relation := range ofganames.AllRelations {
		res, err := ofganames.ParseRelation(relation.String())
		c.Assert(err, gc.IsNil, gc.Commentf("testing relation %s", relation))
		c.Assert(res, gc.Equals, relation, gc.Commentf("testing relation %s", relation))
	}
}
