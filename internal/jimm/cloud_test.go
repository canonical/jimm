// Copyright 2020 Canonical Ltd.

package jimm_test

import (
	"context"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v4"

	"github.com/canonical/jimm/internal/auth"
	"github.com/canonical/jimm/internal/db"
	"github.com/canonical/jimm/internal/dbmodel"
	"github.com/canonical/jimm/internal/errors"
	"github.com/canonical/jimm/internal/jimm"
	"github.com/canonical/jimm/internal/jimmtest"
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
				Username: auth.Everyone,
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
		ModelHardDelete: dbmodel.ModelHardDelete{ID: 1, CreatedAt: now, UpdatedAt: now},
		Name:            "test-cloud-1",
		Regions:         []dbmodel.CloudRegion{},
		Users: []dbmodel.UserCloudAccess{{
			ModelHardDelete: dbmodel.ModelHardDelete{ID: 1, CreatedAt: now, UpdatedAt: now},
			Username:        "alice@external",
			User: dbmodel.User{
				ModelHardDelete:  dbmodel.ModelHardDelete{ID: 1, CreatedAt: now, UpdatedAt: now},
				Username:         "alice@external",
				ControllerAccess: "login",
			},
			CloudName: "test-cloud-1",
			Access:    "admin",
		}, {
			ModelHardDelete: dbmodel.ModelHardDelete{ID: 2, CreatedAt: now, UpdatedAt: now},
			Username:        "bob@external",
			User: dbmodel.User{
				ModelHardDelete:  dbmodel.ModelHardDelete{ID: 2, CreatedAt: now, UpdatedAt: now},
				Username:         "bob@external",
				ControllerAccess: "login",
			},
			CloudName: "test-cloud-1",
			Access:    "add-model",
		}},
	})

	cld, err = j.GetCloud(ctx, bob, names.NewCloudTag("test-cloud-1"))
	c.Assert(err, qt.IsNil)
	c.Check(cld, qt.DeepEquals, dbmodel.Cloud{
		ModelHardDelete: dbmodel.ModelHardDelete{ID: 1, CreatedAt: now, UpdatedAt: now},
		Name:            "test-cloud-1",
		Regions:         []dbmodel.CloudRegion{},
		Users: []dbmodel.UserCloudAccess{{
			Username: "bob@external",
			User:     *bob,
			Access:   "add-model",
		}},
	})

	cld, err = j.GetCloud(ctx, daphne, names.NewCloudTag("test-cloud-1"))
	c.Assert(err, qt.IsNil)
	c.Check(cld, qt.DeepEquals, dbmodel.Cloud{
		ModelHardDelete: dbmodel.ModelHardDelete{ID: 1, CreatedAt: now, UpdatedAt: now},
		Name:            "test-cloud-1",
		Regions:         []dbmodel.CloudRegion{},
		Users: []dbmodel.UserCloudAccess{{
			ModelHardDelete: dbmodel.ModelHardDelete{ID: 1, CreatedAt: now, UpdatedAt: now},
			Username:        "alice@external",
			User: dbmodel.User{
				ModelHardDelete:  dbmodel.ModelHardDelete{ID: 1, CreatedAt: now, UpdatedAt: now},
				Username:         "alice@external",
				ControllerAccess: "login",
			},
			CloudName: "test-cloud-1",
			Access:    "admin",
		}, {
			ModelHardDelete: dbmodel.ModelHardDelete{ID: 2, CreatedAt: now, UpdatedAt: now},
			Username:        "bob@external",
			User: dbmodel.User{
				ModelHardDelete:  dbmodel.ModelHardDelete{ID: 2, CreatedAt: now, UpdatedAt: now},
				Username:         "bob@external",
				ControllerAccess: "login",
			},
			CloudName: "test-cloud-1",
			Access:    "add-model",
		}},
	})

	cld, err = j.GetCloud(ctx, charlie, names.NewCloudTag("test-cloud-2"))
	c.Assert(err, qt.IsNil)
	c.Check(cld, qt.DeepEquals, dbmodel.Cloud{
		ModelHardDelete: dbmodel.ModelHardDelete{ID: 2, CreatedAt: now, UpdatedAt: now},
		Name:            "test-cloud-2",
		Regions:         []dbmodel.CloudRegion{},
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
				Username: auth.Everyone,
			},
			Access: "add-model",
		}},
	})
	c.Assert(err, qt.IsNil)

	err = j.Database.AddCloud(ctx, &dbmodel.Cloud{
		Name: "test-cloud-3",
		Users: []dbmodel.UserCloudAccess{{
			User: dbmodel.User{
				Username: auth.Everyone,
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
		ModelHardDelete: dbmodel.ModelHardDelete{ID: 1, CreatedAt: now, UpdatedAt: now},
		Name:            "test-cloud-1",
		Regions:         []dbmodel.CloudRegion{},
		Users: []dbmodel.UserCloudAccess{{
			ModelHardDelete: dbmodel.ModelHardDelete{ID: 1, CreatedAt: now, UpdatedAt: now},
			Username:        "alice@external",
			User: dbmodel.User{
				ModelHardDelete:  dbmodel.ModelHardDelete{ID: 1, CreatedAt: now, UpdatedAt: now},
				Username:         "alice@external",
				ControllerAccess: "login",
			},
			CloudName: "test-cloud-1",
			Access:    "admin",
		}, {
			ModelHardDelete: dbmodel.ModelHardDelete{ID: 2, CreatedAt: now, UpdatedAt: now},
			Username:        "bob@external",
			User: dbmodel.User{
				ModelHardDelete:  dbmodel.ModelHardDelete{ID: 2, CreatedAt: now, UpdatedAt: now},
				Username:         "bob@external",
				ControllerAccess: "login",
			},
			CloudName: "test-cloud-1",
			Access:    "add-model",
		}},
	}, {
		ModelHardDelete: dbmodel.ModelHardDelete{ID: 2, CreatedAt: now, UpdatedAt: now},
		Name:            "test-cloud-2",
		Regions:         []dbmodel.CloudRegion{},
		Users: []dbmodel.UserCloudAccess{{
			Username: "alice@external",
			User:     *alice,
			Access:   "add-model",
		}},
	}, {
		ModelHardDelete: dbmodel.ModelHardDelete{ID: 3, CreatedAt: now, UpdatedAt: now},
		Name:            "test-cloud-3",
		Regions:         []dbmodel.CloudRegion{},
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
		ModelHardDelete: dbmodel.ModelHardDelete{ID: 1, CreatedAt: now, UpdatedAt: now},
		Name:            "test-cloud-1",
		Regions:         []dbmodel.CloudRegion{},
		Users: []dbmodel.UserCloudAccess{{
			Username: "bob@external",
			User:     *bob,
			Access:   "add-model",
		}},
	}, {
		ModelHardDelete: dbmodel.ModelHardDelete{ID: 2, CreatedAt: now, UpdatedAt: now},
		Name:            "test-cloud-2",
		Regions:         []dbmodel.CloudRegion{},
		Users: []dbmodel.UserCloudAccess{{
			Username: "bob@external",
			User:     *bob,
			Access:   "add-model",
		}},
	}, {
		ModelHardDelete: dbmodel.ModelHardDelete{ID: 3, CreatedAt: now, UpdatedAt: now},
		Name:            "test-cloud-3",
		Regions:         []dbmodel.CloudRegion{},
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
		ModelHardDelete: dbmodel.ModelHardDelete{ID: 2, CreatedAt: now, UpdatedAt: now},
		Name:            "test-cloud-2",
		Regions:         []dbmodel.CloudRegion{},
		Users: []dbmodel.UserCloudAccess{{
			Username: "charlie@external",
			User:     *charlie,
			Access:   "add-model",
		}},
	}, {
		ModelHardDelete: dbmodel.ModelHardDelete{ID: 3, CreatedAt: now, UpdatedAt: now},
		Name:            "test-cloud-3",
		Regions:         []dbmodel.CloudRegion{},
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
		ModelHardDelete: dbmodel.ModelHardDelete{ID: 1, CreatedAt: now, UpdatedAt: now},
		Name:            "test-cloud-1",
		Regions:         []dbmodel.CloudRegion{},
		Users: []dbmodel.UserCloudAccess{{
			ModelHardDelete: dbmodel.ModelHardDelete{ID: 1, CreatedAt: now, UpdatedAt: now},
			Username:        "alice@external",
			User: dbmodel.User{
				ModelHardDelete:  dbmodel.ModelHardDelete{ID: 1, CreatedAt: now, UpdatedAt: now},
				Username:         "alice@external",
				ControllerAccess: "login",
			},
			CloudName: "test-cloud-1",
			Access:    "admin",
		}, {
			ModelHardDelete: dbmodel.ModelHardDelete{ID: 2, CreatedAt: now, UpdatedAt: now},
			Username:        "bob@external",
			User: dbmodel.User{
				ModelHardDelete:  dbmodel.ModelHardDelete{ID: 2, CreatedAt: now, UpdatedAt: now},
				Username:         "bob@external",
				ControllerAccess: "login",
			},
			CloudName: "test-cloud-1",
			Access:    "add-model",
		}},
	}, {
		ModelHardDelete: dbmodel.ModelHardDelete{ID: 2, CreatedAt: now, UpdatedAt: now},
		Name:            "test-cloud-2",
		Regions:         []dbmodel.CloudRegion{},
		Users: []dbmodel.UserCloudAccess{{
			ModelHardDelete: dbmodel.ModelHardDelete{ID: 3, CreatedAt: now, UpdatedAt: now},
			Username:        "bob@external",
			User: dbmodel.User{
				ModelHardDelete:  dbmodel.ModelHardDelete{ID: 2, CreatedAt: now, UpdatedAt: now},
				Username:         "bob@external",
				ControllerAccess: "login",
			},
			CloudName: "test-cloud-2",
			Access:    "add-model",
		}, {
			ModelHardDelete: dbmodel.ModelHardDelete{ID: 4, CreatedAt: now, UpdatedAt: now},
			Username:        auth.Everyone,
			User: dbmodel.User{
				ModelHardDelete:  dbmodel.ModelHardDelete{ID: 3, CreatedAt: now, UpdatedAt: now},
				Username:         auth.Everyone,
				ControllerAccess: "login",
			},
			CloudName: "test-cloud-2",
			Access:    "add-model",
		}},
	}, {
		ModelHardDelete: dbmodel.ModelHardDelete{ID: 3, CreatedAt: now, UpdatedAt: now},
		Name:            "test-cloud-3",
		Regions:         []dbmodel.CloudRegion{},
		Users: []dbmodel.UserCloudAccess{{
			ModelHardDelete: dbmodel.ModelHardDelete{ID: 5, CreatedAt: now, UpdatedAt: now},
			Username:        auth.Everyone,
			User: dbmodel.User{
				ModelHardDelete:  dbmodel.ModelHardDelete{ID: 3, CreatedAt: now, UpdatedAt: now},
				Username:         auth.Everyone,
				ControllerAccess: "login",
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
- name: private-cloud2
  type: test-provider3
  regions:
  - name: test-region-2
  users:
  - user: bob@external
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
  cloud: test-cloud
  region: test-region
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
					Name:        "test-controller",
					UUID:        "00000001-0000-0000-0000-000000000001",
					CloudName:   "test-cloud",
					CloudRegion: "test-region",
				},
				Priority: 1,
			}},
		}},
		CACertificates: dbmodel.Strings{"CACERT"},
		Config:         dbmodel.Map{"A": string("a")},
		Users: []dbmodel.UserCloudAccess{{
			Username: "bob@external",
			User: dbmodel.User{
				Username:         "bob@external",
				ControllerAccess: "login",
			},
			CloudName: "new-cloud",
			Access:    "admin",
		}},
	},
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

var addHostedCloudToControllerTests = []struct {
	name             string
	dialError        error
	addCloud         func(context.Context, names.CloudTag, jujuparams.Cloud) error
	grantCloudAccess func(context.Context, names.CloudTag, names.UserTag, string) error
	cloud_           func(context.Context, names.CloudTag, *jujuparams.Cloud) error
	username         string
	controllerName   string
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
		cld.Type = "maas"
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
	username:       "bob@external",
	controllerName: "test-controller",
	cloudName:      "new-cloud",
	cloud: jujuparams.Cloud{
		Type:             "maas",
		AuthTypes:        []string{"empty", "userpass"},
		Endpoint:         "https://example.com",
		IdentityEndpoint: "https://example.com/identity",
		StorageEndpoint:  "https://example.com/storage",
	},
	expectCloud: dbmodel.Cloud{
		Name:             "new-cloud",
		Type:             "maas",
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
					Name:        "test-controller",
					UUID:        "00000001-0000-0000-0000-000000000001",
					CloudName:   "test-cloud",
					CloudRegion: "test-region",
				},
				Priority: 1,
			}},
		}},
		CACertificates: dbmodel.Strings{"CACERT"},
		Config:         dbmodel.Map{"A": string("a")},
		Users: []dbmodel.UserCloudAccess{{
			Username: "bob@external",
			User: dbmodel.User{
				Username:         "bob@external",
				ControllerAccess: "login",
			},
			CloudName: "new-cloud",
			Access:    "admin",
		}},
	},
}, {
	name: "Controller not found",
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
	username:       "alice@external",
	controllerName: "no-such-controller",
	cloudName:      "new-cloud",
	cloud: jujuparams.Cloud{
		Type:             "kubernetes",
		HostCloudRegion:  "test-provider/test-region",
		AuthTypes:        []string{"empty", "userpass"},
		Endpoint:         "https://example.com",
		IdentityEndpoint: "https://example.com/identity",
		StorageEndpoint:  "https://example.com/storage",
	},
	expectError:     `controller not found`,
	expectErrorCode: errors.CodeNotFound,
}, {
	name:           "CloudWithReservedName",
	username:       "alice@external",
	controllerName: "test-controller",
	cloudName:      "aws",
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
	name:           "HostCloudRegionNotFound",
	username:       "alice@external",
	controllerName: "test-controller",
	cloudName:      "new-cloud",
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
	name:           "InvalidHostCloudRegion",
	username:       "alice@external",
	controllerName: "test-controller",
	cloudName:      "new-cloud",
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
	name:           "UserHasNoCloudAccess",
	username:       "alice@external",
	controllerName: "test-controller",
	cloudName:      "new-cloud",
	cloud: jujuparams.Cloud{
		Type:             "kubernetes",
		HostCloudRegion:  "test-provider3/test-region-3",
		AuthTypes:        []string{"empty", "userpass"},
		Endpoint:         "https://example.com",
		IdentityEndpoint: "https://example.com/identity",
		StorageEndpoint:  "https://example.com/storage",
	},
	expectError:     `unsupported cloud host region "test-provider3/test-region-3"`,
	expectErrorCode: errors.CodeIncompatibleClouds,
}, {
	name:           "HostCloudIsHosted",
	username:       "alice@external",
	controllerName: "test-controller",
	cloudName:      "new-cloud",
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
	name:           "DialError",
	dialError:      errors.E("dial error"),
	username:       "alice@external",
	controllerName: "test-controller",
	cloudName:      "new-cloud",
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
	username:       "alice@external",
	controllerName: "test-controller",
	cloudName:      "new-cloud",
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

func TestAddCloudToController(t *testing.T) {
	c := qt.New(t)

	for _, test := range addHostedCloudToControllerTests {
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

			err = j.AddCloudToController(ctx, &u, test.controllerName, names.NewCloudTag(test.cloudName), test.cloud)
			c.Assert(dialer.IsClosed(), qt.Equals, true)
			if test.expectError != "" {
				c.Check(err, qt.ErrorMatches, test.expectError)
				if test.expectErrorCode != "" {
					c.Check(errors.ErrorCode(err), qt.Equals, test.expectErrorCode)
				}
				return
			}
			c.Assert(err, qt.IsNil)

			cloud, err := j.GetCloud(ctx, &u, names.NewCloudTag(test.cloudName))
			c.Assert(err, qt.IsNil)
			c.Check(cloud, jimmtest.DBObjectEquals, test.expectCloud)
		})
	}
}

