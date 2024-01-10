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
	c.Check(result.Error, qt.ErrorMatches, `.*violates unique constraint "users_username_key".*`)
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
		Owner:    &u,
		AuthType: "empty",
	}
	result = db.Create(&cred1)
	c.Assert(result.Error, qt.IsNil)

	cred2 := dbmodel.CloudCredential{
		Name:     "test-cred-2",
		Cloud:    cl,
		Owner:    &u,
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
