// Copyright 2020 Canonical Ltd.

package jimm_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	jujuparams "github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/status"
	"github.com/juju/names/v4"
	"gorm.io/gorm"

	"github.com/CanonicalLtd/jimm/internal/db"
	"github.com/CanonicalLtd/jimm/internal/dbmodel"
	"github.com/CanonicalLtd/jimm/internal/errors"
	"github.com/CanonicalLtd/jimm/internal/jimm"
	"github.com/CanonicalLtd/jimm/internal/jimmtest"
)

func TestModelCreateArgs(t *testing.T) {
	c := qt.New(t)

	tests := []struct {
		about         string
		args          jujuparams.ModelCreateArgs
		expectedArgs  jimm.ModelCreateArgs
		expectedError string
	}{{
		about: "all ok",
		args: jujuparams.ModelCreateArgs{
			Name:               "test-model",
			OwnerTag:           names.NewUserTag("alice@external").String(),
			CloudTag:           names.NewCloudTag("test-cloud").String(),
			CloudCredentialTag: names.NewCloudCredentialTag("test-cloud/alice@external/test-credential-1").String(),
		},
		expectedArgs: jimm.ModelCreateArgs{
			Name:            "test-model",
			Owner:           names.NewUserTag("alice@external"),
			Cloud:           names.NewCloudTag("test-cloud"),
			CloudCredential: names.NewCloudCredentialTag("test-cloud/alice@external/test-credential-1"),
		},
	}, {
		about: "name not specified",
		args: jujuparams.ModelCreateArgs{
			OwnerTag:           names.NewUserTag("alice@external").String(),
			CloudTag:           names.NewCloudTag("test-cloud").String(),
			CloudCredentialTag: names.NewCloudCredentialTag("test-cloud/alice/test-credential-1").String(),
		},
		expectedError: "name not specified",
	}, {
		about: "invalid owner tag",
		args: jujuparams.ModelCreateArgs{
			Name:               "test-model",
			OwnerTag:           "alice@external",
			CloudTag:           names.NewCloudTag("test-cloud").String(),
			CloudCredentialTag: names.NewCloudCredentialTag("test-cloud/alice/test-credential-1").String(),
		},
		expectedError: "invalid owner tag",
	}, {
		about: "invalid cloud tag",
		args: jujuparams.ModelCreateArgs{
			Name:               "test-model",
			OwnerTag:           names.NewUserTag("alice@external").String(),
			CloudTag:           "test-cloud",
			CloudCredentialTag: names.NewCloudCredentialTag("test-cloud/alice/test-credential-1").String(),
		},
		expectedError: "invalid cloud tag",
	}, {
		about: "invalid cloud credential tag",
		args: jujuparams.ModelCreateArgs{
			Name:               "test-model",
			OwnerTag:           names.NewUserTag("alice@external").String(),
			CloudTag:           names.NewCloudTag("test-cloud").String(),
			CloudCredentialTag: "test-credential-1",
		},
		expectedError: "invalid cloud credential tag",
	}, {
		about: "cloud does not match cloud credential cloud",
		args: jujuparams.ModelCreateArgs{
			Name:               "test-model",
			OwnerTag:           names.NewUserTag("alice@external").String(),
			CloudTag:           names.NewCloudTag("test-cloud").String(),
			CloudCredentialTag: names.NewCloudCredentialTag("another-cloud/alice/test-credential-1").String(),
		},
		expectedError: "cloud credential cloud mismatch",
	}, {
		about: "owner tag not specified",
		args: jujuparams.ModelCreateArgs{
			Name:               "test-model",
			CloudTag:           names.NewCloudTag("test-cloud").String(),
			CloudCredentialTag: names.NewCloudCredentialTag("test-cloud/alice/test-credential-1").String(),
		},
		expectedError: "owner tag not specified",
	}, {
		about: "cloud tag not specified",
		args: jujuparams.ModelCreateArgs{
			Name:               "test-model",
			OwnerTag:           names.NewUserTag("alice@external").String(),
			CloudCredentialTag: names.NewCloudCredentialTag("test-cloud/alice/test-credential-1").String(),
		},
		expectedError: "cloud tag not specified",
	}}

	opts := []cmp.Option{
		cmp.Comparer(func(t1, t2 names.UserTag) bool {
			return t1.String() == t2.String()
		}),
		cmp.Comparer(func(t1, t2 names.CloudTag) bool {
			return t1.String() == t2.String()
		}),
		cmp.Comparer(func(t1, t2 names.CloudCredentialTag) bool {
			return t1.String() == t2.String()
		}),
	}
	for _, test := range tests {
		c.Run(test.about, func(c *qt.C) {
			var a jimm.ModelCreateArgs
			err := a.FromJujuModelCreateArgs(&test.args)
			if test.expectedError == "" {
				c.Assert(err, qt.IsNil)
				c.Assert(a, qt.CmpEquals(opts...), test.expectedArgs)
			} else {
				c.Assert(err, qt.ErrorMatches, test.expectedError)
			}
		})
	}
}

