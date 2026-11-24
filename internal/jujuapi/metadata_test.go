// Copyright 2026 Canonical.

package jujuapi_test

import (
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
	"github.com/juju/names/v5"

	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/jimm/metadata"
	"github.com/canonical/jimm/v3/internal/jujuapi"
	"github.com/canonical/jimm/v3/internal/openfga"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
	"github.com/canonical/jimm/v3/internal/testutils/testdb"
	"github.com/canonical/jimm/v3/pkg/api/params"
)

func TestMetadataRequiresLogin(t *testing.T) {
	c := qt.New(t)
	root := jujuapi.NewControllerRoot(&jimmtest.JIMM{}, jujuapi.Params{})
	for _, method := range []string{"GetMetadata", "SetMetadata", "RemoveMetadata", "QueryMetadata"} {
		_, err := root.FindMethod("JIMM_Metadata", 1, method)
		c.Check(err, qt.IsNotNil, qt.Commentf("%s must not be registered before login", method))
	}
}

func TestMetadataGetRejectsInvalidTargets(t *testing.T) {
	c := qt.New(t)
	ctx := t.Context()
	root := newTestControllerRoot(&jimmtest.JIMM{}, "reader@example.com", false)

	_, err := root.GetMetadata(ctx, params.GetMetadataRequest{Entity: params.EntityMetadataTarget{
		EntityTag: "model-00000002-0000-0000-0000-000000000001",
		ModelTag:  "model-00000002-0000-0000-0000-000000000002",
	}})
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeBadRequest)
	_, err = root.GetMetadata(ctx, params.GetMetadataRequest{Entity: params.EntityMetadataTarget{
		EntityTag: "application-test-app",
	}})
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeBadRequest)
}

func TestMetadataQueryRequiresFilter(t *testing.T) {
	c := qt.New(t)
	root := newTestControllerRoot(&jimmtest.JIMM{}, "reader@example.com", false)

	_, err := root.QueryMetadata(t.Context(), params.QueryMetadataRequest{})
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeBadRequest)
}

func TestMetadataGetPermissions(t *testing.T) {
	c := qt.New(t)
	ctx := t.Context()
	f := setupMetadataTest(c)
	f.seed(c, f.app)

	req := params.GetMetadataRequest{Entity: f.app}
	for _, root := range []*jujuapi.ControllerRoot{f.reader, f.writer} {
		result, err := root.GetMetadata(ctx, req)
		c.Assert(err, qt.IsNil)
		c.Check(result.Metadata, qt.DeepEquals, map[string]string{"deletable": "true"})
	}

	_, err := f.reader.GetMetadata(ctx, params.GetMetadataRequest{Entity: f.otherApp})
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeUnauthorized)
}

func TestMetadataSetPermissions(t *testing.T) {
	c := qt.New(t)
	ctx := t.Context()
	f := setupMetadataTest(c)

	values := map[string]string{"deletable": "true"}
	_, err := f.reader.SetMetadata(ctx, params.SetMetadataRequest{Entity: f.app, Metadata: values})
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeUnauthorized)

	for _, target := range []params.EntityMetadataTarget{f.app, f.otherApp} {
		result, err := f.writer.SetMetadata(ctx, params.SetMetadataRequest{Entity: target, Metadata: values})
		c.Assert(err, qt.IsNil)
		c.Check(result.Metadata, qt.DeepEquals, values)
	}
}

func TestMetadataRemovePermissions(t *testing.T) {
	c := qt.New(t)
	ctx := t.Context()
	f := setupMetadataTest(c)
	f.seed(c, f.otherApp)

	err := f.reader.RemoveMetadata(ctx, params.RemoveMetadataRequest{Entity: f.app, All: true})
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeUnauthorized)

	c.Assert(f.writer.RemoveMetadata(ctx, params.RemoveMetadataRequest{Entity: f.otherApp, All: true}), qt.IsNil)
	result, err := f.writer.GetMetadata(ctx, params.GetMetadataRequest{Entity: f.otherApp})
	c.Assert(err, qt.IsNil)
	c.Check(result.Metadata, qt.HasLen, 0)
}

