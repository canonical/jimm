// Copyright 2024 Canonical.

package openfga_test

import (
	"context"
	"sort"

	cofga "github.com/canonical/ofga"
	"github.com/google/uuid"
	"github.com/juju/names/v5"
	gc "gopkg.in/check.v1"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/openfga"
	ofganames "github.com/canonical/jimm/v3/internal/openfga/names"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
	jimmnames "github.com/canonical/jimm/v3/pkg/names"
)

type userTestSuite struct {
	ofgaClient  *openfga.OFGAClient
	cofgaClient *cofga.Client
}

var _ = gc.Suite(&userTestSuite{})

func (s *userTestSuite) SetUpTest(c *gc.C) {
	client, cofgaClient, _, err := jimmtest.SetupTestOFGAClient(c.TestName())
	c.Assert(err, gc.IsNil)
	s.cofgaClient = cofgaClient
	s.ofgaClient = client
}
func (s *userTestSuite) TestIsAdministrator(c *gc.C) {
	ctx := context.Background()

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
	c.Assert(err, gc.IsNil)

	uIdentity, err := dbmodel.NewIdentity(user.Id())
	c.Assert(err, gc.IsNil)
	u := openfga.NewUser(
		uIdentity,
		s.ofgaClient,
	)

	allowed, err := openfga.IsAdministrator(ctx, u, controller)
	c.Assert(err, gc.IsNil)
	c.Assert(allowed, gc.Equals, true)
}

func (s *userTestSuite) TestModelAccess(c *gc.C) {
	ctx := context.Background()

	groupUUID := uuid.NewString()
	group := jimmnames.NewGroupTag(groupUUID)

	controllerUUID, err := uuid.NewRandom()
	c.Assert(err, gc.IsNil)
	controller := names.NewControllerTag(controllerUUID.String())

	modelUUID, err := uuid.NewRandom()
	c.Assert(err, gc.IsNil)
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
	c.Assert(err, gc.IsNil)

	adamIdentity, err := dbmodel.NewIdentity("adam")
	c.Assert(err, gc.IsNil)
	eveIdentity, err := dbmodel.NewIdentity(eve.Id())
	c.Assert(err, gc.IsNil)
	aliceIdentity, err := dbmodel.NewIdentity(alice.Id())
	c.Assert(err, gc.IsNil)

	adamUser := openfga.NewUser(adamIdentity, s.ofgaClient)
	eveUser := openfga.NewUser(eveIdentity, s.ofgaClient)
	aliceUser := openfga.NewUser(aliceIdentity, s.ofgaClient)

	relation := eveUser.GetModelAccess(ctx, model)
	c.Assert(relation, gc.DeepEquals, ofganames.AdministratorRelation)

	relation = aliceUser.GetModelAccess(ctx, model)
	c.Assert(relation, gc.DeepEquals, ofganames.WriterRelation)

	relation = adamUser.GetModelAccess(ctx, model)
	c.Assert(relation, gc.DeepEquals, ofganames.NoRelation)

	allowed, err := eveUser.IsModelReader(ctx, model)
	c.Assert(err, gc.IsNil)
	c.Assert(allowed, gc.Equals, true)

	allowed, err = eveUser.IsModelWriter(ctx, model)
	c.Assert(err, gc.IsNil)
	c.Assert(allowed, gc.Equals, true)

	allowed, err = adamUser.IsModelWriter(ctx, model)
	c.Assert(err, gc.IsNil)
	c.Assert(allowed, gc.Equals, false)
}

func (s *userTestSuite) TestSetModelAccess(c *gc.C) {
	ctx := context.Background()
	modelUUID, err := uuid.NewRandom()
	c.Assert(err, gc.IsNil)
	model := names.NewModelTag(modelUUID.String())

	eve := names.NewUserTag("eve")
	alice := names.NewUserTag("alice")

	adamIdentity, err := dbmodel.NewIdentity("adam")
	c.Assert(err, gc.IsNil)
	eveIdentity, err := dbmodel.NewIdentity(eve.Id())
	c.Assert(err, gc.IsNil)
	aliceIdentity, err := dbmodel.NewIdentity(alice.Id())
	c.Assert(err, gc.IsNil)

	adamUser := openfga.NewUser(adamIdentity, s.ofgaClient)
	eveUser := openfga.NewUser(eveIdentity, s.ofgaClient)
	aliceUser := openfga.NewUser(aliceIdentity, s.ofgaClient)

	err = eveUser.SetModelAccess(ctx, model, ofganames.AdministratorRelation)
	c.Assert(err, gc.IsNil)

	err = eveUser.SetModelAccess(ctx, model, ofganames.AdministratorRelation)
	c.Assert(err, gc.IsNil)

	err = aliceUser.SetModelAccess(ctx, model, ofganames.WriterRelation)
	c.Assert(err, gc.IsNil)

	relation := eveUser.GetModelAccess(ctx, model)
	c.Assert(relation, gc.DeepEquals, ofganames.AdministratorRelation)

	relation = aliceUser.GetModelAccess(ctx, model)
	c.Assert(relation, gc.DeepEquals, ofganames.WriterRelation)

	relation = adamUser.GetModelAccess(ctx, model)
	c.Assert(relation, gc.DeepEquals, ofganames.NoRelation)
}

