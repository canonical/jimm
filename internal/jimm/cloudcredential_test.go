// Copyright 2020 Canonical Ltd.

package jimm_test

import (
	"context"
	"errors"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
	jujuparams "github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/status"
	"github.com/juju/names/v4"

	"github.com/CanonicalLtd/jimm/internal/db"
	"github.com/CanonicalLtd/jimm/internal/dbmodel"
	"github.com/CanonicalLtd/jimm/internal/jimm"
	"github.com/CanonicalLtd/jimm/internal/jimmtest"
)

func TestUpdateCloudCredential(t *testing.T) {
	c := qt.New(t)

	now := time.Now().UTC().Round(time.Millisecond)
	arch := "amd64"
	mem := uint64(8096)
	cores := uint64(8)

	tests := []struct {
		about                  string
		checkCredentialErrors  []error
		updateCredentialErrors []error
		createEnv              func(*qt.C, *jimm.JIMM) (jimm.UpdateCloudCredentialArgs, dbmodel.CloudCredential, string)
	}{{
		about: "all ok",
		createEnv: func(c *qt.C, j *jimm.JIMM) (jimm.UpdateCloudCredentialArgs, dbmodel.CloudCredential, string) {
			controller1 := dbmodel.Controller{
				Name: "test-controller-1",
				UUID: "00000000-0000-0000-0000-0000-0000000000001",
			}
			err := j.Database.AddController(context.Background(), &controller1)
			c.Assert(err, qt.Equals, nil)

			controller2 := dbmodel.Controller{
				Name: "test-controller-2",
				UUID: "00000000-0000-0000-0000-0000-0000000000002",
			}
			err = j.Database.AddController(context.Background(), &controller2)
			c.Assert(err, qt.Equals, nil)

			u := dbmodel.User{
				Username:         "alice@external",
				ControllerAccess: "superuser",
			}
			c.Assert(j.Database.DB.Create(&u).Error, qt.IsNil)

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
			c.Assert(j.Database.DB.Create(&cloud).Error, qt.IsNil)

			cred := dbmodel.CloudCredential{
				Name:      "test-credential-1",
				CloudName: cloud.Name,
				OwnerID:   u.Username,
				AuthType:  "empty",
			}
			err = j.Database.SetCloudCredential(context.Background(), &cred)
			c.Assert(err, qt.Equals, nil)

			_, err = j.AddModel(context.Background(), &u, &jimm.ModelCreateArgs{
				Name:            "test-model",
				Owner:           names.NewUserTag(u.Username),
				Cloud:           names.NewCloudTag(cloud.Name),
				CloudRegion:     "test-region-1",
				CloudCredential: names.NewCloudCredentialTag("test-cloud/alice@external/test-credential-1"),
			})
			c.Assert(err, qt.Equals, nil)

			arg := jimm.UpdateCloudCredentialArgs{
				User:          &u,
				CredentialTag: names.NewCloudCredentialTag("test-cloud/alice@external/test-credential-1"),
				Credential: jujuparams.CloudCredential{
					Attributes: map[string]string{
						"key1": "value1",
						"key2": "value2",
					},
					AuthType: "test-auth-type",
				},
			}

			expectedCredential := cred
			expectedCredential.AuthType = "test-auth-type"
			expectedCredential.Attributes = map[string]string{
				"key1": "value1",
				"key2": "value2",
			}
			expectedCredential.Label = names.NewCloudCredentialTag("test-cloud/alice@external/test-credential-1").String()

			return arg, expectedCredential, ""
		},
	}, {
		about:                  "update credential error returned by controller",
		updateCredentialErrors: []error{nil, errors.New("test error")},
		createEnv: func(c *qt.C, j *jimm.JIMM) (jimm.UpdateCloudCredentialArgs, dbmodel.CloudCredential, string) {
			controller1 := dbmodel.Controller{
				Name: "test-controller-1",
				UUID: "00000000-0000-0000-0000-0000-0000000000001",
			}
			err := j.Database.AddController(context.Background(), &controller1)
			c.Assert(err, qt.Equals, nil)

			controller2 := dbmodel.Controller{
				Name: "test-controller-2",
				UUID: "00000000-0000-0000-0000-0000-0000000000002",
			}
			err = j.Database.AddController(context.Background(), &controller2)
			c.Assert(err, qt.Equals, nil)

			u := dbmodel.User{
				Username:         "alice@external",
				ControllerAccess: "superuser",
			}
			c.Assert(j.Database.DB.Create(&u).Error, qt.IsNil)

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
			c.Assert(j.Database.DB.Create(&cloud).Error, qt.IsNil)

			cred := dbmodel.CloudCredential{
				Name:      "test-credential-1",
				CloudName: cloud.Name,
				OwnerID:   u.Username,
				AuthType:  "empty",
			}
			err = j.Database.SetCloudCredential(context.Background(), &cred)
			c.Assert(err, qt.Equals, nil)

			_, err = j.AddModel(context.Background(), &u, &jimm.ModelCreateArgs{
				Name:            "test-model",
				Owner:           names.NewUserTag(u.Username),
				Cloud:           names.NewCloudTag(cloud.Name),
				CloudRegion:     "test-region-1",
				CloudCredential: names.NewCloudCredentialTag("test-cloud/alice@external/test-credential-1"),
			})
			c.Assert(err, qt.Equals, nil)

			arg := jimm.UpdateCloudCredentialArgs{
				User:          &u,
				CredentialTag: names.NewCloudCredentialTag("test-cloud/alice@external/test-credential-1"),
				Credential: jujuparams.CloudCredential{
					Attributes: map[string]string{
						"key1": "value1",
						"key2": "value2",
					},
					AuthType: "test-auth-type",
				},
			}
			return arg, dbmodel.CloudCredential{}, "controller test-controller-2: cannot update credentials"
		},
	}, {
		about:                  "check credential error returned by controller",
		checkCredentialErrors:  []error{errors.New("test error")},
		updateCredentialErrors: []error{nil},
		createEnv: func(c *qt.C, j *jimm.JIMM) (jimm.UpdateCloudCredentialArgs, dbmodel.CloudCredential, string) {
			controller1 := dbmodel.Controller{
				Name: "test-controller-1",
				UUID: "00000000-0000-0000-0000-0000-0000000000001",
			}
			err := j.Database.AddController(context.Background(), &controller1)
			c.Assert(err, qt.Equals, nil)

			controller2 := dbmodel.Controller{
				Name: "test-controller-2",
				UUID: "00000000-0000-0000-0000-0000-0000000000002",
			}
			err = j.Database.AddController(context.Background(), &controller2)
			c.Assert(err, qt.Equals, nil)

			u := dbmodel.User{
				Username:         "alice@external",
				ControllerAccess: "superuser",
			}
			c.Assert(j.Database.DB.Create(&u).Error, qt.IsNil)

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
			c.Assert(j.Database.DB.Create(&cloud).Error, qt.IsNil)

			cred := dbmodel.CloudCredential{
				Name:      "test-credential-1",
				CloudName: cloud.Name,
				OwnerID:   u.Username,
				AuthType:  "empty",
			}
			err = j.Database.SetCloudCredential(context.Background(), &cred)
			c.Assert(err, qt.Equals, nil)

			_, err = j.AddModel(context.Background(), &u, &jimm.ModelCreateArgs{
				Name:            "test-model",
				Owner:           names.NewUserTag(u.Username),
				Cloud:           names.NewCloudTag(cloud.Name),
				CloudRegion:     "test-region-1",
				CloudCredential: names.NewCloudCredentialTag("test-cloud/alice@external/test-credential-1"),
			})
			c.Assert(err, qt.Equals, nil)

			arg := jimm.UpdateCloudCredentialArgs{
				User:          &u,
				CredentialTag: names.NewCloudCredentialTag("test-cloud/alice@external/test-credential-1"),
				Credential: jujuparams.CloudCredential{
					Attributes: map[string]string{
						"key1": "value1",
						"key2": "value2",
					},
					AuthType: "test-auth-type",
				},
			}
			return arg, dbmodel.CloudCredential{}, "controller test-controller-2: credential check failed"
		},
	}, {
		about: "user not owner of credentials - unauthorized",
		createEnv: func(c *qt.C, j *jimm.JIMM) (jimm.UpdateCloudCredentialArgs, dbmodel.CloudCredential, string) {
			controller1 := dbmodel.Controller{
				Name: "test-controller-1",
				UUID: "00000000-0000-0000-0000-0000-0000000000001",
			}
			err := j.Database.AddController(context.Background(), &controller1)
			c.Assert(err, qt.Equals, nil)

			controller2 := dbmodel.Controller{
				Name: "test-controller-2",
				UUID: "00000000-0000-0000-0000-0000-0000000000002",
			}
			err = j.Database.AddController(context.Background(), &controller2)
			c.Assert(err, qt.Equals, nil)

			u := dbmodel.User{
				Username:         "alice@external",
				ControllerAccess: "superuser",
			}
			c.Assert(j.Database.DB.Create(&u).Error, qt.IsNil)

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
			c.Assert(j.Database.DB.Create(&cloud).Error, qt.IsNil)

			cred := dbmodel.CloudCredential{
				Name:      "test-credential-1",
				CloudName: cloud.Name,
				OwnerID:   u.Username,
				AuthType:  "empty",
			}
			err = j.Database.SetCloudCredential(context.Background(), &cred)
			c.Assert(err, qt.Equals, nil)

			_, err = j.AddModel(context.Background(), &u, &jimm.ModelCreateArgs{
				Name:            "test-model",
				Owner:           names.NewUserTag(u.Username),
				Cloud:           names.NewCloudTag(cloud.Name),
				CloudRegion:     "test-region-1",
				CloudCredential: names.NewCloudCredentialTag("test-cloud/alice@external/test-credential-1"),
			})
			c.Assert(err, qt.Equals, nil)

			arg := jimm.UpdateCloudCredentialArgs{
				User:          &u,
				CredentialTag: names.NewCloudCredentialTag("test-cloud/eve@external/test-credential-1"),
				Credential: jujuparams.CloudCredential{
					Attributes: map[string]string{
						"key1": "value1",
						"key2": "value2",
					},
					AuthType: "test-auth-type",
				},
			}
			return arg, dbmodel.CloudCredential{}, "unauthorized access"
		},
	}, {
		about:                 "skip check, which would return an error",
		checkCredentialErrors: []error{errors.New("test error")},
		createEnv: func(c *qt.C, j *jimm.JIMM) (jimm.UpdateCloudCredentialArgs, dbmodel.CloudCredential, string) {
			controller1 := dbmodel.Controller{
				Name: "test-controller-1",
				UUID: "00000000-0000-0000-0000-0000-0000000000001",
			}
			err := j.Database.AddController(context.Background(), &controller1)
			c.Assert(err, qt.Equals, nil)

			controller2 := dbmodel.Controller{
				Name: "test-controller-2",
				UUID: "00000000-0000-0000-0000-0000-0000000000002",
			}
			err = j.Database.AddController(context.Background(), &controller2)
			c.Assert(err, qt.Equals, nil)

			u := dbmodel.User{
				Username:         "alice@external",
				ControllerAccess: "superuser",
			}
			c.Assert(j.Database.DB.Create(&u).Error, qt.IsNil)

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
			c.Assert(j.Database.DB.Create(&cloud).Error, qt.IsNil)

			cred := dbmodel.CloudCredential{
				Name:      "test-credential-1",
				CloudName: cloud.Name,
				OwnerID:   u.Username,
				AuthType:  "empty",
			}
			err = j.Database.SetCloudCredential(context.Background(), &cred)
			c.Assert(err, qt.Equals, nil)

			_, err = j.AddModel(context.Background(), &u, &jimm.ModelCreateArgs{
				Name:            "test-model",
				Owner:           names.NewUserTag(u.Username),
				Cloud:           names.NewCloudTag(cloud.Name),
				CloudRegion:     "test-region-1",
				CloudCredential: names.NewCloudCredentialTag("test-cloud/alice@external/test-credential-1"),
			})
			c.Assert(err, qt.Equals, nil)

			arg := jimm.UpdateCloudCredentialArgs{
				User:          &u,
				CredentialTag: names.NewCloudCredentialTag("test-cloud/alice@external/test-credential-1"),
				Credential: jujuparams.CloudCredential{
					Attributes: map[string]string{
						"key1": "value1",
						"key2": "value2",
					},
					AuthType: "test-auth-type",
				},
				SkipCheck: true,
			}

			expectedCredential := cred
			expectedCredential.AuthType = "test-auth-type"
			expectedCredential.Attributes = map[string]string{
				"key1": "value1",
				"key2": "value2",
			}
			expectedCredential.Label = names.NewCloudCredentialTag("test-cloud/alice@external/test-credential-1").String()

			return arg, expectedCredential, ""
		},
	}, {
		about: "skip update",
		createEnv: func(c *qt.C, j *jimm.JIMM) (jimm.UpdateCloudCredentialArgs, dbmodel.CloudCredential, string) {
			controller1 := dbmodel.Controller{
				Name: "test-controller-1",
				UUID: "00000000-0000-0000-0000-0000-0000000000001",
			}
			err := j.Database.AddController(context.Background(), &controller1)
			c.Assert(err, qt.Equals, nil)

			controller2 := dbmodel.Controller{
				Name: "test-controller-2",
				UUID: "00000000-0000-0000-0000-0000-0000000000002",
			}
			err = j.Database.AddController(context.Background(), &controller2)
			c.Assert(err, qt.Equals, nil)

			u := dbmodel.User{
				Username:         "alice@external",
				ControllerAccess: "superuser",
			}
			c.Assert(j.Database.DB.Create(&u).Error, qt.IsNil)

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
			c.Assert(j.Database.DB.Create(&cloud).Error, qt.IsNil)

			cred := dbmodel.CloudCredential{
				Name:      "test-credential-1",
				CloudName: cloud.Name,
				OwnerID:   u.Username,
				AuthType:  "empty",
			}
			err = j.Database.SetCloudCredential(context.Background(), &cred)
			c.Assert(err, qt.Equals, nil)

			_, err = j.AddModel(context.Background(), &u, &jimm.ModelCreateArgs{
				Name:            "test-model",
				Owner:           names.NewUserTag(u.Username),
				Cloud:           names.NewCloudTag(cloud.Name),
				CloudRegion:     "test-region-1",
				CloudCredential: names.NewCloudCredentialTag("test-cloud/alice@external/test-credential-1"),
			})
			c.Assert(err, qt.Equals, nil)

			arg := jimm.UpdateCloudCredentialArgs{
				User:          &u,
				CredentialTag: names.NewCloudCredentialTag("test-cloud/alice@external/test-credential-1"),
				Credential: jujuparams.CloudCredential{
					Attributes: map[string]string{
						"key1": "value1",
						"key2": "value2",
					},
					AuthType: "test-auth-type",
				},
				SkipUpdate: true,
			}

			return arg, cred, ""
		},
	}}
	for _, test := range tests {
		c.Run(test.about, func(c *qt.C) {
			checkErrors := test.checkCredentialErrors
			updateErrors := test.updateCredentialErrors
			api := &jimmtest.API{
				SupportsCheckCredentialModels_: true,
				CheckCredentialModels_: func(context.Context, jujuparams.TaggedCredential) ([]jujuparams.UpdateCredentialModelResult, error) {
					if len(checkErrors) > 0 {
						var err error
						err, checkErrors = checkErrors[0], checkErrors[1:]
						if err == nil {
							return []jujuparams.UpdateCredentialModelResult{{
								ModelUUID: "00000001-0000-0000-0000-0000-000000000001",
								ModelName: "test-model",
							}}, nil
						} else {
							return []jujuparams.UpdateCredentialModelResult{{
								ModelUUID: "00000001-0000-0000-0000-0000-000000000001",
								ModelName: "test-model",
								Errors: []jujuparams.ErrorResult{{
									Error: &jujuparams.Error{
										Message: err.Error(),
										Code:    "test-error",
									},
								}},
							}}, err
						}
					} else {
						return []jujuparams.UpdateCredentialModelResult{{
							ModelUUID: "00000001-0000-0000-0000-0000-000000000001",
							ModelName: "test-model",
						}}, nil
					}
				},
				UpdateCredential_: func(context.Context, jujuparams.TaggedCredential) ([]jujuparams.UpdateCredentialModelResult, error) {
					if len(updateErrors) > 0 {
						var err error
						err, updateErrors = updateErrors[0], updateErrors[1:]
						if err == nil {
							return []jujuparams.UpdateCredentialModelResult{{
								ModelUUID: "00000001-0000-0000-0000-0000-000000000001",
								ModelName: "test-model",
							}}, nil
						} else {
							return []jujuparams.UpdateCredentialModelResult{{
								ModelUUID: "00000001-0000-0000-0000-0000-000000000001",
								ModelName: "test-model",
								Errors: []jujuparams.ErrorResult{{
									Error: &jujuparams.Error{
										Message: err.Error(),
										Code:    "test-error",
									},
								}},
							}}, err
						}
					} else {
						return []jujuparams.UpdateCredentialModelResult{{
							ModelUUID: "00000001-0000-0000-0000-0000-000000000001",
							ModelName: "test-model",
						}}, nil
					}
				},
				GrantJIMMModelAdmin_: func(_ context.Context, _ names.ModelTag) error {
					return nil
				},
				CreateModel_: func(ctx context.Context, args *jujuparams.ModelCreateArgs, mi *jujuparams.ModelInfo) error {
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

			arg, expectedCredential, expectedError := test.createEnv(c, j)

			result, err := j.UpdateCloudCredential(ctx, arg)
			if expectedError == "" {
				c.Assert(err, qt.Equals, nil)
				if !arg.SkipUpdate {
					c.Assert(result, qt.HasLen, 1)
					c.Assert(result[0].Errors, qt.HasLen, 0)
					c.Assert(result[0].ModelName, qt.Equals, "test-model")
					c.Assert(result[0].ModelUUID, qt.Equals, "00000001-0000-0000-0000-0000-000000000001")
				} else {
					c.Assert(result, qt.HasLen, 0)
				}
				credential := dbmodel.CloudCredential{
					Name:      expectedCredential.Name,
					CloudName: expectedCredential.CloudName,
					OwnerID:   expectedCredential.OwnerID,
				}
				err = j.Database.GetCloudCredential(ctx, &credential)
				c.Assert(err, qt.Equals, nil)
				c.Assert(credential, qt.DeepEquals, expectedCredential)
			} else {
				c.Assert(err, qt.ErrorMatches, expectedError)
			}
		})
	}
}
