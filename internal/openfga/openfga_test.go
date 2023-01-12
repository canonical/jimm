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
	fgaErrCode, _ := openfga.NewErrorCodeFromValue("type_not_found")
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

func (suite *openFGATestSuite) TestDeletingTuplesToOFGADetectsSucceeds() {
	t := suite.T()
	ctx := context.Background()

	//Create tuples before writing to db
	uuid1, _ := uuid.NewRandom()
	user1 := fmt.Sprintf("user:%s", uuid1)
	key1 := suite.ofgaClient.CreateTupleKey(user1, "member", "group:pokemon")
	uuid2, _ := uuid.NewRandom()
	user2 := fmt.Sprintf("user:%s", uuid2)
	key2 := suite.ofgaClient.CreateTupleKey(user2, "member", "group:pokemon")

	//Delete before insert should fail
	err := suite.ofgaClient.DeleteRelations(ctx, key1, key2)
	_, ok := err.(openfga.FgaApiValidationError)
	assert.True(t, ok)
	assert.Contains(t, err.Error(), "cannot delete a tuple which does not exist")

	err = suite.ofgaClient.AddRelations(ctx, key1, key2)
	assert.NoError(t, err)

	//Delete after insert should succeed.
	err = suite.ofgaClient.DeleteRelations(ctx, key1, key2)
	assert.NoError(t, err)
	changes, _, err := suite.ofgaApi.ReadChanges(ctx).Type_("group").Execute()
	assert.NoError(t, err)

	secondToLastInsertedTuple := changes.GetChanges()[len(changes.GetChanges())-2]
	secondLastKey := secondToLastInsertedTuple.GetTupleKey()
	assert.Equal(t, user1, secondLastKey.GetUser())
	assert.Equal(t, openfga.DELETE, secondToLastInsertedTuple.GetOperation())

	lastInsertedTuple := changes.GetChanges()[len(changes.GetChanges())-1]
	lastKey := lastInsertedTuple.GetTupleKey()
	assert.Equal(t, user2, lastKey.GetUser())
	assert.Equal(t, openfga.DELETE, lastInsertedTuple.GetOperation())
}

func TestOpenFGATestSuite(t *testing.T) {
	suite.Run(t, new(openFGATestSuite))
}
