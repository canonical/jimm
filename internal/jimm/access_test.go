// Copyright 2024 Canonical.

package jimm_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/canonical/ofga"
	petname "github.com/dustinkirkland/golang-petname"
	qt "github.com/frankban/quicktest"
	"github.com/google/uuid"
	"github.com/juju/names/v5"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/jimm"
	"github.com/canonical/jimm/v3/internal/openfga"
	ofganames "github.com/canonical/jimm/v3/internal/openfga/names"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
	jimmnames "github.com/canonical/jimm/v3/pkg/names"
)

func TestAuditLogAccess(t *testing.T) {
	c := qt.New(t)

	j := jimmtest.NewJIMM(c, nil)

	ctx := context.Background()

	i, err := dbmodel.NewIdentity("alice")
	c.Assert(err, qt.IsNil)
	adminUser := openfga.NewUser(i, j.OpenFGAClient)
	err = adminUser.SetControllerAccess(ctx, j.ResourceTag(), ofganames.AdministratorRelation)
	c.Assert(err, qt.IsNil)

	i2, err := dbmodel.NewIdentity("bob")
	c.Assert(err, qt.IsNil)
	user := openfga.NewUser(i2, j.OpenFGAClient)

	// admin user can grant other users audit log access.
	err = j.GrantAuditLogAccess(ctx, adminUser, user.ResourceTag())
	c.Assert(err, qt.IsNil)

	access := user.GetAuditLogViewerAccess(ctx, j.ResourceTag())
	c.Assert(access, qt.Equals, ofganames.AuditLogViewerRelation)

	// re-granting access does not result in error.
	err = j.GrantAuditLogAccess(ctx, adminUser, user.ResourceTag())
	c.Assert(err, qt.IsNil)

	// admin user can revoke other users audit log access.
	err = j.RevokeAuditLogAccess(ctx, adminUser, user.ResourceTag())
	c.Assert(err, qt.IsNil)

	access = user.GetAuditLogViewerAccess(ctx, j.ResourceTag())
	c.Assert(access, qt.Equals, ofganames.NoRelation)

	// re-revoking access does not result in error.
	err = j.RevokeAuditLogAccess(ctx, adminUser, user.ResourceTag())
	c.Assert(err, qt.IsNil)

	// non-admin user cannot grant audit log access
	err = j.GrantAuditLogAccess(ctx, user, adminUser.ResourceTag())
	c.Assert(err, qt.ErrorMatches, "unauthorized")

	// non-admin user cannot revoke audit log access
	err = j.RevokeAuditLogAccess(ctx, user, adminUser.ResourceTag())
	c.Assert(err, qt.ErrorMatches, "unauthorized")
}

func TestParseAndValidateTag(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	j := jimmtest.NewJIMM(c, nil)

	user, _, _, model, _, _, _, _ := jimmtest.CreateTestControllerEnvironment(ctx, c, j.Database)

	jimmTag := "model-" + user.Name + "/" + model.Name + "#administrator"

	// JIMM tag syntax for models
	tag, err := j.ParseAndValidateTag(ctx, jimmTag)
	c.Assert(err, qt.IsNil)
	c.Assert(tag.Kind.String(), qt.Equals, names.ModelTagKind)
	c.Assert(tag.ID, qt.Equals, model.UUID.String)
	c.Assert(tag.Relation.String(), qt.Equals, "administrator")

	jujuTag := "model-" + model.UUID.String + "#administrator"

	// Juju tag syntax for models
	tag, err = j.ParseAndValidateTag(ctx, jujuTag)
	c.Assert(err, qt.IsNil)
	c.Assert(tag.ID, qt.Equals, model.UUID.String)
	c.Assert(tag.Kind.String(), qt.Equals, names.ModelTagKind)
	c.Assert(tag.Relation.String(), qt.Equals, "administrator")

	// JIMM tag only kind
	kindTag := "model"
	tag, err = j.ParseAndValidateTag(ctx, kindTag)
	c.Assert(err, qt.IsNil)
	c.Assert(tag.ID, qt.Equals, "")
	c.Assert(tag.Kind.String(), qt.Equals, names.ModelTagKind)

	// JIMM tag not valid
	_, err = j.ParseAndValidateTag(ctx, "")
	c.Assert(err, qt.ErrorMatches, "unknown tag kind")
}

