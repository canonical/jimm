// Copyright 2025 Canonical.

package permissions_test

import (
	"context"
	"fmt"
	"slices"

	petname "github.com/dustinkirkland/golang-petname"
	qt "github.com/frankban/quicktest"
	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"github.com/canonical/jimm/v3/internal/common/pagination"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/jimm/permissions"
	"github.com/canonical/jimm/v3/internal/openfga"
	"github.com/canonical/jimm/v3/internal/openfga/names"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
	apiparams "github.com/canonical/jimm/v3/pkg/api/params"
)

func (s *permissionManagerSuite) TestListRelationshipTuples(c *qt.C) {
	c.Parallel()
	ctx := context.Background()

	u := openfga.NewUser(&dbmodel.Identity{Name: "admin@canonical.com"}, s.ofgaClient)
	u.JimmAdmin = true

	user, _, controller, model, _, _, _, _ := jimmtest.CreateTestControllerEnvironment(ctx, c, s.db)

	err := s.manager.AddRelation(ctx, u, []apiparams.RelationshipTuple{
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
			expectedLength: 4,
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
			tuples, _, err := s.manager.ListRelationshipTuples(ctx, s.adminUser, apiparams.RelationshipTuple{
				Object:       t.object,
				Relation:     t.relation,
				TargetObject: t.targetObject,
			}, 10, "")
			c.Assert(err, qt.Equals, t.expectedError)
			c.Assert(tuples, qt.HasLen, t.expectedLength)
			// Sort the tuples by relation in ascending order
			// to ensure the order is consistent for testing.
			sortFunc := func(a, b openfga.Tuple) int {
				if a.Relation < b.Relation {
					return -1
				}
				if a.Relation > b.Relation {
					return 1
				}
				return 0
			}
			slices.SortFunc(tuples, sortFunc)
			sortFuncExpected := func(a, b ExpectedTuple) int {
				if a.expectedRelation < b.expectedRelation {
					return -1
				}
				if a.expectedRelation > b.expectedRelation {
					return 1
				}
				return 0
			}
			slices.SortFunc(t.expectedTuples, sortFuncExpected)
			for i, expectedTuple := range t.expectedTuples {
				c.Assert(tuples[i].Relation.String(), qt.Equals, expectedTuple.expectedRelation)
				c.Assert(tuples[i].Target.ID, qt.Equals, expectedTuple.expectedTargetId)
			}
		})
	}
}

func (s *permissionManagerSuite) TestCheckRelationUsesUserIDPGroupsAsContextualTuples(c *qt.C) {
	c.Parallel()
	ctx := context.Background()

	userIdentity, err := dbmodel.NewIdentity(fmt.Sprintf("subject-%s", petname.Generate(2, "-")))
	c.Assert(err, qt.IsNil)
	c.Assert(s.db.DB.Create(userIdentity).Error, qt.IsNil)
	user := openfga.NewUser(userIdentity, s.ofgaClient)
	user.SetIDPGroups([]string{"engineering-team"})

	_, _, _, model, _, _, _, _ := jimmtest.CreateTestControllerEnvironment(ctx, c, s.db)
	err = s.manager.AddRelation(ctx, s.adminUser, []apiparams.RelationshipTuple{{
		Object:       "idpgroup-engineering-team#member",
		Relation:     names.ReaderRelation.String(),
		TargetObject: model.ResourceTag().String(),
	}})
	c.Assert(err, qt.IsNil)

	checkTuple := apiparams.RelationshipTuple{
		Object:       userIdentity.Tag().String(),
		Relation:     names.ReaderRelation.String(),
		TargetObject: model.ResourceTag().String(),
	}

	allowed, err := s.manager.CheckRelation(ctx, openfga.NewUser(userIdentity, s.ofgaClient), checkTuple, false)
	c.Assert(err, qt.IsNil)
	c.Assert(allowed, qt.IsFalse)

	allowed, err = s.manager.CheckRelation(ctx, user, checkTuple, false)
	c.Assert(err, qt.IsNil)
	c.Assert(allowed, qt.IsTrue)
}