const grantCloudAccessTestEnv = `clouds:
- name: test-cloud
  type: test-provider
  regions:
  - name: test-cloud-region
- name: test
  type: kubernetes
  host-cloud-region: test-cloud/test-cloud-region
  regions:
  - name: default
  - name: region2
  users:
  - user: alice@external
    access: admin
controllers:
- name: controller-1
  uuid: 00000001-0000-0000-0000-000000000001
  cloud: test-cloud
  region: test-cloud-region
  cloud-regions:
  - cloud: test-cloud
    region: test-cloud-region
    priority: 10
  - cloud: test
    region: default
    priority: 1
  - cloud: test
    region: region2
    priority: 1
`

var grantCloudAccessTests = []struct {
	name             string
	env              string
	grantCloudAccess func(context.Context, names.CloudTag, names.UserTag, string) error
	dialError        error
	username         string
	cloud            string
	targetUsername   string
	access           string
	expectCloud      dbmodel.Cloud
	expectError      string
	expectErrorCode  errors.Code
}{{
	name:            "CloudNotFound",
	username:        "alice@external",
	cloud:           "test2",
	targetUsername:  "bob@external",
	access:          "add-model",
	expectError:     `cloud "test2" not found`,
	expectErrorCode: errors.CodeNotFound,
}, {
	name: "Success",
	env:  grantCloudAccessTestEnv,
	grantCloudAccess: func(_ context.Context, ct names.CloudTag, ut names.UserTag, access string) error {
		if ct.Id() != "test" {
			return errors.E("bad cloud tag")
		}
		if ut.Id() != "bob@external" {
			return errors.E("bad user tag")
		}
		if access != "add-model" {
			return errors.E("bad permission")
		}
		return nil
	},
	username:       "alice@external",
	cloud:          "test",
	targetUsername: "bob@external",
	access:         "add-model",
	expectCloud: dbmodel.Cloud{
		Name:            "test",
		Type:            "kubernetes",
		HostCloudRegion: "test-cloud/test-cloud-region",
		Regions: []dbmodel.CloudRegion{{
			Name: "default",
			Controllers: []dbmodel.CloudRegionControllerPriority{{
				Controller: dbmodel.Controller{
					Name:        "controller-1",
					UUID:        "00000001-0000-0000-0000-000000000001",
					CloudName:   "test-cloud",
					CloudRegion: "test-cloud-region",
				},
				Priority: 1,
			}},
		}, {
			Name: "region2",
			Controllers: []dbmodel.CloudRegionControllerPriority{{
				Controller: dbmodel.Controller{
					Name:        "controller-1",
					UUID:        "00000001-0000-0000-0000-000000000001",
					CloudName:   "test-cloud",
					CloudRegion: "test-cloud-region",
				},
				Priority: 1,
			}},
		}},
		Users: []dbmodel.UserCloudAccess{{
			Username: "alice@external",
			User: dbmodel.User{
				Username:         "alice@external",
				ControllerAccess: "login",
			},
			CloudName: "test",
			Access:    "admin",
		}, {
			Username: "bob@external",
			User: dbmodel.User{
				Username:         "bob@external",
				ControllerAccess: "login",
			},
			CloudName: "test",
			Access:    "add-model",
		}},
	},
}, {
	name:            "UserNotAuthorized",
	env:             grantCloudAccessTestEnv,
	username:        "charlie@external",
	cloud:           "test",
	targetUsername:  "bob@external",
	access:          "add-model",
	expectError:     `unauthorized`,
	expectErrorCode: errors.CodeUnauthorized,
}, {
	name:           "DialError",
	env:            grantCloudAccessTestEnv,
	dialError:      errors.E("test dial error"),
	username:       "alice@external",
	cloud:          "test",
	targetUsername: "bob@external",
	access:         "add-model",
	expectError:    `test dial error`,
}, {
	name: "APIError",
	env:  grantCloudAccessTestEnv,
	grantCloudAccess: func(_ context.Context, mt names.CloudTag, ut names.UserTag, access string) error {
		return errors.E("test error")
	},
	username:       "alice@external",
	cloud:          "test",
	targetUsername: "bob@external",
	access:         "add-model",
	expectError:    `test error`,
}}

