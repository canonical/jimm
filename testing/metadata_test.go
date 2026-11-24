// Copyright 2026 Canonical.

package testing

import (
	"testing"

	qt "github.com/frankban/quicktest"

	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
	"github.com/canonical/jimm/v3/pkg/api/params"
)

func TestMetadataFacade(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupJimmWithControllers(c)
	model := s.CreateModelForBob(c)
	conn := s.Open(c, nil, "bob@canonical.com", nil)
	defer conn.Close()

	const facade = "JIMM_Metadata"
	const version = 1
	entity := params.EntityMetadataTarget{EntityTag: model.ResourceTag().String()}
	values := map[string]string{"contact": "ops@example.com"}

	var setResult params.MetadataResults
	c.Assert(conn.APICall(facade, version, "", "SetMetadata", params.SetMetadataRequest{
		Entity: entity, Metadata: values,
	}, &setResult), qt.IsNil)
	c.Check(setResult.Metadata, qt.DeepEquals, values)

	get := params.GetMetadataRequest{Entity: entity}
	var getResult params.MetadataResults
	c.Assert(conn.APICall(facade, version, "", "GetMetadata", get, &getResult), qt.IsNil)
	c.Check(getResult.Metadata, qt.DeepEquals, values)

	var queryResult params.QueryMetadataResponse
	c.Assert(conn.APICall(facade, version, "", "QueryMetadata", params.QueryMetadataRequest{
		Matcher: &params.MetadataMatcher{Key: "contact", Value: "ops@example.com"},
	}, &queryResult), qt.IsNil)
	c.Check(queryResult.Results, qt.DeepEquals, []params.MetadataMatch{
		{Entity: entity, Key: "contact", Value: "ops@example.com"},
	})

	c.Assert(conn.APICall(facade, version, "", "RemoveMetadata", params.RemoveMetadataRequest{
		Entity: entity, Keys: []string{"contact"},
	}, nil), qt.IsNil)
	var afterRemove params.MetadataResults
	c.Assert(conn.APICall(facade, version, "", "GetMetadata", get, &afterRemove), qt.IsNil)
	c.Check(afterRemove.Metadata, qt.HasLen, 0)
}
