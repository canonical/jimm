// Copyright 2016 Canonical Ltd.

package jujuapi_test

import (
	"context"
	"time"

	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/juju/juju/api/base"
	cloudapi "github.com/juju/juju/api/client/cloud"
	"github.com/juju/juju/api/client/modelmanager"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs/config"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing/factory"
	jujuversion "github.com/juju/juju/version"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v2"
	gc "gopkg.in/check.v1"

	"github.com/CanonicalLtd/jimm/internal/dbmodel"
	"github.com/CanonicalLtd/jimm/internal/jimmtest"
	"github.com/CanonicalLtd/jimm/internal/kubetest"
)

type modelManagerSuite struct {
	websocketSuite
}

var _ = gc.Suite(&modelManagerSuite{})

func (s *modelManagerSuite) TestListModelSummaries(c *gc.C) {
	conn := s.open(c, nil, "bob")
	defer conn.Close()

	// Add some machines and units to test the counts.
	s.Model.Machines = 1
	s.Model.Cores = 2
	s.Model.Units = 1
	ctx := context.Background()
	err := s.JIMM.Database.UpdateModel(ctx, s.Model)
	c.Assert(err, gc.Equals, nil)

	client := modelmanager.NewClient(conn)
	models, err := client.ListModelSummaries("bob", false)
	c.Assert(err, gc.Equals, nil)
	c.Assert(models, jimmtest.CmpEquals(cmpopts.IgnoreTypes(&time.Time{})), []base.UserModelSummary{{
		Name:            "model-1",
		UUID:            s.Model.UUID.String,
		ControllerUUID:  "914487b5-60e7-42bb-bd63-1adc3fd3a388",
		ProviderType:    jimmtest.TestProviderType,
		DefaultSeries:   "focal",
		Cloud:           jimmtest.TestCloudName,
		CloudRegion:     jimmtest.TestCloudRegionName,
		CloudCredential: jimmtest.TestCloudName + "/bob@external/cred",
		Owner:           "bob@external",
		Life:            "alive",
		Status: base.Status{
			Status: status.Available,
			Data:   map[string]interface{}{},
		},
		ModelUserAccess: "admin",
		Counts: []base.EntityCount{{
			Entity: "machines",
			Count:  1,
		}, {
			Entity: "cores",
			Count:  2,
		}, {
			Entity: "units",
			Count:  1,
		}},
		AgentVersion: &jujuversion.Current,
		Type:         "iaas",
		SLA: &base.SLASummary{
			Level: "unsupported",
		},
	}, {
		Name:            "model-3",
		UUID:            s.Model3.UUID.String,
		ControllerUUID:  "914487b5-60e7-42bb-bd63-1adc3fd3a388",
		ProviderType:    jimmtest.TestProviderType,
		DefaultSeries:   "focal",
		Cloud:           jimmtest.TestCloudName,
		CloudRegion:     jimmtest.TestCloudRegionName,
		CloudCredential: jimmtest.TestCloudName + "/charlie@external/cred",
		Owner:           "charlie@external",
		Life:            "alive",
		Status: base.Status{
			Status: status.Available,
			Data:   map[string]interface{}{},
		},
		ModelUserAccess: "read",
		Counts: []base.EntityCount{{
			Entity: "machines",
			Count:  0,
		}, {
			Entity: "cores",
			Count:  0,
		}, {
			Entity: "units",
			Count:  0,
		}},
		AgentVersion: &jujuversion.Current,
		Type:         "iaas",
		SLA: &base.SLASummary{
			Level: "unsupported",
		},
	}})
}

func (s *modelManagerSuite) TestListModelSummariesWithoutControllerUUIDMasking(c *gc.C) {
	conn1 := s.open(c, nil, "charlie")
	defer conn1.Close()
	err := conn1.APICall("JIMM", 2, "", "DisableControllerUUIDMasking", nil, nil)
	c.Assert(err, gc.ErrorMatches, `unauthorized \(unauthorized access\)`)

	s.Candid.AddUser("bob", "controller-admin")
	conn := s.open(c, nil, "bob")
	defer conn.Close()
	err = conn.APICall("JIMM", 2, "", "DisableControllerUUIDMasking", nil, nil)
	c.Assert(err, gc.Equals, nil)

	client := modelmanager.NewClient(conn)
	models, err := client.ListModelSummaries("bob", false)
	c.Assert(err, gc.Equals, nil)
	c.Assert(models, jimmtest.CmpEquals(cmpopts.IgnoreTypes(&time.Time{})), []base.UserModelSummary{{
		Name:            "model-1",
		UUID:            s.Model.UUID.String,
		ControllerUUID:  "deadbeef-1bad-500d-9000-4b1d0d06f00d",
		ProviderType:    jimmtest.TestProviderType,
		DefaultSeries:   "focal",
		Cloud:           jimmtest.TestCloudName,
		CloudRegion:     jimmtest.TestCloudRegionName,
		CloudCredential: jimmtest.TestCloudName + "/bob@external/cred",
		Owner:           "bob@external",
		Life:            "alive",
		Status: base.Status{
			Status: status.Available,
			Data:   map[string]interface{}{},
		},
		ModelUserAccess: "admin",
		Counts: []base.EntityCount{{
			Entity: "machines",
			Count:  0,
		}, {
			Entity: "cores",
			Count:  0,
		}, {
			Entity: "units",
			Count:  0,
		}},
		AgentVersion: &jujuversion.Current,
		Type:         "iaas",
		SLA: &base.SLASummary{
			Level: "unsupported",
		},
	}, {
		Name:            "model-3",
		UUID:            s.Model3.UUID.String,
		ControllerUUID:  "deadbeef-1bad-500d-9000-4b1d0d06f00d",
		ProviderType:    jimmtest.TestProviderType,
		DefaultSeries:   "focal",
		Cloud:           jimmtest.TestCloudName,
		CloudRegion:     jimmtest.TestCloudRegionName,
		CloudCredential: jimmtest.TestCloudName + "/charlie@external/cred",
		Owner:           "charlie@external",
		Life:            "alive",
		Status: base.Status{
			Status: status.Available,
			Data:   map[string]interface{}{},
		},
		ModelUserAccess: "read",
		Counts: []base.EntityCount{{
			Entity: "machines",
			Count:  0,
		}, {
			Entity: "cores",
			Count:  0,
		}, {
			Entity: "units",
			Count:  0,
		}},
		AgentVersion: &jujuversion.Current,
		Type:         "iaas",
		SLA: &base.SLASummary{
			Level: "unsupported",
		},
	}})
}

func (s *modelManagerSuite) TestListModels(c *gc.C) {
	conn := s.open(c, nil, "bob")
	defer conn.Close()

	client := modelmanager.NewClient(conn)
	models, err := client.ListModels("bob")
	c.Assert(err, gc.Equals, nil)
	c.Assert(models, jc.DeepEquals, []base.UserModel{{
		Name:  "model-1",
		UUID:  s.Model.UUID.String,
		Owner: "bob@external",
		Type:  "iaas",
	}, {
		Name:  "model-3",
		UUID:  s.Model3.UUID.String,
		Owner: "charlie@external",
		Type:  "iaas",
	}})
}

