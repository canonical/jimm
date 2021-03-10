// Copyright 2020 Canonical Ltd.

package jimm_test

import (
	"context"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
	jujuparams "github.com/juju/juju/apiserver/params"
	"github.com/juju/names/v4"
	"gorm.io/gorm"

	"github.com/CanonicalLtd/jimm/internal/db"
	"github.com/CanonicalLtd/jimm/internal/dbmodel"
	"github.com/CanonicalLtd/jimm/internal/errors"
	"github.com/CanonicalLtd/jimm/internal/jimm"
	"github.com/CanonicalLtd/jimm/internal/jimmtest"
)

func TestGetCloud(t *testing.T) {
	c := qt.New(t)

	ctx := context.Background()
	now := time.Now().UTC().Round(time.Millisecond)
	j := &jimm.JIMM{
		Database: db.Database{
			DB: jimmtest.MemoryDB(c, func() time.Time { return now }),
		},
	}

	err := j.Database.Migrate(ctx, false)
	c.Assert(err, qt.IsNil)

	err = j.Database.AddCloud(ctx, &dbmodel.Cloud{
		Name: "test-cloud-1",
		Users: []dbmodel.UserCloudAccess{{
			User: dbmodel.User{
				Username: "alice@external",
			},
			Access: "admin",
		}, {
			User: dbmodel.User{
				Username: "bob@external",
			},
			Access: "add-model",
		}},
	})
	c.Assert(err, qt.IsNil)

	err = j.Database.AddCloud(ctx, &dbmodel.Cloud{
		Name: "test-cloud-2",
		Users: []dbmodel.UserCloudAccess{{
			User: dbmodel.User{
				Username: "everyone@external",
			},
			Access: "add-model",
		}},
	})
	c.Assert(err, qt.IsNil)

	alice := &dbmodel.User{Username: "alice@external"}
	bob := &dbmodel.User{Username: "bob@external"}
	charlie := &dbmodel.User{Username: "charlie@external"}
	daphne := &dbmodel.User{
		Username:         "daphne@external",
		ControllerAccess: "superuser",
	}

	_, err = j.GetCloud(ctx, alice, names.NewCloudTag("test-cloud-0"))
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeNotFound)

	_, err = j.GetCloud(ctx, charlie, names.NewCloudTag("test-cloud-1"))
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeUnauthorized)

	cld, err := j.GetCloud(ctx, alice, names.NewCloudTag("test-cloud-1"))
	c.Assert(err, qt.IsNil)
	c.Check(cld, qt.DeepEquals, dbmodel.Cloud{
		ID:        1,
		CreatedAt: now,
		UpdatedAt: now,
		Name:      "test-cloud-1",
		Regions:   []dbmodel.CloudRegion{},
		Users: []dbmodel.UserCloudAccess{{
			Model: gorm.Model{
				ID:        1,
				CreatedAt: now,
				UpdatedAt: now,
			},
			Username: "alice@external",
			User: dbmodel.User{
				Model: gorm.Model{
					ID:        1,
					CreatedAt: now,
					UpdatedAt: now,
				},
				Username:         "alice@external",
				ControllerAccess: "add-model",
			},
			CloudName: "test-cloud-1",
			Access:    "admin",
		}, {
			Model: gorm.Model{
				ID:        2,
				CreatedAt: now,
				UpdatedAt: now,
			},
			Username: "bob@external",
			User: dbmodel.User{
				Model: gorm.Model{
					ID:        2,
					CreatedAt: now,
					UpdatedAt: now,
				},
				Username:         "bob@external",
				ControllerAccess: "add-model",
			},
			CloudName: "test-cloud-1",
			Access:    "add-model",
		}},
	})

	cld, err = j.GetCloud(ctx, bob, names.NewCloudTag("test-cloud-1"))
	c.Assert(err, qt.IsNil)
	c.Check(cld, qt.DeepEquals, dbmodel.Cloud{
		ID:        1,
		CreatedAt: now,
		UpdatedAt: now,
		Name:      "test-cloud-1",
		Regions:   []dbmodel.CloudRegion{},
		Users: []dbmodel.UserCloudAccess{{
			Username: "bob@external",
			User:     *bob,
			Access:   "add-model",
		}},
	})

	cld, err = j.GetCloud(ctx, daphne, names.NewCloudTag("test-cloud-1"))
	c.Assert(err, qt.IsNil)
	c.Check(cld, qt.DeepEquals, dbmodel.Cloud{
		ID:        1,
		CreatedAt: now,
		UpdatedAt: now,
		Name:      "test-cloud-1",
		Regions:   []dbmodel.CloudRegion{},
		Users: []dbmodel.UserCloudAccess{{
			Model: gorm.Model{
				ID:        1,
				CreatedAt: now,
				UpdatedAt: now,
			},
			Username: "alice@external",
			User: dbmodel.User{
				Model: gorm.Model{
					ID:        1,
					CreatedAt: now,
					UpdatedAt: now,
				},
				Username:         "alice@external",
				ControllerAccess: "add-model",
			},
			CloudName: "test-cloud-1",
			Access:    "admin",
		}, {
			Model: gorm.Model{
				ID:        2,
				CreatedAt: now,
				UpdatedAt: now,
			},
			Username: "bob@external",
			User: dbmodel.User{
				Model: gorm.Model{
					ID:        2,
					CreatedAt: now,
					UpdatedAt: now,
				},
				Username:         "bob@external",
				ControllerAccess: "add-model",
			},
			CloudName: "test-cloud-1",
			Access:    "add-model",
		}},
	})

	cld, err = j.GetCloud(ctx, charlie, names.NewCloudTag("test-cloud-2"))
	c.Check(cld, qt.DeepEquals, dbmodel.Cloud{
		ID:        2,
		CreatedAt: now,
		UpdatedAt: now,
		Name:      "test-cloud-2",
		Regions:   []dbmodel.CloudRegion{},
		Users: []dbmodel.UserCloudAccess{{
			Username: "charlie@external",
			User: dbmodel.User{
				Username: "charlie@external",
			},
			Access: "add-model",
		}},
	})
}

