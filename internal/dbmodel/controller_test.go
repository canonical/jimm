// Copyright 2020 Canonical Ltd.

package dbmodel_test

import (
	"database/sql"
	"testing"

	"github.com/canonical/jimm/internal/dbmodel"
	qt "github.com/frankban/quicktest"
	"github.com/juju/names/v4"
)

func TestControllerTag(t *testing.T) {
	c := qt.New(t)

	ctl := dbmodel.Controller{
		UUID: "11111111-2222-3333-4444-555555555555",
	}

	tag := ctl.Tag()
	c.Check(tag.String(), qt.Equals, "controller-11111111-2222-3333-4444-555555555555")

	var ctl2 dbmodel.Controller
	ctl2.SetTag(tag.(names.ControllerTag))
	c.Check(ctl2, qt.DeepEquals, ctl)
}

func TestController(t *testing.T) {
	c := qt.New(t)
	db := gormDB(c)

	ctl := dbmodel.Controller{
		Name:          "test-controller",
		UUID:          "00000000-0000-0000-0000-000000000001",
		AdminUser:     "admin",
		AdminPassword: "pw",
		CACertificate: "ca-cert",
		PublicAddress: "controller.example.com:443",
		Addresses:     dbmodel.Strings{"1.1.1.1:1", "2.2.2.2:2", "3.3.3.3:3"},
	}
	result := db.Create(&ctl)
	c.Assert(result.Error, qt.IsNil)

	var ctl2 dbmodel.Controller
	result = db.Where("name = ?", "test-controller").First(&ctl2)
	c.Assert(result.Error, qt.IsNil)

	c.Check(ctl2, qt.DeepEquals, ctl)
}

func TestControllerModels(t *testing.T) {
	c := qt.New(t)
	db := gormDB(c)
	cl, cred, ctl, u1 := initModelEnv(c, db)

	m1 := dbmodel.Model{
		Name: "test",
		UUID: sql.NullString{
			String: "00000001-0000-0000-0000-0000-000000000001",
			Valid:  true,
		},
		Owner:           u1,
		Controller:      ctl,
		CloudRegion:     cl.Regions[0],
		CloudCredential: cred,
	}
	c.Assert(db.Create(&m1).Error, qt.IsNil)

	u2 := dbmodel.User{
		Username: "charlie@external",
	}
	c.Assert(db.Create(&u2).Error, qt.IsNil)

	m2 := dbmodel.Model{
		Name: "test",
		UUID: sql.NullString{
			String: "00000001-0000-0000-0000-0000-000000000002",
			Valid:  true,
		},
		Owner:           u2,
		Controller:      ctl,
		CloudRegion:     cl.Regions[0],
		CloudCredential: cred,
	}
	c.Assert(db.Create(&m2).Error, qt.IsNil)

	var models []dbmodel.Model
	err := db.Model(&ctl).Association("Models").Find(&models)
	c.Assert(err, qt.IsNil)

	c.Check(models, qt.DeepEquals, []dbmodel.Model{{
		ID:                m1.ID,
		CreatedAt:         m1.CreatedAt,
		UpdatedAt:         m1.UpdatedAt,
		Name:              m1.Name,
		UUID:              m1.UUID,
		OwnerUsername:     m1.OwnerUsername,
		ControllerID:      m1.ControllerID,
		CloudRegionID:     m1.CloudRegionID,
		CloudCredentialID: m1.CloudCredentialID,
	}, {
		ID:                m2.ID,
		CreatedAt:         m2.CreatedAt,
		UpdatedAt:         m2.UpdatedAt,
		Name:              m2.Name,
		UUID:              m2.UUID,
		OwnerUsername:     m2.OwnerUsername,
		ControllerID:      m2.ControllerID,
		CloudRegionID:     m2.CloudRegionID,
		CloudCredentialID: m2.CloudCredentialID,
	}})
}
