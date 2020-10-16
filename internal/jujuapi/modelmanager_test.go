// Copyright 2016 Canonical Ltd.

package jujuapi_test

import (
	"context"
	"time"

	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/juju/juju/api/base"
	cloudapi "github.com/juju/juju/api/cloud"
	"github.com/juju/juju/api/modelmanager"
	jujuparams "github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing/factory"
	jujuversion "github.com/juju/juju/version"
	"github.com/juju/names/v4"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	errgo "gopkg.in/errgo.v1"

	"github.com/CanonicalLtd/jimm/internal/conv"
	"github.com/CanonicalLtd/jimm/internal/jem/jimmdb"
	"github.com/CanonicalLtd/jimm/internal/jemtest"
	"github.com/CanonicalLtd/jimm/internal/kubetest"
	"github.com/CanonicalLtd/jimm/internal/mongodoc"
	"github.com/CanonicalLtd/jimm/params"
)

type modelManagerSuite struct {
	websocketSuite
}

var _ = gc.Suite(&modelManagerSuite{})

func (s *modelManagerSuite) TestListModelSummaries(c *gc.C) {
	conn := s.open(c, nil, "bob")
	defer conn.Close()

	client := modelmanager.NewClient(conn)
	models, err := client.ListModelSummaries("bob", false)
	c.Assert(err, gc.Equals, nil)
	c.Assert(models, jemtest.CmpEquals(cmpopts.IgnoreTypes(&time.Time{})), []base.UserModelSummary{{
		Name:            "model-1",
		UUID:            s.Model.UUID,
		ControllerUUID:  "914487b5-60e7-42bb-bd63-1adc3fd3a388",
		ProviderType:    "dummy",
		DefaultSeries:   "bionic",
		Cloud:           "dummy",
		CloudRegion:     "dummy-region",
		CloudCredential: "dummy/bob@external/cred",
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
	}, {
		Name:            "model-3",
		UUID:            s.Model3.UUID,
		ControllerUUID:  "914487b5-60e7-42bb-bd63-1adc3fd3a388",
		ProviderType:    "dummy",
		DefaultSeries:   "bionic",
		Cloud:           "dummy",
		CloudRegion:     "dummy-region",
		CloudCredential: "dummy/charlie@external/cred",
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
	}})
}

func (s *modelManagerSuite) TestListModelSummariesWithoutControllerUUIDMasking(c *gc.C) {
	conn1 := s.open(c, nil, "charlie")
	defer conn1.Close()
	err := conn1.APICall("JIMM", 2, "", "DisableControllerUUIDMasking", nil, nil)
	c.Assert(err, gc.ErrorMatches, `unauthorized \(unauthorized access\)`)

	s.Candid.AddUser("bob", string(s.JEM.ControllerAdmin()))
	conn := s.open(c, nil, "bob")
	defer conn.Close()
	err = conn.APICall("JIMM", 2, "", "DisableControllerUUIDMasking", nil, nil)
	c.Assert(err, gc.Equals, nil)

	client := modelmanager.NewClient(conn)
	models, err := client.ListModelSummaries("bob", false)
	c.Assert(err, gc.Equals, nil)
	c.Assert(models, jemtest.CmpEquals(cmpopts.IgnoreTypes(&time.Time{})), []base.UserModelSummary{{
		Name:            "model-1",
		UUID:            s.Model.UUID,
		ControllerUUID:  "deadbeef-1bad-500d-9000-4b1d0d06f00d",
		ProviderType:    "dummy",
		DefaultSeries:   "bionic",
		Cloud:           "dummy",
		CloudRegion:     "dummy-region",
		CloudCredential: "dummy/bob@external/cred",
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
	}, {
		Name:            "model-3",
		UUID:            s.Model3.UUID,
		ControllerUUID:  "deadbeef-1bad-500d-9000-4b1d0d06f00d",
		ProviderType:    "dummy",
		DefaultSeries:   "bionic",
		Cloud:           "dummy",
		CloudRegion:     "dummy-region",
		CloudCredential: "dummy/charlie@external/cred",
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
		UUID:  s.Model.UUID,
		Owner: "bob@external",
		Type:  "iaas",
	}, {
		Name:  "model-3",
		UUID:  s.Model3.UUID,
		Owner: "charlie@external",
		Type:  "iaas",
	}})
}

