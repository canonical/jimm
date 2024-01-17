// Copyright 2020 Canonical Ltd.

package dbmodel_test

import (
	"database/sql"
	"testing"

	apiparams "github.com/canonical/jimm/api/params"
	"github.com/canonical/jimm/internal/dbmodel"
	qt "github.com/frankban/quicktest"
	jujuparams "github.com/juju/juju/rpc/params"
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

	cl := dbmodel.Cloud{
		Name: "test-cloud",
	}
	result := db.Create(&cl)
	c.Assert(result.Error, qt.IsNil)

	ctl := dbmodel.Controller{
		Name:              "test-controller",
		UUID:              "00000000-0000-0000-0000-000000000001",
		AdminIdentityName: "admin",
		AdminPassword:     "pw",
		CACertificate:     "ca-cert",
		PublicAddress:     "controller.example.com:443",
		CloudName:         "test-cloud",
		Addresses: dbmodel.HostPorts([][]jujuparams.HostPort{{{
			Address: jujuparams.Address{
				Value: "1.1.1.1",
			},
			Port: 1,
		}}, {{
			Address: jujuparams.Address{
				Value: "2.2.2.2",
			},
			Port: 2,
		}}, {{
			Address: jujuparams.Address{
				Value: "3.3.3.3",
			},
			Port: 3,
		}}}),
	}
	result = db.Create(&ctl)
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

	u2 := dbmodel.Identity{
		Name: "charlie@external",
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
		OwnerIdentityName: m1.OwnerIdentityName,
		ControllerID:      m1.ControllerID,
		CloudRegionID:     m1.CloudRegionID,
		CloudCredentialID: m1.CloudCredentialID,
	}, {
		ID:                m2.ID,
		CreatedAt:         m2.CreatedAt,
		UpdatedAt:         m2.UpdatedAt,
		Name:              m2.Name,
		UUID:              m2.UUID,
		OwnerIdentityName: m2.OwnerIdentityName,
		ControllerID:      m2.ControllerID,
		CloudRegionID:     m2.CloudRegionID,
		CloudCredentialID: m2.CloudCredentialID,
	}})
}

func TestToAPIControllerInfo(t *testing.T) {
	c := qt.New(t)
	db := gormDB(c)
	cl, _, ctl, _ := initModelEnv(c, db)
	ctl.PublicAddress = "example.com:443"
	ctl.Addresses = dbmodel.HostPorts{
		{{
			Address: jujuparams.Address{
				Value: "1.1.1.1",
			},
			Port: 1,
		}},
		{{
			Address: jujuparams.Address{
				Value: "2.2.2.2",
			},
			Port: 2,
		}},
		{{
			Address: jujuparams.Address{
				Value: "3.3.3.3",
			},
			Port: 3,
		}},
	}
	ctl.CACertificate = "ca-cert"
	ctl.CloudRegions = []dbmodel.CloudRegionControllerPriority{{
		CloudRegion: cl.Regions[0],
		Priority:    dbmodel.CloudRegionControllerPriorityDeployed,
	}}
	ctl.AdminIdentityName = "admin"
	ctl.AgentVersion = "1.2.3"

	ci := ctl.ToAPIControllerInfo()
	c.Check(ci, qt.DeepEquals, apiparams.ControllerInfo{
		Name:          "test-controller",
		UUID:          "00000000-0000-0000-0000-0000-0000000000001",
		PublicAddress: "example.com:443",
		APIAddresses: []string{
			"1.1.1.1:1",
			"2.2.2.2:2",
			"3.3.3.3:3",
		},
		CACertificate: "ca-cert",
		CloudTag:      names.NewCloudTag("test-cloud").String(),
		CloudRegion:   "test-region",
		Username:      "admin",
		AgentVersion:  "1.2.3",
		Status: jujuparams.EntityStatus{
			Status: "available",
		},
	})
}

func TestToJujuRedirectInfoResult(t *testing.T) {
	c := qt.New(t)
	db := gormDB(c)
	_, _, ctl, _ := initModelEnv(c, db)
	ctl.PublicAddress = "example.com:443"
	ctl.Addresses = dbmodel.HostPorts{
		{{
			Address: jujuparams.Address{
				Value: "1.1.1.1",
			},
			Port: 1,
		}},
		{{
			Address: jujuparams.Address{
				Value: "2.2.2.2",
			},
			Port: 2,
		}},
		{{
			Address: jujuparams.Address{
				Value: "3.3.3.3",
			},
			Port: 3,
		}},
	}
	ctl.CACertificate = "ca-cert"

	ri := ctl.ToJujuRedirectInfoResult()
	c.Check(ri, qt.DeepEquals, jujuparams.RedirectInfoResult{
		Servers: [][]jujuparams.HostPort{
			{{Address: jujuparams.Address{Value: "example.com", Type: "hostname", Scope: "public"}, Port: 443}},
			{{Address: jujuparams.Address{Value: "1.1.1.1"}, Port: 1}},
			{{Address: jujuparams.Address{Value: "2.2.2.2"}, Port: 2}},
			{{Address: jujuparams.Address{Value: "3.3.3.3"}, Port: 3}},
		},
		CACert: "ca-cert",
	})
}

func TestControllerConfig(t *testing.T) {
	c := qt.New(t)
	db := gormDB(c)

	cfg := dbmodel.ControllerConfig{
		Name: "test-config",
		Config: map[string]interface{}{
			"key1": float64(1),
			"key2": "test-value-2",
			"key3": 42.0,
		},
	}
	result := db.Create(&cfg)
	c.Assert(result.Error, qt.IsNil)

	var cfg2 dbmodel.ControllerConfig
	result = db.Where("name = ?", "test-config").First(&cfg2)
	c.Assert(result.Error, qt.IsNil)

	c.Check(cfg2, qt.DeepEquals, cfg)
}
