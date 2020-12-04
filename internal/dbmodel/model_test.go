// Copyright 2020 Canonical Ltd.

package dbmodel_test

import (
	"database/sql"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
	jujuparams "github.com/juju/juju/apiserver/params"
	"github.com/juju/names/v4"
	"gorm.io/gorm"

	"github.com/CanonicalLtd/jimm/internal/dbmodel"
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
	db := gormDB(t, &dbmodel.Model{}, &dbmodel.User{})
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
	c.Check(db.Create(&m2).Error, qt.ErrorMatches, `UNIQUE constraint failed: models.name, models.owner_id`)

	c.Assert(db.Delete(&m1).Error, qt.IsNil)
	c.Check(db.First(&m1).Error, qt.Equals, gorm.ErrRecordNotFound)
	c.Assert(db.Create(&m2).Error, qt.IsNil)
}

func TestDeleteModelRemovesMachinesAndApplications(t *testing.T) {
	c := qt.New(t)
	db := gormDB(t, &dbmodel.Application{}, &dbmodel.Machine{}, &dbmodel.Model{}, &dbmodel.User{})
	cl, cred, ctl, u := initModelEnv(c, db)

	m := dbmodel.Model{
		Name:  "test-model",
		Owner: u,
		UUID: sql.NullString{
			String: "00000001-0000-0000-0000-0000-000000000003",
			Valid:  true,
		},
		Controller:      ctl,
		CloudRegion:     cl.Regions[0],
		CloudCredential: cred,
		Applications: []dbmodel.Application{{
			Name: "app-1",
		}, {
			Name: "app-2",
		}},
		Machines: []dbmodel.Machine{{
			MachineID: "0",
		}, {
			MachineID: "1",
		}},
	}
	c.Assert(db.Create(&m).Error, qt.IsNil)

	// Sanity check the applications and machines have been created.
	var app1 dbmodel.Application
	c.Assert(db.First(&app1).Error, qt.IsNil)
	c.Check(app1.Name, qt.Equals, "app-1")
	var mach1 dbmodel.Machine
	c.Assert(db.First(&mach1).Error, qt.IsNil)
	c.Check(mach1.MachineID, qt.Equals, "0")

	c.Assert(db.Delete(&m).Error, qt.IsNil)

	c.Check(db.First(&dbmodel.Application{}).Error, qt.Equals, gorm.ErrRecordNotFound)
	c.Check(db.First(&dbmodel.Machine{}).Error, qt.Equals, gorm.ErrRecordNotFound)
}