func (s *modelManagerSuite) TestModelInfo(c *gc.C) {
	ctx := context.Background()

	var model4, model5 mongodoc.Model
	model4.Path = params.EntityPath{User: "charlie", Name: "model-4"}
	model4.Credential = s.Credential2.Path
	s.CreateModel(c, &model4, nil, map[params.User]jujuparams.UserAccessPermission{"bob": jujuparams.ModelWriteAccess})
	model5.Path = params.EntityPath{User: "charlie", Name: "model-5"}
	model5.Credential = s.Credential2.Path
	s.CreateModel(c, &model5, nil, map[params.User]jujuparams.UserAccessPermission{"bob": jujuparams.ModelAdminAccess})

	// Add some machines to one of the models
	err := s.JEM.UpdateMachineInfo(ctx, s.Controller.Path, &jujuparams.MachineInfo{
		ModelUUID: s.Model3.UUID,
		Id:        "machine-0",
	})
	c.Assert(err, gc.Equals, nil)
	machineArch := "bbc-micro"
	err = s.JEM.UpdateMachineInfo(ctx, s.Controller.Path, &jujuparams.MachineInfo{
		ModelUUID: s.Model3.UUID,
		Id:        "machine-1",
		HardwareCharacteristics: &instance.HardwareCharacteristics{
			Arch: &machineArch,
		},
	})
	c.Assert(err, gc.Equals, nil)
	err = s.JEM.UpdateMachineInfo(ctx, s.Controller.Path, &jujuparams.MachineInfo{
		ModelUUID: s.Model3.UUID,
		Id:        "machine-2",
		Life:      "dead",
	})
	c.Assert(err, gc.Equals, nil)
	err = s.JEM.UpdateMachineInfo(ctx, s.Controller.Path, &jujuparams.MachineInfo{
		ModelUUID: model4.UUID,
		Id:        "machine-0",
	})
	c.Assert(err, gc.Equals, nil)
	err = s.JEM.UpdateMachineInfo(ctx, s.Controller.Path, &jujuparams.MachineInfo{
		ModelUUID: model4.UUID,
		Id:        "machine-1",
		HardwareCharacteristics: &instance.HardwareCharacteristics{
			Arch: &machineArch,
		},
	})
	c.Assert(err, gc.Equals, nil)
	err = s.JEM.UpdateMachineInfo(ctx, s.Controller.Path, &jujuparams.MachineInfo{
		ModelUUID: model4.UUID,
		Id:        "machine-2",
		Life:      "dead",
	})
	c.Assert(err, gc.Equals, nil)

	conn := s.open(c, nil, "bob")
	defer conn.Close()
	client := modelmanager.NewClient(conn)

	models, err := client.ModelInfo([]names.ModelTag{
		names.NewModelTag(s.Model.UUID),
		names.NewModelTag(s.Model2.UUID),
		names.NewModelTag(s.Model3.UUID),
		names.NewModelTag(model4.UUID),
		names.NewModelTag(model5.UUID),
		names.NewModelTag("00000000-0000-0000-0000-000000000007"),
	})
	c.Assert(err, gc.Equals, nil)

	assertModelInfo(c, models, []jujuparams.ModelInfoResult{{
		Result: &jujuparams.ModelInfo{
			Name:               "model-1",
			UUID:               s.Model.UUID,
			ControllerUUID:     "914487b5-60e7-42bb-bd63-1adc3fd3a388",
			ProviderType:       "dummy",
			CloudTag:           "cloud-dummy",
			CloudRegion:        "dummy-region",
			CloudCredentialTag: conv.ToCloudCredentialTag(s.Credential.Path.ToParams()).String(),
			OwnerTag:           names.NewUserTag("bob@external").String(),
			Life:               life.Alive,
			Status: jujuparams.EntityStatus{
				Status: status.Available,
			},
			Users: []jujuparams.ModelUserInfo{{
				UserName:    "bob@external",
				DisplayName: "bob",
				Access:      jujuparams.ModelAdminAccess,
			}},
			AgentVersion: &jujuversion.Current,
			Type:         "iaas",
		},
	}, {
		Error: &jujuparams.Error{
			Message: "unauthorized",
			Code:    jujuparams.CodeUnauthorized,
		},
	}, {
		Result: &jujuparams.ModelInfo{
			Name:               "model-3",
			UUID:               s.Model3.UUID,
			ControllerUUID:     "914487b5-60e7-42bb-bd63-1adc3fd3a388",
			ProviderType:       "dummy",
			CloudTag:           "cloud-dummy",
			CloudRegion:        "dummy-region",
			CloudCredentialTag: conv.ToCloudCredentialTag(s.Credential2.Path.ToParams()).String(),
			OwnerTag:           names.NewUserTag("charlie@external").String(),
			Life:               life.Alive,
			Status: jujuparams.EntityStatus{
				Status: status.Available,
			},
			Users: []jujuparams.ModelUserInfo{{
				UserName:    "bob@external",
				DisplayName: "bob",
				Access:      jujuparams.ModelReadAccess,
			}},
			AgentVersion: &jujuversion.Current,
			Type:         "iaas",
		},
	}, {
		Result: &jujuparams.ModelInfo{
			Name:               "model-4",
			UUID:               model4.UUID,
			ControllerUUID:     "914487b5-60e7-42bb-bd63-1adc3fd3a388",
			ProviderType:       "dummy",
			CloudTag:           "cloud-dummy",
			CloudRegion:        "dummy-region",
			CloudCredentialTag: conv.ToCloudCredentialTag(s.Credential2.Path.ToParams()).String(),
			OwnerTag:           names.NewUserTag("charlie@external").String(),
			Life:               life.Alive,
			Status: jujuparams.EntityStatus{
				Status: status.Available,
			},
			Users: []jujuparams.ModelUserInfo{{
				UserName:    "bob@external",
				DisplayName: "bob",
				Access:      jujuparams.ModelWriteAccess,
			}},
			Machines: []jujuparams.ModelMachineInfo{{
				Id: "machine-0",
			}, {
				Id: "machine-1",
				Hardware: &jujuparams.MachineHardware{
					Arch: &machineArch,
				},
			}},
			AgentVersion: &jujuversion.Current,
			Type:         "iaas",
		},
	}, {
		Result: &jujuparams.ModelInfo{
			Name:               "model-5",
			UUID:               model5.UUID,
			ControllerUUID:     "914487b5-60e7-42bb-bd63-1adc3fd3a388",
			ProviderType:       "dummy",
			CloudTag:           "cloud-dummy",
			CloudRegion:        "dummy-region",
			CloudCredentialTag: conv.ToCloudCredentialTag(s.Credential2.Path.ToParams()).String(),
			OwnerTag:           names.NewUserTag("charlie@external").String(),
			Life:               life.Alive,
			Status: jujuparams.EntityStatus{
				Status: status.Available,
			},
			Users: []jujuparams.ModelUserInfo{{
				UserName:    "bob@external",
				DisplayName: "bob",
				Access:      jujuparams.ModelAdminAccess,
			}, {
				UserName:    "charlie@external",
				DisplayName: "charlie",
				Access:      jujuparams.ModelAdminAccess,
			}},
			AgentVersion: &jujuversion.Current,
			Type:         "iaas",
		},
	}, {
		Error: &jujuparams.Error{
			Message: `unauthorized`,
			Code:    jujuparams.CodeUnauthorized,
		},
	}})
}

