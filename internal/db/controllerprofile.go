// Copyright 2026 Canonical.

package db

import (
	"context"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/servermon"
)

func validateControllerProfile(profile *dbmodel.ControllerProfile) error {
	if profile.Name == "" {
		return errors.Codef(errors.CodeBadRequest, "controller profile name must be provided")
	}
	if profile.Cloud.Name == "" {
		return errors.Codef(errors.CodeBadRequest, "controller profile cloud name must be provided")
	}
	if profile.Cloud.Region.Name == "" {
		return errors.Codef(errors.CodeBadRequest, "controller profile cloud region name must be provided")
	}
	storagePool := profile.BootstrapOptions.StoragePool
	if (storagePool.Name == "") != (storagePool.Type == "") {
		return errors.Codef(errors.CodeBadRequest, "controller profile storage pool requires both name and type")
	}
	return nil
}

// CreateOrReplaceControllerProfile creates a new controller profile or replaces
// an existing one with the same name.
//
// New profiles must be created with Version set to 0. Replacing an existing
// profile requires Version to match the last value read by the caller.
func (d *Database) CreateOrReplaceControllerProfile(ctx context.Context, profile *dbmodel.ControllerProfile) (err error) {
	const op = "db.CreateOrReplaceControllerProfile"
	if err := d.ready(); err != nil {
		return err
	}
	if err := validateControllerProfile(profile); err != nil {
		return err
	}

	durationObserver := servermon.DurationObserver(servermon.DBQueryDurationHistogram, op)
	defer durationObserver()
	defer servermon.ErrorCounter(servermon.DBQueryErrorCount, &err, op)

	err = d.Transaction(func(d *Database) error {
		current := dbmodel.ControllerProfile{Name: profile.Name}
		err := d.ForUpdate().GetControllerProfile(ctx, &current)
		if err != nil {
			if errors.ErrorCode(err) != errors.CodeNotFound {
				return err
			}
			// When creating a new profile, Version must be 0. The database will assign Version 1 to the new profile.
			if profile.Version != 0 {
				return errors.Codef(errors.CodeBadRequest, "controller profile %q does not exist", profile.Name)
			}
			profile.Version = 1
			if err := d.DB.WithContext(ctx).Create(profile).Error; err != nil {
				return dbError(err)
			}
			return nil
		}

		if profile.Version != current.Version {
			return errors.Codef(
				errors.CodeBadRequest,
				"controller profile %q version mismatch: expected %d, got %d", profile.Name, current.Version, profile.Version,
			)
		}

		updated := *profile
		updated.ID = current.ID
		updated.CreatedAt = current.CreatedAt
		updated.Version = current.Version + 1
		if err := d.DB.WithContext(ctx).Save(&updated).Error; err != nil {
			return dbError(err)
		}
		*profile = updated
		return nil
	})
	if err != nil {
		return err
	}
	return nil
}

// GetControllerProfile fills profile using the stored profile with the same
// name.
func (d *Database) GetControllerProfile(ctx context.Context, profile *dbmodel.ControllerProfile) (err error) {
	const op = "db.GetControllerProfile"
	if err := d.ready(); err != nil {
		return err
	}
	if profile.Name == "" {
		return errors.Codef(errors.CodeBadRequest, "controller profile name must be provided")
	}

	durationObserver := servermon.DurationObserver(servermon.DBQueryDurationHistogram, op)
	defer durationObserver()
	defer servermon.ErrorCounter(servermon.DBQueryErrorCount, &err, op)

	db := d.DB.WithContext(ctx).Where("name = ?", profile.Name)
	if err := db.First(profile).Error; err != nil {
		err := dbError(err)
		if errors.ErrorCode(err) == errors.CodeNotFound {
			return errors.Codef(errors.CodeNotFound, "controller profile %q not found", profile.Name)
		}
		return err
	}
	return nil
}

// ListControllerProfiles retrieves all saved controller profiles ordered by
// name. When jujuVersion is provided, only profiles whose saved juju-version
// is a prefix of the requested version are returned.
func (d *Database) ListControllerProfiles(ctx context.Context, jujuVersion string) (_ []dbmodel.ControllerProfile, err error) {
	const op = "db.ListControllerProfiles"
	if err := d.ready(); err != nil {
		return nil, err
	}

	durationObserver := servermon.DurationObserver(servermon.DBQueryDurationHistogram, op)
	defer durationObserver()
	defer servermon.ErrorCounter(servermon.DBQueryErrorCount, &err, op)

	var profiles []dbmodel.ControllerProfile
	query := d.DB.WithContext(ctx).Order("name asc")
	if jujuVersion != "" {
		query = query.Where("juju_version IN ?", dbmodel.PartialJujuVersionPrefixes(jujuVersion))
	}
	if err := query.Find(&profiles).Error; err != nil {
		return nil, dbError(err)
	}
	return profiles, nil
}

// RemoveControllerProfile deletes the saved controller profile with the given
// name.
func (d *Database) RemoveControllerProfile(ctx context.Context, name string) (err error) {
	const op = "db.RemoveControllerProfile"
	if err := d.ready(); err != nil {
		return err
	}
	if name == "" {
		return errors.Codef(errors.CodeBadRequest, "controller profile name must be provided")
	}

	durationObserver := servermon.DurationObserver(servermon.DBQueryDurationHistogram, op)
	defer durationObserver()
	defer servermon.ErrorCounter(servermon.DBQueryErrorCount, &err, op)

	query := d.DB.WithContext(ctx).Where("name = ?", name).Delete(&dbmodel.ControllerProfile{})
	if query.Error != nil {
		return dbError(query.Error)
	}
	if query.RowsAffected == 0 {
		return errors.Codef(errors.CodeNotFound, "controller profile %q not found", name)
	}
	return nil
}