func TestForEachCloud(t *testing.T) {
	c := qt.New(t)

	ctx := context.Background()
	now := time.Now().UTC().Round(time.Millisecond)
	j := &jimm.JIMM{
		Database: db.Database{
			DB: jimmtest.MemoryDB(c, func() time.Time { return now }),
		},
	}

	err := j.Database.Migrate(ctx, false)
	c.Assert(err, qt.IsNil)

	err = j.Database.AddCloud(ctx, &dbmodel.Cloud{
		Name: "test-cloud-1",
		Users: []dbmodel.UserCloudAccess{{
			User: dbmodel.User{
				Username: "alice@external",
			},
			Access: "admin",
		}, {
			User: dbmodel.User{
				Username: "bob@external",
			},
			Access: "add-model",
		}},
	})
	c.Assert(err, qt.IsNil)

	err = j.Database.AddCloud(ctx, &dbmodel.Cloud{
		Name: "test-cloud-2",
		Users: []dbmodel.UserCloudAccess{{
			User: dbmodel.User{
				Username: "bob@external",
			},
			Access: "add-model",
		}, {
			User: dbmodel.User{
				Username: "everyone@external",
			},
			Access: "add-model",
		}},
	})
	c.Assert(err, qt.IsNil)

	err = j.Database.AddCloud(ctx, &dbmodel.Cloud{
		Name: "test-cloud-3",
		Users: []dbmodel.UserCloudAccess{{
			User: dbmodel.User{
				Username: "everyone@external",
			},
			Access: "add-model",
		}},
	})
	c.Assert(err, qt.IsNil)

	alice := &dbmodel.User{Username: "alice@external"}
	bob := &dbmodel.User{Username: "bob@external"}
	charlie := &dbmodel.User{Username: "charlie@external"}
	daphne := &dbmodel.User{
		Username:         "daphne@external",
		ControllerAccess: "superuser",
	}

	var clds []dbmodel.Cloud
	err = j.ForEachUserCloud(ctx, alice, func(cld *dbmodel.Cloud) error {
		clds = append(clds, *cld)
		return nil
	})
	c.Assert(err, qt.IsNil)
	c.Check(clds, qt.DeepEquals, []dbmodel.Cloud{{
		ID:        1,
		CreatedAt: now,
		UpdatedAt: now,
		Name:      "test-cloud-1",
		Regions:   []dbmodel.CloudRegion{},
		Users: []dbmodel.UserCloudAccess{{
			Model: gorm.Model{
				ID:        1,
				CreatedAt: now,
				UpdatedAt: now,
			},
			Username: "alice@external",
			User: dbmodel.User{
				Model: gorm.Model{
					ID:        1,
					CreatedAt: now,
					UpdatedAt: now,
				},
				Username:         "alice@external",
				ControllerAccess: "add-model",
			},
			CloudName: "test-cloud-1",
			Access:    "admin",
		}, {
			Model: gorm.Model{
				ID:        2,
				CreatedAt: now,
				UpdatedAt: now,
			},
			Username: "bob@external",
			User: dbmodel.User{
				Model: gorm.Model{
					ID:        2,
					CreatedAt: now,
					UpdatedAt: now,
				},
				Username:         "bob@external",
				ControllerAccess: "add-model",
			},
			CloudName: "test-cloud-1",
			Access:    "add-model",
		}},
	}, {
		ID:        2,
		CreatedAt: now,
		UpdatedAt: now,
		Name:      "test-cloud-2",
		Regions:   []dbmodel.CloudRegion{},
		Users: []dbmodel.UserCloudAccess{{
			Username: "alice@external",
			User:     *alice,
			Access:   "add-model",
		}},
	}, {
		ID:        3,
		CreatedAt: now,
		UpdatedAt: now,
		Name:      "test-cloud-3",
		Regions:   []dbmodel.CloudRegion{},
		Users: []dbmodel.UserCloudAccess{{
			Username: "alice@external",
			User:     *alice,
			Access:   "add-model",
		}},
	}})

	clds = clds[:0]
	err = j.ForEachUserCloud(ctx, bob, func(cld *dbmodel.Cloud) error {
		clds = append(clds, *cld)
		return nil
	})
	c.Assert(err, qt.IsNil)
	c.Check(clds, qt.DeepEquals, []dbmodel.Cloud{{
		ID:        1,
		CreatedAt: now,
		UpdatedAt: now,
		Name:      "test-cloud-1",
		Regions:   []dbmodel.CloudRegion{},
		Users: []dbmodel.UserCloudAccess{{
			Username: "bob@external",
			User:     *bob,
			Access:   "add-model",
		}},
	}, {
		ID:        2,
		CreatedAt: now,
		UpdatedAt: now,
		Name:      "test-cloud-2",
		Regions:   []dbmodel.CloudRegion{},
		Users: []dbmodel.UserCloudAccess{{
			Username: "bob@external",
			User:     *bob,
			Access:   "add-model",
		}},
	}, {
		ID:        3,
		CreatedAt: now,
		UpdatedAt: now,
		Name:      "test-cloud-3",
		Regions:   []dbmodel.CloudRegion{},
		Users: []dbmodel.UserCloudAccess{{
			Username: "bob@external",
			User:     *bob,
			Access:   "add-model",
		}},
	}})

	clds = clds[:0]
	err = j.ForEachUserCloud(ctx, charlie, func(cld *dbmodel.Cloud) error {
		clds = append(clds, *cld)
		return nil
	})
	c.Assert(err, qt.IsNil)
	c.Check(clds, qt.DeepEquals, []dbmodel.Cloud{{
		ID:        2,
		CreatedAt: now,
		UpdatedAt: now,
		Name:      "test-cloud-2",
		Regions:   []dbmodel.CloudRegion{},
		Users: []dbmodel.UserCloudAccess{{
			Username: "charlie@external",
			User:     *charlie,
			Access:   "add-model",
		}},
	}, {
		ID:        3,
		CreatedAt: now,
		UpdatedAt: now,
		Name:      "test-cloud-3",
		Regions:   []dbmodel.CloudRegion{},
		Users: []dbmodel.UserCloudAccess{{
			Username: "charlie@external",
			User:     *charlie,
			Access:   "add-model",
		}},
	}})

	clds = clds[:0]
	err = j.ForEachCloud(ctx, daphne, func(cld *dbmodel.Cloud) error {
		clds = append(clds, *cld)
		return nil
	})
	c.Assert(err, qt.IsNil)
	c.Check(clds, qt.DeepEquals, []dbmodel.Cloud{{
		ID:        1,
		CreatedAt: now,
		UpdatedAt: now,
		Name:      "test-cloud-1",
		Regions:   []dbmodel.CloudRegion{},
		Users: []dbmodel.UserCloudAccess{{
			Model: gorm.Model{
				ID:        1,
				CreatedAt: now,
				UpdatedAt: now,
			},
			Username: "alice@external",
			User: dbmodel.User{
				Model: gorm.Model{
					ID:        1,
					CreatedAt: now,
					UpdatedAt: now,
				},
				Username:         "alice@external",
				ControllerAccess: "add-model",
			},
			CloudName: "test-cloud-1",
			Access:    "admin",
		}, {
			Model: gorm.Model{
				ID:        2,
				CreatedAt: now,
				UpdatedAt: now,
			},
			Username: "bob@external",
			User: dbmodel.User{
				Model: gorm.Model{
					ID:        2,
					CreatedAt: now,
					UpdatedAt: now,
				},
				Username:         "bob@external",
				ControllerAccess: "add-model",
			},
			CloudName: "test-cloud-1",
			Access:    "add-model",
		}},
	}, {
		ID:        2,
		CreatedAt: now,
		UpdatedAt: now,
		Name:      "test-cloud-2",
		Regions:   []dbmodel.CloudRegion{},
		Users: []dbmodel.UserCloudAccess{{
			Model: gorm.Model{
				ID:        3,
				CreatedAt: now,
				UpdatedAt: now,
			},
			Username: "bob@external",
			User: dbmodel.User{
				Model: gorm.Model{
					ID:        2,
					CreatedAt: now,
					UpdatedAt: now,
				},
				Username:         "bob@external",
				ControllerAccess: "add-model",
			},
			CloudName: "test-cloud-2",
			Access:    "add-model",
		}, {
			Model: gorm.Model{
				ID:        4,
				CreatedAt: now,
				UpdatedAt: now,
			},
			Username: "everyone@external",
			User: dbmodel.User{
				Model: gorm.Model{
					ID:        3,
					CreatedAt: now,
					UpdatedAt: now,
				},
				Username:         "everyone@external",
				ControllerAccess: "add-model",
			},
			CloudName: "test-cloud-2",
			Access:    "add-model",
		}},
	}, {
		ID:        3,
		CreatedAt: now,
		UpdatedAt: now,
		Name:      "test-cloud-3",
		Regions:   []dbmodel.CloudRegion{},
		Users: []dbmodel.UserCloudAccess{{
			Model: gorm.Model{
				ID:        5,
				CreatedAt: now,
				UpdatedAt: now,
			},
			Username: "everyone@external",
			User: dbmodel.User{
				Model: gorm.Model{
					ID:        3,
					CreatedAt: now,
					UpdatedAt: now,
				},
				Username:         "everyone@external",
				ControllerAccess: "add-model",
			},
			CloudName: "test-cloud-3",
			Access:    "add-model",
		}},
	}})
}