func (s *modelManagerSuite) TestModelInfoDisableControllerUUIDMasking(c *gc.C) {
	ctx := context.Background()

	var model4, model5 mongodoc.Model
	model4.Path = params.EntityPath{User: "charlie", Name: "model-4"}
	model4.Credential = s.Credential2.Path
	s.CreateModel(c, &model4, nil, map[params.User]jujuparams.UserAccessPermission{"bob": jujuparams.ModelWriteAccess})
	model5.Path = params.EntityPath{User: "charlie", Name: "model-5"}
	model5.Credential = s.Credential2.Path
	s.CreateModel(c, &model5, nil, map[params.User]jujuparams.UserAccessPermission{"bob": jujuparams.ModelAdminAccess})

	// Add some machines to one of the models
	err := s.JEM.UpdateMachineInfo(ctx, s.Controller.Path, &jujuparams.MachineInfo{
		ModelUUID: s.Model3.UUID,
		Id:        "machine-0",
	})
	c.Assert(err, gc.Equals, nil)
	machineArch := "bbc-micro"
	err = s.JEM.UpdateMachineInfo(ctx, s.Controller.Path, &jujuparams.MachineInfo{
		ModelUUID: s.Model3.UUID,
		Id:        "machine-1",
		HardwareCharacteristics: &instance.HardwareCharacteristics{
			Arch: &machineArch,
		},
	})
	c.Assert(err, gc.Equals, nil)
	err = s.JEM.UpdateMachineInfo(ctx, s.Controller.Path, &jujuparams.MachineInfo{
		ModelUUID: s.Model3.UUID,
		Id:        "machine-2",
		Life:      "dead",
	})
	c.Assert(err, gc.Equals, nil)
	err = s.JEM.UpdateMachineInfo(ctx, s.Controller.Path, &jujuparams.MachineInfo{
		ModelUUID: model4.UUID,
		Id:        "machine-0",
	})
	c.Assert(err, gc.Equals, nil)
	err = s.JEM.UpdateMachineInfo(ctx, s.Controller.Path, &jujuparams.MachineInfo{
		ModelUUID: model4.UUID,
		Id:        "machine-1",
		HardwareCharacteristics: &instance.HardwareCharacteristics{
			Arch: &machineArch,
		},
	})
	c.Assert(err, gc.Equals, nil)
	err = s.JEM.UpdateMachineInfo(ctx, s.Controller.Path, &jujuparams.MachineInfo{
		ModelUUID: model4.UUID,
		Id:        "machine-2",
		Life:      "dead",
	})
	c.Assert(err, gc.Equals, nil)

	s.Candid.AddUser("bob", string(s.JEM.ControllerAdmin()))
	conn := s.open(c, nil, "bob")
	defer conn.Close()
	client := modelmanager.NewClient(conn)

	err = conn.APICall("JIMM", 2, "", "DisableControllerUUIDMasking", nil, nil)
	c.Assert(err, gc.Equals, nil)

	models, err := client.ModelInfo([]names.ModelTag{
		names.NewModelTag(s.Model.UUID),
		names.NewModelTag(s.Model2.UUID),
		names.NewModelTag(s.Model3.UUID),
		names.NewModelTag(model4.UUID),
		names.NewModelTag(model5.UUID),
		names.NewModelTag("00000000-0000-0000-0000-000000000007"),
	})
	c.Assert(err, gc.Equals, nil)

	assertModelInfo(c, models, []jujuparams.ModelInfoResult{{
		Result: &jujuparams.ModelInfo{
			Name:               "model-1",
			UUID:               s.Model.UUID,
			ControllerUUID:     "deadbeef-1bad-500d-9000-4b1d0d06f00d",
			ProviderType:       "dummy",
			CloudTag:           "cloud-dummy",
			CloudRegion:        "dummy-region",
			CloudCredentialTag: conv.ToCloudCredentialTag(s.Credential.Path.ToParams()).String(),
			OwnerTag:           names.NewUserTag("bob@external").String(),
			Life:               life.Alive,
			Status: jujuparams.EntityStatus{
				Status: status.Available,
			},
			Users: []jujuparams.ModelUserInfo{{
				UserName:    "bob@external",
				DisplayName: "bob",
				Access:      jujuparams.ModelAdminAccess,
			}},
			AgentVersion: &jujuversion.Current,
			Type:         "iaas",
		},
	}, {
		Error: &jujuparams.Error{
			Message: "unauthorized",
			Code:    jujuparams.CodeUnauthorized,
		},
	}, {
		Result: &jujuparams.ModelInfo{
			Name:               "model-3",
			UUID:               s.Model3.UUID,
			ControllerUUID:     "deadbeef-1bad-500d-9000-4b1d0d06f00d",
			ProviderType:       "dummy",
			CloudTag:           "cloud-dummy",
			CloudRegion:        "dummy-region",
			CloudCredentialTag: conv.ToCloudCredentialTag(s.Credential2.Path.ToParams()).String(),
			OwnerTag:           names.NewUserTag("charlie@external").String(),
			Life:               life.Alive,
			Status: jujuparams.EntityStatus{
				Status: status.Available,
			},
			Users: []jujuparams.ModelUserInfo{{
				UserName:    "bob@external",
				DisplayName: "bob",
				Access:      jujuparams.ModelReadAccess,
			}},
			AgentVersion: &jujuversion.Current,
			Type:         "iaas",
		},
	}, {
		Result: &jujuparams.ModelInfo{
			Name:               "model-4",
			UUID:               model4.UUID,
			ControllerUUID:     "deadbeef-1bad-500d-9000-4b1d0d06f00d",
			ProviderType:       "dummy",
			CloudTag:           "cloud-dummy",
			CloudRegion:        "dummy-region",
			CloudCredentialTag: conv.ToCloudCredentialTag(s.Credential2.Path.ToParams()).String(),
			OwnerTag:           names.NewUserTag("charlie@external").String(),
			Life:               life.Alive,
			Status: jujuparams.EntityStatus{
				Status: status.Available,
			},
			Users: []jujuparams.ModelUserInfo{{
				UserName:    "bob@external",
				DisplayName: "bob",
				Access:      jujuparams.ModelWriteAccess,
			}},
			Machines: []jujuparams.ModelMachineInfo{{
				Id: "machine-0",
			}, {
				Id: "machine-1",
				Hardware: &jujuparams.MachineHardware{
					Arch: &machineArch,
				},
			}},
			AgentVersion: &jujuversion.Current,
			Type:         "iaas",
		},
	}, {
		Result: &jujuparams.ModelInfo{
			Name:               "model-5",
			UUID:               model5.UUID,
			ControllerUUID:     "deadbeef-1bad-500d-9000-4b1d0d06f00d",
			ProviderType:       "dummy",
			CloudTag:           "cloud-dummy",
			CloudRegion:        "dummy-region",
			CloudCredentialTag: conv.ToCloudCredentialTag(s.Credential2.Path.ToParams()).String(),
			OwnerTag:           names.NewUserTag("charlie@external").String(),
			Life:               life.Alive,
			Status: jujuparams.EntityStatus{
				Status: status.Available,
			},
			Users: []jujuparams.ModelUserInfo{{
				UserName:    "bob@external",
				DisplayName: "bob",
				Access:      jujuparams.ModelAdminAccess,
			}, {
				UserName:    "charlie@external",
				DisplayName: "charlie",
				Access:      jujuparams.ModelAdminAccess,
			}},
			AgentVersion: &jujuversion.Current,
			Type:         "iaas",
		},
	}, {
		Error: &jujuparams.Error{
			Message: `unauthorized`,
			Code:    jujuparams.CodeUnauthorized,
		},
	}})
}

