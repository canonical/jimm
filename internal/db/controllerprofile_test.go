// Copyright 2026 Canonical.

package db_test

import (
	"context"

	qt "github.com/frankban/quicktest"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
)

func testControllerProfile() dbmodel.ControllerProfile {
	return dbmodel.ControllerProfile{
		Name:        "openstack-production",
		Description: "Reusable bootstrap settings for OpenStack production controllers",
		Cloud: dbmodel.ControllerProfileCloud{
			Name:            "my-private-cloud",
			Type:            "openstack",
			AuthTypes:       dbmodel.Strings{"access-key", "userpass"},
			CACertificates:  dbmodel.Strings{"cert-1", "cert-2"},
			Config:          dbmodel.Map{"default-series": "ubuntu@24.04", "skip-image-validation": true},
			Endpoint:        "https://private-cloud.internal",
			HostCloudRegion: "aws/us-east-1",
			Region: dbmodel.ControllerProfileCloudRegion{
				Name:             "dc-1",
				Endpoint:         "https://private-cloud.internal/dc-1",
				IdentityEndpoint: "https://identity.private-cloud.internal/dc-1",
				StorageEndpoint:  "https://storage.private-cloud.internal/dc-1",
			},
		},
		BootstrapOptions: dbmodel.ControllerProfileBootstrapOptions{
			BootstrapBase:        "ubuntu@24.04",
			BootstrapConstraints: dbmodel.StringMap{"mem": "8G", "cores": "2"},
			ModelConstraints:     dbmodel.StringMap{"arch": "amd64"},
			ModelDefault:         dbmodel.StringMap{"logging-config": "<root>=INFO"},
			StoragePool: dbmodel.ControllerProfileStoragePool{
				Name:       "controller-pool",
				Type:       "ebs",
				Attributes: dbmodel.StringMap{"volume-type": "gp3"},
			},
			BootstrapConfig:       dbmodel.StringMap{"bootstrap-timeout": "20m"},
			ControllerConfig:      dbmodel.StringMap{"audit-log-enabled": "true"},
			ControllerModelConfig: dbmodel.StringMap{"logging-config": "<root>=INFO"},
		},
	}
}

var controllerProfileEquals = qt.CmpEquals(
	cmpopts.EquateEmpty(),
	cmpopts.IgnoreFields(dbmodel.ControllerProfile{}, "ID", "CreatedAt", "UpdatedAt"),
)

func (s *dbSuite) TestControllerProfileCRUD(c *qt.C) {
	ctx := context.Background()
	c.Assert(s.Database.Migrate(ctx), qt.IsNil)

	profile := testControllerProfile()
	c.Assert(s.Database.CreateOrReplaceControllerProfile(ctx, &profile), qt.IsNil)
	c.Check(profile.Version, qt.Equals, uint(1))

	lookup := dbmodel.ControllerProfile{Name: profile.Name}
	c.Assert(s.Database.GetControllerProfile(ctx, &lookup), qt.IsNil)
	c.Check(lookup, controllerProfileEquals, profile)

	profile.Description = "Updated description"
	profile.Cloud.Endpoint = "https://private-cloud-2.internal"
	profile.BootstrapOptions.ControllerConfig = dbmodel.StringMap{"audit-log-enabled": "false", "audit-log-max-size": "100MB"}
	c.Assert(s.Database.CreateOrReplaceControllerProfile(ctx, &profile), qt.IsNil)
	c.Check(profile.Version, qt.Equals, uint(2))

	lookup = dbmodel.ControllerProfile{Name: profile.Name}
	c.Assert(s.Database.GetControllerProfile(ctx, &lookup), qt.IsNil)
	c.Check(lookup, controllerProfileEquals, profile)

	profiles, err := s.Database.ListControllerProfiles(ctx)
	c.Assert(err, qt.IsNil)
	c.Assert(profiles, qt.HasLen, 1)
	c.Check(profiles[0], controllerProfileEquals, profile)

	c.Assert(s.Database.RemoveControllerProfile(ctx, profile.Name), qt.IsNil)
	err = s.Database.GetControllerProfile(ctx, &dbmodel.ControllerProfile{Name: profile.Name})
	c.Assert(errors.ErrorCode(err), qt.Equals, errors.CodeNotFound)
}

func (s *dbSuite) TestCreateOrReplaceControllerProfileRequiresCurrentVersion(c *qt.C) {
	ctx := context.Background()
	c.Assert(s.Database.Migrate(ctx), qt.IsNil)

	profile := testControllerProfile()
	c.Assert(s.Database.CreateOrReplaceControllerProfile(ctx, &profile), qt.IsNil)

	current := dbmodel.ControllerProfile{Name: profile.Name}
	stale := dbmodel.ControllerProfile{Name: profile.Name}
	c.Assert(s.Database.GetControllerProfile(ctx, &current), qt.IsNil)
	c.Assert(s.Database.GetControllerProfile(ctx, &stale), qt.IsNil)

	current.Description = "fresh update"
	c.Assert(s.Database.CreateOrReplaceControllerProfile(ctx, &current), qt.IsNil)
	c.Check(current.Version, qt.Equals, uint(2))

	stale.Description = "stale update"
	err := s.Database.CreateOrReplaceControllerProfile(ctx, &stale)
	c.Assert(errors.ErrorCode(err), qt.Equals, errors.CodeBadRequest)
	c.Check(err, qt.ErrorMatches, `controller profile "openstack-production" version mismatch: expected 2, got 1`)

	lookup := dbmodel.ControllerProfile{Name: profile.Name}
	c.Assert(s.Database.GetControllerProfile(ctx, &lookup), qt.IsNil)
	c.Check(lookup.Description, qt.Equals, "fresh update")
	c.Check(lookup.Version, qt.Equals, uint(2))
}

func (s *dbSuite) TestControllerProfileNameMustBeUnique(c *qt.C) {
	ctx := context.Background()
	c.Assert(s.Database.Migrate(ctx), qt.IsNil)

	profile := testControllerProfile()
	c.Assert(s.Database.CreateOrReplaceControllerProfile(ctx, &profile), qt.IsNil)

	duplicate := testControllerProfile()
	duplicate.Version = 1
	err := s.Database.DB.WithContext(ctx).Create(&duplicate).Error
	c.Assert(err, qt.ErrorMatches, `.*duplicate key value violates unique constraint.*`)
}