func TestGrantCloudAccess(t *testing.T) {
	c := qt.New(t)

	for _, test := range grantCloudAccessTests {
		c.Run(test.name, func(c *qt.C) {
			ctx := context.Background()

			env := jimmtest.ParseEnvironment(c, test.env)
			dialer := &jimmtest.Dialer{
				API: &jimmtest.API{
					GrantCloudAccess_: test.grantCloudAccess,
				},
				Err: test.dialError,
			}
			j := &jimm.JIMM{
				Database: db.Database{
					DB: jimmtest.MemoryDB(c, nil),
				},
				Dialer: dialer,
			}
			err := j.Database.Migrate(ctx, false)
			c.Assert(err, qt.IsNil)
			env.PopulateDB(c, j.Database)

			u := env.User(test.username).DBObject(c, j.Database)

			err = j.GrantCloudAccess(ctx, &u, names.NewCloudTag(test.cloud), names.NewUserTag(test.targetUsername), test.access)
			c.Assert(dialer.IsClosed(), qt.Equals, true)
			if test.expectError != "" {
				c.Check(err, qt.ErrorMatches, test.expectError)
				if test.expectErrorCode != "" {
					c.Check(errors.ErrorCode(err), qt.Equals, test.expectErrorCode)
				}
				return
			}
			c.Assert(err, qt.IsNil)
			cl := dbmodel.Cloud{
				Name: test.cloud,
			}
			err = j.Database.GetCloud(ctx, &cl)
			c.Assert(err, qt.IsNil)
			c.Check(cl, jimmtest.DBObjectEquals, test.expectCloud)
		})
	}
}

