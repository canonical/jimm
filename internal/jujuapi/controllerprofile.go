// Copyright 2026 Canonical.

package jujuapi

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	jujuversion "github.com/juju/version/v2"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	apiparams "github.com/canonical/jimm/v3/pkg/api/params"
)

const partialJujuVersionFormatMessage = "must be a partial/full version string with one, two, or three dot-separated numeric components"

// SaveControllerProfile creates or replaces a saved controller profile.
func (r *controllerRoot) SaveControllerProfile(ctx context.Context, req apiparams.SaveControllerProfileRequest) (apiparams.SaveControllerProfileResponse, error) {
	if !r.user.JimmAdmin {
		return apiparams.SaveControllerProfileResponse{}, errors.E(errors.CodeUnauthorized, "unauthorized")
	}
	if err := validateSaveControllerProfileRequest(req); err != nil {
		return apiparams.SaveControllerProfileResponse{}, err
	}

	profile := controllerProfileFromParams(req.ControllerProfile)
	if err := r.jimm.ControllerProfileManager().SaveControllerProfile(ctx, &profile); err != nil {
		return apiparams.SaveControllerProfileResponse{}, fmt.Errorf("failed to save controller profile: %w", err)
	}

	return apiparams.SaveControllerProfileResponse{ControllerProfile: controllerProfileToParams(profile)}, nil
}

// GetControllerProfile retrieves a saved controller profile by name.
func (r *controllerRoot) GetControllerProfile(ctx context.Context, req apiparams.GetControllerProfileRequest) (apiparams.GetControllerProfileResponse, error) {
	if !r.user.JimmAdmin {
		return apiparams.GetControllerProfileResponse{}, errors.E(errors.CodeUnauthorized, "unauthorized")
	}
	profile, err := r.jimm.ControllerProfileManager().GetControllerProfile(ctx, req.Name)
	if err != nil {
		return apiparams.GetControllerProfileResponse{}, fmt.Errorf("failed to get controller profile: %w", err)
	}

	return apiparams.GetControllerProfileResponse{ControllerProfile: controllerProfileToParams(*profile)}, nil
}

// ListControllerProfiles lists saved controller profiles, optionally filtered
// by Juju version.
func (r *controllerRoot) ListControllerProfiles(ctx context.Context, req apiparams.ListControllerProfilesRequest) (apiparams.ListControllerProfilesResponse, error) {
	if !r.user.JimmAdmin {
		return apiparams.ListControllerProfilesResponse{}, errors.E(errors.CodeUnauthorized, "unauthorized")
	}
	if err := validateListControllerProfilesRequest(req); err != nil {
		return apiparams.ListControllerProfilesResponse{}, err
	}
	profiles, err := r.jimm.ControllerProfileManager().ListControllerProfiles(ctx, req.JujuVersion)
	if err != nil {
		return apiparams.ListControllerProfilesResponse{}, fmt.Errorf("failed to list controller profiles: %w", err)
	}

	resp := apiparams.ListControllerProfilesResponse{Profiles: make([]apiparams.ControllerProfileSummary, len(profiles))}
	for i, profile := range profiles {
		resp.Profiles[i] = controllerProfileSummaryToParams(profile)
	}
	return resp, nil
}

// RemoveControllerProfile removes a saved controller profile by name.
func (r *controllerRoot) RemoveControllerProfile(ctx context.Context, req apiparams.RemoveControllerProfileRequest) error {
	if !r.user.JimmAdmin {
		return errors.E(errors.CodeUnauthorized, "unauthorized")
	}
	if err := r.jimm.ControllerProfileManager().RemoveControllerProfile(ctx, req.Name); err != nil {
		return fmt.Errorf("failed to remove controller profile: %w", err)
	}
	return nil
}

func validateSaveControllerProfileRequest(req apiparams.SaveControllerProfileRequest) error {
	if err := validatePartialJujuVersion(req.JujuVersion, "controller profile juju version", false); err != nil {
		return err
	}
	if slices.Contains(builtInClouds, req.Cloud.Name) {
		return errors.E(errors.CodeIncompatibleClouds, fmt.Errorf("controller profiles do not support built-in clouds like %q", req.Cloud.Name))
	}
	storagePool := req.BootstrapOptions.StoragePool
	if storagePool != nil && (storagePool.Name == "") != (storagePool.Type == "") {
		return errors.E(errors.CodeBadRequest, "controller profile storage pool requires both name and type")
	}
	return nil
}

func validateListControllerProfilesRequest(req apiparams.ListControllerProfilesRequest) error {
	return validatePartialJujuVersion(req.JujuVersion, "controller profile juju version filter", true)
}

func validatePartialJujuVersion(versionString, fieldName string, allowEmpty bool) error {
	if versionString == "" {
		if allowEmpty {
			return nil
		}
		return errors.E(errors.CodeBadRequest, fmt.Sprintf("%s must be provided", fieldName))
	}
	if _, err := jujuversion.ParseNonStrict(versionString); err != nil {
		return errors.E(errors.CodeBadRequest, fmt.Sprintf("%s %s", fieldName, partialJujuVersionFormatMessage))
	}
	if strings.Contains(versionString, "-") || len(strings.Split(versionString, ".")) > 3 {
		return errors.E(errors.CodeBadRequest, fmt.Sprintf("%s %s", fieldName, partialJujuVersionFormatMessage))
	}
	return nil
}

