// Copyright 2020 Canonical Ltd.

package dbmodel_test

import (
	"database/sql"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
	"github.com/juju/juju/core/life"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	"github.com/juju/names/v5"
	"gorm.io/gorm"

	"github.com/canonical/jimm/internal/dbmodel"
)

func TestModelTag(t *testing.T) {
	c := qt.New(t)

	m := dbmodel.Model{
		UUID: sql.NullString{
			String: "00000001-0000-0000-0000-0000-000000000000",
			Valid:  true,
		},
	}

	tag := m.Tag()
	c.Check(tag.String(), qt.Equals, "model-00000001-0000-0000-0000-0000-000000000000")

	var m2 dbmodel.Model
	m2.SetTag(tag.(names.ModelTag))

	c.Check(m2, qt.DeepEquals, m)
}

func TestRecreateDeletedModel(t *testing.T) {
	c := qt.New(t)
	db := gormDB(c)
	cl, cred, ctl, u := initModelEnv(c, db)

	m1 := dbmodel.Model{
		Owner:           u,
		Name:            "test-1",
		Controller:      ctl,
		CloudRegion:     cl.Regions[0],
		CloudCredential: cred,
	}
	c.Assert(db.Create(&m1).Error, qt.IsNil)

	m2 := dbmodel.Model{
		Owner:           u,
		Name:            "test-1",
		Controller:      ctl,
		CloudRegion:     cl.Regions[0],
		CloudCredential: cred,
	}
	c.Check(db.Create(&m2).Error, qt.ErrorMatches, `.*violates unique constraint "unique_model_names".*`)

	c.Assert(db.Delete(&m1).Error, qt.IsNil)
	c.Check(db.First(&m1).Error, qt.Equals, gorm.ErrRecordNotFound)
	c.Assert(db.Create(&m2).Error, qt.IsNil)
}

func TestModel(t *testing.T) {
	c := qt.New(t)
	db := gormDB(c)
	cl, cred, ctl, u := initModelEnv(c, db)

	m := dbmodel.Model{
		Name: "test-model",
		UUID: sql.NullString{
			String: "00000001-0000-0000-0000-0000-000000000001",
			Valid:  true,
		},
		Owner:           u,
		Controller:      ctl,
		CloudRegion:     cl.Regions[0],
		CloudCredential: cred,
		Type:            "iaas",
		IsController:    false,
		DefaultSeries:   "warty",
		Life:            state.Alive.String(),
		Status: dbmodel.Status{
			Status: "available",
			Since: sql.NullTime{
				Time:  time.Now().Truncate(time.Millisecond),
				Valid: true,
			},
		},
		SLA: dbmodel.SLA{
			Level: "unsupported",
		},
	}
	c.Assert(db.Create(&m).Error, qt.IsNil)

	var m2 dbmodel.Model
	pdb := db.Preload("CloudRegion")
	pdb = pdb.Preload("CloudCredential").Preload("CloudCredential.Cloud").Preload("CloudCredential.Cloud.Regions").Preload("CloudCredential.Owner")
	pdb = pdb.Preload("Controller")
	pdb = pdb.Preload("Owner")
	c.Assert(pdb.First(&m2).Error, qt.IsNil)
	c.Check(m2, qt.DeepEquals, m)
}

func TestModelUniqueConstraint(t *testing.T) {
	c := qt.New(t)
	db := gormDB(c)
	cl1, cred1, ctl1, u := initModelEnv(c, db)

	cl2 := dbmodel.Cloud{
		Name: "test-cloud-2",
		Type: "test-provider",
		Regions: []dbmodel.CloudRegion{{
			Name: "test-region-2",
		}},
	}
	c.Assert(db.Create(&cl2).Error, qt.IsNil)

	cred2 := dbmodel.CloudCredential{
		Name:     "test-cred-2",
		Cloud:    cl2,
		Owner:    u,
		AuthType: "empty",
	}
	c.Assert(db.Create(&cred2).Error, qt.IsNil)

	ctl2 := dbmodel.Controller{
		Name:        "test-controller-2",
		UUID:        "00000000-0000-0000-0000-0000-0000000000002",
		CloudName:   cl2.Name,
		CloudRegion: "test-region",
	}
	c.Assert(db.Create(&ctl2).Error, qt.IsNil)

	m1 := dbmodel.Model{
		Name: "staging",
		UUID: sql.NullString{
			String: "00000001-0000-0000-0000-0000-000000000001",
			Valid:  true,
		},
		Owner:           u,
		Controller:      ctl1,
		CloudRegion:     cl1.Regions[0],
		CloudCredential: cred1,
		Type:            "iaas",
		IsController:    false,
		DefaultSeries:   "warty",
		Life:            state.Alive.String(),
		Status: dbmodel.Status{
			Status: "available",
			Since: sql.NullTime{
				Time:  time.Now().Truncate(time.Millisecond),
				Valid: true,
			},
		},
		SLA: dbmodel.SLA{
			Level: "unsupported",
		},
	}
	c.Assert(db.Create(&m1).Error, qt.IsNil)

	m2 := dbmodel.Model{
		Name: "staging",
		UUID: sql.NullString{
			String: "00000001-0000-0000-0000-0000-000000000002",
			Valid:  true,
		},
		Owner:           u,
		Controller:      ctl2,
		CloudRegion:     cl2.Regions[0],
		CloudCredential: cred2,
		Type:            "iaas",
		IsController:    false,
		DefaultSeries:   "jammy",
		Life:            state.Alive.String(),
		Status: dbmodel.Status{
			Status: "available",
			Since: sql.NullTime{
				Time:  time.Now().Truncate(time.Millisecond),
				Valid: true,
			},
		},
		SLA: dbmodel.SLA{
			Level: "unsupported",
		},
	}
	c.Assert(db.Create(&m2).Error, qt.ErrorMatches, `ERROR: duplicate key value violates unique constraint .*`)

	m3 := dbmodel.Model{
		UUID: sql.NullString{
			String: "00000001-0000-0000-0000-0000-000000000001",
			Valid:  true,
		},
	}
	pdb := db.Preload("CloudRegion")
	pdb = pdb.Preload("CloudCredential").Preload("CloudCredential.Cloud").Preload("CloudCredential.Cloud.Regions").Preload("CloudCredential.Owner")
	pdb = pdb.Preload("Controller")
	pdb = pdb.Preload("Owner")
	c.Assert(pdb.First(&m3).Error, qt.IsNil)
	c.Check(m3, qt.DeepEquals, m1)
}