func TestAddModel(t *testing.T) {
	c := qt.New(t)

	now := time.Now().UTC().Round(time.Millisecond)
	arch := "amd64"
	mem := uint64(8096)
	cores := uint64(8)

	tests := []struct {
		about               string
		updateCredential    func(context.Context, jujuparams.TaggedCredential) ([]jujuparams.UpdateCredentialModelResult, error)
		grantJIMMModelAdmin func(context.Context, names.ModelTag) error
		createModel         func(ctx context.Context, args *jujuparams.ModelCreateArgs, mi *jujuparams.ModelInfo) error
		createEnv           func(*qt.C, db.Database) (dbmodel.User, jujuparams.ModelCreateArgs, dbmodel.Model, string)
	}{{
		about: "creating a model by specifying the cloud region",
		updateCredential: func(_ context.Context, _ jujuparams.TaggedCredential) ([]jujuparams.UpdateCredentialModelResult, error) {
			return nil, nil
		},
		grantJIMMModelAdmin: func(_ context.Context, _ names.ModelTag) error {
			return nil
		},
		createModel: func(ctx context.Context, args *jujuparams.ModelCreateArgs, mi *jujuparams.ModelInfo) error {
			mi.Name = args.Name
			mi.UUID = "00000001-0000-0000-0000-0000-000000000001"
			mi.CloudTag = args.CloudTag
			mi.CloudCredentialTag = args.CloudCredentialTag
			mi.CloudRegion = args.CloudRegion
			mi.OwnerTag = args.OwnerTag
			mi.Status = jujuparams.EntityStatus{
				Status: status.Started,
				Info:   "running a test",
			}
			mi.Life = life.Alive
			mi.Users = []jujuparams.ModelUserInfo{{
				UserName: "alice@external",
				Access:   jujuparams.ModelAdminAccess,
			}, {
				// "bob" is a local user
				UserName: "bob",
				Access:   jujuparams.ModelReadAccess,
			}}
			mi.Machines = []jujuparams.ModelMachineInfo{{
				Id: "test-machine-id",
				Hardware: &jujuparams.MachineHardware{
					Arch:  &arch,
					Mem:   &mem,
					Cores: &cores,
				},
				DisplayName: "a test machine",
				Status:      "running",
				Message:     "a test message",
				HasVote:     true,
				WantsVote:   false,
			}}
			return nil
		},
		createEnv: func(c *qt.C, db db.Database) (dbmodel.User, jujuparams.ModelCreateArgs, dbmodel.Model, string) {
			controller1 := dbmodel.Controller{
				Name: "test-controller-1",
				UUID: "00000000-0000-0000-0000-0000-0000000000001",
			}
			err := db.AddController(context.Background(), &controller1)
			c.Assert(err, qt.Equals, nil)

			controller2 := dbmodel.Controller{
				Name: "test-controller-2",
				UUID: "00000000-0000-0000-0000-0000-0000000000002",
			}
			err = db.AddController(context.Background(), &controller2)
			c.Assert(err, qt.Equals, nil)

			u := dbmodel.User{
				Username:         "alice@external",
				ControllerAccess: "superuser",
			}
			c.Assert(db.DB.Create(&u).Error, qt.IsNil)

			cloud := dbmodel.Cloud{
				Name: "test-cloud",
				Type: "test-provider",
				Regions: []dbmodel.CloudRegion{{
					Name: "test-region-1",
					Controllers: []dbmodel.CloudRegionControllerPriority{{
						Priority:     0,
						ControllerID: controller1.ID,
					}, {
						// controller2 has a higher priority and the model
						// should be created on this controller
						Priority:     2,
						ControllerID: controller2.ID,
					}},
				}},
				Users: []dbmodel.UserCloudAccess{{
					Username: u.Username,
				}},
			}
			c.Assert(db.DB.Create(&cloud).Error, qt.IsNil)

			cred := dbmodel.CloudCredential{
				Name:      "test-credential-1",
				CloudName: cloud.Name,
				OwnerID:   u.Username,
				AuthType:  "empty",
			}
			err = db.SetCloudCredential(context.Background(), &cred)
			c.Assert(err, qt.Equals, nil)

			args := jujuparams.ModelCreateArgs{
				Name:               "test-model",
				OwnerTag:           names.NewUserTag(u.Username).String(),
				CloudTag:           names.NewCloudTag(cloud.Name).String(),
				CloudRegion:        "test-region-1",
				CloudCredentialTag: names.NewCloudCredentialTag("test-cloud/alice@external/test-credential-1").String(),
			}

			expectedModel := dbmodel.Model{
				ID:        1,
				CreatedAt: now,
				UpdatedAt: now,
				Name:      "test-model",
				UUID: sql.NullString{
					String: "00000001-0000-0000-0000-0000-000000000001",
					Valid:  true,
				},
				OwnerID:           "alice@external",
				ControllerID:      controller2.ID,
				CloudRegionID:     1,
				CloudCredentialID: cred.ID,
				Life:              "alive",
				Status: dbmodel.Status{
					Status: "started",
					Info:   "running a test",
				},
				Users: []dbmodel.UserModelAccess{{
					Model: gorm.Model{
						ID:        1,
						CreatedAt: now,
						UpdatedAt: now,
					},
					UserID:  1,
					ModelID: 1,
					Access:  "admin",
				}},
			}
			return u, args, expectedModel, ""
		},
	}, {
		about: "creating a model by specifying the cloud",
		updateCredential: func(_ context.Context, _ jujuparams.TaggedCredential) ([]jujuparams.UpdateCredentialModelResult, error) {
			return nil, nil
		},
		grantJIMMModelAdmin: func(_ context.Context, _ names.ModelTag) error {
			return nil
		},
		createModel: func(ctx context.Context, args *jujuparams.ModelCreateArgs, mi *jujuparams.ModelInfo) error {
			mi.Name = args.Name
			mi.UUID = "00000001-0000-0000-0000-0000-000000000001"
			mi.CloudTag = args.CloudTag
			mi.CloudCredentialTag = args.CloudCredentialTag
			mi.CloudRegion = args.CloudRegion
			mi.OwnerTag = args.OwnerTag
			mi.Status = jujuparams.EntityStatus{
				Status: status.Started,
				Info:   "running a test",
			}
			mi.Life = life.Alive
			mi.Users = []jujuparams.ModelUserInfo{{
				UserName: "alice@external",
				Access:   jujuparams.ModelAdminAccess,
			}}
			mi.Machines = []jujuparams.ModelMachineInfo{{
				Id: "test-machine-id",
				Hardware: &jujuparams.MachineHardware{
					Arch:  &arch,
					Mem:   &mem,
					Cores: &cores,
				},
				DisplayName: "a test machine",
				Status:      "running",
				Message:     "a test message",
				HasVote:     true,
				WantsVote:   false,
			}}
			return nil
		},
		createEnv: func(c *qt.C, db db.Database) (dbmodel.User, jujuparams.ModelCreateArgs, dbmodel.Model, string) {
			controller1 := dbmodel.Controller{
				Name: "test-controller-1",
				UUID: "00000000-0000-0000-0000-0000-0000000000001",
			}
			err := db.AddController(context.Background(), &controller1)
			c.Assert(err, qt.Equals, nil)

			controller2 := dbmodel.Controller{
				Name: "test-controller-2",
				UUID: "00000000-0000-0000-0000-0000-0000000000002",
			}
			err = db.AddController(context.Background(), &controller2)
			c.Assert(err, qt.Equals, nil)

			u := dbmodel.User{
				Username:         "alice@external",
				ControllerAccess: "superuser",
			}
			c.Assert(db.DB.Create(&u).Error, qt.IsNil)

			cloud := dbmodel.Cloud{
				Name: "test-cloud",
				Type: "test-provider",
				Regions: []dbmodel.CloudRegion{{
					Name: "test-region-1",
					Controllers: []dbmodel.CloudRegionControllerPriority{{
						Priority:     0,
						ControllerID: controller1.ID,
					}, {
						// controller2 has a higher priority and the model
						// should be created on this controller
						Priority:     2,
						ControllerID: controller2.ID,
					}},
				}},
				Users: []dbmodel.UserCloudAccess{{
					Username: u.Username,
				}},
			}
			c.Assert(db.DB.Create(&cloud).Error, qt.IsNil)

			cred := dbmodel.CloudCredential{
				Name:      "test-credential-1",
				CloudName: cloud.Name,
				OwnerID:   u.Username,
				AuthType:  "empty",
			}
			err = db.SetCloudCredential(context.Background(), &cred)
			c.Assert(err, qt.Equals, nil)

			args := jujuparams.ModelCreateArgs{
				Name:               "test-model",
				OwnerTag:           names.NewUserTag(u.Username).String(),
				CloudTag:           names.NewCloudTag(cloud.Name).String(),
				CloudCredentialTag: names.NewCloudCredentialTag("test-cloud/alice@external/test-credential-1").String(),
			}

			expectedModel := dbmodel.Model{
				ID:        1,
				CreatedAt: now,
				UpdatedAt: now,
				Name:      "test-model",
				UUID: sql.NullString{
					String: "00000001-0000-0000-0000-0000-000000000001",
					Valid:  true,
				},
				OwnerID:           "alice@external",
				ControllerID:      controller2.ID,
				CloudRegionID:     1,
				CloudCredentialID: cred.ID,
				Life:              "alive",
				Status: dbmodel.Status{
					Status: "started",
					Info:   "running a test",
				},
				Users: []dbmodel.UserModelAccess{{
					Model: gorm.Model{
						ID:        1,
						CreatedAt: now,
						UpdatedAt: now,
					},
					UserID:  1,
					ModelID: 1,
					Access:  "admin",
				}},
			}
			return u, args, expectedModel, ""
		},
	}, {
		about: "creating a model in another namespace - allowed since alice is superuser",
		updateCredential: func(_ context.Context, _ jujuparams.TaggedCredential) ([]jujuparams.UpdateCredentialModelResult, error) {
			return nil, nil
		},
		grantJIMMModelAdmin: func(_ context.Context, _ names.ModelTag) error {
			return nil
		},
		createModel: func(ctx context.Context, args *jujuparams.ModelCreateArgs, mi *jujuparams.ModelInfo) error {
			mi.Name = args.Name
			mi.UUID = "00000001-0000-0000-0000-0000-000000000001"
			mi.CloudTag = args.CloudTag
			mi.CloudCredentialTag = args.CloudCredentialTag
			mi.CloudRegion = args.CloudRegion
			mi.OwnerTag = args.OwnerTag
			mi.Status = jujuparams.EntityStatus{
				Status: status.Started,
				Info:   "running a test",
			}
			mi.Life = life.Alive
			mi.Users = []jujuparams.ModelUserInfo{{
				UserName: "alice@external",
				Access:   jujuparams.ModelAdminAccess,
			}}
			mi.Machines = []jujuparams.ModelMachineInfo{{
				Id: "test-machine-id",
				Hardware: &jujuparams.MachineHardware{
					Arch:  &arch,
					Mem:   &mem,
					Cores: &cores,
				},
				DisplayName: "a test machine",
				Status:      "running",
				Message:     "a test message",
				HasVote:     true,
				WantsVote:   false,
			}}
			return nil
		},
		createEnv: func(c *qt.C, db db.Database) (dbmodel.User, jujuparams.ModelCreateArgs, dbmodel.Model, string) {
			controller1 := dbmodel.Controller{
				Name: "test-controller-1",
				UUID: "00000000-0000-0000-0000-0000-0000000000001",
			}
			err := db.AddController(context.Background(), &controller1)
			c.Assert(err, qt.Equals, nil)

			u1 := dbmodel.User{
				Username:         "alice@external",
				ControllerAccess: "superuser",
			}
			c.Assert(db.DB.Create(&u1).Error, qt.IsNil)

			u2 := dbmodel.User{
				Username:         "bob",
				ControllerAccess: "add-model",
			}
			c.Assert(db.DB.Create(&u2).Error, qt.IsNil)

			cloud := dbmodel.Cloud{
				Name: "test-cloud",
				Type: "test-provider",
				Regions: []dbmodel.CloudRegion{{
					Name: "test-region-1",
					Controllers: []dbmodel.CloudRegionControllerPriority{{
						Priority:     0,
						ControllerID: controller1.ID,
					}},
				}},
				Users: []dbmodel.UserCloudAccess{{
					Username: u1.Username,
				}, {
					Username: u2.Username,
				}},
			}
			c.Assert(db.DB.Create(&cloud).Error, qt.IsNil)

			cred := dbmodel.CloudCredential{
				Name:      "test-credential-1",
				CloudName: cloud.Name,
				OwnerID:   u1.Username,
				AuthType:  "empty",
			}
			err = db.SetCloudCredential(context.Background(), &cred)
			c.Assert(err, qt.Equals, nil)

			args := jujuparams.ModelCreateArgs{
				Name:               "test-model",
				OwnerTag:           names.NewUserTag(u2.Username).String(),
				CloudTag:           names.NewCloudTag(cloud.Name).String(),
				CloudCredentialTag: names.NewCloudCredentialTag("test-cloud/alice@external/test-credential-1").String(),
			}

			expectedModel := dbmodel.Model{
				ID:        1,
				CreatedAt: now,
				UpdatedAt: now,
				Name:      "test-model",
				UUID: sql.NullString{
					String: "00000001-0000-0000-0000-0000-000000000001",
					Valid:  true,
				},
				OwnerID:           "bob",
				ControllerID:      controller1.ID,
				CloudRegionID:     1,
				CloudCredentialID: cred.ID,
				Life:              "alive",
				Status: dbmodel.Status{
					Status: "started",
					Info:   "running a test",
				},
				Users: []dbmodel.UserModelAccess{{
					Model: gorm.Model{
						ID:        1,
						CreatedAt: now,
						UpdatedAt: now,
					},
					UserID:  1,
					ModelID: 1,
					Access:  "admin",
				}},
			}
			return u1, args, expectedModel, ""
		},
	}, {
		about: "creating a model in another namespace - not allowed since alice is not superuser",
		updateCredential: func(_ context.Context, _ jujuparams.TaggedCredential) ([]jujuparams.UpdateCredentialModelResult, error) {
			return nil, nil
		},
		grantJIMMModelAdmin: func(_ context.Context, _ names.ModelTag) error {
			return nil
		},
		createModel: func(ctx context.Context, args *jujuparams.ModelCreateArgs, mi *jujuparams.ModelInfo) error {
			mi.Name = args.Name
			mi.UUID = "00000001-0000-0000-0000-0000-000000000001"
			mi.CloudTag = args.CloudTag
			mi.CloudCredentialTag = args.CloudCredentialTag
			mi.CloudRegion = args.CloudRegion
			mi.OwnerTag = args.OwnerTag
			mi.Status = jujuparams.EntityStatus{
				Status: status.Started,
				Info:   "running a test",
			}
			mi.Life = life.Alive
			mi.Users = []jujuparams.ModelUserInfo{{
				UserName: "alice@external",
				Access:   jujuparams.ModelAdminAccess,
			}}
			mi.Machines = []jujuparams.ModelMachineInfo{{
				Id: "test-machine-id",
				Hardware: &jujuparams.MachineHardware{
					Arch:  &arch,
					Mem:   &mem,
					Cores: &cores,
				},
				DisplayName: "a test machine",
				Status:      "running",
				Message:     "a test message",
				HasVote:     true,
				WantsVote:   false,
			}}
			return nil
		},
		createEnv: func(c *qt.C, db db.Database) (dbmodel.User, jujuparams.ModelCreateArgs, dbmodel.Model, string) {
			controller1 := dbmodel.Controller{
				Name: "test-controller-1",
				UUID: "00000000-0000-0000-0000-0000-0000000000001",
			}
			err := db.AddController(context.Background(), &controller1)
			c.Assert(err, qt.Equals, nil)

			u1 := dbmodel.User{
				Username:         "alice",
				ControllerAccess: "add-model",
			}
			c.Assert(db.DB.Create(&u1).Error, qt.IsNil)

			u2 := dbmodel.User{
				Username:         "bob",
				ControllerAccess: "add-model",
			}
			c.Assert(db.DB.Create(&u2).Error, qt.IsNil)

			cloud := dbmodel.Cloud{
				Name: "test-cloud",
				Type: "test-provider",
				Regions: []dbmodel.CloudRegion{{
					Name: "test-region-1",
					Controllers: []dbmodel.CloudRegionControllerPriority{{
						Priority:     0,
						ControllerID: controller1.ID,
					}},
				}},
				Users: []dbmodel.UserCloudAccess{{
					Username: u1.Username,
				}, {
					Username: u2.Username,
				}},
			}
			c.Assert(db.DB.Create(&cloud).Error, qt.IsNil)

			cred := dbmodel.CloudCredential{
				Name:      "test-credential-1",
				CloudName: cloud.Name,
				OwnerID:   u1.Username,
				AuthType:  "empty",
			}
			err = db.SetCloudCredential(context.Background(), &cred)
			c.Assert(err, qt.Equals, nil)

			args := jujuparams.ModelCreateArgs{
				Name:               "test-model",
				OwnerTag:           names.NewUserTag(u2.Username).String(),
				CloudTag:           names.NewCloudTag(cloud.Name).String(),
				CloudCredentialTag: names.NewCloudCredentialTag("test-cloud/alice/test-credential-1").String(),
			}

			expectedModel := dbmodel.Model{
				ID:        1,
				CreatedAt: now,
				UpdatedAt: now,
				Name:      "test-model",
				UUID: sql.NullString{
					String: "00000001-0000-0000-0000-0000-000000000001",
					Valid:  true,
				},
				OwnerID:           "bob",
				ControllerID:      controller1.ID,
				CloudRegionID:     1,
				CloudCredentialID: cred.ID,
				Life:              "alive",
				Status: dbmodel.Status{
					Status: "started",
					Info:   "running a test",
				},
				Users: []dbmodel.UserModelAccess{{
					Model: gorm.Model{
						ID:        1,
						CreatedAt: now,
						UpdatedAt: now,
					},
					UserID:  1,
					ModelID: 1,
					Access:  "admin",
				}},
			}
			return u1, args, expectedModel, "unauthorized access"
		},
	}, {
		about: "CreateModel returns an error",
		updateCredential: func(_ context.Context, _ jujuparams.TaggedCredential) ([]jujuparams.UpdateCredentialModelResult, error) {
			return nil, nil
		},
		grantJIMMModelAdmin: func(_ context.Context, _ names.ModelTag) error {
			return nil
		},
		createModel: func(ctx context.Context, args *jujuparams.ModelCreateArgs, mi *jujuparams.ModelInfo) error {
			return errors.E("a test error")
		},
		createEnv: func(c *qt.C, db db.Database) (dbmodel.User, jujuparams.ModelCreateArgs, dbmodel.Model, string) {
			controller1 := dbmodel.Controller{
				Name: "test-controller-1",
				UUID: "00000000-0000-0000-0000-0000-0000000000001",
			}
			err := db.AddController(context.Background(), &controller1)
			c.Assert(err, qt.Equals, nil)

			controller2 := dbmodel.Controller{
				Name: "test-controller-2",
				UUID: "00000000-0000-0000-0000-0000-0000000000002",
			}
			err = db.AddController(context.Background(), &controller2)
			c.Assert(err, qt.Equals, nil)

			u := dbmodel.User{
				Username:         "alice",
				ControllerAccess: "superuser",
			}
			c.Assert(db.DB.Create(&u).Error, qt.IsNil)

			cloud := dbmodel.Cloud{
				Name: "test-cloud",
				Type: "test-provider",
				Regions: []dbmodel.CloudRegion{{
					Name: "test-region-1",
					Controllers: []dbmodel.CloudRegionControllerPriority{{
						Priority:     0,
						ControllerID: controller1.ID,
					}, {
						// controller2 has a higher priority and the model
						// should be created on this controller
						Priority:     2,
						ControllerID: controller2.ID,
					}},
				}},
				Users: []dbmodel.UserCloudAccess{{
					Username: u.Username,
				}},
			}
			c.Assert(db.DB.Create(&cloud).Error, qt.IsNil)

			cred := dbmodel.CloudCredential{
				Name:      "test-credential-1",
				CloudName: cloud.Name,
				OwnerID:   u.Username,
				AuthType:  "empty",
			}
			err = db.SetCloudCredential(context.Background(), &cred)
			c.Assert(err, qt.Equals, nil)

			args := jujuparams.ModelCreateArgs{
				Name:               "test-model",
				OwnerTag:           names.NewUserTag(u.Username).String(),
				CloudTag:           names.NewCloudTag(cloud.Name).String(),
				CloudRegion:        "test-region-1",
				CloudCredentialTag: names.NewCloudCredentialTag("test-cloud/alice/test-credential-1").String(),
			}

			return u, args, dbmodel.Model{}, "a test error"
		},
	}, {
		about: "model with the same name already exists",
		updateCredential: func(_ context.Context, _ jujuparams.TaggedCredential) ([]jujuparams.UpdateCredentialModelResult, error) {
			return nil, nil
		},
		grantJIMMModelAdmin: func(_ context.Context, _ names.ModelTag) error {
			return nil
		},
		createModel: func(ctx context.Context, args *jujuparams.ModelCreateArgs, mi *jujuparams.ModelInfo) error {
			mi.Name = args.Name
			mi.UUID = "00000001-0000-0000-0000-0000-000000000001"
			mi.CloudTag = args.CloudTag
			mi.CloudCredentialTag = args.CloudCredentialTag
			mi.CloudRegion = args.CloudRegion
			mi.OwnerTag = args.OwnerTag
			mi.Status = jujuparams.EntityStatus{
				Status: status.Started,
				Info:   "running a test",
			}
			mi.Life = life.Alive
			mi.Users = []jujuparams.ModelUserInfo{{
				UserName: "alice@external",
				Access:   jujuparams.ModelAdminAccess,
			}}
			mi.Machines = []jujuparams.ModelMachineInfo{{
				Id: "test-machine-id",
				Hardware: &jujuparams.MachineHardware{
					Arch:  &arch,
					Mem:   &mem,
					Cores: &cores,
				},
				DisplayName: "a test machine",
				Status:      "running",
				Message:     "a test message",
				HasVote:     true,
				WantsVote:   false,
			}}
			return nil
		},
		createEnv: func(c *qt.C, db db.Database) (dbmodel.User, jujuparams.ModelCreateArgs, dbmodel.Model, string) {
			controller1 := dbmodel.Controller{
				Name: "test-controller-1",
				UUID: "00000000-0000-0000-0000-0000-0000000000001",
			}
			err := db.AddController(context.Background(), &controller1)
			c.Assert(err, qt.Equals, nil)

			controller2 := dbmodel.Controller{
				Name: "test-controller-2",
				UUID: "00000000-0000-0000-0000-0000-0000000000002",
			}
			err = db.AddController(context.Background(), &controller2)
			c.Assert(err, qt.Equals, nil)

			u := dbmodel.User{
				Username:         "alice",
				ControllerAccess: "superuser",
			}
			c.Assert(db.DB.Create(&u).Error, qt.IsNil)

			cloud := dbmodel.Cloud{
				Name: "test-cloud",
				Type: "test-provider",
				Regions: []dbmodel.CloudRegion{{
					Name: "test-region-1",
					Controllers: []dbmodel.CloudRegionControllerPriority{{
						Priority:     0,
						ControllerID: controller1.ID,
					}, {
						// controller2 has a higher priority and the model
						// should be created on this controller
						Priority:     2,
						ControllerID: controller2.ID,
					}},
				}},
				Users: []dbmodel.UserCloudAccess{{
					Username: u.Username,
				}},
			}
			c.Assert(db.DB.Create(&cloud).Error, qt.IsNil)

			cred := dbmodel.CloudCredential{
				Name:      "test-credential-1",
				CloudName: cloud.Name,
				OwnerID:   u.Username,
				AuthType:  "empty",
			}
			err = db.SetCloudCredential(context.Background(), &cred)
			c.Assert(err, qt.Equals, nil)

			model := dbmodel.Model{
				Name:              "test-model",
				OwnerID:           u.Username,
				CloudRegionID:     cloud.Regions[0].ID,
				CloudCredentialID: cred.ID,
				ControllerID:      controller2.ID,
			}
			err = db.AddModel(context.Background(), &model)
			c.Assert(err, qt.Equals, nil)

			args := jujuparams.ModelCreateArgs{
				Name:               "test-model",
				OwnerTag:           names.NewUserTag(u.Username).String(),
				CloudTag:           names.NewCloudTag(cloud.Name).String(),
				CloudCredentialTag: names.NewCloudCredentialTag("test-cloud/alice/test-credential-1").String(),
			}

			expectedModel := dbmodel.Model{
				ID:        1,
				CreatedAt: now,
				UpdatedAt: now,
				Name:      "test-model",
				UUID: sql.NullString{
					String: "00000001-0000-0000-0000-0000-000000000001",
					Valid:  true,
				},
				OwnerID:           "alice",
				ControllerID:      controller2.ID,
				CloudRegionID:     1,
				CloudCredentialID: cred.ID,
				Life:              "alive",
				Status: dbmodel.Status{
					Status: "started",
					Info:   "running a test",
				},
				Users: []dbmodel.UserModelAccess{{
					Model: gorm.Model{
						ID:        1,
						CreatedAt: now,
						UpdatedAt: now,
					},
					UserID:  1,
					ModelID: 1,
					Access:  "admin",
				}},
			}
			return u, args, expectedModel, "model alice/test-model already exists"
		},
	}, {
		about: "UpdateCredential returns an error",
		updateCredential: func(_ context.Context, _ jujuparams.TaggedCredential) ([]jujuparams.UpdateCredentialModelResult, error) {
			return nil, errors.E("a silly error")
		},
		grantJIMMModelAdmin: func(_ context.Context, _ names.ModelTag) error {
			return nil
		},
		createModel: func(ctx context.Context, args *jujuparams.ModelCreateArgs, mi *jujuparams.ModelInfo) error {
			mi.Name = args.Name
			mi.UUID = "00000001-0000-0000-0000-0000-000000000001"
			mi.CloudTag = args.CloudTag
			mi.CloudCredentialTag = args.CloudCredentialTag
			mi.CloudRegion = args.CloudRegion
			mi.OwnerTag = args.OwnerTag
			mi.Status = jujuparams.EntityStatus{
				Status: status.Started,
				Info:   "running a test",
			}
			mi.Life = life.Alive
			mi.Users = []jujuparams.ModelUserInfo{{
				UserName: "alice@external",
				Access:   jujuparams.ModelAdminAccess,
			}}
			mi.Machines = []jujuparams.ModelMachineInfo{{
				Id: "test-machine-id",
				Hardware: &jujuparams.MachineHardware{
					Arch:  &arch,
					Mem:   &mem,
					Cores: &cores,
				},
				DisplayName: "a test machine",
				Status:      "running",
				Message:     "a test message",
				HasVote:     true,
				WantsVote:   false,
			}}
			return nil
		},
		createEnv: func(c *qt.C, db db.Database) (dbmodel.User, jujuparams.ModelCreateArgs, dbmodel.Model, string) {
			controller1 := dbmodel.Controller{
				Name: "test-controller-1",
				UUID: "00000000-0000-0000-0000-0000-0000000000001",
			}
			err := db.AddController(context.Background(), &controller1)
			c.Assert(err, qt.Equals, nil)

			controller2 := dbmodel.Controller{
				Name: "test-controller-2",
				UUID: "00000000-0000-0000-0000-0000-0000000000002",
			}
			err = db.AddController(context.Background(), &controller2)
			c.Assert(err, qt.Equals, nil)

			u := dbmodel.User{
				Username:         "alice",
				ControllerAccess: "superuser",
			}
			c.Assert(db.DB.Create(&u).Error, qt.IsNil)

			cloud := dbmodel.Cloud{
				Name: "test-cloud",
				Type: "test-provider",
				Regions: []dbmodel.CloudRegion{{
					Name: "test-region-1",
					Controllers: []dbmodel.CloudRegionControllerPriority{{
						Priority:     0,
						ControllerID: controller1.ID,
					}, {
						// controller2 has a higher priority and the model
						// should be created on this controller
						Priority:     2,
						ControllerID: controller2.ID,
					}},
				}},
				Users: []dbmodel.UserCloudAccess{{
					Username: u.Username,
				}},
			}
			c.Assert(db.DB.Create(&cloud).Error, qt.IsNil)

			cred := dbmodel.CloudCredential{
				Name:      "test-credential-1",
				CloudName: cloud.Name,
				OwnerID:   u.Username,
				AuthType:  "empty",
			}
			err = db.SetCloudCredential(context.Background(), &cred)
			c.Assert(err, qt.Equals, nil)

			args := jujuparams.ModelCreateArgs{
				Name:               "test-model",
				OwnerTag:           names.NewUserTag(u.Username).String(),
				CloudTag:           names.NewCloudTag(cloud.Name).String(),
				CloudCredentialTag: names.NewCloudCredentialTag("test-cloud/alice/test-credential-1").String(),
			}

			expectedModel := dbmodel.Model{
				ID:        1,
				CreatedAt: now,
				UpdatedAt: now,
				Name:      "test-model",
				UUID: sql.NullString{
					String: "00000001-0000-0000-0000-0000-000000000001",
					Valid:  true,
				},
				OwnerID:           "alice",
				ControllerID:      controller2.ID,
				CloudRegionID:     1,
				CloudCredentialID: cred.ID,
				Life:              "alive",
				Status: dbmodel.Status{
					Status: "started",
					Info:   "running a test",
				},
				Users: []dbmodel.UserModelAccess{{
					Model: gorm.Model{
						ID:        1,
						CreatedAt: now,
						UpdatedAt: now,
					},
					UserID:  1,
					ModelID: 1,
					Access:  "admin",
				}},
			}
			return u, args, expectedModel, "failed to update cloud credential"
		},
	}}

	for _, test := range tests {
		c.Run(test.about, func(c *qt.C) {
			api := &jimmtest.API{
				UpdateCredential_:    test.updateCredential,
				GrantJIMMModelAdmin_: test.grantJIMMModelAdmin,
				CreateModel_:         test.createModel,
			}

			j := &jimm.JIMM{
				Database: db.Database{
					DB: jimmtest.MemoryDB(c, func() time.Time { return now }),
				},
				Dialer: &jimmtest.Dialer{
					API: api,
				},
			}

			ctx := context.Background()
			err := j.Database.Migrate(ctx, false)
			c.Assert(err, qt.IsNil)

			u, jujuArgs, model, expectedError := test.createEnv(c, j.Database)

			args := jimm.ModelCreateArgs{}
			err = args.FromJujuModelCreateArgs(&jujuArgs)
			c.Assert(err, qt.IsNil)

			_, err = j.AddModel(context.Background(), &u, &args)
			if expectedError == "" {
				c.Assert(err, qt.IsNil)

				m1 := dbmodel.Model{
					UUID: model.UUID,
				}
				err = j.Database.GetModel(ctx, &m1)
				c.Assert(err, qt.IsNil)
				c.Assert(m1, qt.CmpEquals(cmpopts.EquateEmpty()), model)
			} else {
				c.Assert(err, qt.ErrorMatches, expectedError)
			}
		})
	}
}