func TestResolveTags(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	j := jimmtest.NewJIMM(c, nil)

	identity, group, controller, model, offer, cloud, _, role := jimmtest.CreateTestControllerEnvironment(ctx, c, j.Database)

	testCases := []struct {
		desc     string
		input    string
		expected *ofga.Entity
	}{{
		desc:     "map identity name with relation",
		input:    "user-" + identity.Name + "#member",
		expected: ofganames.ConvertTagWithRelation(names.NewUserTag(identity.Name), ofganames.MemberRelation),
	}, {
		desc:     "map group name with relation",
		input:    "group-" + group.Name + "#member",
		expected: ofganames.ConvertTagWithRelation(jimmnames.NewGroupTag(group.UUID), ofganames.MemberRelation),
	}, {
		desc:     "map group UUID",
		input:    "group-" + group.UUID,
		expected: ofganames.ConvertTag(jimmnames.NewGroupTag(group.UUID)),
	}, {
		desc:     "map group UUID with relation",
		input:    "group-" + group.UUID + "#member",
		expected: ofganames.ConvertTagWithRelation(jimmnames.NewGroupTag(group.UUID), ofganames.MemberRelation),
	}, {
		desc:     "map role UUID",
		input:    "role-" + role.UUID,
		expected: ofganames.ConvertTag(jimmnames.NewRoleTag(role.UUID)),
	}, {
		desc:     "map role UUID with relation",
		input:    "role-" + role.UUID + "#assignee",
		expected: ofganames.ConvertTagWithRelation(jimmnames.NewRoleTag(role.UUID), ofganames.AssigneeRelation),
	}, {
		desc:     "map jimm controller",
		input:    "controller-" + "jimm",
		expected: ofganames.ConvertTag(names.NewControllerTag(j.UUID)),
	}, {
		desc:     "map controller",
		input:    "controller-" + controller.Name + "#administrator",
		expected: ofganames.ConvertTagWithRelation(names.NewControllerTag(model.UUID.String), ofganames.AdministratorRelation),
	}, {
		desc:     "map controller UUID",
		input:    "controller-" + controller.UUID,
		expected: ofganames.ConvertTag(names.NewControllerTag(model.UUID.String)),
	}, {
		desc:     "map model",
		input:    "model-" + model.OwnerIdentityName + "/" + model.Name + "#administrator",
		expected: ofganames.ConvertTagWithRelation(names.NewModelTag(model.UUID.String), ofganames.AdministratorRelation),
	}, {
		desc:     "map model UUID",
		input:    "model-" + model.UUID.String,
		expected: ofganames.ConvertTag(names.NewModelTag(model.UUID.String)),
	}, {
		desc:     "map offer",
		input:    "applicationoffer-" + offer.URL + "#administrator",
		expected: ofganames.ConvertTagWithRelation(names.NewApplicationOfferTag(offer.UUID), ofganames.AdministratorRelation),
	}, {
		desc:     "map offer UUID",
		input:    "applicationoffer-" + offer.UUID,
		expected: ofganames.ConvertTag(names.NewApplicationOfferTag(offer.UUID)),
	}, {
		desc:     "map cloud",
		input:    "cloud-" + cloud.Name + "#administrator",
		expected: ofganames.ConvertTagWithRelation(names.NewCloudTag(cloud.Name), ofganames.AdministratorRelation),
	}}

	for _, tC := range testCases {
		t.Run(tC.desc, func(t *testing.T) {
			jujuTag, err := jimm.ResolveTag(j.UUID, j.Database, tC.input)
			c.Assert(err, qt.IsNil)
			c.Assert(jujuTag, qt.DeepEquals, tC.expected)
		})
	}
}

