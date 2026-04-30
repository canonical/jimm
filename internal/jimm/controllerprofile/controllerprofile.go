// Copyright 2026 Canonical.

// Package controllerprofile provides an implementation
// for managing controller profiles within JIMM.
package controllerprofile

import (
	"context"

	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
)

// ControllerProfileManager provides a means to manage controller profiles within JIMM.
type ControllerProfileManager struct {
	store *db.Database
}

// NewControllerProfileManager returns a new controller profile manager backed by the provided store.
func NewControllerProfileManager(store *db.Database) (*ControllerProfileManager, error) {
	if store == nil {
		return nil, errors.New("controller profile store cannot be nil")
	}
	return &ControllerProfileManager{store: store}, nil
}

// SaveControllerProfile creates or replaces a saved controller profile.
func (m *ControllerProfileManager) SaveControllerProfile(ctx context.Context, profile *dbmodel.ControllerProfile) error {
	if err := m.store.CreateOrReplaceControllerProfile(ctx, profile); err != nil {
		return err
	}
	return nil
}

// GetControllerProfile retrieves a saved controller profile by name.
func (m *ControllerProfileManager) GetControllerProfile(ctx context.Context, name string) (*dbmodel.ControllerProfile, error) {
	profile := &dbmodel.ControllerProfile{Name: name}
	if err := m.store.GetControllerProfile(ctx, profile); err != nil {
		return nil, err
	}
	return profile, nil
}

// ListControllerProfiles lists saved controller profiles, optionally filtered
// by Juju version.
func (m *ControllerProfileManager) ListControllerProfiles(ctx context.Context, jujuVersion string) ([]dbmodel.ControllerProfile, error) {
	profiles, err := m.store.ListControllerProfiles(ctx, jujuVersion)
	if err != nil {
		return nil, err
	}
	return profiles, nil
}

// RemoveControllerProfile removes a saved controller profile by name.
func (m *ControllerProfileManager) RemoveControllerProfile(ctx context.Context, name string) error {
	if err := m.store.RemoveControllerProfile(ctx, name); err != nil {
		return err
	}
	return nil
}