func (s *userTestSuite) TestCloudAccess(c *gc.C) {
	ctx := context.Background()

	groupUUID := uuid.NewString()
	group := jimmnames.NewGroupTag(groupUUID)

	controllerUUID, err := uuid.NewRandom()
	c.Assert(err, gc.IsNil)
	controller := names.NewControllerTag(controllerUUID.String())

	cloudUUID, err := uuid.NewRandom()
	c.Assert(err, gc.IsNil)
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
	c.Assert(err, gc.IsNil)
	i, err := dbmodel.NewIdentity("adam")
	c.Assert(err, gc.IsNil)
	eveIdentity, err := dbmodel.NewIdentity(eve.Id())
	c.Assert(err, gc.IsNil)
	aliceIdentity, err := dbmodel.NewIdentity(alice.Id())
	c.Assert(err, gc.IsNil)

	adamUser := openfga.NewUser(i, s.ofgaClient)
	eveUser := openfga.NewUser(eveIdentity, s.ofgaClient)
	aliceUser := openfga.NewUser(aliceIdentity, s.ofgaClient)

	relation := eveUser.GetCloudAccess(ctx, cloud)
	c.Assert(relation, gc.DeepEquals, ofganames.AdministratorRelation)

	relation = aliceUser.GetCloudAccess(ctx, cloud)
	c.Assert(relation, gc.DeepEquals, ofganames.CanAddModelRelation)

	relation = adamUser.GetCloudAccess(ctx, cloud)
	c.Assert(relation, gc.DeepEquals, ofganames.NoRelation)
}

func (s *userTestSuite) TestSetCloudAccess(c *gc.C) {
	ctx := context.Background()
	cloudUUID, err := uuid.NewRandom()
	c.Assert(err, gc.IsNil)
	cloud := names.NewCloudTag(cloudUUID.String())

	eve := names.NewUserTag("eve")
	alice := names.NewUserTag("alice")

	adamIdentity, err := dbmodel.NewIdentity("adam")
	c.Assert(err, gc.IsNil)
	eveIdentity, err := dbmodel.NewIdentity(eve.Id())
	c.Assert(err, gc.IsNil)
	aliceIdentity, err := dbmodel.NewIdentity(alice.Id())
	c.Assert(err, gc.IsNil)

	adamUser := openfga.NewUser(adamIdentity, s.ofgaClient)
	eveUser := openfga.NewUser(eveIdentity, s.ofgaClient)
	aliceUser := openfga.NewUser(aliceIdentity, s.ofgaClient)

	err = eveUser.SetCloudAccess(ctx, cloud, ofganames.AdministratorRelation)
	c.Assert(err, gc.IsNil)

	// re-setting an existing relation should be fine
	err = eveUser.SetCloudAccess(ctx, cloud, ofganames.AdministratorRelation)
	c.Assert(err, gc.IsNil)

	err = aliceUser.SetCloudAccess(ctx, cloud, ofganames.CanAddModelRelation)
	c.Assert(err, gc.IsNil)

	relation := eveUser.GetCloudAccess(ctx, cloud)
	c.Assert(relation, gc.DeepEquals, ofganames.AdministratorRelation)

	relation = aliceUser.GetCloudAccess(ctx, cloud)
	c.Assert(relation, gc.DeepEquals, ofganames.CanAddModelRelation)

	relation = adamUser.GetCloudAccess(ctx, cloud)
	c.Assert(relation, gc.DeepEquals, ofganames.NoRelation)
}

func (s *userTestSuite) TestControllerAccess(c *gc.C) {
	ctx := context.Background()

	groupUUID := uuid.NewString()
	group := jimmnames.NewGroupTag(groupUUID)

	controllerUUID, err := uuid.NewRandom()
	c.Assert(err, gc.IsNil)
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
	c.Assert(err, gc.IsNil)

	adamIdentity, err := dbmodel.NewIdentity("adam")
	c.Assert(err, gc.IsNil)
	eveIdentity, err := dbmodel.NewIdentity(eve.Id())
	c.Assert(err, gc.IsNil)
	aliceIdentity, err := dbmodel.NewIdentity(alice.Id())
	c.Assert(err, gc.IsNil)

	adamUser := openfga.NewUser(adamIdentity, s.ofgaClient)
	eveUser := openfga.NewUser(eveIdentity, s.ofgaClient)
	aliceUser := openfga.NewUser(aliceIdentity, s.ofgaClient)

	relation := eveUser.GetControllerAccess(ctx, controller)
	c.Assert(relation, gc.DeepEquals, ofganames.AdministratorRelation)

	relation = aliceUser.GetControllerAccess(ctx, controller)
	c.Assert(relation, gc.DeepEquals, ofganames.NoRelation)

	relation = aliceUser.GetAuditLogViewerAccess(ctx, controller)
	c.Assert(relation, gc.DeepEquals, ofganames.AuditLogViewerRelation)

	relation = adamUser.GetControllerAccess(ctx, controller)
	c.Assert(relation, gc.DeepEquals, ofganames.NoRelation)

	relation = adamUser.GetAuditLogViewerAccess(ctx, controller)
	c.Assert(relation, gc.DeepEquals, ofganames.NoRelation)
}

