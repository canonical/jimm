// Copyright 2024 Canonical Ltd.

package jimm_test

import (
	"context"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
	"github.com/google/uuid"

	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/jimm"
	"github.com/canonical/jimm/v3/internal/jimmtest"
	"github.com/canonical/jimm/v3/internal/openfga"
	"github.com/canonical/jimm/v3/internal/openfga/names"
	apiparams "github.com/canonical/jimm/v3/pkg/api/params"
)

func TestListRelationshipTuples(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	ofgaClient, _, _, err := jimmtest.SetupTestOFGAClient(c.Name())
	c.Assert(err, qt.IsNil)

	now := time.Now().UTC().Round(time.Millisecond)
	j := &jimm.JIMM{
		UUID: uuid.NewString(),
		Database: db.Database{
			DB: jimmtest.PostgresDB(c, func() time.Time { return now }),
		},
		OpenFGAClient: ofgaClient,
	}

	err = j.Database.Migrate(ctx, false)
	c.Assert(err, qt.IsNil)

	u := openfga.NewUser(&dbmodel.Identity{Name: "admin@canonical.com"}, ofgaClient)
	u.JimmAdmin = true

	user, _, controller, model, _, _, _ := createTestControllerEnvironment(ctx, c, j.Database)
	c.Assert(err, qt.IsNil)

	err = j.AddRelation(ctx, u, []apiparams.RelationshipTuple{
		{
			Object:       user.Tag().String(),
			Relation:     names.ReaderRelation.String(),
			TargetObject: model.ResourceTag().String(),
		},
		{
			Object:       user.Tag().String(),
			Relation:     names.WriterRelation.String(),
			TargetObject: model.ResourceTag().String(),
		},
		{
			Object:       user.Tag().String(),
			Relation:     names.AuditLogViewerRelation.String(),
			TargetObject: controller.ResourceTag().String(),
		},
	})
	c.Assert(err, qt.IsNil)

	// test listing all relations of all entities
	tuples, _, err := j.ListRelationshipTuples(ctx, u, apiparams.RelationshipTuple{
		Object:       "",
		Relation:     "",
		TargetObject: "",
	}, 10, "")
	c.Assert(err, qt.IsNil)
	c.Assert(len(tuples), qt.Equals, 3)

	// test listing a specific relation
	tuples, _, err = j.ListRelationshipTuples(ctx, u, apiparams.RelationshipTuple{
		Object:       user.Tag().String(),
		Relation:     names.ReaderRelation.String(),
		TargetObject: model.ResourceTag().String(),
	}, 10, "")
	c.Assert(err, qt.IsNil)
	c.Assert(tuples, qt.HasLen, 1)
	c.Assert(names.ReaderRelation.String(), qt.Equals, tuples[0].Relation.String())
	c.Assert(model.Tag().Id(), qt.Equals, tuples[0].Target.ID)

	// test listing all relations between two entities leaving relation empty
	tuples, _, err = j.ListRelationshipTuples(ctx, u, apiparams.RelationshipTuple{
		Object:       user.Tag().String(),
		Relation:     "",
		TargetObject: model.ResourceTag().String(),
	}, 10, "")
	c.Assert(err, qt.IsNil)
	c.Assert(tuples, qt.HasLen, 2)
	c.Assert(names.ReaderRelation.String(), qt.Equals, tuples[0].Relation.String())
	c.Assert(model.Tag().Id(), qt.Equals, tuples[0].Target.ID)
	c.Assert(names.WriterRelation.String(), qt.Equals, tuples[1].Relation.String())
	c.Assert(model.Tag().Id(), qt.Equals, tuples[1].Target.ID)

	// test listing all relations of a specific target entity
	tuples, _, err = j.ListRelationshipTuples(ctx, u, apiparams.RelationshipTuple{
		Object:       "",
		Relation:     "",
		TargetObject: model.ResourceTag().String(),
	}, 10, "")
	c.Assert(err, qt.IsNil)
	c.Assert(tuples, qt.HasLen, 2)
	c.Assert(names.ReaderRelation.String(), qt.Equals, tuples[0].Relation.String())
	c.Assert(model.Tag().Id(), qt.Equals, tuples[0].Target.ID)
	c.Assert(names.WriterRelation.String(), qt.Equals, tuples[1].Relation.String())
	c.Assert(model.Tag().Id(), qt.Equals, tuples[1].Target.ID)

	// test listing all relations of specific object entity
	tuples, _, err = j.ListRelationshipTuples(ctx, u, apiparams.RelationshipTuple{
		Object:       user.ResourceTag().String(),
		Relation:     names.ReaderRelation.String(),
		TargetObject: "model",
	}, 10, "")
	c.Assert(err, qt.IsNil)
	c.Assert(tuples, qt.HasLen, 1)

}
