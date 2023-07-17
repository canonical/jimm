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
	c.Check(db.Create(&m2).Error, qt.ErrorMatches, `UNIQUE constraint failed: models.owner_username, models.name`)

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
			User:   u,
			Access: "admin",
		}},
	}
	c.Assert(db.Create(&m).Error, qt.IsNil)

	var m2 dbmodel.Model
	pdb := db.Preload("CloudRegion")
	pdb = pdb.Preload("CloudCredential").Preload("CloudCredential.Cloud").Preload("CloudCredential.Cloud.Regions").Preload("CloudCredential.Owner")
	pdb = pdb.Preload("Controller")
	pdb = pdb.Preload("Owner")
	pdb = pdb.Preload("Users").Preload("Users.User")
	c.Assert(pdb.First(&m2).Error, qt.IsNil)
	c.Check(m2, qt.DeepEquals, m)
}

func TestToJujuModel(t *testing.T) {
	c := qt.New(t)
	db := gormDB(c)
	cl, cred, ctl, u := initModelEnv(c, db)
	now := time.Now()
	m := dbmodel.Model{
		Name: "test-model",
		UUID: sql.NullString{
			String: "00000001-0000-0000-0000-0000-000000000001",
			Valid:  true,
		},
		OwnerUsername:   u.Username,
		Owner:           u,
		Controller:      ctl,
		CloudRegion:     cl.Regions[0],
		CloudCredential: cred,
		Type:            "iaas",
		IsController:    false,
		DefaultSeries:   "warty",
		Life:            "alive",
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
		Users: []dbmodel.UserModelAccess{{
			Username: u.Username,
			User:     u,
			Access:   "admin",
		}},
	}
	m.CloudRegion.Cloud = cl

	jm := m.ToJujuModel()
	c.Check(jm, qt.DeepEquals, jujuparams.Model{
		Name:     "test-model",
		UUID:     "00000001-0000-0000-0000-0000-000000000001",
		Type:     "iaas",
		OwnerTag: "user-bob@external",
	})
}

func TestUserModelAccessToJujuUserModel(t *testing.T) {
	c := qt.New(t)
	db := gormDB(c)
	cl, cred, ctl, u := initModelEnv(c, db)
	now := time.Now()
	m := dbmodel.Model{
		Name: "test-model",
		UUID: sql.NullString{
			String: "00000001-0000-0000-0000-0000-000000000001",
			Valid:  true,
		},
		OwnerUsername:   u.Username,
		Owner:           u,
		Controller:      ctl,
		CloudRegion:     cl.Regions[0],
		CloudCredential: cred,
		Type:            "iaas",
		IsController:    false,
		DefaultSeries:   "warty",
		Life:            "alive",
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
		Users: []dbmodel.UserModelAccess{{
			Username: u.Username,
			User:     u,
			Access:   "admin",
		}},
	}
	m.CloudRegion.Cloud = cl

	uma := dbmodel.UserModelAccess{
		User:   u,
		Model_: m,
		Access: "write",
		LastConnection: sql.NullTime{
			Time:  time.Now(),
			Valid: true,
		},
	}

	jum := uma.ToJujuUserModel()
	c.Check(jum, qt.DeepEquals, jujuparams.UserModel{
		Model: jujuparams.Model{
			Name:     "test-model",
			UUID:     "00000001-0000-0000-0000-0000-000000000001",
			Type:     "iaas",
			OwnerTag: "user-bob@external",
		},
		LastConnection: &uma.LastConnection.Time,
	})
}

func TestToJujuModelSummary(t *testing.T) {
	c := qt.New(t)
	db := gormDB(c)
	cl, cred, ctl, u := initModelEnv(c, db)
	now := time.Now()
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
		Life:            "alive",
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
		Users: []dbmodel.UserModelAccess{{
			Username: u.Username,
			Access:   "admin",
		}},
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
		CloudCredentialTag: "cloudcred-test-cloud_bob@external_test-cred",
		OwnerTag:           "user-bob@external",
		Life:               "alive",
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

func TestUserModelAccessToJujuModelSummary(t *testing.T) {
	c := qt.New(t)
	db := gormDB(c)
	cl, cred, ctl, u := initModelEnv(c, db)
	now := time.Now()
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
		Life:            "alive",
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
		Users: []dbmodel.UserModelAccess{{
			Username: u.Username,
			Access:   "admin",
		}},
		Machines: 1,
		Cores:    2,
		Units:    3,
	}
	m.CloudRegion.Cloud = cl

	uma := dbmodel.UserModelAccess{
		User:   u,
		Model_: m,
		Access: "write",
		LastConnection: sql.NullTime{
			Time:  time.Now(),
			Valid: true,
		},
	}

	ms := uma.ToJujuModelSummary()
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
		CloudCredentialTag: "cloudcred-test-cloud_bob@external_test-cred",
		OwnerTag:           "user-bob@external",
		Life:               "alive",
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
		UserAccess:         "write",
		UserLastConnection: &uma.LastConnection.Time,
	})
}

// initModelEnv initialises a controller, cloud and cloud-credential so
// that a model can be created.
func initModelEnv(c *qt.C, db *gorm.DB) (dbmodel.Cloud, dbmodel.CloudCredential, dbmodel.Controller, dbmodel.User) {
	u := dbmodel.User{
		Username: "bob@external",
	}
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
		Owner:    u,
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

	return cl, cred, ctl, u
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
		CloudCredentialTag:      "cloudcred-test-cloud_bob@external_test-cred",
		CloudCredentialValidity: nil,
		OwnerTag:                "user-bob@external",
		Life:                    "alive",
		Status: jujuparams.EntityStatus{
			Status: "available",
			Since:  &now,
		},
		Users: []jujuparams.ModelUserInfo{{
			UserName:    "bob@external",
			DisplayName: "Bobby The Tester",
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
			Owner: dbmodel.User{
				Username: "bob@external",
			},
		},
		OwnerUsername: "bob@external",
		Type:          "iaas",
		IsController:  false,
		DefaultSeries: "warty",
		Life:          "alive",
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
		Users: []dbmodel.UserModelAccess{{
			Access: "admin",
			User: dbmodel.User{
				Username:    "bob@external",
				DisplayName: "Bobby The Tester",
			},
		}},
	})
}

func TestModelFromJujuModelUpdate(t *testing.T) {
	c := qt.New(t)
	now := time.Now().UTC().Truncate(time.Millisecond)

	info := jujuparams.ModelUpdate{
		Name: "test-model",
		Life: "alive",
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
		Life: "alive",
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