func (s *modelManagerSuite) TestModelInfo(c *gc.C) {
	mt4 := s.AddModel(c, names.NewUserTag("charlie@external"), "model-4", names.NewCloudTag(jimmtest.TestCloudName), jimmtest.TestCloudRegionName, s.Model2.CloudCredential.Tag().(names.CloudCredentialTag))
	conn := s.open(c, nil, "charlie")
	defer conn.Close()
	client := modelmanager.NewClient(conn)
	err := client.GrantModel("bob@external", "write", mt4.Id())
	c.Assert(err, gc.Equals, nil)

	mt5 := s.AddModel(c, names.NewUserTag("charlie@external"), "model-5", names.NewCloudTag(jimmtest.TestCloudName), jimmtest.TestCloudRegionName, s.Model2.CloudCredential.Tag().(names.CloudCredentialTag))
	err = client.GrantModel("bob@external", "admin", mt5.Id())
	c.Assert(err, gc.Equals, nil)

	// Add some machines to one of the models
	state, err := s.StatePool.Get(s.Model3.Tag().Id())
	c.Assert(err, gc.Equals, nil)
	f := factory.NewFactory(state.State, s.StatePool)
	f.MakeMachine(c, nil)
	f.MakeMachine(c, &factory.MachineParams{
		Characteristics: &instance.HardwareCharacteristics{
			Arch: newString("bbc-micro"),
		},
	})
	f.MakeMachine(c, nil)

	state, err = s.StatePool.Get(mt4.Id())
	c.Assert(err, gc.Equals, nil)
	f = factory.NewFactory(state.State, s.StatePool)
	f.MakeMachine(c, nil)
	f.MakeMachine(c, &factory.MachineParams{
		Characteristics: &instance.HardwareCharacteristics{
			Arch: newString("bbc-micro"),
		},
	})
	f.MakeMachine(c, nil)

	conn = s.open(c, nil, "bob")
	defer conn.Close()
	client = modelmanager.NewClient(conn)

	models, err := client.ModelInfo([]names.ModelTag{
		s.Model.Tag().(names.ModelTag),
		s.Model2.Tag().(names.ModelTag),
		s.Model3.Tag().(names.ModelTag),
		mt4,
		mt5,
		names.NewModelTag("00000000-0000-0000-0000-000000000007"),
	})
	c.Assert(err, gc.Equals, nil)

	for i := range models {
		if models[i].Result == nil {
			continue
		}
		for j := range models[i].Result.Machines {
			models[i].Result.Machines[j].InstanceId = ""
		}
	}
	assertModelInfo(c, models, []jujuparams.ModelInfoResult{{
		Result: &jujuparams.ModelInfo{
			Name:               "model-1",
			UUID:               s.Model.UUID.String,
			ControllerUUID:     "914487b5-60e7-42bb-bd63-1adc3fd3a388",
			ProviderType:       jimmtest.TestProviderType,
			CloudTag:           names.NewCloudTag(jimmtest.TestCloudName).String(),
			CloudRegion:        jimmtest.TestCloudRegionName,
			CloudCredentialTag: s.Model.CloudCredential.Tag().String(),
			OwnerTag:           names.NewUserTag("bob@external").String(),
			Life:               life.Alive,
			Status: jujuparams.EntityStatus{
				Status: status.Available,
			},
			SLA: &jujuparams.ModelSLAInfo{
				Level: "unsupported",
			},
			Users: []jujuparams.ModelUserInfo{{
				UserName:    "bob@external",
				DisplayName: "bob",
				Access:      jujuparams.ModelAdminAccess,
			}},
			AgentVersion:            &jujuversion.Current,
			Type:                    "iaas",
			CloudCredentialValidity: newBool(true),
			SupportedFeatures: []jujuparams.SupportedFeature{{
				Name:        "juju",
				Description: "the version of Juju used by the model",
				Version:     "2.9.33",
			}},
		},
	}, {
		Error: &jujuparams.Error{
			Message: "unauthorized",
			Code:    jujuparams.CodeUnauthorized,
		},
	}, {
		Result: &jujuparams.ModelInfo{
			Name:               "model-3",
			UUID:               s.Model3.UUID.String,
			ControllerUUID:     "914487b5-60e7-42bb-bd63-1adc3fd3a388",
			ProviderType:       jimmtest.TestProviderType,
			CloudTag:           names.NewCloudTag(jimmtest.TestCloudName).String(),
			CloudRegion:        jimmtest.TestCloudRegionName,
			CloudCredentialTag: s.Model3.CloudCredential.Tag().String(),
			OwnerTag:           names.NewUserTag("charlie@external").String(),
			Life:               life.Alive,
			Status: jujuparams.EntityStatus{
				Status: status.Available,
			},
			SLA: &jujuparams.ModelSLAInfo{
				Level: "unsupported",
			},
			Users: []jujuparams.ModelUserInfo{{
				UserName: "bob@external",
				Access:   jujuparams.ModelReadAccess,
			}},
			AgentVersion:            &jujuversion.Current,
			Type:                    "iaas",
			CloudCredentialValidity: newBool(true),
			SupportedFeatures: []jujuparams.SupportedFeature{{
				Name:        "juju",
				Description: "the version of Juju used by the model",
				Version:     "2.9.33",
			}},
		},
	}, {
		Result: &jujuparams.ModelInfo{
			Name:               "model-4",
			UUID:               mt4.Id(),
			ControllerUUID:     "914487b5-60e7-42bb-bd63-1adc3fd3a388",
			ProviderType:       jimmtest.TestProviderType,
			CloudTag:           names.NewCloudTag(jimmtest.TestCloudName).String(),
			CloudRegion:        jimmtest.TestCloudRegionName,
			CloudCredentialTag: s.Model2.CloudCredential.Tag().String(),
			OwnerTag:           names.NewUserTag("charlie@external").String(),
			Life:               life.Alive,
			Status: jujuparams.EntityStatus{
				Status: status.Available,
			},
			SLA: &jujuparams.ModelSLAInfo{
				Level: "unsupported",
			},
			Users: []jujuparams.ModelUserInfo{{
				UserName: "bob@external",
				Access:   jujuparams.ModelWriteAccess,
			}},
			Machines: []jujuparams.ModelMachineInfo{{
				Id: "0",
				Hardware: &jujuparams.MachineHardware{
					Arch: newString("amd64"),
					Mem:  newUint64(64 * 1024 * 1024 * 1024),
				},
				Status: "pending",
			}, {
				Id: "1",
				Hardware: &jujuparams.MachineHardware{
					Arch: newString("bbc-micro"),
				},
				Status: "pending",
			}, {
				Id: "2",
				Hardware: &jujuparams.MachineHardware{
					Arch: newString("amd64"),
					Mem:  newUint64(64 * 1024 * 1024 * 1024),
				},
				Status: "pending",
			}},
			AgentVersion:            &jujuversion.Current,
			Type:                    "iaas",
			CloudCredentialValidity: newBool(true),
			SupportedFeatures: []jujuparams.SupportedFeature{{
				Name:        "juju",
				Description: "the version of Juju used by the model",
				Version:     "2.9.33",
			}},
		},
	}, {
		Result: &jujuparams.ModelInfo{
			Name:               "model-5",
			UUID:               mt5.Id(),
			ControllerUUID:     "914487b5-60e7-42bb-bd63-1adc3fd3a388",
			ProviderType:       jimmtest.TestProviderType,
			CloudTag:           names.NewCloudTag(jimmtest.TestCloudName).String(),
			CloudRegion:        jimmtest.TestCloudRegionName,
			CloudCredentialTag: s.Model2.CloudCredential.Tag().String(),
			OwnerTag:           names.NewUserTag("charlie@external").String(),
			Life:               life.Alive,
			Status: jujuparams.EntityStatus{
				Status: status.Available,
			},
			SLA: &jujuparams.ModelSLAInfo{
				Level: "unsupported",
			},
			Users: []jujuparams.ModelUserInfo{{
				UserName: "bob@external",
				Access:   jujuparams.ModelAdminAccess,
			}, {
				UserName:    "charlie@external",
				DisplayName: "charlie",
				Access:      jujuparams.ModelAdminAccess,
			}},
			AgentVersion:            &jujuversion.Current,
			Type:                    "iaas",
			CloudCredentialValidity: newBool(true),
			SupportedFeatures: []jujuparams.SupportedFeature{{
				Name:        "juju",
				Description: "the version of Juju used by the model",
				Version:     "2.9.33",
			}},
		},
	}, {
		Error: &jujuparams.Error{
			Message: `unauthorized`,
			Code:    jujuparams.CodeUnauthorized,
		},
	}})
}

