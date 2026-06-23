// Copyright 2025 Canonical.

package openfga_test

import (
	"sort"
	"testing"

	qt "github.com/frankban/quicktest"
	"github.com/google/uuid"
	"github.com/juju/names/v5"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/openfga"
	ofganames "github.com/canonical/jimm/v3/internal/openfga/names"
	jimmnames "github.com/canonical/jimm/v3/pkg/names"
)

func TestIsAdministrator(t *testing.T) {
	c := qt.New(t)
	s := SetupTest(c)
	ctx := c.Context()

	groupUUID := uuid.NewString()
	controllerUUID, _ := uuid.NewRandom()
	controller := names.NewControllerTag(controllerUUID.String())

	user := names.NewUserTag("eve")
	userToGroup := openfga.Tuple{
		Object:   ofganames.ConvertTag(user),
		Relation: "member",
		Target:   ofganames.ConvertTag(jimmnames.NewGroupTag(groupUUID)),
	}
	groupToController := openfga.Tuple{
		Object:   ofganames.ConvertTagWithRelation(jimmnames.NewGroupTag(groupUUID), ofganames.MemberRelation),
		Relation: "administrator",
		Target:   ofganames.ConvertTag(controller),
	}

	err := s.ofgaClient.AddRelation(ctx, userToGroup, groupToController)
	c.Assert(err, qt.IsNil)

	uIdentity, err := dbmodel.NewIdentity(user.Id())
	c.Assert(err, qt.IsNil)
	u := openfga.NewUser(
		uIdentity,
		s.ofgaClient,
	)

	allowed, err := openfga.IsAdministrator(ctx, u, controller)
	c.Assert(err, qt.IsNil)
	c.Assert(allowed, qt.Equals, true)
}

func TestUserContextualTuples(c *testing.T) {
	assert := qt.New(c)
	identity, err := dbmodel.NewIdentity("eve")
	assert.Assert(err, qt.IsNil)

	user := openfga.NewUser(identity, nil)
	groups := []string{"engineering", "platform"}
	user.SetIDPGroups(groups)
	groups[0] = "mutated"

	tuples, err := user.ContextualTuples()
	assert.Assert(err, qt.IsNil)
	assert.Assert(tuples, qt.DeepEquals, []openfga.Tuple{{
		Object:   ofganames.ConvertTag(names.NewUserTag("eve")),
		Relation: ofganames.MemberRelation,
		Target:   ofganames.ConvertTag(jimmnames.NewIdPGroupTag("engineering")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("eve")),
		Relation: ofganames.MemberRelation,
		Target:   ofganames.ConvertTag(jimmnames.NewIdPGroupTag("platform")),
	}})
}

func TestUserContextualTuplesRejectsTooManyGroups(c *testing.T) {
	assert := qt.New(c)
	identity, err := dbmodel.NewIdentity("eve")
	assert.Assert(err, qt.IsNil)

	user := openfga.NewUser(identity, nil)
	user.SetIDPGroups([]string{
		"group-01", "group-02", "group-03", "group-04", "group-05",
		"group-06", "group-07", "group-08", "group-09", "group-10",
		"group-11", "group-12", "group-13", "group-14", "group-15",
		"group-16", "group-17", "group-18", "group-19", "group-20",
		"group-21",
	})

	tuples, err := user.ContextualTuples()
	assert.Assert(tuples, qt.IsNil)
	assert.Assert(err, qt.ErrorMatches, "too many IDP groups in session: got 21, maximum is 20")
}

func TestUserContextualTuplesRejectsInvalidGroup(c *testing.T) {
	assert := qt.New(c)
	identity, err := dbmodel.NewIdentity("eve")
	assert.Assert(err, qt.IsNil)

	user := openfga.NewUser(identity, nil)
	user.SetIDPGroups([]string{""})

	tuples, err := user.ContextualTuples()
	assert.Assert(tuples, qt.IsNil)
	assert.Assert(err, qt.ErrorMatches, `invalid IDP group identifier in session: ""`)
}