func (s *permissionManagerSuite) TestResourceAdminCheckUsesUserIDPGroupsAsContextualTuples(c *qt.C) {
	c.Parallel()
	ctx := context.Background()

	grantorIdentity, err := dbmodel.NewIdentity(fmt.Sprintf("grantor-%s", petname.Generate(2, "-")))
	c.Assert(err, qt.IsNil)
	c.Assert(s.db.DB.Create(grantorIdentity).Error, qt.IsNil)
	grantor := openfga.NewUser(grantorIdentity, s.ofgaClient)
	grantor.SetIDPGroups([]string{"model-admins"})

	subjectIdentity, err := dbmodel.NewIdentity(fmt.Sprintf("subject-%s", petname.Generate(2, "-")))
	c.Assert(err, qt.IsNil)
	c.Assert(s.db.DB.Create(subjectIdentity).Error, qt.IsNil)

	_, _, _, model, _, _, _, _ := jimmtest.CreateTestControllerEnvironment(ctx, c, s.db)
	err = s.manager.AddRelation(ctx, s.adminUser, []apiparams.RelationshipTuple{{
		Object:       "idpgroup-model-admins#member",
		Relation:     names.AdministratorRelation.String(),
		TargetObject: model.ResourceTag().String(),
	}})
	c.Assert(err, qt.IsNil)

	tuples := []apiparams.RelationshipTuple{{
		Object:       subjectIdentity.Tag().String(),
		Relation:     names.ReaderRelation.String(),
		TargetObject: model.ResourceTag().String(),
	}}

	err = s.manager.AddRelation(ctx, openfga.NewUser(grantorIdentity, s.ofgaClient), tuples)
	c.Assert(err, qt.ErrorMatches, "unauthorized")

	err = s.manager.AddRelation(ctx, grantor, tuples)
	c.Assert(err, qt.IsNil)
}

func (s *permissionManagerSuite) TestListObjectRelations(c *qt.C) {
	c.Parallel()
	ctx := context.Background()

	u := openfga.NewUser(&dbmodel.Identity{Name: "admin@canonical.com"}, s.ofgaClient)
	u.JimmAdmin = true

	user, group, controller, model, _, cloud, _, _ := jimmtest.CreateTestControllerEnvironment(ctx, c, s.db)

	err := s.manager.AddRelation(ctx, u, []apiparams.RelationshipTuple{
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
		{
			Object:       user.Tag().String(),
			Relation:     names.AdministratorRelation.String(),
			TargetObject: controller.ResourceTag().String(),
		},
		{
			Object:       user.Tag().String(),
			Relation:     names.AdministratorRelation.String(),
			TargetObject: cloud.ResourceTag().String(),
		},
		{
			Object:       user.Tag().String(),
			Relation:     names.CanAddModelRelation.String(),
			TargetObject: cloud.ResourceTag().String(),
		},
		{
			Object:       user.Tag().String(),
			Relation:     names.MemberRelation.String(),
			TargetObject: group.ResourceTag().String(),
		},
	})

	c.Assert(err, qt.IsNil)
	type ExpectedTuple struct {
		expectedRelation string
		expectedTargetId string
	}

	testCases := []struct {
		description          string
		object               string
		initialToken         pagination.EntitlementToken
		pageSize             int32
		expectNumPages       int
		expectedError        string
		expectedTuplesLength int
		expectedTuples       []ExpectedTuple
	}{
		{
			description:          "test listing all relations in single page",
			object:               user.Tag().String(),
			pageSize:             10,
			expectNumPages:       1,
			expectedTuplesLength: 7,
		},
		{
			description:          "test listing all relations in multiple pages",
			object:               user.Tag().String(),
			pageSize:             2,
			expectNumPages:       4,
			expectedTuplesLength: 7,
		},
		{
			description:   "invalid initial token",
			object:        user.Tag().String(),
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
			numPages := 0
			for {
				res, nextToken, err := s.manager.ListObjectRelations(ctx, s.adminUser, t.object, t.pageSize, token)
				if t.expectedError != "" {
					c.Assert(err, qt.ErrorMatches, t.expectedError)
					break
				}
				tuples = append(tuples, res...)
				numPages += 1
				if nextToken.String() == "" {
					break
				}
				token = nextToken
			}
			c.Assert(numPages, qt.Equals, t.expectNumPages)
			c.Assert(tuples, qt.HasLen, t.expectedTuplesLength)
			for i, expectedTuple := range t.expectedTuples {
				c.Assert(tuples[i].Relation.String(), qt.Equals, expectedTuple.expectedRelation)
				c.Assert(tuples[i].Target.ID, qt.Equals, expectedTuple.expectedTargetId)
			}
		})
	}
}