func (s *modelManagerSuite) TestModelInfoDisableControllerUUIDMasking(c *gc.C) {
	mt4 := s.AddModel(c, names.NewUserTag("charlie@external"), "model-4", names.NewCloudTag(jimmtest.TestCloudName), jimmtest.TestCloudRegionName, s.Model2.CloudCredential.Tag().(names.CloudCredentialTag))

	mt5 := s.AddModel(c, names.NewUserTag("charlie@external"), "model-5", names.NewCloudTag(jimmtest.TestCloudName), jimmtest.TestCloudRegionName, s.Model2.CloudCredential.Tag().(names.CloudCredentialTag))

	// Add some machines to one of the models
	state, err := s.StatePool.Get(s.Model3.Tag().Id())
	c.Assert(err, gc.Equals, nil)
	f := factory.NewFactory(state.State, s.StatePool)
	f.MakeMachine(c, nil)
	f.MakeMachine(c, &factory.MachineParams{
		Characteristics: &instance.HardwareCharacteristics{
			Arch: newString("bbc-micro"),
		},
	})
	f.MakeMachine(c, nil)

	state, err = s.StatePool.Get(mt4.Id())
	c.Assert(err, gc.Equals, nil)
	f = factory.NewFactory(state.State, s.StatePool)
	f.MakeMachine(c, nil)
	f.MakeMachine(c, &factory.MachineParams{
		Characteristics: &instance.HardwareCharacteristics{
			Arch: newString("bbc-micro"),
		},
	})
	f.MakeMachine(c, nil)

	s.Candid.AddUser("bob", "controller-admin")
	conn := s.open(c, nil, "bob")
	defer conn.Close()
	client := modelmanager.NewClient(conn)

	err = conn.APICall("JIMM", 2, "", "DisableControllerUUIDMasking", nil, nil)
	c.Assert(err, gc.Equals, nil)

	models, err := client.ModelInfo([]names.ModelTag{
		s.Model.Tag().(names.ModelTag),
		s.Model2.Tag().(names.ModelTag),
		s.Model3.Tag().(names.ModelTag),
		mt4,
		mt5,
		names.NewModelTag("00000000-0000-0000-0000-000000000007"),
	})
	c.Assert(err, gc.Equals, nil)

	for i := range models {
		if models[i].Result == nil {
			continue
		}
		for j := range models[i].Result.Machines {
			models[i].Result.Machines[j].InstanceId = ""
		}
	}
	assertModelInfo(c, models, []jujuparams.ModelInfoResult{{
		Result: &jujuparams.ModelInfo{
			Name:               "model-1",
			UUID:               s.Model.UUID.String,
			ControllerUUID:     "deadbeef-1bad-500d-9000-4b1d0d06f00d",
			ProviderType:       jimmtest.TestProviderType,
			CloudTag:           names.NewCloudTag(jimmtest.TestCloudName).String(),
			CloudRegion:        jimmtest.TestCloudRegionName,
			CloudCredentialTag: s.Model.CloudCredential.Tag().String(),
			OwnerTag:           s.Model.Owner.Tag().String(),
			Life:               life.Alive,
			Status: jujuparams.EntityStatus{
				Status: status.Available,
			},
			SLA: &jujuparams.ModelSLAInfo{
				Level: "unsupported",
			},
			Users: []jujuparams.ModelUserInfo{{
				UserName:    "bob@external",
				DisplayName: "bob",
				Access:      jujuparams.ModelAdminAccess,
			}},
			AgentVersion:            &jujuversion.Current,
			Type:                    "iaas",
			CloudCredentialValidity: newBool(true),
			SupportedFeatures: []jujuparams.SupportedFeature{{
				Name:        "juju",
				Description: "the version of Juju used by the model",
				Version:     "2.9.33",
			}},
		},
	}, {
		Result: &jujuparams.ModelInfo{
			Name:               "model-2",
			UUID:               s.Model2.UUID.String,
			ControllerUUID:     "deadbeef-1bad-500d-9000-4b1d0d06f00d",
			ProviderType:       jimmtest.TestProviderType,
			CloudTag:           names.NewCloudTag(jimmtest.TestCloudName).String(),
			CloudRegion:        jimmtest.TestCloudRegionName,
			CloudCredentialTag: s.Model2.CloudCredential.Tag().String(),
			OwnerTag:           s.Model2.Owner.Tag().String(),
			Life:               life.Alive,
			Status: jujuparams.EntityStatus{
				Status: status.Available,
			},
			SLA: &jujuparams.ModelSLAInfo{
				Level: "unsupported",
			},
			Users: []jujuparams.ModelUserInfo{{
				UserName:    "charlie@external",
				DisplayName: "charlie",
				Access:      jujuparams.ModelAdminAccess,
			}},
			AgentVersion:            &jujuversion.Current,
			Type:                    "iaas",
			CloudCredentialValidity: newBool(true),
			SupportedFeatures: []jujuparams.SupportedFeature{{
				Name:        "juju",
				Description: "the version of Juju used by the model",
				Version:     "2.9.33",
			}},
		},
	}, {
		Result: &jujuparams.ModelInfo{
			Name:               "model-3",
			UUID:               s.Model3.UUID.String,
			ControllerUUID:     "deadbeef-1bad-500d-9000-4b1d0d06f00d",
			ProviderType:       jimmtest.TestProviderType,
			CloudTag:           names.NewCloudTag(jimmtest.TestCloudName).String(),
			CloudRegion:        jimmtest.TestCloudRegionName,
			CloudCredentialTag: s.Model3.CloudCredential.Tag().String(),
			OwnerTag:           s.Model3.Owner.Tag().String(),
			Life:               life.Alive,
			Status: jujuparams.EntityStatus{
				Status: status.Available,
			},
			SLA: &jujuparams.ModelSLAInfo{
				Level: "unsupported",
			},
			Users: []jujuparams.ModelUserInfo{{
				UserName: "bob@external",
				Access:   jujuparams.ModelReadAccess,
			}, {
				UserName:    "charlie@external",
				DisplayName: "charlie",
				Access:      jujuparams.ModelAdminAccess,
			}},
			Machines: []jujuparams.ModelMachineInfo{{
				Id: "0",
				Hardware: &jujuparams.MachineHardware{
					Arch: newString("amd64"),
					Mem:  newUint64(64 * 1024 * 1024 * 1024),
				},
				Status: "pending",
			}, {
				Id: "1",
				Hardware: &jujuparams.MachineHardware{
					Arch: newString("bbc-micro"),
				},
				Status: "pending",
			}, {
				Id: "2",
				Hardware: &jujuparams.MachineHardware{
					Arch: newString("amd64"),
					Mem:  newUint64(64 * 1024 * 1024 * 1024),
				},
				Status: "pending",
			}},
			AgentVersion:            &jujuversion.Current,
			Type:                    "iaas",
			CloudCredentialValidity: newBool(true),
			SupportedFeatures: []jujuparams.SupportedFeature{{
				Name:        "juju",
				Description: "the version of Juju used by the model",
				Version:     "2.9.33",
			}},
		},
	}, {
		Result: &jujuparams.ModelInfo{
			Name:               "model-4",
			UUID:               mt4.Id(),
			ControllerUUID:     "deadbeef-1bad-500d-9000-4b1d0d06f00d",
			ProviderType:       jimmtest.TestProviderType,
			CloudTag:           names.NewCloudTag(jimmtest.TestCloudName).String(),
			CloudRegion:        jimmtest.TestCloudRegionName,
			CloudCredentialTag: s.Model2.CloudCredential.Tag().String(),
			OwnerTag:           names.NewUserTag("charlie@external").String(),
			Life:               life.Alive,
			Status: jujuparams.EntityStatus{
				Status: status.Available,
			},
			SLA: &jujuparams.ModelSLAInfo{
				Level: "unsupported",
			},
			Users: []jujuparams.ModelUserInfo{{
				UserName:    "charlie@external",
				DisplayName: "charlie",
				Access:      jujuparams.ModelAdminAccess,
			}},
			Machines: []jujuparams.ModelMachineInfo{{
				Id: "0",
				Hardware: &jujuparams.MachineHardware{
					Arch: newString("amd64"),
					Mem:  newUint64(64 * 1024 * 1024 * 1024),
				},
				Status: "pending",
			}, {
				Id: "1",
				Hardware: &jujuparams.MachineHardware{
					Arch: newString("bbc-micro"),
				},
				Status: "pending",
			}, {
				Id: "2",
				Hardware: &jujuparams.MachineHardware{
					Arch: newString("amd64"),
					Mem:  newUint64(64 * 1024 * 1024 * 1024),
				},
				Status: "pending",
			}},
			AgentVersion:            &jujuversion.Current,
			Type:                    "iaas",
			CloudCredentialValidity: newBool(true),
			SupportedFeatures: []jujuparams.SupportedFeature{{
				Name:        "juju",
				Description: "the version of Juju used by the model",
				Version:     "2.9.33",
			}},
		},
	}, {
		Result: &jujuparams.ModelInfo{
			Name:               "model-5",
			UUID:               mt5.Id(),
			ControllerUUID:     "deadbeef-1bad-500d-9000-4b1d0d06f00d",
			ProviderType:       jimmtest.TestProviderType,
			CloudTag:           names.NewCloudTag(jimmtest.TestCloudName).String(),
			CloudRegion:        jimmtest.TestCloudRegionName,
			CloudCredentialTag: s.Model2.CloudCredential.Tag().String(),
			OwnerTag:           names.NewUserTag("charlie@external").String(),
			Life:               life.Alive,
			Status: jujuparams.EntityStatus{
				Status: status.Available,
			},
			SLA: &jujuparams.ModelSLAInfo{
				Level: "unsupported",
			},
			Users: []jujuparams.ModelUserInfo{{
				UserName:    "charlie@external",
				DisplayName: "charlie",
				Access:      jujuparams.ModelAdminAccess,
			}},
			AgentVersion:            &jujuversion.Current,
			Type:                    "iaas",
			CloudCredentialValidity: newBool(true),
			SupportedFeatures: []jujuparams.SupportedFeature{{
				Name:        "juju",
				Description: "the version of Juju used by the model",
				Version:     "2.9.33",
			}},
		},
	}, {
		Error: &jujuparams.Error{
			Message: `unauthorized`,
			Code:    jujuparams.CodeUnauthorized,
		},
	}})
}

