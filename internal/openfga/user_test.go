// Copyright 2023 CanonicalLtd.

package openfga_test

import (
	"context"

	"github.com/google/uuid"
	"github.com/juju/names/v4"
	openfga "github.com/openfga/go-sdk"
	gc "gopkg.in/check.v1"

	"github.com/CanonicalLtd/jimm/internal/dbmodel"
	"github.com/CanonicalLtd/jimm/internal/jimmtest"
	ofga "github.com/CanonicalLtd/jimm/internal/openfga"
	ofganames "github.com/CanonicalLtd/jimm/internal/openfga/names"
	jimmnames "github.com/CanonicalLtd/jimm/pkg/names"
)

type userTestSuite struct {
	ofgaClient *ofga.OFGAClient
	ofgaApi    openfga.OpenFgaApi
}

var _ = gc.Suite(&userTestSuite{})

func (s *userTestSuite) SetUpTest(c *gc.C) {
	api, client, _ := jimmtest.SetupTestOFGAClient(c)
	s.ofgaApi = api
	s.ofgaClient = client
}
func (s *userTestSuite) TestControllerAdministrator(c *gc.C) {
	ctx := context.Background()

	groupid := "3"
	controllerUUID, _ := uuid.NewRandom()
	controller := names.NewControllerTag(controllerUUID.String())

	user := names.NewUserTag("eve")
	userToGroup := ofga.Tuple{
		Object:   ofganames.FromTag(user),
		Relation: "member",
		Target:   ofganames.FromTag(jimmnames.NewGroupTag(groupid)),
	}
	groupToController := ofga.Tuple{
		Object:   ofganames.FromTagWithRelation(jimmnames.NewGroupTag(groupid), ofganames.MemberRelation),
		Relation: "administrator",
		Target:   ofganames.FromTag(controller),
	}

	err := s.ofgaClient.AddRelations(ctx, userToGroup, groupToController)
	c.Assert(err, gc.IsNil)

	u := ofga.NewUser(
		&dbmodel.User{
			Username: user.Id(),
		},
		s.ofgaClient,
	)

	allowed, err := u.ControllerAdministrator(ctx, controller)
	c.Assert(err, gc.IsNil)
	c.Assert(allowed, gc.Equals, true)
}
