// Copyright 2020 Canonical Ltd.

package db_test

import (
	"context"
	"testing"

	qt "github.com/frankban/quicktest"
	"github.com/google/go-cmp/cmp/cmpopts"
	"gorm.io/gorm"

	"github.com/canonical/jimm/internal/auth"
	"github.com/canonical/jimm/internal/db"
	"github.com/canonical/jimm/internal/dbmodel"
	"github.com/canonical/jimm/internal/errors"
	"github.com/canonical/jimm/internal/jimmtest"
)

func TestAddCloudUnconfiguredDatabase(t *testing.T) {
	c := qt.New(t)

	var d db.Database
	err := d.AddCloud(context.Background(), &dbmodel.Cloud{})
	c.Check(err, qt.ErrorMatches, `database not configured`)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeServerConfiguration)
}

func (s *dbSuite) TestAddCloud(c *qt.C) {
	ctx := context.Background()

	cl := dbmodel.Cloud{
		Name:             "test-cloud",
		Type:             "test-provider",
		Endpoint:         "https://example.com",
		IdentityEndpoint: "https://identity.example.com",
		StorageEndpoint:  "https://storage.example.com",
		Regions: []dbmodel.CloudRegion{{
			Name: "test-cloud-region",
		}},
		CACertificates: dbmodel.Strings{"CACERT 1", "CACERT 2"},
		Users: []dbmodel.UserCloudAccess{{
			User: dbmodel.User{
				Username:    auth.Everyone,
				DisplayName: "everyone",
			},
			Access: "add-model",
		}},
	}

	err := s.Database.AddCloud(ctx, &cl)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeUpgradeInProgress)

	err = s.Database.Migrate(context.Background(), false)
	c.Assert(err, qt.IsNil)

	err = s.Database.AddCloud(ctx, &cl)
	c.Assert(err, qt.IsNil)

	cl2 := dbmodel.Cloud{
		Name: cl.Name,
	}
	err = s.Database.AddCloud(ctx, &cl2)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeAlreadyExists)

	cl3 := dbmodel.Cloud{
		Name: cl.Name,
	}

	err = s.Database.GetCloud(ctx, &cl3)
	c.Assert(err, qt.IsNil)
	c.Check(cl3, qt.CmpEquals(cmpopts.EquateEmpty()), cl)
}

func TestGetCloudUnconfiguredDatabase(t *testing.T) {
	c := qt.New(t)

	var d db.Database
	err := d.GetCloud(context.Background(), &dbmodel.Cloud{})
	c.Check(err, qt.ErrorMatches, `database not configured`)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeServerConfiguration)
}

func (s *dbSuite) TestGetCloud(c *qt.C) {
	ctx := context.Background()

	cl := dbmodel.Cloud{
		Name: "test-cloud",
	}
	err := s.Database.GetCloud(ctx, &cl)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeUpgradeInProgress)

	err = s.Database.Migrate(context.Background(), false)
	c.Assert(err, qt.IsNil)

	err = s.Database.GetCloud(ctx, &cl)
	c.Check(err, qt.ErrorMatches, `cloud "test-cloud" not found`)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeNotFound)

	cl2 := dbmodel.Cloud{
		Name:             "test-cloud",
		Type:             "test-provider",
		Endpoint:         "https://example.com",
		IdentityEndpoint: "https://identity.example.com",
		StorageEndpoint:  "https://storage.example.com",
		Regions: []dbmodel.CloudRegion{{
			Name: "test-cloud-region",
		}},
		CACertificates: dbmodel.Strings{"CACERT 1", "CACERT 2"},
		Users: []dbmodel.UserCloudAccess{{
			User: dbmodel.User{
				Username:    auth.Everyone,
				DisplayName: "everyone",
			},
			Access: "add-model",
		}},
	}

	err = s.Database.AddCloud(ctx, &cl2)
	c.Assert(err, qt.IsNil)

	err = s.Database.GetCloud(ctx, &cl)
	c.Assert(err, qt.IsNil)
	c.Check(cl, qt.CmpEquals(cmpopts.EquateEmpty()), cl2)
}
func TestGetCloudsUnconfiguredDatabase(t *testing.T) {
	c := qt.New(t)

	var d db.Database
	_, err := d.GetClouds(context.Background())
	c.Check(err, qt.ErrorMatches, `database not configured`)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeServerConfiguration)
}