func TestModelAccess(t *testing.T) {
	c := qt.New(t)
	s := SetupTest(c)
	ctx := c.Context()

	groupUUID := uuid.NewString()
	group := jimmnames.NewGroupTag(groupUUID)

	controllerUUID, err := uuid.NewRandom()
	c.Assert(err, qt.IsNil)
	controller := names.NewControllerTag(controllerUUID.String())

	modelUUID, err := uuid.NewRandom()
	c.Assert(err, qt.IsNil)
	model := names.NewModelTag(modelUUID.String())

	eve := names.NewUserTag("eve")
	alice := names.NewUserTag("alice")

	tuples := []openfga.Tuple{{
		Object:   ofganames.ConvertTag(eve),
		Relation: ofganames.MemberRelation,
		Target:   ofganames.ConvertTag(jimmnames.NewGroupTag(groupUUID)),
	}, {
		Object:   ofganames.ConvertTagWithRelation(group, ofganames.MemberRelation),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(controller),
	}, {
		Object:   ofganames.ConvertTag(controller),
		Relation: ofganames.ControllerRelation,
		Target:   ofganames.ConvertTag(model),
	}, {
		Object:   ofganames.ConvertTag(alice),
		Relation: ofganames.WriterRelation,
		Target:   ofganames.ConvertTag(model),
	}}
	err = s.ofgaClient.AddRelation(ctx, tuples...)
	c.Assert(err, qt.IsNil)

	adamIdentity, err := dbmodel.NewIdentity("adam")
	c.Assert(err, qt.IsNil)
	eveIdentity, err := dbmodel.NewIdentity(eve.Id())
	c.Assert(err, qt.IsNil)
	aliceIdentity, err := dbmodel.NewIdentity(alice.Id())
	c.Assert(err, qt.IsNil)

	adamUser := openfga.NewUser(adamIdentity, s.ofgaClient)
	eveUser := openfga.NewUser(eveIdentity, s.ofgaClient)
	aliceUser := openfga.NewUser(aliceIdentity, s.ofgaClient)

	relation := eveUser.GetModelAccess(ctx, model)
	c.Assert(relation, qt.DeepEquals, ofganames.AdministratorRelation)

	relation = aliceUser.GetModelAccess(ctx, model)
	c.Assert(relation, qt.DeepEquals, ofganames.WriterRelation)

	relation = adamUser.GetModelAccess(ctx, model)
	c.Assert(relation, qt.DeepEquals, ofganames.NoRelation)

	allowed, err := eveUser.IsModelReader(ctx, model)
	c.Assert(err, qt.IsNil)
	c.Assert(allowed, qt.Equals, true)

	allowed, err = eveUser.IsModelWriter(ctx, model)
	c.Assert(err, qt.IsNil)
	c.Assert(allowed, qt.Equals, true)

	allowed, err = adamUser.IsModelWriter(ctx, model)
	c.Assert(err, qt.IsNil)
	c.Assert(allowed, qt.Equals, false)

	allowed, err = eveUser.IsModelAdmin(ctx, model)
	c.Assert(err, qt.IsNil)
	c.Assert(allowed, qt.Equals, true)

	allowed, err = adamUser.IsModelAdmin(ctx, model)
	c.Assert(err, qt.IsNil)
	c.Assert(allowed, qt.Equals, false)

	allowed, err = eveUser.HasModelRelation(ctx, model, ofganames.ReaderRelation)
	c.Assert(err, qt.IsNil)
	c.Assert(allowed, qt.Equals, true)

	allowed, err = eveUser.HasModelRelation(ctx, model, ofganames.AdministratorRelation)
	c.Assert(err, qt.IsNil)
	c.Assert(allowed, qt.Equals, true)

	allowed, err = adamUser.HasModelRelation(ctx, model, ofganames.ReaderRelation)
	c.Assert(err, qt.IsNil)
	c.Assert(allowed, qt.Equals, false)
}