func TestToJujuModel(t *testing.T) {
	c := qt.New(t)
	db := gormDB(c)
	cl, cred, ctl, u := initModelEnv(c, db)
	now := time.Now().Truncate(time.Millisecond)
	m := dbmodel.Model{
		Name: "test-model",
		UUID: sql.NullString{
			String: "00000001-0000-0000-0000-0000-000000000001",
			Valid:  true,
		},
		OwnerIdentityName: u.Name,
		Owner:             u,
		Controller:        ctl,
		CloudRegion:       cl.Regions[0],
		CloudCredential:   cred,
		Type:              "iaas",
		IsController:      false,
		DefaultSeries:     "warty",
		Life:              state.Alive.String(),
		Status: dbmodel.Status{
			Status: "available",
			Since: sql.NullTime{
				Time:  now,
				Valid: true,
			},
		},
		SLA: dbmodel.SLA{
			Level: "unsupported",
		},
	}
	m.CloudRegion.Cloud = cl

	jm := m.ToJujuModel()
	c.Check(jm, qt.DeepEquals, jujuparams.Model{
		Name:     "test-model",
		UUID:     "00000001-0000-0000-0000-0000-000000000001",
		Type:     "iaas",
		OwnerTag: "user-bob@canonical.com",
	})
}

func TestToJujuModelSummary(t *testing.T) {
	c := qt.New(t)
	db := gormDB(c)
	cl, cred, ctl, u := initModelEnv(c, db)
	now := time.Now().Truncate(time.Millisecond)
	m := dbmodel.Model{
		Name: "test-model",
		UUID: sql.NullString{
			String: "00000001-0000-0000-0000-0000-000000000001",
			Valid:  true,
		},
		Owner:           u,
		Controller:      ctl,
		CloudRegion:     cl.Regions[0],
		CloudCredential: cred,
		Type:            "iaas",
		IsController:    false,
		DefaultSeries:   "warty",
		Life:            state.Alive.String(),
		Status: dbmodel.Status{
			Status: "available",
			Since: sql.NullTime{
				Time:  now,
				Valid: true,
			},
		},
		SLA: dbmodel.SLA{
			Level: "unsupported",
		},
		Machines: 1,
		Cores:    2,
		Units:    3,
	}
	m.CloudRegion.Cloud = cl

	ms := m.ToJujuModelSummary()
	c.Check(ms, qt.DeepEquals, jujuparams.ModelSummary{
		Name:               "test-model",
		Type:               "iaas",
		UUID:               "00000001-0000-0000-0000-0000-000000000001",
		ControllerUUID:     "00000000-0000-0000-0000-0000-0000000000001",
		IsController:       false,
		ProviderType:       "test-provider",
		DefaultSeries:      "warty",
		CloudTag:           "cloud-test-cloud",
		CloudRegion:        "test-region",
		CloudCredentialTag: "cloudcred-test-cloud_bob@canonical.com_test-cred",
		OwnerTag:           "user-bob@canonical.com",
		Life:               life.Value(state.Alive.String()),
		Status: jujuparams.EntityStatus{
			Status: "available",
			Since:  &now,
		},
		Counts: []jujuparams.ModelEntityCount{{
			Entity: "machines",
			Count:  1,
		}, {
			Entity: "cores",
			Count:  2,
		}, {
			Entity: "units",
			Count:  3,
		}},
		SLA: &jujuparams.ModelSLAInfo{
			Level: "unsupported",
		},
	})
}

