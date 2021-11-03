// Copyright 2020 Canonical Ltd.

package db_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/CanonicalLtd/jimm/internal/db"
	"github.com/CanonicalLtd/jimm/internal/dbmodel"
	"github.com/CanonicalLtd/jimm/internal/errors"
)

func TestAddApplicationOfferUnconfiguredDatabase(t *testing.T) {
	c := qt.New(t)
	var d db.Database

	err := d.AddApplicationOffer(context.Background(), &dbmodel.ApplicationOffer{})
	c.Check(err, qt.ErrorMatches, `database not configured`)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeServerConfiguration)
}

type testEnvironment struct {
	u          dbmodel.User
	cloud      dbmodel.Cloud
	cred       dbmodel.CloudCredential
	controller dbmodel.Controller
	model      dbmodel.Model
}

func initTestEnvironment(c *qt.C, db *db.Database) testEnvironment {
	err := db.Migrate(context.Background(), true)
	c.Assert(err, qt.Equals, nil)

	env := testEnvironment{}

	env.u = dbmodel.User{
		Username: "bob@external",
	}
	c.Assert(db.DB.Create(&env.u).Error, qt.IsNil)

	env.cloud = dbmodel.Cloud{
		Name: "test-cloud",
		Type: "test-provider",
		Regions: []dbmodel.CloudRegion{{
			Name: "test-region",
		}},
	}
	c.Assert(db.DB.Create(&env.cloud).Error, qt.IsNil)

	env.cred = dbmodel.CloudCredential{
		Name:     "test-cred",
		Cloud:    env.cloud,
		Owner:    env.u,
		AuthType: "empty",
	}
	c.Assert(db.DB.Create(&env.cred).Error, qt.IsNil)

	env.controller = dbmodel.Controller{
		Name:        "test-controller",
		UUID:        "00000000-0000-0000-0000-0000-0000000000001",
		CloudName:   "test-cloud",
		CloudRegion: "test-region",
	}
	c.Assert(db.DB.Create(&env.controller).Error, qt.IsNil)

	env.model = dbmodel.Model{
		Name: "test-model",
		UUID: sql.NullString{
			String: "00000001-0000-0000-0000-0000-000000000001",
			Valid:  true,
		},
		Owner:           env.u,
		Controller:      env.controller,
		CloudRegion:     env.cloud.Regions[0],
		CloudCredential: env.cred,
		Type:            "iaas",
		IsController:    false,
		DefaultSeries:   "warty",
		Life:            "alive",
		Status: dbmodel.Status{
			Status: "available",
			Since: sql.NullTime{
				Time:  time.Now(),
				Valid: true,
			},
		},
		SLA: dbmodel.SLA{
			Level: "unsupported",
		},
		Users: []dbmodel.UserModelAccess{{
			User:   env.u,
			Access: "admin",
		}},
	}
	c.Assert(db.DB.Create(&env.model).Error, qt.IsNil)

	return env
}

func (s *dbSuite) TestAddApplicationOffer(c *qt.C) {
	env := initTestEnvironment(c, s.Database)

	offer := dbmodel.ApplicationOffer{
		UUID:            "00000000-0000-0000-0000-000000000001",
		Name:            "offer1",
		ModelID:         env.model.ID,
		ApplicationName: "app-1",
	}
	err := s.Database.AddApplicationOffer(context.Background(), &offer)
	c.Assert(err, qt.Equals, nil)

	var dbOffer dbmodel.ApplicationOffer
	result := s.Database.DB.Where("uuid = ?", offer.UUID).First(&dbOffer)
	c.Assert(result.Error, qt.Equals, nil)
	c.Assert(dbOffer, qt.DeepEquals, offer)
}

func TestGetApplicationOfferUnconfiguredDatabase(t *testing.T) {
	c := qt.New(t)

	var d db.Database
	err := d.GetApplicationOffer(context.Background(), &dbmodel.ApplicationOffer{})
	c.Check(err, qt.ErrorMatches, `database not configured`)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeServerConfiguration)
}

