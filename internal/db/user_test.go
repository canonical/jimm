// Copyright 2020 Canonical Ltd.

package db_test

import (
	"context"
	"database/sql"
	"testing"

	qt "github.com/frankban/quicktest"
	"github.com/google/go-cmp/cmp/cmpopts"
	"gorm.io/gorm"

	"github.com/CanonicalLtd/jimm/internal/auth"
	"github.com/CanonicalLtd/jimm/internal/db"
	"github.com/CanonicalLtd/jimm/internal/dbmodel"
	"github.com/CanonicalLtd/jimm/internal/errors"
	"github.com/CanonicalLtd/jimm/internal/jimmtest"
)

func TestGetUserUnconfiguredDatabase(t *testing.T) {
	c := qt.New(t)

	var d db.Database
	err := d.GetUser(context.Background(), &dbmodel.User{})
	c.Check(err, qt.ErrorMatches, `database not configured`)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeServerConfiguration)
}

func (s *dbSuite) TestGetUser(c *qt.C) {
	ctx := context.Background()
	err := s.Database.GetUser(ctx, &dbmodel.User{})
	c.Check(err, qt.ErrorMatches, `upgrade in progress`)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeUpgradeInProgress)

	err = s.Database.Migrate(ctx, false)
	c.Assert(err, qt.IsNil)

	err = s.Database.GetUser(ctx, &dbmodel.User{})
	c.Check(err, qt.ErrorMatches, `invalid username ""`)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeNotFound)

	u := dbmodel.User{
		Username: "bob@external",
	}
	err = s.Database.GetUser(ctx, &u)
	c.Assert(err, qt.IsNil)
	c.Check(u.ControllerAccess, qt.Equals, "login")

	u2 := dbmodel.User{
		Username: u.Username,
	}
	err = s.Database.GetUser(ctx, &u2)
	c.Assert(err, qt.IsNil)
	c.Check(u2, qt.DeepEquals, u)
}

func TestUpdateUserUnconfiguredDatabase(t *testing.T) {
	c := qt.New(t)

	var d db.Database
	err := d.UpdateUser(context.Background(), &dbmodel.User{})
	c.Check(err, qt.ErrorMatches, `database not configured`)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeServerConfiguration)
}

func (s *dbSuite) TestUpdateUser(c *qt.C) {
	ctx := context.Background()
	err := s.Database.UpdateUser(ctx, &dbmodel.User{})
	c.Check(err, qt.ErrorMatches, `upgrade in progress`)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeUpgradeInProgress)

	err = s.Database.Migrate(ctx, false)
	c.Assert(err, qt.IsNil)

	err = s.Database.UpdateUser(ctx, &dbmodel.User{})
	c.Check(err, qt.ErrorMatches, `invalid username ""`)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeNotFound)

	u := dbmodel.User{
		Username: "bob@external",
	}
	err = s.Database.GetUser(ctx, &u)
	c.Assert(err, qt.IsNil)
	c.Check(u.ControllerAccess, qt.Equals, "login")

	u.ControllerAccess = "superuser"
	u.Models = []dbmodel.UserModelAccess{{
		Model_: dbmodel.Model{
			Name:  "model-1",
			Owner: u,
			UUID: sql.NullString{
				String: "00000001-0000-0000-0000-0000-00000000001",
				Valid:  true,
			},
		},
		Access: "admin",
	}}
	err = s.Database.UpdateUser(ctx, &u)
	c.Assert(err, qt.IsNil)

	u.Models = nil

	u2 := dbmodel.User{
		Username: u.Username,
	}
	err = s.Database.GetUser(ctx, &u2)
	c.Assert(err, qt.IsNil)
	c.Check(u2, qt.DeepEquals, u)
}

func TestGetUserCloudsUnconfiguredDatabase(t *testing.T) {
	c := qt.New(t)

	var d db.Database
	_, err := d.GetUserClouds(context.Background(), &dbmodel.User{})
	c.Check(err, qt.ErrorMatches, `database not configured`)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeServerConfiguration)
}

func (s *dbSuite) TestGetUserClouds(c *qt.C) {
	ctx := context.Background()

	u := dbmodel.User{
		Username:    auth.Everyone,
		DisplayName: "everyone",
	}

	_, err := s.Database.GetUserClouds(ctx, &u)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeUpgradeInProgress)

	err = s.Database.Migrate(context.Background(), false)
	c.Assert(err, qt.IsNil)

	clouds, err := s.Database.GetUserClouds(ctx, &u)
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
			User:   u,
			Access: "add-model",
		}},
	}

	err = s.Database.AddCloud(ctx, &cl)
	c.Assert(err, qt.IsNil)

	clouds, err = s.Database.GetUserClouds(ctx, &u)
	c.Assert(err, qt.IsNil)
	c.Check(clouds, qt.CmpEquals(cmpopts.EquateEmpty(), cmpopts.IgnoreTypes(gorm.Model{})), []dbmodel.UserCloudAccess{{
		Username:  auth.Everyone,
		CloudName: "test-cloud",
		Cloud:     cl,
		Access:    "add-model",
	}})
}

func TestGetUserCloudCredentialsUnconfiguredDatabase(t *testing.T) {
	c := qt.New(t)

	var d db.Database
	_, err := d.GetUserCloudCredentials(context.Background(), &dbmodel.User{}, "")
	c.Check(err, qt.ErrorMatches, `database not configured`)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeServerConfiguration)
}