func (s *modelManagerSuite) TestModelInfoForLegacyModel(c *gc.C) {
	ctx := context.Background()

	model := s.Model
	u := new(jimmdb.Update).Unset("cloud").Unset("cloudregion").Unset("cretential").Unset("defaultseries")
	err := s.JEM.DB.UpdateModel(ctx, &model, u, true)
	c.Assert(err, gc.Equals, nil)

	// Sanity check the required fields aren't present.
	err = s.JEM.DB.GetModel(ctx, &model)
	c.Assert(err, gc.Equals, nil)
	c.Assert(model.Cloud, gc.Equals, params.Cloud(""))

	conn := s.open(c, nil, "bob")
	defer conn.Close()
	client := modelmanager.NewClient(conn)
	models, err := client.ModelInfo([]names.ModelTag{names.NewModelTag(s.Model.UUID)})
	c.Assert(err, gc.Equals, nil)
	assertModelInfo(c, models, []jujuparams.ModelInfoResult{{
		Result: &jujuparams.ModelInfo{
			Name:               "model-1",
			UUID:               s.Model.UUID,
			ControllerUUID:     "914487b5-60e7-42bb-bd63-1adc3fd3a388",
			ProviderType:       "dummy",
			CloudTag:           "cloud-dummy",
			CloudRegion:        "dummy-region",
			CloudCredentialTag: conv.ToCloudCredentialTag(s.Credential.Path.ToParams()).String(),
			OwnerTag:           names.NewUserTag("bob@external").String(),
			Life:               life.Alive,
			Status: jujuparams.EntityStatus{
				Status: status.Available,
			},
			Users: []jujuparams.ModelUserInfo{{
				UserName:    "bob@external",
				DisplayName: "bob",
				Access:      jujuparams.ModelAdminAccess,
			}},
			AgentVersion:            &jujuversion.Current,
			Type:                    "iaas",
			CloudCredentialValidity: newBool(true),
			SLA: &jujuparams.ModelSLAInfo{
				Level: "unsupported",
			},
		},
	}})

	// Ensure the values in the database have been updated.
	err = s.JEM.DB.GetModel(ctx, &model)
	c.Assert(err, gc.Equals, nil)
	c.Assert(model.Cloud, gc.Equals, params.Cloud("dummy"))
	c.Assert(model.CloudRegion, gc.Equals, "dummy-region")
	c.Assert(model.Credential.String(), gc.Equals, s.Credential.Path.String())
	c.Assert(model.DefaultSeries, gc.Not(gc.Equals), "")
}