// initModelEnv initialises a controller, cloud and cloud-credential so
// that a model can be created.
func initModelEnv(c *qt.C, db *gorm.DB) (dbmodel.Cloud, dbmodel.CloudCredential, dbmodel.Controller, dbmodel.Identity) {
	u, err := dbmodel.NewIdentity("bob@canonical.com")
	c.Assert(err, qt.IsNil)

	c.Assert(db.Create(&u).Error, qt.IsNil)

	cl := dbmodel.Cloud{
		Name: "test-cloud",
		Type: "test-provider",
		Regions: []dbmodel.CloudRegion{{
			Name: "test-region",
		}},
	}
	c.Assert(db.Create(&cl).Error, qt.IsNil)

	cred := dbmodel.CloudCredential{
		Name:     "test-cred",
		Cloud:    cl,
		Owner:    *u,
		AuthType: "empty",
	}
	c.Assert(db.Create(&cred).Error, qt.IsNil)

	ctl := dbmodel.Controller{
		Name:        "test-controller",
		UUID:        "00000000-0000-0000-0000-0000-0000000000001",
		CloudName:   cl.Name,
		CloudRegion: "test-region",
	}
	c.Assert(db.Create(&ctl).Error, qt.IsNil)

	return cl, cred, ctl, *u
}

func TestModelFromJujuModelInfo(t *testing.T) {
	c := qt.New(t)
	now := time.Now().UTC().Truncate(time.Millisecond)

	arch := "amd64"
	count := uint64(2000)
	modelInfo := jujuparams.ModelInfo{
		Name:                    "test-model",
		Type:                    "iaas",
		UUID:                    "00000001-0000-0000-0000-0000-000000000001",
		ControllerUUID:          "00000000-0000-0000-0000-0000-0000000000001",
		IsController:            false,
		ProviderType:            "test-provider",
		DefaultSeries:           "warty",
		CloudTag:                "cloud-test-cloud",
		CloudRegion:             "test-region",
		CloudCredentialTag:      "cloudcred-test-cloud_bob@canonical.com_test-cred",
		CloudCredentialValidity: nil,
		OwnerTag:                "user-bob@canonical.com",
		Life:                    life.Value(state.Alive.String()),
		Status: jujuparams.EntityStatus{
			Status: "available",
			Since:  &now,
		},
		Users: []jujuparams.ModelUserInfo{{
			UserName:    "bob@canonical.com",
			DisplayName: "bob",
			Access:      "admin",
		}},
		Machines: []jujuparams.ModelMachineInfo{{
			Id: "0",
			Hardware: &jujuparams.MachineHardware{
				Arch: &arch,
				Mem:  &count,
			},
			InstanceId:  "test-machine-0",
			DisplayName: "Machine 0",
			Status:      "running",
			Message:     "ACTIVE",
		}},
		SLA: &jujuparams.ModelSLAInfo{
			Level: "unsupported",
		},
	}

	model := dbmodel.Model{}
	err := model.FromJujuModelInfo(modelInfo)
	c.Assert(err, qt.IsNil)

	i, err := dbmodel.NewIdentity("bob@canonical.com")
	// We set display name to nothing, as when running from model info
	// you will never get a display name. The way we use FromJujuModelInfo is that
	// we get as much as we can from the model info, and fill in the bits of
	// the dbmodel.Model (like the identity) where we can. As such, this doesn't
	// need to be tested and doesn't make any sense.
	i.DisplayName = ""
	c.Assert(err, qt.IsNil)
	c.Assert(model, qt.DeepEquals, dbmodel.Model{
		Name: "test-model",
		UUID: sql.NullString{
			String: "00000001-0000-0000-0000-0000-000000000001",
			Valid:  true,
		},
		CloudRegion: dbmodel.CloudRegion{
			Name: "test-region",
			Cloud: dbmodel.Cloud{
				Name: "test-cloud",
			},
		},
		CloudCredential: dbmodel.CloudCredential{
			Name:      "test-cred",
			CloudName: "test-cloud",
			Owner:     *i,
		},
		OwnerIdentityName: "bob@canonical.com",
		Type:              "iaas",
		IsController:      false,
		DefaultSeries:     "warty",
		Life:              state.Alive.String(),
		Status: dbmodel.Status{
			Status: "available",
			Since: sql.NullTime{
				Time:  now,
				Valid: true,
			},
		},
		SLA: dbmodel.SLA{
			Level: "unsupported",
		},
	})
}

func TestModelFromJujuModelUpdate(t *testing.T) {
	c := qt.New(t)
	now := time.Now().UTC().Truncate(time.Millisecond)

	info := jujuparams.ModelUpdate{
		Name: "test-model",
		Life: life.Value(state.Alive.String()),
		Status: jujuparams.StatusInfo{
			Current: "available",
			Since:   &now,
		},
		SLA: jujuparams.ModelSLAInfo{
			Level: "unsupported",
		},
	}

	model := dbmodel.Model{}
	model.FromJujuModelUpdate(info)
	c.Assert(model, qt.DeepEquals, dbmodel.Model{
		Name: "test-model",
		Life: state.Alive.String(),
		Status: dbmodel.Status{
			Status: "available",
			Since: sql.NullTime{
				Time:  now,
				Valid: true,
			},
		},
		SLA: dbmodel.SLA{
			Level: "unsupported",
		},
	})
}