func TestSetModelAccess(t *testing.T) {
	c := qt.New(t)
	s := SetupTest(c)
	ctx := c.Context()
	modelUUID, err := uuid.NewRandom()
	c.Assert(err, qt.IsNil)
	model := names.NewModelTag(modelUUID.String())

	eve := names.NewUserTag("eve")
	alice := names.NewUserTag("alice")

	adamIdentity, err := dbmodel.NewIdentity("adam")
	c.Assert(err, qt.IsNil)
	eveIdentity, err := dbmodel.NewIdentity(eve.Id())
	c.Assert(err, qt.IsNil)
	aliceIdentity, err := dbmodel.NewIdentity(alice.Id())
	c.Assert(err, qt.IsNil)

	adamUser := openfga.NewUser(adamIdentity, s.ofgaClient)
	eveUser := openfga.NewUser(eveIdentity, s.ofgaClient)
	aliceUser := openfga.NewUser(aliceIdentity, s.ofgaClient)

	err = eveUser.SetModelAccess(ctx, model, ofganames.AdministratorRelation)
	c.Assert(err, qt.IsNil)

	err = eveUser.SetModelAccess(ctx, model, ofganames.AdministratorRelation)
	c.Assert(err, qt.IsNil)

	err = aliceUser.SetModelAccess(ctx, model, ofganames.WriterRelation)
	c.Assert(err, qt.IsNil)

	relation := eveUser.GetModelAccess(ctx, model)
	c.Assert(relation, qt.DeepEquals, ofganames.AdministratorRelation)

	relation = aliceUser.GetModelAccess(ctx, model)
	c.Assert(relation, qt.DeepEquals, ofganames.WriterRelation)

	relation = adamUser.GetModelAccess(ctx, model)
	c.Assert(relation, qt.DeepEquals, ofganames.NoRelation)
}

func TestCloudAccess(t *testing.T) {
	c := qt.New(t)
	s := SetupTest(c)
	ctx := c.Context()

	groupUUID := uuid.NewString()
	group := jimmnames.NewGroupTag(groupUUID)

	controllerUUID, err := uuid.NewRandom()
	c.Assert(err, qt.IsNil)
	controller := names.NewControllerTag(controllerUUID.String())

	cloudUUID, err := uuid.NewRandom()
	c.Assert(err, qt.IsNil)
	cloud := names.NewCloudTag(cloudUUID.String())

	eve := names.NewUserTag("eve")
	alice := names.NewUserTag("alice")

	tuples := []openfga.Tuple{{
		Object:   ofganames.ConvertTag(eve),
		Relation: ofganames.MemberRelation,
		Target:   ofganames.ConvertTag(jimmnames.NewGroupTag(groupUUID)),
	}, {
		Object:   ofganames.ConvertTagWithRelation(group, ofganames.MemberRelation),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(controller),
	}, {
		Object:   ofganames.ConvertTag(controller),
		Relation: ofganames.ControllerRelation,
		Target:   ofganames.ConvertTag(cloud),
	}, {
		Object:   ofganames.ConvertTag(alice),
		Relation: ofganames.CanAddModelRelation,
		Target:   ofganames.ConvertTag(cloud),
	}}
	err = s.ofgaClient.AddRelation(ctx, tuples...)
	c.Assert(err, qt.IsNil)
	i, err := dbmodel.NewIdentity("adam")
	c.Assert(err, qt.IsNil)
	eveIdentity, err := dbmodel.NewIdentity(eve.Id())
	c.Assert(err, qt.IsNil)
	aliceIdentity, err := dbmodel.NewIdentity(alice.Id())
	c.Assert(err, qt.IsNil)

	adamUser := openfga.NewUser(i, s.ofgaClient)
	eveUser := openfga.NewUser(eveIdentity, s.ofgaClient)
	aliceUser := openfga.NewUser(aliceIdentity, s.ofgaClient)

	relation := eveUser.GetCloudAccess(ctx, cloud)
	c.Assert(relation, qt.DeepEquals, ofganames.AdministratorRelation)

	relation = aliceUser.GetCloudAccess(ctx, cloud)
	c.Assert(relation, qt.DeepEquals, ofganames.CanAddModelRelation)

	relation = adamUser.GetCloudAccess(ctx, cloud)
	c.Assert(relation, qt.DeepEquals, ofganames.NoRelation)
}