func (s *dbSuite) TestGetApplicationOffer(c *qt.C) {
	env := initTestEnvironment(c, s.Database)

	offer := dbmodel.ApplicationOffer{
		UUID:            "00000000-0000-0000-0000-000000000001",
		ModelID:         env.model.ID,
		ApplicationName: "app-1",
		Users: []dbmodel.UserApplicationOfferAccess{{
			Username: env.u.Username,
			User:     env.u,
			Access:   "admin",
		}},
		Endpoints:   []dbmodel.ApplicationOfferRemoteEndpoint{},
		Spaces:      []dbmodel.ApplicationOfferRemoteSpace{},
		Connections: []dbmodel.ApplicationOfferConnection{},
	}
	err := s.Database.AddApplicationOffer(context.Background(), &offer)
	c.Assert(err, qt.Equals, nil)

	dbOffer := dbmodel.ApplicationOffer{
		UUID: "00000000-0000-0000-0000-000000000001",
	}

	err = s.Database.GetApplicationOffer(context.Background(), &dbOffer)
	c.Assert(err, qt.Equals, nil)
	c.Assert(dbOffer, qt.CmpEquals(cmpopts.EquateEmpty(), cmpopts.IgnoreTypes(dbmodel.Model{})), offer)

	dbOffer = dbmodel.ApplicationOffer{
		UUID: "00000000-0000-0000-0000-000000000002",
	}
	err = s.Database.GetApplicationOffer(context.Background(), &dbOffer)
	c.Assert(errors.ErrorCode(err), qt.Equals, errors.CodeNotFound)
}

func (s *dbSuite) TestUpdateApplicationOffer(c *qt.C) {
	env := initTestEnvironment(c, s.Database)

	offer := dbmodel.ApplicationOffer{
		UUID:            "00000000-0000-0000-0000-000000000001",
		ModelID:         env.model.ID,
		ApplicationName: "app-1",
		Users:           []dbmodel.UserApplicationOfferAccess{},
		Endpoints:       []dbmodel.ApplicationOfferRemoteEndpoint{},
		Spaces:          []dbmodel.ApplicationOfferRemoteSpace{},
		Connections:     []dbmodel.ApplicationOfferConnection{},
	}
	err := s.Database.AddApplicationOffer(context.Background(), &offer)
	c.Assert(err, qt.Equals, nil)

	dbOffer := dbmodel.ApplicationOffer{
		UUID: "00000000-0000-0000-0000-000000000001",
	}
	err = s.Database.GetApplicationOffer(context.Background(), &dbOffer)
	c.Assert(err, qt.Equals, nil)
	c.Logf("%#v %#v", dbOffer.Users, offer.Users)
	c.Assert(dbOffer, qt.CmpEquals(cmpopts.EquateEmpty(), cmpopts.IgnoreTypes(dbmodel.Model{})), offer)

	offer1 := offer
	offer1.Users = []dbmodel.UserApplicationOfferAccess{{
		Username: env.u.Username,
		Access:   "read",
	}}
	err = s.Database.UpdateApplicationOffer(context.Background(), &offer1)
	c.Assert(err, qt.Equals, nil)

	dbOffer = dbmodel.ApplicationOffer{
		UUID: "00000000-0000-0000-0000-000000000001",
	}
	err = s.Database.GetApplicationOffer(context.Background(), &dbOffer)
	c.Assert(err, qt.Equals, nil)
	c.Assert(dbOffer, qt.DeepEquals, offer1)

	offer2 := offer
	offer2.Users = []dbmodel.UserApplicationOfferAccess{{
		Username: env.u.Username,
		Access:   "admin",
	}}
	err = s.Database.UpdateApplicationOffer(context.Background(), &offer2)
	c.Assert(err, qt.Equals, nil)

	err = s.Database.GetApplicationOffer(context.Background(), &dbOffer)
	c.Assert(err, qt.Equals, nil)
	c.Assert(dbOffer, qt.DeepEquals, offer2)

	offer3 := dbmodel.ApplicationOffer{
		UUID:            "00000000-0000-0000-0000-000000000002",
		ModelID:         env.model.ID,
		ApplicationName: "app-1",
	}
	err = s.Database.UpdateApplicationOffer(context.Background(), &offer3)
	c.Assert(err, qt.Not(qt.IsNil))
}