var createModelTests = []struct {
	about         string
	name          string
	ownerTag      string
	region        string
	cloudTag      string
	credentialTag string
	config        map[string]interface{}
	expectError   string
}{{
	about:         "success",
	name:          "model",
	ownerTag:      names.NewUserTag("bob@external").String(),
	cloudTag:      names.NewCloudTag(jimmtest.TestCloudName).String(),
	credentialTag: "cloudcred-" + jimmtest.TestCloudName + "_bob@external_cred",
}, {
	about:         "unauthorized user",
	name:          "model-2",
	ownerTag:      names.NewUserTag("charlie@external").String(),
	cloudTag:      names.NewCloudTag(jimmtest.TestCloudName).String(),
	credentialTag: "cloudcred-" + jimmtest.TestCloudName + "_bob@external_cred",
	expectError:   `unauthorized \(unauthorized access\)`,
}, {
	about:         "existing model name",
	name:          "model-1",
	ownerTag:      names.NewUserTag("bob@external").String(),
	cloudTag:      names.NewCloudTag(jimmtest.TestCloudName).String(),
	credentialTag: "cloudcred-" + jimmtest.TestCloudName + "_bob@external_cred",
	expectError:   "model bob@external/model-1 already exists \\(already exists\\)",
}, {
	about:         "no controller",
	name:          "model-3",
	ownerTag:      names.NewUserTag("bob@external").String(),
	region:        "no-such-region",
	cloudTag:      names.NewCloudTag(jimmtest.TestCloudName).String(),
	credentialTag: "",
	expectError:   `cloudregion not found \(not found\)`,
}, {
	about:         "local user",
	name:          "model-4",
	ownerTag:      names.NewUserTag("bob").String(),
	cloudTag:      names.NewCloudTag(jimmtest.TestCloudName).String(),
	credentialTag: "cloudcred-" + jimmtest.TestCloudName + "_bob@external_cred",
	expectError:   `unauthorized \(unauthorized access\)`,
}, {
	about:         "invalid user",
	name:          "model-5",
	ownerTag:      "user-bob/test@external",
	cloudTag:      names.NewCloudTag(jimmtest.TestCloudName).String(),
	credentialTag: "cloudcred-" + jimmtest.TestCloudName + "_bob@external_cred",
	expectError:   `"user-bob/test@external" is not a valid user tag \(bad request\)`,
}, {
	about:         "specific cloud",
	name:          "model-6",
	ownerTag:      names.NewUserTag("bob@external").String(),
	cloudTag:      names.NewCloudTag(jimmtest.TestCloudName).String(),
	credentialTag: "cloudcred-" + jimmtest.TestCloudName + "_bob@external_cred",
}, {
	about:         "specific cloud and region",
	name:          "model-7",
	ownerTag:      names.NewUserTag("bob@external").String(),
	cloudTag:      names.NewCloudTag(jimmtest.TestCloudName).String(),
	region:        jimmtest.TestCloudRegionName,
	credentialTag: "cloudcred-" + jimmtest.TestCloudName + "_bob@external_cred",
}, {
	about:         "bad cloud tag",
	name:          "model-8",
	ownerTag:      names.NewUserTag("bob@external").String(),
	cloudTag:      "not-a-cloud-tag",
	credentialTag: "cloudcred-" + jimmtest.TestCloudName + "_bob@external_cred1",
	expectError:   `"not-a-cloud-tag" is not a valid tag \(bad request\)`,
}, {
	about:         "no cloud tag",
	name:          "model-8",
	ownerTag:      names.NewUserTag("bob@external").String(),
	cloudTag:      "",
	credentialTag: "cloudcred-" + jimmtest.TestCloudName + "_bob@external_cred1",
	expectError:   `no cloud specified for model; please specify one`,
}, {
	about:    "no credential tag selects unambigous creds",
	name:     "model-8",
	ownerTag: names.NewUserTag("bob@external").String(),
	cloudTag: names.NewCloudTag(jimmtest.TestCloudName).String(),
	region:   jimmtest.TestCloudRegionName,
}}