func (s *dbSuite) TestGetClouds(c *qt.C) {
	ctx := context.Background()

	_, err := s.Database.GetClouds(ctx)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeUpgradeInProgress)

	err = s.Database.Migrate(context.Background(), false)
	c.Assert(err, qt.IsNil)

	clouds, err := s.Database.GetClouds(ctx)
	c.Assert(err, qt.IsNil)
	c.Check(clouds, qt.HasLen, 0)

	cl := dbmodel.Cloud{
		Name:             "test-cloud",
		Type:             "test-provider",
		Endpoint:         "https://example.com",
		IdentityEndpoint: "https://identity.example.com",
		StorageEndpoint:  "https://storage.example.com",
		Regions: []dbmodel.CloudRegion{{
			Name: "test-cloud-region",
		}},
		CACertificates: dbmodel.Strings{"CACERT 1", "CACERT 2"},
		Users: []dbmodel.UserCloudAccess{{
			User: dbmodel.User{
				Username:    auth.Everyone,
				DisplayName: "everyone",
			},
			Access: "add-model",
		}},
	}

	err = s.Database.AddCloud(ctx, &cl)
	c.Assert(err, qt.IsNil)

	clouds, err = s.Database.GetClouds(ctx)
	c.Assert(err, qt.IsNil)
	c.Check(clouds, qt.CmpEquals(cmpopts.EquateEmpty()), []dbmodel.Cloud{cl})
}

func TestUpdateCloudUnconfiguredDatabase(t *testing.T) {
	c := qt.New(t)

	var d db.Database
	err := d.UpdateCloud(context.Background(), &dbmodel.Cloud{})
	c.Check(err, qt.ErrorMatches, `database not configured`)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeServerConfiguration)
}

func (s *dbSuite) TestUpdateCloud(c *qt.C) {
	ctx := context.Background()

	cl := dbmodel.Cloud{
		Name:             "test-cloud",
		Type:             "test-provider",
		Endpoint:         "https://example.com",
		IdentityEndpoint: "https://identity.example.com",
		StorageEndpoint:  "https://storage.example.com",
		Regions: []dbmodel.CloudRegion{{
			Name: "test-cloud-region",
		}},
		CACertificates: dbmodel.Strings{"CACERT 1", "CACERT 2"},
		Users: []dbmodel.UserCloudAccess{{
			User: dbmodel.User{
				Username:    auth.Everyone,
				DisplayName: "everyone",
			},
			Access: "add-model",
		}, {
			User: dbmodel.User{
				Username:    "alice@external",
				DisplayName: "Alice",
			},
			Access: "add-model",
		}},
	}

	err := s.Database.UpdateCloud(ctx, &cl)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeUpgradeInProgress)

	err = s.Database.Migrate(context.Background(), false)
	c.Assert(err, qt.IsNil)

	err = s.Database.UpdateCloud(ctx, &cl)
	c.Assert(err, qt.IsNil)

	cl2 := dbmodel.Cloud{
		Name: "test-cloud",
	}

	err = s.Database.GetCloud(ctx, &cl2)
	c.Assert(err, qt.IsNil)
	c.Check(cl2, jimmtest.DBObjectEquals, cl)

	cl2.Endpoint = "https://new.example.com"
	cl2.IdentityEndpoint = "https://new.identity.example.com"
	cl2.StorageEndpoint = "https://new.storage.example.com"
	cl2.Regions = append(cl2.Regions, dbmodel.CloudRegion{
		Name:             "test-cloud-region-2",
		Endpoint:         "https://new.region.example.com",
		IdentityEndpoint: "https://new.region.identity.example.com",
		StorageEndpoint:  "https://new.region.storage.example.com",
	})

	err = s.Database.UpdateCloud(ctx, &cl2)
	c.Assert(err, qt.IsNil)

	cl3 := dbmodel.Cloud{
		Name: "test-cloud",
	}

	err = s.Database.GetCloud(ctx, &cl3)
	c.Assert(err, qt.IsNil)
	c.Check(cl3, jimmtest.DBObjectEquals, cl2)
}

func TestFindRegionUnconfiguredDatabase(t *testing.T) {
	c := qt.New(t)

	var d db.Database
	cr, err := d.FindRegion(context.Background(), "", "")
	c.Check(err, qt.ErrorMatches, `database not configured`)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeServerConfiguration)
	c.Check(cr, qt.IsNil)
}

