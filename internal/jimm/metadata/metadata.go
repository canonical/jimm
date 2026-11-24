// Copyright 2026 Canonical.

package metadata

import (
	"context"

	"github.com/juju/names/v5"

	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/openfga"
	ofganames "github.com/canonical/jimm/v3/internal/openfga/names"
)

// Manager manages user-defined metadata for models and applications.
type Manager struct {
	store *db.Database
}

// NewManager creates a metadata manager backed by store.
func NewManager(store *db.Database) *Manager {
	return &Manager{store: store}
}

// GetMetadata returns all metadata for the target if the user can read its model.
func (m *Manager) GetMetadata(ctx context.Context, user *openfga.User, target dbmodel.EntityMetadataTarget) (map[string]string, error) {
	if err := requireModelAccess(ctx, user, target.ModelUUID, ofganames.ReaderRelation); err != nil {
		return nil, err
	}
	return m.store.GetEntityMetadata(ctx, target)
}

// SetMetadata updates only the supplied keys and returns all metadata for the
// target. The user must have writer access to its model.
func (m *Manager) SetMetadata(ctx context.Context, user *openfga.User, target dbmodel.EntityMetadataTarget, values map[string]string) (map[string]string, error) {
	if err := requireModelAccess(ctx, user, target.ModelUUID, ofganames.WriterRelation); err != nil {
		return nil, err
	}
	var result map[string]string
	err := m.store.Transaction(func(tx *db.Database) error {
		if err := tx.UpsertEntityMetadata(ctx, target, values); err != nil {
			return err
		}
		var err error
		result, err = tx.GetEntityMetadata(ctx, target)
		return err
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// RemoveMetadata deletes the specified keys, or every key if all is true.
// The user must have writer access to the target's model.
func (m *Manager) RemoveMetadata(ctx context.Context, user *openfga.User, target dbmodel.EntityMetadataTarget, all bool, keys []string) error {
	if err := requireModelAccess(ctx, user, target.ModelUUID, ofganames.WriterRelation); err != nil {
		return err
	}
	return m.store.DeleteEntityMetadata(ctx, target, all, keys)
}

// QueryMetadata returns exact key/value matches from models the user can read,
// including matches on applications in those models. A matcher is required.
func (m *Manager) QueryMetadata(ctx context.Context, user *openfga.User, matcher *db.EntityMetadataMatcher) ([]dbmodel.EntityMetadata, error) {
	if user == nil {
		return nil, errors.Codef(errors.CodeUnauthorized, "unauthorized")
	}
	modelUUIDs, err := user.ListModels(ctx, ofganames.ReaderRelation)
	if err != nil {
		return nil, err
	}
	return m.store.QueryEntityMetadata(ctx, modelUUIDs, matcher)
}

func requireModelAccess(ctx context.Context, user *openfga.User, modelUUID string, minimumRelation openfga.Relation) error {
	if user == nil {
		return errors.Codef(errors.CodeUnauthorized, "unauthorized")
	}
	model, err := names.ParseModelTag(names.ModelTagKind + "-" + modelUUID)
	if err != nil {
		return errors.Codef(errors.CodeBadRequest, "%w", err)
	}
	// OpenFGA evaluates inherited access: administrator implies writer, which implies reader.
	allowed, err := user.HasModelRelation(ctx, model, minimumRelation)
	if err != nil {
		return err
	}
	if !allowed {
		return errors.Codef(errors.CodeUnauthorized, "unauthorized")
	}
	return nil
}