func (s *permissionManagerSuite) TestListResources(c *qt.C) {
	c.Parallel()
	ctx := context.Background()

	_, _, controller, model, applicationOffer, cloud, _, _ := jimmtest.CreateTestControllerEnvironment(ctx, c, s.db)

	ids := []string{applicationOffer.UUID, cloud.Name, controller.UUID, model.UUID.String}

	testCases := []struct {
		desc       string
		limit      int
		offset     int
		identities []string
	}{
		{
			desc:       "test with first resources",
			limit:      3,
			offset:     0,
			identities: []string{ids[0], ids[1], ids[2]},
		},
		{
			desc:       "test with remianing ids",
			limit:      3,
			offset:     3,
			identities: []string{ids[3]},
		},
		{
			desc:       "test out of range",
			limit:      3,
			offset:     6,
			identities: []string{},
		},
	}
	for _, t := range testCases {
		c.Run(t.desc, func(c *qt.C) {
			filter := pagination.NewOffsetFilter(t.limit, t.offset)
			resources, err := s.manager.ListResources(ctx, s.adminUser, filter, "", "")
			c.Assert(err, qt.IsNil)
			c.Assert(resources, qt.HasLen, len(t.identities))
			for i := range len(t.identities) {
				c.Assert(resources[i].ID.String, qt.Equals, t.identities[i])
			}
		})
	}
}

func (s *permissionManagerSuite) TestCheckPermissions(c *qt.C) {
	c.Parallel()
	ctx := context.Background()

	u := openfga.NewUser(&dbmodel.Identity{Name: "admin@canonical.com"}, s.ofgaClient)
	u.JimmAdmin = true

	user, group, controller, model, _, cloud, _, _ := jimmtest.CreateTestControllerEnvironment(ctx, c, s.db)
	tuples := []apiparams.RelationshipTuple{
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
		{
			Object:       user.Tag().String(),
			Relation:     names.AdministratorRelation.String(),
			TargetObject: controller.ResourceTag().String(),
		},
		{
			Object:       user.Tag().String(),
			Relation:     names.AdministratorRelation.String(),
			TargetObject: cloud.ResourceTag().String(),
		},
		{
			Object:       user.Tag().String(),
			Relation:     names.CanAddModelRelation.String(),
			TargetObject: cloud.ResourceTag().String(),
		},
		{
			Object:       user.Tag().String(),
			Relation:     names.MemberRelation.String(),
			TargetObject: group.ResourceTag().String(),
		},
	}
	err := s.manager.AddRelation(ctx, u, tuples)

	c.Assert(err, qt.IsNil)
	results, err := s.manager.CheckRelations(ctx, u, tuples)
	c.Assert(err, qt.IsNil)
	c.Assert(results, qt.HasLen, len(tuples))
	for i := range tuples {
		c.Assert(results[i].Allowed, qt.IsTrue)
		c.Assert(results[i].Error, qt.IsNil)
	}
}