func (s *userTestSuite) TestSetControllerAccess(c *gc.C) {
	ctx := context.Background()
	controllerUUID, err := uuid.NewRandom()
	c.Assert(err, gc.IsNil)
	controller := names.NewControllerTag(controllerUUID.String())

	eve := names.NewUserTag("eve")
	alice := names.NewUserTag("alice")

	adamIdentity, err := dbmodel.NewIdentity("adam")
	c.Assert(err, gc.IsNil)
	eveIdentity, err := dbmodel.NewIdentity(eve.Id())
	c.Assert(err, gc.IsNil)
	aliceIdentity, err := dbmodel.NewIdentity(alice.Id())
	c.Assert(err, gc.IsNil)

	adamUser := openfga.NewUser(adamIdentity, s.ofgaClient)
	eveUser := openfga.NewUser(eveIdentity, s.ofgaClient)
	aliceUser := openfga.NewUser(aliceIdentity, s.ofgaClient)

	err = eveUser.SetControllerAccess(ctx, controller, ofganames.AdministratorRelation)
	c.Assert(err, gc.IsNil)

	// re-setting an existing relation should be fine
	err = eveUser.SetControllerAccess(ctx, controller, ofganames.AdministratorRelation)
	c.Assert(err, gc.IsNil)

	err = aliceUser.SetControllerAccess(ctx, controller, ofganames.AuditLogViewerRelation)
	c.Assert(err, gc.IsNil)

	relation := eveUser.GetControllerAccess(ctx, controller)
	c.Assert(relation, gc.DeepEquals, ofganames.AdministratorRelation)

	relation = aliceUser.GetControllerAccess(ctx, controller)
	c.Assert(relation, gc.DeepEquals, ofganames.NoRelation)

	relation = aliceUser.GetAuditLogViewerAccess(ctx, controller)
	c.Assert(relation, gc.DeepEquals, ofganames.AuditLogViewerRelation)

	relation = adamUser.GetControllerAccess(ctx, controller)
	c.Assert(relation, gc.DeepEquals, ofganames.NoRelation)

	relation = adamUser.GetAuditLogViewerAccess(ctx, controller)
	c.Assert(relation, gc.DeepEquals, ofganames.NoRelation)
}

func (s *userTestSuite) TestUnsetAuditLogViewerAccess(c *gc.C) {
	ctx := context.Background()

	controllerUUID, err := uuid.NewRandom()
	c.Assert(err, gc.IsNil)
	controller := names.NewControllerTag(controllerUUID.String())

	aliceIdentity, err := dbmodel.NewIdentity("alice")
	c.Assert(err, gc.IsNil)

	aliceUser := openfga.NewUser(aliceIdentity, s.ofgaClient)

	tuples := []openfga.Tuple{{
		Object:   ofganames.ConvertTag(aliceUser.Identity.ResourceTag()),
		Relation: ofganames.AuditLogViewerRelation,
		Target:   ofganames.ConvertTag(controller),
	}}
	err = s.ofgaClient.AddRelation(ctx, tuples...)
	c.Assert(err, gc.IsNil)

	relation := aliceUser.GetAuditLogViewerAccess(ctx, controller)
	c.Assert(relation, gc.DeepEquals, ofganames.AuditLogViewerRelation)

	// Un-setting audit log viewer relation
	err = aliceUser.UnsetAuditLogViewerAccess(ctx, controller)
	c.Assert(err, gc.IsNil)

	relation = aliceUser.GetAuditLogViewerAccess(ctx, controller)
	c.Assert(relation, gc.DeepEquals, ofganames.NoRelation)

	// Un-setting again should be fine
	err = aliceUser.UnsetAuditLogViewerAccess(ctx, controller)
	c.Assert(err, gc.IsNil)
}