func (s *modelManagerSuite) TestCreateModel(c *gc.C) {
	conn := s.open(c, nil, "bob")
	defer conn.Close()

	for i, test := range createModelTests {
		c.Logf("test %d. %s", i, test.about)
		var mi jujuparams.ModelInfo
		err := conn.APICall("ModelManager", 2, "", "CreateModel", jujuparams.ModelCreateArgs{
			Name:               test.name,
			OwnerTag:           test.ownerTag,
			Config:             test.config,
			CloudTag:           test.cloudTag,
			CloudRegion:        test.region,
			CloudCredentialTag: test.credentialTag,
		}, &mi)
		if test.expectError != "" {
			c.Assert(err, gc.ErrorMatches, test.expectError)
			continue
		}
		c.Assert(err, gc.Equals, nil)
		c.Assert(mi.Name, gc.Equals, test.name)
		c.Assert(mi.UUID, gc.Not(gc.Equals), "")
		c.Assert(mi.OwnerTag, gc.Equals, test.ownerTag)
		c.Assert(mi.ControllerUUID, gc.Equals, "914487b5-60e7-42bb-bd63-1adc3fd3a388")
		c.Assert(mi.Users, gc.Not(gc.HasLen), 0)
		if test.credentialTag == "" {
			c.Assert(mi.CloudCredentialTag, gc.Not(gc.Equals), "")
		} else {
			tag, err := names.ParseCloudCredentialTag(mi.CloudCredentialTag)
			c.Assert(err, gc.Equals, nil)
			c.Assert(tag.String(), gc.Equals, test.credentialTag)
		}
		if test.cloudTag == "" {
			c.Assert(mi.CloudTag, gc.Equals, names.NewCloudTag(jimmtest.TestCloudName).String())
		} else {
			ct, err := names.ParseCloudTag(test.cloudTag)
			c.Assert(err, gc.Equals, nil)
			c.Assert(mi.CloudTag, gc.Equals, names.NewCloudTag(ct.Id()).String())
		}
	}
}

func (s *modelManagerSuite) TestGrantAndRevokeModel(c *gc.C) {
	conn := s.open(c, nil, "bob")
	defer conn.Close()
	client := modelmanager.NewClient(conn)

	conn2 := s.open(c, nil, "charlie")
	defer conn2.Close()
	client2 := modelmanager.NewClient(conn2)

	res, err := client2.ModelInfo([]names.ModelTag{s.Model.Tag().(names.ModelTag)})
	c.Assert(err, gc.Equals, nil)
	c.Assert(res, gc.HasLen, 1)
	c.Assert(res[0].Error, gc.ErrorMatches, "unauthorized")

	err = client.GrantModel("charlie@external", "write", s.Model.UUID.String)
	c.Assert(err, gc.Equals, nil)

	res, err = client2.ModelInfo([]names.ModelTag{s.Model.Tag().(names.ModelTag)})
	c.Assert(err, gc.Equals, nil)
	c.Assert(res, gc.HasLen, 1)
	c.Assert(res[0].Error, gc.IsNil)
	c.Assert(res[0].Result.UUID, gc.Equals, s.Model.UUID.String)

	err = client.RevokeModel("charlie@external", "read", s.Model.UUID.String)
	c.Assert(err, gc.Equals, nil)

	res, err = client2.ModelInfo([]names.ModelTag{s.Model.Tag().(names.ModelTag)})
	c.Assert(err, gc.Equals, nil)
	c.Assert(res, gc.HasLen, 1)
	c.Assert(res[0].Error, gc.Not(gc.IsNil))
	c.Assert(res[0].Error, gc.ErrorMatches, "unauthorized")
}

func (s *modelManagerSuite) TestUserRevokeOwnAccess(c *gc.C) {
	conn := s.open(c, nil, "bob")
	defer conn.Close()
	client := modelmanager.NewClient(conn)

	conn2 := s.open(c, nil, "charlie")
	defer conn2.Close()
	client2 := modelmanager.NewClient(conn2)

	err := client.GrantModel("charlie@external", "read", s.Model.UUID.String)
	c.Assert(err, gc.Equals, nil)

	res, err := client2.ModelInfo([]names.ModelTag{names.NewModelTag(s.Model.UUID.String)})
	c.Assert(err, gc.Equals, nil)
	c.Assert(res, gc.HasLen, 1)
	c.Assert(res[0].Error, gc.IsNil)
	c.Assert(res[0].Result.UUID, gc.Equals, s.Model.UUID.String)

	err = client2.RevokeModel("charlie@external", "read", s.Model.UUID.String)
	c.Assert(err, gc.Equals, nil)

	res, err = client2.ModelInfo([]names.ModelTag{names.NewModelTag(s.Model.UUID.String)})
	c.Assert(err, gc.Equals, nil)
	c.Assert(res, gc.HasLen, 1)
	c.Assert(res[0].Error, gc.Not(gc.IsNil))
	c.Assert(res[0].Error, gc.ErrorMatches, "unauthorized")
}

func (s *modelManagerSuite) TestModifyModelAccessErrors(c *gc.C) {
	conn := s.open(c, nil, "bob")
	defer conn.Close()

	modifyModelAccessErrorTests := []struct {
		about             string
		modifyModelAccess jujuparams.ModifyModelAccess
		expectError       string
	}{{
		about: "unauthorized",
		modifyModelAccess: jujuparams.ModifyModelAccess{
			UserTag:  names.NewUserTag("eve@external").String(),
			Action:   jujuparams.GrantModelAccess,
			Access:   jujuparams.ModelReadAccess,
			ModelTag: s.Model2.Tag().String(),
		},
		expectError: `unauthorized`,
	}, {
		about: "bad user domain",
		modifyModelAccess: jujuparams.ModifyModelAccess{
			UserTag:  names.NewUserTag("eve@local").String(),
			Action:   jujuparams.GrantModelAccess,
			Access:   jujuparams.ModelReadAccess,
			ModelTag: s.Model.Tag().String(),
		},
		expectError: `unsupported local user`,
	}, {
		about: "no such model",
		modifyModelAccess: jujuparams.ModifyModelAccess{
			UserTag:  names.NewUserTag("eve@external").String(),
			Action:   jujuparams.GrantModelAccess,
			Access:   jujuparams.ModelReadAccess,
			ModelTag: names.NewModelTag("00000000-0000-0000-0000-000000000000").String(),
		},
		expectError: `unauthorized`,
	}, {
		about: "invalid model tag",
		modifyModelAccess: jujuparams.ModifyModelAccess{
			UserTag:  names.NewUserTag("eve@external").String(),
			Action:   jujuparams.GrantModelAccess,
			Access:   jujuparams.ModelReadAccess,
			ModelTag: "not-a-model-tag",
		},
		expectError: `"not-a-model-tag" is not a valid tag`,
	}, {
		about: "invalid user tag",
		modifyModelAccess: jujuparams.ModifyModelAccess{
			UserTag:  "not-a-user-tag",
			Action:   jujuparams.GrantModelAccess,
			Access:   jujuparams.ModelReadAccess,
			ModelTag: s.Model.Tag().String(),
		},
		expectError: `"not-a-user-tag" is not a valid tag`,
	}, {
		about: "unknown action",
		modifyModelAccess: jujuparams.ModifyModelAccess{
			UserTag:  names.NewUserTag("eve@external").String(),
			Action:   "not-an-action",
			Access:   jujuparams.ModelReadAccess,
			ModelTag: s.Model.Tag().String(),
		},
		expectError: `invalid action "not-an-action"`,
	}, {
		about: "invalid access",
		modifyModelAccess: jujuparams.ModifyModelAccess{
			UserTag:  names.NewUserTag("eve@external").String(),
			Action:   jujuparams.GrantModelAccess,
			Access:   "not-an-access",
			ModelTag: s.Model.Tag().String(),
		},
		expectError: `could not modify model access: "not-an-access" model access not valid`,
	}}

	for i, test := range modifyModelAccessErrorTests {
		c.Logf("%d. %s", i, test.about)
		var res jujuparams.ErrorResults
		req := jujuparams.ModifyModelAccessRequest{
			Changes: []jujuparams.ModifyModelAccess{
				test.modifyModelAccess,
			},
		}
		err := conn.APICall("ModelManager", 2, "", "ModifyModelAccess", req, &res)
		c.Assert(err, gc.Equals, nil)
		c.Assert(res.Results, gc.HasLen, 1)
		c.Assert(res.Results[0].Error, gc.ErrorMatches, test.expectError)
	}
}