func TestMetadataQueryScope(t *testing.T) {
	c := qt.New(t)
	ctx := t.Context()
	f := setupMetadataTest(c)
	f.seed(c, f.model, f.app, f.otherApp)

	query := params.QueryMetadataRequest{Matcher: &params.MetadataMatcher{Key: "deletable", Value: "true"}}
	matches, err := f.reader.QueryMetadata(ctx, query)
	c.Assert(err, qt.IsNil)
	c.Check(matches.Results, qt.DeepEquals, []params.MetadataMatch{
		{Entity: f.app, Key: "deletable", Value: "true"},
		{Entity: f.model, Key: "deletable", Value: "true"},
	})

	matches, err = f.writer.QueryMetadata(ctx, query)
	c.Assert(err, qt.IsNil)
	c.Check(matches.Results, qt.HasLen, 3)
}

type metadataFixture struct {
	db                   *db.Database
	reader, writer       *jujuapi.ControllerRoot
	model, app, otherApp params.EntityMetadataTarget
}

func setupMetadataTest(c *qt.C) metadataFixture {
	c.Helper()
	ctx := c.Context()
	database := &db.Database{DB: testdb.PostgresDB(c, time.Now)}
	c.Assert(database.Migrate(ctx), qt.IsNil)
	client, _, _, err := jimmtest.SetupTestOFGAClient(c.Name())
	c.Assert(err, qt.IsNil)
	env := jimmtest.ParseEnvironment(c, metadataTestEnv)
	jimmTag := names.NewControllerTag("00000000-0000-0000-0000-000000000001")
	env.PopulateDBAndPermissions(c, jimmTag, database, client)

	manager := metadata.NewManager(database)
	jimm := &jimmtest.JIMM{MetadataManager_: func() jujuapi.MetadataManager { return manager }}
	model := names.NewModelTag(env.Models[0].UUID)
	otherModel := names.NewModelTag(env.Models[1].UUID)
	newRoot := func(email string) *jujuapi.ControllerRoot {
		identity := env.User(email).DBObject(c, database)
		root := jujuapi.NewControllerRoot(jimm, jujuapi.Params{})
		jujuapi.SetUser(root, openfga.NewUser(&identity, client))
		return root
	}

	const app = "application-test-app"
	return metadataFixture{
		db:       database,
		reader:   newRoot("reader@example.com"),
		writer:   newRoot("writer@example.com"),
		model:    params.EntityMetadataTarget{EntityTag: model.String()},
		app:      params.EntityMetadataTarget{EntityTag: app, ModelTag: model.String()},
		otherApp: params.EntityMetadataTarget{EntityTag: app, ModelTag: otherModel.String()},
	}
}

func (f metadataFixture) seed(c *qt.C, targets ...params.EntityMetadataTarget) {
	c.Helper()
	for _, target := range targets {
		entity, err := names.ParseTag(target.EntityTag)
		c.Assert(err, qt.IsNil)
		modelUUID := entity.Id()
		if target.ModelTag != "" {
			model, err := names.ParseModelTag(target.ModelTag)
			c.Assert(err, qt.IsNil)
			modelUUID = model.Id()
		}
		entry := dbmodel.EntityMetadataTarget{EntityType: entity.Kind(), Entity: entity.Id(), ModelUUID: modelUUID}
		c.Assert(f.db.UpsertEntityMetadata(c.Context(), entry, map[string]string{"deletable": "true"}), qt.IsNil)
	}
}

const metadataTestEnv = `
users:
- username: owner@example.com
- username: reader@example.com
- username: writer@example.com
clouds:
- name: test
  type: test
  regions:
  - name: test-region
cloud-credentials:
- name: test-cred
  cloud: test
  owner: owner@example.com
  type: empty
controllers:
- name: test
  uuid: 00000001-0000-0000-0000-000000000001
  cloud: test
  region: test-region
models:
- name: test-model
  uuid: 00000002-0000-0000-0000-000000000001
  owner: owner@example.com
  cloud: test
  region: test-region
  cloud-credential: test-cred
  controller: test
  users:
  - user: reader@example.com
    access: read
  - user: writer@example.com
    access: write
- name: other-model
  uuid: 00000002-0000-0000-0000-000000000002
  owner: owner@example.com
  cloud: test
  region: test-region
  cloud-credential: test-cred
  controller: test
  users:
  - user: writer@example.com
    access: admin
`
