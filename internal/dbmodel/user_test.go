// Copyright 2020 Canonical Ltd.

package dbmodel_test

import (
	"database/sql"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v4"
	"gorm.io/gorm"

	"github.com/canonical/jimm/internal/dbmodel"
)

func TestUser(t *testing.T) {
	c := qt.New(t)
	db := gormDB(c)

	var u0 dbmodel.User
	result := db.Where("username = ?", "bob@external").First(&u0)
	c.Check(result.Error, qt.Equals, gorm.ErrRecordNotFound)

	u1 := dbmodel.User{
		Username:    "bob@external",
		DisplayName: "bob",
	}
	result = db.Create(&u1)
	c.Assert(result.Error, qt.IsNil)
	c.Check(result.RowsAffected, qt.Equals, int64(1))

	var u2 dbmodel.User
	result = db.Where("username = ?", "bob@external").First(&u2)
	c.Assert(result.Error, qt.IsNil)
	c.Check(u2, qt.DeepEquals, u1)

	u2.LastLogin.Time = time.Now().UTC().Round(time.Millisecond)
	u2.LastLogin.Valid = true
	result = db.Save(&u2)
	c.Assert(result.Error, qt.IsNil)
	var u3 dbmodel.User
	result = db.Where("username = ?", "bob@external").First(&u3)
	c.Assert(result.Error, qt.IsNil)
	c.Check(u3, qt.DeepEquals, u2)

	u4 := dbmodel.User{
		Username:    "bob@external",
		DisplayName: "bob",
	}
	result = db.Create(&u4)
	c.Check(result.Error, qt.ErrorMatches, "UNIQUE constraint failed: users.username")
}

func TestUserTag(t *testing.T) {
	c := qt.New(t)

	u := dbmodel.User{
		Username: "bob@external",
	}
	tag := u.Tag()
	c.Check(tag.String(), qt.Equals, "user-bob@external")
	var u2 dbmodel.User
	u2.SetTag(tag.(names.UserTag))
	c.Check(u2, qt.DeepEquals, u)
}

func TestUserClouds(t *testing.T) {
	c := qt.New(t)

	db := gormDB(c)

	cl := dbmodel.Cloud{
		Name: "test-cloud",
		Users: []dbmodel.UserCloudAccess{{
			User: dbmodel.User{
				Username:    "bob@external",
				DisplayName: "bob",
			},
			Access: "add-model",
		}},
	}
	result := db.Create(&cl)
	c.Assert(result.Error, qt.IsNil)

	var u dbmodel.User
	result = db.Preload("Clouds").Where("username = ?", "bob@external").First(&u)
	c.Assert(result.Error, qt.IsNil)

	c.Assert(u.Clouds, qt.HasLen, 1)
	c.Check(u.Clouds[0].Username, qt.Equals, u.Username)
	c.Check(u.Clouds[0].CloudName, qt.Equals, cl.Name)
	c.Check(u.Clouds[0].Access, qt.Equals, "add-model")
}

func TestUserCloudCredentials(t *testing.T) {
	c := qt.New(t)
	db := gormDB(c)

	cl := dbmodel.Cloud{
		Name: "test-cloud",
	}
	result := db.Create(&cl)
	c.Assert(result.Error, qt.IsNil)

	u := dbmodel.User{
		Username: "bob@external",
	}
	result = db.Create(&u)
	c.Assert(result.Error, qt.IsNil)

	cred1 := dbmodel.CloudCredential{
		Name:     "test-cred-1",
		Cloud:    cl,
		Owner:    u,
		AuthType: "empty",
	}
	result = db.Create(&cred1)
	c.Assert(result.Error, qt.IsNil)

	cred2 := dbmodel.CloudCredential{
		Name:     "test-cred-2",
		Cloud:    cl,
		Owner:    u,
		AuthType: "empty",
	}
	result = db.Create(&cred2)
	c.Assert(result.Error, qt.IsNil)

	var creds []dbmodel.CloudCredential
	err := db.Model(u).Association("CloudCredentials").Find(&creds)
	c.Assert(err, qt.IsNil)
	c.Check(creds, qt.DeepEquals, []dbmodel.CloudCredential{{
		Model:         cred1.Model,
		Name:          cred1.Name,
		CloudName:     cred1.CloudName,
		OwnerUsername: cred1.OwnerUsername,
		AuthType:      cred1.AuthType,
	}, {
		Model:         cred2.Model,
		Name:          cred2.Name,
		CloudName:     cred2.CloudName,
		OwnerUsername: cred2.OwnerUsername,
		AuthType:      cred2.AuthType,
	}})
}