func (s *dbSuite) TestDeleteApplicationOffer(c *qt.C) {
	env := initTestEnvironment(c, s.Database)

	offer := dbmodel.ApplicationOffer{
		UUID:            "00000000-0000-0000-0000-000000000001",
		ModelID:         env.model.ID,
		ApplicationName: "app-1",
	}
	err := s.Database.AddApplicationOffer(context.Background(), &offer)
	c.Assert(err, qt.Equals, nil)

	err = s.Database.DeleteApplicationOffer(context.Background(), &offer)
	c.Assert(err, qt.Equals, nil)

	dbOffer := dbmodel.ApplicationOffer{
		UUID:  "00000000-0000-0000-0000-000000000001",
		Users: nil,
	}
	err = s.Database.GetApplicationOffer(context.Background(), &dbOffer)
	c.Assert(errors.ErrorCode(err), qt.Equals, errors.CodeNotFound)
}

func (s *dbSuite) TestFindApplicationOffers(c *qt.C) {
	env := initTestEnvironment(c, s.Database)

	offer1 := dbmodel.ApplicationOffer{
		UUID:                   "00000000-0000-0000-0000-000000000001",
		Name:                   "offer-1",
		ModelID:                env.model.ID,
		ApplicationName:        "app-1",
		ApplicationDescription: "this is a test application description",
		Users: []dbmodel.UserApplicationOfferAccess{{
			Username: env.u.Username,
			User:     env.u,
			Access:   "read",
		}},
		Endpoints: []dbmodel.ApplicationOfferRemoteEndpoint{{
			Name:      "test-endpoint-1",
			Role:      "provider",
			Interface: "http",
		}},
	}
	err := s.Database.AddApplicationOffer(context.Background(), &offer1)
	c.Assert(err, qt.Equals, nil)

	u := dbmodel.User{
		Username: "alice@external",
	}
	c.Assert(s.Database.DB.Create(&u).Error, qt.IsNil)

	offer2 := dbmodel.ApplicationOffer{
		UUID:                   "00000000-0000-0000-0000-000000000002",
		Name:                   "offer-2",
		ModelID:                env.model.ID,
		ApplicationName:        "app-1",
		ApplicationDescription: "this is another test offer",
		Users: []dbmodel.UserApplicationOfferAccess{{
			Username: env.u.Username,
			User:     env.u,
			Access:   "consume",
		}, {
			Username: u.Username,
			User:     u,
			Access:   "admin",
		}},
		Endpoints: []dbmodel.ApplicationOfferRemoteEndpoint{{
			Name:      "test-endpoint-2",
			Role:      "requirer",
			Interface: "db",
		}},
	}
	err = s.Database.AddApplicationOffer(context.Background(), &offer2)
	c.Assert(err, qt.Equals, nil)

	offer3 := dbmodel.ApplicationOffer{
		UUID:                   "00000000-0000-0000-0000-000000000003",
		Name:                   "test-3",
		ModelID:                env.model.ID,
		ApplicationName:        "app-1",
		ApplicationDescription: "this is yet another application offer",
		Users: []dbmodel.UserApplicationOfferAccess{{
			Username: u.Username,
			User:     u,
			Access:   "consume",
		}},
		Endpoints: []dbmodel.ApplicationOfferRemoteEndpoint{{
			Name:      "test-endpoint-3",
			Role:      "requirer",
			Interface: "http",
		}},
	}
	err = s.Database.AddApplicationOffer(context.Background(), &offer3)
	c.Assert(err, qt.Equals, nil)

	tests := []struct {
		about          string
		filters        []db.ApplicationOfferFilter
		expectedOffers []dbmodel.ApplicationOffer
		expectedError  string
	}{{
		about: "filter by offer name",
		filters: []db.ApplicationOfferFilter{
			db.ApplicationOfferFilterByName("offer-1"),
		},
		expectedOffers: []dbmodel.ApplicationOffer{offer1},
	}, {
		about: "filter by offer name - multiple found",
		filters: []db.ApplicationOfferFilter{
			db.ApplicationOfferFilterByName("offer"),
		},
		expectedOffers: []dbmodel.ApplicationOffer{offer1, offer2},
	}, {
		about: "filter by offer name - not found",
		filters: []db.ApplicationOfferFilter{
			db.ApplicationOfferFilterByName("no-such-offer"),
		},
		expectedOffers: []dbmodel.ApplicationOffer{},
	}, {
		about: "filter by application",
		filters: []db.ApplicationOfferFilter{
			db.ApplicationOfferFilterByApplication("app-1"),
		},
		expectedOffers: []dbmodel.ApplicationOffer{offer1, offer2, offer3},
	}, {
		about: "filter by application - not found",
		filters: []db.ApplicationOfferFilter{
			db.ApplicationOfferFilterByApplication("no such application"),
		},
		expectedOffers: []dbmodel.ApplicationOffer{},
	}, {
		about: "filter by model",
		filters: []db.ApplicationOfferFilter{
			db.ApplicationOfferFilterByModel(env.model.Name),
		},
		expectedOffers: []dbmodel.ApplicationOffer{offer1, offer2, offer3},
	}, {
		about: "filter by model - not found",
		filters: []db.ApplicationOfferFilter{
			db.ApplicationOfferFilterByModel("no such model"),
		},
		expectedOffers: []dbmodel.ApplicationOffer{},
	}, {
		about: "filter by description",
		filters: []db.ApplicationOfferFilter{
			db.ApplicationOfferFilterByDescription("description"),
		},
		expectedOffers: []dbmodel.ApplicationOffer{offer1},
	}, {
		about: "filter by description - multiple matches",
		filters: []db.ApplicationOfferFilter{
			db.ApplicationOfferFilterByDescription("test"),
		},
		expectedOffers: []dbmodel.ApplicationOffer{offer1, offer2},
	}, {
		about: "filter by description - not found",
		filters: []db.ApplicationOfferFilter{
			db.ApplicationOfferFilterByDescription("unknown"),
		},
		expectedOffers: []dbmodel.ApplicationOffer{},
	}, {
		about: "filter by user",
		filters: []db.ApplicationOfferFilter{
			db.ApplicationOfferFilterByUser(u.Username),
		},
		expectedOffers: []dbmodel.ApplicationOffer{offer2, offer3},
	}, {
		about: "filter by first user ",
		filters: []db.ApplicationOfferFilter{
			db.ApplicationOfferFilterByUser(env.u.Username),
		},
		expectedOffers: []dbmodel.ApplicationOffer{offer1, offer2},
	}, {
		about: "filter by user - not found",
		filters: []db.ApplicationOfferFilter{
			db.ApplicationOfferFilterByUser("no such user"),
		},
		expectedOffers: []dbmodel.ApplicationOffer{},
	}, {
		about: "filter by endpoint interface",
		filters: []db.ApplicationOfferFilter{
			db.ApplicationOfferFilterByEndpoint(dbmodel.ApplicationOfferRemoteEndpoint{
				Interface: "db",
			}),
		},
		expectedOffers: []dbmodel.ApplicationOffer{offer2},
	}, {
		about: "filter by endpoint interface - not found",
		filters: []db.ApplicationOfferFilter{
			db.ApplicationOfferFilterByEndpoint(dbmodel.ApplicationOfferRemoteEndpoint{
				Interface: "no-such-interface",
			}),
		},
		expectedOffers: []dbmodel.ApplicationOffer{},
	}, {
		about: "filter by endpoint role",
		filters: []db.ApplicationOfferFilter{
			db.ApplicationOfferFilterByEndpoint(dbmodel.ApplicationOfferRemoteEndpoint{
				Role: "provider",
			}),
		},
		expectedOffers: []dbmodel.ApplicationOffer{offer1},
	}, {
		about: "filter by endpoint role - not found",
		filters: []db.ApplicationOfferFilter{
			db.ApplicationOfferFilterByEndpoint(dbmodel.ApplicationOfferRemoteEndpoint{
				Role: "no-such-role",
			}),
		},
		expectedOffers: []dbmodel.ApplicationOffer{},
	}, {
		about: "filter by endpoint name",
		filters: []db.ApplicationOfferFilter{
			db.ApplicationOfferFilterByEndpoint(dbmodel.ApplicationOfferRemoteEndpoint{
				Name: "test-endpoint-2",
			}),
		},
		expectedOffers: []dbmodel.ApplicationOffer{offer2},
	}, {
		about: "filter by endpoint name - not found",
		filters: []db.ApplicationOfferFilter{
			db.ApplicationOfferFilterByEndpoint(dbmodel.ApplicationOfferRemoteEndpoint{
				Name: "no-such-endpoint",
			}),
		},
		expectedOffers: []dbmodel.ApplicationOffer{},
	}, {
		about: "filter by endpoint name and role",
		filters: []db.ApplicationOfferFilter{
			db.ApplicationOfferFilterByEndpoint(dbmodel.ApplicationOfferRemoteEndpoint{
				Name: "test-endpoint-2",
				Role: "requirer",
			}),
		},
		expectedOffers: []dbmodel.ApplicationOffer{offer2},
	}, {
		about: "filter by model and application",
		filters: []db.ApplicationOfferFilter{
			db.ApplicationOfferFilterByModel(env.model.Name),
			db.ApplicationOfferFilterByApplication("app-1"),
		},
		expectedOffers: []dbmodel.ApplicationOffer{offer1, offer2, offer3},
	}, {
		about: "filter by consumer",
		filters: []db.ApplicationOfferFilter{
			db.ApplicationOfferFilterByConsumer(u.Username),
		},
		expectedOffers: []dbmodel.ApplicationOffer{offer2, offer3},
	}, {
		about: "filter by user - not found",
		filters: []db.ApplicationOfferFilter{
			db.ApplicationOfferFilterByConsumer("no such user"),
		},
		expectedOffers: []dbmodel.ApplicationOffer{},
	}, {
		about: "filter by user and consumer",
		filters: []db.ApplicationOfferFilter{
			db.ApplicationOfferFilterByUser(u.Username),
			db.ApplicationOfferFilterByConsumer(u.Username),
		},
		expectedOffers: []dbmodel.ApplicationOffer{offer2, offer3},
	}, {
		about: "filter by consumer and endpoint",
		filters: []db.ApplicationOfferFilter{
			db.ApplicationOfferFilterByConsumer(u.Username),
			db.ApplicationOfferFilterByEndpoint(dbmodel.ApplicationOfferRemoteEndpoint{
				Role:      "requirer",
				Interface: "http",
			}),
		},
		expectedOffers: []dbmodel.ApplicationOffer{offer3},
	}}

	for _, test := range tests {
		c.Run(test.about, func(c *qt.C) {
			offers, err := s.Database.FindApplicationOffers(context.Background(), test.filters...)
			if test.expectedError == "" {
				c.Assert(err, qt.IsNil)
				c.Assert(offers, qt.CmpEquals(
					cmpopts.EquateEmpty(),
					cmpopts.IgnoreTypes(time.Time{}),
					cmpopts.IgnoreTypes(dbmodel.Model{}),
				), test.expectedOffers)
			} else {
				c.Assert(err, qt.ErrorMatches, test.expectedError)
			}
		})
	}
}