func TestSetCloudAccess(t *testing.T) {
	c := qt.New(t)
	s := SetupTest(c)
	ctx := c.Context()
	cloudUUID, err := uuid.NewRandom()
	c.Assert(err, qt.IsNil)
	cloud := names.NewCloudTag(cloudUUID.String())

	eve := names.NewUserTag("eve")
	alice := names.NewUserTag("alice")

	adamIdentity, err := dbmodel.NewIdentity("adam")
	c.Assert(err, qt.IsNil)
	eveIdentity, err := dbmodel.NewIdentity(eve.Id())
	c.Assert(err, qt.IsNil)
	aliceIdentity, err := dbmodel.NewIdentity(alice.Id())
	c.Assert(err, qt.IsNil)

	adamUser := openfga.NewUser(adamIdentity, s.ofgaClient)
	eveUser := openfga.NewUser(eveIdentity, s.ofgaClient)
	aliceUser := openfga.NewUser(aliceIdentity, s.ofgaClient)

	err = eveUser.SetCloudAccess(ctx, cloud, ofganames.AdministratorRelation)
	c.Assert(err, qt.IsNil)

	// re-setting an existing relation should be fine
	err = eveUser.SetCloudAccess(ctx, cloud, ofganames.AdministratorRelation)
	c.Assert(err, qt.IsNil)

	err = aliceUser.SetCloudAccess(ctx, cloud, ofganames.CanAddModelRelation)
	c.Assert(err, qt.IsNil)

	relation := eveUser.GetCloudAccess(ctx, cloud)
	c.Assert(relation, qt.DeepEquals, ofganames.AdministratorRelation)

	relation = aliceUser.GetCloudAccess(ctx, cloud)
	c.Assert(relation, qt.DeepEquals, ofganames.CanAddModelRelation)

	relation = adamUser.GetCloudAccess(ctx, cloud)
	c.Assert(relation, qt.DeepEquals, ofganames.NoRelation)
}

func TestControllerAccess(t *testing.T) {
	c := qt.New(t)
	s := SetupTest(c)
	ctx := c.Context()

	groupUUID := uuid.NewString()
	group := jimmnames.NewGroupTag(groupUUID)

	controllerUUID, err := uuid.NewRandom()
	c.Assert(err, qt.IsNil)
	controller := names.NewControllerTag(controllerUUID.String())

	eve := names.NewUserTag("eve")
	alice := names.NewUserTag("alice")

	tuples := []openfga.Tuple{{
		Object:   ofganames.ConvertTag(eve),
		Relation: ofganames.MemberRelation,
		Target:   ofganames.ConvertTag(jimmnames.NewGroupTag(groupUUID)),
	}, {
		Object:   ofganames.ConvertTagWithRelation(group, ofganames.MemberRelation),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(controller),
	}, {
		Object:   ofganames.ConvertTag(alice),
		Relation: ofganames.AuditLogViewerRelation,
		Target:   ofganames.ConvertTag(controller),
	}}
	err = s.ofgaClient.AddRelation(ctx, tuples...)
	c.Assert(err, qt.IsNil)

	adamIdentity, err := dbmodel.NewIdentity("adam")
	c.Assert(err, qt.IsNil)
	eveIdentity, err := dbmodel.NewIdentity(eve.Id())
	c.Assert(err, qt.IsNil)
	aliceIdentity, err := dbmodel.NewIdentity(alice.Id())
	c.Assert(err, qt.IsNil)

	adamUser := openfga.NewUser(adamIdentity, s.ofgaClient)
	eveUser := openfga.NewUser(eveIdentity, s.ofgaClient)
	aliceUser := openfga.NewUser(aliceIdentity, s.ofgaClient)

	relation := eveUser.GetControllerAccess(ctx, controller)
	c.Assert(relation, qt.DeepEquals, ofganames.AdministratorRelation)

	relation = aliceUser.GetControllerAccess(ctx, controller)
	c.Assert(relation, qt.DeepEquals, ofganames.NoRelation)

	relation = aliceUser.GetAuditLogViewerAccess(ctx, controller)
	c.Assert(relation, qt.DeepEquals, ofganames.AuditLogViewerRelation)

	relation = adamUser.GetControllerAccess(ctx, controller)
	c.Assert(relation, qt.DeepEquals, ofganames.NoRelation)

	relation = adamUser.GetAuditLogViewerAccess(ctx, controller)
	c.Assert(relation, qt.DeepEquals, ofganames.NoRelation)
}