const addHostedCloudTestEnv = `clouds:
- name: test-cloud
  type: test-provider
  regions:
  - name: test-region
  users:
  - user: everyone@external
    access: add-model
- name: private-cloud
  type: test-provider2
  regions:
  - name: test-region
  users:
  - user: alice@external
    access: admin
- name: existing-cloud
  type: kubernetes
  host-cloud-region: test-provider/test-region
  regions:
  - name: default
  users:
  - user: alice@external
    access: admin
controllers:
- name: test-controller
  uuid: 00000001-0000-0000-0000-000000000001
  cloud-regions:
  - cloud: test-cloud
    region: test-region
    priority: 1
  - cloud: existing-cloud
    region: default
    priority: 1
users:
- username: alice@external
  controller-access: superuser
- username: bob@external
  controller-access: add-model
- username: charlie@external
  controller-access: login
`

var addHostedCloudTests = []struct {
	name             string
	dialError        error
	addCloud         func(context.Context, names.CloudTag, jujuparams.Cloud) error
	grantCloudAccess func(context.Context, names.CloudTag, names.UserTag, string) error
	cloud_           func(context.Context, names.CloudTag, *jujuparams.Cloud) error
	username         string
	cloudName        string
	cloud            jujuparams.Cloud
	expectCloud      dbmodel.Cloud
	expectError      string
	expectErrorCode  errors.Code
}{{
	name: "Success",
	addCloud: func(context.Context, names.CloudTag, jujuparams.Cloud) error {
		return nil
	},
	grantCloudAccess: func(context.Context, names.CloudTag, names.UserTag, string) error {
		return nil
	},
	cloud_: func(_ context.Context, _ names.CloudTag, cld *jujuparams.Cloud) error {
		cld.Type = "kubernetes"
		cld.HostCloudRegion = "test-provider/test-region"
		cld.AuthTypes = []string{"empty", "userpass"}
		cld.Endpoint = "https://example.com"
		cld.IdentityEndpoint = "https://example.com/identity"
		cld.StorageEndpoint = "https://example.com/storage"
		cld.Regions = []jujuparams.CloudRegion{{
			Name: "default",
		}}
		cld.CACertificates = []string{"CACERT"}
		cld.Config = map[string]interface{}{"A": "a"}
		cld.RegionConfig = map[string]map[string]interface{}{
			"default": {"B": 2},
		}
		return nil
	},
	username:  "bob@external",
	cloudName: "new-cloud",
	cloud: jujuparams.Cloud{
		Type:             "kubernetes",
		HostCloudRegion:  "test-provider/test-region",
		AuthTypes:        []string{"empty", "userpass"},
		Endpoint:         "https://example.com",
		IdentityEndpoint: "https://example.com/identity",
		StorageEndpoint:  "https://example.com/storage",
	},
	expectCloud: dbmodel.Cloud{
		Name:             "new-cloud",
		Type:             "kubernetes",
		HostCloudRegion:  "test-provider/test-region",
		AuthTypes:        []string{"empty", "userpass"},
		Endpoint:         "https://example.com",
		IdentityEndpoint: "https://example.com/identity",
		StorageEndpoint:  "https://example.com/storage",
		Regions: []dbmodel.CloudRegion{{
			Name:   "default",
			Config: dbmodel.Map{"B": float64(2)},
			Controllers: []dbmodel.CloudRegionControllerPriority{{
				Controller: dbmodel.Controller{
					Name: "test-controller",
					UUID: "00000001-0000-0000-0000-000000000001",
				},
				Priority: 1,
			}},
		}},
		Users: []dbmodel.UserCloudAccess{{
			Username: "bob@external",
			User: dbmodel.User{
				Username:         "bob@external",
				ControllerAccess: "add-model",
			},
			CloudName: "new-cloud",
			Access:    "admin",
		}},
	},
}, {
	name:      "UserWithoutAccess",
	username:  "charlie@external",
	cloudName: "new-cloud",
	cloud: jujuparams.Cloud{
		Type:             "kubernetes",
		HostCloudRegion:  "test-provider/test-region",
		AuthTypes:        []string{"empty", "userpass"},
		Endpoint:         "https://example.com",
		IdentityEndpoint: "https://example.com/identity",
		StorageEndpoint:  "https://example.com/storage",
	},
	expectError:     `unauthorized access`,
	expectErrorCode: errors.CodeUnauthorized,
}, {
	name:      "CloudWithReservedName",
	username:  "bob@external",
	cloudName: "aws",
	cloud: jujuparams.Cloud{
		Type:             "kubernetes",
		HostCloudRegion:  "test-provider/test-region",
		AuthTypes:        []string{"empty", "userpass"},
		Endpoint:         "https://example.com",
		IdentityEndpoint: "https://example.com/identity",
		StorageEndpoint:  "https://example.com/storage",
	},
	expectError:     `cloud "aws" already exists`,
	expectErrorCode: errors.CodeAlreadyExists,
}, {
	name:      "ExistingCloud",
	username:  "bob@external",
	cloudName: "existing-cloud",
	cloud: jujuparams.Cloud{
		Type:             "kubernetes",
		HostCloudRegion:  "test-provider/test-region",
		AuthTypes:        []string{"empty", "userpass"},
		Endpoint:         "https://example.com",
		IdentityEndpoint: "https://example.com/identity",
		StorageEndpoint:  "https://example.com/storage",
	},
	expectError:     `cloud "existing-cloud" already exists`,
	expectErrorCode: errors.CodeAlreadyExists,
}, {
	name:      "InvalidCloudType",
	username:  "bob@external",
	cloudName: "new-cloud",
	cloud: jujuparams.Cloud{
		Type:             "ec2",
		HostCloudRegion:  "test-provider/test-region",
		AuthTypes:        []string{"empty", "userpass"},
		Endpoint:         "https://example.com",
		IdentityEndpoint: "https://example.com/identity",
		StorageEndpoint:  "https://example.com/storage",
	},
	expectError:     `unsupported cloud type "ec2"`,
	expectErrorCode: errors.CodeIncompatibleClouds,
}, {
	name:      "HostCloudRegionNotFound",
	username:  "bob@external",
	cloudName: "new-cloud",
	cloud: jujuparams.Cloud{
		Type:             "kubernetes",
		HostCloudRegion:  "ec2/default",
		AuthTypes:        []string{"empty", "userpass"},
		Endpoint:         "https://example.com",
		IdentityEndpoint: "https://example.com/identity",
		StorageEndpoint:  "https://example.com/storage",
	},
	expectError:     `unsupported cloud host region "ec2/default"`,
	expectErrorCode: errors.CodeIncompatibleClouds,
}, {
	name:      "InvalidHostCloudRegion",
	username:  "bob@external",
	cloudName: "new-cloud",
	cloud: jujuparams.Cloud{
		Type:             "kubernetes",
		HostCloudRegion:  "ec2",
		AuthTypes:        []string{"empty", "userpass"},
		Endpoint:         "https://example.com",
		IdentityEndpoint: "https://example.com/identity",
		StorageEndpoint:  "https://example.com/storage",
	},
	expectError:     `unsupported cloud host region "ec2"`,
	expectErrorCode: errors.CodeIncompatibleClouds,
}, {
	name:      "UserHasNoCloudAccess",
	username:  "bob@external",
	cloudName: "new-cloud",
	cloud: jujuparams.Cloud{
		Type:             "kubernetes",
		HostCloudRegion:  "test-provider2/test-region",
		AuthTypes:        []string{"empty", "userpass"},
		Endpoint:         "https://example.com",
		IdentityEndpoint: "https://example.com/identity",
		StorageEndpoint:  "https://example.com/storage",
	},
	expectError:     `unsupported cloud host region "test-provider2/test-region"`,
	expectErrorCode: errors.CodeIncompatibleClouds,
}, {
	name:      "HostCloudIsHosted",
	username:  "alice@external",
	cloudName: "new-cloud",
	cloud: jujuparams.Cloud{
		Type:             "kubernetes",
		HostCloudRegion:  "kubernetes/default",
		AuthTypes:        []string{"empty", "userpass"},
		Endpoint:         "https://example.com",
		IdentityEndpoint: "https://example.com/identity",
		StorageEndpoint:  "https://example.com/storage",
	},
	expectError:     `unsupported cloud host region "kubernetes/default"`,
	expectErrorCode: errors.CodeIncompatibleClouds,
}, {
	name:      "DialError",
	dialError: errors.E("dial error"),
	username:  "bob@external",
	cloudName: "new-cloud",
	cloud: jujuparams.Cloud{
		Type:             "kubernetes",
		HostCloudRegion:  "test-provider/test-region",
		AuthTypes:        []string{"empty", "userpass"},
		Endpoint:         "https://example.com",
		IdentityEndpoint: "https://example.com/identity",
		StorageEndpoint:  "https://example.com/storage",
	},
	expectError: `dial error`,
}, {
	name: "AddCloudError",
	addCloud: func(context.Context, names.CloudTag, jujuparams.Cloud) error {
		return errors.E("addcloud error")
	},
	username:  "bob@external",
	cloudName: "new-cloud",
	cloud: jujuparams.Cloud{
		Type:             "kubernetes",
		HostCloudRegion:  "test-provider/test-region",
		AuthTypes:        []string{"empty", "userpass"},
		Endpoint:         "https://example.com",
		IdentityEndpoint: "https://example.com/identity",
		StorageEndpoint:  "https://example.com/storage",
	},
	expectError: `addcloud error`,
}}

