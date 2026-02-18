// Copyright 2025 Canonical.

package jujuclient

import (
	"context"

	"github.com/juju/juju/api/client/storage"
	jujuparams "github.com/juju/juju/rpc/params"
)

// ListFilesystems lists filesystems for desired machines.
// If no machines provided, a list of all filesystems is returned.
func (c Connection) ListFilesystems(ctx context.Context, machines []string) ([]jujuparams.FilesystemDetailsListResult, error) {
	return storage.NewClient(&c).ListFilesystems(machines)
}

// ListVolumes lists volumes for desired machines.
// If no machines provided, a list of all volumes is returned.
func (c Connection) ListVolumes(ctx context.Context, machines []string) ([]jujuparams.VolumeDetailsListResult, error) {
	return storage.NewClient(&c).ListVolumes(machines)
}

// ListStorageDetails lists all storage.
func (c Connection) ListStorageDetails(ctx context.Context) ([]jujuparams.StorageDetails, error) {
	return storage.NewClient(&c).ListStorageDetails()
}