const revokeCloudAccessTestEnv = `clouds:
- name: test-cloud
  type: test-provider
  regions:
  - name: test-cloud-region
- name: test
  type: kubernetes
  host-cloud-region: test-cloud/test-cloud-region
  regions:
  - name: default
  users:
  - user: alice@external
    access: admin
  - user: bob@external
    access: admin
  - user: charlie@external
    access: add-model
controllers:
- name: controller-1
  uuid: 00000001-0000-0000-0000-000000000001
  cloud: test-cloud
  region: test-cloud-region
  cloud-regions:
  - cloud: test-cloud
    region: test-cloud-region
    priority: 10
  - cloud: test
    region: default
    priority: 1
`

var revokeCloudAccessTests = []struct {
	name              string
	env               string
	revokeCloudAccess func(context.Context, names.CloudTag, names.UserTag, string) error
	dialError         error
	username          string
	cloud             string
	targetUsername    string
	access            string
	expectCloud       dbmodel.Cloud
	expectError       string
	expectErrorCode   errors.Code
}{{
	name:            "CloudNotFound",
	username:        "alice@external",
	cloud:           "test2",
	targetUsername:  "bob@external",
	access:          "admin",
	expectError:     `cloud "test2" not found`,
	expectErrorCode: errors.CodeNotFound,
}, {
	name: "SuccessAdmin",
	env:  revokeCloudAccessTestEnv,
	revokeCloudAccess: func(_ context.Context, ct names.CloudTag, ut names.UserTag, access string) error {
		if ct.Id() != "test" {
			return errors.E("bad model tag")
		}
		if ut.Id() != "bob@external" {
			return errors.E("bad user tag")
		}
		if access != "admin" {
			return errors.E("bad permission")
		}
		return nil
	},
	username:       "alice@external",
	cloud:          "test",
	targetUsername: "bob@external",
	access:         "admin",
	expectCloud: dbmodel.Cloud{
		Name:            "test",
		Type:            "kubernetes",
		HostCloudRegion: "test-cloud/test-cloud-region",
		Regions: []dbmodel.CloudRegion{{
			Name: "default",
			Controllers: []dbmodel.CloudRegionControllerPriority{{
				Controller: dbmodel.Controller{
					Name:        "controller-1",
					UUID:        "00000001-0000-0000-0000-000000000001",
					CloudName:   "test-cloud",
					CloudRegion: "test-cloud-region",
				},
				Priority: 1,
			}},
		}},
		Users: []dbmodel.UserCloudAccess{{
			Username: "alice@external",
			User: dbmodel.User{
				Username:         "alice@external",
				ControllerAccess: "login",
			},
			CloudName: "test",
			Access:    "admin",
		}, {
			Username: "bob@external",
			User: dbmodel.User{
				Username:         "bob@external",
				ControllerAccess: "login",
			},
			CloudName: "test",
			Access:    "add-model",
		}, {
			Username: "charlie@external",
			User: dbmodel.User{
				Username:         "charlie@external",
				ControllerAccess: "login",
			},
			CloudName: "test",
			Access:    "add-model",
		}},
	},
}, {
	name: "SuccessAddModel",
	env:  revokeCloudAccessTestEnv,
	revokeCloudAccess: func(_ context.Context, ct names.CloudTag, ut names.UserTag, access string) error {
		if ct.Id() != "test" {
			return errors.E("bad model tag")
		}
		if ut.Id() != "bob@external" {
			return errors.E("bad user tag")
		}
		if access != "add-model" {
			return errors.E("bad permission")
		}
		return nil
	},
	username:       "alice@external",
	cloud:          "test",
	targetUsername: "bob@external",
	access:         "add-model",
	expectCloud: dbmodel.Cloud{
		Name:            "test",
		Type:            "kubernetes",
		HostCloudRegion: "test-cloud/test-cloud-region",
		Regions: []dbmodel.CloudRegion{{
			Name: "default",
			Controllers: []dbmodel.CloudRegionControllerPriority{{
				Controller: dbmodel.Controller{
					Name:        "controller-1",
					UUID:        "00000001-0000-0000-0000-000000000001",
					CloudName:   "test-cloud",
					CloudRegion: "test-cloud-region",
				},
				Priority: 1,
			}},
		}},
		Users: []dbmodel.UserCloudAccess{{
			Username: "alice@external",
			User: dbmodel.User{
				Username:         "alice@external",
				ControllerAccess: "login",
			},
			CloudName: "test",
			Access:    "admin",
		}, {
			Username: "charlie@external",
			User: dbmodel.User{
				Username:         "charlie@external",
				ControllerAccess: "login",
			},
			CloudName: "test",
			Access:    "add-model",
		}},
	},
}, {
	name:            "UserNotAuthorized",
	env:             revokeCloudAccessTestEnv,
	username:        "charlie@external",
	cloud:           "test",
	targetUsername:  "bob@external",
	access:          "add-model",
	expectError:     `unauthorized`,
	expectErrorCode: errors.CodeUnauthorized,
}, {
	name:           "DialError",
	env:            revokeCloudAccessTestEnv,
	dialError:      errors.E("test dial error"),
	username:       "alice@external",
	cloud:          "test",
	targetUsername: "bob@external",
	access:         "add-model",
	expectError:    `test dial error`,
}, {
	name: "APIError",
	env:  revokeCloudAccessTestEnv,
	revokeCloudAccess: func(_ context.Context, mt names.CloudTag, ut names.UserTag, access string) error {
		return errors.E("test error")
	},
	username:       "alice@external",
	cloud:          "test",
	targetUsername: "bob@external",
	access:         "add-model",
	expectError:    `test error`,
}}