func (s *permissionManagerSuite) TestCheckRelationsWithErrors(c *qt.C) {
	c.Parallel()
	ctx := context.Background()

	u := openfga.NewUser(&dbmodel.Identity{Name: "admin@canonical.com"}, s.ofgaClient)
	u.JimmAdmin = true

	user, _, _, model, _, _, _, _ := jimmtest.CreateTestControllerEnvironment(ctx, c, s.db)
	tuples := []apiparams.RelationshipTuple{
		{
			Object:       user.Tag().String(),
			Relation:     names.ReaderRelation.String(),
			TargetObject: model.ResourceTag().String(),
		},
	}

	err := s.manager.AddRelation(ctx, u, tuples)
	c.Assert(err, qt.IsNil)
	tuplesToCheck := tuples
	tuplesToCheck = append(tuplesToCheck, apiparams.RelationshipTuple{
		Object:       "invalid-object",
		Relation:     names.WriterRelation.String(),
		TargetObject: model.ResourceTag().String(),
	})
	results, err := s.manager.CheckRelations(ctx, u, tuplesToCheck)
	c.Assert(err, qt.IsNil)
	c.Assert(results, qt.HasLen, len(tuplesToCheck))
	c.Assert(results[0].Allowed, qt.IsTrue)
	c.Assert(results[0].Error, qt.IsNil)
	c.Assert(results[1].Allowed, qt.IsFalse)
	c.Assert(results[1].Error, qt.IsNotNil)
}

func (s *permissionManagerSuite) TestRelationManagementAsResourceAdministrator(c *qt.C) {
	c.Parallel()
	ctx := context.Background()

	grantorIdentity, err := dbmodel.NewIdentity(fmt.Sprintf("grantor-%s", petname.Generate(2, "-")))
	c.Assert(err, qt.IsNil)
	c.Assert(s.db.DB.Create(grantorIdentity).Error, qt.IsNil)
	grantor := openfga.NewUser(grantorIdentity, s.ofgaClient)

	subjectIdentity, err := dbmodel.NewIdentity(fmt.Sprintf("subject-%s", petname.Generate(2, "-")))
	c.Assert(err, qt.IsNil)
	c.Assert(s.db.DB.Create(subjectIdentity).Error, qt.IsNil)

	_, _, controller, model, offer, cloud, _, _ := jimmtest.CreateTestControllerEnvironment(ctx, c, s.db)

	testCases := []struct {
		description string
		grantAdmin  func() error
		tuple       apiparams.RelationshipTuple
	}{
		{
			description: "controller administrator can manage relations",
			grantAdmin: func() error {
				return grantor.SetControllerAccess(ctx, controller.ResourceTag(), names.AdministratorRelation)
			},
			tuple: apiparams.RelationshipTuple{
				Object:       subjectIdentity.Tag().String(),
				Relation:     names.AuditLogViewerRelation.String(),
				TargetObject: controller.ResourceTag().String(),
			},
		},
		{
			description: "model administrator can manage relations",
			grantAdmin: func() error {
				return grantor.SetModelAccess(ctx, model.ResourceTag(), names.AdministratorRelation)
			},
			tuple: apiparams.RelationshipTuple{
				Object:       subjectIdentity.Tag().String(),
				Relation:     names.ReaderRelation.String(),
				TargetObject: model.ResourceTag().String(),
			},
		},
		{
			description: "model administrator can manage IDP group relations",
			grantAdmin: func() error {
				return grantor.SetModelAccess(ctx, model.ResourceTag(), names.AdministratorRelation)
			},
			tuple: apiparams.RelationshipTuple{
				Object:       "idpgroup-engineering-team#member",
				Relation:     names.ReaderRelation.String(),
				TargetObject: model.ResourceTag().String(),
			},
		},
		{
			description: "application offer administrator can manage relations",
			grantAdmin: func() error {
				return grantor.SetApplicationOfferAccess(ctx, offer.ResourceTag(), names.AdministratorRelation)
			},
			tuple: apiparams.RelationshipTuple{
				Object:       subjectIdentity.Tag().String(),
				Relation:     names.ReaderRelation.String(),
				TargetObject: offer.ResourceTag().String(),
			},
		},
		{
			description: "cloud administrator can manage relations",
			grantAdmin: func() error {
				return grantor.SetCloudAccess(ctx, cloud.ResourceTag(), names.AdministratorRelation)
			},
			tuple: apiparams.RelationshipTuple{
				Object:       subjectIdentity.Tag().String(),
				Relation:     names.CanAddModelRelation.String(),
				TargetObject: cloud.ResourceTag().String(),
			},
		},
	}

	for _, testCase := range testCases {
		c.Run(testCase.description, func(c *qt.C) {
			c.Assert(testCase.grantAdmin(), qt.IsNil)

			err := s.manager.AddRelation(ctx, grantor, []apiparams.RelationshipTuple{testCase.tuple})
			c.Assert(err, qt.IsNil)

			allowed, err := s.manager.CheckRelation(ctx, grantor, testCase.tuple, false)
			c.Assert(err, qt.IsNil)
			c.Assert(allowed, qt.IsTrue)

			results, err := s.manager.CheckRelations(ctx, grantor, []apiparams.RelationshipTuple{testCase.tuple})
			c.Assert(err, qt.IsNil)
			c.Assert(results, qt.HasLen, 1)
			c.Assert(results[0].Allowed, qt.IsTrue)
			c.Assert(results[0].Error, qt.IsNil)

			err = s.manager.RemoveRelation(ctx, grantor, []apiparams.RelationshipTuple{testCase.tuple})
			c.Assert(err, qt.IsNil)

			allowed, err = s.manager.CheckRelation(ctx, grantor, testCase.tuple, false)
			c.Assert(err, qt.IsNil)
			c.Assert(allowed, qt.IsFalse)
		})
	}
}

