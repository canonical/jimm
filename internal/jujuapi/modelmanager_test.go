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
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing/factory"
	jujuversion "github.com/juju/juju/version"
	"github.com/juju/names/v4"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"
	errgo "gopkg.in/errgo.v1"
	"gopkg.in/mgo.v2/bson"

	"github.com/CanonicalLtd/jimm/internal/jemtest"
	"github.com/CanonicalLtd/jimm/internal/kubetest"
	"github.com/CanonicalLtd/jimm/internal/mongodoc"
	"github.com/CanonicalLtd/jimm/params"
)

type modelManagerSuite struct {
	websocketSuite
}

var _ = gc.Suite(&modelManagerSuite{})

func (s *modelManagerSuite) SetUpTest(c *gc.C) {
	s.ServerParams.CharmstoreLocation = "https://api.jujucharms.com/charmstore"
	s.ServerParams.MeteringLocation = "https://api.jujucharms.com/omnibus"
	s.websocketSuite.SetUpTest(c)
	s.PatchValue(&utils.OutgoingAccessAllowed, true)
}

func (s *modelManagerSuite) TestListModelSummaries(c *gc.C) {
	ctx := context.Background()

	ctlPath := s.AssertAddController(ctx, c, params.EntityPath{User: "test", Name: "controller-1"}, true)
	cred := s.AssertUpdateCredential(ctx, c, "test", "dummy", "cred1", "empty")
	cred2 := s.AssertUpdateCredential(ctx, c, "test2", "dummy", "cred1", "empty")
	err := s.JEM.DB.SetACL(ctx, s.JEM.DB.Controllers(), ctlPath, params.ACL{
		Read: []string{"test2"},
	})

	conn := s.open(c, nil, "test")
	defer conn.Close()

	c.Assert(err, gc.Equals, nil)
	mi := s.assertCreateModel(c, createModelParams{name: "model-1", username: "test", cred: cred})
	modelUUID1 := mi.UUID
	s.assertCreateModel(c, createModelParams{name: "model-2", username: "test2", cred: cred2})
	mi = s.assertCreateModel(c, createModelParams{name: "model-3", username: "test2", cred: cred2})
	modelUUID3 := mi.UUID
	err = s.JEM.DB.SetACL(ctx, s.JEM.DB.Models(), params.EntityPath{User: "test2", Name: "model-3"}, params.ACL{
		Read: []string{"test"},
	})
	c.Assert(err, gc.Equals, nil)

	client := modelmanager.NewClient(conn)
	models, err := client.ListModelSummaries("test", false)
	c.Assert(err, gc.Equals, nil)
	c.Assert(models, jemtest.CmpEquals(cmpopts.IgnoreTypes(&time.Time{})), []base.UserModelSummary{{
		Name:            "model-1",
		UUID:            modelUUID1,
		ControllerUUID:  "914487b5-60e7-42bb-bd63-1adc3fd3a388",
		ProviderType:    "dummy",
		DefaultSeries:   "focal",
		Cloud:           "dummy",
		CloudRegion:     "dummy-region",
		CloudCredential: "dummy/test@external/cred1",
		Owner:           "test@external",
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
		}},
		AgentVersion: &jujuversion.Current,
		Type:         "iaas",
	}, {
		Name:            "model-3",
		UUID:            modelUUID3,
		ControllerUUID:  "914487b5-60e7-42bb-bd63-1adc3fd3a388",
		ProviderType:    "dummy",
		DefaultSeries:   "focal",
		Cloud:           "dummy",
		CloudRegion:     "dummy-region",
		CloudCredential: "dummy/test2@external/cred1",
		Owner:           "test2@external",
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
		}},
		AgentVersion: &jujuversion.Current,
		Type:         "iaas",
	}})
}

func (s *modelManagerSuite) TestListModelSummariesWitouthControllerUUIDMasking(c *gc.C) {
	ctx := context.Background()
	ctlPath := s.AssertAddController(ctx, c, params.EntityPath{User: "test", Name: "controller-1"}, true)
	cred := s.AssertUpdateCredential(ctx, c, "test", "dummy", "cred1", "empty")
	cred2 := s.AssertUpdateCredential(ctx, c, "test2", "dummy", "cred1", "empty")
	err := s.JEM.DB.SetACL(ctx, s.JEM.DB.Controllers(), ctlPath, params.ACL{
		Read: []string{"test2"},
	})

	conn1 := s.open(c, nil, "test-unknown")
	defer conn1.Close()
	err = conn1.APICall("JIMM", 2, "", "DisableControllerUUIDMasking", nil, nil)
	c.Assert(err, gc.ErrorMatches, `unauthorized \(unauthorized access\)`)

	conn := s.open(c, nil, "test")
	defer conn.Close()
	err = conn.APICall("JIMM", 2, "", "DisableControllerUUIDMasking", nil, nil)
	c.Assert(err, gc.Equals, nil)

	c.Assert(err, gc.Equals, nil)
	mi := s.assertCreateModel(c, createModelParams{name: "model-1", username: "test", cred: cred})
	modelUUID1 := mi.UUID
	s.assertCreateModel(c, createModelParams{name: "model-2", username: "test2", cred: cred2})
	mi = s.assertCreateModel(c, createModelParams{name: "model-3", username: "test2", cred: cred2})
	modelUUID3 := mi.UUID
	err = s.JEM.DB.SetACL(ctx, s.JEM.DB.Models(), params.EntityPath{User: "test2", Name: "model-3"}, params.ACL{
		Read: []string{"test"},
	})
	c.Assert(err, gc.Equals, nil)

	client := modelmanager.NewClient(conn)
	models, err := client.ListModelSummaries("test", false)
	c.Assert(err, gc.Equals, nil)
	c.Assert(models, jemtest.CmpEquals(cmpopts.IgnoreTypes(&time.Time{})), []base.UserModelSummary{{
		Name:            "model-1",
		UUID:            modelUUID1,
		ControllerUUID:  "deadbeef-1bad-500d-9000-4b1d0d06f00d",
		ProviderType:    "dummy",
		DefaultSeries:   "focal",
		Cloud:           "dummy",
		CloudRegion:     "dummy-region",
		CloudCredential: "dummy/test@external/cred1",
		Owner:           "test@external",
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
		}},
		AgentVersion: &jujuversion.Current,
		Type:         "iaas",
	}, {
		Name:            "model-3",
		UUID:            modelUUID3,
		ControllerUUID:  "deadbeef-1bad-500d-9000-4b1d0d06f00d",
		ProviderType:    "dummy",
		DefaultSeries:   "focal",
		Cloud:           "dummy",
		CloudRegion:     "dummy-region",
		CloudCredential: "dummy/test2@external/cred1",
		Owner:           "test2@external",
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
		}},
		AgentVersion: &jujuversion.Current,
		Type:         "iaas",
	}})
}

func (s *modelManagerSuite) TestListModels(c *gc.C) {
	ctx := context.Background()

	ctlPath := s.AssertAddController(ctx, c, params.EntityPath{User: "test", Name: "controller-1"}, true)
	cred := s.AssertUpdateCredential(ctx, c, "test", "dummy", "cred1", "empty")
	cred2 := s.AssertUpdateCredential(ctx, c, "test2", "dummy", "cred1", "empty")
	err := s.JEM.DB.SetACL(ctx, s.JEM.DB.Controllers(), ctlPath, params.ACL{
		Read: []string{"test2"},
	})
	c.Assert(err, gc.Equals, nil)

	conn := s.open(c, nil, "test")
	defer conn.Close()

	mi := s.assertCreateModel(c, createModelParams{name: "model-1", username: "test", cred: cred})
	modelUUID1 := mi.UUID
	s.assertCreateModel(c, createModelParams{name: "model-2", username: "test2", cred: cred2})
	mi = s.assertCreateModel(c, createModelParams{name: "model-3", username: "test2", cred: cred2})
	modelUUID3 := mi.UUID
	err = s.JEM.DB.SetACL(ctx, s.JEM.DB.Models(), params.EntityPath{User: "test2", Name: "model-3"}, params.ACL{
		Read: []string{"test"},
	})
	c.Assert(err, gc.Equals, nil)

	client := modelmanager.NewClient(conn)
	models, err := client.ListModels("test")
	c.Assert(err, gc.Equals, nil)
	c.Assert(models, jc.DeepEquals, []base.UserModel{{
		Name:  "model-1",
		UUID:  modelUUID1,
		Owner: "test@external",
		Type:  "iaas",
	}, {
		Name:  "model-3",
		UUID:  modelUUID3,
		Owner: "test2@external",
		Type:  "iaas",
	}})
}