func TestModel(t *testing.T) {
	c := qt.New(t)
	db := gormDB(c,
		&dbmodel.Application{},
		&dbmodel.Machine{},
		&dbmodel.Model{},
		&dbmodel.Unit{},
		&dbmodel.UserModelAccess{},
	)
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
		Applications: []dbmodel.Application{{
			Name:     "app-1",
			Exposed:  true,
			CharmURL: "ch:charm",
			Life:     "alive",
			MinUnits: 1,
			Constraints: dbmodel.Constraints{
				Arch: sql.NullString{
					String: "amd64",
					Valid:  true,
				},
			},
			Config: dbmodel.Map(map[string]interface{}{
				"a": "b",
			}),
			Subordinate: false,
			Status: dbmodel.Status{
				Status: "active",
				Info:   "Unit Is Ready",
				Since: sql.NullTime{
					Time:  time.Now(),
					Valid: true,
				},
			},
			WorkloadVersion: "1.0.0",
		}},
		Machines: []dbmodel.Machine{{
			MachineID: "0",
			Hardware: dbmodel.MachineHardware{
				Arch: sql.NullString{
					String: "amd64",
					Valid:  true,
				},
				Mem: dbmodel.NullUint64{
					Uint64: 2000,
					Valid:  true,
				},
			},
			InstanceID:  "test-machine-0",
			DisplayName: "Machine 0",
			AgentStatus: dbmodel.Status{
				Status: "started",
				Since: sql.NullTime{
					Time:  time.Now(),
					Valid: true,
				},
				Version: "1.0.0",
			},
			InstanceStatus: dbmodel.Status{
				Status: "running",
				Info:   "ACTIVE",
				Since: sql.NullTime{
					Time:  time.Now(),
					Valid: true,
				},
			},
			Series: "warty",
		}},
		Users: []dbmodel.UserModelAccess{{
			User:   u,
			Access: "admin",
		}},
	}
	c.Assert(db.Create(&m).Error, qt.IsNil)
	m.Applications[0].Units = []dbmodel.Unit{{
		Name:           "0",
		Machine:        m.Machines[0],
		Life:           "alive",
		PublicAddress:  "100.100.100.100",
		PrivateAddress: "10.10.10.10",
		Ports: dbmodel.Ports{{
			Protocol: "tcp",
			Number:   8080,
		}},
		PortRanges: dbmodel.PortRanges{{
			Protocol: "udp",
			FromPort: 9000,
			ToPort:   9090,
		}},
		Principal: "principal",
		WorkloadStatus: dbmodel.Status{
			Status: "active",
			Info:   "OK",
			Since: sql.NullTime{
				Time:  time.Now(),
				Valid: true,
			},
		},
		AgentStatus: dbmodel.Status{
			Status: "alive",
			Since: sql.NullTime{
				Time:  time.Now(),
				Valid: true,
			},
			Version: "1.0.0",
		},
	}}
	c.Assert(db.Save(&m).Error, qt.IsNil)
	c.Assert(db.Model(&m.Machines[0]).Association("Units").Find(&m.Machines[0].Units), qt.IsNil)
	c.Check(m.Machines[0].Units, qt.HasLen, 1)

	var m2 dbmodel.Model
	pdb := db.Preload("Applications").Preload("Applications.Units").Preload("Applications.Units.Machine")
	pdb = pdb.Preload("CloudRegion")
	pdb = pdb.Preload("CloudCredential").Preload("CloudCredential.Cloud").Preload("CloudCredential.Cloud.Regions").Preload("CloudCredential.Owner")
	pdb = pdb.Preload("Controller")
	pdb = pdb.Preload("Owner")
	pdb = pdb.Preload("Machines").Preload("Machines.Units")
	pdb = pdb.Preload("Users").Preload("Users.User")
	c.Assert(pdb.First(&m2).Error, qt.IsNil)
	c.Check(m2, qt.DeepEquals, m)
}

func TestWriteModelInfo(t *testing.T) {
	c := qt.New(t)
	db := gormDB(c,
		&dbmodel.Application{},
		&dbmodel.Machine{},
		&dbmodel.Model{},
		&dbmodel.Unit{},
		&dbmodel.UserModelAccess{},
	)
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
		Applications: []dbmodel.Application{{
			Name:     "app-1",
			Exposed:  true,
			CharmURL: "ch:charm",
			Life:     "alive",
			MinUnits: 1,
			Constraints: dbmodel.Constraints{
				Arch: sql.NullString{
					String: "amd64",
					Valid:  true,
				},
			},
			Config: dbmodel.Map(map[string]interface{}{
				"a": "b",
			}),
			Subordinate: false,
			Status: dbmodel.Status{
				Status: "active",
				Info:   "Unit Is Ready",
				Since: sql.NullTime{
					Time:  now,
					Valid: true,
				},
			},
			WorkloadVersion: "1.0.0",
		}},
		Machines: []dbmodel.Machine{{
			MachineID: "0",
			Hardware: dbmodel.MachineHardware{
				Arch: sql.NullString{
					String: "amd64",
					Valid:  true,
				},
				Mem: dbmodel.NullUint64{
					Uint64: 2000,
					Valid:  true,
				},
			},
			InstanceID:  "test-machine-0",
			DisplayName: "Machine 0",
			AgentStatus: dbmodel.Status{
				Status: "started",
				Since: sql.NullTime{
					Time:  now,
					Valid: true,
				},
				Version: "1.0.0",
			},
			InstanceStatus: dbmodel.Status{
				Status: "running",
				Info:   "ACTIVE",
				Since: sql.NullTime{
					Time:  now,
					Valid: true,
				},
			},
			Series: "warty",
		}},
		Users: []dbmodel.UserModelAccess{{
			User:   u,
			Access: "admin",
		}},
	}
	m.CloudRegion.Cloud = cl
	var mi jujuparams.ModelInfo
	m.WriteModelInfo(&mi)

	c.Check(mi, qt.DeepEquals, jujuparams.ModelInfo{
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
			UserName: "bob@external",
			Access:   "admin",
		}},
		Machines: []jujuparams.ModelMachineInfo{{
			Id: "0",
			Hardware: &jujuparams.MachineHardware{
				Arch: &m.Machines[0].Hardware.Arch.String,
				Mem:  &m.Machines[0].Hardware.Mem.Uint64,
			},
			InstanceId:  "test-machine-0",
			DisplayName: "Machine 0",
			Status:      "running",
			Message:     "ACTIVE",
		}},
		SLA: &jujuparams.ModelSLAInfo{
			Level: "unsupported",
		},
	})
}

