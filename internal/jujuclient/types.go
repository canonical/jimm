// Copyright 2026 Canonical.

package jujuclient

import (
	"maps"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/names/v4"
)

// This file contains conversions for API types returned by Juju
// to types that we want to use in JIMM where necessary. For the
// most part the Juju client already returns types that
// we can use directly, but in some cases we need to convert
// them where the Juju client has inconsistent behaviour.

// convertParamsModelInfo converts a params.ModelInfo to a base.ModelInfo.
// It is copied from the Juju codebase because it is not exported and has
// additional logic to parse the migration status. In some API methods like
// CreateModel, a base.ModelInfo type is returned, while in calls to ModelInfo
// a params.ModelInfo type is returned. We want consistent types preferably of
// the form base.ModelInfo. Until the client returns a consistent set of types
// we copy it here here to have better consistency in our domain layer.
func convertParamsModelInfo(modelInfo params.ModelInfo) (ModelInfo, error) {
	cloud, err := names.ParseCloudTag(modelInfo.CloudTag)
	if err != nil {
		return ModelInfo{}, err
	}
	var credential string
	if modelInfo.CloudCredentialTag != "" {
		credTag, err := names.ParseCloudCredentialTag(modelInfo.CloudCredentialTag)
		if err != nil {
			return ModelInfo{}, err
		}
		credential = credTag.Id()
	}
	ownerTag, err := names.ParseUserTag(modelInfo.OwnerTag)
	if err != nil {
		return ModelInfo{}, err
	}
	result := base.ModelInfo{
		Name:            modelInfo.Name,
		UUID:            modelInfo.UUID,
		ControllerUUID:  modelInfo.ControllerUUID,
		IsController:    modelInfo.IsController,
		ProviderType:    modelInfo.ProviderType,
		DefaultSeries:   modelInfo.DefaultSeries,
		Cloud:           cloud.Id(),
		CloudRegion:     modelInfo.CloudRegion,
		CloudCredential: credential,
		Owner:           ownerTag.Id(),
		Life:            modelInfo.Life,
		AgentVersion:    modelInfo.AgentVersion,
	}
	modelType := modelInfo.Type
	if modelType == "" {
		modelType = model.IAAS.String()
	}
	result.Type = model.ModelType(modelType)
	result.Status = base.Status{
		Status: modelInfo.Status.Status,
		Info:   modelInfo.Status.Info,
		Data:   make(map[string]any),
		Since:  modelInfo.Status.Since,
	}
	maps.Copy(result.Status.Data, modelInfo.Status.Data)
	result.Users = make([]base.UserInfo, len(modelInfo.Users))
	for i, u := range modelInfo.Users {
		result.Users[i] = base.UserInfo{
			UserName:       u.UserName,
			DisplayName:    u.DisplayName,
			Access:         string(u.Access),
			LastConnection: u.LastConnection,
		}
	}
	result.Machines = make([]base.Machine, len(modelInfo.Machines))
	for i, m := range modelInfo.Machines {
		machine := base.Machine{
			Id:          m.Id,
			InstanceId:  m.InstanceId,
			DisplayName: m.DisplayName,
			HasVote:     m.HasVote,
			WantsVote:   m.WantsVote,
			Status:      m.Status,
			HAPrimary:   m.HAPrimary,
		}
		if m.Hardware != nil {
			machine.Hardware = &instance.HardwareCharacteristics{
				Arch:             m.Hardware.Arch,
				Mem:              m.Hardware.Mem,
				RootDisk:         m.Hardware.RootDisk,
				CpuCores:         m.Hardware.Cores,
				CpuPower:         m.Hardware.CpuPower,
				Tags:             m.Hardware.Tags,
				AvailabilityZone: m.Hardware.AvailabilityZone,
			}
		}
		result.Machines[i] = machine
	}
	var migrationStatus *ModelMigrationStatus
	if modelInfo.Migration != nil {
		migrationStatus = &ModelMigrationStatus{
			Status: modelInfo.Migration.Status,
			Start:  modelInfo.Migration.Start,
			End:    modelInfo.Migration.End,
		}
	}
	supportedFeatures := make([]SupportedFeature, len(modelInfo.SupportedFeatures))
	for i, f := range modelInfo.SupportedFeatures {
		supportedFeatures[i] = SupportedFeature{
			Name:        f.Name,
			Description: f.Description,
			Version:     f.Version,
		}
	}
	var secretBackendResults []SecretBackendResult
	for _, sb := range modelInfo.SecretBackends {
		res := SecretBackendResult{
			Result: SecretBackend{
				Name:                sb.Result.Name,
				BackendType:         sb.Result.BackendType,
				Config:              sb.Result.Config,
				TokenRotateInterval: sb.Result.TokenRotateInterval,
			},
			ID:         sb.ID,
			Status:     sb.Status,
			NumSecrets: sb.NumSecrets,
			Message:    sb.Message,
		}
		if sb.Error != nil {
			res.Error = sb.Error
		}
		secretBackendResults = append(secretBackendResults, res)
	}
	return ModelInfo{
		ModelInfo:               result,
		MigrationStatus:         migrationStatus,
		CloudCredentialValidity: modelInfo.CloudCredentialValidity,
		SupportedFeatures:       supportedFeatures,
		SecretBackends:          secretBackendResults,
	}, nil
}
