// Copyright 2024 Canonical.

package jimm_test

import (
	"context"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
	"github.com/google/uuid"

	"github.com/canonical/jimm/v3/internal/common/pagination"
	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/jimm"
	"github.com/canonical/jimm/v3/internal/openfga"
	"github.com/canonical/jimm/v3/internal/openfga/names"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
	apiparams "github.com/canonical/jimm/v3/pkg/api/params"
)

func TestListRelationshipTuples(t *testing.T) {
	// setup
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
	type ExpectedTuple struct {
		expectedRelation string
		expectedTargetId string
	}
	// test
	testCases := []struct {
		description    string
		object         string
		relation       string
		targetObject   string
		expectedError  error
		expectedLength int
		expectedTuples []ExpectedTuple
	}{
		{
			description:    "test listing all relations of all entities",
			object:         "",
			relation:       "",
			targetObject:   "",
			expectedError:  nil,
			expectedLength: 3,
		},
		{
			description:    "test listing a specific relation",
			object:         user.Tag().String(),
			relation:       names.ReaderRelation.String(),
			targetObject:   model.ResourceTag().String(),
			expectedError:  nil,
			expectedLength: 1,
			expectedTuples: []ExpectedTuple{
				{

					expectedRelation: names.ReaderRelation.String(),
					expectedTargetId: model.Tag().Id(),
				},
			},
		},
		{
			description:    "test listing all relations between two entities leaving relation empty",
			object:         user.Tag().String(),
			relation:       "",
			targetObject:   model.ResourceTag().String(),
			expectedError:  nil,
			expectedLength: 2,
			expectedTuples: []ExpectedTuple{
				{
					expectedRelation: names.ReaderRelation.String(),
					expectedTargetId: model.Tag().Id(),
				},
				{
					expectedRelation: names.WriterRelation.String(),
					expectedTargetId: model.Tag().Id(),
				},
			},
		},
		{
			description:    "test listing all relations of a specific target entity",
			object:         "",
			relation:       "",
			targetObject:   model.ResourceTag().String(),
			expectedError:  nil,
			expectedLength: 2,
			expectedTuples: []ExpectedTuple{
				{
					expectedRelation: names.ReaderRelation.String(),
					expectedTargetId: model.Tag().Id(),
				},
				{
					expectedRelation: names.WriterRelation.String(),
					expectedTargetId: model.Tag().Id(),
				},
			},
		},
		{
			description:    "test listing all relations of specific object entity",
			object:         user.ResourceTag().String(),
			relation:       names.ReaderRelation.String(),
			targetObject:   "model",
			expectedError:  nil,
			expectedLength: 1,
			expectedTuples: []ExpectedTuple{
				{
					expectedRelation: names.ReaderRelation.String(),
					expectedTargetId: model.Tag().Id(),
				},
			},
		},
	}

	for _, t := range testCases {
		c.Run(t.description, func(c *qt.C) {
			tuples, _, err := j.ListRelationshipTuples(ctx, u, apiparams.RelationshipTuple{
				Object:       t.object,
				Relation:     t.relation,
				TargetObject: t.targetObject,
			}, 10, "")
			c.Assert(err, qt.Equals, t.expectedError)
			c.Assert(tuples, qt.HasLen, t.expectedLength)
			for i, expectedTuple := range t.expectedTuples {
				c.Assert(tuples[i].Relation.String(), qt.Equals, expectedTuple.expectedRelation)
				c.Assert(tuples[i].Target.ID, qt.Equals, expectedTuple.expectedTargetId)
			}
		})
	}
}

func TestListObjectRelations(t *testing.T) {
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
	type ExpectedTuple struct {
		expectedRelation string
		expectedTargetId string
	}

	testCases := []struct {
		description    string
		object         string
		initialToken   pagination.EntitlementToken
		expectedError  string
		expectedLength int
		expectedTuples []ExpectedTuple
	}{
		{
			description:    "test listing all relations",
			object:         user.Tag().String(),
			expectedLength: 3,
		},
		{
			description:   "invalid initial token",
			initialToken:  pagination.NewEntitlementToken("bar"),
			expectedError: "failed to decode pagination token.*",
		},
		{
			description:   "invalid user tag token",
			object:        "foo" + user.Tag().String(),
			expectedError: "failed to map tag, unknown kind: foouser",
		},
	}

	for _, t := range testCases {
		c.Run(t.description, func(c *qt.C) {
			token := t.initialToken
			tuples := []openfga.Tuple{}
			for {
				res, nextToken, err := j.ListObjectRelations(ctx, u, t.object, 10, token)
				if t.expectedError != "" {
					c.Assert(err, qt.ErrorMatches, t.expectedError)
					break
				}
				tuples = append(tuples, res...)
				if nextToken.String() == "" {
					break
				}
				token = nextToken
			}
			c.Assert(tuples, qt.HasLen, t.expectedLength)
			for i, expectedTuple := range t.expectedTuples {
				c.Assert(tuples[i].Relation.String(), qt.Equals, expectedTuple.expectedRelation)
				c.Assert(tuples[i].Target.ID, qt.Equals, expectedTuple.expectedTargetId)
			}
		})
	}
}