func TestCanAddModelToController(t *testing.T) {
	c := qt.New(t)
	s := SetupTest(c)
	ctx := c.Context()

	groupUUID := uuid.NewString()
	group := jimmnames.NewGroupTag(groupUUID)

	controllerUUID, err := uuid.NewRandom()
	c.Assert(err, qt.IsNil)
	controller := names.NewControllerTag(controllerUUID.String())

	// eve has addmodel access via group membership
	eve := names.NewUserTag("eve")
	// alice has addmodel access set directly
	alice := names.NewUserTag("alice")
	// mike does not have access
	mike := names.NewUserTag("mike")

	tuples := []openfga.Tuple{{
		Object:   ofganames.ConvertTag(eve),
		Relation: ofganames.MemberRelation,
		Target:   ofganames.ConvertTag(jimmnames.NewGroupTag(groupUUID)),
	}, {
		Object:   ofganames.ConvertTagWithRelation(group, ofganames.MemberRelation),
		Relation: ofganames.CanAddModelRelation,
		Target:   ofganames.ConvertTag(controller),
	}, {
		Object:   ofganames.ConvertTag(alice),
		Relation: ofganames.CanAddModelRelation,
		Target:   ofganames.ConvertTag(controller),
	}}
	err = s.ofgaClient.AddRelation(ctx, tuples...)
	c.Assert(err, qt.IsNil)

	eveIdentity, err := dbmodel.NewIdentity(eve.Id())
	c.Assert(err, qt.IsNil)
	aliceIdentity, err := dbmodel.NewIdentity(alice.Id())
	c.Assert(err, qt.IsNil)
	mikeIdentity, err := dbmodel.NewIdentity(mike.Id())
	c.Assert(err, qt.IsNil)

	eveUser := openfga.NewUser(eveIdentity, s.ofgaClient)
	aliceUser := openfga.NewUser(aliceIdentity, s.ofgaClient)
	mikeUser := openfga.NewUser(mikeIdentity, s.ofgaClient)

	canAddModel, err := eveUser.IsAllowedAddModelToController(ctx, controller)
	c.Assert(err, qt.IsNil)
	c.Assert(canAddModel, qt.Equals, true)

	canAddModel, err = aliceUser.IsAllowedAddModelToController(ctx, controller)
	c.Assert(err, qt.IsNil)
	c.Assert(canAddModel, qt.Equals, true)

	canAddModel, err = mikeUser.IsAllowedAddModelToController(ctx, controller)
	c.Assert(err, qt.IsNil)
	c.Assert(canAddModel, qt.Equals, false)
}

func TestSetControllerAccess(t *testing.T) {
	c := qt.New(t)
	s := SetupTest(c)
	ctx := c.Context()
	controllerUUID, err := uuid.NewRandom()
	c.Assert(err, qt.IsNil)
	controller := names.NewControllerTag(controllerUUID.String())

	eve := names.NewUserTag("eve")
	alice := names.NewUserTag("alice")

	adamIdentity, err := dbmodel.NewIdentity("adam")
	c.Assert(err, qt.IsNil)
	eveIdentity, err := dbmodel.NewIdentity(eve.Id())
	c.Assert(err, qt.IsNil)
	aliceIdentity, err := dbmodel.NewIdentity(alice.Id())
	c.Assert(err, qt.IsNil)

	adamUser := openfga.NewUser(adamIdentity, s.ofgaClient)
	eveUser := openfga.NewUser(eveIdentity, s.ofgaClient)
	aliceUser := openfga.NewUser(aliceIdentity, s.ofgaClient)

	err = eveUser.SetControllerAccess(ctx, controller, ofganames.AdministratorRelation)
	c.Assert(err, qt.IsNil)

	// re-setting an existing relation should be fine
	err = eveUser.SetControllerAccess(ctx, controller, ofganames.AdministratorRelation)
	c.Assert(err, qt.IsNil)

	err = aliceUser.SetControllerAccess(ctx, controller, ofganames.AuditLogViewerRelation)
	c.Assert(err, qt.IsNil)

	relation := eveUser.GetControllerAccess(ctx, controller)
	c.Assert(relation, qt.DeepEquals, ofganames.AdministratorRelation)

	relation = aliceUser.GetControllerAccess(ctx, controller)
	c.Assert(relation, qt.DeepEquals, ofganames.NoRelation)

	relation = aliceUser.GetAuditLogViewerAccess(ctx, controller)
	c.Assert(relation, qt.DeepEquals, ofganames.AuditLogViewerRelation)

	relation = adamUser.GetControllerAccess(ctx, controller)
	c.Assert(relation, qt.DeepEquals, ofganames.NoRelation)

	relation = adamUser.GetAuditLogViewerAccess(ctx, controller)
	c.Assert(relation, qt.DeepEquals, ofganames.NoRelation)
}