func TestRevokeCloudAccess(t *testing.T) {
	c := qt.New(t)

	for _, test := range revokeCloudAccessTests {
		c.Run(test.name, func(c *qt.C) {
			ctx := context.Background()

			env := jimmtest.ParseEnvironment(c, test.env)
			dialer := &jimmtest.Dialer{
				API: &jimmtest.API{
					RevokeCloudAccess_: test.revokeCloudAccess,
				},
				Err: test.dialError,
			}
			j := &jimm.JIMM{
				Database: db.Database{
					DB: jimmtest.MemoryDB(c, nil),
				},
				Dialer: dialer,
			}
			err := j.Database.Migrate(ctx, false)
			c.Assert(err, qt.IsNil)
			env.PopulateDB(c, j.Database)

			u := env.User(test.username).DBObject(c, j.Database)

			err = j.RevokeCloudAccess(ctx, &u, names.NewCloudTag(test.cloud), names.NewUserTag(test.targetUsername), test.access)
			c.Assert(dialer.IsClosed(), qt.Equals, true)
			if test.expectError != "" {
				c.Check(err, qt.ErrorMatches, test.expectError)
				if test.expectErrorCode != "" {
					c.Check(errors.ErrorCode(err), qt.Equals, test.expectErrorCode)
				}
				return
			}
			c.Assert(err, qt.IsNil)
			cl := dbmodel.Cloud{
				Name: test.cloud,
			}
			err = j.Database.GetCloud(ctx, &cl)
			c.Assert(err, qt.IsNil)
			c.Check(cl, jimmtest.DBObjectEquals, test.expectCloud)
		})
	}
}