func TestUserModels(t *testing.T) {
	c := qt.New(t)
	db := gormDB(c)
	cl, cred, ctl, u := initModelEnv(c, db)

	m1 := dbmodel.Model{
		Name:  "test-model-1",
		Owner: u,
		UUID: sql.NullString{
			String: "00000001-0000-0000-0000-0000-000000000001",
			Valid:  true,
		},
		Controller:      ctl,
		CloudRegion:     cl.Regions[0],
		CloudCredential: cred,
		Users: []dbmodel.UserModelAccess{{
			User:   u,
			Access: "admin",
		}},
	}
	c.Assert(db.Create(&m1).Error, qt.IsNil)

	m2 := dbmodel.Model{
		Name:  "test-model-2",
		Owner: u,
		UUID: sql.NullString{
			String: "00000001-0000-0000-0000-0000-000000000002",
			Valid:  true,
		},
		Controller:      ctl,
		CloudRegion:     cl.Regions[0],
		CloudCredential: cred,
		Users: []dbmodel.UserModelAccess{{
			User:   u,
			Access: "write",
		}},
	}
	c.Assert(db.Create(&m2).Error, qt.IsNil)

	var models []dbmodel.UserModelAccess
	err := db.Model(&u).Preload("Model_").Association("Models").Find(&models)
	c.Assert(err, qt.IsNil)

	c.Check(models, qt.DeepEquals, []dbmodel.UserModelAccess{{
		Model:    m1.Users[0].Model,
		Username: u.Username,
		ModelID:  m1.ID,
		Model_: dbmodel.Model{
			ID:                m1.ID,
			CreatedAt:         m1.CreatedAt,
			UpdatedAt:         m1.UpdatedAt,
			Name:              m1.Name,
			OwnerUsername:     m1.OwnerUsername,
			UUID:              m1.UUID,
			ControllerID:      m1.ControllerID,
			CloudRegionID:     m1.CloudRegionID,
			CloudCredentialID: m1.CloudCredentialID,
		},
		Access: "admin",
	}, {
		Model:    m2.Users[0].Model,
		Username: u.Username,
		ModelID:  m2.ID,
		Model_: dbmodel.Model{
			ID:                m2.ID,
			CreatedAt:         m2.CreatedAt,
			UpdatedAt:         m2.UpdatedAt,
			Name:              m2.Name,
			OwnerUsername:     m2.OwnerUsername,
			UUID:              m2.UUID,
			ControllerID:      m2.ControllerID,
			CloudRegionID:     m2.CloudRegionID,
			CloudCredentialID: m2.CloudCredentialID,
		},
		Access: "write",
	}})
}

func TestUserApplicationOffers(t *testing.T) {
	c := qt.New(t)
	db := gormDB(c)
	cl, cred, ctl, u := initModelEnv(c, db)

	m := dbmodel.Model{
		Name:            "test-model",
		Owner:           u,
		Controller:      ctl,
		CloudRegion:     cl.Regions[0],
		CloudCredential: cred,
		Offers: []dbmodel.ApplicationOffer{{
			Name: "offer-1",
			UUID: "00000004-0000-0000-0000-0000-000000000001",
			Users: []dbmodel.UserApplicationOfferAccess{{
				User:   u,
				Access: "admin",
			}},
			ApplicationName: "app-1",
		}, {
			Name: "offer-1",
			UUID: "00000004-0000-0000-0000-0000-000000000002",
			Users: []dbmodel.UserApplicationOfferAccess{{
				User:   u,
				Access: "consume",
			}},
			ApplicationName: "app-2",
		}},
	}

	c.Assert(db.Create(&m).Error, qt.IsNil)

	var offers []dbmodel.UserApplicationOfferAccess
	err := db.Model(&u).Association("ApplicationOffers").Find(&offers)
	c.Assert(err, qt.IsNil)

	c.Check(offers, qt.DeepEquals, []dbmodel.UserApplicationOfferAccess{{
		Model: gorm.Model{
			ID:        m.Offers[0].Users[0].ID,
			CreatedAt: m.Offers[0].Users[0].CreatedAt,
			UpdatedAt: m.Offers[0].Users[0].UpdatedAt,
		},
		Username:           u.Username,
		ApplicationOfferID: m.Offers[0].ID,
		Access:             "admin",
	}, {
		Model: gorm.Model{
			ID:        m.Offers[1].Users[0].ID,
			CreatedAt: m.Offers[1].Users[0].CreatedAt,
			UpdatedAt: m.Offers[1].Users[0].UpdatedAt,
		},
		Username:           u.Username,
		ApplicationOfferID: m.Offers[1].ID,
		Access:             "consume",
	}})
}

func TestUserToJujuUserInfo(t *testing.T) {
	c := qt.New(t)

	u := dbmodel.User{
		Model: gorm.Model{
			CreatedAt: time.Now(),
		},
		Username:    "alice@external",
		DisplayName: "Alice",
	}
	ui := u.ToJujuUserInfo()
	c.Check(ui, qt.DeepEquals, jujuparams.UserInfo{
		Username:    "alice@external",
		DisplayName: "Alice",
		Access:      "",
		DateCreated: u.CreatedAt,
	})

	u.LastLogin = sql.NullTime{
		Time:  time.Now(),
		Valid: true,
	}
	ui = u.ToJujuUserInfo()
	c.Check(ui, qt.DeepEquals, jujuparams.UserInfo{
		Username:       "alice@external",
		DisplayName:    "Alice",
		Access:         "",
		DateCreated:    u.CreatedAt,
		LastConnection: &u.LastLogin.Time,
	})
}
