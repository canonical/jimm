// Copyright 2026 Canonical.

package controllerprofile_test

import (
	"context"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"

	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/jimm/controllerprofile"
	"github.com/canonical/jimm/v3/internal/testutils/testdb"
)

type controllerProfileTestEnv struct {
	manager *controllerprofile.ControllerProfileManager
	db      *db.Database
}

func testControllerProfile(name string) dbmodel.ControllerProfile {
	return dbmodel.ControllerProfile{
		Name:        name,
		Description: "Reusable profile",
		JujuVersion: "3.6",
		Cloud: dbmodel.ControllerProfileCloud{
			Name:           "aws",
			Type:           "ec2",
			AuthTypes:      dbmodel.Strings{"access-key"},
			CACertificates: dbmodel.Strings{"ca-cert"},
			Config:         dbmodel.Map{"default-base": "ubuntu@24.04"},
			Endpoint:       "https://aws.example.com",
			Region: dbmodel.ControllerProfileCloudRegion{
				Name:             "eu-west-1",
				Endpoint:         "https://region.example.com",
				IdentityEndpoint: "https://identity.example.com",
				StorageEndpoint:  "https://storage.example.com",
			},
		},
		BootstrapOptions: dbmodel.ControllerProfileBootstrapOptions{
			BootstrapBase:         "ubuntu@24.04",
			BootstrapConstraints:  dbmodel.StringMap{"mem": "8G"},
			ModelConstraints:      dbmodel.StringMap{"arch": "amd64"},
			ModelDefault:          dbmodel.StringMap{"logging-config": "<root>=INFO"},
			BootstrapConfig:       dbmodel.StringMap{"bootstrap-timeout": "20m"},
			ControllerConfig:      dbmodel.StringMap{"audit-log-enabled": "true"},
			ControllerModelConfig: dbmodel.StringMap{"logging-config": "<root>=INFO"},
			StoragePool: dbmodel.ControllerProfileStoragePool{
				Name:       "controller-pool",
				Type:       "ebs",
				Attributes: dbmodel.StringMap{"volume-type": "gp3"},
			},
		},
	}
}

func setupControllerProfileTestEnv(c *qt.C) *controllerProfileTestEnv {
	database := &db.Database{DB: testdb.PostgresDB(c, time.Now)}
	err := database.Migrate(context.Background())
	c.Assert(err, qt.IsNil)

	manager, err := controllerprofile.NewControllerProfileManager(database)
	c.Assert(err, qt.IsNil)

	return &controllerProfileTestEnv{
		manager: manager,
		db:      database,
	}
}

func TestSaveControllerProfile(t *testing.T) {
	c := qt.New(t)
	t.Parallel()
	env := setupControllerProfileTestEnv(c)
	ctx := context.Background()

	profile := testControllerProfile("profile-a")
	err := env.manager.SaveControllerProfile(ctx, &profile)
	c.Assert(err, qt.IsNil)
	c.Assert(profile.Version, qt.Equals, uint(1))

	lookup := dbmodel.ControllerProfile{Name: profile.Name}
	err = env.db.GetControllerProfile(ctx, &lookup)
	c.Assert(err, qt.IsNil)
	c.Assert(lookup.Name, qt.Equals, profile.Name)
	c.Assert(lookup.Version, qt.Equals, uint(1))
	c.Assert(lookup.Cloud.Name, qt.Equals, "aws")
}

func TestGetControllerProfile(t *testing.T) {
	c := qt.New(t)
	t.Parallel()
	env := setupControllerProfileTestEnv(c)
	ctx := context.Background()

	profile := testControllerProfile("profile-b")
	err := env.db.CreateOrReplaceControllerProfile(ctx, &profile)
	c.Assert(err, qt.IsNil)

	loaded, err := env.manager.GetControllerProfile(ctx, profile.Name)
	c.Assert(err, qt.IsNil)
	c.Assert(loaded.Name, qt.Equals, profile.Name)
	c.Assert(loaded.Description, qt.Equals, profile.Description)
	c.Assert(loaded.BootstrapOptions.BootstrapBase, qt.Equals, profile.BootstrapOptions.BootstrapBase)
}

func TestListControllerProfiles(t *testing.T) {
	c := qt.New(t)
	t.Parallel()
	env := setupControllerProfileTestEnv(c)
	ctx := context.Background()

	for _, name := range []string{"profile-a", "profile-b"} {
		profile := testControllerProfile(name)
		err := env.db.CreateOrReplaceControllerProfile(ctx, &profile)
		c.Assert(err, qt.IsNil)
	}

	profiles, err := env.manager.ListControllerProfiles(ctx, "")
	c.Assert(err, qt.IsNil)
	c.Assert(profiles, qt.HasLen, 2)
	c.Assert(profiles[0].Name, qt.Equals, "profile-a")
	c.Assert(profiles[1].Name, qt.Equals, "profile-b")
}

func TestListControllerProfilesFiltersByJujuVersion(t *testing.T) {
	c := qt.New(t)
	t.Parallel()
	env := setupControllerProfileTestEnv(c)
	ctx := context.Background()

	for _, tc := range []struct {
		name        string
		jujuVersion string
	}{
		{name: "profile-3", jujuVersion: "3"},
		{name: "profile-3-6", jujuVersion: "3.6"},
		{name: "profile-4", jujuVersion: "4"},
	} {
		profile := testControllerProfile(tc.name)
		profile.JujuVersion = tc.jujuVersion
		err := env.db.CreateOrReplaceControllerProfile(ctx, &profile)
		c.Assert(err, qt.IsNil)
	}

	profiles, err := env.manager.ListControllerProfiles(ctx, "3.6.4")
	c.Assert(err, qt.IsNil)
	c.Assert(profiles, qt.HasLen, 2)
	c.Assert(profiles[0].Name, qt.Equals, "profile-3")
	c.Assert(profiles[1].Name, qt.Equals, "profile-3-6")
}

func TestRemoveControllerProfile(t *testing.T) {
	c := qt.New(t)
	t.Parallel()
	env := setupControllerProfileTestEnv(c)
	ctx := context.Background()

	profile := testControllerProfile("profile-c")
	err := env.db.CreateOrReplaceControllerProfile(ctx, &profile)
	c.Assert(err, qt.IsNil)

	err = env.manager.RemoveControllerProfile(ctx, profile.Name)
	c.Assert(err, qt.IsNil)

	err = env.db.GetControllerProfile(ctx, &dbmodel.ControllerProfile{Name: profile.Name})
	c.Assert(errors.ErrorCode(err), qt.Equals, errors.CodeNotFound)
}
