// Copyright 2020 Canonical Ltd.

package jimm_test

import (
	"context"
	"database/sql"
	"sync"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
	"github.com/google/uuid"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/status"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v4"
	"github.com/lestrrat-go/jwx/v2/jwk"

	"github.com/canonical/jimm/internal/db"
	"github.com/canonical/jimm/internal/dbmodel"
	"github.com/canonical/jimm/internal/errors"
	"github.com/canonical/jimm/internal/jimm"
	"github.com/canonical/jimm/internal/jimmtest"
	"github.com/canonical/jimm/internal/openfga"
	ofganames "github.com/canonical/jimm/internal/openfga/names"
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
		createEnv              func(*qt.C, *jimm.JIMM, *openfga.OFGAClient) (*dbmodel.User, jimm.UpdateCloudCredentialArgs, dbmodel.CloudCredential, string)
	}{{
		about: "all ok",
		createEnv: func(c *qt.C, j *jimm.JIMM, client *openfga.OFGAClient) (*dbmodel.User, jimm.UpdateCloudCredentialArgs, dbmodel.CloudCredential, string) {
			u := dbmodel.User{
				Username: "alice@external",
			}
			c.Assert(j.Database.DB.Create(&u).Error, qt.IsNil)

			user := openfga.NewUser(&u, client)
			err := user.SetControllerAccess(context.Background(), j.ResourceTag(), ofganames.AdministratorRelation)
			c.Assert(err, qt.IsNil)

			cloud := dbmodel.Cloud{
				Name: "test-cloud",
				Type: "test-provider",
				Regions: []dbmodel.CloudRegion{{
					Name: "test-region-1",
				}},
				Users: []dbmodel.UserCloudAccess{{
					Username: u.Username,
					Access:   "admin",
				}},
			}
			c.Assert(j.Database.DB.Create(&cloud).Error, qt.IsNil)

			err = user.SetCloudAccess(context.Background(), cloud.ResourceTag(), ofganames.AdministratorRelation)
			c.Assert(err, qt.IsNil)

			controller1 := dbmodel.Controller{
				Name:        "test-controller-1",
				UUID:        "00000000-0000-0000-0000-0000-0000000000001",
				CloudName:   "test-cloud",
				CloudRegion: "test-region-1",
				CloudRegions: []dbmodel.CloudRegionControllerPriority{{
					Priority:      0,
					CloudRegionID: cloud.Regions[0].ID,
				}},
			}
			err = j.Database.AddController(context.Background(), &controller1)
			c.Assert(err, qt.Equals, nil)

			controller2 := dbmodel.Controller{
				Name:        "test-controller-2",
				UUID:        "00000000-0000-0000-0000-0000-0000000000002",
				CloudName:   "test-cloud",
				CloudRegion: "test-region-1",
				CloudRegions: []dbmodel.CloudRegionControllerPriority{{
					// controller2 has a higher priority and the model
					// should be created on this controller
					Priority:      2,
					CloudRegionID: cloud.Regions[0].ID,
				}},
			}
			err = j.Database.AddController(context.Background(), &controller2)
			c.Assert(err, qt.Equals, nil)

			cred := dbmodel.CloudCredential{
				Name:          "test-credential-1",
				CloudName:     cloud.Name,
				OwnerUsername: u.Username,
				AuthType:      "empty",
			}
			err = j.Database.SetCloudCredential(context.Background(), &cred)
			c.Assert(err, qt.Equals, nil)

			cred.Cloud = dbmodel.Cloud{
				Name: "test-cloud",
				Type: "test-provider",
			}

			mi, err := j.AddModel(context.Background(), user, &jimm.ModelCreateArgs{
				Name:            "test-model",
				Owner:           names.NewUserTag(u.Username),
				Cloud:           names.NewCloudTag(cloud.Name),
				CloudRegion:     "test-region-1",
				CloudCredential: names.NewCloudCredentialTag("test-cloud/alice@external/test-credential-1"),
			})
			c.Assert(err, qt.Equals, nil)

			arg := jimm.UpdateCloudCredentialArgs{
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

			m := dbmodel.Model{
				UUID: sql.NullString{
					String: mi.UUID,
					Valid:  true,
				},
			}
			err = j.Database.GetModel(context.Background(), &m)
			c.Assert(err, qt.IsNil)
			// Clear some fields we don't need.
			// TODO(mhilton) don't fetch these in the first place.
			m.Owner = dbmodel.User{}
			m.Controller = dbmodel.Controller{}
			m.CloudCredential = dbmodel.CloudCredential{}
			m.CloudRegion = dbmodel.CloudRegion{}
			m.Users[0].User = dbmodel.User{}

			expectedCredential.Models = []dbmodel.Model{m}

			return &u, arg, expectedCredential, ""
		},
	}, {
		about:                  "update credential error returned by controller",
		updateCredentialErrors: []error{nil, errors.E("test error")},
		createEnv: func(c *qt.C, j *jimm.JIMM, client *openfga.OFGAClient) (*dbmodel.User, jimm.UpdateCloudCredentialArgs, dbmodel.CloudCredential, string) {
			u := dbmodel.User{
				Username: "alice@external",
			}
			c.Assert(j.Database.DB.Create(&u).Error, qt.IsNil)

			user := openfga.NewUser(&u, client)

			err := user.SetControllerAccess(context.Background(), j.ResourceTag(), ofganames.AdministratorRelation)
			c.Assert(err, qt.IsNil)

			cloud := dbmodel.Cloud{
				Name: "test-cloud",
				Type: "test-provider",
				Regions: []dbmodel.CloudRegion{{
					Name: "test-region-1",
				}},
				Users: []dbmodel.UserCloudAccess{{
					Username: u.Username,
					Access:   "admin",
				}},
			}
			c.Assert(j.Database.DB.Create(&cloud).Error, qt.IsNil)

			err = user.SetCloudAccess(context.Background(), cloud.ResourceTag(), ofganames.AdministratorRelation)
			c.Assert(err, qt.IsNil)

			controller1 := dbmodel.Controller{
				Name:        "test-controller-1",
				UUID:        "00000000-0000-0000-0000-0000-0000000000001",
				CloudName:   "test-cloud",
				CloudRegion: "test-region-1",
				CloudRegions: []dbmodel.CloudRegionControllerPriority{{
					Priority:      0,
					CloudRegionID: cloud.Regions[0].ID,
				}},
			}
			err = j.Database.AddController(context.Background(), &controller1)
			c.Assert(err, qt.Equals, nil)

			controller2 := dbmodel.Controller{
				Name:        "test-controller-2",
				UUID:        "00000000-0000-0000-0000-0000-0000000000002",
				CloudName:   "test-cloud",
				CloudRegion: "test-region-1",
				CloudRegions: []dbmodel.CloudRegionControllerPriority{{
					// controller2 has a higher priority and the model
					// should be created on this controller
					Priority:      2,
					CloudRegionID: cloud.Regions[0].ID,
				}},
			}
			err = j.Database.AddController(context.Background(), &controller2)
			c.Assert(err, qt.Equals, nil)

			cred := dbmodel.CloudCredential{
				Name:          "test-credential-1",
				CloudName:     cloud.Name,
				OwnerUsername: u.Username,
				AuthType:      "empty",
			}
			err = j.Database.SetCloudCredential(context.Background(), &cred)
			c.Assert(err, qt.Equals, nil)

			cred.Cloud = dbmodel.Cloud{
				Name: "test-cloud",
				Type: "test-provider",
			}

			_, err = j.AddModel(context.Background(), user, &jimm.ModelCreateArgs{
				Name:            "test-model",
				Owner:           names.NewUserTag(u.Username),
				Cloud:           names.NewCloudTag(cloud.Name),
				CloudRegion:     "test-region-1",
				CloudCredential: names.NewCloudCredentialTag("test-cloud/alice@external/test-credential-1"),
			})
			c.Assert(err, qt.Equals, nil)

			arg := jimm.UpdateCloudCredentialArgs{
				CredentialTag: names.NewCloudCredentialTag("test-cloud/alice@external/test-credential-1"),
				Credential: jujuparams.CloudCredential{
					Attributes: map[string]string{
						"key1": "value1",
						"key2": "value2",
					},
					AuthType: "test-auth-type",
				},
			}
			return &u, arg, dbmodel.CloudCredential{}, "test error"
		},
	}, {
		about:                  "check credential error returned by controller",
		checkCredentialErrors:  []error{errors.E("test error")},
		updateCredentialErrors: []error{nil},
		createEnv: func(c *qt.C, j *jimm.JIMM, client *openfga.OFGAClient) (*dbmodel.User, jimm.UpdateCloudCredentialArgs, dbmodel.CloudCredential, string) {
			u := dbmodel.User{
				Username: "alice@external",
			}
			c.Assert(j.Database.DB.Create(&u).Error, qt.IsNil)

			user := openfga.NewUser(&u, client)

			err := user.SetControllerAccess(context.Background(), j.ResourceTag(), ofganames.AdministratorRelation)
			c.Assert(err, qt.IsNil)

			cloud := dbmodel.Cloud{
				Name: "test-cloud",
				Type: "test-provider",
				Regions: []dbmodel.CloudRegion{{
					Name: "test-region-1",
				}},
				Users: []dbmodel.UserCloudAccess{{
					Username: u.Username,
					Access:   "admin",
				}},
			}
			c.Assert(j.Database.DB.Create(&cloud).Error, qt.IsNil)

			err = user.SetCloudAccess(context.Background(), cloud.ResourceTag(), ofganames.AdministratorRelation)
			c.Assert(err, qt.IsNil)

			controller1 := dbmodel.Controller{
				Name:        "test-controller-1",
				UUID:        "00000000-0000-0000-0000-0000-0000000000001",
				CloudName:   "test-cloud",
				CloudRegion: "test-region-1",
				CloudRegions: []dbmodel.CloudRegionControllerPriority{{
					Priority:      0,
					CloudRegionID: cloud.Regions[0].ID,
				}},
			}
			err = j.Database.AddController(context.Background(), &controller1)
			c.Assert(err, qt.Equals, nil)

			controller2 := dbmodel.Controller{
				Name:        "test-controller-2",
				UUID:        "00000000-0000-0000-0000-0000-0000000000002",
				CloudName:   "test-cloud",
				CloudRegion: "test-region-1",
				CloudRegions: []dbmodel.CloudRegionControllerPriority{{
					// controller2 has a higher priority and the model
					// should be created on this controller
					Priority:      2,
					CloudRegionID: cloud.Regions[0].ID,
				}},
			}
			err = j.Database.AddController(context.Background(), &controller2)
			c.Assert(err, qt.Equals, nil)

			cred := dbmodel.CloudCredential{
				Name:          "test-credential-1",
				CloudName:     cloud.Name,
				OwnerUsername: u.Username,
				AuthType:      "empty",
			}
			err = j.Database.SetCloudCredential(context.Background(), &cred)
			c.Assert(err, qt.Equals, nil)

			_, err = j.AddModel(context.Background(), user, &jimm.ModelCreateArgs{
				Name:            "test-model",
				Owner:           names.NewUserTag(u.Username),
				Cloud:           names.NewCloudTag(cloud.Name),
				CloudRegion:     "test-region-1",
				CloudCredential: names.NewCloudCredentialTag("test-cloud/alice@external/test-credential-1"),
			})
			c.Assert(err, qt.Equals, nil)

			arg := jimm.UpdateCloudCredentialArgs{
				CredentialTag: names.NewCloudCredentialTag("test-cloud/alice@external/test-credential-1"),
				Credential: jujuparams.CloudCredential{
					Attributes: map[string]string{
						"key1": "value1",
						"key2": "value2",
					},
					AuthType: "test-auth-type",
				},
			}
			return &u, arg, dbmodel.CloudCredential{}, "test error"
		},
	}, {
		about: "user is controller superuser",
		createEnv: func(c *qt.C, j *jimm.JIMM, client *openfga.OFGAClient) (*dbmodel.User, jimm.UpdateCloudCredentialArgs, dbmodel.CloudCredential, string) {
			u := dbmodel.User{
				Username: "alice@external",
			}
			c.Assert(j.Database.DB.Create(&u).Error, qt.IsNil)

			alice := openfga.NewUser(&u, client)

			err := alice.SetControllerAccess(context.Background(), j.ResourceTag(), ofganames.AdministratorRelation)
			c.Assert(err, qt.IsNil)

			eve := dbmodel.User{
				Username: "eve@external",
			}
			c.Assert(j.Database.DB.Create(&eve).Error, qt.IsNil)

			cloud := dbmodel.Cloud{
				Name: "test-cloud",
				Type: "test-provider",
				Regions: []dbmodel.CloudRegion{{
					Name: "test-region-1",
				}},
				Users: []dbmodel.UserCloudAccess{{
					Username: u.Username,
					Access:   "admin",
				}},
			}
			c.Assert(j.Database.DB.Create(&cloud).Error, qt.IsNil)

			err = alice.SetCloudAccess(context.Background(), cloud.ResourceTag(), ofganames.AdministratorRelation)
			c.Assert(err, qt.IsNil)

			controller1 := dbmodel.Controller{
				Name:        "test-controller-1",
				UUID:        "00000000-0000-0000-0000-0000-0000000000001",
				CloudName:   "test-cloud",
				CloudRegion: "test-region-1",
				CloudRegions: []dbmodel.CloudRegionControllerPriority{{
					Priority:      0,
					CloudRegionID: cloud.Regions[0].ID,
				}},
			}
			err = j.Database.AddController(context.Background(), &controller1)
			c.Assert(err, qt.Equals, nil)

			controller2 := dbmodel.Controller{
				Name:        "test-controller-2",
				UUID:        "00000000-0000-0000-0000-0000-0000000000002",
				CloudName:   "test-cloud",
				CloudRegion: "test-region-1",
				CloudRegions: []dbmodel.CloudRegionControllerPriority{{
					// controller2 has a higher priority and the model
					// should be created on this controller
					Priority:      2,
					CloudRegionID: cloud.Regions[0].ID,
				}},
			}
			err = j.Database.AddController(context.Background(), &controller2)
			c.Assert(err, qt.Equals, nil)

			cred := dbmodel.CloudCredential{
				Name:          "test-credential-1",
				CloudName:     cloud.Name,
				OwnerUsername: eve.Username,
				AuthType:      "empty",
			}
			err = j.Database.SetCloudCredential(context.Background(), &cred)
			c.Assert(err, qt.Equals, nil)

			mi, err := j.AddModel(context.Background(), alice, &jimm.ModelCreateArgs{
				Name:            "test-model",
				Owner:           names.NewUserTag("eve@external"),
				Cloud:           names.NewCloudTag(cloud.Name),
				CloudRegion:     "test-region-1",
				CloudCredential: names.NewCloudCredentialTag("test-cloud/eve@external/test-credential-1"),
			})
			c.Assert(err, qt.Equals, nil)

			arg := jimm.UpdateCloudCredentialArgs{
				CredentialTag: names.NewCloudCredentialTag("test-cloud/eve@external/test-credential-1"),
				Credential: jujuparams.CloudCredential{
					Attributes: map[string]string{
						"key1": "value1",
						"key2": "value2",
					},
					AuthType: "test-auth-type",
				},
			}
			m := dbmodel.Model{
				UUID: sql.NullString{
					String: mi.UUID,
					Valid:  true,
				},
			}
			err = j.Database.GetModel(context.Background(), &m)
			c.Assert(err, qt.IsNil)
			// Clear some fields we don't need.
			// TODO(mhilton) don't fetch these in the first place.
			m.Owner = dbmodel.User{}
			m.Controller = dbmodel.Controller{}
			m.CloudCredential = dbmodel.CloudCredential{}
			m.CloudRegion = dbmodel.CloudRegion{}
			m.Users[0].User = dbmodel.User{}

			return &u, arg, dbmodel.CloudCredential{
				Name:      "test-credential-1",
				CloudName: cloud.Name,
				Cloud: dbmodel.Cloud{
					Name: cloud.Name,
					Type: cloud.Type,
				},
				OwnerUsername: eve.Username,
				Attributes: map[string]string{
					"key1": "value1",
					"key2": "value2",
				},
				AuthType: "test-auth-type",
				Models:   []dbmodel.Model{m},
			}, ""
		},
	}, {
		about:                 "skip check, which would return an error",
		checkCredentialErrors: []error{errors.E("test error")},
		createEnv: func(c *qt.C, j *jimm.JIMM, client *openfga.OFGAClient) (*dbmodel.User, jimm.UpdateCloudCredentialArgs, dbmodel.CloudCredential, string) {
			u := dbmodel.User{
				Username: "alice@external",
			}
			c.Assert(j.Database.DB.Create(&u).Error, qt.IsNil)

			user := openfga.NewUser(&u, client)

			err := user.SetControllerAccess(context.Background(), j.ResourceTag(), ofganames.AdministratorRelation)
			c.Assert(err, qt.IsNil)

			cloud := dbmodel.Cloud{
				Name: "test-cloud",
				Type: "test-provider",
				Regions: []dbmodel.CloudRegion{{
					Name: "test-region-1",
				}},
				Users: []dbmodel.UserCloudAccess{{
					Username: u.Username,
					Access:   "admin",
				}},
			}
			c.Assert(j.Database.DB.Create(&cloud).Error, qt.IsNil)

			err = user.SetCloudAccess(context.Background(), cloud.ResourceTag(), ofganames.AdministratorRelation)
			c.Assert(err, qt.IsNil)

			controller1 := dbmodel.Controller{
				Name:        "test-controller-1",
				UUID:        "00000000-0000-0000-0000-0000-0000000000001",
				CloudName:   "test-cloud",
				CloudRegion: "test-region-1",
				CloudRegions: []dbmodel.CloudRegionControllerPriority{{
					Priority:      0,
					CloudRegionID: cloud.Regions[0].ID,
				}},
			}
			err = j.Database.AddController(context.Background(), &controller1)
			c.Assert(err, qt.Equals, nil)

			controller2 := dbmodel.Controller{
				Name:        "test-controller-2",
				UUID:        "00000000-0000-0000-0000-0000-0000000000002",
				CloudName:   "test-cloud",
				CloudRegion: "test-region-1",
				CloudRegions: []dbmodel.CloudRegionControllerPriority{{
					// controller2 has a higher priority and the model
					// should be created on this controller
					Priority:      2,
					CloudRegionID: cloud.Regions[0].ID,
				}},
			}
			err = j.Database.AddController(context.Background(), &controller2)
			c.Assert(err, qt.Equals, nil)

			cred := dbmodel.CloudCredential{
				Name:          "test-credential-1",
				CloudName:     cloud.Name,
				OwnerUsername: u.Username,
				AuthType:      "empty",
			}
			err = j.Database.SetCloudCredential(context.Background(), &cred)
			c.Assert(err, qt.Equals, nil)

			cred.Cloud = dbmodel.Cloud{
				Name: "test-cloud",
				Type: "test-provider",
			}

			mi, err := j.AddModel(context.Background(), user, &jimm.ModelCreateArgs{
				Name:            "test-model",
				Owner:           names.NewUserTag(u.Username),
				Cloud:           names.NewCloudTag(cloud.Name),
				CloudRegion:     "test-region-1",
				CloudCredential: names.NewCloudCredentialTag("test-cloud/alice@external/test-credential-1"),
			})
			c.Assert(err, qt.Equals, nil)

			arg := jimm.UpdateCloudCredentialArgs{
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
			m := dbmodel.Model{
				UUID: sql.NullString{
					String: mi.UUID,
					Valid:  true,
				},
			}
			err = j.Database.GetModel(context.Background(), &m)
			c.Assert(err, qt.IsNil)
			// Clear some fields we don't need.
			// TODO(mhilton) don't fetch these in the first place.
			m.Owner = dbmodel.User{}
			m.Controller = dbmodel.Controller{}
			m.CloudCredential = dbmodel.CloudCredential{}
			m.CloudRegion = dbmodel.CloudRegion{}
			m.Users[0].User = dbmodel.User{}
			expectedCredential.Models = []dbmodel.Model{m}

			return &u, arg, expectedCredential, ""
		},
	}, {
		about: "skip update",
		createEnv: func(c *qt.C, j *jimm.JIMM, client *openfga.OFGAClient) (*dbmodel.User, jimm.UpdateCloudCredentialArgs, dbmodel.CloudCredential, string) {
			u := dbmodel.User{
				Username: "alice@external",
			}
			c.Assert(j.Database.DB.Create(&u).Error, qt.IsNil)

			user := openfga.NewUser(&u, client)

			err := user.SetControllerAccess(context.Background(), j.ResourceTag(), ofganames.AdministratorRelation)
			c.Assert(err, qt.IsNil)

			cloud := dbmodel.Cloud{
				Name: "test-cloud",
				Type: "test-provider",
				Regions: []dbmodel.CloudRegion{{
					Name: "test-region-1",
				}},
				Users: []dbmodel.UserCloudAccess{{
					Username: u.Username,
					Access:   "admin",
				}},
			}
			c.Assert(j.Database.DB.Create(&cloud).Error, qt.IsNil)

			err = user.SetCloudAccess(context.Background(), cloud.ResourceTag(), ofganames.AdministratorRelation)
			c.Assert(err, qt.IsNil)

			controller1 := dbmodel.Controller{
				Name:        "test-controller-1",
				UUID:        "00000000-0000-0000-0000-0000-0000000000001",
				CloudName:   "test-cloud",
				CloudRegion: "test-region-1",
				CloudRegions: []dbmodel.CloudRegionControllerPriority{{
					Priority:      0,
					CloudRegionID: cloud.Regions[0].ID,
				}},
			}
			err = j.Database.AddController(context.Background(), &controller1)
			c.Assert(err, qt.Equals, nil)

			controller2 := dbmodel.Controller{
				Name:        "test-controller-2",
				UUID:        "00000000-0000-0000-0000-0000-0000000000002",
				CloudName:   "test-cloud",
				CloudRegion: "test-region-1",
				CloudRegions: []dbmodel.CloudRegionControllerPriority{{
					// controller2 has a higher priority and the model
					// should be created on this controller
					Priority:      2,
					CloudRegionID: cloud.Regions[0].ID,
				}},
			}
			err = j.Database.AddController(context.Background(), &controller2)
			c.Assert(err, qt.Equals, nil)

			cred := dbmodel.CloudCredential{
				Name:          "test-credential-1",
				CloudName:     cloud.Name,
				OwnerUsername: u.Username,
				AuthType:      "empty",
			}
			err = j.Database.SetCloudCredential(context.Background(), &cred)
			c.Assert(err, qt.Equals, nil)

			cred.Cloud = dbmodel.Cloud{
				Name: "test-cloud",
				Type: "test-provider",
			}
			mi, err := j.AddModel(context.Background(), user, &jimm.ModelCreateArgs{
				Name:            "test-model",
				Owner:           names.NewUserTag(u.Username),
				Cloud:           names.NewCloudTag(cloud.Name),
				CloudRegion:     "test-region-1",
				CloudCredential: names.NewCloudCredentialTag("test-cloud/alice@external/test-credential-1"),
			})
			c.Assert(err, qt.Equals, nil)

			arg := jimm.UpdateCloudCredentialArgs{
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

			m := dbmodel.Model{
				UUID: sql.NullString{
					String: mi.UUID,
					Valid:  true,
				},
			}
			err = j.Database.GetModel(context.Background(), &m)
			c.Assert(err, qt.IsNil)
			// Clear some fields we don't need.
			// TODO(mhilton) don't fetch these in the first place.
			m.Owner = dbmodel.User{}
			m.Controller = dbmodel.Controller{}
			m.CloudCredential = dbmodel.CloudCredential{}
			m.CloudRegion = dbmodel.CloudRegion{}
			m.Users[0].User = dbmodel.User{}
			cred.Models = []dbmodel.Model{m}

			return &u, arg, cred, ""
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

			client, _, _, err := jimmtest.SetupTestOFGAClient(c.Name())
			c.Assert(err, qt.IsNil)

			j := &jimm.JIMM{
				UUID: uuid.NewString(),
				Database: db.Database{
					DB: jimmtest.MemoryDB(c, func() time.Time { return now }),
				},
				Dialer: &jimmtest.Dialer{
					API: api,
				},
				OpenFGAClient: client,
			}

			ctx := context.Background()
			err = j.Database.Migrate(ctx, false)
			c.Assert(err, qt.IsNil)

			u, arg, expectedCredential, expectedError := test.createEnv(c, j, client)
			user := openfga.NewUser(u, client)
			result, err := j.UpdateCloudCredential(ctx, user, arg)
			if expectedError == "" {
				c.Assert(err, qt.Equals, nil)
				c.Assert(result, qt.HasLen, 1)
				c.Assert(result[0].Errors, qt.HasLen, 0)
				c.Assert(result[0].ModelName, qt.Equals, "test-model")
				c.Assert(result[0].ModelUUID, qt.Equals, "00000001-0000-0000-0000-0000-000000000001")
				credential := dbmodel.CloudCredential{
					Name:          expectedCredential.Name,
					CloudName:     expectedCredential.CloudName,
					OwnerUsername: expectedCredential.OwnerUsername,
				}
				err = j.Database.GetCloudCredential(ctx, &credential)
				c.Assert(err, qt.Equals, nil)
				c.Assert(credential, jimmtest.DBObjectEquals, expectedCredential)
			} else {
				c.Assert(err, qt.ErrorMatches, expectedError)
			}
		})
	}
}

func TestUpdateCloudCredentialForUnknownUser(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	client, _, _, err := jimmtest.SetupTestOFGAClient(c.Name())
	c.Assert(err, qt.IsNil)

	env := jimmtest.ParseEnvironment(c, `clouds:
- name: test-cloud
  type: `+jimmtest.TestProviderType+`
  regions:
  - name: default
users:
- username: alice@external
  controller-access: superuser
`)
	j := &jimm.JIMM{
		UUID: uuid.NewString(),
		Database: db.Database{
			DB: jimmtest.MemoryDB(c, nil),
		},
		Dialer: &jimmtest.Dialer{
			API: &jimmtest.API{},
		},
		OpenFGAClient: client,
	}

	err = j.Database.Migrate(ctx, false)
	c.Assert(err, qt.IsNil)
	env.PopulateDB(c, j.Database, client)
	u := env.User("alice@external").DBObject(c, j.Database, client)
	user := openfga.NewUser(&u, client)
	_, err = j.UpdateCloudCredential(ctx, user, jimm.UpdateCloudCredentialArgs{
		CredentialTag: names.NewCloudCredentialTag("test-cloud/bob@external/test"),
		Credential: jujuparams.CloudCredential{
			AuthType: "empty",
		},
	})
	c.Assert(err, qt.IsNil)
}

func TestRevokeCloudCredential(t *testing.T) {
	c := qt.New(t)

	now := time.Now().UTC().Round(time.Millisecond)
	arch := "amd64"
	mem := uint64(8096)
	cores := uint64(8)

	tests := []struct {
		about                  string
		revokeCredentialErrors []error
		createEnv              func(*qt.C, *jimm.JIMM, *openfga.OFGAClient) (*dbmodel.User, names.CloudCredentialTag, string)
	}{{
		about: "credential revoked",
		createEnv: func(c *qt.C, j *jimm.JIMM, client *openfga.OFGAClient) (*dbmodel.User, names.CloudCredentialTag, string) {
			u := dbmodel.User{
				Username: "alice@external",
			}
			c.Assert(j.Database.DB.Create(&u).Error, qt.IsNil)

			alice := openfga.NewUser(&u, client)

			err := alice.SetControllerAccess(context.Background(), j.ResourceTag(), ofganames.AdministratorRelation)
			c.Assert(err, qt.IsNil)

			cloud := dbmodel.Cloud{
				Name: "test-cloud",
				Type: "test-provider",
				Regions: []dbmodel.CloudRegion{{
					Name: "test-region-1",
				}},
				Users: []dbmodel.UserCloudAccess{{
					Username: u.Username,
					Access:   "admin",
				}},
			}
			c.Assert(j.Database.DB.Create(&cloud).Error, qt.IsNil)

			err = alice.SetCloudAccess(context.Background(), cloud.ResourceTag(), ofganames.AdministratorRelation)
			c.Assert(err, qt.IsNil)

			controller1 := dbmodel.Controller{
				Name:        "test-controller-1",
				UUID:        "00000000-0000-0000-0000-0000-0000000000001",
				CloudName:   "test-cloud",
				CloudRegion: "test-region-1",
				CloudRegions: []dbmodel.CloudRegionControllerPriority{{
					Priority:      0,
					CloudRegionID: cloud.Regions[0].ID,
				}},
			}
			err = j.Database.AddController(context.Background(), &controller1)
			c.Assert(err, qt.Equals, nil)

			controller2 := dbmodel.Controller{
				Name:        "test-controller-2",
				UUID:        "00000000-0000-0000-0000-0000-0000000000002",
				CloudName:   "test-cloud",
				CloudRegion: "test-region-1",
				CloudRegions: []dbmodel.CloudRegionControllerPriority{{
					// controller2 has a higher priority and the model
					// should be created on this controller
					Priority:      2,
					CloudRegionID: cloud.Regions[0].ID,
				}},
			}
			err = j.Database.AddController(context.Background(), &controller2)
			c.Assert(err, qt.Equals, nil)

			cred := dbmodel.CloudCredential{
				Name:          "test-credential-1",
				CloudName:     cloud.Name,
				OwnerUsername: u.Username,
				AuthType:      "empty",
			}
			err = j.Database.SetCloudCredential(context.Background(), &cred)
			c.Assert(err, qt.Equals, nil)

			cred.Cloud = dbmodel.Cloud{
				Name: "test-cloud",
				Type: "test-provider",
			}

			tag := names.NewCloudCredentialTag("test-cloud/alice@external/test-credential-1")
			return &u, tag, ""
		},
	}, {
		about: "credential revoked - controller returns a not found error",
		revokeCredentialErrors: []error{&errors.Error{
			Message: "credential not found",
			Code:    jujuparams.CodeNotFound,
		}},
		createEnv: func(c *qt.C, j *jimm.JIMM, client *openfga.OFGAClient) (*dbmodel.User, names.CloudCredentialTag, string) {
			u := dbmodel.User{
				Username: "alice@external",
			}
			c.Assert(j.Database.DB.Create(&u).Error, qt.IsNil)

			alice := openfga.NewUser(&u, client)

			err := alice.SetControllerAccess(context.Background(), j.ResourceTag(), ofganames.AdministratorRelation)
			c.Assert(err, qt.IsNil)

			cloud := dbmodel.Cloud{
				Name: "test-cloud",
				Type: "test-provider",
				Regions: []dbmodel.CloudRegion{{
					Name: "test-region-1",
				}},
				Users: []dbmodel.UserCloudAccess{{
					Username: u.Username,
					Access:   "admin",
				}},
			}
			c.Assert(j.Database.DB.Create(&cloud).Error, qt.IsNil)

			err = alice.SetCloudAccess(context.Background(), cloud.ResourceTag(), ofganames.AdministratorRelation)
			c.Assert(err, qt.IsNil)

			controller1 := dbmodel.Controller{
				Name:        "test-controller-1",
				UUID:        "00000000-0000-0000-0000-0000-0000000000001",
				CloudName:   "test-cloud",
				CloudRegion: "test-region-1",
				CloudRegions: []dbmodel.CloudRegionControllerPriority{{
					Priority:      0,
					CloudRegionID: cloud.Regions[0].ID,
				}},
			}
			err = j.Database.AddController(context.Background(), &controller1)
			c.Assert(err, qt.Equals, nil)

			controller2 := dbmodel.Controller{
				Name:        "test-controller-2",
				UUID:        "00000000-0000-0000-0000-0000-0000000000002",
				CloudName:   "test-cloud",
				CloudRegion: "test-region-1",
				CloudRegions: []dbmodel.CloudRegionControllerPriority{{
					// controller2 has a higher priority and the model
					// should be created on this controller
					Priority:      2,
					CloudRegionID: cloud.Regions[0].ID,
				}},
			}
			err = j.Database.AddController(context.Background(), &controller2)
			c.Assert(err, qt.Equals, nil)

			cred := dbmodel.CloudCredential{
				Name:          "test-credential-1",
				CloudName:     cloud.Name,
				OwnerUsername: u.Username,
				AuthType:      "empty",
			}
			err = j.Database.SetCloudCredential(context.Background(), &cred)
			c.Assert(err, qt.Equals, nil)

			cred.Cloud = dbmodel.Cloud{
				Name: "test-cloud",
				Type: "test-provider",
			}

			tag := names.NewCloudCredentialTag("test-cloud/alice@external/test-credential-1")
			return &u, tag, ""
		},
	}, {
		about: "credential still used by a model",
		createEnv: func(c *qt.C, j *jimm.JIMM, client *openfga.OFGAClient) (*dbmodel.User, names.CloudCredentialTag, string) {
			u := dbmodel.User{
				Username: "alice@external",
			}
			c.Assert(j.Database.DB.Create(&u).Error, qt.IsNil)

			alice := openfga.NewUser(&u, client)

			err := alice.SetControllerAccess(context.Background(), j.ResourceTag(), ofganames.AdministratorRelation)
			c.Assert(err, qt.IsNil)

			cloud := dbmodel.Cloud{
				Name: "test-cloud",
				Type: "test-provider",
				Regions: []dbmodel.CloudRegion{{
					Name: "test-region-1",
				}},
				Users: []dbmodel.UserCloudAccess{{
					Username: u.Username,
					Access:   "admin",
				}},
			}
			c.Assert(j.Database.DB.Create(&cloud).Error, qt.IsNil)

			err = alice.SetCloudAccess(context.Background(), cloud.ResourceTag(), ofganames.AdministratorRelation)
			c.Assert(err, qt.IsNil)

			controller1 := dbmodel.Controller{
				Name:        "test-controller-1",
				UUID:        "00000000-0000-0000-0000-0000-0000000000001",
				CloudName:   "test-cloud",
				CloudRegion: "test-region-1",
				CloudRegions: []dbmodel.CloudRegionControllerPriority{{
					Priority:      0,
					CloudRegionID: cloud.Regions[0].ID,
				}},
			}
			err = j.Database.AddController(context.Background(), &controller1)
			c.Assert(err, qt.Equals, nil)

			controller2 := dbmodel.Controller{
				Name:        "test-controller-2",
				UUID:        "00000000-0000-0000-0000-0000-0000000000002",
				CloudName:   "test-cloud",
				CloudRegion: "test-region-1",
				CloudRegions: []dbmodel.CloudRegionControllerPriority{{
					// controller2 has a higher priority and the model
					// should be created on this controller
					Priority:      2,
					CloudRegionID: cloud.Regions[0].ID,
				}},
			}
			err = j.Database.AddController(context.Background(), &controller2)
			c.Assert(err, qt.Equals, nil)

			cred := dbmodel.CloudCredential{
				Name:          "test-credential-1",
				CloudName:     cloud.Name,
				OwnerUsername: u.Username,
				AuthType:      "empty",
			}
			err = j.Database.SetCloudCredential(context.Background(), &cred)
			c.Assert(err, qt.Equals, nil)

			_, err = j.AddModel(context.Background(), alice, &jimm.ModelCreateArgs{
				Name:            "test-model",
				Owner:           names.NewUserTag(u.Username),
				Cloud:           names.NewCloudTag(cloud.Name),
				CloudRegion:     "test-region-1",
				CloudCredential: names.NewCloudCredentialTag("test-cloud/alice@external/test-credential-1"),
			})
			c.Assert(err, qt.Equals, nil)

			tag := names.NewCloudCredentialTag("test-cloud/alice@external/test-credential-1")

			return &u, tag, `cloud credential still used by 1 model\(s\)`
		},
	}, {
		about: "user not owner of credentials - unauthorizer error",
		createEnv: func(c *qt.C, j *jimm.JIMM, client *openfga.OFGAClient) (*dbmodel.User, names.CloudCredentialTag, string) {
			u := dbmodel.User{
				Username: "alice@external",
			}
			c.Assert(j.Database.DB.Create(&u).Error, qt.IsNil)

			alice := openfga.NewUser(&u, client)

			err := alice.SetControllerAccess(context.Background(), j.ResourceTag(), ofganames.AdministratorRelation)
			c.Assert(err, qt.IsNil)

			tag := names.NewCloudCredentialTag("test-cloud/eve@external/test-credential-1")

			return &u, tag, "unauthorized"
		},
	}, {
		about:                  "error revoking credential on controller",
		revokeCredentialErrors: []error{errors.E("test error")},
		createEnv: func(c *qt.C, j *jimm.JIMM, client *openfga.OFGAClient) (*dbmodel.User, names.CloudCredentialTag, string) {
			u := dbmodel.User{
				Username: "alice@external",
			}
			c.Assert(j.Database.DB.Create(&u).Error, qt.IsNil)

			alice := openfga.NewUser(&u, client)

			err := alice.SetControllerAccess(context.Background(), j.ResourceTag(), ofganames.AdministratorRelation)
			c.Assert(err, qt.IsNil)

			cloud := dbmodel.Cloud{
				Name: "test-cloud",
				Type: "test-provider",
				Regions: []dbmodel.CloudRegion{{
					Name: "test-region-1",
				}},
				Users: []dbmodel.UserCloudAccess{{
					Username: u.Username,
					Access:   "admin",
				}},
			}
			c.Assert(j.Database.DB.Create(&cloud).Error, qt.IsNil)

			err = alice.SetCloudAccess(context.Background(), cloud.ResourceTag(), ofganames.AdministratorRelation)
			c.Assert(err, qt.IsNil)

			controller1 := dbmodel.Controller{
				Name:        "test-controller-1",
				UUID:        "00000000-0000-0000-0000-0000-0000000000001",
				CloudName:   "test-cloud",
				CloudRegion: "test-region-1",
				CloudRegions: []dbmodel.CloudRegionControllerPriority{{
					Priority:      0,
					CloudRegionID: cloud.Regions[0].ID,
				}},
			}
			err = j.Database.AddController(context.Background(), &controller1)
			c.Assert(err, qt.Equals, nil)

			controller2 := dbmodel.Controller{
				Name:        "test-controller-2",
				UUID:        "00000000-0000-0000-0000-0000-0000000000002",
				CloudName:   "test-cloud",
				CloudRegion: "test-region-1",
				CloudRegions: []dbmodel.CloudRegionControllerPriority{{
					// controller2 has a higher priority and the model
					// should be created on this controller
					Priority:      2,
					CloudRegionID: cloud.Regions[0].ID,
				}},
			}
			err = j.Database.AddController(context.Background(), &controller2)
			c.Assert(err, qt.Equals, nil)

			cred := dbmodel.CloudCredential{
				Name:          "test-credential-1",
				CloudName:     cloud.Name,
				OwnerUsername: u.Username,
				AuthType:      "empty",
			}
			err = j.Database.SetCloudCredential(context.Background(), &cred)
			c.Assert(err, qt.Equals, nil)

			tag := names.NewCloudCredentialTag("test-cloud/alice@external/test-credential-1")

			return &u, tag, "test error"
		},
	}}
	for _, test := range tests {
		c.Run(test.about, func(c *qt.C) {
			var mu sync.Mutex
			revokeErrors := test.revokeCredentialErrors
			api := &jimmtest.API{
				RevokeCredential_: func(context.Context, names.CloudCredentialTag) error {
					mu.Lock()
					defer mu.Unlock()
					if len(revokeErrors) > 0 {
						var err error
						err, revokeErrors = revokeErrors[0], revokeErrors[1:]
						return err
					}
					return nil
				},
				UpdateCredential_: func(context.Context, jujuparams.TaggedCredential) ([]jujuparams.UpdateCredentialModelResult, error) {
					return []jujuparams.UpdateCredentialModelResult{{
						ModelUUID: "00000001-0000-0000-0000-0000-000000000001",
						ModelName: "test-model",
					}}, nil
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

			client, _, _, err := jimmtest.SetupTestOFGAClient(c.Name(), test.about)
			c.Assert(err, qt.IsNil)

			j := &jimm.JIMM{
				UUID: uuid.NewString(),
				Database: db.Database{
					DB: jimmtest.MemoryDB(c, func() time.Time { return now }),
				},
				Dialer: &jimmtest.Dialer{
					API: api,
				},
				OpenFGAClient: client,
			}

			ctx := context.Background()
			err = j.Database.Migrate(ctx, false)
			c.Assert(err, qt.IsNil)

			user, tag, expectedError := test.createEnv(c, j, client)

			err = j.RevokeCloudCredential(ctx, user, tag, false)
			if expectedError == "" {
				c.Assert(err, qt.Equals, nil)

				var credential dbmodel.CloudCredential
				credential.SetTag(tag)
				err = j.Database.GetCloudCredential(ctx, &credential)
				c.Assert(errors.ErrorCode(err), qt.Equals, errors.CodeNotFound)
			} else {
				c.Assert(err, qt.ErrorMatches, expectedError)
			}
		})
	}
}

func TestGetCloudCredential(t *testing.T) {
	c := qt.New(t)

	now := time.Now().UTC().Round(time.Millisecond)

	tests := []struct {
		about                  string
		revokeCredentialErrors []error
		createEnv              func(*qt.C, *jimm.JIMM, *openfga.OFGAClient) (*dbmodel.User, names.CloudCredentialTag, dbmodel.CloudCredential, string)
	}{{
		about: "all ok",
		createEnv: func(c *qt.C, j *jimm.JIMM, client *openfga.OFGAClient) (*dbmodel.User, names.CloudCredentialTag, dbmodel.CloudCredential, string) {
			u := dbmodel.User{
				Username: "alice@external",
			}
			c.Assert(j.Database.DB.Create(&u).Error, qt.IsNil)

			alice := openfga.NewUser(&u, client)

			err := alice.SetControllerAccess(context.Background(), j.ResourceTag(), ofganames.AdministratorRelation)
			c.Assert(err, qt.IsNil)

			cloud := dbmodel.Cloud{
				Name: "test-cloud",
				Type: "test-provider",
				Regions: []dbmodel.CloudRegion{{
					Name: "test-region-1",
				}},
				Users: []dbmodel.UserCloudAccess{{
					Username: u.Username,
				}},
			}
			c.Assert(j.Database.DB.Create(&cloud).Error, qt.IsNil)

			err = alice.SetCloudAccess(context.Background(), cloud.ResourceTag(), ofganames.AdministratorRelation)
			c.Assert(err, qt.IsNil)

			controller1 := dbmodel.Controller{
				Name:        "test-controller-1",
				UUID:        "00000000-0000-0000-0000-0000-0000000000001",
				CloudName:   "test-cloud",
				CloudRegion: "test-region-1",
				CloudRegions: []dbmodel.CloudRegionControllerPriority{{
					Priority:      0,
					CloudRegionID: cloud.Regions[0].ID,
				}},
			}
			err = j.Database.AddController(context.Background(), &controller1)
			c.Assert(err, qt.Equals, nil)

			controller2 := dbmodel.Controller{
				Name:        "test-controller-2",
				UUID:        "00000000-0000-0000-0000-0000-0000000000002",
				CloudName:   "test-cloud",
				CloudRegion: "test-region-1",
				CloudRegions: []dbmodel.CloudRegionControllerPriority{{
					// controller2 has a higher priority and the model
					// should be created on this controller
					Priority:      2,
					CloudRegionID: cloud.Regions[0].ID,
				}},
			}
			err = j.Database.AddController(context.Background(), &controller2)
			c.Assert(err, qt.Equals, nil)

			cred := dbmodel.CloudCredential{
				Name:      "test-credential-1",
				CloudName: cloud.Name,
				Cloud: dbmodel.Cloud{
					Name: cloud.Name,
					Type: cloud.Type,
				},
				OwnerUsername: u.Username,
				AuthType:      "empty",
			}
			err = j.Database.SetCloudCredential(context.Background(), &cred)
			c.Assert(err, qt.Equals, nil)

			tag := names.NewCloudCredentialTag("test-cloud/alice@external/test-credential-1")

			return &u, tag, cred, ""
		},
	}, {
		about: "credential not found",
		createEnv: func(c *qt.C, j *jimm.JIMM, client *openfga.OFGAClient) (*dbmodel.User, names.CloudCredentialTag, dbmodel.CloudCredential, string) {
			u := dbmodel.User{
				Username: "alice@external",
			}
			c.Assert(j.Database.DB.Create(&u).Error, qt.IsNil)

			alice := openfga.NewUser(&u, client)

			err := alice.SetControllerAccess(context.Background(), j.ResourceTag(), ofganames.AdministratorRelation)
			c.Assert(err, qt.IsNil)

			tag := names.NewCloudCredentialTag("test-cloud/alice@external/test-credential-1")

			return &u, tag, dbmodel.CloudCredential{}, `cloudcredential "test-cloud/alice@external/test-credential-1" not found`
		},
	}}
	for _, test := range tests {
		c.Run(test.about, func(c *qt.C) {
			client, _, _, err := jimmtest.SetupTestOFGAClient(c.Name(), test.about)
			c.Assert(err, qt.IsNil)

			j := &jimm.JIMM{
				UUID: uuid.NewString(),
				Database: db.Database{
					DB: jimmtest.MemoryDB(c, func() time.Time { return now }),
				},
				OpenFGAClient: client,
			}

			ctx := context.Background()
			err = j.Database.Migrate(ctx, false)
			c.Assert(err, qt.IsNil)

			u, tag, expectedCredential, expectedError := test.createEnv(c, j, client)
			user := openfga.NewUser(u, client)
			credential, err := j.GetCloudCredential(ctx, user, tag)
			if expectedError == "" {
				c.Assert(err, qt.Equals, nil)
				c.Assert(credential, jimmtest.DBObjectEquals, &expectedCredential)
			} else {
				c.Assert(err, qt.ErrorMatches, expectedError)
			}
		})
	}
}

const forEachUserCloudCredentialEnv = `clouds:
- name: cloud-1
  regions:
  - name: default
- name: cloud-2
  regions:
  - name: default
cloud-credentials:
- name: cred-1
  cloud: cloud-1
  owner: alice@external
  attributes:
    k1: v1
    k2: v2
- name: cred-2
  cloud: cloud-1
  owner: bob@external
  attributes:
    k1: v1
    k2: v2
- name: cred-3
  cloud: cloud-2
  owner: alice@external
- name: cred-4
  cloud: cloud-2
  owner: bob@external
- name: cred-5
  cloud: cloud-1
  owner: alice@external
users:
- username: alice@external
  controller-access: superuser
- username: bob@external
`

var forEachUserCloudCredentialTests = []struct {
	name              string
	env               string
	username          string
	cloudTag          names.CloudTag
	f                 func(cred *dbmodel.CloudCredential) error
	expectCredentials []string
	expectError       string
	expectErrorCode   errors.Code
}{{
	name:     "UserCredentialsWithCloud",
	env:      forEachUserCloudCredentialEnv,
	username: "alice@external",
	cloudTag: names.NewCloudTag("cloud-1"),
	expectCredentials: []string{
		names.NewCloudCredentialTag("cloud-1/alice@external/cred-1").String(),
		names.NewCloudCredentialTag("cloud-1/alice@external/cred-5").String(),
	},
}, {
	name:     "UserCredentialsWithoutCloud",
	env:      forEachUserCloudCredentialEnv,
	username: "bob@external",
	expectCredentials: []string{
		names.NewCloudCredentialTag("cloud-1/bob@external/cred-2").String(),
		names.NewCloudCredentialTag("cloud-2/bob@external/cred-4").String(),
	},
}, {
	name:     "IterationError",
	env:      forEachUserCloudCredentialEnv,
	username: "alice@external",
	f: func(*dbmodel.CloudCredential) error {
		return errors.E("test error", errors.Code("test code"))
	},
	expectError:     "test error",
	expectErrorCode: "test code",
}}

func TestForEachUserCloudCredential(t *testing.T) {
	c := qt.New(t)

	for _, test := range forEachUserCloudCredentialTests {
		c.Run(test.name, func(c *qt.C) {
			ctx := context.Background()

			client, _, _, err := jimmtest.SetupTestOFGAClient(c.Name(), test.name)
			c.Assert(err, qt.IsNil)

			env := jimmtest.ParseEnvironment(c, test.env)
			j := &jimm.JIMM{
				UUID: uuid.NewString(),
				Database: db.Database{
					DB: jimmtest.MemoryDB(c, nil),
				},
				Dialer: &jimmtest.Dialer{
					API: &jimmtest.API{},
				},
				OpenFGAClient: client,
			}
			err = j.Database.Migrate(ctx, false)
			c.Assert(err, qt.IsNil)
			env.PopulateDB(c, j.Database, client)
			u := env.User(test.username).DBObject(c, j.Database, client)

			var credentials []string
			if test.f == nil {
				test.f = func(cred *dbmodel.CloudCredential) error {
					credentials = append(credentials, cred.Tag().String())
					if cred.Attributes != nil {
						return errors.E("credential contains attributes")
					}
					return nil
				}
			}
			err = j.ForEachUserCloudCredential(ctx, &u, test.cloudTag, test.f)
			if test.expectError != "" {
				c.Check(err, qt.ErrorMatches, test.expectError)
				if test.expectErrorCode != "" {
					c.Check(errors.ErrorCode(err), qt.Equals, test.expectErrorCode)
				}
				return
			}
			c.Assert(err, qt.IsNil)
			c.Check(credentials, qt.DeepEquals, test.expectCredentials)
		})
	}
}

const getCloudCredentialAttributesEnv = `clouds:
- name: test-cloud
  type: gce
  regions:
  - name: default
cloud-credentials:
- name: cred-1
  cloud: test-cloud
  owner: bob@external
  auth-type: oauth2
  attributes:
    client-email: bob@example.com
    client-id: 1234
    private-key: super-secret
    project-id: 5678
users:
- username: alice@external
  controller-access: superuser
- username: bob@external
`

var getCloudCredentialAttributesTests = []struct {
	name             string
	username         string
	hidden           bool
	expectAttributes map[string]string
	expectRedacted   []string
	expectError      string
	expectErrorCode  errors.Code
}{{
	name:     "OwnerNoHidden",
	username: "bob@external",
	expectAttributes: map[string]string{
		"client-email": "bob@example.com",
		"client-id":    "1234",
		"project-id":   "5678",
	},
	expectRedacted: []string{"private-key"},
}, {
	name:     "OwnerWithHidden",
	username: "bob@external",
	hidden:   true,
	expectAttributes: map[string]string{
		"client-email": "bob@example.com",
		"client-id":    "1234",
		"private-key":  "super-secret",
		"project-id":   "5678",
	},
}, {
	name:     "SuperUserNoHidden",
	username: "alice@external",
	expectAttributes: map[string]string{
		"client-email": "bob@example.com",
		"client-id":    "1234",
		"project-id":   "5678",
	},
	expectRedacted: []string{"private-key"},
}, {
	name:            "SuperUserWithHiddenUnauthorized",
	username:        "alice@external",
	hidden:          true,
	expectError:     `unauthorized`,
	expectErrorCode: errors.CodeUnauthorized,
}, {
	name:            "OtherUserUnauthorized",
	username:        "charlie@external",
	expectError:     `unauthorized`,
	expectErrorCode: errors.CodeUnauthorized,
}}

func TestGetCloudCredentialAttributes(t *testing.T) {
	c := qt.New(t)

	for _, test := range getCloudCredentialAttributesTests {
		c.Run(test.name, func(c *qt.C) {
			ctx := context.Background()

			client, _, _, err := jimmtest.SetupTestOFGAClient(c.Name(), test.name)
			c.Assert(err, qt.IsNil)

			env := jimmtest.ParseEnvironment(c, getCloudCredentialAttributesEnv)
			j := &jimm.JIMM{
				UUID: uuid.NewString(),
				Database: db.Database{
					DB: jimmtest.MemoryDB(c, nil),
				},
				Dialer: &jimmtest.Dialer{
					API: &jimmtest.API{},
				},
				OpenFGAClient: client,
			}
			err = j.Database.Migrate(ctx, false)
			c.Assert(err, qt.IsNil)
			env.PopulateDB(c, j.Database, client)
			u := env.User("bob@external").DBObject(c, j.Database, client)
			userBob := openfga.NewUser(&u, client)
			cred, err := j.GetCloudCredential(ctx, userBob, names.NewCloudCredentialTag("test-cloud/bob@external/cred-1"))
			c.Assert(err, qt.IsNil)

			u = env.User(test.username).DBObject(c, j.Database, client)
			userTest := openfga.NewUser(&u, client)
			attr, redacted, err := j.GetCloudCredentialAttributes(ctx, userTest, cred, test.hidden)
			if test.expectError != "" {
				c.Check(err, qt.ErrorMatches, test.expectError)
				if test.expectErrorCode != "" {
					c.Check(errors.ErrorCode(err), qt.Equals, test.expectErrorCode)
				}
				return
			}
			c.Assert(err, qt.IsNil)
			c.Check(attr, qt.DeepEquals, test.expectAttributes)
			c.Check(redacted, qt.DeepEquals, test.expectRedacted)
		})
	}
}

func TestCloudCredentialAttributeStore(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	client, _, _, err := jimmtest.SetupTestOFGAClient(c.Name())
	c.Assert(err, qt.IsNil)

	attrStore := testCloudCredentialAttributeStore{
		attrs: make(map[string]map[string]string),
	}
	j := &jimm.JIMM{
		UUID: uuid.NewString(),
		Database: db.Database{
			DB: jimmtest.MemoryDB(c, nil),
		},
		Dialer: &jimmtest.Dialer{
			API: &jimmtest.API{},
		},
		CredentialStore: attrStore,
	}
	err = j.Database.Migrate(ctx, false)
	c.Assert(err, qt.IsNil)

	env := jimmtest.ParseEnvironment(c, `clouds:
- name: test
  type: test-provider
  regions:
  - name: test-region
`)
	env.PopulateDB(c, j.Database, client)

	u := env.User("alice@external").DBObject(c, j.Database, client)
	user := openfga.NewUser(&u, client)
	args := jimm.UpdateCloudCredentialArgs{
		CredentialTag: names.NewCloudCredentialTag("test/alice@external/cred-1"),
		Credential: jujuparams.CloudCredential{
			AuthType: "userpass",
			Attributes: map[string]string{
				"username": "test-user",
				"password": "test-password",
			},
		},
	}
	_, err = j.UpdateCloudCredential(ctx, user, args)
	c.Assert(err, qt.IsNil)

	cred := dbmodel.CloudCredential{
		OwnerUsername: "alice@external",
		Name:          "cred-1",
		CloudName:     "test",
	}
	err = j.Database.GetCloudCredential(ctx, &cred)
	c.Assert(err, qt.IsNil)
	c.Check(cred, jimmtest.DBObjectEquals, dbmodel.CloudCredential{
		OwnerUsername: "alice@external",
		Name:          "cred-1",
		CloudName:     "test",
		AuthType:      "userpass",
		Cloud: dbmodel.Cloud{
			Name: "test",
			Type: "test-provider",
		},
		AttributesInVault: true,
	})
	attr, _, err := j.GetCloudCredentialAttributes(ctx, user, &cred, true)
	c.Assert(err, qt.IsNil)
	c.Check(attr, qt.DeepEquals, args.Credential.Attributes)

	// Update to an "empty" credential
	args.Credential.AuthType = "empty"
	args.Credential.Attributes = nil
	_, err = j.UpdateCloudCredential(ctx, user, args)
	c.Assert(err, qt.IsNil)

	cred.AuthType = args.Credential.AuthType
	attr, _, err = j.GetCloudCredentialAttributes(ctx, user, &cred, true)
	c.Assert(err, qt.IsNil)
	c.Check(attr, qt.DeepEquals, args.Credential.Attributes)
}

type testCloudCredentialAttributeStore struct {
	attrs map[string]map[string]string
}

func (s testCloudCredentialAttributeStore) Get(_ context.Context, tag names.CloudCredentialTag) (map[string]string, error) {
	return s.attrs[tag.String()], nil
}

func (s testCloudCredentialAttributeStore) Put(_ context.Context, tag names.CloudCredentialTag, attr map[string]string) error {
	s.attrs[tag.String()] = attr
	return nil
}

func (s testCloudCredentialAttributeStore) GetControllerCredentials(ctx context.Context, controllerName string) (string, string, error) {
	return "", "", errors.E(errors.CodeNotImplemented)
}

func (s testCloudCredentialAttributeStore) PutControllerCredentials(ctx context.Context, controllerName string, username string, password string) error {
	return errors.E(errors.CodeNotImplemented)
}

func (s testCloudCredentialAttributeStore) GetJWKS(ctx context.Context) (jwk.Set, error) {
	return nil, errors.E(errors.CodeNotImplemented)
}

func (s testCloudCredentialAttributeStore) GetJWKSPrivateKey(ctx context.Context) ([]byte, error) {
	return nil, errors.E(errors.CodeNotImplemented)
}

func (s testCloudCredentialAttributeStore) GetJWKSExpiry(ctx context.Context) (time.Time, error) {
	return time.Now(), errors.E(errors.CodeNotImplemented)
}

func (s testCloudCredentialAttributeStore) PutJWKS(ctx context.Context, jwks jwk.Set) error {
	return errors.E(errors.CodeNotImplemented)
}
func (s testCloudCredentialAttributeStore) PutJWKSPrivateKey(ctx context.Context, pem []byte) error {
	return errors.E(errors.CodeNotImplemented)
}

func (s testCloudCredentialAttributeStore) PutJWKSExpiry(ctx context.Context, expiry time.Time) error {
	return errors.E(errors.CodeNotImplemented)
}

func (s testCloudCredentialAttributeStore) CleanupJWKS(ctx context.Context) error {
	return errors.E(errors.CodeNotImplemented)
}