func controllerProfileFromParams(profile apiparams.ControllerProfile) dbmodel.ControllerProfile {
	return dbmodel.ControllerProfile{
		Name:        profile.Name,
		Description: profile.Description,
		JujuVersion: profile.JujuVersion,
		Version:     profile.Version,
		Cloud: dbmodel.ControllerProfileCloud{
			Name:            profile.Cloud.Name,
			Type:            profile.Cloud.Type,
			AuthTypes:       dbmodel.Strings(profile.Cloud.AuthTypes),
			CACertificates:  dbmodel.Strings(profile.Cloud.CACertificates),
			Config:          dbmodel.Map(profile.Cloud.Config),
			Endpoint:        profile.Cloud.Endpoint,
			HostCloudRegion: profile.Cloud.HostCloudRegion,
			Region: dbmodel.ControllerProfileCloudRegion{
				Name:             profile.Cloud.Region.Name,
				Endpoint:         profile.Cloud.Region.Endpoint,
				IdentityEndpoint: profile.Cloud.Region.IdentityEndpoint,
				StorageEndpoint:  profile.Cloud.Region.StorageEndpoint,
			},
		},
		BootstrapOptions: dbmodel.ControllerProfileBootstrapOptions{
			BootstrapBase:         profile.BootstrapOptions.BootstrapBase,
			BootstrapConstraints:  dbmodel.StringMap(profile.BootstrapOptions.BootstrapConstraints),
			ModelConstraints:      dbmodel.StringMap(profile.BootstrapOptions.ModelConstraints),
			ModelDefault:          dbmodel.StringMap(profile.BootstrapOptions.ModelDefault),
			StoragePool:           storagePoolFromParams(profile.BootstrapOptions.StoragePool),
			BootstrapConfig:       dbmodel.StringMap(profile.BootstrapOptions.BootstrapConfig),
			ControllerConfig:      dbmodel.StringMap(profile.BootstrapOptions.ControllerConfig),
			ControllerModelConfig: dbmodel.StringMap(profile.BootstrapOptions.ControllerModelConfig),
		},
	}
}

func storagePoolFromParams(pool *apiparams.BootstrapStoragePool) dbmodel.ControllerProfileStoragePool {
	if pool == nil {
		return dbmodel.ControllerProfileStoragePool{}
	}
	return dbmodel.ControllerProfileStoragePool{
		Name:       pool.Name,
		Type:       pool.Type,
		Attributes: dbmodel.StringMap(pool.Attributes),
	}
}

func controllerProfileToParams(profile dbmodel.ControllerProfile) apiparams.ControllerProfile {
	return apiparams.ControllerProfile{
		Name:        profile.Name,
		Description: profile.Description,
		JujuVersion: profile.JujuVersion,
		Version:     profile.Version,
		CreatedAt:   profile.CreatedAt.Format(time.RFC3339),
		UpdatedAt:   profile.UpdatedAt.Format(time.RFC3339),
		Cloud: apiparams.BootstrapCloud{
			Name:            profile.Cloud.Name,
			Type:            profile.Cloud.Type,
			AuthTypes:       []string(profile.Cloud.AuthTypes),
			CACertificates:  []string(profile.Cloud.CACertificates),
			Config:          map[string]interface{}(profile.Cloud.Config),
			Endpoint:        profile.Cloud.Endpoint,
			HostCloudRegion: profile.Cloud.HostCloudRegion,
			Region: apiparams.BootstrapCloudRegion{
				Name:             profile.Cloud.Region.Name,
				Endpoint:         profile.Cloud.Region.Endpoint,
				IdentityEndpoint: profile.Cloud.Region.IdentityEndpoint,
				StorageEndpoint:  profile.Cloud.Region.StorageEndpoint,
			},
		},
		BootstrapOptions: apiparams.BootstrapOptions{
			BootstrapBase:         profile.BootstrapOptions.BootstrapBase,
			BootstrapConstraints:  map[string]string(profile.BootstrapOptions.BootstrapConstraints),
			ModelConstraints:      map[string]string(profile.BootstrapOptions.ModelConstraints),
			ModelDefault:          map[string]string(profile.BootstrapOptions.ModelDefault),
			StoragePool:           storagePoolToParams(profile.BootstrapOptions.StoragePool),
			BootstrapConfig:       map[string]string(profile.BootstrapOptions.BootstrapConfig),
			ControllerConfig:      map[string]string(profile.BootstrapOptions.ControllerConfig),
			ControllerModelConfig: map[string]string(profile.BootstrapOptions.ControllerModelConfig),
		},
	}
}

func controllerProfileSummaryToParams(profile dbmodel.ControllerProfile) apiparams.ControllerProfileSummary {
	return apiparams.ControllerProfileSummary{
		Name:        profile.Name,
		Description: profile.Description,
		CreatedAt:   profile.CreatedAt.Format(time.RFC3339),
		UpdatedAt:   profile.UpdatedAt.Format(time.RFC3339),
	}
}

func storagePoolToParams(pool dbmodel.ControllerProfileStoragePool) *apiparams.BootstrapStoragePool {
	if pool.Name == "" && pool.Type == "" && len(pool.Attributes) == 0 {
		return nil
	}
	return &apiparams.BootstrapStoragePool{
		Name:       pool.Name,
		Type:       pool.Type,
		Attributes: map[string]string(pool.Attributes),
	}
}
