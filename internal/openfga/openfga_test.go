package openfga_test

import (
	"context"
	"fmt"
	"testing"

	ofga "github.com/CanonicalLtd/jimm/internal/openfga"
	"github.com/google/uuid"
	openfga "github.com/openfga/go-sdk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type openFGATestSuite struct {
	suite.Suite
	ofgaClient *ofga.OFGAClient
	ofgaApi    openfga.OpenFgaApi
}

func (suite *openFGATestSuite) SetupSuite() {
	api, client := ofga.SetupTestOFGAClient()
	suite.ofgaApi = api
	suite.ofgaClient = client

}

func (suite *openFGATestSuite) TestCreateTupleKey() {
	t := suite.T()
	key := suite.ofgaClient.CreateTupleKey("user:diglett", "legendary", "pokemon:earth")
	assert.Equal(t, "user:diglett", key.GetUser())
	assert.Equal(t, "legendary", key.GetRelation())
	assert.Equal(t, "pokemon:earth", key.GetObject())
}

func (suite *openFGATestSuite) TestWritingTuplesToOFGADetectsBadObjects() {
	t := suite.T()
	ctx := context.Background()
	key1 := suite.ofgaClient.CreateTupleKey("user:diglett", "legendary", "pokemon:earth")
	key2 := suite.ofgaClient.CreateTupleKey("user:diglett", "awesome", "pokemon:earth")
	key3 := suite.ofgaClient.CreateTupleKey("user:dugtrio", "legendary", "pokemon:fire")

	err := suite.ofgaClient.AddRelations(ctx, key1, key2, key3)
	fgaErrCode, _ := openfga.NewErrorCodeFromValue("validation_error")
	serr, ok := err.(openfga.FgaApiValidationError)
	assert.True(t, ok)
	assert.Equal(t, 400, serr.ResponseStatusCode())
	assert.Equal(t, "POST", serr.RequestMethod())
	assert.Equal(t, "Write", serr.EndpointCategory())
	assert.Equal(t, *fgaErrCode, serr.ResponseCode())
	assert.ErrorContains(t, serr, "pokemon")
}

func (suite *openFGATestSuite) TestWritingTuplesToOFGADetectsSucceeds() {
	t := suite.T()
	ctx := context.Background()

	uuid1, _ := uuid.NewRandom()
	user1 := fmt.Sprintf("user:%s", uuid1)
	key1 := suite.ofgaClient.CreateTupleKey(user1, "member", "group:pokemon")

	uuid2, _ := uuid.NewRandom()
	user2 := fmt.Sprintf("user:%s", uuid2)
	key2 := suite.ofgaClient.CreateTupleKey(user2, "member", "group:pokemon")

	err := suite.ofgaClient.AddRelations(ctx, key1, key2)
	assert.NoError(t, err)
	changes, _, err := suite.ofgaApi.ReadChanges(ctx).Type_("group").Execute()
	assert.NoError(t, err)

	secondToLastInsertedTuple := changes.GetChanges()[len(changes.GetChanges())-2].GetTupleKey()
	assert.Equal(t, user1, secondToLastInsertedTuple.GetUser())

	lastInsertedTuple := changes.GetChanges()[len(changes.GetChanges())-1].GetTupleKey()
	assert.Equal(t, user2, lastInsertedTuple.GetUser())
}