func TestWriteModelSummary(t *testing.T) {
	c := qt.New(t)
	db := gormDB(c,
		&dbmodel.Application{},
		&dbmodel.Machine{},
		&dbmodel.Model{},
		&dbmodel.Unit{},
		&dbmodel.UserModelAccess{},
	)
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
		Applications: []dbmodel.Application{{
			Name:     "app-1",
			Exposed:  true,
			CharmURL: "ch:charm",
			Life:     "alive",
			MinUnits: 1,
			Constraints: dbmodel.Constraints{
				Arch: sql.NullString{
					String: "amd64",
					Valid:  true,
				},
			},
			Config: dbmodel.Map(map[string]interface{}{
				"a": "b",
			}),
			Subordinate: false,
			Status: dbmodel.Status{
				Status: "active",
				Info:   "Unit Is Ready",
				Since: sql.NullTime{
					Time:  now,
					Valid: true,
				},
			},
			WorkloadVersion: "1.0.0",
		}},
		Machines: []dbmodel.Machine{{
			MachineID: "0",
			Hardware: dbmodel.MachineHardware{
				Arch: sql.NullString{
					String: "amd64",
					Valid:  true,
				},
				Mem: dbmodel.NullUint64{
					Uint64: 2000,
					Valid:  true,
				},
			},
			InstanceID:  "test-machine-0",
			DisplayName: "Machine 0",
			AgentStatus: dbmodel.Status{
				Status: "started",
				Since: sql.NullTime{
					Time:  now,
					Valid: true,
				},
				Version: "1.0.0",
			},
			InstanceStatus: dbmodel.Status{
				Status: "running",
				Info:   "ACTIVE",
				Since: sql.NullTime{
					Time:  now,
					Valid: true,
				},
			},
			Series: "warty",
		}},
		Users: []dbmodel.UserModelAccess{{
			User:   u,
			Access: "admin",
		}},
	}
	m.CloudRegion.Cloud = cl
	var ms jujuparams.ModelSummary
	m.WriteModelSummary(&ms)

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
			Count:  0,
		}, {
			Entity: "units",
			Count:  0,
		}},
		SLA: &jujuparams.ModelSLAInfo{
			Level: "unsupported",
		},
	})
}

func TestApplicationOfferTag(t *testing.T) {
	c := qt.New(t)

	ao := dbmodel.ApplicationOffer{
		UUID: "00000003-0000-0000-0000-0000-000000000001",
	}

	tag := ao.Tag()
	c.Check(tag.String(), qt.Equals, "applicationoffer-00000003-0000-0000-0000-0000-000000000001")

	var ao2 dbmodel.ApplicationOffer
	ao2.SetTag(tag.(names.ApplicationOfferTag))
	c.Check(ao2, qt.DeepEquals, ao)
}

// initModelEnv initialises a controller, cloud and cloud-credential so
// that a model can be created.
func initModelEnv(c *qt.C, db *gorm.DB) (dbmodel.Cloud, dbmodel.CloudCredential, dbmodel.Controller, dbmodel.User) {
	err := db.AutoMigrate(
		&dbmodel.Cloud{},
		&dbmodel.CloudRegion{},
		&dbmodel.CloudCredential{},
		&dbmodel.Controller{},
		&dbmodel.User{},
	)
	c.Assert(err, qt.Equals, nil)

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
		Name: "test-controller",
		UUID: "00000000-0000-0000-0000-0000-0000000000001",
	}
	c.Assert(db.Create(&ctl).Error, qt.IsNil)

	return cl, cred, ctl, u
}
