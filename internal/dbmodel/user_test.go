// Copyright 2020 Canonical Ltd.

package dbmodel_test

import (
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
	"gorm.io/gorm"

	"github.com/CanonicalLtd/jimm/internal/dbmodel"
)

func TestUser(t *testing.T) {
	c := qt.New(t)
	db := gormDB(c, &dbmodel.User{})

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
	c.Check(u1.ControllerAccess, qt.Equals, "add-model")

	var u2 dbmodel.User
	result = db.Where("username = ?", "bob@external").First(&u2)
	c.Assert(result.Error, qt.IsNil)
	c.Check(u2, qt.DeepEquals, u1)

	u2.LastLogin = time.Now().UTC().Round(time.Millisecond)
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
	c.Check(u.Tag().String(), qt.Equals, "user-bob@external")
}