func (suite *openFGATestSuite) TestCheckRelationResolvesX() {
	t := suite.T()
	ctx := context.Background()

	c := suite.ofgaClient
	u, _ := uuid.NewRandom()
	id := u.String()

	// Test the administrator flow of a group user being related to a controller via administrator relation
	// Make user member of group
	userToGroup := c.CreateTupleKey("user:diglett"+id, "member", "group:test-group-1"+id)
	// Make group members administrator of controller via member union
	groupToControllerAdmin := c.CreateTupleKey("group:test-group-1"+id+"#member", "administrator", "controller:test-controller-1"+id)
	// Make controller administrators admins of model via administrator union
	controllerToModelAdmin := c.CreateTupleKey("controller:test-controller-1"+id+"#administrator", "administrator", "model:test-model-1"+id)
	// Make controller administrators admin of appoffers via administrator union
	controllerToAppOfferAdmin := c.CreateTupleKey("controller:test-controller-1"+id+"#administrator", "administrator", "applicationoffer:test-offer-1"+id)

	// Test direct relation to a model from a user of a group via "writer" relation
	// Make user member of group
	userToGroupModelWriter := c.CreateTupleKey("user:dugtrio"+id, "member", "group:test-group-2"+id)
	// Make group members writer of model via member union
	groupToModelWriter := c.CreateTupleKey("group:test-group-2"+id+"#member", "writer", "model:test-model-2"+id)

	// Test direct relation to a model from a user of a group via "reader" relation
	// Make user member of group
	userToGroupModelReader := c.CreateTupleKey("user:dugtrio"+id, "member", "group:test-group-3"+id)
	// Make group members writer of model via member union
	groupToModelReader := c.CreateTupleKey("group:test-group-3"+id+"#member", "reader", "model:test-model-3"+id)

	// Test direct relation to an applicationoffer from a user of a group via "consumer" relation
	// Make user member of group
	userToGroupOfferConsumer := c.CreateTupleKey("user:dugtrio"+id, "member", "group:test-group-4"+id)
	// Make group members consumer of offer via member union
	groupToOfferConsumer := c.CreateTupleKey("group:test-group-4"+id+"#member", "consumer", "applicationoffer:test-offer-2"+id)

	// Test direct relation to an applicationoffer from a user of a group via "reader" relation
	// Make user member of group
	userToGroupOfferReader := c.CreateTupleKey("user:dugtrio"+id, "member", "group:test-group-5"+id)
	// Make group members reader of offer via member union
	groupToOfferReader := c.CreateTupleKey("group:test-group-5"+id+"#member", "reader", "applicationoffer:test-offer-3"+id)

	err := c.AddRelations(
		ctx,
		userToGroup,
		groupToControllerAdmin,
		controllerToModelAdmin,
		controllerToAppOfferAdmin,
		userToGroupModelWriter,
		groupToModelWriter,
		userToGroupModelReader,
		groupToModelReader,
		userToGroupOfferConsumer,
		groupToOfferConsumer,
		userToGroupOfferReader,
		groupToOfferReader,
	)
	assert.NoError(t, err)

	type expected struct {
		resolution string
		allowed    bool
	}

	type test struct {
		input openfga.TupleKey
		want  expected
	}

	tests := []test{
		// Test user:diglett -> member -> group:test-group-1
		{
			input: c.CreateTupleKey("user:diglett"+id, "member", "group:test-group-1"+id), want: expected{
				resolution: ".(direct).",
				allowed:    true,
			},
		},
		// Test user:diglett -> member -> controller:test-controller-1
		{
			input: c.CreateTupleKey("user:diglett"+id, "administrator", "controller:test-controller-1"+id), want: expected{
				resolution: ".(direct).group:test-group-1" + id + "#member.(direct).",
				allowed:    true,
			},
		},
		// Test user:diglett -> administrator -> model:test-model-1
		{
			input: c.CreateTupleKey("user:diglett"+id, "administrator", "model:test-model-1"+id), want: expected{
				resolution: ".union.0(direct).controller:test-controller-1" + id + "#administrator.(direct).group:test-group-1" + id + "#member.(direct).",
				allowed:    true,
			},
		},
		// Test user:diglett -> reader -> model:test-model-1 (due to group#member -> controller#admin unioned to model #admin)
		{
			input: c.CreateTupleKey("user:diglett"+id, "reader", "model:test-model-1"+id), want: expected{
				resolution: ".union.1(computed-userset).model:test-model-1" + id +
					"#writer.union.1(computed-userset).model:test-model-1" + id +
					"#administrator.union.0(direct).controller:test-controller-1" + id +
					"#administrator.(direct).group:test-group-1" + id +
					"#member.(direct).",
				allowed: true,
			},
		},

		// Test user:diglett -> writer -> model:test-model-1 (due to group#member -> controller#admin unioned to model #admin)
		{
			input: c.CreateTupleKey("user:diglett"+id, "writer", "model:test-model-1"+id), want: expected{
				resolution: ".union.1(computed-userset).model:test-model-1" + id + "#administrator.union.0(direct).controller:test-controller-1" + id + "#administrator.(direct).group:test-group-1" + id + "#member.(direct).",
				allowed:    true,
			},
		},
		// Test user:diglett -> administrator -> applicationoffer:test-offer-1
		{
			input: c.CreateTupleKey("user:diglett"+id, "administrator", "applicationoffer:test-offer-1"+id), want: expected{
				resolution: ".union.0(direct).controller:test-controller-1" + id + "#administrator.(direct).group:test-group-1" + id + "#member.(direct).",
				allowed:    true,
			},
		},
		// Test user:dugtrio -> writer -> model:test-model-2
		{
			input: c.CreateTupleKey("user:dugtrio"+id, "writer", "model:test-model-2"+id), want: expected{
				resolution: ".union.0(direct).group:test-group-2" + id + "#member.(direct).",
				allowed:    true,
			},
		},
		// Test user:dugtrio -> reader -> model:test-model-2 (due to union from writer to reader)
		{
			input: c.CreateTupleKey("user:dugtrio"+id, "reader", "model:test-model-2"+id), want: expected{
				resolution: ".union.1(computed-userset).model:test-model-2" + id + "#writer.union.0(direct).group:test-group-2" + id + "#member.(direct).",
				allowed:    true,
			},
		},

		// Test user:dugtrio -> reader -> model:test-model-3 (due to direct relation from group)
		{
			input: c.CreateTupleKey("user:dugtrio"+id, "reader", "model:test-model-3"+id), want: expected{
				resolution: ".union.0(direct).group:test-group-3" + id + "#member.(direct).",
				allowed:    true,
			},
		},
		// Test user:dugtrio -> writer -> model:test-model-3 (FAILS as there is no union or direct relation to writer)
		{
			input: c.CreateTupleKey("user:dugtrio"+id, "writer", "model:test-model-3"+id), want: expected{
				resolution: "",
				allowed:    false,
			},
		},
		// Test user:dugtrio -> consumer -> applicationoffer:test-offer-2 (due to direct relation from group)
		{
			input: c.CreateTupleKey("user:dugtrio"+id, "consumer", "applicationoffer:test-offer-2"+id), want: expected{
				resolution: ".union.0(direct).group:test-group-4" + id + "#member.(direct).",
				allowed:    true,
			},
		},
		// Test user:dugtrio -> reader -> applicationoffer:test-offer-2 (due to direct relation from group and union from consumer to reader)
		{
			input: c.CreateTupleKey("user:dugtrio"+id, "reader", "applicationoffer:test-offer-2"+id), want: expected{
				resolution: ".union.1(computed-userset).applicationoffer:test-offer-2" + id + "#consumer.union.0(direct).group:test-group-4" + id + "#member.(direct).",
				allowed:    true,
			},
		},
		// Test user:dugtrio -> reader -> applicationoffer:test-offer-3 (due to direct relation from group)
		{
			input: c.CreateTupleKey("user:dugtrio"+id, "reader", "applicationoffer:test-offer-3"+id), want: expected{
				resolution: ".union.0(direct).group:test-group-5" + id + "#member.(direct).",
				allowed:    true,
			},
		},
		// Test user:dugtrio -> consumer -> applicationoffer:test-offer-3 (FAILS as there is no union or direct relation to writer)
		{
			input: c.CreateTupleKey("user:dugtrio"+id, "consumer", "applicationoffer:test-offer-3"+id), want: expected{
				resolution: "",
				allowed:    false,
			},
		},
	}

	for _, tc := range tests {
		allowed, resolution, err := c.CheckRelation(ctx, tc.input, true)
		assert.NoError(t, err)
		assert.Equal(t, tc.want.resolution, resolution)
		assert.Equal(t, tc.want.allowed, allowed)
	}

}

func TestOpenFGATestSuite(t *testing.T) {
	suite.Run(t, new(openFGATestSuite))
}
