// Copyright 2026 Canonical.

package db

import (
	"context"
	"maps"
	"slices"

	"github.com/juju/names/v5"
	"gorm.io/gorm/clause"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/servermon"
)

func validateEntityMetadataTarget(target dbmodel.EntityMetadataTarget) error {
	if target.ModelUUID == "" || target.Entity == "" {
		return errors.Codef(errors.CodeBadRequest, "model UUID and entity are required")
	}
	switch target.EntityType {
	case names.ModelTagKind:
		if target.Entity != target.ModelUUID {
			return errors.Codef(errors.CodeBadRequest, "model metadata must match the model UUID")
		}
	case names.ApplicationTagKind:
	default:
		return errors.Codef(errors.CodeBadRequest, "unsupported metadata entity type %q", target.EntityType)
	}
	return nil
}

// GetEntityMetadata returns the metadata for one entity. An entity with no
// metadata returns an empty map.
func (d *Database) GetEntityMetadata(ctx context.Context, target dbmodel.EntityMetadataTarget) (_ map[string]string, err error) {
	const op = "db.GetEntityMetadata"
	if err := d.ready(); err != nil {
		return nil, err
	}
	if err := validateEntityMetadataTarget(target); err != nil {
		return nil, err
	}

	durationObserver := servermon.DurationObserver(servermon.DBQueryDurationHistogram, op)
	defer durationObserver()
	defer servermon.ErrorCounter(servermon.DBQueryErrorCount, &err, op)

	var rows []dbmodel.EntityMetadata
	if err := d.DB.WithContext(ctx).Where(target).Select("key", "value").Find(&rows).Error; err != nil {
		return nil, dbError(err)
	}
	metadata := make(map[string]string, len(rows))
	for _, row := range rows {
		metadata[row.Key] = row.Value
	}
	return metadata, nil
}

// UpsertEntityMetadata atomically inserts or updates the given keys without
// changing any other keys for the entity.
func (d *Database) UpsertEntityMetadata(ctx context.Context, target dbmodel.EntityMetadataTarget, metadata map[string]string) (err error) {
	const op = "db.UpsertEntityMetadata"
	if err := d.ready(); err != nil {
		return err
	}
	if err := validateEntityMetadataTarget(target); err != nil {
		return err
	}
	if len(metadata) == 0 {
		return nil
	}

	durationObserver := servermon.DurationObserver(servermon.DBQueryDurationHistogram, op)
	defer durationObserver()
	defer servermon.ErrorCounter(servermon.DBQueryErrorCount, &err, op)

	rows := make([]dbmodel.EntityMetadata, 0, len(metadata))
	// Acquire row locks in the same order for concurrent upserts.
	for _, key := range slices.Sorted(maps.Keys(metadata)) {
		if key == "" {
			return errors.Codef(errors.CodeBadRequest, "metadata key must not be empty")
		}
		rows = append(rows, dbmodel.EntityMetadata{EntityMetadataTarget: target, Key: key, Value: metadata[key]})
	}
	if err := d.DB.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "entity_type"}, {Name: "model_uuid"}, {Name: "entity"}, {Name: "key"}},
		DoUpdates: clause.AssignmentColumns([]string{"value", "updated_at"}),
	}).Create(&rows).Error; err != nil {
		return dbError(err)
	}
	return nil
}

// DeleteEntityMetadata removes the given keys, or all keys if explicitly
// requested. Missing keys are ignored.
func (d *Database) DeleteEntityMetadata(ctx context.Context, target dbmodel.EntityMetadataTarget, all bool, keys []string) (err error) {
	const op = "db.DeleteEntityMetadata"
	if err := d.ready(); err != nil {
		return err
	}
	if err := validateEntityMetadataTarget(target); err != nil {
		return err
	}
	if (all && len(keys) > 0) || (!all && len(keys) == 0) {
		return errors.Codef(errors.CodeBadRequest, "specify either all metadata or a list of keys to delete")
	}

	durationObserver := servermon.DurationObserver(servermon.DBQueryDurationHistogram, op)
	defer durationObserver()
	defer servermon.ErrorCounter(servermon.DBQueryErrorCount, &err, op)

	query := d.DB.WithContext(ctx).Where(target)
	if !all {
		query = query.Where("key IN ?", keys)
	}
	if err := query.Delete(&dbmodel.EntityMetadata{}).Error; err != nil {
		return dbError(err)
	}
	return nil
}

// EntityMetadataMatcher matches one exact key/value pair, including empty values.
type EntityMetadataMatcher struct {
	Key   string
	Value string
}

// QueryEntityMetadata returns matching rows across entity types within the
// supplied model UUIDs. The key/value filter is required and an empty model
// scope matches no rows.
func (d *Database) QueryEntityMetadata(ctx context.Context, modelUUIDs []string, matcher *EntityMetadataMatcher) (_ []dbmodel.EntityMetadata, err error) {
	const op = "db.QueryEntityMetadata"
	if err := d.ready(); err != nil {
		return nil, err
	}
	if matcher == nil || matcher.Key == "" {
		return nil, errors.Codef(errors.CodeBadRequest, "a metadata filter with a key is required")
	}
	if len(modelUUIDs) == 0 {
		return []dbmodel.EntityMetadata{}, nil
	}

	durationObserver := servermon.DurationObserver(servermon.DBQueryDurationHistogram, op)
	defer durationObserver()
	defer servermon.ErrorCounter(servermon.DBQueryErrorCount, &err, op)

	query := d.DB.WithContext(ctx).
		Where("model_uuid IN ?", modelUUIDs).
		Where("key = ? AND value = ?", matcher.Key, matcher.Value)
	var rows []dbmodel.EntityMetadata
	if err := query.Order("model_uuid, entity_type, entity").Find(&rows).Error; err != nil {
		return nil, dbError(err)
	}
	return rows, nil
}