func (s *modelManagerSuite) TestDestroyModel(c *gc.C) {
	ctx := context.Background()

	conn := s.open(c, nil, "bob")
	defer conn.Close()

	client := modelmanager.NewClient(conn)
	tag := s.Model.Tag().(names.ModelTag)
	err := client.DestroyModel(tag, nil, nil, nil, time.Duration(0))
	c.Assert(err, gc.Equals, nil)

	// Check the model is now dying.
	mis, err := client.ModelInfo([]names.ModelTag{tag})
	c.Assert(err, gc.Equals, nil)
	c.Assert(mis, gc.HasLen, 1)
	c.Assert(mis[0].Error, gc.Equals, (*jujuparams.Error)(nil))
	c.Assert(mis[0].Result.Life, gc.Equals, life.Dying)

	// Kill the model.
	err = s.JIMM.Database.DeleteModel(ctx, s.Model)
	c.Assert(err, gc.Equals, nil)

	// Make sure it's not an error if you destroy a model that't not there.
	err = client.DestroyModel(s.Model.Tag().(names.ModelTag), nil, nil, nil, time.Duration(0))
	c.Assert(err, gc.Equals, nil)
}

func (s *modelManagerSuite) TestDestroyModelV3(c *gc.C) {
	conn := s.open(c, nil, "bob")
	defer conn.Close()

	tag := s.Model.Tag().(names.ModelTag)
	var results jujuparams.ErrorResults
	err := conn.APICall("ModelManager", 3, "", "DestroyModels", jujuparams.Entities{
		Entities: []jujuparams.Entity{{
			Tag: tag.String(),
		}},
	}, &results)
	c.Assert(err, gc.Equals, nil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.Equals, (*jujuparams.Error)(nil))

	// Check the model is now dying.
	client := modelmanager.NewClient(conn)
	mis, err := client.ModelInfo([]names.ModelTag{tag})
	c.Assert(err, gc.Equals, nil)
	c.Assert(mis, gc.HasLen, 1)
	c.Assert(mis[0].Error, gc.Equals, (*jujuparams.Error)(nil))
	c.Assert(mis[0].Result.Life, gc.Equals, life.Dying)
}

func (s *modelManagerSuite) TestDumpModel(c *gc.C) {
	conn := s.open(c, nil, "bob")
	defer conn.Close()

	tag := s.Model.Tag().(names.ModelTag)
	client := modelmanager.NewClient(conn)
	res, err := client.DumpModel(tag, false)
	c.Check(err, gc.Equals, nil)
	c.Check(res, gc.Not(gc.HasLen), 0)
}

func (s *modelManagerSuite) TestDumpModelUnauthorized(c *gc.C) {
	conn := s.open(c, nil, "charlie")
	defer conn.Close()

	tag := s.Model.Tag().(names.ModelTag)
	client := modelmanager.NewClient(conn)
	res, err := client.DumpModel(tag, true)
	c.Check(err, gc.ErrorMatches, `unauthorized`)
	c.Check(res, gc.IsNil)
}

func (s *modelManagerSuite) TestDumpModelDB(c *gc.C) {
	conn := s.open(c, nil, "bob")
	defer conn.Close()

	tag := s.Model.Tag().(names.ModelTag)
	client := modelmanager.NewClient(conn)
	res, err := client.DumpModelDB(tag)
	c.Check(err, gc.Equals, nil)
	c.Check(res, gc.Not(gc.HasLen), 0)
}

func (s *modelManagerSuite) TestDumpModelDBUnauthorized(c *gc.C) {
	conn := s.open(c, nil, "charlie")
	defer conn.Close()

	tag := s.Model.Tag().(names.ModelTag)
	client := modelmanager.NewClient(conn)
	res, err := client.DumpModelDB(tag)
	c.Check(err, gc.ErrorMatches, `unauthorized`)
	c.Check(res, gc.IsNil)
}

func (s *modelManagerSuite) TestChangeModelCredential(c *gc.C) {
	conn := s.open(c, nil, "bob")
	defer conn.Close()

	modelTag := s.Model.Tag().(names.ModelTag)
	credTag := names.NewCloudCredentialTag(jimmtest.TestCloudName + "/bob@external/cred2")
	s.UpdateCloudCredential(c, credTag, jujuparams.CloudCredential{AuthType: "empty"})

	client := modelmanager.NewClient(conn)
	err := client.ChangeModelCredential(modelTag, credTag)
	c.Assert(err, gc.Equals, nil)
	mir, err := client.ModelInfo([]names.ModelTag{modelTag})
	c.Assert(err, gc.Equals, nil)
	c.Assert(mir, gc.HasLen, 1)
	c.Assert(mir[0].Error, gc.IsNil)
	c.Assert(mir[0].Result.CloudCredentialTag, gc.Equals, credTag.String())
}

func (s *modelManagerSuite) TestChangeModelCredentialUnauthorizedModel(c *gc.C) {
	conn := s.open(c, nil, "charlie")
	defer conn.Close()

	modelTag := s.Model.Tag().(names.ModelTag)
	credTag := names.NewCloudCredentialTag(jimmtest.TestCloudName + "/bob@external/cred2")
	client := modelmanager.NewClient(conn)
	err := client.ChangeModelCredential(modelTag, credTag)
	c.Assert(err, gc.ErrorMatches, `unauthorized`)
}

func (s *modelManagerSuite) TestChangeModelCredentialUnauthorizedCredential(c *gc.C) {
	conn := s.open(c, nil, "bob")
	defer conn.Close()

	modelTag := s.Model.Tag().(names.ModelTag)
	credTag := names.NewCloudCredentialTag(jimmtest.TestCloudName + "/alice@external/cred2")
	client := modelmanager.NewClient(conn)
	err := client.ChangeModelCredential(modelTag, credTag)
	c.Assert(err, gc.ErrorMatches, `unauthorized`)
}

func (s *modelManagerSuite) TestChangeModelCredentialNotFoundModel(c *gc.C) {
	conn := s.open(c, nil, "bob")
	defer conn.Close()

	modelTag := names.NewModelTag("000000000-0000-0000-0000-000000000000")
	credTag := s.Model.CloudCredential.Tag().(names.CloudCredentialTag)
	client := modelmanager.NewClient(conn)
	err := client.ChangeModelCredential(modelTag, credTag)
	c.Assert(err, gc.ErrorMatches, `model not found`)
}

func (s *modelManagerSuite) TestChangeModelCredentialNotFoundCredential(c *gc.C) {
	conn := s.open(c, nil, "bob")
	defer conn.Close()

	modelTag := s.Model.Tag().(names.ModelTag)
	credTag := names.NewCloudCredentialTag(jimmtest.TestCloudName + "/bob@external/cred2")
	client := modelmanager.NewClient(conn)
	err := client.ChangeModelCredential(modelTag, credTag)
	c.Assert(err, gc.ErrorMatches, `cloudcredential "`+jimmtest.TestCloudName+`/bob@external/cred2" not found`)
}