func (s *modelManagerSuite) TestModelInfoForLegacyModelDisableControllerUUIDMasking(c *gc.C) {
	ctx := context.Background()

	model := s.Model
	u := new(jimmdb.Update).Unset("cloud").Unset("cloudregion").Unset("cretential").Unset("defaultseries")
	err := s.JEM.DB.UpdateModel(ctx, &model, u, true)
	c.Assert(err, gc.Equals, nil)

	// Sanity check the required fields aren't present.
	err = s.JEM.DB.GetModel(ctx, &model)
	c.Assert(err, gc.Equals, nil)
	c.Assert(model.Cloud, gc.Equals, params.Cloud(""))

	s.Candid.AddUser("bob", string(s.JEM.ControllerAdmin()))
	conn := s.open(c, nil, "bob")
	defer conn.Close()
	client := modelmanager.NewClient(conn)

	err = conn.APICall("JIMM", 2, "", "DisableControllerUUIDMasking", nil, nil)
	c.Assert(err, gc.Equals, nil)

	models, err := client.ModelInfo([]names.ModelTag{names.NewModelTag(s.Model.UUID)})
	c.Assert(err, gc.Equals, nil)
	assertModelInfo(c, models, []jujuparams.ModelInfoResult{{
		Result: &jujuparams.ModelInfo{
			Name:               "model-1",
			UUID:               s.Model.UUID,
			ControllerUUID:     s.Controller.UUID,
			ProviderType:       "dummy",
			CloudTag:           "cloud-dummy",
			CloudRegion:        "dummy-region",
			CloudCredentialTag: conv.ToCloudCredentialTag(s.Credential.Path.ToParams()).String(),
			OwnerTag:           names.NewUserTag("bob@external").String(),
			Life:               life.Alive,
			Status: jujuparams.EntityStatus{
				Status: status.Available,
			},
			Users: []jujuparams.ModelUserInfo{{
				UserName:    "bob@external",
				DisplayName: "bob",
				Access:      jujuparams.ModelAdminAccess,
			}},
			AgentVersion:            &jujuversion.Current,
			Type:                    "iaas",
			CloudCredentialValidity: newBool(true),
			SLA: &jujuparams.ModelSLAInfo{
				Level: "unsupported",
			},
		},
	}})

	// Ensure the values in the database have been updated.
	err = s.JEM.DB.GetModel(ctx, &model)
	c.Assert(err, gc.Equals, nil)
	c.Assert(model.Cloud, gc.Equals, params.Cloud("dummy"))
	c.Assert(model.CloudRegion, gc.Equals, "dummy-region")
	c.Assert(model.Credential.String(), gc.Equals, s.Credential.Path.String())
	c.Assert(model.DefaultSeries, gc.Not(gc.Equals), "")
}

func (s *modelManagerSuite) TestModelInfoRequestTimeout(c *gc.C) {
	info := s.APIInfo(c)
	proxy := testing.NewTCPProxy(c, info.Addrs[0])
	hps, err := mongodoc.ParseAddresses([]string{proxy.Addr()})
	c.Assert(err, gc.Equals, nil)
	s.AddController(c, &mongodoc.Controller{
		Path:      params.EntityPath{User: "alice", Name: "dummy-2"},
		HostPorts: [][]mongodoc.HostPort{hps},
	})
	model := mongodoc.Model{
		Path:       params.EntityPath{User: "bob", Name: "model-2"},
		Controller: params.EntityPath{User: "alice", Name: "dummy-2"},
	}
	s.CreateModel(c, &model, nil, nil)

	conn := s.open(c, nil, "bob")
	defer conn.Close()
	client := modelmanager.NewClient(conn)

	models, err := client.ModelInfo([]names.ModelTag{
		names.NewModelTag(model.UUID),
	})
	c.Assert(err, gc.Equals, nil)

	assertModelInfo(c, models, []jujuparams.ModelInfoResult{{
		Result: &jujuparams.ModelInfo{
			Name:               "model-2",
			UUID:               model.UUID,
			ControllerUUID:     "914487b5-60e7-42bb-bd63-1adc3fd3a388",
			ProviderType:       "dummy",
			CloudTag:           "cloud-dummy",
			CloudRegion:        "dummy-region",
			CloudCredentialTag: names.NewCloudCredentialTag("dummy/bob@external/cred").String(),
			OwnerTag:           names.NewUserTag("bob@external").String(),
			Life:               life.Alive,
			Status: jujuparams.EntityStatus{
				Status: status.Available,
			},
			Users: []jujuparams.ModelUserInfo{{
				UserName:    "bob@external",
				DisplayName: "bob",
				Access:      jujuparams.ModelAdminAccess,
			}},
			AgentVersion:            &jujuversion.Current,
			Type:                    "iaas",
			CloudCredentialValidity: newBool(true),
			SLA: &jujuparams.ModelSLAInfo{
				Level: "unsupported",
			},
		},
	}})

	proxy.PauseConns()
	models, err = client.ModelInfo([]names.ModelTag{
		names.NewModelTag(model.UUID),
	})
	c.Assert(err, gc.Equals, nil)

	assertModelInfo(c, models, []jujuparams.ModelInfoResult{{
		Result: &jujuparams.ModelInfo{
			Name:               "model-2",
			UUID:               model.UUID,
			ControllerUUID:     "914487b5-60e7-42bb-bd63-1adc3fd3a388",
			ProviderType:       "dummy",
			CloudTag:           "cloud-dummy",
			CloudRegion:        "dummy-region",
			CloudCredentialTag: names.NewCloudCredentialTag("dummy/bob@external/cred").String(),
			OwnerTag:           names.NewUserTag("bob@external").String(),
			Life:               life.Alive,
			Status: jujuparams.EntityStatus{
				Status: status.Available,
			},
			Users: []jujuparams.ModelUserInfo{{
				UserName:    "bob@external",
				DisplayName: "bob",
				Access:      jujuparams.ModelAdminAccess,
			}},
			AgentVersion:            &jujuversion.Current,
			Type:                    "iaas",
			CloudCredentialValidity: newBool(true),
			SLA: &jujuparams.ModelSLAInfo{
				Level: "unsupported",
			},
		},
	}})

	proxy.ResumeConns()

	models, err = client.ModelInfo([]names.ModelTag{
		names.NewModelTag(model.UUID),
	})
	c.Assert(err, gc.Equals, nil)

	assertModelInfo(c, models, []jujuparams.ModelInfoResult{{
		Result: &jujuparams.ModelInfo{
			Name:               "model-2",
			UUID:               model.UUID,
			ControllerUUID:     "914487b5-60e7-42bb-bd63-1adc3fd3a388",
			ProviderType:       "dummy",
			CloudTag:           "cloud-dummy",
			CloudRegion:        "dummy-region",
			CloudCredentialTag: conv.ToCloudCredentialTag(s.Credential.Path.ToParams()).String(),
			OwnerTag:           names.NewUserTag("bob@external").String(),
			Life:               life.Alive,
			Status: jujuparams.EntityStatus{
				Status: status.Available,
			},
			Users: []jujuparams.ModelUserInfo{{
				UserName:    "bob@external",
				DisplayName: "bob",
				Access:      jujuparams.ModelAdminAccess,
			}},
			AgentVersion:            &jujuversion.Current,
			Type:                    "iaas",
			CloudCredentialValidity: newBool(true),
			SLA: &jujuparams.ModelSLAInfo{
				Level: "unsupported",
			},
		},
	}})
}