func (s *dbSuite) TestFindRegion(c *qt.C) {
	ctx := context.Background()

	err := s.Database.Migrate(ctx, false)
	c.Assert(err, qt.IsNil)

	env := jimmtest.ParseEnvironment(c, `clouds:
- name: test
  type: testp
  regions:
  - name: test-region
controllers:
- name: test
  uuid: 00000001-0000-0000-0000-000000000001
  cloud: test
  region: test-region
  cloud-regions:
  - cloud: test
    region: test-region
    priority: 1
`)
	env.PopulateDB(c, *s.Database)

	cr, err := s.Database.FindRegion(ctx, "testp", "test-region")
	c.Assert(err, qt.IsNil)
	c.Check(cr, jimmtest.DBObjectEquals, &dbmodel.CloudRegion{
		Cloud: dbmodel.Cloud{
			Name: "test",
			Type: "testp",
		},
		Name: "test-region",
		Controllers: []dbmodel.CloudRegionControllerPriority{{
			Controller: dbmodel.Controller{
				Name:        "test",
				UUID:        "00000001-0000-0000-0000-000000000001",
				CloudName:   "test",
				CloudRegion: "test-region",
			},
			Priority: 1,
		}},
	})

	_, err = s.Database.FindRegion(ctx, "no-such", "region")
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeNotFound)
}

func TestUpdateUserCloudAccessUnconfiguredDatabase(t *testing.T) {
	c := qt.New(t)

	var d db.Database
	err := d.UpdateUserCloudAccess(context.Background(), nil)
	c.Check(err, qt.ErrorMatches, `database not configured`)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeServerConfiguration)
}

const testUpdateUserCloudAccessEnv = `clouds:
- name: test
  type: test
  regions:
  - name: test-region
- name: test-hosted
  type: kubernetes
  host-cloud-region: test/test-region
  regions:
  - name: default
  users:
  - user: alice@external
    access: admin
  - user: bob@external
    access: add-model
controllers:
- name: test
  uuid: 00000001-0000-0000-0000-000000000001
  cloud: test
  region: test-region
  cloud-regions:
  - cloud: test
    region: test-region
    priority: 10
  - cloud: test-hosted
    region: default
    priority: 1
`

func (s *dbSuite) TestUpdateUserCloudAccess(c *qt.C) {
	ctx := context.Background()
	err := s.Database.Migrate(context.Background(), true)
	c.Assert(err, qt.Equals, nil)

	env := jimmtest.ParseEnvironment(c, testUpdateUserCloudAccessEnv)
	env.PopulateDB(c, *s.Database)

	cld := dbmodel.Cloud{
		Name: "test-hosted",
	}
	err = s.Database.GetCloud(ctx, &cld)
	c.Assert(err, qt.IsNil)

	charlie := env.User("charlie@external").DBObject(c, *s.Database)

	// Add a new user
	err = s.Database.UpdateUserCloudAccess(ctx, &dbmodel.UserCloudAccess{
		User:   charlie,
		Cloud:  cld,
		Access: "add-model",
	})
	c.Assert(err, qt.Equals, nil)
	err = s.Database.GetCloud(ctx, &cld)
	c.Assert(err, qt.IsNil)
	c.Check(cld.Users, jimmtest.DBObjectEquals, []dbmodel.UserCloudAccess{{
		Username: "alice@external",
		User: dbmodel.User{
			Username:         "alice@external",
			ControllerAccess: "login",
		},
		CloudName: "test-hosted",
		Access:    "admin",
	}, {
		Username: "bob@external",
		User: dbmodel.User{
			Username:         "bob@external",
			ControllerAccess: "login",
		},
		CloudName: "test-hosted",
		Access:    "add-model",
	}, {
		Username: "charlie@external",
		User: dbmodel.User{
			Username:         "charlie@external",
			ControllerAccess: "login",
		},
		CloudName: "test-hosted",
		Access:    "add-model",
	}})

	// Update an existing user
	uca := cld.Users[1]
	uca.Access = "admin"
	err = s.Database.UpdateUserCloudAccess(ctx, &uca)
	c.Assert(err, qt.Equals, nil)
	err = s.Database.GetCloud(ctx, &cld)
	c.Assert(err, qt.IsNil)
	c.Check(cld.Users, jimmtest.DBObjectEquals, []dbmodel.UserCloudAccess{{
		Username: "alice@external",
		User: dbmodel.User{
			Username:         "alice@external",
			ControllerAccess: "login",
		},
		CloudName: "test-hosted",
		Access:    "admin",
	}, {
		Username: "bob@external",
		User: dbmodel.User{
			Username:         "bob@external",
			ControllerAccess: "login",
		},
		CloudName: "test-hosted",
		Access:    "admin",
	}, {
		Username: "charlie@external",
		User: dbmodel.User{
			Username:         "charlie@external",
			ControllerAccess: "login",
		},
		CloudName: "test-hosted",
		Access:    "add-model",
	}})

	// Remove a user
	uca = cld.Users[1]
	uca.Access = ""
	err = s.Database.UpdateUserCloudAccess(ctx, &uca)
	c.Assert(err, qt.Equals, nil)
	err = s.Database.GetCloud(ctx, &cld)
	c.Assert(err, qt.IsNil)
	c.Check(cld.Users, jimmtest.DBObjectEquals, []dbmodel.UserCloudAccess{{
		Username: "alice@external",
		User: dbmodel.User{
			Username:         "alice@external",
			ControllerAccess: "login",
		},
		CloudName: "test-hosted",
		Access:    "admin",
	}, {
		Username: "charlie@external",
		User: dbmodel.User{
			Username:         "charlie@external",
			ControllerAccess: "login",
		},
		CloudName: "test-hosted",
		Access:    "add-model",
	}})
}