const removeCloudTestEnv = `clouds:
- name: test-cloud
  type: test-provider
  regions:
  - name: test-cloud-region
- name: test
  type: kubernetes
  host-cloud-region: test-cloud/test-cloud-region
  regions:
  - name: default
  users:
  - user: alice@external
    access: admin
  - user: bob@external
    access: add-model
controllers:
- name: controller-1
  uuid: 00000001-0000-0000-0000-000000000001
  cloud: test-cloud
  region: test-cloud-region
  cloud-regions:
  - cloud: test-cloud
    region: test-cloud-region
    priority: 10
  - cloud: test
    region: default
    priority: 1
`

var removeCloudTests = []struct {
	name            string
	env             string
	removeCloud     func(context.Context, names.CloudTag) error
	dialError       error
	username        string
	cloud           string
	expectError     string
	expectErrorCode errors.Code
}{{
	name:            "CloudNotFound",
	username:        "alice@external",
	cloud:           "test2",
	expectError:     `cloud "test2" not found`,
	expectErrorCode: errors.CodeNotFound,
}, {
	name: "Success",
	env:  removeCloudTestEnv,
	removeCloud: func(_ context.Context, ct names.CloudTag) error {
		if ct.Id() != "test" {
			return errors.E("bad cloud tag")
		}
		return nil
	},
	username: "alice@external",
	cloud:    "test",
}, {
	name:            "UserNotAuthorized",
	env:             removeCloudTestEnv,
	username:        "bob@external",
	cloud:           "test",
	expectError:     `unauthorized`,
	expectErrorCode: errors.CodeUnauthorized,
}, {
	name:        "DialError",
	env:         removeCloudTestEnv,
	dialError:   errors.E("test dial error"),
	username:    "alice@external",
	cloud:       "test",
	expectError: `test dial error`,
}, {
	name: "APIError",
	env:  removeCloudTestEnv,
	removeCloud: func(_ context.Context, mt names.CloudTag) error {
		return errors.E("test error")
	},
	username:    "alice@external",
	cloud:       "test",
	expectError: `test error`,
}}

func TestRemoveCloud(t *testing.T) {
	c := qt.New(t)

	for _, test := range removeCloudTests {
		c.Run(test.name, func(c *qt.C) {
			ctx := context.Background()

			env := jimmtest.ParseEnvironment(c, test.env)
			dialer := &jimmtest.Dialer{
				API: &jimmtest.API{
					RemoveCloud_: test.removeCloud,
				},
				Err: test.dialError,
			}
			j := &jimm.JIMM{
				Database: db.Database{
					DB: jimmtest.MemoryDB(c, nil),
				},
				Dialer: dialer,
			}
			err := j.Database.Migrate(ctx, false)
			c.Assert(err, qt.IsNil)
			env.PopulateDB(c, j.Database)

			u := env.User(test.username).DBObject(c, j.Database)

			err = j.RemoveCloud(ctx, &u, names.NewCloudTag(test.cloud))
			c.Assert(dialer.IsClosed(), qt.Equals, true)
			if test.expectError != "" {
				c.Check(err, qt.ErrorMatches, test.expectError)
				if test.expectErrorCode != "" {
					c.Check(errors.ErrorCode(err), qt.Equals, test.expectErrorCode)
				}
				return
			}
			c.Assert(err, qt.IsNil)
			cl := dbmodel.Cloud{
				Name: test.cloud,
			}
			err = j.Database.GetCloud(ctx, &cl)
			c.Assert(errors.ErrorCode(err), qt.Equals, errors.CodeNotFound)
		})
	}
}

const updateCloudTestEnv = `clouds:
- name: test-cloud
  type: test-provider
  regions:
  - name: test-cloud-region
- name: test
  type: kubernetes
  host-cloud-region: test-cloud/test-cloud-region
  regions:
  - name: default
  users:
  - user: alice@external
    access: admin
  - user: bob@external
    access: admin
controllers:
- name: controller-1
  uuid: 00000001-0000-0000-0000-000000000001
  cloud: test-cloud
  region: test-cloud-region
  cloud-regions:
  - cloud: test-cloud
    region: test-cloud-region
    priority: 10
  - cloud: test
    region: default
    priority: 1
users:
- username: alice@external
  controller-access: superuser
`