func TestUnsetAuditLogViewerAccess(t *testing.T) {
	c := qt.New(t)
	s := SetupTest(c)
	ctx := c.Context()

	controllerUUID, err := uuid.NewRandom()
	c.Assert(err, qt.IsNil)
	controller := names.NewControllerTag(controllerUUID.String())

	aliceIdentity, err := dbmodel.NewIdentity("alice")
	c.Assert(err, qt.IsNil)

	aliceUser := openfga.NewUser(aliceIdentity, s.ofgaClient)

	tuples := []openfga.Tuple{{
		Object:   ofganames.ConvertTag(aliceUser.ResourceTag()),
		Relation: ofganames.AuditLogViewerRelation,
		Target:   ofganames.ConvertTag(controller),
	}}
	err = s.ofgaClient.AddRelation(ctx, tuples...)
	c.Assert(err, qt.IsNil)

	relation := aliceUser.GetAuditLogViewerAccess(ctx, controller)
	c.Assert(relation, qt.DeepEquals, ofganames.AuditLogViewerRelation)

	// Un-setting audit log viewer relation
	err = aliceUser.UnsetAuditLogViewerAccess(ctx, controller)
	c.Assert(err, qt.IsNil)

	relation = aliceUser.GetAuditLogViewerAccess(ctx, controller)
	c.Assert(relation, qt.DeepEquals, ofganames.NoRelation)

	// Un-setting again should be fine
	err = aliceUser.UnsetAuditLogViewerAccess(ctx, controller)
	c.Assert(err, qt.IsNil)
}