func TestDeleteCloudUnconfiguredDatabase(t *testing.T) {
	c := qt.New(t)

	var d db.Database
	err := d.DeleteCloud(context.Background(), nil)
	c.Check(err, qt.ErrorMatches, `database not configured`)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeServerConfiguration)
}

func (s *dbSuite) TestDeleteCloud(c *qt.C) {
	ctx := context.Background()

	cl := dbmodel.Cloud{
		Name:             "test-cloud",
		Type:             "test-provider",
		Endpoint:         "https://example.com",
		IdentityEndpoint: "https://identity.example.com",
		StorageEndpoint:  "https://storage.example.com",
		Regions: []dbmodel.CloudRegion{{
			Name: "test-cloud-region",
		}},
		CACertificates: dbmodel.Strings{"CACERT 1", "CACERT 2"},
		Users: []dbmodel.UserCloudAccess{{
			User: dbmodel.User{
				Username:    auth.Everyone,
				DisplayName: "everyone",
			},
			Access: "add-model",
		}},
	}

	err := s.Database.DeleteCloud(ctx, &cl)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeUpgradeInProgress)

	err = s.Database.Migrate(context.Background(), false)
	c.Assert(err, qt.IsNil)

	err = s.Database.AddCloud(ctx, &cl)
	c.Assert(err, qt.IsNil)

	err = s.Database.DeleteCloud(ctx, &cl)
	c.Assert(err, qt.IsNil)

	cl2 := dbmodel.Cloud{
		Name: cl.Name,
	}
	err = s.Database.GetCloud(ctx, &cl2)
	c.Check(err, qt.ErrorMatches, `cloud "test-cloud" not found`)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeNotFound)
}

func TestDeleteCloudRegionControllerPriorityUnconfiguredDatabase(t *testing.T) {
	c := qt.New(t)

	var d db.Database
	err := d.DeleteCloudRegionControllerPriority(context.Background(), nil)
	c.Check(err, qt.ErrorMatches, `database not configured`)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeServerConfiguration)
}

func (s *dbSuite) TestDeleteCloudRegionControllerPriority(c *qt.C) {
	ctx := context.Background()

	crp := dbmodel.CloudRegionControllerPriority{
		Model: gorm.Model{
			ID: 1,
		},
	}

	err := s.Database.DeleteCloudRegionControllerPriority(ctx, &crp)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeUpgradeInProgress)

	err = s.Database.Migrate(context.Background(), false)
	c.Assert(err, qt.IsNil)

	env := jimmtest.ParseEnvironment(c, `clouds:
- name: test-controller-cloud
  type: testp
  regions:
  - name: default
- name: test-cloud-1
  type: testp
  regions:
  - name: test-region-1
- name: test-cloud-2
  type: testp
  regions:
  - name: test-region-2
controllers:
- name: test
  uuid: 00000001-0000-0000-0000-000000000001
  cloud: test-controller-cloud
  region: default
  cloud-regions:
  - cloud: test-cloud-1
    region: test-region-1
    priority: 1
  - cloud: test-cloud-2
    region: test-region-2
    priority: 1
`)
	env.PopulateDB(c, *s.Database)

	cl := dbmodel.Cloud{
		Name: "test-cloud-1",
	}
	err = s.Database.GetCloud(ctx, &cl)

	crp = cl.Regions[0].Controllers[0]

	err = s.Database.DeleteCloudRegionControllerPriority(ctx, &crp)
	c.Assert(err, qt.IsNil)

	cl2 := dbmodel.Cloud{
		Name: cl.Name,
	}
	err = s.Database.GetCloud(ctx, &cl2)
	c.Assert(err, qt.Equals, nil)

	for _, cr := range cl2.Regions {
		for _, controllerPriority := range cr.Controllers {
			c.Assert(controllerPriority.ID, qt.Not(qt.Equals), crp.ID)
		}
	}
}
