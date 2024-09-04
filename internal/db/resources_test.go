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

func (s *dbSuite) Setup(c *qt.C) (dbmodel.Model, dbmodel.Controller, dbmodel.Cloud) {
	err := s.Database.Migrate(context.Background(), true)
	c.Assert(err, qt.Equals, nil)

	u, err := dbmodel.NewIdentity("bob@canonical.com")
	c.Assert(err, qt.IsNil)
	c.Assert(s.Database.DB.Create(&u).Error, qt.IsNil)

	cloud := dbmodel.Cloud{
		Name: "test-cloud",
		Type: "test-provider",
		Regions: []dbmodel.CloudRegion{{
			Name: "test-region",
		}},
	}
	c.Assert(s.Database.DB.Create(&cloud).Error, qt.IsNil)

	cred := dbmodel.CloudCredential{
		Name:     "test-cred",
		Cloud:    cloud,
		Owner:    *u,
		AuthType: "empty",
	}
	c.Assert(s.Database.DB.Create(&cred).Error, qt.IsNil)

	controller := dbmodel.Controller{
		Name:        "test-controller",
		UUID:        "00000000-0000-0000-0000-0000-0000000000001",
		CloudName:   "test-cloud",
		CloudRegion: "test-region",
	}
	err = s.Database.AddController(context.Background(), &controller)
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
	err = s.Database.AddModel(context.Background(), &model)
	c.Assert(err, qt.Equals, nil)
	return model, controller, cloud
}

func (s *dbSuite) TestGetResources(c *qt.C) {
	// create one model, one controller, one cloud
	model, controller, cloud := s.Setup(c)
	ctx := context.Background()
	res, err := s.Database.ListResources(ctx, 10, 0)
	c.Assert(err, qt.Equals, nil)
	c.Assert(res, qt.HasLen, 3)
	for _, r := range res {
		switch r.Type {
		case "model":
			c.Assert(r.ID.String, qt.Equals, model.UUID.String)
			c.Assert(r.ParentId.String, qt.Equals, controller.UUID)
		case "controller":
			c.Assert(r.ID.String, qt.Equals, controller.UUID)
		case "cloud":
			c.Assert(r.ID.String, qt.Equals, cloud.Name)
		}
	}
}