func (s *userTestSuite) TestListRelatedUsers(c *gc.C) {
	ctx := context.Background()

	groupUUID := uuid.NewString()
	group := jimmnames.NewGroupTag(groupUUID)

	groupUUID2 := uuid.NewString()
	group2 := jimmnames.NewGroupTag(groupUUID2)

	controllerUUID, err := uuid.NewRandom()
	c.Assert(err, gc.IsNil)
	controller := names.NewControllerTag(controllerUUID.String())

	modelUUID, err := uuid.NewRandom()
	c.Assert(err, gc.IsNil)
	model := names.NewModelTag(modelUUID.String())

	offerUUID, err := uuid.NewRandom()
	c.Assert(err, gc.IsNil)
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
	c.Assert(err, gc.IsNil)

	eveIdentity, err := dbmodel.NewIdentity("eve")
	c.Assert(err, gc.IsNil)

	eveUser := openfga.NewUser(eveIdentity, s.ofgaClient)
	isAdministrator, err := openfga.IsAdministrator(ctx, eveUser, offer)
	c.Assert(err, gc.IsNil)
	c.Assert(isAdministrator, gc.Equals, true)

	users, err := openfga.ListUsersWithAccess(ctx, s.ofgaClient, offer, ofganames.ReaderRelation)
	c.Assert(err, gc.IsNil)
	usernames := make([]string, len(users))
	for i, user := range users {
		usernames[i] = user.Tag().Id()
	}
	sort.Strings(usernames)
	c.Assert(usernames, gc.DeepEquals, []string{"adam", "alice", "eve"})
}

func (s *userTestSuite) TestListModels(c *gc.C) {
	ctx := context.Background()

	model1UUID, err := uuid.NewRandom()
	c.Assert(err, gc.IsNil)
	model1 := names.NewModelTag(model1UUID.String())

	model2UUID, err := uuid.NewRandom()
	c.Assert(err, gc.IsNil)
	model2 := names.NewModelTag(model2UUID.String())

	model3UUID, err := uuid.NewRandom()
	c.Assert(err, gc.IsNil)
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
	c.Assert(err, gc.IsNil)

	adamIdentity, err := dbmodel.NewIdentity(adam.Name())
	c.Assert(err, gc.IsNil)

	adamUser := openfga.NewUser(adamIdentity, s.ofgaClient)
	modelUUIDs, err := adamUser.ListModels(ctx, ofganames.ReaderRelation)
	c.Assert(err, gc.IsNil)
	wantUUIDs := []string{model1UUID.String(), model2UUID.String(), model3UUID.String()}
	sort.Strings(wantUUIDs)
	sort.Strings(modelUUIDs)
	c.Assert(modelUUIDs, gc.DeepEquals, wantUUIDs)
}

func (s *userTestSuite) TestListApplicationOffers(c *gc.C) {
	ctx := context.Background()

	offer1UUID, err := uuid.NewRandom()
	c.Assert(err, gc.IsNil)
	offer1 := names.NewApplicationOfferTag(offer1UUID.String())

	offer2UUID, err := uuid.NewRandom()
	c.Assert(err, gc.IsNil)
	offer2 := names.NewApplicationOfferTag(offer2UUID.String())

	offer3UUID, err := uuid.NewRandom()
	c.Assert(err, gc.IsNil)
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
	c.Assert(err, gc.IsNil)

	adamIdentity, err := dbmodel.NewIdentity(adam.Name())
	c.Assert(err, gc.IsNil)

	adamUser := openfga.NewUser(adamIdentity, s.ofgaClient)
	offerUUIDs, err := adamUser.ListApplicationOffers(ctx, ofganames.ReaderRelation)
	c.Assert(err, gc.IsNil)
	wantUUIDs := []string{offer1UUID.String(), offer2UUID.String(), offer3UUID.String()}
	sort.Strings(wantUUIDs)
	sort.Strings(offerUUIDs)
	c.Assert(offerUUIDs, gc.DeepEquals, wantUUIDs)
}

func (s *userTestSuite) TestUnsetMultipleResourceAccesses(c *gc.C) {
	ctx := context.Background()

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
		c.Assert(err, gc.IsNil)
		model := names.NewModelTag(modelUUID.String())

		adam := names.NewUserTag("adam")

		adamIdentity, err := dbmodel.NewIdentity("adam")
		c.Assert(err, gc.IsNil)

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
		c.Assert(err, gc.IsNil)

		err = openfga.UnsetMultipleResourceAccesses(
			ctx, adamUser, model,
			[]openfga.Relation{
				ofganames.ReaderRelation,
				ofganames.WriterRelation,
				ofganames.AdministratorRelation,
			},
			tt.pageSize,
		)
		c.Assert(err, gc.IsNil)

		retrieved, _, err := s.cofgaClient.FindMatchingTuples(ctx, openfga.Tuple{Target: ofganames.ConvertTag(model)}, 0, "")
		c.Assert(err, gc.IsNil)
		c.Assert(retrieved, gc.HasLen, 0)
	}
}
