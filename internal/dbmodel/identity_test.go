// Copyright 2020 Canonical Ltd.

package dbmodel_test

import (
	"database/sql"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v5"
	"gorm.io/gorm"

	"github.com/canonical/jimm/internal/dbmodel"
)

func TestIdentity(t *testing.T) {
	c := qt.New(t)
	db := gormDB(c)

	var u0 dbmodel.Identity
	result := db.Where("name = ?", "bob@canonical.com").First(&u0)
	c.Check(result.Error, qt.Equals, gorm.ErrRecordNotFound)

	u1 := dbmodel.Identity{
		Name:        "bob@canonical.com",
		DisplayName: "bob",
	}
	result = db.Create(&u1)
	c.Assert(result.Error, qt.IsNil)
	c.Check(result.RowsAffected, qt.Equals, int64(1))

	var u2 dbmodel.Identity
	result = db.Where("name = ?", "bob@canonical.com").First(&u2)
	c.Assert(result.Error, qt.IsNil)
	c.Check(u2, qt.DeepEquals, u1)

	u2.LastLogin.Time = time.Now().UTC().Round(time.Millisecond)
	u2.LastLogin.Valid = true
	result = db.Save(&u2)
	c.Assert(result.Error, qt.IsNil)
	var u3 dbmodel.Identity
	result = db.Where("name = ?", "bob@canonical.com").First(&u3)
	c.Assert(result.Error, qt.IsNil)
	c.Check(u3, qt.DeepEquals, u2)

	u4 := dbmodel.Identity{
		Name:        "bob@canonical.com",
		DisplayName: "bob",
	}
	result = db.Create(&u4)
	c.Check(result.Error, qt.ErrorMatches, `.*violates unique constraint "identities_name_key".*`)
}

func TestUserTag(t *testing.T) {
	c := qt.New(t)

	u := dbmodel.Identity{
		Name: "bob@canonical.com",
	}
	tag := u.Tag()
	c.Check(tag.String(), qt.Equals, "user-bob@canonical.com")
	var u2 dbmodel.Identity
	u2.SetTag(tag.(names.UserTag))
	c.Check(u2, qt.DeepEquals, u)
}

func TestIdentityCloudCredentials(t *testing.T) {
	c := qt.New(t)
	db := gormDB(c)

	cl := dbmodel.Cloud{
		Name: "test-cloud",
	}
	result := db.Create(&cl)
	c.Assert(result.Error, qt.IsNil)

	u := dbmodel.Identity{
		Name: "bob@canonical.com",
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
		Model:             cred1.Model,
		Name:              cred1.Name,
		CloudName:         cred1.CloudName,
		OwnerIdentityName: cred1.OwnerIdentityName,
		AuthType:          cred1.AuthType,
	}, {
		Model:             cred2.Model,
		Name:              cred2.Name,
		CloudName:         cred2.CloudName,
		OwnerIdentityName: cred2.OwnerIdentityName,
		AuthType:          cred2.AuthType,
	}})
}

func TestIdentityToJujuUserInfo(t *testing.T) {
	c := qt.New(t)

	u := dbmodel.Identity{
		Model: gorm.Model{
			CreatedAt: time.Now(),
		},
		Name:        "alice@canonical.com",
		DisplayName: "Alice",
	}
	ui := u.ToJujuUserInfo()
	c.Check(ui, qt.DeepEquals, jujuparams.UserInfo{
		Username:    "alice@canonical.com",
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
		Username:       "alice@canonical.com",
		DisplayName:    "Alice",
		Access:         "",
		DateCreated:    u.CreatedAt,
		LastConnection: &u.LastLogin.Time,
	})
}

func TestSanitiseIdentityId(t *testing.T) {
	c := qt.New(t)

	tests := []struct {
		about    string
		input    string
		expected string
	}{
		{
			about:    "catch all test",
			input:    "hi~!$%^&*_=}{'?@~!$%^&*_=}{'?bye.com",
			expected: "hi-------------861fcb@-------------bye.com",
		},
		{
			about:    "test bad email",
			input:    "alice_wonderland@bad_domain.com",
			expected: "alice-wonderland39cfd5@bad-domain.com",
		},
		{
			about:    "test good email",
			input:    "alice-wonderland@good-domain.com",
			expected: "alice-wonderland@good-domain.com",
		},
		{
			about:    "test good service account",
			input:    "fca1f605-736e-4d1f-bcd2-aecc726923be@serviceaccount",
			expected: "fca1f605-736e-4d1f-bcd2-aecc726923be@serviceaccount",
		},
		{
			about:    "test bad service account",
			input:    "fca1f605_736e_4d1f_bcd2_aecc726923be@serviceaccount",
			expected: "fca1f605-736e-4d1f-bcd2-aecc726923be28d4eb@serviceaccount",
		},
	}
	for _, tc := range tests {
		c.Run(tc.about, func(c *qt.C) {
			u := dbmodel.Identity{
				Model: gorm.Model{
					CreatedAt: time.Now(),
				},
				Name: tc.input,
			}

			u.SantiseIdentityId()

			c.Assert(u.Name, qt.Equals, tc.expected)
		})
	}
}

func TestSetDisplayName(t *testing.T) {
	c := qt.New(t)

	u := dbmodel.Identity{
		Model: gorm.Model{
			CreatedAt: time.Now(),
		},
		Name: "jimm-test@canonical.com",
	}

	u.SetDisplayName()

	c.Assert(u.DisplayName, qt.Equals, "jimm-test")
}