func (s *modelManagerSuite) TestModelInfo(c *gc.C) {
	ctx := context.Background()

	ctlPath := s.AssertAddController(ctx, c, params.EntityPath{User: "test", Name: "controller-1"}, true)
	s.AssertUpdateCredential(ctx, c, "test", "dummy", "cred1", "empty")
	s.AssertUpdateCredential(ctx, c, "test2", "dummy", "cred1", "empty")
	err := s.JEM.DB.SetACL(ctx, s.JEM.DB.Controllers(), ctlPath, params.ACL{
		Read: []string{"test2"},
	})

	c.Assert(err, gc.Equals, nil)

	mi := s.assertCreateModel(c, createModelParams{name: "model-1", username: "test", cred: "cred1"})
	modelUUID1 := mi.UUID
	mi = s.assertCreateModel(c, createModelParams{name: "model-2", username: "test2", cred: "cred1"})
	modelUUID2 := mi.UUID
	mi = s.assertCreateModel(c, createModelParams{name: "model-3", username: "test2", cred: "cred1"})
	modelUUID3 := mi.UUID
	mi = s.assertCreateModel(c, createModelParams{name: "model-4", username: "test2", cred: "cred1"})
	modelUUID4 := mi.UUID
	mi = s.assertCreateModel(c, createModelParams{name: "model-5", username: "test2", cred: "cred1"})
	modelUUID5 := mi.UUID

	s.grant(c, params.EntityPath{User: "test2", Name: "model-3"}, params.User("test"), "read")
	s.grant(c, params.EntityPath{User: "test2", Name: "model-4"}, params.User("test"), "write")
	s.grant(c, params.EntityPath{User: "test2", Name: "model-5"}, params.User("test"), "admin")

	// Add some machines to one of the models
	err = s.JEM.UpdateMachineInfo(ctx, ctlPath, &jujuparams.MachineInfo{
		ModelUUID: modelUUID3,
		Id:        "machine-0",
	})
	c.Assert(err, gc.Equals, nil)
	machineArch := "bbc-micro"
	err = s.JEM.UpdateMachineInfo(ctx, ctlPath, &jujuparams.MachineInfo{
		ModelUUID: modelUUID3,
		Id:        "machine-1",
		HardwareCharacteristics: &instance.HardwareCharacteristics{
			Arch: &machineArch,
		},
	})
	c.Assert(err, gc.Equals, nil)
	err = s.JEM.UpdateMachineInfo(ctx, ctlPath, &jujuparams.MachineInfo{
		ModelUUID: modelUUID3,
		Id:        "machine-2",
		Life:      "dead",
	})
	c.Assert(err, gc.Equals, nil)
	err = s.JEM.UpdateMachineInfo(ctx, ctlPath, &jujuparams.MachineInfo{
		ModelUUID: modelUUID4,
		Id:        "machine-0",
	})
	c.Assert(err, gc.Equals, nil)
	err = s.JEM.UpdateMachineInfo(ctx, ctlPath, &jujuparams.MachineInfo{
		ModelUUID: modelUUID4,
		Id:        "machine-1",
		HardwareCharacteristics: &instance.HardwareCharacteristics{
			Arch: &machineArch,
		},
	})
	c.Assert(err, gc.Equals, nil)
	err = s.JEM.UpdateMachineInfo(ctx, ctlPath, &jujuparams.MachineInfo{
		ModelUUID: modelUUID4,
		Id:        "machine-2",
		Life:      "dead",
	})
	c.Assert(err, gc.Equals, nil)

	conn := s.open(c, nil, "test")
	defer conn.Close()
	client := modelmanager.NewClient(conn)

	models, err := client.ModelInfo([]names.ModelTag{
		names.NewModelTag(modelUUID1),
		names.NewModelTag(modelUUID2),
		names.NewModelTag(modelUUID3),
		names.NewModelTag(modelUUID4),
		names.NewModelTag(modelUUID5),
		names.NewModelTag("00000000-0000-0000-0000-000000000007"),
	})
	c.Assert(err, gc.Equals, nil)

	assertModelInfo(c, models, []jujuparams.ModelInfoResult{{
		Result: &jujuparams.ModelInfo{
			Name:               "model-1",
			UUID:               modelUUID1,
			ControllerUUID:     "914487b5-60e7-42bb-bd63-1adc3fd3a388",
			ProviderType:       "dummy",
			CloudTag:           "cloud-dummy",
			CloudRegion:        "dummy-region",
			CloudCredentialTag: names.NewCloudCredentialTag("dummy/test@external/cred1").String(),
			OwnerTag:           names.NewUserTag("test@external").String(),
			Life:               life.Alive,
			Status: jujuparams.EntityStatus{
				Status: status.Available,
			},
			Users: []jujuparams.ModelUserInfo{{
				UserName:    "test@external",
				DisplayName: "test",
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
			UUID:               modelUUID3,
			ControllerUUID:     "914487b5-60e7-42bb-bd63-1adc3fd3a388",
			ProviderType:       "dummy",
			CloudTag:           "cloud-dummy",
			CloudRegion:        "dummy-region",
			CloudCredentialTag: names.NewCloudCredentialTag("dummy/test2@external/cred1").String(),
			OwnerTag:           names.NewUserTag("test2@external").String(),
			Life:               life.Alive,
			Status: jujuparams.EntityStatus{
				Status: status.Available,
			},
			Users: []jujuparams.ModelUserInfo{{
				UserName:    "test@external",
				DisplayName: "test",
				Access:      jujuparams.ModelReadAccess,
			}},
			AgentVersion: &jujuversion.Current,
			Type:         "iaas",
		},
	}, {
		Result: &jujuparams.ModelInfo{
			Name:               "model-4",
			UUID:               modelUUID4,
			ControllerUUID:     "914487b5-60e7-42bb-bd63-1adc3fd3a388",
			ProviderType:       "dummy",
			CloudTag:           "cloud-dummy",
			CloudRegion:        "dummy-region",
			CloudCredentialTag: names.NewCloudCredentialTag("dummy/test2@external/cred1").String(),
			OwnerTag:           names.NewUserTag("test2@external").String(),
			Life:               life.Alive,
			Status: jujuparams.EntityStatus{
				Status: status.Available,
			},
			Users: []jujuparams.ModelUserInfo{{
				UserName:    "test@external",
				DisplayName: "test",
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
			UUID:               modelUUID5,
			ControllerUUID:     "914487b5-60e7-42bb-bd63-1adc3fd3a388",
			ProviderType:       "dummy",
			CloudTag:           "cloud-dummy",
			CloudRegion:        "dummy-region",
			CloudCredentialTag: names.NewCloudCredentialTag("dummy/test2@external/cred1").String(),
			OwnerTag:           names.NewUserTag("test2@external").String(),
			Life:               life.Alive,
			Status: jujuparams.EntityStatus{
				Status: status.Available,
			},
			Users: []jujuparams.ModelUserInfo{{
				UserName:    "test2@external",
				DisplayName: "test2",
				Access:      jujuparams.ModelAdminAccess,
			}, {
				UserName:    "test@external",
				DisplayName: "test",
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
	ctlPath := s.AssertAddController(ctx, c, params.EntityPath{User: "test", Name: "controller-1"}, true)
	s.AssertUpdateCredential(ctx, c, "test", "dummy", "cred1", "empty")
	s.AssertUpdateCredential(ctx, c, "test2", "dummy", "cred1", "empty")
	err := s.JEM.DB.SetACL(ctx, s.JEM.DB.Controllers(), ctlPath, params.ACL{
		Read: []string{"test2"},
	})

	c.Assert(err, gc.Equals, nil)

	mi := s.assertCreateModel(c, createModelParams{name: "model-1", username: "test", cred: "cred1"})
	modelUUID1 := mi.UUID
	mi = s.assertCreateModel(c, createModelParams{name: "model-2", username: "test2", cred: "cred1"})
	modelUUID2 := mi.UUID
	mi = s.assertCreateModel(c, createModelParams{name: "model-3", username: "test2", cred: "cred1"})
	modelUUID3 := mi.UUID
	mi = s.assertCreateModel(c, createModelParams{name: "model-4", username: "test2", cred: "cred1"})
	modelUUID4 := mi.UUID
	mi = s.assertCreateModel(c, createModelParams{name: "model-5", username: "test2", cred: "cred1"})
	modelUUID5 := mi.UUID

	s.grant(c, params.EntityPath{User: "test2", Name: "model-3"}, params.User("test"), "read")
	s.grant(c, params.EntityPath{User: "test2", Name: "model-4"}, params.User("test"), "write")
	s.grant(c, params.EntityPath{User: "test2", Name: "model-5"}, params.User("test"), "admin")

	// Add some machines to one of the models
	err = s.JEM.UpdateMachineInfo(ctx, ctlPath, &jujuparams.MachineInfo{
		ModelUUID: modelUUID3,
		Id:        "machine-0",
	})
	c.Assert(err, gc.Equals, nil)
	machineArch := "bbc-micro"
	err = s.JEM.UpdateMachineInfo(ctx, ctlPath, &jujuparams.MachineInfo{
		ModelUUID: modelUUID3,
		Id:        "machine-1",
		HardwareCharacteristics: &instance.HardwareCharacteristics{
			Arch: &machineArch,
		},
	})
	c.Assert(err, gc.Equals, nil)
	err = s.JEM.UpdateMachineInfo(ctx, ctlPath, &jujuparams.MachineInfo{
		ModelUUID: modelUUID3,
		Id:        "machine-2",
		Life:      "dead",
	})
	c.Assert(err, gc.Equals, nil)
	err = s.JEM.UpdateMachineInfo(ctx, ctlPath, &jujuparams.MachineInfo{
		ModelUUID: modelUUID4,
		Id:        "machine-0",
	})
	c.Assert(err, gc.Equals, nil)
	err = s.JEM.UpdateMachineInfo(ctx, ctlPath, &jujuparams.MachineInfo{
		ModelUUID: modelUUID4,
		Id:        "machine-1",
		HardwareCharacteristics: &instance.HardwareCharacteristics{
			Arch: &machineArch,
		},
	})
	c.Assert(err, gc.Equals, nil)
	err = s.JEM.UpdateMachineInfo(ctx, ctlPath, &jujuparams.MachineInfo{
		ModelUUID: modelUUID4,
		Id:        "machine-2",
		Life:      "dead",
	})
	c.Assert(err, gc.Equals, nil)

	conn := s.open(c, nil, "test")
	defer conn.Close()
	client := modelmanager.NewClient(conn)

	err = conn.APICall("JIMM", 2, "", "DisableControllerUUIDMasking", nil, nil)
	c.Assert(err, gc.Equals, nil)

	models, err := client.ModelInfo([]names.ModelTag{
		names.NewModelTag(modelUUID1),
		names.NewModelTag(modelUUID2),
		names.NewModelTag(modelUUID3),
		names.NewModelTag(modelUUID4),
		names.NewModelTag(modelUUID5),
		names.NewModelTag("00000000-0000-0000-0000-000000000007"),
	})
	c.Assert(err, gc.Equals, nil)

	assertModelInfo(c, models, []jujuparams.ModelInfoResult{{
		Result: &jujuparams.ModelInfo{
			Name:               "model-1",
			UUID:               modelUUID1,
			ControllerUUID:     "deadbeef-1bad-500d-9000-4b1d0d06f00d",
			ProviderType:       "dummy",
			CloudTag:           "cloud-dummy",
			CloudRegion:        "dummy-region",
			CloudCredentialTag: names.NewCloudCredentialTag("dummy/test@external/cred1").String(),
			OwnerTag:           names.NewUserTag("test@external").String(),
			Life:               life.Alive,
			Status: jujuparams.EntityStatus{
				Status: status.Available,
			},
			Users: []jujuparams.ModelUserInfo{{
				UserName:    "test@external",
				DisplayName: "test",
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
			UUID:               modelUUID3,
			ControllerUUID:     "deadbeef-1bad-500d-9000-4b1d0d06f00d",
			ProviderType:       "dummy",
			CloudTag:           "cloud-dummy",
			CloudRegion:        "dummy-region",
			CloudCredentialTag: names.NewCloudCredentialTag("dummy/test2@external/cred1").String(),
			OwnerTag:           names.NewUserTag("test2@external").String(),
			Life:               life.Alive,
			Status: jujuparams.EntityStatus{
				Status: status.Available,
			},
			Users: []jujuparams.ModelUserInfo{{
				UserName:    "test@external",
				DisplayName: "test",
				Access:      jujuparams.ModelReadAccess,
			}},
			AgentVersion: &jujuversion.Current,
			Type:         "iaas",
		},
	}, {
		Result: &jujuparams.ModelInfo{
			Name:               "model-4",
			UUID:               modelUUID4,
			ControllerUUID:     "deadbeef-1bad-500d-9000-4b1d0d06f00d",
			ProviderType:       "dummy",
			CloudTag:           "cloud-dummy",
			CloudRegion:        "dummy-region",
			CloudCredentialTag: names.NewCloudCredentialTag("dummy/test2@external/cred1").String(),
			OwnerTag:           names.NewUserTag("test2@external").String(),
			Life:               life.Alive,
			Status: jujuparams.EntityStatus{
				Status: status.Available,
			},
			Users: []jujuparams.ModelUserInfo{{
				UserName:    "test@external",
				DisplayName: "test",
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
			UUID:               modelUUID5,
			ControllerUUID:     "deadbeef-1bad-500d-9000-4b1d0d06f00d",
			ProviderType:       "dummy",
			CloudTag:           "cloud-dummy",
			CloudRegion:        "dummy-region",
			CloudCredentialTag: names.NewCloudCredentialTag("dummy/test2@external/cred1").String(),
			OwnerTag:           names.NewUserTag("test2@external").String(),
			Life:               life.Alive,
			Status: jujuparams.EntityStatus{
				Status: status.Available,
			},
			Users: []jujuparams.ModelUserInfo{{
				UserName:    "test2@external",
				DisplayName: "test2",
				Access:      jujuparams.ModelAdminAccess,
			}, {
				UserName:    "test@external",
				DisplayName: "test",
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

	s.AssertAddController(ctx, c, params.EntityPath{User: "test", Name: "controller-1"}, true)
	s.AssertUpdateCredential(ctx, c, "test", "dummy", "cred1", "empty")
	mi := s.assertCreateModel(c, createModelParams{name: "model-1", username: "test", cred: "cred1"})
	modelUUID1 := mi.UUID

	err := s.JEM.DB.Models().UpdateId("test/model-1", bson.D{{
		"$unset",
		bson.D{{
			"cloud", 1,
		}, {
			"cloudregion", 1,
		}, {
			"credential", 1,
		}, {
			"defaultseries", 1,
		}},
	}})
	c.Assert(err, gc.Equals, nil)

	// Sanity check the required fields aren't present.
	model := mongodoc.Model{Path: params.EntityPath{"test", "model-1"}}
	err = s.JEM.DB.GetModel(ctx, &model)
	c.Assert(err, gc.Equals, nil)
	c.Assert(model.Cloud, gc.Equals, params.Cloud(""))

	conn := s.open(c, nil, "test")
	defer conn.Close()
	client := modelmanager.NewClient(conn)
	models, err := client.ModelInfo([]names.ModelTag{names.NewModelTag(modelUUID1)})
	c.Assert(err, gc.Equals, nil)
	assertModelInfo(c, models, []jujuparams.ModelInfoResult{{
		Result: &jujuparams.ModelInfo{
			Name:               "model-1",
			UUID:               modelUUID1,
			ControllerUUID:     "914487b5-60e7-42bb-bd63-1adc3fd3a388",
			ProviderType:       "dummy",
			CloudTag:           "cloud-dummy",
			CloudRegion:        "dummy-region",
			CloudCredentialTag: names.NewCloudCredentialTag("dummy/test@external/cred1").String(),
			OwnerTag:           names.NewUserTag("test@external").String(),
			Life:               life.Alive,
			Status: jujuparams.EntityStatus{
				Status: status.Available,
			},
			Users: []jujuparams.ModelUserInfo{{
				UserName:    "test@external",
				DisplayName: "test",
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
	c.Assert(model.Credential.String(), gc.Equals, "dummy/test/cred1")
	c.Assert(model.DefaultSeries, gc.Not(gc.Equals), "")
}

func (s *modelManagerSuite) TestModelInfoForLegacyModelDisableControllerUUIDMasking(c *gc.C) {
	ctx := context.Background()
	s.AssertAddController(ctx, c, params.EntityPath{User: "test", Name: "controller-1"}, true)
	s.AssertUpdateCredential(ctx, c, "test", "dummy", "cred1", "empty")
	mi := s.assertCreateModel(c, createModelParams{name: "model-1", username: "test", cred: "cred1"})
	modelUUID1 := mi.UUID

	err := s.JEM.DB.Models().UpdateId("test/model-1", bson.D{{
		"$unset",
		bson.D{{
			"cloud", 1,
		}, {
			"cloudregion", 1,
		}, {
			"credential", 1,
		}, {
			"defaultseries", 1,
		}},
	}})
	c.Assert(err, gc.Equals, nil)

	// Sanity check the required fields aren't present.
	model := mongodoc.Model{Path: params.EntityPath{"test", "model-1"}}
	err = s.JEM.DB.GetModel(ctx, &model)
	c.Assert(err, gc.Equals, nil)
	c.Assert(model.Cloud, gc.Equals, params.Cloud(""))

	conn := s.open(c, nil, "test")
	defer conn.Close()
	client := modelmanager.NewClient(conn)

	err = conn.APICall("JIMM", 2, "", "DisableControllerUUIDMasking", nil, nil)
	c.Assert(err, gc.Equals, nil)

	models, err := client.ModelInfo([]names.ModelTag{names.NewModelTag(modelUUID1)})
	c.Assert(err, gc.Equals, nil)
	assertModelInfo(c, models, []jujuparams.ModelInfoResult{{
		Result: &jujuparams.ModelInfo{
			Name:               "model-1",
			UUID:               modelUUID1,
			ControllerUUID:     "deadbeef-1bad-500d-9000-4b1d0d06f00d",
			ProviderType:       "dummy",
			CloudTag:           "cloud-dummy",
			CloudRegion:        "dummy-region",
			CloudCredentialTag: names.NewCloudCredentialTag("dummy/test@external/cred1").String(),
			OwnerTag:           names.NewUserTag("test@external").String(),
			Life:               life.Alive,
			Status: jujuparams.EntityStatus{
				Status: status.Available,
			},
			Users: []jujuparams.ModelUserInfo{{
				UserName:    "test@external",
				DisplayName: "test",
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
	c.Assert(model.Credential.String(), gc.Equals, "dummy/test/cred1")
	c.Assert(model.DefaultSeries, gc.Not(gc.Equals), "")
}

func (s *modelManagerSuite) TestModelInfoRequestTimeout(c *gc.C) {
	ctx := context.Background()

	info := s.APIInfo(c)
	proxy := testing.NewTCPProxy(c, info.Addrs[0])
	p := &params.AddController{
		EntityPath: params.EntityPath{User: "test", Name: "controller-1"},
		Info: params.ControllerInfo{
			HostPorts:      []string{proxy.Addr()},
			CACert:         info.CACert,
			User:           info.Tag.Id(),
			Password:       info.Password,
			ControllerUUID: s.ControllerConfig.ControllerUUID(),
			Public:         true,
		},
	}
	s.IDMSrv.AddUser("test", "controller-admin")
	err := s.NewClient("test").AddController(ctx, p)
	c.Assert(err, gc.Equals, nil)
	s.AssertUpdateCredential(ctx, c, "test", "dummy", "cred1", "empty")

	mi := s.assertCreateModel(c, createModelParams{name: "model-1", username: "test", cred: "cred1"})

	conn := s.open(c, nil, "test")
	defer conn.Close()
	client := modelmanager.NewClient(conn)

	models, err := client.ModelInfo([]names.ModelTag{
		names.NewModelTag(mi.UUID),
	})
	c.Assert(err, gc.Equals, nil)

	assertModelInfo(c, models, []jujuparams.ModelInfoResult{{
		Result: &jujuparams.ModelInfo{
			Name:               "model-1",
			UUID:               mi.UUID,
			ControllerUUID:     "914487b5-60e7-42bb-bd63-1adc3fd3a388",
			ProviderType:       "dummy",
			CloudTag:           "cloud-dummy",
			CloudRegion:        "dummy-region",
			CloudCredentialTag: names.NewCloudCredentialTag("dummy/test@external/cred1").String(),
			OwnerTag:           names.NewUserTag("test@external").String(),
			Life:               life.Alive,
			Status: jujuparams.EntityStatus{
				Status: status.Available,
			},
			Users: []jujuparams.ModelUserInfo{{
				UserName:    "test@external",
				DisplayName: "test",
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
		names.NewModelTag(mi.UUID),
	})
	c.Assert(err, gc.Equals, nil)

	assertModelInfo(c, models, []jujuparams.ModelInfoResult{{
		Result: &jujuparams.ModelInfo{
			Name:               "model-1",
			UUID:               mi.UUID,
			ControllerUUID:     "914487b5-60e7-42bb-bd63-1adc3fd3a388",
			ProviderType:       "dummy",
			CloudTag:           "cloud-dummy",
			CloudRegion:        "dummy-region",
			CloudCredentialTag: names.NewCloudCredentialTag("dummy/test@external/cred1").String(),
			OwnerTag:           names.NewUserTag("test@external").String(),
			Life:               life.Alive,
			Status: jujuparams.EntityStatus{
				Status: status.Available,
			},
			Users: []jujuparams.ModelUserInfo{{
				UserName:    "test@external",
				DisplayName: "test",
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
		names.NewModelTag(mi.UUID),
	})
	c.Assert(err, gc.Equals, nil)

	assertModelInfo(c, models, []jujuparams.ModelInfoResult{{
		Result: &jujuparams.ModelInfo{
			Name:               "model-1",
			UUID:               mi.UUID,
			ControllerUUID:     "914487b5-60e7-42bb-bd63-1adc3fd3a388",
			ProviderType:       "dummy",
			CloudTag:           "cloud-dummy",
			CloudRegion:        "dummy-region",
			CloudCredentialTag: names.NewCloudCredentialTag("dummy/test@external/cred1").String(),
			OwnerTag:           names.NewUserTag("test@external").String(),
			Life:               life.Alive,
			Status: jujuparams.EntityStatus{
				Status: status.Available,
			},
			Users: []jujuparams.ModelUserInfo{{
				UserName:    "test@external",
				DisplayName: "test",
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

	ctlPath := s.AssertAddController(ctx, c, params.EntityPath{User: "alice", Name: "controller-1"}, true)
	s.AssertUpdateCredential(ctx, c, "alice", "dummy", "cred1", "empty")

	err := s.JEM.DB.AddModel(ctx, &mongodoc.Model{
		Controller:  ctlPath,
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
	ownerTag:      "user-test@external",
	cloudTag:      names.NewCloudTag("dummy").String(),
	credentialTag: "cloudcred-dummy_test@external_cred1",
}, {
	about:         "unauthorized user",
	name:          "model-2",
	ownerTag:      "user-not-test@external",
	cloudTag:      names.NewCloudTag("dummy").String(),
	credentialTag: "cloudcred-dummy_test@external_cred1",
	expectError:   `unauthorized \(unauthorized access\)`,
}, {
	about:         "existing model name",
	name:          "existing-model",
	ownerTag:      "user-test@external",
	cloudTag:      names.NewCloudTag("dummy").String(),
	credentialTag: "cloudcred-dummy_test@external_cred1",
	expectError:   "already exists",
}, {
	about:         "no controller",
	name:          "model-3",
	ownerTag:      "user-test@external",
	region:        "no-such-region",
	cloudTag:      names.NewCloudTag("dummy").String(),
	credentialTag: "",
	expectError:   `cloudregion not found \(not found\)`,
}, {
	about:         "local user",
	name:          "model-4",
	ownerTag:      "user-test@local",
	cloudTag:      names.NewCloudTag("dummy").String(),
	credentialTag: "cloudcred-dummy_test@external_cred1",
	expectError:   `unsupported local user \(user not found\)`,
}, {
	about:         "invalid user",
	name:          "model-5",
	ownerTag:      "user-test/test@external",
	cloudTag:      names.NewCloudTag("dummy").String(),
	credentialTag: "cloudcred-dummy_test@external_cred1",
	expectError:   `"user-test/test@external" is not a valid user tag \(bad request\)`,
}, {
	about:         "specific cloud",
	name:          "model-6",
	ownerTag:      "user-test@external",
	cloudTag:      names.NewCloudTag("dummy").String(),
	credentialTag: "cloudcred-dummy_test@external_cred1",
}, {
	about:         "specific cloud and region",
	name:          "model-7",
	ownerTag:      "user-test@external",
	cloudTag:      names.NewCloudTag("dummy").String(),
	region:        "dummy-region",
	credentialTag: "cloudcred-dummy_test@external_cred1",
}, {
	about:         "bad cloud tag",
	name:          "model-8",
	ownerTag:      "user-test@external",
	cloudTag:      "not-a-cloud-tag",
	credentialTag: "cloudcred-dummy_test@external_cred1",
	expectError:   `invalid cloud tag: "not-a-cloud-tag" is not a valid tag \(bad request\)`,
}, {
	about:         "no cloud tag",
	name:          "model-8",
	ownerTag:      "user-test@external",
	cloudTag:      "",
	credentialTag: "cloudcred-dummy_test@external_cred1",
	expectError:   `no cloud specified for model; please specify one`,
}, {
	about:         "no credential tag selects unambigous creds",
	name:          "model-8",
	ownerTag:      "user-test@external",
	cloudTag:      names.NewCloudTag("dummy").String(),
	region:        "dummy-region",
	credentialTag: "cloudcred-dummy_test@external_cred1",
}}

func (s *modelManagerSuite) TestCreateModel(c *gc.C) {
	ctx := context.Background()

	s.AssertAddController(ctx, c, params.EntityPath{User: "test", Name: "controller-1"}, true)
	s.AssertUpdateCredential(ctx, c, "test", "dummy", "cred1", "empty")
	s.assertCreateModel(c, createModelParams{name: "existing-model", username: "test", cred: "cred1"})

	conn := s.open(c, nil, "test")
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
			c.Assert(mi.CloudCredentialTag, gc.Equals, "")
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
	ctx := context.Background()

	s.AssertAddController(ctx, c, params.EntityPath{User: "test", Name: "controller-1"}, true)
	s.AssertUpdateCredential(ctx, c, "test", "dummy", "cred1", "empty")
	mi := s.assertCreateModel(c, createModelParams{name: "test-model", username: "test", cred: "cred1"})

	conn := s.open(c, nil, "test")
	defer conn.Close()
	client := modelmanager.NewClient(conn)

	conn2 := s.open(c, nil, "bob")
	defer conn2.Close()
	client2 := modelmanager.NewClient(conn2)

	res, err := client2.ModelInfo([]names.ModelTag{names.NewModelTag(mi.UUID)})
	c.Assert(err, gc.Equals, nil)
	c.Assert(res, gc.HasLen, 1)
	c.Assert(res[0].Error, gc.ErrorMatches, "unauthorized")

	err = client.GrantModel("bob@external", "write", mi.UUID)
	c.Assert(err, gc.Equals, nil)

	res, err = client2.ModelInfo([]names.ModelTag{names.NewModelTag(mi.UUID)})
	c.Assert(err, gc.Equals, nil)
	c.Assert(res, gc.HasLen, 1)
	c.Assert(res[0].Error, gc.IsNil)
	c.Assert(res[0].Result.UUID, gc.Equals, mi.UUID)

	err = client.RevokeModel("bob@external", "read", mi.UUID)
	c.Assert(err, gc.Equals, nil)

	res, err = client2.ModelInfo([]names.ModelTag{names.NewModelTag(mi.UUID)})
	c.Assert(err, gc.Equals, nil)
	c.Assert(res, gc.HasLen, 1)
	c.Assert(res[0].Error, gc.Not(gc.IsNil))
	c.Assert(res[0].Error, gc.ErrorMatches, "unauthorized")
}

func (s *modelManagerSuite) TestModifyModelAccessErrors(c *gc.C) {
	ctx := context.Background()

	s.AssertAddController(ctx, c, params.EntityPath{User: "alice", Name: "controller-1"}, true)
	s.AssertAddController(ctx, c, params.EntityPath{User: "bob", Name: "controller-1"}, true)
	s.AssertUpdateCredential(ctx, c, "alice", "dummy", "cred1", "empty")
	s.AssertUpdateCredential(ctx, c, "bob", "dummy", "cred1", "empty")
	mi := s.assertCreateModel(c, createModelParams{name: "test-model", username: "alice", cred: "cred1"})
	mi2 := s.assertCreateModel(c, createModelParams{name: "test-model", username: "bob", cred: "cred1"})

	conn := s.open(c, nil, "alice")
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
			ModelTag: names.NewModelTag(mi2.UUID).String(),
		},
		expectError: `unauthorized`,
	}, {
		about: "bad user domain",
		modifyModelAccess: jujuparams.ModifyModelAccess{
			UserTag:  names.NewUserTag("eve@local").String(),
			Action:   jujuparams.GrantModelAccess,
			Access:   jujuparams.ModelReadAccess,
			ModelTag: names.NewModelTag(mi.UUID).String(),
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
			ModelTag: names.NewModelTag(mi.UUID).String(),
		},
		expectError: `"not-a-user-tag" is not a valid tag`,
	}, {
		about: "unknown action",
		modifyModelAccess: jujuparams.ModifyModelAccess{
			UserTag:  names.NewUserTag("eve@external").String(),
			Action:   "not-an-action",
			Access:   jujuparams.ModelReadAccess,
			ModelTag: names.NewModelTag(mi.UUID).String(),
		},
		expectError: `invalid action "not-an-action"`,
	}, {
		about: "invalid access",
		modifyModelAccess: jujuparams.ModifyModelAccess{
			UserTag:  names.NewUserTag("eve@external").String(),
			Action:   jujuparams.GrantModelAccess,
			Access:   "not-an-access",
			ModelTag: names.NewModelTag(mi.UUID).String(),
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

	ctlPath := params.EntityPath{User: "alice", Name: "controller-1"}
	s.AssertAddController(ctx, c, ctlPath, true)
	s.AssertUpdateCredential(ctx, c, "alice", "dummy", "cred1", "empty")
	mi := s.assertCreateModel(c, createModelParams{name: "test-model", username: "alice", cred: "cred1"})

	conn := s.open(c, nil, "alice")
	defer conn.Close()

	client := modelmanager.NewClient(conn)
	tag := names.NewModelTag(mi.UUID)
	err := client.DestroyModel(tag, nil, nil, nil)
	c.Assert(err, gc.Equals, nil)

	// Check the model is now dying.
	mis, err := client.ModelInfo([]names.ModelTag{tag})
	c.Assert(err, gc.Equals, nil)
	c.Assert(mis, gc.HasLen, 1)
	c.Assert(mis[0].Error, gc.Equals, (*jujuparams.Error)(nil))
	c.Assert(mis[0].Result.Life, gc.Equals, life.Dying)

	// Kill the model.
	err = s.JEM.DB.DeleteModelWithUUID(ctx, ctlPath, mi.UUID)
	c.Assert(err, gc.Equals, nil)

	// Make sure it's not an error if you destroy a model that't not there.
	err = client.DestroyModel(names.NewModelTag(mi.UUID), nil, nil, nil)
	c.Assert(err, gc.Equals, nil)
}

func (s *modelManagerSuite) TestDestroyModelWithStorageError(c *gc.C) {
	ctx := context.Background()

	ctlPath := params.EntityPath{User: "alice", Name: "controller-1"}
	s.AssertAddController(ctx, c, ctlPath, true)
	s.AssertUpdateCredential(ctx, c, "alice", "dummy", "cred1", "empty")
	mi := s.assertCreateModel(c, createModelParams{name: "test-model", username: "alice", cred: "cred1"})

	modelState, err := s.StatePool.Get(mi.UUID)
	c.Assert(err, gc.Equals, nil)
	defer modelState.Release()
	f := factory.NewFactory(modelState.State, s.StatePool)
	f.MakeUnit(c, &factory.UnitParams{
		Application: f.MakeApplication(c, &factory.ApplicationParams{
			Charm: f.MakeCharm(c, &factory.CharmParams{
				Name: "storage-block",
			}),
			Storage: map[string]state.StorageConstraints{
				"data": {Pool: "modelscoped"},
			},
		}),
	})

	conn := s.open(c, nil, "alice")
	defer conn.Close()

	tag := names.NewModelTag(mi.UUID)
	client := modelmanager.NewClient(conn)
	err = client.DestroyModel(tag, nil, nil, nil)
	c.Assert(errgo.Cause(err), jc.Satisfies, jujuparams.IsCodeHasPersistentStorage)

	// Check the model is not now dying.
	mis, err := client.ModelInfo([]names.ModelTag{tag})
	c.Assert(err, gc.Equals, nil)
	c.Assert(mis, gc.HasLen, 1)
	c.Assert(mis[0].Error, gc.Equals, (*jujuparams.Error)(nil))
	c.Assert(mis[0].Result.Life, gc.Equals, life.Alive)
}

func (s *modelManagerSuite) TestDestroyModelWithStorageDestroyStorageTrue(c *gc.C) {
	ctx := context.Background()

	ctlPath := params.EntityPath{User: "alice", Name: "controller-1"}
	s.AssertAddController(ctx, c, ctlPath, true)
	s.AssertUpdateCredential(ctx, c, "alice", "dummy", "cred1", "empty")
	mi := s.assertCreateModel(c, createModelParams{name: "test-model", username: "alice", cred: "cred1"})

	modelState, err := s.StatePool.Get(mi.UUID)
	c.Assert(err, gc.Equals, nil)
	defer modelState.Release()
	f := factory.NewFactory(modelState.State, s.StatePool)
	f.MakeUnit(c, &factory.UnitParams{
		Application: f.MakeApplication(c, &factory.ApplicationParams{
			Charm: f.MakeCharm(c, &factory.CharmParams{
				Name: "storage-block",
			}),
			Storage: map[string]state.StorageConstraints{
				"data": {Pool: "modelscoped"},
			},
		}),
	})

	conn := s.open(c, nil, "alice")
	defer conn.Close()

	tag := names.NewModelTag(mi.UUID)
	client := modelmanager.NewClient(conn)
	err = client.DestroyModel(tag, newBool(true), nil, nil)
	c.Assert(err, gc.Equals, nil)

	// Check the model is not now dying.
	mis, err := client.ModelInfo([]names.ModelTag{tag})
	c.Assert(err, gc.Equals, nil)
	c.Assert(mis, gc.HasLen, 1)
	c.Assert(mis[0].Error, gc.Equals, (*jujuparams.Error)(nil))
	c.Assert(mis[0].Result.Life, gc.Equals, life.Dying)
}

func (s *modelManagerSuite) TestDestroyModelWithStorageDestroyStorageFalse(c *gc.C) {
	ctx := context.Background()

	ctlPath := params.EntityPath{User: "alice", Name: "controller-1"}
	s.AssertAddController(ctx, c, ctlPath, true)
	s.AssertUpdateCredential(ctx, c, "alice", "dummy", "cred1", "empty")
	mi := s.assertCreateModel(c, createModelParams{name: "test-model", username: "alice", cred: "cred1"})

	modelState, err := s.StatePool.Get(mi.UUID)
	c.Assert(err, gc.Equals, nil)
	defer modelState.Release()
	f := factory.NewFactory(modelState.State, s.StatePool)
	f.MakeUnit(c, &factory.UnitParams{
		Application: f.MakeApplication(c, &factory.ApplicationParams{
			Charm: f.MakeCharm(c, &factory.CharmParams{
				Name: "storage-block",
			}),
			Storage: map[string]state.StorageConstraints{
				"data": {Pool: "modelscoped"},
			},
		}),
	})

	conn := s.open(c, nil, "alice")
	defer conn.Close()

	tag := names.NewModelTag(mi.UUID)
	client := modelmanager.NewClient(conn)
	err = client.DestroyModel(tag, newBool(false), nil, nil)
	c.Assert(err, gc.Equals, nil)

	// Check the model is not now dying.
	mis, err := client.ModelInfo([]names.ModelTag{tag})
	c.Assert(err, gc.Equals, nil)
	c.Assert(mis, gc.HasLen, 1)
	c.Assert(mis[0].Error, gc.Equals, (*jujuparams.Error)(nil))
	c.Assert(mis[0].Result.Life, gc.Equals, life.Dying)
}

func (s *modelManagerSuite) TestDestroyModelV3(c *gc.C) {
	ctx := context.Background()

	ctlPath := params.EntityPath{User: "alice", Name: "controller-1"}
	s.AssertAddController(ctx, c, ctlPath, true)
	s.AssertUpdateCredential(ctx, c, "alice", "dummy", "cred1", "empty")
	mi := s.assertCreateModel(c, createModelParams{name: "test-model", username: "alice", cred: "cred1"})

	conn := s.open(c, nil, "alice")
	defer conn.Close()

	tag := names.NewModelTag(mi.UUID)
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
	ctx := context.Background()

	ctlPath := params.EntityPath{User: "alice", Name: "controller-1"}
	s.AssertAddController(ctx, c, ctlPath, true)
	s.AssertUpdateCredential(ctx, c, "alice", "dummy", "cred1", "empty")
	mi := s.assertCreateModel(c, createModelParams{name: "test-model", username: "alice", cred: "cred1"})

	conn := s.open(c, nil, "alice")
	defer conn.Close()

	tag := names.NewModelTag(mi.UUID)
	client := modelmanager.NewClient(conn)
	res, err := client.DumpModel(tag, false)
	c.Check(err, gc.Equals, nil)
	c.Check(res, gc.Not(gc.HasLen), 0)
}

func (s *modelManagerSuite) TestDumpModelV2(c *gc.C) {
	ctx := context.Background()

	ctlPath := params.EntityPath{User: "alice", Name: "controller-1"}
	s.AssertAddController(ctx, c, ctlPath, true)
	s.AssertUpdateCredential(ctx, c, "alice", "dummy", "cred1", "empty")
	mi := s.assertCreateModel(c, createModelParams{name: "test-model", username: "alice", cred: "cred1"})

	conn := s.open(c, nil, "alice")
	defer conn.Close()
	var results jujuparams.MapResults
	err := conn.APICall("ModelManager", 2, "", "DumpModels", jujuparams.Entities{
		Entities: []jujuparams.Entity{{
			Tag: names.NewModelTag(mi.UUID).String(),
		}},
	}, &results)
	c.Assert(err, gc.Equals, nil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Check(results.Results[0].Error, gc.IsNil)
	c.Check(results.Results[0].Result, gc.Not(gc.HasLen), 0)
}

func (s *modelManagerSuite) TestDumpModelUnauthorized(c *gc.C) {
	ctx := context.Background()

	ctlPath := params.EntityPath{User: "alice", Name: "controller-1"}
	s.AssertAddController(ctx, c, ctlPath, true)
	s.AssertUpdateCredential(ctx, c, "alice", "dummy", "cred1", "empty")
	mi := s.assertCreateModel(c, createModelParams{name: "test-model", username: "alice", cred: "cred1"})

	conn := s.open(c, nil, "bob")
	defer conn.Close()

	tag := names.NewModelTag(mi.UUID)
	client := modelmanager.NewClient(conn)
	res, err := client.DumpModel(tag, true)
	c.Check(err, gc.ErrorMatches, `unauthorized`)
	c.Check(res, gc.IsNil)
}

func (s *modelManagerSuite) TestDumpModelDB(c *gc.C) {
	ctx := context.Background()

	ctlPath := params.EntityPath{User: "alice", Name: "controller-1"}
	s.AssertAddController(ctx, c, ctlPath, true)
	s.AssertUpdateCredential(ctx, c, "alice", "dummy", "cred1", "empty")
	mi := s.assertCreateModel(c, createModelParams{name: "test-model", username: "alice", cred: "cred1"})

	conn := s.open(c, nil, "alice")
	defer conn.Close()

	tag := names.NewModelTag(mi.UUID)
	client := modelmanager.NewClient(conn)
	res, err := client.DumpModelDB(tag)
	c.Check(err, gc.Equals, nil)
	c.Check(res, gc.Not(gc.HasLen), 0)
}

func (s *modelManagerSuite) TestDumpModelDBUnauthorized(c *gc.C) {
	ctx := context.Background()

	ctlPath := params.EntityPath{User: "alice", Name: "controller-1"}
	s.AssertAddController(ctx, c, ctlPath, true)
	s.AssertUpdateCredential(ctx, c, "alice", "dummy", "cred1", "empty")
	mi := s.assertCreateModel(c, createModelParams{name: "test-model", username: "alice", cred: "cred1"})

	conn := s.open(c, nil, "bob")
	defer conn.Close()

	tag := names.NewModelTag(mi.UUID)
	client := modelmanager.NewClient(conn)
	res, err := client.DumpModelDB(tag)
	c.Check(err, gc.ErrorMatches, `unauthorized`)
	c.Check(res, gc.IsNil)
}

func (s *modelManagerSuite) TestChangeModelCredential(c *gc.C) {
	ctx := context.Background()

	ctlPath := params.EntityPath{User: "alice", Name: "controller-1"}
	s.AssertAddController(ctx, c, ctlPath, true)
	s.AssertUpdateCredential(ctx, c, "alice", "dummy", "cred1", "empty")
	mi := s.assertCreateModel(c, createModelParams{name: "test-model", username: "alice", cred: "cred1"})
	s.AssertUpdateCredential(ctx, c, "alice", "dummy", "cred2", "empty")

	conn := s.open(c, nil, "alice")
	defer conn.Close()

	modelTag := names.NewModelTag(mi.UUID)
	credTag := names.NewCloudCredentialTag("dummy/alice@external/cred2")
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
	ctx := context.Background()

	ctlPath := params.EntityPath{User: "alice", Name: "controller-1"}
	s.AssertAddController(ctx, c, ctlPath, true)
	s.AssertUpdateCredential(ctx, c, "alice", "dummy", "cred1", "empty")
	mi := s.assertCreateModel(c, createModelParams{name: "test-model", username: "alice", cred: "cred1"})
	s.AssertUpdateCredential(ctx, c, "bob", "dummy", "cred2", "empty")

	conn := s.open(c, nil, "bob")
	defer conn.Close()

	modelTag := names.NewModelTag(mi.UUID)
	credTag := names.NewCloudCredentialTag("dummy/bob@external/cred2")
	client := modelmanager.NewClient(conn)
	err := client.ChangeModelCredential(modelTag, credTag)
	c.Assert(err, gc.ErrorMatches, `unauthorized`)
}

func (s *modelManagerSuite) TestChangeModelCredentialUnauthorizedCredential(c *gc.C) {
	ctx := context.Background()

	ctlPath := params.EntityPath{User: "alice", Name: "controller-1"}
	s.AssertAddController(ctx, c, ctlPath, true)
	s.AssertUpdateCredential(ctx, c, "alice", "dummy", "cred1", "empty")
	mi := s.assertCreateModel(c, createModelParams{name: "test-model", username: "alice", cred: "cred1"})
	s.AssertUpdateCredential(ctx, c, "bob", "dummy", "cred2", "empty")

	conn := s.open(c, nil, "alice")
	defer conn.Close()

	modelTag := names.NewModelTag(mi.UUID)
	credTag := names.NewCloudCredentialTag("dummy/bob@external/cred2")
	client := modelmanager.NewClient(conn)
	err := client.ChangeModelCredential(modelTag, credTag)
	c.Assert(err, gc.ErrorMatches, `unauthorized`)
}

func (s *modelManagerSuite) TestChangeModelCredentialNotFoundModel(c *gc.C) {
	ctx := context.Background()

	ctlPath := params.EntityPath{User: "alice", Name: "controller-1"}
	s.AssertAddController(ctx, c, ctlPath, true)
	s.AssertUpdateCredential(ctx, c, "alice", "dummy", "cred1", "empty")
	s.assertCreateModel(c, createModelParams{name: "test-model", username: "alice", cred: "cred1"})
	s.AssertUpdateCredential(ctx, c, "bob", "dummy", "cred2", "empty")

	conn := s.open(c, nil, "alice")
	defer conn.Close()

	modelTag := names.NewModelTag("000000000-0000-0000-0000-000000000000")
	credTag := names.NewCloudCredentialTag("dummy/bob@external/cred2")
	client := modelmanager.NewClient(conn)
	err := client.ChangeModelCredential(modelTag, credTag)
	c.Assert(err, gc.ErrorMatches, `model not found`)
}

func (s *modelManagerSuite) TestChangeModelCredentialNotFoundCredential(c *gc.C) {
	ctx := context.Background()

	ctlPath := params.EntityPath{User: "alice", Name: "controller-1"}
	s.AssertAddController(ctx, c, ctlPath, true)
	s.AssertUpdateCredential(ctx, c, "alice", "dummy", "cred1", "empty")
	mi := s.assertCreateModel(c, createModelParams{name: "test-model", username: "alice", cred: "cred1"})

	conn := s.open(c, nil, "alice")
	defer conn.Close()

	modelTag := names.NewModelTag(mi.UUID)
	credTag := names.NewCloudCredentialTag("dummy/alice@external/cred2")
	client := modelmanager.NewClient(conn)
	err := client.ChangeModelCredential(modelTag, credTag)
	c.Assert(err, gc.ErrorMatches, `credential not found`)
}

func (s *modelManagerSuite) TestChangeModelCredentialLocalUserCredential(c *gc.C) {
	ctx := context.Background()

	ctlPath := params.EntityPath{User: "alice", Name: "controller-1"}
	s.AssertAddController(ctx, c, ctlPath, true)
	s.AssertUpdateCredential(ctx, c, "alice", "dummy", "cred1", "empty")
	mi := s.assertCreateModel(c, createModelParams{name: "test-model", username: "alice", cred: "cred1"})

	conn := s.open(c, nil, "alice")
	defer conn.Close()

	modelTag := names.NewModelTag(mi.UUID)
	credTag := names.NewCloudCredentialTag("dummy/alice/cred2")
	client := modelmanager.NewClient(conn)
	err := client.ChangeModelCredential(modelTag, credTag)
	c.Assert(err, gc.ErrorMatches, `unsupported local user`)
}

type caasModelManagerSuite struct {
	websocketSuite

	creds names.CloudCredentialTag
}

var _ = gc.Suite(&caasModelManagerSuite{})

func (s *caasModelManagerSuite) SetUpTest(c *gc.C) {
	ctx := context.Background()

	s.ServerParams.CharmstoreLocation = "https://api.jujucharms.com/charmstore"
	s.ServerParams.MeteringLocation = "https://api.jujucharms.com/omnibus"
	s.websocketSuite.SetUpTest(c)
	s.PatchValue(&utils.OutgoingAccessAllowed, true)

	s.AssertAddController(ctx, c, params.EntityPath{User: "test", Name: "controller-1"}, true)
	s.creds = names.NewCloudCredentialTag("test-cloud/test@external/test-cred")
}

func (s *caasModelManagerSuite) TestCreateModelKubernetes(c *gc.C) {
	s.AssertAddKubernetesCloud(c, s.creds)
	conn := s.open(c, nil, "test")
	defer conn.Close()

	client := modelmanager.NewClient(conn)
	mi, err := client.CreateModel("test-model", "test@external", "test-cloud", "", s.creds, nil)
	c.Assert(err, gc.Equals, nil)

	c.Assert(mi.Name, gc.Equals, "test-model")
	c.Assert(mi.Type, gc.Equals, model.CAAS)
	c.Assert(mi.ProviderType, gc.Equals, "kubernetes")
	c.Assert(mi.Cloud, gc.Equals, "test-cloud")
	c.Assert(mi.CloudRegion, gc.Equals, "default")
	c.Assert(mi.Owner, gc.Equals, "test@external")
}

func (s *caasModelManagerSuite) TestListCAASModelSummaries(c *gc.C) {
	s.AssertAddKubernetesCloud(c, s.creds)
	conn := s.open(c, nil, "test")
	defer conn.Close()

	client := modelmanager.NewClient(conn)
	mi, err := client.CreateModel("model-1", "test@external", "test-cloud", "", s.creds, nil)
	c.Assert(err, gc.Equals, nil)

	models, err := client.ListModelSummaries("test", false)
	c.Assert(err, gc.Equals, nil)
	c.Assert(models, jemtest.CmpEquals(cmpopts.IgnoreTypes(&time.Time{})), []base.UserModelSummary{{
		Name:            "model-1",
		UUID:            mi.UUID,
		ControllerUUID:  "914487b5-60e7-42bb-bd63-1adc3fd3a388",
		ProviderType:    "kubernetes",
		DefaultSeries:   "focal",
		Cloud:           "test-cloud",
		CloudRegion:     "default",
		CloudCredential: "test-cloud/test@external/test-cred",
		Owner:           "test@external",
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
		}},
		AgentVersion: &jujuversion.Current,
		Type:         "caas",
	}})
}

func (s *caasModelManagerSuite) TestListCAASModels(c *gc.C) {
	s.AssertAddKubernetesCloud(c, s.creds)
	conn := s.open(c, nil, "test")
	defer conn.Close()

	client := modelmanager.NewClient(conn)
	mi, err := client.CreateModel("model-1", "test@external", "test-cloud", "", s.creds, nil)
	c.Assert(err, gc.Equals, nil)

	models, err := client.ListModels("test")
	c.Assert(err, gc.Equals, nil)
	c.Assert(models, jc.DeepEquals, []base.UserModel{{
		Name:  "model-1",
		UUID:  mi.UUID,
		Owner: "test@external",
		Type:  "caas",
	}})
}

func (s *modelManagerSuite) TestValidateModelUpgrades(c *gc.C) {
	ctx := context.Background()

	ctlPath := params.EntityPath{User: "alice", Name: "controller-1"}
	s.AssertAddController(ctx, c, ctlPath, true)
	s.AssertUpdateCredential(ctx, c, "alice", "dummy", "cred1", "empty")
	mi := s.assertCreateModel(c, createModelParams{name: "test-model", username: "alice", cred: "cred1"})
	s.AssertUpdateCredential(ctx, c, "bob", "dummy", "cred2", "empty")

	conn := s.open(c, nil, "alice")
	defer conn.Close()

	modelTag := names.NewModelTag(mi.UUID)
	client := modelmanager.NewClient(conn)
	err := client.ValidateModelUpgrade(modelTag, false)
	c.Assert(err, gc.Equals, nil)

	uuid := utils.MustNewUUID().String()
	err = client.ValidateModelUpgrade(names.NewModelTag(uuid), false)
	c.Assert(err, gc.ErrorMatches, "model not found")
}

func (s *caasModelManagerSuite) AssertAddKubernetesCloud(c *gc.C, credTag names.CloudCredentialTag) {
	ksrv := kubetest.NewFakeKubernetes(c)
	s.AddCleanup(func(c *gc.C) {
		ksrv.Close()
	})

	userTag := credTag.Owner()
	user := userTag.Id()
	if userTag.Domain() == "external" {
		user = userTag.Name()
	}

	conn := s.open(c, nil, user)
	defer conn.Close()

	cloudclient := cloudapi.NewClient(conn)
	err := cloudclient.AddCloud(cloud.Cloud{
		Name:            credTag.Cloud().Id(),
		Type:            "kubernetes",
		AuthTypes:       cloud.AuthTypes{cloud.UserPassAuthType},
		Endpoint:        ksrv.URL,
		HostCloudRegion: "dummy/dummy-region",
	}, false)
	c.Assert(err, gc.Equals, nil)

	cred := cloud.NewCredential(cloud.UserPassAuthType, map[string]string{
		"username": kubetest.Username,
		"password": kubetest.Password,
	})
	res, err := cloudclient.UpdateCredentialsCheckModels(credTag, cred)
	c.Assert(err, gc.Equals, nil)
	for _, model := range res {
		for _, err := range model.Errors {
			c.Assert(err, gc.Equals, nil)
		}
	}
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
	ctx := context.Background()

	ctlPath := params.EntityPath{User: "alice", Name: "controller-1"}
	s.AssertAddController(ctx, c, ctlPath, true)
	s.AssertUpdateCredential(ctx, c, "alice", "dummy", "cred1", "empty")

	conn := s.open(c, nil, "alice")
	defer conn.Close()
	client := modelmanager.NewClient(conn)

	err := client.SetModelDefaults("aws", "eu-central-1", map[string]interface{}{
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