func (s *modelManagerSuite) TestChangeModelCredentialLocalUserCredential(c *gc.C) {
	conn := s.open(c, nil, "bob")
	defer conn.Close()

	modelTag := s.Model.Tag().(names.ModelTag)
	credTag := names.NewCloudCredentialTag(jimmtest.TestCloudName + "/bob/cred2")
	client := modelmanager.NewClient(conn)
	err := client.ChangeModelCredential(modelTag, credTag)
	c.Assert(err, gc.ErrorMatches, `unauthorized`)
}

func (s *modelManagerSuite) TestValidateModelUpgrades(c *gc.C) {
	conn := s.open(c, nil, "alice")
	defer conn.Close()

	modelTag := s.Model.Tag().(names.ModelTag)
	client := modelmanager.NewClient(conn)
	err := client.ValidateModelUpgrade(modelTag, false)
	c.Assert(err, gc.Equals, nil)

	uuid := utils.MustNewUUID().String()
	err = client.ValidateModelUpgrade(names.NewModelTag(uuid), false)
	c.Assert(err, gc.ErrorMatches, "model not found")
}

type modelManagerStorageSuite struct {
	websocketSuite
	state   *state.PooledState
	factory *factory.Factory
}

var _ = gc.Suite(&modelManagerStorageSuite{})

func (s *modelManagerStorageSuite) SetUpTest(c *gc.C) {
	s.websocketSuite.SetUpTest(c)
	var err error
	s.state, err = s.StatePool.Get(s.Model.UUID.String)
	c.Assert(err, gc.Equals, nil)
	s.factory = factory.NewFactory(s.state.State, s.StatePool)
	s.factory.MakeUnit(c, &factory.UnitParams{
		Application: s.factory.MakeApplication(c, &factory.ApplicationParams{
			Charm: s.factory.MakeCharm(c, &factory.CharmParams{
				Name: "storage-block",
			}),
			Storage: map[string]state.StorageConstraints{
				"data": {Pool: "modelscoped"},
			},
		}),
	})
}

func (s *modelManagerStorageSuite) TearDownTest(c *gc.C) {
	s.factory = nil
	if s.state != nil {
		s.state.Release()
		s.state = nil
	}
	s.websocketSuite.TearDownTest(c)
}

func (s *modelManagerStorageSuite) TestDestroyModelWithStorageError(c *gc.C) {
	conn := s.open(c, nil, "bob")
	defer conn.Close()

	tag := s.Model.Tag().(names.ModelTag)
	client := modelmanager.NewClient(conn)
	err := client.DestroyModel(tag, nil, nil, nil, time.Duration(0))
	c.Assert(err, jc.Satisfies, jujuparams.IsCodeHasPersistentStorage)

	// Check the model is not now dying.
	mis, err := client.ModelInfo([]names.ModelTag{tag})
	c.Assert(err, gc.Equals, nil)
	c.Assert(mis, gc.HasLen, 1)
	c.Assert(mis[0].Error, gc.Equals, (*jujuparams.Error)(nil))
	c.Assert(mis[0].Result.Life, gc.Equals, life.Alive)
}

func (s *modelManagerStorageSuite) TestDestroyModelWithStorageDestroyStorageTrue(c *gc.C) {
	conn := s.open(c, nil, "bob")
	defer conn.Close()

	tag := s.Model.Tag().(names.ModelTag)
	client := modelmanager.NewClient(conn)
	err := client.DestroyModel(tag, newBool(true), nil, nil, time.Duration(0))
	c.Assert(err, gc.Equals, nil)

	// Check the model is not now dying.
	mis, err := client.ModelInfo([]names.ModelTag{tag})
	c.Assert(err, gc.Equals, nil)
	c.Assert(mis, gc.HasLen, 1)
	c.Assert(mis[0].Error, gc.Equals, (*jujuparams.Error)(nil))
	c.Assert(mis[0].Result.Life, gc.Equals, life.Dying)
}

func (s *modelManagerStorageSuite) TestDestroyModelWithStorageDestroyStorageFalse(c *gc.C) {
	conn := s.open(c, nil, "bob")
	defer conn.Close()

	tag := s.Model.Tag().(names.ModelTag)
	client := modelmanager.NewClient(conn)
	err := client.DestroyModel(tag, newBool(false), nil, nil, time.Duration(0))
	c.Assert(err, gc.Equals, nil)

	// Check the model is not now dying.
	mis, err := client.ModelInfo([]names.ModelTag{tag})
	c.Assert(err, gc.Equals, nil)
	c.Assert(mis, gc.HasLen, 1)
	c.Assert(mis[0].Error, gc.Equals, (*jujuparams.Error)(nil))
	c.Assert(mis[0].Result.Life, gc.Equals, life.Dying)
}

type caasModelManagerSuite struct {
	websocketSuite

	cred names.CloudCredentialTag
}

var _ = gc.Suite(&caasModelManagerSuite{})

func (s *caasModelManagerSuite) SetUpTest(c *gc.C) {
	s.websocketSuite.SetUpTest(c)

	ksrv := kubetest.NewFakeKubernetes(c)
	s.AddCleanup(func(c *gc.C) {
		ksrv.Close()
	})

	conn := s.open(c, nil, "bob")
	defer conn.Close()

	cloudclient := cloudapi.NewClient(conn)
	err := cloudclient.AddCloud(cloud.Cloud{
		Name:            "bob-cloud",
		Type:            "kubernetes",
		AuthTypes:       cloud.AuthTypes{cloud.UserPassAuthType},
		Endpoint:        ksrv.URL,
		HostCloudRegion: jimmtest.TestProviderType + "/" + jimmtest.TestCloudRegionName,
	}, false)
	c.Assert(err, gc.Equals, nil)

	s.cred = names.NewCloudCredentialTag("bob-cloud/bob@external/k8s")
	cred := cloud.NewCredential(cloud.UserPassAuthType, map[string]string{
		"username": kubetest.Username,
		"password": kubetest.Password,
	})
	res, err := cloudclient.UpdateCredentialsCheckModels(s.cred, cred)
	c.Assert(err, gc.Equals, nil)
	for _, model := range res {
		for _, err := range model.Errors {
			c.Assert(err, gc.Equals, nil)
		}
	}
}

func (s *caasModelManagerSuite) TestCreateModelKubernetes(c *gc.C) {
	// TODO (ashipika): remove skip when the issue is resolved
	// Error message: enumerating features supported by environment: querying kubernetes API version: the server could not find the requested resource
	c.Skip("k8s_issue")
	conn := s.open(c, nil, "bob")
	defer conn.Close()

	client := modelmanager.NewClient(conn)
	mi, err := client.CreateModel("k8s-model-1", "bob@external", "bob-cloud", "", s.cred, nil)
	c.Assert(err, gc.Equals, nil)

	c.Assert(mi.Name, gc.Equals, "k8s-model-1")
	c.Assert(mi.Type, gc.Equals, model.CAAS)
	c.Assert(mi.ProviderType, gc.Equals, "kubernetes")
	c.Assert(mi.Cloud, gc.Equals, "bob-cloud")
	c.Assert(mi.CloudRegion, gc.Equals, "default")
	c.Assert(mi.Owner, gc.Equals, "bob@external")
}

