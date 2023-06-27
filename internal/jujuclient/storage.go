// Copyright 2023 Canonical Ltd.

package jujuclient

import (
	"context"

	jujuerrors "github.com/juju/errors"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v4"

	"github.com/CanonicalLtd/jimm/internal/errors"
)

// ListFilesystems lists filesystems for desired machines.
// If no machines provided, a list of all filesystems is returned.
func (c Connection) ListFilesystems(ctx context.Context, machines []string) ([]jujuparams.FilesystemDetailsListResult, error) {
	const op = errors.Op("jujuclient.ListFilesystems")

	filters := make([]jujuparams.FilesystemFilter, len(machines))
	for i, machine := range machines {
		filters[i].Machines = []string{names.NewMachineTag(machine).String()}
	}
	if len(filters) == 0 {
		filters = []jujuparams.FilesystemFilter{{}}
	}

	args := jujuparams.FilesystemFilters{
		Filters: filters,
	}
	var results jujuparams.FilesystemDetailsListResults

	if err := c.CallHighestFacadeVersion(ctx, "Storage", []int{6}, "", "ListFilesystems", &args, &results); err != nil {
		return nil, errors.E(op, jujuerrors.Cause(err))
	}

	if len(results.Results) != len(filters) {
		return nil, errors.E(
			op,
			jujuerrors.Errorf(
				"expected %d result(s), got %d",
				len(filters), len(results.Results),
			),
		)
	}

	return results.Results, nil
}

// ListVolumes lists volumes for desired machines.
// If no machines provided, a list of all volumes is returned.
func (c Connection) ListVolumes(ctx context.Context, machines []string) ([]jujuparams.VolumeDetailsListResult, error) {
	const op = errors.Op("jujuclient.ListVolumes")

	filters := make([]jujuparams.VolumeFilter, len(machines))
	for i, machine := range machines {
		filters[i].Machines = []string{names.NewMachineTag(machine).String()}
	}
	if len(filters) == 0 {
		filters = []jujuparams.VolumeFilter{{}}
	}
	args := jujuparams.VolumeFilters{Filters: filters}
	var results jujuparams.VolumeDetailsListResults

	if err := c.CallHighestFacadeVersion(ctx, "Storage", []int{6}, "", "ListVolumes", &args, &results); err != nil {
		return nil, errors.E(op, jujuerrors.Cause(err))
	}

	if len(results.Results) != len(filters) {
		return nil, errors.E(
			op,
			jujuerrors.Errorf(
				"expected %d result(s), got %d",
				len(filters), len(results.Results),
			),
		)
	}

	return results.Results, nil
}

// ListStorageDetails lists all storage.
func (c Connection) ListStorageDetails(ctx context.Context) ([]jujuparams.StorageDetails, error) {
	const op = errors.Op("jujuclient.ListStorageDetails")

	args := jujuparams.StorageFilters{
		Filters: []jujuparams.StorageFilter{{}}, // one empty filter
	}
	var results jujuparams.StorageDetailsListResults

	if err := c.CallHighestFacadeVersion(ctx, "Storage", []int{6}, "", "ListStorageDetails", &args, &results); err != nil {
		return nil, errors.E(op, jujuerrors.Cause(err))
	}

	if len(results.Results) != 1 {
		return nil, errors.E(
			op,
			jujuerrors.Errorf(
				"expected 1 result, got %d",
				len(results.Results),
			),
		)
	}
	if results.Results[0].Error != nil {
		return nil, errors.E(
			op,
			jujuerrors.Trace(results.Results[0].Error),
		)
	}
	return results.Results[0].Result, nil
}
