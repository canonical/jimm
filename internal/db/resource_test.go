// Copyright 2024 Canonical.
package db_test

import (
	"context"
	"database/sql"

	qt "github.com/frankban/quicktest"
	"github.com/juju/juju/state"

	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/dbmodel"
)

func SetupDB(c *qt.C, database *db.Database) (dbmodel.Model, dbmodel.Controller, dbmodel.Cloud, dbmodel.Identity) {
	u, err := dbmodel.NewIdentity("bob@canonical.com")
	c.Assert(err, qt.IsNil)
	c.Assert(database.DB.Create(&u).Error, qt.IsNil)

	cloud := dbmodel.Cloud{
		Name: "test-cloud",
		Type: "test-provider",
		Regions: []dbmodel.CloudRegion{{
			Name: "test-region",
		}},
	}
	c.Assert(database.DB.Create(&cloud).Error, qt.IsNil)

	cred := dbmodel.CloudCredential{
		Name:     "test-cred",
		Cloud:    cloud,
		Owner:    *u,
		AuthType: "empty",
	}
	c.Assert(database.DB.Create(&cred).Error, qt.IsNil)

	controller := dbmodel.Controller{
		Name:        "test-controller",
		UUID:        "00000000-0000-0000-0000-0000-0000000000001",
		CloudName:   "test-cloud",
		CloudRegion: "test-region",
	}
	err = database.AddController(context.Background(), &controller)
	c.Assert(err, qt.Equals, nil)

	model := dbmodel.Model{
		Name: "test-model-1",
		UUID: sql.NullString{
			String: "00000001-0000-0000-0000-0000-000000000001",
			Valid:  true,
		},
		OwnerIdentityName: u.Name,
		ControllerID:      controller.ID,
		CloudRegionID:     cloud.Regions[0].ID,
		CloudCredentialID: cred.ID,
		Type:              "iaas",
		DefaultSeries:     "warty",
		Life:              state.Alive.String(),
		Status: dbmodel.Status{
			Status: "available",
			Since:  db.Now(),
		},
		SLA: dbmodel.SLA{
			Level: "unsupported",
		},
	}
	err = database.AddModel(context.Background(), &model)
	c.Assert(err, qt.Equals, nil)
	clientIDWithDomain := "abda51b2-d735-4794-a8bd-49c506baa4af@serviceaccount"
	sa, err := dbmodel.NewIdentity(clientIDWithDomain)
	c.Assert(err, qt.Equals, nil)
	err = database.GetIdentity(context.Background(), sa)
	return model, controller, cloud, *sa
}

func (s *dbSuite) TestGetResources(c *qt.C) {
	ctx := context.Background()
	err := s.Database.Migrate(context.Background(), true)
	c.Assert(err, qt.Equals, nil)
	res, err := s.Database.ListResources(ctx, 10, 0, "", "")
	c.Assert(err, qt.Equals, nil)
	c.Assert(res, qt.HasLen, 0)
	// create one model, one controller, one cloud
	model, controller, cloud, sva := SetupDB(c, s.Database)
	res, err = s.Database.ListResources(ctx, 10, 0, "", "")
	c.Assert(err, qt.Equals, nil)
	c.Assert(res, qt.HasLen, 4)
	for _, r := range res {
		switch r.Type {
		case "model":
			c.Assert(r.ID.String, qt.Equals, model.UUID.String)
			c.Assert(r.ParentId.String, qt.Equals, controller.UUID)
		case "controller":
			c.Assert(r.ID.String, qt.Equals, controller.UUID)
		case "cloud":
			c.Assert(r.ID.String, qt.Equals, cloud.Name)
		case "service_account":
			c.Assert(r.ID.String, qt.Equals, sva.Name)
		}
	}
}

func (s *dbSuite) TestGetResourcesWithNameTypeFilter(c *qt.C) {
	ctx := context.Background()
	err := s.Database.Migrate(context.Background(), true)
	c.Assert(err, qt.Equals, nil)
	// create one model, one controller, one cloud
	model, controller, cloud, sva := SetupDB(c, s.Database)

	tests := []struct {
		description   string
		nameFilter    string
		typeFilter    string
		limit         int
		offset        int
		expectedSize  int
		expectedUUIDs []string
	}{
		{
			description:   "filter on model name",
			nameFilter:    model.Name,
			limit:         10,
			offset:        0,
			typeFilter:    "",
			expectedSize:  1,
			expectedUUIDs: []string{model.UUID.String},
		},
		{
			description:   "filter name test",
			nameFilter:    "test",
			limit:         10,
			offset:        0,
			typeFilter:    "",
			expectedSize:  3,
			expectedUUIDs: []string{cloud.Name, controller.UUID, model.UUID.String},
		},
		{
			description:   "filter only models",
			nameFilter:    "test",
			limit:         10,
			offset:        0,
			typeFilter:    "models",
			expectedSize:  1,
			expectedUUIDs: []string{model.UUID.String},
		},
		{
			description:   "filter only service accounts",
			nameFilter:    "",
			limit:         10,
			offset:        0,
			typeFilter:    "identities",
			expectedSize:  1,
			expectedUUIDs: []string{sva.Name},
		},
		{
			description:   "filter only service accounts and name",
			nameFilter:    "not-found",
			limit:         10,
			offset:        0,
			typeFilter:    "identities",
			expectedSize:  0,
			expectedUUIDs: []string{},
		},
	}
	for _, t := range tests {
		c.Run(t.description, func(c *qt.C) {
			res, err := s.Database.ListResources(ctx, t.limit, t.offset, t.nameFilter, t.typeFilter)
			c.Assert(err, qt.Equals, nil)
			c.Assert(res, qt.HasLen, t.expectedSize)
			for i, r := range res {
				c.Assert(r.ID.String, qt.Equals, t.expectedUUIDs[i])
			}
		})
	}
}