func TestListRelatedUsers(t *testing.T) {
	c := qt.New(t)
	s := SetupTest(c)
	ctx := c.Context()

	groupUUID := uuid.NewString()
	group := jimmnames.NewGroupTag(groupUUID)

	groupUUID2 := uuid.NewString()
	group2 := jimmnames.NewGroupTag(groupUUID2)

	controllerUUID, err := uuid.NewRandom()
	c.Assert(err, qt.IsNil)
	controller := names.NewControllerTag(controllerUUID.String())

	modelUUID, err := uuid.NewRandom()
	c.Assert(err, qt.IsNil)
	model := names.NewModelTag(modelUUID.String())

	offerUUID, err := uuid.NewRandom()
	c.Assert(err, qt.IsNil)
	offer := names.NewApplicationOfferTag(offerUUID.String())

	adam := names.NewUserTag("adam")
	alice := names.NewUserTag("alice")
	eve := names.NewUserTag("eve")

	tuples := []openfga.Tuple{{
		Object:   ofganames.ConvertTag(eve),
		Relation: ofganames.MemberRelation,
		Target:   ofganames.ConvertTag(jimmnames.NewGroupTag(groupUUID)),
	}, {
		Object:   ofganames.ConvertTagWithRelation(group, ofganames.MemberRelation),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(controller),
	}, {
		Object:   ofganames.ConvertTag(controller),
		Relation: ofganames.ControllerRelation,
		Target:   ofganames.ConvertTag(model),
	}, {
		Object:   ofganames.ConvertTag(alice),
		Relation: ofganames.WriterRelation,
		Target:   ofganames.ConvertTag(model),
	}, {
		Object:   ofganames.ConvertTag(model),
		Relation: ofganames.ModelRelation,
		Target:   ofganames.ConvertTag(offer),
	}, {
		Object:   ofganames.ConvertTag(alice),
		Relation: ofganames.ReaderRelation,
		Target:   ofganames.ConvertTag(offer),
	}, {
		Object:   ofganames.ConvertTag(adam),
		Relation: ofganames.MemberRelation,
		Target:   ofganames.ConvertTag(group2),
	}, {
		Object:   ofganames.ConvertTagWithRelation(group2, ofganames.MemberRelation),
		Relation: ofganames.MemberRelation,
		Target:   ofganames.ConvertTag(group),
	}}
	err = s.ofgaClient.AddRelation(ctx, tuples...)
	c.Assert(err, qt.IsNil)

	eveIdentity, err := dbmodel.NewIdentity("eve")
	c.Assert(err, qt.IsNil)

	eveUser := openfga.NewUser(eveIdentity, s.ofgaClient)
	isAdministrator, err := openfga.IsAdministrator(ctx, eveUser, offer)
	c.Assert(err, qt.IsNil)
	c.Assert(isAdministrator, qt.Equals, true)

	users, err := openfga.ListUsersWithAccess(ctx, s.ofgaClient, offer, ofganames.ReaderRelation)
	c.Assert(err, qt.IsNil)
	usernames := make([]string, len(users))
	for i, user := range users {
		usernames[i] = user.Tag().Id()
	}
	sort.Strings(usernames)
	c.Assert(usernames, qt.DeepEquals, []string{"adam", "alice", "eve"})
}

func TestListModels(t *testing.T) {
	c := qt.New(t)
	s := SetupTest(c)
	ctx := c.Context()

	model1UUID, err := uuid.NewRandom()
	c.Assert(err, qt.IsNil)
	model1 := names.NewModelTag(model1UUID.String())

	model2UUID, err := uuid.NewRandom()
	c.Assert(err, qt.IsNil)
	model2 := names.NewModelTag(model2UUID.String())

	model3UUID, err := uuid.NewRandom()
	c.Assert(err, qt.IsNil)
	model3 := names.NewModelTag(model3UUID.String())

	adam := names.NewUserTag("adam")

	tuples := []openfga.Tuple{{
		Object:   ofganames.ConvertTag(adam),
		Relation: ofganames.ReaderRelation,
		Target:   ofganames.ConvertTag(model1),
	}, {
		Object:   ofganames.ConvertTag(adam),
		Relation: ofganames.WriterRelation,
		Target:   ofganames.ConvertTag(model2),
	}, {
		Object:   ofganames.ConvertTag(adam),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(model3),
	}}
	err = s.ofgaClient.AddRelation(ctx, tuples...)
	c.Assert(err, qt.IsNil)

	adamIdentity, err := dbmodel.NewIdentity(adam.Name())
	c.Assert(err, qt.IsNil)

	adamUser := openfga.NewUser(adamIdentity, s.ofgaClient)
	modelUUIDs, err := adamUser.ListModels(ctx, ofganames.ReaderRelation)
	c.Assert(err, qt.IsNil)
	wantUUIDs := []string{model1UUID.String(), model2UUID.String(), model3UUID.String()}
	sort.Strings(wantUUIDs)
	sort.Strings(modelUUIDs)
	c.Assert(modelUUIDs, qt.DeepEquals, wantUUIDs)
}