func (s *permissionManagerSuite) TestGroupAndRoleRelationManagementRemainJimmAdminOnly(c *qt.C) {
	c.Parallel()
	ctx := context.Background()

	grantorIdentity, err := dbmodel.NewIdentity(fmt.Sprintf("grantor-%s", petname.Generate(2, "-")))
	c.Assert(err, qt.IsNil)
	c.Assert(s.db.DB.Create(grantorIdentity).Error, qt.IsNil)
	grantor := openfga.NewUser(grantorIdentity, s.ofgaClient)

	subjectIdentity, err := dbmodel.NewIdentity(fmt.Sprintf("subject-%s", petname.Generate(2, "-")))
	c.Assert(err, qt.IsNil)
	c.Assert(s.db.DB.Create(subjectIdentity).Error, qt.IsNil)

	_, group, _, _, _, _, _, role := jimmtest.CreateTestControllerEnvironment(ctx, c, s.db)

	testCases := []struct {
		description string
		tuple       apiparams.RelationshipTuple
	}{
		{
			description: "group membership changes remain restricted",
			tuple: apiparams.RelationshipTuple{
				Object:       subjectIdentity.Tag().String(),
				Relation:     names.MemberRelation.String(),
				TargetObject: group.ResourceTag().String(),
			},
		},
		{
			description: "role assignment changes remain restricted",
			tuple: apiparams.RelationshipTuple{
				Object:       subjectIdentity.Tag().String(),
				Relation:     names.AssigneeRelation.String(),
				TargetObject: role.ResourceTag().String(),
			},
		},
	}

	for _, testCase := range testCases {
		c.Run(testCase.description, func(c *qt.C) {
			err := s.manager.AddRelation(ctx, grantor, []apiparams.RelationshipTuple{testCase.tuple})
			c.Assert(err, qt.ErrorMatches, "unauthorized")

			_, err = s.manager.CheckRelation(ctx, grantor, testCase.tuple, false)
			c.Assert(err, qt.ErrorMatches, "unauthorized")

			results, err := s.manager.CheckRelations(ctx, grantor, []apiparams.RelationshipTuple{testCase.tuple})
			c.Assert(err, qt.IsNil)
			c.Assert(results, qt.HasLen, 1)
			c.Assert(results[0].Allowed, qt.IsFalse)
			c.Assert(results[0].Error, qt.ErrorMatches, "unauthorized")

			err = s.manager.RemoveRelation(ctx, grantor, []apiparams.RelationshipTuple{testCase.tuple})
			c.Assert(err, qt.ErrorMatches, "unauthorized")
		})
	}
}

