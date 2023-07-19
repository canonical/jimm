// Copyright 2023 canonical.

package openfga_test

import (
	"context"
	"sort"

	"github.com/google/uuid"
	"github.com/juju/names/v4"
	openfga "github.com/openfga/go-sdk"
	gc "gopkg.in/check.v1"

	"github.com/canonical/jimm/internal/dbmodel"
	"github.com/canonical/jimm/internal/jimmtest"
	ofga "github.com/canonical/jimm/internal/openfga"
	ofganames "github.com/canonical/jimm/internal/openfga/names"
	jimmnames "github.com/canonical/jimm/pkg/names"
)

type userTestSuite struct {
	ofgaClient *ofga.OFGAClient
	ofgaApi    openfga.OpenFgaApi
}

var _ = gc.Suite(&userTestSuite{})

func (s *userTestSuite) SetUpTest(c *gc.C) {
	api, client, _, err := jimmtest.SetupTestOFGAClient(c.TestName())
	c.Assert(err, gc.IsNil)
	s.ofgaApi = api
	s.ofgaClient = client
}
func (s *userTestSuite) TestIsAdministrator(c *gc.C) {
	ctx := context.Background()

	groupid := "3"
	controllerUUID, _ := uuid.NewRandom()
	controller := names.NewControllerTag(controllerUUID.String())

	user := names.NewUserTag("eve")
	userToGroup := ofga.Tuple{
		Object:   ofganames.ConvertTag(user),
		Relation: "member",
		Target:   ofganames.ConvertTag(jimmnames.NewGroupTag(groupid)),
	}
	groupToController := ofga.Tuple{
		Object:   ofganames.ConvertTagWithRelation(jimmnames.NewGroupTag(groupid), ofganames.MemberRelation),
		Relation: "administrator",
		Target:   ofganames.ConvertTag(controller),
	}

	err := s.ofgaClient.AddRelations(ctx, userToGroup, groupToController)
	c.Assert(err, gc.IsNil)

	u := ofga.NewUser(
		&dbmodel.User{
			Username: user.Id(),
		},
		s.ofgaClient,
	)

	allowed, err := ofga.IsAdministrator(ctx, u, controller)
	c.Assert(err, gc.IsNil)
	c.Assert(allowed, gc.Equals, true)
}

