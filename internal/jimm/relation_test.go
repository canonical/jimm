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

	user, _, _, model, _, _, _ := createTestControllerEnvironment(ctx, c, j.Database)
	c.Assert(err, qt.IsNil)
	// gr2, err := j.AddGroup(ctx, u, "group-test2")
	// c.Assert(err, qt.IsNil)

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
	})
	c.Assert(err, qt.IsNil)

	tuples, _, err := j.ListRelationshipTuples(ctx, u, apiparams.RelationshipTuple{
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
}