// TestStructuralRelationManagementRequiresJimmAdmin ensures that a resource
// administrator (a non-JIMM-admin who administers a resource) cannot manage the
// structural relations that define the resource hierarchy (controller, model),
// nor grant an access relation to a non-grantee object kind. Without this guard
// a model administrator could, for example, remove the model->controller tuple
// and detach their model from its controller. Only JIMM admins may manage these.
func (s *permissionManagerSuite) TestStructuralRelationManagementRequiresJimmAdmin(c *qt.C) {
	c.Parallel()
	ctx := context.Background()

	grantorIdentity, err := dbmodel.NewIdentity(fmt.Sprintf("grantor-%s", petname.Generate(2, "-")))
	c.Assert(err, qt.IsNil)
	c.Assert(s.db.DB.Create(grantorIdentity).Error, qt.IsNil)
	grantor := openfga.NewUser(grantorIdentity, s.ofgaClient)

	_, _, controller, model, offer, _, _, _ := jimmtest.CreateTestControllerEnvironment(ctx, c, s.db)

	// The grantor administers the model and the offer, so the target-admin
	// check passes; only the structural-relation and grantee-kind guards
	// should reject the operations below.
	c.Assert(grantor.SetModelAccess(ctx, model.ResourceTag(), names.AdministratorRelation), qt.IsNil)
	c.Assert(grantor.SetApplicationOfferAccess(ctx, offer.ResourceTag(), names.AdministratorRelation), qt.IsNil)

	testCases := []struct {
		description string
		tuple       apiparams.RelationshipTuple
	}{
		{
			description: "model administrator cannot manage the model->controller structural relation",
			tuple: apiparams.RelationshipTuple{
				Object:       controller.ResourceTag().String(),
				Relation:     names.ControllerRelation.String(),
				TargetObject: model.ResourceTag().String(),
			},
		},
		{
			description: "offer administrator cannot manage the offer->model structural relation",
			tuple: apiparams.RelationshipTuple{
				Object:       model.ResourceTag().String(),
				Relation:     names.ModelRelation.String(),
				TargetObject: offer.ResourceTag().String(),
			},
		},
		{
			description: "model administrator cannot grant an access relation to a non-grantee object kind",
			tuple: apiparams.RelationshipTuple{
				Object:       controller.ResourceTag().String(),
				Relation:     names.ReaderRelation.String(),
				TargetObject: model.ResourceTag().String(),
			},
		},
	}

	for _, testCase := range testCases {
		c.Run(testCase.description, func(c *qt.C) {
			err := s.manager.AddRelation(ctx, grantor, []apiparams.RelationshipTuple{testCase.tuple})
			c.Assert(err, qt.ErrorMatches, "unauthorized")

			err = s.manager.RemoveRelation(ctx, grantor, []apiparams.RelationshipTuple{testCase.tuple})
			c.Assert(err, qt.ErrorMatches, "unauthorized")

			_, err = s.manager.CheckRelation(ctx, grantor, testCase.tuple, false)
			c.Assert(err, qt.ErrorMatches, "unauthorized")
		})
	}

	// A JIMM admin is still able to manage structural relations.
	err = s.manager.AddRelation(ctx, s.adminUser, []apiparams.RelationshipTuple{{
		Object:       controller.ResourceTag().String(),
		Relation:     names.ControllerRelation.String(),
		TargetObject: model.ResourceTag().String(),
	}})
	c.Assert(err, qt.IsNil)
}

