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

	u0, err := dbmodel.NewIdentity("bob@canonical.com")
	c.Assert(err, qt.IsNil)
	result := db.Where("name = ?", "bob@canonical.com").First(&u0)
	c.Check(result.Error, qt.Equals, gorm.ErrRecordNotFound)

	u1, err := dbmodel.NewIdentity("bob@canonical.com")
	c.Assert(err, qt.IsNil)
	result = db.Create(u1)
	c.Assert(result.Error, qt.IsNil)
	c.Check(result.RowsAffected, qt.Equals, int64(1))

	u2, err := dbmodel.NewIdentity("bob@canonical.com")
	c.Assert(err, qt.IsNil)
	result = db.Where("name = ?", "bob@canonical.com").First(&u2)
	c.Assert(result.Error, qt.IsNil)
	c.Check(u2, qt.DeepEquals, u1)

	u2.LastLogin.Time = time.Now().UTC().Round(time.Millisecond)
	u2.LastLogin.Valid = true
	result = db.Save(&u2)
	c.Assert(result.Error, qt.IsNil)
	u3, err := dbmodel.NewIdentity("bob@canonical.com")
	c.Assert(err, qt.IsNil)
	result = db.Where("name = ?", "bob@canonical.com").First(&u3)
	c.Assert(result.Error, qt.IsNil)
	c.Check(u3, qt.DeepEquals, u2)

	u4, err := dbmodel.NewIdentity("bob@canonical.com")
	c.Assert(err, qt.IsNil)
	result = db.Create(&u4)
	c.Check(result.Error, qt.ErrorMatches, `.*violates unique constraint "identities_name_key".*`)
}

func TestUserTag(t *testing.T) {
	c := qt.New(t)

	u, err := dbmodel.NewIdentity("bob@canonical.com")
	c.Assert(err, qt.IsNil)
	tag := u.Tag()
	c.Check(tag.String(), qt.Equals, "user-bob@canonical.com")
	u2, err := dbmodel.NewIdentity("bob@canonical.com")
	c.Assert(err, qt.IsNil)
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

	u, err := dbmodel.NewIdentity("bob@canonical.com")
	c.Assert(err, qt.IsNil)
	result = db.Create(&u)
	c.Assert(result.Error, qt.IsNil)

	cred1 := dbmodel.CloudCredential{
		Name:     "test-cred-1",
		Cloud:    cl,
		Owner:    *u,
		AuthType: "empty",
	}
	result = db.Create(&cred1)
	c.Assert(result.Error, qt.IsNil)

	cred2 := dbmodel.CloudCredential{
		Name:     "test-cred-2",
		Cloud:    cl,
		Owner:    *u,
		AuthType: "empty",
	}
	result = db.Create(&cred2)
	c.Assert(result.Error, qt.IsNil)

	var creds []dbmodel.CloudCredential
	err = db.Model(u).Association("CloudCredentials").Find(&creds)
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

	u, err := dbmodel.NewIdentity("alice@canonical.com")
	c.Assert(err, qt.IsNil)
	u.Model = gorm.Model{
		CreatedAt: time.Now(),
	}

	ui := u.ToJujuUserInfo()
	c.Check(ui, qt.DeepEquals, jujuparams.UserInfo{
		Username:    "alice@canonical.com",
		DisplayName: "alice",
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
		DisplayName:    "alice",
		Access:         "",
		DateCreated:    u.CreatedAt,
		LastConnection: &u.LastLogin.Time,
	})
}

func TestNewIdentity(t *testing.T) {
	c := qt.New(t)

	tests := []struct {
		about                            string
		input                            string
		expectedSanitisedEmailOrClientId string
		expectedDisplayName              string
	}{
		{
			about:                            "catch all test",
			input:                            "hi~!$%^&*_=}{'?@~!$%^&*_=}{'?bye.com",
			expectedSanitisedEmailOrClientId: "hi-------------861fcb@-------------bye.com",
			expectedDisplayName:              "hi-------------861fcb",
		},
		{
			about:                            "test bad email",
			input:                            "alice_wonderland@bad_domain.com",
			expectedSanitisedEmailOrClientId: "alice-wonderland39cfd5@bad-domain.com",
			expectedDisplayName:              "alice-wonderland39cfd5",
		},
		{
			about:                            "test good email",
			input:                            "alice-wonderland@good-domain.com",
			expectedSanitisedEmailOrClientId: "alice-wonderland@good-domain.com",
			expectedDisplayName:              "alice-wonderland",
		},
		{
			about:                            "test good service account",
			input:                            "fca1f605-736e-4d1f-bcd2-aecc726923be@serviceaccount",
			expectedSanitisedEmailOrClientId: "fca1f605-736e-4d1f-bcd2-aecc726923be@serviceaccount",
			expectedDisplayName:              "fca1f605-736e-4d1f-bcd2-aecc726923be",
		},
		{
			about:                            "test bad service account",
			input:                            "fca1f605_736e_4d1f_bcd2_aecc726923be@serviceaccount",
			expectedSanitisedEmailOrClientId: "fca1f605-736e-4d1f-bcd2-aecc726923be28d4eb@serviceaccount",
			expectedDisplayName:              "fca1f605-736e-4d1f-bcd2-aecc726923be28d4eb",
		},
	}
	for _, tc := range tests {
		c.Run(tc.about, func(c *qt.C) {
			i, err := dbmodel.NewIdentity(tc.input)
			c.Assert(err, qt.IsNil)

			c.Assert(i.Name, qt.Equals, tc.expectedSanitisedEmailOrClientId)
			c.Assert(i.DisplayName, qt.Equals, tc.expectedDisplayName)
		})
	}
}