func (s *caasModelManagerSuite) TestListCAASModelSummaries(c *gc.C) {
	// TODO (ashipika): remove skip when the issue is resolved
	// Error message: enumerating features supported by environment: querying kubernetes API version: the server could not find the requested resource
	c.Skip("k8s_issue")
	conn := s.open(c, nil, "bob")
	defer conn.Close()

	client := modelmanager.NewClient(conn)
	mi, err := client.CreateModel("k8s-model-1", "bob@external", "bob-cloud", "", s.cred, nil)
	c.Assert(err, gc.Equals, nil)

	models, err := client.ListModelSummaries("bob", false)
	c.Assert(err, gc.Equals, nil)
	c.Assert(models, jimmtest.CmpEquals(cmpopts.IgnoreTypes(&time.Time{})), []base.UserModelSummary{{
		Name:            "k8s-model-1",
		UUID:            mi.UUID,
		ControllerUUID:  "914487b5-60e7-42bb-bd63-1adc3fd3a388",
		ProviderType:    "kubernetes",
		DefaultSeries:   "focal",
		Cloud:           "bob-cloud",
		CloudRegion:     "default",
		CloudCredential: s.cred.Id(),
		Owner:           "bob@external",
		Life:            "alive",
		Status: base.Status{
			Status: status.Available,
			Data:   map[string]interface{}{},
		},
		ModelUserAccess: "admin",
		Counts: []base.EntityCount{{
			Entity: "machines",
			Count:  0,
		}, {
			Entity: "cores",
			Count:  0,
		}, {
			Entity: "units",
			Count:  0,
		}},
		AgentVersion: &jujuversion.Current,
		Type:         "caas",
		SLA: &base.SLASummary{
			Level: "unsupported",
		},
	}, {
		Name:            "model-1",
		UUID:            s.Model.UUID.String,
		Type:            "iaas",
		ControllerUUID:  "914487b5-60e7-42bb-bd63-1adc3fd3a388",
		ProviderType:    jimmtest.TestProviderType,
		DefaultSeries:   "focal",
		Cloud:           jimmtest.TestCloudName,
		CloudRegion:     jimmtest.TestCloudRegionName,
		CloudCredential: jimmtest.TestCloudName + "/bob@external/cred",
		Owner:           "bob@external",
		Life:            "alive",
		Status: base.Status{
			Status: status.Available,
			Data:   map[string]interface{}{},
		},
		ModelUserAccess: "admin",
		Counts:          []base.EntityCount{{Entity: "machines"}, {Entity: "cores"}, {Entity: "units"}},
		AgentVersion:    &jujuversion.Current,
		SLA: &base.SLASummary{
			Level: "unsupported",
		},
	}, {
		Name:            "model-3",
		UUID:            s.Model3.UUID.String,
		Type:            "iaas",
		ControllerUUID:  "914487b5-60e7-42bb-bd63-1adc3fd3a388",
		ProviderType:    jimmtest.TestProviderType,
		DefaultSeries:   "focal",
		Cloud:           jimmtest.TestCloudName,
		CloudRegion:     jimmtest.TestCloudRegionName,
		CloudCredential: jimmtest.TestCloudName + "/charlie@external/cred",
		Owner:           "charlie@external",
		Life:            "alive",
		Status: base.Status{
			Status: status.Available,
			Data:   map[string]interface{}{},
		},
		ModelUserAccess: "read",
		Counts:          []base.EntityCount{{Entity: "machines"}, {Entity: "cores"}, {Entity: "units"}},
		AgentVersion:    &jujuversion.Current,
		SLA: &base.SLASummary{
			Level: "unsupported",
		},
	}})
}

func (s *caasModelManagerSuite) TestListCAASModels(c *gc.C) {
	// TODO (ashipika): remove skip when the issue is resolved
	// Error message: enumerating features supported by environment: querying kubernetes API version: the server could not find the requested resource
	c.Skip("k8s_issue")
	conn := s.open(c, nil, "bob")
	defer conn.Close()

	client := modelmanager.NewClient(conn)
	mi, err := client.CreateModel("k8s-model-1", "bob@external", "bob-cloud", "", s.cred, nil)
	c.Assert(err, gc.Equals, nil)

	models, err := client.ListModels("bob")
	c.Assert(err, gc.Equals, nil)
	c.Assert(models, jc.DeepEquals, []base.UserModel{{
		Name:  "k8s-model-1",
		UUID:  mi.UUID,
		Owner: "bob@external",
		Type:  "caas",
	}, {
		Name:  "model-1",
		UUID:  s.Model.UUID.String,
		Owner: "bob@external",
		Type:  "iaas",
	}, {
		Name:  "model-3",
		UUID:  s.Model3.UUID.String,
		Owner: "charlie@external",
		Type:  "iaas",
	}})
}

func assertModelInfo(c *gc.C, obtained, expected []jujuparams.ModelInfoResult) {
	for i := range obtained {
		// DefaultSeries changes between juju versions and
		// we don't care about its specific value.
		if obtained[i].Result != nil {
			obtained[i].Result.DefaultSeries = ""
		}
	}
	for i := range obtained {
		if obtained[i].Result == nil {
			continue
		}
		obtained[i].Result.Status.Since = nil
		for j := range obtained[i].Result.Users {
			obtained[i].Result.Users[j].LastConnection = nil
		}
	}
	c.Assert(obtained, jc.DeepEquals, expected)
}

func (s *modelManagerSuite) TestModelDefaults(c *gc.C) {
	err := s.JIMM.Database.AddCloud(context.Background(), &dbmodel.Cloud{
		Name: "aws",
		Type: "ec2",
		Regions: []dbmodel.CloudRegion{{
			Name: "eu-central-1",
		}, {
			Name: "eu-central-2",
		}},
	})
	c.Assert(err, jc.ErrorIsNil)

	conn := s.open(c, nil, "alice")
	defer conn.Close()
	client := modelmanager.NewClient(conn)

	err = client.SetModelDefaults("aws", "eu-central-1", map[string]interface{}{
		"a": 1,
		"b": "value1",
	})
	c.Assert(err, jc.ErrorIsNil)
	err = client.SetModelDefaults("aws", "eu-central-2", map[string]interface{}{
		"b": "value2",
		"c": 17,
	})
	c.Assert(err, jc.ErrorIsNil)

	values, err := client.ModelDefaults("aws")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(values, jc.DeepEquals, config.ModelDefaultAttributes{
		"a": config.AttributeDefaultValues{
			Regions: []config.RegionDefaultValue{{
				Name:  "eu-central-1",
				Value: float64(1),
			}},
		},
		"b": config.AttributeDefaultValues{
			Regions: []config.RegionDefaultValue{{
				Name:  "eu-central-1",
				Value: "value1",
			}, {
				Name:  "eu-central-2",
				Value: "value2",
			}},
		},
		"c": config.AttributeDefaultValues{
			Regions: []config.RegionDefaultValue{{
				Name:  "eu-central-2",
				Value: float64(17),
			}},
		},
	})

	err = client.UnsetModelDefaults("aws", "eu-central-1", "b", "c")
	c.Assert(err, jc.ErrorIsNil)

	err = client.UnsetModelDefaults("aws", "eu-central-2", "a", "b")
	c.Assert(err, jc.ErrorIsNil)

	values, err = client.ModelDefaults("aws")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(values, jc.DeepEquals, config.ModelDefaultAttributes{
		"a": config.AttributeDefaultValues{
			Regions: []config.RegionDefaultValue{{
				Name:  "eu-central-1",
				Value: float64(1),
			}},
		},
		"c": config.AttributeDefaultValues{
			Regions: []config.RegionDefaultValue{{
				Name:  "eu-central-2",
				Value: float64(17),
			}},
		},
	})

	conn1 := s.open(c, nil, "bob")
	defer conn1.Close()
	client1 := modelmanager.NewClient(conn1)

	values, err = client1.ModelDefaults("aws")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(values, jc.DeepEquals, config.ModelDefaultAttributes{})
}

func newBool(b bool) *bool {
	return &b
}

func newString(s string) *string {
	return &s
}

func newUint64(i uint64) *uint64 {
	return &i
}
