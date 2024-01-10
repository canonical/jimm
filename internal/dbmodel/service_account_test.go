// Copyright 2020 Canonical Ltd.

package dbmodel_test

import (
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
	"gorm.io/gorm"

	"github.com/canonical/jimm/internal/dbmodel"
)

func TestServiceAccount(t *testing.T) {
	c := qt.New(t)
	db := gormDB(c)

	var u0 dbmodel.ServiceAccount
	result := db.Where("client_id = ?", "some-service-account").First(&u0)
	c.Check(result.Error, qt.Equals, gorm.ErrRecordNotFound)

	sa1 := dbmodel.ServiceAccount{
		ClientID:    "some-service-account",
		DisplayName: "some-display-name",
	}
	result = db.Create(&sa1)
	c.Assert(result.Error, qt.IsNil)
	c.Check(result.RowsAffected, qt.Equals, int64(1))

	var sa2 dbmodel.ServiceAccount
	result = db.Where("client_id = ?", "some-service-account").First(&sa2)
	c.Assert(result.Error, qt.IsNil)
	c.Check(sa2, qt.DeepEquals, sa1)

	sa2.LastLogin.Time = time.Now().UTC().Round(time.Millisecond)
	sa2.LastLogin.Valid = true
	result = db.Save(&sa2)
	c.Assert(result.Error, qt.IsNil)
	var sa3 dbmodel.ServiceAccount
	result = db.Where("client_id = ?", "some-service-account").First(&sa3)
	c.Assert(result.Error, qt.IsNil)
	c.Check(sa3, qt.DeepEquals, sa2)

	sa4 := dbmodel.ServiceAccount{
		ClientID:    "some-service-account",
		DisplayName: "a-different-display-name",
	}
	result = db.Create(&sa4)
	c.Check(result.Error, qt.ErrorMatches, `.*violates unique constraint "service_accounts_client_id_key".*`)
}

func TestServiceAccountCloudCredentials(t *testing.T) {
	c := qt.New(t)
	db := gormDB(c)

	cl := dbmodel.Cloud{
		Name: "test-cloud",
	}
	result := db.Create(&cl)
	c.Assert(result.Error, qt.IsNil)

	sa := dbmodel.ServiceAccount{
		ClientID: "some-service-account",
	}
	result = db.Create(&sa)
	c.Assert(result.Error, qt.IsNil)

	cred1 := dbmodel.CloudCredential{
		Name:                "test-cred-1",
		Cloud:               cl,
		OwnerServiceAccount: &sa,
		AuthType:            "empty",
	}
	result = db.Create(&cred1)
	c.Assert(result.Error, qt.IsNil)

	cred2 := dbmodel.CloudCredential{
		Name:                "test-cred-2",
		Cloud:               cl,
		OwnerServiceAccount: &sa,
		AuthType:            "empty",
	}
	result = db.Create(&cred2)
	c.Assert(result.Error, qt.IsNil)

	var creds []dbmodel.CloudCredential
	err := db.Model(sa).Association("CloudCredentials").Find(&creds)
	c.Assert(err, qt.IsNil)
	c.Check(creds, qt.DeepEquals, []dbmodel.CloudCredential{{
		Model:         cred1.Model,
		Name:          cred1.Name,
		CloudName:     cred1.CloudName,
		OwnerClientID: cred1.OwnerClientID,
		AuthType:      cred1.AuthType,
	}, {
		Model:         cred2.Model,
		Name:          cred2.Name,
		CloudName:     cred2.CloudName,
		OwnerClientID: cred2.OwnerClientID,
		AuthType:      cred2.AuthType,
	}})
}