func TestAddHostedCloud(t *testing.T) {
	c := qt.New(t)

	for _, test := range addHostedCloudTests {
		c.Run(test.name, func(c *qt.C) {
			ctx := context.Background()

			api := &jimmtest.API{
				AddCloud_:         test.addCloud,
				GrantCloudAccess_: test.grantCloudAccess,
				Cloud_:            test.cloud_,
			}

			dialer := &jimmtest.Dialer{
				Err: test.dialError,
				API: api,
			}
			j := &jimm.JIMM{
				Database: db.Database{
					DB: jimmtest.MemoryDB(c, nil),
				},
				Dialer: dialer,
			}

			err := j.Database.Migrate(ctx, false)
			c.Assert(err, qt.IsNil)

			env := jimmtest.ParseEnvironment(c, addHostedCloudTestEnv)
			env.PopulateDB(c, j.Database)

			u := env.User(test.username).DBObject(c, j.Database)

			err = j.AddHostedCloud(ctx, &u, names.NewCloudTag(test.cloudName), test.cloud)
			c.Assert(dialer.IsClosed(), qt.Equals, true)
			if test.expectError != "" {
				c.Check(err, qt.ErrorMatches, test.expectError)
				if test.expectErrorCode != "" {
					c.Check(errors.ErrorCode(err), qt.Equals, test.expectErrorCode)
				}
				return
			}
			cloud, err := j.GetCloud(ctx, &u, names.NewCloudTag(test.cloudName))
			c.Assert(err, qt.IsNil)
			c.Check(cloud, jimmtest.DBObjectEquals, test.expectCloud)
		})
	}
}