func (s *userTestSuite) TestModelAccess(c *gc.C) {
	ctx := context.Background()

	groupid := "3"
	group := jimmnames.NewGroupTag(groupid)

	controllerUUID, err := uuid.NewRandom()
	c.Assert(err, gc.IsNil)
	controller := names.NewControllerTag(controllerUUID.String())

	modelUUID, err := uuid.NewRandom()
	c.Assert(err, gc.IsNil)
	model := names.NewModelTag(modelUUID.String())

	eve := names.NewUserTag("eve")
	alice := names.NewUserTag("alice")

	tuples := []ofga.Tuple{{
		Object:   ofganames.ConvertTag(eve),
		Relation: ofganames.MemberRelation,
		Target:   ofganames.ConvertTag(jimmnames.NewGroupTag(groupid)),
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
	err = s.ofgaClient.AddRelations(ctx, tuples...)
	c.Assert(err, gc.IsNil)

	adamUser := ofga.NewUser(&dbmodel.User{Username: "adam"}, s.ofgaClient)
	eveUser := ofga.NewUser(&dbmodel.User{Username: eve.Id()}, s.ofgaClient)
	aliceUser := ofga.NewUser(&dbmodel.User{Username: alice.Id()}, s.ofgaClient)

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

	adamUser := ofga.NewUser(&dbmodel.User{Username: "adam"}, s.ofgaClient)
	eveUser := ofga.NewUser(&dbmodel.User{Username: eve.Id()}, s.ofgaClient)
	aliceUser := ofga.NewUser(&dbmodel.User{Username: alice.Id()}, s.ofgaClient)

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

	groupid := "3"
	group := jimmnames.NewGroupTag(groupid)

	controllerUUID, err := uuid.NewRandom()
	c.Assert(err, gc.IsNil)
	controller := names.NewControllerTag(controllerUUID.String())

	cloudUUID, err := uuid.NewRandom()
	c.Assert(err, gc.IsNil)
	cloud := names.NewCloudTag(cloudUUID.String())

	eve := names.NewUserTag("eve")
	alice := names.NewUserTag("alice")

	tuples := []ofga.Tuple{{
		Object:   ofganames.ConvertTag(eve),
		Relation: ofganames.MemberRelation,
		Target:   ofganames.ConvertTag(jimmnames.NewGroupTag(groupid)),
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
	err = s.ofgaClient.AddRelations(ctx, tuples...)
	c.Assert(err, gc.IsNil)

	adamUser := ofga.NewUser(&dbmodel.User{Username: "adam"}, s.ofgaClient)
	eveUser := ofga.NewUser(&dbmodel.User{Username: eve.Id()}, s.ofgaClient)
	aliceUser := ofga.NewUser(&dbmodel.User{Username: alice.Id()}, s.ofgaClient)

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

	adamUser := ofga.NewUser(&dbmodel.User{Username: "adam"}, s.ofgaClient)
	eveUser := ofga.NewUser(&dbmodel.User{Username: eve.Id()}, s.ofgaClient)
	aliceUser := ofga.NewUser(&dbmodel.User{Username: alice.Id()}, s.ofgaClient)

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

	groupid := "3"
	group := jimmnames.NewGroupTag(groupid)

	controllerUUID, err := uuid.NewRandom()
	c.Assert(err, gc.IsNil)
	controller := names.NewControllerTag(controllerUUID.String())

	eve := names.NewUserTag("eve")
	alice := names.NewUserTag("alice")

	tuples := []ofga.Tuple{{
		Object:   ofganames.ConvertTag(eve),
		Relation: ofganames.MemberRelation,
		Target:   ofganames.ConvertTag(jimmnames.NewGroupTag(groupid)),
	}, {
		Object:   ofganames.ConvertTagWithRelation(group, ofganames.MemberRelation),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(controller),
	}, {
		Object:   ofganames.ConvertTag(alice),
		Relation: ofganames.AuditLogViewerRelation,
		Target:   ofganames.ConvertTag(controller),
	}}
	err = s.ofgaClient.AddRelations(ctx, tuples...)
	c.Assert(err, gc.IsNil)

	adamUser := ofga.NewUser(&dbmodel.User{Username: "adam"}, s.ofgaClient)
	eveUser := ofga.NewUser(&dbmodel.User{Username: eve.Id()}, s.ofgaClient)
	aliceUser := ofga.NewUser(&dbmodel.User{Username: alice.Id()}, s.ofgaClient)

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

	adamUser := ofga.NewUser(&dbmodel.User{Username: "adam"}, s.ofgaClient)
	eveUser := ofga.NewUser(&dbmodel.User{Username: eve.Id()}, s.ofgaClient)
	aliceUser := ofga.NewUser(&dbmodel.User{Username: alice.Id()}, s.ofgaClient)

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

	aliceUser := ofga.NewUser(&dbmodel.User{Username: "alice"}, s.ofgaClient)

	tuples := []ofga.Tuple{{
		Object:   ofganames.ConvertTag(aliceUser.User.ResourceTag()),
		Relation: ofganames.AuditLogViewerRelation,
		Target:   ofganames.ConvertTag(controller),
	}}
	err = s.ofgaClient.AddRelations(ctx, tuples...)
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

	groupid := "3"
	group := jimmnames.NewGroupTag(groupid)

	groupid2 := "4"
	group2 := jimmnames.NewGroupTag(groupid2)

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

	tuples := []ofga.Tuple{{
		Object:   ofganames.ConvertTag(eve),
		Relation: ofganames.MemberRelation,
		Target:   ofganames.ConvertTag(jimmnames.NewGroupTag(groupid)),
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
	err = s.ofgaClient.AddRelations(ctx, tuples...)
	c.Assert(err, gc.IsNil)

	eveUser := ofga.NewUser(&dbmodel.User{Username: "eve"}, s.ofgaClient)
	isAdministrator, err := ofga.IsAdministrator(ctx, eveUser, offer)
	c.Assert(err, gc.IsNil)
	c.Assert(isAdministrator, gc.Equals, true)

	users, err := ofga.ListUsersWithAccess(ctx, s.ofgaClient, offer, ofganames.ReaderRelation)
	c.Assert(err, gc.IsNil)
	usernames := make([]string, len(users))
	for i, user := range users {
		usernames[i] = user.Tag().Id()
	}
	sort.Strings(usernames)
	c.Assert(usernames, gc.DeepEquals, []string{"adam", "alice", "eve"})
}