func (s *permissionManagerSuite) TestRelationshipLogUserUpdated(c *qt.C) {
	c.Parallel()
	ctx := context.Background()

	adminId := "admin@canonical.com"

	u := openfga.NewUser(&dbmodel.Identity{Name: adminId}, s.ofgaClient)
	u.JimmAdmin = true

	user, _, _, model, _, _, _, _ := jimmtest.CreateTestControllerEnvironment(ctx, c, s.db)
	tuples := []apiparams.RelationshipTuple{
		{
			Object:       user.Tag().String(),
			Relation:     names.ReaderRelation.String(),
			TargetObject: model.ResourceTag().String(),
		},
	}

	core, logs := observer.New(zap.InfoLevel)
	ctx = zapctx.WithLogger(ctx, zap.New(core))

	err := s.manager.AddRelation(ctx, u, tuples)
	c.Assert(err, qt.IsNil)
	c.Assert(logs.Len(), qt.Equals, 1)
	c.Assert(logs.All()[0].Message, qt.Contains, fmt.Sprintf("user_updated:%s,%s,add,", adminId, user.Name))

	err = s.manager.RemoveRelation(ctx, u, tuples)
	c.Assert(err, qt.IsNil)
	c.Assert(logs.Len(), qt.Equals, 2)

	err = s.manager.AddRelation(ctx, u, tuples)
	c.Assert(err, qt.IsNil)
	c.Assert(logs.Len(), qt.Equals, 3)
}

func (s *permissionManagerSuite) TestAddRelationBatch(c *qt.C) {
	c.Parallel()

	u := openfga.NewUser(&dbmodel.Identity{Name: "admin@canonical.com"}, s.ofgaClient)
	u.JimmAdmin = true

	_, _, _, model, _, _, _, _ := jimmtest.CreateTestControllerEnvironment(c.Context(), c, s.db)
	tuples := []apiparams.RelationshipTuple{}

	expectedLength := permissions.BatchSizeOpenfga*2 + 1
	for range expectedLength {
		u, err := dbmodel.NewIdentity(petname.Generate(3, "-"+"canonical.com"))
		c.Assert(err, qt.IsNil)

		c.Assert(s.db.DB.Create(u).Error, qt.IsNil)
		tuples = append(tuples, apiparams.RelationshipTuple{
			Object:       u.Tag().String(),
			Relation:     names.ReaderRelation.String(),
			TargetObject: model.ResourceTag().String(),
		})
	}

	err := s.manager.AddRelation(c.Context(), u, tuples)
	c.Assert(err, qt.IsNil)

	tuplesListed, _, err := s.manager.ListRelationshipTuples(c.Context(), s.adminUser, apiparams.RelationshipTuple{
		Relation:     names.ReaderRelation.String(),
		TargetObject: model.ResourceTag().String(),
	}, 100, "")
	c.Assert(err, qt.IsNil)
	c.Assert(tuplesListed, qt.HasLen, 100)
}

func (s *permissionManagerSuite) TestRemoveRelationBatch(c *qt.C) {
	c.Parallel()

	u := openfga.NewUser(&dbmodel.Identity{Name: "admin@canonical.com"}, s.ofgaClient)
	u.JimmAdmin = true

	_, _, _, model, _, _, _, _ := jimmtest.CreateTestControllerEnvironment(c.Context(), c, s.db)
	tuples := []apiparams.RelationshipTuple{}

	for range permissions.BatchSizeOpenfga*2 + 1 {
		u, err := dbmodel.NewIdentity(petname.Generate(3, "-"+"canonical.com"))
		c.Assert(err, qt.IsNil)

		c.Assert(s.db.DB.Create(u).Error, qt.IsNil)
		tuples = append(tuples, apiparams.RelationshipTuple{
			Object:       u.Tag().String(),
			Relation:     names.ReaderRelation.String(),
			TargetObject: model.ResourceTag().String(),
		})
	}

	err := s.manager.AddRelation(c.Context(), u, tuples)
	c.Assert(err, qt.IsNil)

	err = s.manager.RemoveRelation(c.Context(), u, tuples)
	c.Assert(err, qt.IsNil)

	tuplesListed, _, err := s.manager.ListRelationshipTuples(c.Context(), s.adminUser, apiparams.RelationshipTuple{
		Relation:     names.ReaderRelation.String(),
		TargetObject: model.ResourceTag().String(),
	}, 100, "")
	c.Assert(err, qt.IsNil)
	c.Assert(tuplesListed, qt.HasLen, 0)
}