func (s *dbSuite) TestGetUserCloudCredentials(c *qt.C) {
	ctx := context.Background()

	err := s.Database.Migrate(ctx, false)
	c.Assert(err, qt.IsNil)

	_, err = s.Database.GetUserCloudCredentials(ctx, &dbmodel.User{}, "")
	c.Check(err, qt.ErrorMatches, `cloudcredential not found`)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeNotFound)

	_, err = s.Database.GetUserCloudCredentials(ctx, &dbmodel.User{
		Username: "test",
	}, "ec2")
	c.Check(err, qt.IsNil)

	u := dbmodel.User{
		Username: "bob@external",
	}
	c.Assert(s.Database.DB.Create(&u).Error, qt.IsNil)

	cloud := dbmodel.Cloud{
		Name: "test-cloud",
		Type: "test-provider",
		Regions: []dbmodel.CloudRegion{{
			Name: "test-region",
		}},
	}
	c.Assert(s.Database.DB.Create(&cloud).Error, qt.IsNil)

	cred1 := dbmodel.CloudCredential{
		Name:          "test-cred-1",
		CloudName:     cloud.Name,
		OwnerUsername: u.Username,
		AuthType:      "empty",
	}
	err = s.Database.SetCloudCredential(context.Background(), &cred1)
	c.Assert(err, qt.Equals, nil)

	cred2 := dbmodel.CloudCredential{
		Name:          "test-cred-2",
		CloudName:     cloud.Name,
		OwnerUsername: u.Username,
		AuthType:      "empty",
	}
	err = s.Database.SetCloudCredential(context.Background(), &cred2)
	c.Assert(err, qt.Equals, nil)

	credentials, err := s.Database.GetUserCloudCredentials(ctx, &u, cloud.Name)
	c.Check(err, qt.IsNil)
	c.Assert(credentials, qt.DeepEquals, []dbmodel.CloudCredential{cred1, cred2})
}

func TestGetUserModelsUnconfiguredDatabase(t *testing.T) {
	c := qt.New(t)

	var d db.Database
	_, err := d.GetUserModels(context.Background(), &dbmodel.User{})
	c.Check(err, qt.ErrorMatches, `database not configured`)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeServerConfiguration)
}

func (s *dbSuite) TestGetUserModels(c *qt.C) {
	ctx := context.Background()

	err := s.Database.Migrate(ctx, false)
	c.Assert(err, qt.IsNil)

	env := jimmtest.ParseEnvironment(c, `clouds:
- name: test
  type: test
  regions:
  - name: test-region
cloud-credentials:
- name: test
  owner: alice@external
  cloud: test
  type: empty
controllers:
- name: test
  uuid: 00000001-0000-0000-0000-000000000001
  cloud: test
  region: test-region
models:
- name: test-1
  owner: alice@external
  uuid: 00000002-0000-0000-0000-000000000001
  controller: test
  cloud: test
  region: test-region
  cloud-credential: test
  users:
  - user: alice@external
    access: "admin"
  - user: bob@external
    access: "write"
- name: test-2
  owner: alice@external
  uuid: 00000002-0000-0000-0000-000000000002
  controller: test
  cloud: test
  region: test-region
  cloud-credential: test
  users:
  - user: alice@external
    access: "admin"
- name: test-3
  owner: alice@external
  uuid: 00000002-0000-0000-0000-000000000003
  controller: test
  cloud: test
  region: test-region
  cloud-credential: test
  users:
  - user: alice@external
    access: "admin"
  - user: bob@external
    access: "read"
`)
	env.PopulateDB(c, *s.Database, nil)

	u := env.User("bob@external").DBObject(c, *s.Database, nil)
	models, err := s.Database.GetUserModels(ctx, &u)
	c.Assert(err, qt.IsNil)
	c.Check(models, jimmtest.DBObjectEquals, []dbmodel.UserModelAccess{{
		Model_: dbmodel.Model{
			Name: "test-1",
			UUID: sql.NullString{
				String: "00000002-0000-0000-0000-000000000001",
				Valid:  true,
			},
			Owner: dbmodel.User{
				Username:         "alice@external",
				ControllerAccess: "login",
			},
			Controller: dbmodel.Controller{
				Name:        "test",
				UUID:        "00000001-0000-0000-0000-000000000001",
				CloudName:   "test",
				CloudRegion: "test-region",
			},
			CloudRegion: dbmodel.CloudRegion{
				Cloud: dbmodel.Cloud{
					Name: "test",
					Type: "test",
				},
				Name: "test-region",
			},
			CloudCredential: dbmodel.CloudCredential{
				Name: "test",
			},
			Users: []dbmodel.UserModelAccess{{
				User: dbmodel.User{
					Username:         "alice@external",
					ControllerAccess: "login",
				},
				Access: "admin",
			}, {
				User: dbmodel.User{
					Username:         "bob@external",
					ControllerAccess: "login",
				},
				Access: "write",
			}},
		},
		Access: "write",
	}, {
		Model_: dbmodel.Model{
			Name: "test-3",
			UUID: sql.NullString{
				String: "00000002-0000-0000-0000-000000000003",
				Valid:  true,
			},
			Owner: dbmodel.User{
				Username:         "alice@external",
				ControllerAccess: "login",
			},
			Controller: dbmodel.Controller{
				Name:        "test",
				UUID:        "00000001-0000-0000-0000-000000000001",
				CloudName:   "test",
				CloudRegion: "test-region",
			},
			CloudRegion: dbmodel.CloudRegion{
				Cloud: dbmodel.Cloud{
					Name: "test",
					Type: "test",
				},
				Name: "test-region",
			},
			CloudCredential: dbmodel.CloudCredential{
				Name: "test",
			},
			Users: []dbmodel.UserModelAccess{{
				User: dbmodel.User{
					Username:         "alice@external",
					ControllerAccess: "login",
				},
				Access: "admin",
			}, {
				User: dbmodel.User{
					Username:         "bob@external",
					ControllerAccess: "login",
				},
				Access: "read",
			}},
		},
		Access: "read",
	}})
}
