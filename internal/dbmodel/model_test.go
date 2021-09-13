// Copyright 2020 Canonical Ltd.

package dbmodel_test

import (
	"database/sql"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
	jujuparams "github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
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

func TestDeleteModelRemovesMachinesAndApplications(t *testing.T) {
	c := qt.New(t)
	db := gormDB(c)
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

	// Check the applications and machines have been created.
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
		Applications: []dbmodel.Application{{
			Name:     "app-1",
			Exposed:  true,
			CharmURL: "ch:charm",
			Life:     "alive",
			MinUnits: 1,
			Constraints: dbmodel.Hardware{
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
			Hardware: dbmodel.Hardware{
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

func TestToJujuModelInfo(t *testing.T) {
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
		Applications: []dbmodel.Application{{
			Name:     "app-1",
			Exposed:  true,
			CharmURL: "ch:charm",
			Life:     "alive",
			MinUnits: 1,
			Constraints: dbmodel.Hardware{
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
			Hardware: dbmodel.Hardware{
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
			Username: u.Username,
			User:     u,
			Access:   "admin",
		}},
	}
	m.CloudRegion.Cloud = cl

	mi := m.ToJujuModelInfo()
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
		Applications: []dbmodel.Application{{
			Name:     "app-1",
			Exposed:  true,
			CharmURL: "ch:charm",
			Life:     "alive",
			MinUnits: 1,
			Constraints: dbmodel.Hardware{
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
			Hardware: dbmodel.Hardware{
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
			Username: u.Username,
			Access:   "admin",
		}},
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
		Name: "test-controller",
		UUID: "00000000-0000-0000-0000-0000-0000000000001",
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
		Machines: []dbmodel.Machine{{
			MachineID: "0",
			Hardware: dbmodel.Hardware{
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
			InstanceStatus: dbmodel.Status{
				Status: "running",
				Info:   "ACTIVE",
			},
		}},
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

func TestMachineFromJujuMachineInfo(t *testing.T) {
	c := qt.New(t)
	now := time.Now().UTC().Truncate(time.Millisecond)

	arch := "amd64"
	mem := uint64(8096)
	rootDisk := uint64(10240)
	info := jujuparams.MachineInfo{
		Id:         "0",
		InstanceId: "machine-0",
		AgentStatus: jujuparams.StatusInfo{
			Current: "idle",
			Message: "okay",
			Since:   &now,
			Version: "1.2.3",
		},
		InstanceStatus: jujuparams.StatusInfo{
			Current: "running",
			Message: "hello",
			Since:   &now,
			Version: "1.2.4",
		},
		Life:   "alive",
		Series: "warty",
		HardwareCharacteristics: &instance.HardwareCharacteristics{
			Arch:     &arch,
			Mem:      &mem,
			RootDisk: &rootDisk,
		},
		HasVote:   true,
		WantsVote: true,
	}

	machine := dbmodel.Machine{}
	machine.FromJujuMachineInfo(info)
	c.Assert(machine, qt.DeepEquals, dbmodel.Machine{
		MachineID: "0",
		Hardware: dbmodel.Hardware{
			Arch: sql.NullString{
				String: "amd64",
				Valid:  true,
			},
			Mem: dbmodel.NullUint64{
				Uint64: 8096,
				Valid:  true,
			},
			RootDisk: dbmodel.NullUint64{
				Uint64: 10240,
				Valid:  true,
			},
		},
		InstanceID: "machine-0",
		AgentStatus: dbmodel.Status{
			Status: "idle",
			Info:   "okay",
			Since: sql.NullTime{
				Time:  now,
				Valid: true,
			},
			Version: "1.2.3",
		},
		InstanceStatus: dbmodel.Status{
			Status: "running",
			Info:   "hello",
			Since: sql.NullTime{
				Time:  now,
				Valid: true,
			},
			Version: "1.2.4",
		},
		Life:      "alive",
		HasVote:   true,
		WantsVote: true,
		Series:    "warty",
	})
}

func TestApplicationFromJujuApplicationInfo(t *testing.T) {
	c := qt.New(t)
	now := time.Now().UTC().Truncate(time.Millisecond)

	arch := "amd64"
	mem := uint64(8096)
	rootDisk := uint64(10240)
	info := jujuparams.ApplicationInfo{
		Name:     "app-1",
		Exposed:  true,
		CharmURL: "cs:app-1",
		Life:     "alive",
		MinUnits: 3,
		Constraints: constraints.Value{
			Arch:     &arch,
			Mem:      &mem,
			RootDisk: &rootDisk,
		},
		Subordinate: false,
		Status: jujuparams.StatusInfo{
			Current: "idle",
			Message: "okay",
			Since:   &now,
			Version: "1.2.3",
		},
		WorkloadVersion: "12",
	}

	application := dbmodel.Application{}
	application.FromJujuApplicationInfo(info)
	c.Assert(application, qt.DeepEquals, dbmodel.Application{
		Name:     "app-1",
		Exposed:  true,
		CharmURL: "cs:app-1",
		Life:     "alive",
		MinUnits: 3,
		Constraints: dbmodel.Hardware{
			Arch: sql.NullString{
				String: "amd64",
				Valid:  true,
			},
			Mem: dbmodel.NullUint64{
				Uint64: 8096,
				Valid:  true,
			},
			RootDisk: dbmodel.NullUint64{
				Uint64: 10240,
				Valid:  true,
			},
		},
		Subordinate: false,
		Status: dbmodel.Status{
			Status: "idle",
			Info:   "okay",
			Since: sql.NullTime{
				Time:  now,
				Valid: true,
			},
			Version: "1.2.3",
		},
		WorkloadVersion: "12",
	})
}

func TestUnitFromJujuUnitInfo(t *testing.T) {
	c := qt.New(t)
	now := time.Now().UTC().Truncate(time.Millisecond)

	info := jujuparams.UnitInfo{
		Name:           "app-1/0",
		Application:    "app-1",
		Series:         "warty",
		CharmURL:       "cs:app-1",
		Life:           "alive",
		PublicAddress:  "public-address",
		PrivateAddress: "private-address",
		MachineId:      "0",
		Ports:          []jujuparams.Port{{Protocol: "tcp", Number: 8080}},
		PortRanges:     []jujuparams.PortRange{{FromPort: 8080, ToPort: 8090, Protocol: "tcp"}},
		Principal:      "Skinner",
		Subordinate:    false,
		WorkloadStatus: jujuparams.StatusInfo{
			Current: "idle",
			Message: "okay",
			Since:   &now,
			Version: "1.2.3",
		},
		AgentStatus: jujuparams.StatusInfo{
			Current: "running",
			Message: "hello",
			Since:   &now,
			Version: "1.2.4",
		},
	}

	unit := dbmodel.Unit{}
	unit.FromJujuUnitInfo(info)
	c.Assert(unit, qt.DeepEquals, dbmodel.Unit{
		Name:            "app-1/0",
		ApplicationName: "app-1",
		MachineID:       "0",
		Life:            "alive",
		PublicAddress:   "public-address",
		PrivateAddress:  "private-address",
		Ports:           dbmodel.Ports{{Protocol: "tcp", Number: 8080}},
		PortRanges:      dbmodel.PortRanges{{FromPort: 8080, ToPort: 8090, Protocol: "tcp"}},
		Principal:       "Skinner",
		WorkloadStatus: dbmodel.Status{
			Status: "idle",
			Info:   "okay",
			Since: sql.NullTime{
				Time:  now,
				Valid: true,
			},
			Version: "1.2.3",
		},
		AgentStatus: dbmodel.Status{
			Status: "running",
			Info:   "hello",
			Since: sql.NullTime{
				Time:  now,
				Valid: true,
			},
			Version: "1.2.4",
		},
	})
}
