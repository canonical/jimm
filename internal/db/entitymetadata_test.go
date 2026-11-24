// Copyright 2026 Canonical.

package db_test

import (
	"testing"

	qt "github.com/frankban/quicktest"
	"github.com/juju/names/v5"

	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
)

const entityMetadataEnv = `
users:
- username: owner@canonical.com
clouds:
- name: test-cloud
  type: test-provider
  regions:
  - name: test-region
cloud-credentials:
- name: test-cred
  cloud: test-cloud
  owner: owner@canonical.com
  type: empty
controllers:
- name: test-controller
  uuid: 00000001-0000-0000-0000-000000000001
  cloud: test-cloud
  region: test-region
models:
- name: test-model
  uuid: c4a0aa28-ce3d-4c4a-bbb6-aac649972266
  owner: owner@canonical.com
  cloud: test-cloud
  region: test-region
  cloud-credential: test-cred
  controller: test-controller
- name: other-model
  uuid: b3c58acd-73c5-41b6-847c-778f30216035
  owner: owner@canonical.com
  cloud: test-cloud
  region: test-region
  cloud-credential: test-cred
  controller: test-controller
`

func setupEntityMetadataDB(c *qt.C) (*db.Database, *jimmtest.Environment) {
	c.Helper()
	var s postgresSuite
	s.Init(c)
	c.Assert(s.Database.Migrate(c.Context()), qt.IsNil)
	env := jimmtest.ParseEnvironment(c, entityMetadataEnv)
	env.PopulateDB(c, s.Database)
	return s.Database, env
}

func seedEntityMetadata(c *qt.C, d *db.Database, target dbmodel.EntityMetadataTarget, metadata map[string]string) {
	c.Helper()
	for key, value := range metadata {
		row := dbmodel.EntityMetadata{EntityMetadataTarget: target, Key: key, Value: value}
		c.Assert(d.DB.WithContext(c.Context()).Create(&row).Error, qt.IsNil)
	}
}

func checkEntityMetadata(c *qt.C, d *db.Database, target dbmodel.EntityMetadataTarget, want map[string]string) {
	c.Helper()
	got, err := d.GetEntityMetadata(c.Context(), target)
	c.Assert(err, qt.IsNil)
	c.Check(got, qt.DeepEquals, want)
}

func TestGetEntityMetadata(t *testing.T) {
	c := qt.New(t)
	d, env := setupEntityMetadataDB(c)
	uuid := env.Models[0].UUID
	model := dbmodel.EntityMetadataTarget{EntityType: names.ModelTagKind, ModelUUID: uuid, Entity: uuid}
	app := dbmodel.EntityMetadataTarget{EntityType: names.ApplicationTagKind, ModelUUID: uuid, Entity: "test-app"}

	checkEntityMetadata(c, d, model, map[string]string{})
	seedEntityMetadata(c, d, model, map[string]string{"contact": "ops@example.com", "deletable": "false"})
	seedEntityMetadata(c, d, app, map[string]string{"contact": "app@example.com"})
	checkEntityMetadata(c, d, model, map[string]string{"contact": "ops@example.com", "deletable": "false"})
	checkEntityMetadata(c, d, app, map[string]string{"contact": "app@example.com"})
}

func TestUpsertEntityMetadata(t *testing.T) {
	c := qt.New(t)
	d, env := setupEntityMetadataDB(c)
	uuid := env.Models[0].UUID
	model := dbmodel.EntityMetadataTarget{EntityType: names.ModelTagKind, ModelUUID: uuid, Entity: uuid}

	c.Assert(d.UpsertEntityMetadata(t.Context(), model, map[string]string{"contact": "ops@example.com", "deletable": "false"}), qt.IsNil)
	checkEntityMetadata(c, d, model, map[string]string{"contact": "ops@example.com", "deletable": "false"})
	c.Assert(d.UpsertEntityMetadata(t.Context(), model, map[string]string{"deletable": "true"}), qt.IsNil)
	checkEntityMetadata(c, d, model, map[string]string{"contact": "ops@example.com", "deletable": "true"})

	missing := dbmodel.EntityMetadataTarget{EntityType: names.ApplicationTagKind, ModelUUID: "00000002-0000-0000-0000-000000000001", Entity: "test-app"}
	c.Check(d.UpsertEntityMetadata(t.Context(), missing, map[string]string{"product": "example"}), qt.Not(qt.IsNil))
}