func TestResolveTupleObjectHandlesErrors(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	j := jimmtest.NewJIMM(c, nil)

	_, _, controller, model, offer, _, _, _ := jimmtest.CreateTestControllerEnvironment(ctx, c, j.Database)

	type test struct {
		input string
		want  string
	}

	tests := []test{
		// Resolves bad tuple objects in general
		{
			input: "unknowntag-blabla",
			want:  "failed to map tag, unknown kind: unknowntag",
		},
		// Resolves bad groups where they do not exist
		{
			input: "group-myspecialpokemon-his-name-is-youguessedit-diglett",
			want:  "group myspecialpokemon-his-name-is-youguessedit-diglett not found",
		},
		// Resolves bad controllers where they do not exist
		{
			input: "controller-mycontroller-that-does-not-exist",
			want:  "controller not found",
		},
		// Resolves bad models where the user cannot be obtained from the JIMM tag
		{
			input: "model-mycontroller-that-does-not-exist/mymodel",
			want:  "model not found",
		},
		// Resolves bad models where it cannot be found on the specified controller
		{
			input: "model-" + controller.Name + ":alex/",
			want:  "model name format incorrect, expected <model-owner>/<model-name>",
		},
		// Resolves bad applicationoffers where it cannot be found on the specified controller/model combo
		{
			input: "applicationoffer-" + controller.Name + ":alex/" + model.Name + "." + offer.UUID + "fluff",
			want:  "application offer not found",
		},
		{
			input: "abc",
			want:  "failed to setup tag resolver: tag is not properly formatted",
		},
		{
			input: "model-test-unknowncontroller-1:alice@canonical.com/test-model-1",
			want:  "model not found",
		},
	}
	for i, tc := range tests {
		t.Run(fmt.Sprintf("test %d", i), func(t *testing.T) {
			_, err := jimm.ResolveTag(j.UUID, j.Database, tc.input)
			c.Assert(err, qt.ErrorMatches, tc.want)
		})
	}
}

func TestToJAASTag(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	j := jimmtest.NewJIMM(c, nil)

	user, group, controller, model, applicationOffer, cloud, _, role := jimmtest.CreateTestControllerEnvironment(ctx, c, j.Database)

	serviceAccountId := petname.Generate(2, "-") + "@serviceaccount"

	tests := []struct {
		tag             *ofganames.Tag
		expectedJAASTag string
		expectedError   string
	}{{
		tag:             ofganames.ConvertTag(user.ResourceTag()),
		expectedJAASTag: "user-" + user.Name,
	}, {
		tag:             ofganames.ConvertTag(jimmnames.NewServiceAccountTag(serviceAccountId)),
		expectedJAASTag: "serviceaccount-" + serviceAccountId,
	}, {
		tag:             ofganames.ConvertTag(group.ResourceTag()),
		expectedJAASTag: "group-" + group.Name,
	}, {
		tag:             ofganames.ConvertTag(controller.ResourceTag()),
		expectedJAASTag: "controller-" + controller.Name,
	}, {
		tag:             ofganames.ConvertTag(model.ResourceTag()),
		expectedJAASTag: "model-" + user.Name + "/" + model.Name,
	}, {
		tag:             ofganames.ConvertTag(applicationOffer.ResourceTag()),
		expectedJAASTag: "applicationoffer-" + applicationOffer.URL,
	}, {
		tag:           &ofganames.Tag{},
		expectedError: "unexpected tag kind: ",
	}, {
		tag:             ofganames.ConvertTag(cloud.ResourceTag()),
		expectedJAASTag: "cloud-" + cloud.Name,
	}, {
		tag:             ofganames.ConvertTag(role.ResourceTag()),
		expectedJAASTag: "role-" + role.Name,
	}}
	for _, test := range tests {
		t, err := j.ToJAASTag(ctx, test.tag, true)
		if test.expectedError != "" {
			c.Assert(err, qt.ErrorMatches, test.expectedError)
		} else {
			c.Assert(err, qt.IsNil)
			c.Assert(t, qt.Equals, test.expectedJAASTag)
		}
	}
}