func TestListApplicationOffers(t *testing.T) {
	c := qt.New(t)
	s := SetupTest(c)
	ctx := c.Context()

	offer1UUID, err := uuid.NewRandom()
	c.Assert(err, qt.IsNil)
	offer1 := names.NewApplicationOfferTag(offer1UUID.String())

	offer2UUID, err := uuid.NewRandom()
	c.Assert(err, qt.IsNil)
	offer2 := names.NewApplicationOfferTag(offer2UUID.String())

	offer3UUID, err := uuid.NewRandom()
	c.Assert(err, qt.IsNil)
	offer3 := names.NewApplicationOfferTag(offer3UUID.String())

	adam := names.NewUserTag("adam")

	tuples := []openfga.Tuple{{
		Object:   ofganames.ConvertTag(adam),
		Relation: ofganames.ReaderRelation,
		Target:   ofganames.ConvertTag(offer1),
	}, {
		Object:   ofganames.ConvertTag(adam),
		Relation: ofganames.ConsumerRelation,
		Target:   ofganames.ConvertTag(offer2),
	}, {
		Object:   ofganames.ConvertTag(adam),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(offer3),
	}}
	err = s.ofgaClient.AddRelation(ctx, tuples...)
	c.Assert(err, qt.IsNil)

	adamIdentity, err := dbmodel.NewIdentity(adam.Name())
	c.Assert(err, qt.IsNil)

	adamUser := openfga.NewUser(adamIdentity, s.ofgaClient)
	offerUUIDs, err := adamUser.ListApplicationOffers(ctx, ofganames.ReaderRelation)
	c.Assert(err, qt.IsNil)
	wantUUIDs := []string{offer1UUID.String(), offer2UUID.String(), offer3UUID.String()}
	sort.Strings(wantUUIDs)
	sort.Strings(offerUUIDs)
	c.Assert(offerUUIDs, qt.DeepEquals, wantUUIDs)
}

func TestUnsetMultipleResourceAccesses(t *testing.T) {
	c := qt.New(t)
	s := SetupTest(c)
	ctx := c.Context()

	tests := []struct {
		name     string
		pageSize int32
	}{{
		name:     "pageSize: 0 (OpenFGA default)",
		pageSize: 0,
	}, {
		name:     "pageSize: 100 (OpenFGA max)",
		pageSize: 100,
	}, {
		name:     "pageSize: 1",
		pageSize: 1,
	}, {
		name:     "pageSize: 2",
		pageSize: 2,
	}, {
		name:     "pageSize: 3",
		pageSize: 3,
	}, {
		name:     "pageSize: 4",
		pageSize: 4,
	}}

	for _, tt := range tests {
		modelUUID, err := uuid.NewRandom()
		c.Assert(err, qt.IsNil)
		model := names.NewModelTag(modelUUID.String())

		adam := names.NewUserTag("adam")

		adamIdentity, err := dbmodel.NewIdentity("adam")
		c.Assert(err, qt.IsNil)

		adamUser := openfga.NewUser(adamIdentity, s.ofgaClient)

		// Note that the total number of tuples in OpenFGA actually has no
		// effect here, because the `unsetMultipleResourceAccesses` function
		// queries for tuples that have a specific object and target. So, the
		// returned tuples are just a few. That's why we've used user-to-model
		// tuples in this test because they have the highest number of possible
		// relations (i.e., reader, writer, and administrator).
		tuples := []openfga.Tuple{{
			Object:   ofganames.ConvertTag(adam),
			Relation: ofganames.ReaderRelation,
			Target:   ofganames.ConvertTag(model),
		}, {
			Object:   ofganames.ConvertTag(adam),
			Relation: ofganames.WriterRelation,
			Target:   ofganames.ConvertTag(model),
		}, {
			Object:   ofganames.ConvertTag(adam),
			Relation: ofganames.AdministratorRelation,
			Target:   ofganames.ConvertTag(model),
		}}

		err = s.ofgaClient.AddRelation(ctx, tuples...)
		c.Assert(err, qt.IsNil)

		err = openfga.UnsetMultipleResourceAccesses(
			ctx, adamUser, model,
			[]openfga.Relation{
				ofganames.ReaderRelation,
				ofganames.WriterRelation,
				ofganames.AdministratorRelation,
			},
			tt.pageSize,
		)
		c.Assert(err, qt.IsNil)

		retrieved, _, err := s.cofgaClient.FindMatchingTuples(ctx, openfga.Tuple{Target: ofganames.ConvertTag(model)}, 0, "")
		c.Assert(err, qt.IsNil)
		c.Assert(retrieved, qt.HasLen, 0)
	}
}