func (s *modelManagerSuite) TestModelInfoDyingModelNotFound(c *gc.C) {
	ctx := context.Background()

	err := s.JEM.DB.InsertModel(ctx, &mongodoc.Model{
		Controller:  s.Controller.Path,
		Path:        params.EntityPath{User: "alice", Name: "model-1"},
		UUID:        "00000000-0000-0000-0000-000000000007",
		Cloud:       params.Cloud("dummy"),
		CloudRegion: "dummy-region",
		Info: &mongodoc.ModelInfo{
			Life: string(life.Dying),
		},
	})
	c.Assert(err, gc.Equals, nil)

	conn := s.open(c, nil, "alice")
	defer conn.Close()
	client := modelmanager.NewClient(conn)

	models, err := client.ModelInfo([]names.ModelTag{
		names.NewModelTag("00000000-0000-0000-0000-000000000007"),
	})
	c.Assert(err, gc.Equals, nil)

	assertModelInfo(c, models, []jujuparams.ModelInfoResult{{
		Error: &jujuparams.Error{
			Message: `permission denied`,
			Code:    jujuparams.CodeUnauthorized,
		},
	}})

	err = s.JEM.DB.GetModel(ctx, &mongodoc.Model{Path: params.EntityPath{User: "alice", Name: "model-1"}})
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)
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
	cloudTag:      names.NewCloudTag("dummy").String(),
	credentialTag: "cloudcred-dummy_bob@external_cred",
}, {
	about:         "unauthorized user",
	name:          "model-2",
	ownerTag:      names.NewUserTag("charlie@external").String(),
	cloudTag:      names.NewCloudTag("dummy").String(),
	credentialTag: "cloudcred-dummy_bob@external_cred",
	expectError:   `unauthorized \(unauthorized access\)`,
}, {
	about:         "existing model name",
	name:          "model-1",
	ownerTag:      names.NewUserTag("bob@external").String(),
	cloudTag:      names.NewCloudTag("dummy").String(),
	credentialTag: "cloudcred-dummy_bob@external_cred",
	expectError:   "already exists",
}, {
	about:         "no controller",
	name:          "model-3",
	ownerTag:      names.NewUserTag("bob@external").String(),
	region:        "no-such-region",
	cloudTag:      names.NewCloudTag("dummy").String(),
	credentialTag: "",
	expectError:   `cloudregion not found \(not found\)`,
}, {
	about:         "local user",
	name:          "model-4",
	ownerTag:      names.NewUserTag("bob").String(),
	cloudTag:      names.NewCloudTag("dummy").String(),
	credentialTag: "cloudcred-dummy_bob@external_cred",
	expectError:   `unsupported local user \(user not found\)`,
}, {
	about:         "invalid user",
	name:          "model-5",
	ownerTag:      "user-bob/test@external",
	cloudTag:      names.NewCloudTag("dummy").String(),
	credentialTag: "cloudcred-dummy_bob@external_cred",
	expectError:   `"user-bob/test@external" is not a valid user tag \(bad request\)`,
}, {
	about:         "specific cloud",
	name:          "model-6",
	ownerTag:      names.NewUserTag("bob@external").String(),
	cloudTag:      names.NewCloudTag("dummy").String(),
	credentialTag: "cloudcred-dummy_bob@external_cred",
}, {
	about:         "specific cloud and region",
	name:          "model-7",
	ownerTag:      names.NewUserTag("bob@external").String(),
	cloudTag:      names.NewCloudTag("dummy").String(),
	region:        "dummy-region",
	credentialTag: "cloudcred-dummy_bob@external_cred",
}, {
	about:         "bad cloud tag",
	name:          "model-8",
	ownerTag:      names.NewUserTag("bob@external").String(),
	cloudTag:      "not-a-cloud-tag",
	credentialTag: "cloudcred-dummy_bob@external_cred1",
	expectError:   `invalid cloud tag: "not-a-cloud-tag" is not a valid tag \(bad request\)`,
}, {
	about:         "no cloud tag",
	name:          "model-8",
	ownerTag:      names.NewUserTag("bob@external").String(),
	cloudTag:      "",
	credentialTag: "cloudcred-dummy_bob@external_cred1",
	expectError:   `no cloud specified for model; please specify one`,
}, {
	about:    "no credential tag selects unambigous creds",
	name:     "model-8",
	ownerTag: names.NewUserTag("bob@external").String(),
	cloudTag: names.NewCloudTag("dummy").String(),
	region:   "dummy-region",
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
			c.Assert(mi.CloudTag, gc.Equals, "cloud-dummy")
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

	res, err := client2.ModelInfo([]names.ModelTag{names.NewModelTag(s.Model.UUID)})
	c.Assert(err, gc.Equals, nil)
	c.Assert(res, gc.HasLen, 1)
	c.Assert(res[0].Error, gc.ErrorMatches, "unauthorized")

	err = client.GrantModel("charlie@external", "write", s.Model.UUID)
	c.Assert(err, gc.Equals, nil)

	res, err = client2.ModelInfo([]names.ModelTag{names.NewModelTag(s.Model.UUID)})
	c.Assert(err, gc.Equals, nil)
	c.Assert(res, gc.HasLen, 1)
	c.Assert(res[0].Error, gc.IsNil)
	c.Assert(res[0].Result.UUID, gc.Equals, s.Model.UUID)

	err = client.RevokeModel("charlie@external", "read", s.Model.UUID)
	c.Assert(err, gc.Equals, nil)

	res, err = client2.ModelInfo([]names.ModelTag{names.NewModelTag(s.Model.UUID)})
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
			ModelTag: names.NewModelTag(s.Model2.UUID).String(),
		},
		expectError: `unauthorized`,
	}, {
		about: "bad user domain",
		modifyModelAccess: jujuparams.ModifyModelAccess{
			UserTag:  names.NewUserTag("eve@local").String(),
			Action:   jujuparams.GrantModelAccess,
			Access:   jujuparams.ModelReadAccess,
			ModelTag: names.NewModelTag(s.Model.UUID).String(),
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
			ModelTag: names.NewModelTag(s.Model.UUID).String(),
		},
		expectError: `"not-a-user-tag" is not a valid tag`,
	}, {
		about: "unknown action",
		modifyModelAccess: jujuparams.ModifyModelAccess{
			UserTag:  names.NewUserTag("eve@external").String(),
			Action:   "not-an-action",
			Access:   jujuparams.ModelReadAccess,
			ModelTag: names.NewModelTag(s.Model.UUID).String(),
		},
		expectError: `invalid action "not-an-action"`,
	}, {
		about: "invalid access",
		modifyModelAccess: jujuparams.ModifyModelAccess{
			UserTag:  names.NewUserTag("eve@external").String(),
			Action:   jujuparams.GrantModelAccess,
			Access:   "not-an-access",
			ModelTag: names.NewModelTag(s.Model.UUID).String(),
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
	tag := names.NewModelTag(s.Model.UUID)
	err := client.DestroyModel(tag, nil, nil, nil)
	c.Assert(err, gc.Equals, nil)

	// Check the model is now dying.
	mis, err := client.ModelInfo([]names.ModelTag{tag})
	c.Assert(err, gc.Equals, nil)
	c.Assert(mis, gc.HasLen, 1)
	c.Assert(mis[0].Error, gc.Equals, (*jujuparams.Error)(nil))
	c.Assert(mis[0].Result.Life, gc.Equals, life.Dying)

	// Kill the model.
	err = s.JEM.DB.RemoveModel(ctx, &mongodoc.Model{Controller: s.Controller.Path, UUID: s.Model.UUID})
	c.Assert(err, gc.Equals, nil)

	// Make sure it's not an error if you destroy a model that't not there.
	err = client.DestroyModel(names.NewModelTag(s.Model.UUID), nil, nil, nil)
	c.Assert(err, gc.Equals, nil)
}

func (s *modelManagerSuite) TestDestroyModelV3(c *gc.C) {
	conn := s.open(c, nil, "bob")
	defer conn.Close()

	tag := names.NewModelTag(s.Model.UUID)
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

	tag := names.NewModelTag(s.Model.UUID)
	client := modelmanager.NewClient(conn)
	res, err := client.DumpModel(tag, false)
	c.Check(err, gc.Equals, nil)
	c.Check(res, gc.Not(gc.HasLen), 0)
}

func (s *modelManagerSuite) TestDumpModelV2(c *gc.C) {
	conn := s.open(c, nil, "bob")
	defer conn.Close()
	var results jujuparams.MapResults
	err := conn.APICall("ModelManager", 2, "", "DumpModels", jujuparams.Entities{
		Entities: []jujuparams.Entity{{
			Tag: names.NewModelTag(s.Model.UUID).String(),
		}},
	}, &results)
	c.Assert(err, gc.Equals, nil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Check(results.Results[0].Error, gc.IsNil)
	c.Check(results.Results[0].Result, gc.Not(gc.HasLen), 0)
}

func (s *modelManagerSuite) TestDumpModelUnauthorized(c *gc.C) {
	conn := s.open(c, nil, "charlie")
	defer conn.Close()

	tag := names.NewModelTag(s.Model.UUID)
	client := modelmanager.NewClient(conn)
	res, err := client.DumpModel(tag, true)
	c.Check(err, gc.ErrorMatches, `unauthorized`)
	c.Check(res, gc.IsNil)
}

func (s *modelManagerSuite) TestDumpModelDB(c *gc.C) {
	conn := s.open(c, nil, "bob")
	defer conn.Close()

	tag := names.NewModelTag(s.Model.UUID)
	client := modelmanager.NewClient(conn)
	res, err := client.DumpModelDB(tag)
	c.Check(err, gc.Equals, nil)
	c.Check(res, gc.Not(gc.HasLen), 0)
}

func (s *modelManagerSuite) TestDumpModelDBUnauthorized(c *gc.C) {
	conn := s.open(c, nil, "charlie")
	defer conn.Close()

	tag := names.NewModelTag(s.Model.UUID)
	client := modelmanager.NewClient(conn)
	res, err := client.DumpModelDB(tag)
	c.Check(err, gc.ErrorMatches, `unauthorized`)
	c.Check(res, gc.IsNil)
}

func (s *modelManagerSuite) TestChangeModelCredential(c *gc.C) {
	conn := s.open(c, nil, "bob")
	defer conn.Close()

	modelTag := names.NewModelTag(s.Model.UUID)
	cred := jemtest.EmptyCredential("bob", "cred2")
	s.UpdateCredential(c, &cred)
	credTag := conv.ToCloudCredentialTag(cred.Path.ToParams())
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

	modelTag := names.NewModelTag(s.Model.UUID)
	credTag := names.NewCloudCredentialTag("dummy/bob@external/cred2")
	client := modelmanager.NewClient(conn)
	err := client.ChangeModelCredential(modelTag, credTag)
	c.Assert(err, gc.ErrorMatches, `unauthorized`)
}

func (s *modelManagerSuite) TestChangeModelCredentialUnauthorizedCredential(c *gc.C) {
	conn := s.open(c, nil, "bob")
	defer conn.Close()

	modelTag := names.NewModelTag(s.Model.UUID)
	credTag := names.NewCloudCredentialTag("dummy/alice@external/cred2")
	client := modelmanager.NewClient(conn)
	err := client.ChangeModelCredential(modelTag, credTag)
	c.Assert(err, gc.ErrorMatches, `unauthorized`)
}

func (s *modelManagerSuite) TestChangeModelCredentialNotFoundModel(c *gc.C) {
	conn := s.open(c, nil, "bob")
	defer conn.Close()

	modelTag := names.NewModelTag("000000000-0000-0000-0000-000000000000")
	credTag := names.NewCloudCredentialTag("dummy/bob@external/cred2")
	client := modelmanager.NewClient(conn)
	err := client.ChangeModelCredential(modelTag, credTag)
	c.Assert(err, gc.ErrorMatches, `model not found`)
}

func (s *modelManagerSuite) TestChangeModelCredentialNotFoundCredential(c *gc.C) {
	conn := s.open(c, nil, "bob")
	defer conn.Close()

	modelTag := names.NewModelTag(s.Model.UUID)
	credTag := names.NewCloudCredentialTag("dummy/bob@external/cred2")
	client := modelmanager.NewClient(conn)
	err := client.ChangeModelCredential(modelTag, credTag)
	c.Assert(err, gc.ErrorMatches, `credential not found`)
}

func (s *modelManagerSuite) TestChangeModelCredentialLocalUserCredential(c *gc.C) {
	conn := s.open(c, nil, "bob")
	defer conn.Close()

	modelTag := names.NewModelTag(s.Model.UUID)
	credTag := names.NewCloudCredentialTag("dummy/bob/cred2")
	client := modelmanager.NewClient(conn)
	err := client.ChangeModelCredential(modelTag, credTag)
	c.Assert(err, gc.ErrorMatches, `unsupported local user`)
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
	s.state, err = s.StatePool.Get(s.Model.UUID)
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

	tag := names.NewModelTag(s.Model.UUID)
	client := modelmanager.NewClient(conn)
	err := client.DestroyModel(tag, nil, nil, nil)
	c.Assert(errgo.Cause(err), jc.Satisfies, jujuparams.IsCodeHasPersistentStorage)

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

	tag := names.NewModelTag(s.Model.UUID)
	client := modelmanager.NewClient(conn)
	err := client.DestroyModel(tag, newBool(true), nil, nil)
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

	tag := names.NewModelTag(s.Model.UUID)
	client := modelmanager.NewClient(conn)
	err := client.DestroyModel(tag, newBool(false), nil, nil)
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
		HostCloudRegion: "dummy/dummy-region",
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
	conn := s.open(c, nil, "bob")
	defer conn.Close()

	client := modelmanager.NewClient(conn)
	mi, err := client.CreateModel("k8s-model-1", "bob@external", "bob-cloud", "", s.cred, nil)
	c.Assert(err, gc.Equals, nil)

	models, err := client.ListModelSummaries("bob", false)
	c.Assert(err, gc.Equals, nil)
	c.Assert(models, jemtest.CmpEquals(cmpopts.IgnoreTypes(&time.Time{})), []base.UserModelSummary{{
		Name:            "k8s-model-1",
		UUID:            mi.UUID,
		ControllerUUID:  "914487b5-60e7-42bb-bd63-1adc3fd3a388",
		ProviderType:    "kubernetes",
		DefaultSeries:   "bionic",
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
	}, {
		Name:            "model-1",
		UUID:            s.Model.UUID,
		Type:            "iaas",
		ControllerUUID:  "914487b5-60e7-42bb-bd63-1adc3fd3a388",
		ProviderType:    "dummy",
		DefaultSeries:   "bionic",
		Cloud:           "dummy",
		CloudRegion:     "dummy-region",
		CloudCredential: "dummy/bob@external/cred",
		Owner:           "bob@external",
		Life:            "alive",
		Status: base.Status{
			Status: status.Available,
			Data:   map[string]interface{}{},
		},
		ModelUserAccess: "admin",
		Counts:          []base.EntityCount{{Entity: "machines"}, {Entity: "cores"}, {Entity: "units"}},
		AgentVersion:    &jujuversion.Current,
	}, {
		Name:            "model-3",
		UUID:            s.Model3.UUID,
		Type:            "iaas",
		ControllerUUID:  "914487b5-60e7-42bb-bd63-1adc3fd3a388",
		ProviderType:    "dummy",
		DefaultSeries:   "bionic",
		Cloud:           "dummy",
		CloudRegion:     "dummy-region",
		CloudCredential: "dummy/charlie@external/cred",
		Owner:           "charlie@external",
		Life:            "alive",
		Status: base.Status{
			Status: status.Available,
			Data:   map[string]interface{}{},
		},
		ModelUserAccess: "read",
		Counts:          []base.EntityCount{{Entity: "machines"}, {Entity: "cores"}, {Entity: "units"}},
		AgentVersion:    &jujuversion.Current,
	}})
}

func (s *caasModelManagerSuite) TestListCAASModels(c *gc.C) {
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
		UUID:  s.Model.UUID,
		Owner: "bob@external",
		Type:  "iaas",
	}, {
		Name:  "model-3",
		UUID:  s.Model3.UUID,
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