func TestToJAASTagNoUUIDResolution(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	j := jimmtest.NewJIMM(c, nil)

	user, group, controller, model, applicationOffer, cloud, _, role := jimmtest.CreateTestControllerEnvironment(ctx, c, j.Database)
	serviceAccountId := petname.Generate(2, "-") + "@serviceaccount"

	tests := []struct {
		tag             *ofganames.Tag
		expectedJAASTag string
		expectedError   string
	}{{
		tag:             ofganames.ConvertTag(user.ResourceTag()),
		expectedJAASTag: "user-" + user.Name,
	}, {
		tag:             ofganames.ConvertTag(jimmnames.NewServiceAccountTag(serviceAccountId)),
		expectedJAASTag: "serviceaccount-" + serviceAccountId,
	}, {
		tag:             ofganames.ConvertTag(group.ResourceTag()),
		expectedJAASTag: "group-" + group.UUID,
	}, {
		tag:             ofganames.ConvertTag(controller.ResourceTag()),
		expectedJAASTag: "controller-" + controller.UUID,
	}, {
		tag:             ofganames.ConvertTag(model.ResourceTag()),
		expectedJAASTag: "model-" + model.UUID.String,
	}, {
		tag:             ofganames.ConvertTag(applicationOffer.ResourceTag()),
		expectedJAASTag: "applicationoffer-" + applicationOffer.UUID,
	}, {
		tag:             ofganames.ConvertTag(cloud.ResourceTag()),
		expectedJAASTag: "cloud-" + cloud.Name,
	}, {
		tag:             ofganames.ConvertTag(role.ResourceTag()),
		expectedJAASTag: "role-" + role.UUID,
	}, {
		tag:             &ofganames.Tag{},
		expectedJAASTag: "-",
	}}
	for _, test := range tests {
		t, err := j.ToJAASTag(ctx, test.tag, false)
		if test.expectedError != "" {
			c.Assert(err, qt.ErrorMatches, test.expectedError)
		} else {
			c.Assert(err, qt.IsNil)
			c.Assert(t, qt.Equals, test.expectedJAASTag)
		}
	}
}

func TestOpenFGACleanup(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	j := jimmtest.NewJIMM(c, nil)

	// run cleanup on an empty authorizaton store
	err := j.OpenFGACleanup(ctx)
	c.Assert(err, qt.IsNil)

	type createTagFunction func(int) *ofga.Entity

	var (
		createStringTag = func(kind openfga.Kind) createTagFunction {
			return func(i int) *ofga.Entity {
				return &ofga.Entity{
					Kind: kind,
					ID:   fmt.Sprintf("%s-%d", petname.Generate(2, "-"), i),
				}
			}
		}

		createUUIDTag = func(kind openfga.Kind) createTagFunction {
			return func(i int) *ofga.Entity {
				return &ofga.Entity{
					Kind: kind,
					ID:   uuid.NewString(),
				}
			}
		}
	)

	tagTests := []struct {
		createObjectTag createTagFunction
		relation        string
		createTargetTag createTagFunction
	}{{
		createObjectTag: createStringTag(openfga.UserType),
		relation:        "member",
		createTargetTag: createStringTag(openfga.GroupType),
	}, {
		createObjectTag: createStringTag(openfga.UserType),
		relation:        "administrator",
		createTargetTag: createUUIDTag(openfga.ControllerType),
	}, {
		createObjectTag: createStringTag(openfga.UserType),
		relation:        "reader",
		createTargetTag: createUUIDTag(openfga.ModelType),
	}, {
		createObjectTag: createStringTag(openfga.UserType),
		relation:        "administrator",
		createTargetTag: createStringTag(openfga.CloudType),
	}, {
		createObjectTag: createStringTag(openfga.UserType),
		relation:        "consumer",
		createTargetTag: createUUIDTag(openfga.ApplicationOfferType),
	}}

	orphanedTuples := []ofga.Tuple{}
	for i := 0; i < 100; i++ {
		for _, test := range tagTests {
			objectTag := test.createObjectTag(i)
			targetTag := test.createTargetTag(i)

			tuple := openfga.Tuple{
				Object:   objectTag,
				Relation: ofga.Relation(test.relation),
				Target:   targetTag,
			}
			err = j.OpenFGAClient.AddRelation(ctx, tuple)
			c.Assert(err, qt.IsNil)

			orphanedTuples = append(orphanedTuples, tuple)
		}
	}

	err = j.OpenFGACleanup(ctx)
	c.Assert(err, qt.IsNil)

	for _, tuple := range orphanedTuples {
		c.Logf("checking relation for %+v", tuple)
		ok, err := j.OpenFGAClient.CheckRelation(ctx, tuple, false)
		c.Assert(err, qt.IsNil)
		c.Assert(ok, qt.IsFalse)
	}
}