func TestDeleteEntityMetadata(t *testing.T) {
	c := qt.New(t)
	d, env := setupEntityMetadataDB(c)
	uuid := env.Models[0].UUID
	model := dbmodel.EntityMetadataTarget{EntityType: names.ModelTagKind, ModelUUID: uuid, Entity: uuid}
	app := dbmodel.EntityMetadataTarget{EntityType: names.ApplicationTagKind, ModelUUID: uuid, Entity: "test-app"}
	seedEntityMetadata(c, d, model, map[string]string{"contact": "ops@example.com", "deletable": "true"})
	seedEntityMetadata(c, d, app, map[string]string{"contact": "app@example.com"})

	err := d.DeleteEntityMetadata(t.Context(), model, false, nil)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeBadRequest)
	c.Assert(d.DeleteEntityMetadata(t.Context(), model, false, []string{"contact", "missing"}), qt.IsNil)
	checkEntityMetadata(c, d, model, map[string]string{"deletable": "true"})
	c.Assert(d.DeleteEntityMetadata(t.Context(), model, true, nil), qt.IsNil)
	checkEntityMetadata(c, d, model, map[string]string{})
	checkEntityMetadata(c, d, app, map[string]string{"contact": "app@example.com"})
}

func TestQueryEntityMetadata(t *testing.T) {
	c := qt.New(t)
	d, env := setupEntityMetadataDB(c)
	uuid := env.Models[0].UUID
	model := dbmodel.EntityMetadataTarget{EntityType: names.ModelTagKind, ModelUUID: uuid, Entity: uuid}
	app := dbmodel.EntityMetadataTarget{EntityType: names.ApplicationTagKind, ModelUUID: uuid, Entity: "test-app"}
	siblingApp := dbmodel.EntityMetadataTarget{EntityType: names.ApplicationTagKind, ModelUUID: uuid, Entity: "sibling-app"}
	otherApp := dbmodel.EntityMetadataTarget{EntityType: names.ApplicationTagKind, ModelUUID: env.Models[1].UUID, Entity: app.Entity}
	seedEntityMetadata(c, d, model, map[string]string{"deletable": "true"})
	seedEntityMetadata(c, d, app, map[string]string{"deletable": "true", "description": ""})
	seedEntityMetadata(c, d, siblingApp, map[string]string{"deletable": "false"})
	seedEntityMetadata(c, d, otherApp, map[string]string{"deletable": "true"})

	scope := []string{uuid}
	rows, err := d.QueryEntityMetadata(t.Context(), scope, nil)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeBadRequest)
	c.Check(rows, qt.HasLen, 0)

	rows, err = d.QueryEntityMetadata(t.Context(), scope, &db.EntityMetadataMatcher{})
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeBadRequest)
	c.Check(rows, qt.HasLen, 0)

	matcher := &db.EntityMetadataMatcher{Key: "deletable", Value: "true"}
	rows, err = d.QueryEntityMetadata(t.Context(), scope, matcher)
	c.Assert(err, qt.IsNil)
	var targets []dbmodel.EntityMetadataTarget
	for _, row := range rows {
		targets = append(targets, row.EntityMetadataTarget)
		c.Check(row.Key, qt.Equals, matcher.Key)
		c.Check(row.Value, qt.Equals, matcher.Value)
	}
	c.Check(targets, qt.DeepEquals, []dbmodel.EntityMetadataTarget{app, model})

	rows, err = d.QueryEntityMetadata(t.Context(), scope, &db.EntityMetadataMatcher{Key: "description", Value: ""})
	c.Assert(err, qt.IsNil)
	c.Check(rows, qt.HasLen, 1)

	rows, err = d.QueryEntityMetadata(t.Context(), nil, matcher)
	c.Assert(err, qt.IsNil)
	c.Check(rows, qt.HasLen, 0)
}

func TestDeleteModelCascadesEntityMetadata(t *testing.T) {
	c := qt.New(t)
	d, env := setupEntityMetadataDB(c)
	model := env.Models[0].DBObject(c, d)
	uuid := model.UUID.String
	modelTarget := dbmodel.EntityMetadataTarget{EntityType: names.ModelTagKind, ModelUUID: uuid, Entity: uuid}
	app := dbmodel.EntityMetadataTarget{EntityType: names.ApplicationTagKind, ModelUUID: uuid, Entity: "test-app"}
	otherApp := dbmodel.EntityMetadataTarget{EntityType: names.ApplicationTagKind, ModelUUID: env.Models[1].UUID, Entity: app.Entity}
	seedEntityMetadata(c, d, modelTarget, map[string]string{"contact": "ops@example.com"})
	seedEntityMetadata(c, d, app, map[string]string{"product": "example"})
	seedEntityMetadata(c, d, otherApp, map[string]string{"product": "example"})

	c.Assert(d.DeleteModel(t.Context(), &model), qt.IsNil)
	checkEntityMetadata(c, d, modelTarget, map[string]string{})
	checkEntityMetadata(c, d, app, map[string]string{})
	checkEntityMetadata(c, d, otherApp, map[string]string{"product": "example"})
}