var updateCloudTests = []struct {
	name            string
	env             string
	updateCloud     func(context.Context, names.CloudTag, jujuparams.Cloud) error
	dialError       error
	username        string
	cloud           string
	update          jujuparams.Cloud
	expectError     string
	expectErrorCode errors.Code
	expectCloud     dbmodel.Cloud
}{{
	name:            "CloudNotFound",
	username:        "alice@external",
	cloud:           "test2",
	expectError:     `cloud "test2" not found`,
	expectErrorCode: errors.CodeNotFound,
}, {
	name: "SuccessPublicCloud",
	env:  updateCloudTestEnv,
	updateCloud: func(_ context.Context, ct names.CloudTag, c jujuparams.Cloud) error {
		if ct.Id() != "test-cloud" {
			return errors.E("bad cloud tag")
		}
		return nil
	},
	username: "alice@external",
	cloud:    "test-cloud",
	update: jujuparams.Cloud{
		Type:             "test-provider",
		AuthTypes:        []string{"empty", "userpass"},
		Endpoint:         "https://example.com",
		IdentityEndpoint: "https://identity.example.com",
		StorageEndpoint:  "https://storage.example.com",
		Regions: []jujuparams.CloudRegion{{
			Name:             "test-cloud-region",
			Endpoint:         "https://region.example.com",
			IdentityEndpoint: "https://identity.region.example.com",
			StorageEndpoint:  "https://storage.region.example.com",
		}, {
			Name:             "test-cloud-region-2",
			Endpoint:         "https://region2.example.com",
			IdentityEndpoint: "https://identity.region2.example.com",
			StorageEndpoint:  "https://storage.region2.example.com",
		}},
	},
	expectCloud: dbmodel.Cloud{
		Name:             "test-cloud",
		Type:             "test-provider",
		AuthTypes:        dbmodel.Strings{"empty", "userpass"},
		Endpoint:         "https://example.com",
		IdentityEndpoint: "https://identity.example.com",
		StorageEndpoint:  "https://storage.example.com",
		Regions: []dbmodel.CloudRegion{{
			Name:             "test-cloud-region",
			Endpoint:         "https://region.example.com",
			IdentityEndpoint: "https://identity.region.example.com",
			StorageEndpoint:  "https://storage.region.example.com",
			Controllers: []dbmodel.CloudRegionControllerPriority{{
				Controller: dbmodel.Controller{
					Name:        "controller-1",
					UUID:        "00000001-0000-0000-0000-000000000001",
					CloudName:   "test-cloud",
					CloudRegion: "test-cloud-region",
				},
				Priority: 10,
			}},
		}, {
			Name:             "test-cloud-region-2",
			Endpoint:         "https://region2.example.com",
			IdentityEndpoint: "https://identity.region2.example.com",
			StorageEndpoint:  "https://storage.region2.example.com",
			Controllers: []dbmodel.CloudRegionControllerPriority{{
				Controller: dbmodel.Controller{
					Name:        "controller-1",
					UUID:        "00000001-0000-0000-0000-000000000001",
					CloudName:   "test-cloud",
					CloudRegion: "test-cloud-region",
				},
				Priority: 1,
			}},
		}},
	},
}, {
	name: "SuccessHostedCloud",
	env:  updateCloudTestEnv,
	updateCloud: func(_ context.Context, ct names.CloudTag, c jujuparams.Cloud) error {
		if ct.Id() != "test" {
			return errors.E("bad cloud tag")
		}
		return nil
	},
	username: "bob@external",
	cloud:    "test",
	update: jujuparams.Cloud{
		Type:             "kubernetes",
		HostCloudRegion:  "test-cloud/test-cloud-region",
		AuthTypes:        []string{"empty", "userpass"},
		Endpoint:         "https://k8s.example.com",
		IdentityEndpoint: "https://k8s.identity.example.com",
		StorageEndpoint:  "https://k8s.storage.example.com",
		Regions: []jujuparams.CloudRegion{{
			Name: "default",
		}},
	},
	expectCloud: dbmodel.Cloud{
		Name:             "test",
		Type:             "kubernetes",
		HostCloudRegion:  "test-cloud/test-cloud-region",
		AuthTypes:        []string{"empty", "userpass"},
		Endpoint:         "https://k8s.example.com",
		IdentityEndpoint: "https://k8s.identity.example.com",
		StorageEndpoint:  "https://k8s.storage.example.com",
		Regions: []dbmodel.CloudRegion{{
			Name: "default",
			Controllers: []dbmodel.CloudRegionControllerPriority{{
				Controller: dbmodel.Controller{
					Name:        "controller-1",
					UUID:        "00000001-0000-0000-0000-000000000001",
					CloudName:   "test-cloud",
					CloudRegion: "test-cloud-region",
				},
				Priority: 1,
			}},
		}},
		Users: []dbmodel.UserCloudAccess{{
			Username: "alice@external",
			User: dbmodel.User{
				Username:         "alice@external",
				ControllerAccess: "superuser",
			},
			CloudName: "test",
			Access:    "admin",
		}, {
			Username: "bob@external",
			User: dbmodel.User{
				Username:         "bob@external",
				ControllerAccess: "login",
			},
			CloudName: "test",
			Access:    "admin",
		}},
	},
}, {
	name:            "UserNotAuthorized",
	env:             updateCloudTestEnv,
	username:        "bob@external",
	cloud:           "test-cloud",
	expectError:     `unauthorized`,
	expectErrorCode: errors.CodeUnauthorized,
}, {
	name:        "DialError",
	env:         updateCloudTestEnv,
	dialError:   errors.E("test dial error"),
	username:    "alice@external",
	cloud:       "test-cloud",
	expectError: `test dial error`,
}, {
	name: "APIError",
	env:  updateCloudTestEnv,
	updateCloud: func(context.Context, names.CloudTag, jujuparams.Cloud) error {
		return errors.E("test error")
	},
	username:    "alice@external",
	cloud:       "test-cloud",
	expectError: `test error`,
}}

func TestUpdateCloud(t *testing.T) {
	c := qt.New(t)

	for _, test := range updateCloudTests {
		c.Run(test.name, func(c *qt.C) {
			ctx := context.Background()

			env := jimmtest.ParseEnvironment(c, test.env)
			dialer := &jimmtest.Dialer{
				API: &jimmtest.API{
					UpdateCloud_: test.updateCloud,
				},
				Err:          test.dialError,
				UUID:         "00000001-0000-0000-0000-000000000001",
				AgentVersion: "1",
			}
			j := &jimm.JIMM{
				Database: db.Database{
					DB: jimmtest.MemoryDB(c, nil),
				},
				Dialer: dialer,
			}
			err := j.Database.Migrate(ctx, false)
			c.Assert(err, qt.IsNil)
			env.PopulateDB(c, j.Database)

			u := env.User(test.username).DBObject(c, j.Database)

			tag := names.NewCloudTag(test.cloud)
			err = j.UpdateCloud(ctx, &u, tag, test.update)
			c.Assert(dialer.IsClosed(), qt.Equals, true)
			if test.expectError != "" {
				c.Check(err, qt.ErrorMatches, test.expectError)
				if test.expectErrorCode != "" {
					c.Check(errors.ErrorCode(err), qt.Equals, test.expectErrorCode)
				}
				return
			}
			c.Assert(err, qt.IsNil)
			var cloud dbmodel.Cloud
			cloud.SetTag(tag)
			err = j.Database.GetCloud(ctx, &cloud)
			c.Assert(err, qt.IsNil)
			c.Check(cloud, jimmtest.DBObjectEquals, test.expectCloud)
		})
	}
}

const removeCloudFromControllerTestEnv = `clouds:
- name: test-cloud
  type: test-provider
  regions:
  - name: test-cloud-region-1
  - name: test-cloud-region-2
- name: test-cloud-2
  type: test-provider
  regions:
  - name: default
  users:
  - user: alice@external
    access: admin
  - user: bob@external
    access: add-model
- name: test
  type: kubernetes
  host-cloud-region: test-cloud/test-cloud-region
  regions:
  - name: default
  users:
  - user: alice@external
    access: admin
  - user: bob@external
    access: add-model
controllers:
- name: controller-1
  uuid: 00000001-0000-0000-0000-000000000001
  cloud: test-cloud
  region: test-cloud-region
  cloud-regions:
  - cloud: test-cloud
    region: test-cloud-region-1
    priority: 10
  - cloud: test
    region: default
    priority: 1
- name: controller-2
  uuid: 00000001-0000-0000-0000-000000000002
  cloud: test-cloud
  region: test-cloud-region
  cloud-regions:
  - cloud: test-cloud
    region: test-cloud-region-2
    priority: 10
  - cloud: test
    region: default
    priority: 1
  - cloud: test-cloud-2
    region: default
    priority: 2
`

var removeCloudFromControllerTests = []struct {
	name            string
	env             string
	removeCloud     func(context.Context, names.CloudTag) error
	dialError       error
	username        string
	cloud           string
	controllerName  string
	expectError     string
	expectErrorCode errors.Code
	assertSuccess   func(c *qt.C, j *jimm.JIMM)
}{{
	name:            "CloudNotFound",
	username:        "alice@external",
	cloud:           "test2",
	controllerName:  "controller-2",
	expectError:     `cloud "test2" not found`,
	expectErrorCode: errors.CodeNotFound,
}, {
	name: "Success - with other controllers for the cloud",
	env:  removeCloudFromControllerTestEnv,
	removeCloud: func(_ context.Context, ct names.CloudTag) error {
		if ct.Id() != "test" {
			return errors.E("bad cloud tag")
		}
		return nil
	},
	username:       "alice@external",
	cloud:          "test",
	controllerName: "controller-2",
	assertSuccess: func(c *qt.C, j *jimm.JIMM) {
		cloud := dbmodel.Cloud{
			Name: "test",
		}
		err := j.Database.GetCloud(context.Background(), &cloud)
		c.Assert(err, qt.Equals, nil)
		for _, cr := range cloud.Regions {
			for _, crp := range cr.Controllers {
				c.Assert(crp.Controller.Name, qt.Not(qt.Equals), "controller-2")
			}
		}
	},
}, {
	name: "Success - the only controller for the cloud",
	env:  removeCloudFromControllerTestEnv,
	removeCloud: func(_ context.Context, ct names.CloudTag) error {
		if ct.Id() != "test-cloud-2" {
			return errors.E("bad cloud tag")
		}
		return nil
	},
	username:       "alice@external",
	cloud:          "test-cloud-2",
	controllerName: "controller-2",
	assertSuccess: func(c *qt.C, j *jimm.JIMM) {
		cloud := dbmodel.Cloud{
			Name: "test-cloud-2",
		}
		err := j.Database.GetCloud(context.Background(), &cloud)
		c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeNotFound)
	},
}, {
	name:            "UserNotAutfhorized",
	env:             removeCloudFromControllerTestEnv,
	username:        "bob@external",
	cloud:           "test",
	controllerName:  "controller-2",
	expectError:     `unauthorized`,
	expectErrorCode: errors.CodeUnauthorized,
}, {
	name:           "DialError",
	env:            removeCloudFromControllerTestEnv,
	dialError:      errors.E("test dial error"),
	username:       "alice@external",
	cloud:          "test",
	controllerName: "controller-2",
	expectError:    `test dial error`,
}, {
	name: "APIError",
	env:  removeCloudFromControllerTestEnv,
	removeCloud: func(_ context.Context, mt names.CloudTag) error {
		return errors.E("test error")
	},
	username:       "alice@external",
	cloud:          "test",
	controllerName: "controller-2",
	expectError:    `test error`,
}}

func TestRemoveFromControllerCloud(t *testing.T) {
	c := qt.New(t)

	for _, test := range removeCloudFromControllerTests {
		c.Run(test.name, func(c *qt.C) {
			ctx := context.Background()

			env := jimmtest.ParseEnvironment(c, test.env)
			dialer := &jimmtest.Dialer{
				API: &jimmtest.API{
					RemoveCloud_: test.removeCloud,
				},
				Err: test.dialError,
			}
			j := &jimm.JIMM{
				Database: db.Database{
					DB: jimmtest.MemoryDB(c, nil),
				},
				Dialer: dialer,
			}
			err := j.Database.Migrate(ctx, false)
			c.Assert(err, qt.IsNil)
			env.PopulateDB(c, j.Database)

			u := env.User(test.username).DBObject(c, j.Database)

			err = j.RemoveCloudFromController(ctx, &u, test.controllerName, names.NewCloudTag(test.cloud))
			c.Assert(dialer.IsClosed(), qt.Equals, true)
			if test.expectError != "" {
				c.Check(err, qt.ErrorMatches, test.expectError)
				if test.expectErrorCode != "" {
					c.Check(errors.ErrorCode(err), qt.Equals, test.expectErrorCode)
				}
				return
			}
			c.Assert(err, qt.IsNil)
			test.assertSuccess(c, j)
		})
	}
}
