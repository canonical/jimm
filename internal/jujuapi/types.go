// Copyright 2026 Canonical.

package jujuapi

import (
	"fmt"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/cloud"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v5"
	"gopkg.in/yaml.v3"

	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/jimm/jobs"
	"github.com/canonical/jimm/v3/internal/jimm/juju"
	"github.com/canonical/jimm/v3/internal/jujuclient"
	"github.com/canonical/jimm/v3/pkg/api/params"
)

func cloudFromParams(cloudName string, p jujuparams.Cloud) cloud.Cloud {
	authTypes := make([]cloud.AuthType, len(p.AuthTypes))
	for i, authType := range p.AuthTypes {
		authTypes[i] = cloud.AuthType(authType)
	}
	regions := make([]cloud.Region, len(p.Regions))
	for i, region := range p.Regions {
		regions[i] = cloud.Region{
			Name:             region.Name,
			Endpoint:         region.Endpoint,
			IdentityEndpoint: region.IdentityEndpoint,
			StorageEndpoint:  region.StorageEndpoint,
		}
	}
	var regionConfig map[string]cloud.Attrs
	for r, attr := range p.RegionConfig {
		if regionConfig == nil {
			regionConfig = make(map[string]cloud.Attrs)
		}
		regionConfig[r] = attr
	}
	return cloud.Cloud{
		Name:              cloudName,
		Type:              p.Type,
		AuthTypes:         authTypes,
		Endpoint:          p.Endpoint,
		IdentityEndpoint:  p.IdentityEndpoint,
		StorageEndpoint:   p.StorageEndpoint,
		Regions:           regions,
		CACertificates:    p.CACertificates,
		SkipTLSVerify:     p.SkipTLSVerify,
		HostCloudRegion:   p.HostCloudRegion,
		Config:            p.Config,
		RegionConfig:      regionConfig,
		IsControllerCloud: p.IsControllerCloud,
	}
}

func toAddModelArgs(args jujuparams.ModelCreateArgs, authenticatedUser names.UserTag) (*juju.ModelCreateArgs, error) {
	// FromJujuModelCreateArgs converts jujuparams.ModelCreateArgs into AddModelArgs.
	var a juju.ModelCreateArgs
	if args.Name == "" {
		return nil, errors.New("name not specified")
	}
	a.Name = args.Name
	a.Config = args.Config
	a.CloudRegion = args.CloudRegion
	if args.CloudTag != "" {
		ct, err := names.ParseCloudTag(args.CloudTag)
		if err != nil {
			return nil, errors.Codef(errors.CodeBadRequest, "%w", err)
		}
		a.Cloud = ct
	}

	if args.OwnerTag != "" {
		ot, err := names.ParseUserTag(args.OwnerTag)
		if err != nil {
			return nil, errors.Codef(errors.CodeBadRequest, "%w", err)
		}
		a.Owner = ot
	} else {
		a.Owner = authenticatedUser
	}

	if args.CloudCredentialTag != "" {
		ct, err := names.ParseCloudCredentialTag(args.CloudCredentialTag)
		if err != nil {
			return nil, fmt.Errorf("invalid cloud credential tag: %w", err)
		}
		if a.Cloud.Id() != "" && ct.Cloud().Id() != a.Cloud.Id() {
			return nil, errors.New("cloud credential cloud mismatch")
		}

		a.CloudCredential = ct
	}
	return &a, nil
}

func toModelStatusParams(modelStatus base.ModelStatus) jujuparams.ModelStatus {
	if modelStatus.Error != nil {
		return jujuparams.ModelStatus{
			Error: &jujuparams.Error{
				Message: modelStatus.Error.Error(),
				Code:    string(errors.ErrorCode(modelStatus.Error)),
				Info:    errors.ErrorInfo(modelStatus.Error),
			},
		}
	}
	st := jujuparams.ModelStatus{
		ModelTag:           names.NewModelTag(modelStatus.UUID).String(),
		Life:               modelStatus.Life,
		Type:               modelStatus.ModelType.String(),
		HostedMachineCount: modelStatus.HostedMachineCount,
		ApplicationCount:   modelStatus.ApplicationCount,
		UnitCount:          modelStatus.UnitCount,
		OwnerTag:           names.NewUserTag(modelStatus.Owner).String(),
	}
	for _, app := range modelStatus.Applications {
		st.Applications = append(st.Applications, jujuparams.ModelApplicationInfo{
			Name: app.Name,
		})
	}
	for _, machine := range modelStatus.Machines {
		hardware := &jujuparams.MachineHardware{}
		if machine.Hardware != nil {
			hardware = &jujuparams.MachineHardware{
				Arch:             machine.Hardware.Arch,
				Cores:            machine.Hardware.CpuCores,
				Mem:              machine.Hardware.Mem,
				RootDisk:         machine.Hardware.RootDisk,
				CpuPower:         machine.Hardware.CpuPower,
				Tags:             machine.Hardware.Tags,
				AvailabilityZone: machine.Hardware.AvailabilityZone,
				VirtType:         machine.Hardware.VirtType,
			}
		}
		st.Machines = append(st.Machines, jujuparams.ModelMachineInfo{
			Id:          machine.Id,
			InstanceId:  machine.InstanceId,
			DisplayName: machine.DisplayName,
			HasVote:     machine.HasVote,
			WantsVote:   machine.WantsVote,
			Status:      machine.Status,
			Message:     machine.Message,
			Hardware:    hardware,
			HAPrimary:   machine.HAPrimary,
		})
	}
	for _, volume := range modelStatus.Volumes {
		st.Volumes = append(st.Volumes, jujuparams.ModelVolumeInfo{
			Id:         volume.Id,
			ProviderId: volume.ProviderId,
			Detachable: volume.Detachable,
			Status:     volume.Status,
			Message:    volume.Message,
		})
	}
	for _, fs := range modelStatus.Filesystems {
		st.Filesystems = append(st.Filesystems, jujuparams.ModelFilesystemInfo{
			Id:         fs.Id,
			ProviderId: fs.ProviderId,
			Detachable: fs.Detachable,
			Status:     fs.Status,
			Message:    fs.Message,
		})
	}
	return st
}

func toModelSummariesParams(modelSummaries []base.UserModelSummary) jujuparams.ModelSummaryResults {
	modelSummaryResults := make([]jujuparams.ModelSummaryResult, len(modelSummaries))
	for i, ms := range modelSummaries {
		if ms.Error != nil {
			modelSummaryResults[i] = jujuparams.ModelSummaryResult{
				Error: &jujuparams.Error{
					Message: ms.Error.Error(),
					Code:    string(errors.ErrorCode(ms.Error)),
					Info:    errors.ErrorInfo(ms.Error),
				},
			}
			continue
		}
		summaryParams := jujuparams.ModelSummary{
			Name:               ms.Name,
			UUID:               ms.UUID,
			Type:               ms.Type.String(),
			ControllerUUID:     ms.ControllerUUID,
			IsController:       ms.IsController,
			ProviderType:       ms.ProviderType,
			DefaultSeries:      ms.DefaultSeries,
			CloudTag:           names.NewCloudTag(ms.Cloud).String(),
			CloudRegion:        ms.CloudRegion,
			CloudCredentialTag: names.NewCloudCredentialTag(ms.CloudCredential).String(),
			OwnerTag:           names.NewUserTag(ms.Owner).String(),
			Life:               ms.Life,
			Status: jujuparams.EntityStatus{
				Status: ms.Status.Status,
				Info:   ms.Status.Info,
				Data:   ms.Status.Data,
				Since:  ms.Status.Since,
			},
			UserAccess: jujuparams.UserAccessPermission(ms.ModelUserAccess),
		}
		if ms.SLA != nil {
			summaryParams.SLA = &jujuparams.ModelSLAInfo{
				Level: ms.SLA.Level,
				Owner: ms.SLA.Owner,
			}
		}
		if ms.UserLastConnection != nil {
			summaryParams.UserLastConnection = ms.UserLastConnection
		}
		if ms.Migration != nil {
			summaryParams.Migration = &jujuparams.ModelMigrationStatus{
				Status: ms.Migration.Status,
				Start:  ms.Migration.StartTime,
				End:    ms.Migration.EndTime,
			}
		}
		if ms.AgentVersion != nil {
			summaryParams.AgentVersion = ms.AgentVersion
		}
		if ms.Counts != nil {
			for _, count := range ms.Counts {
				summaryParams.Counts = append(summaryParams.Counts, jujuparams.ModelEntityCount{
					Entity: jujuparams.CountedEntity(count.Entity),
					Count:  count.Count,
				})
			}
		}
		modelSummaryResults[i] = jujuparams.ModelSummaryResult{
			Result: &summaryParams,
		}
	}
	return jujuparams.ModelSummaryResults{Results: modelSummaryResults}
}

func toModelDumpParams(modelDump map[string]any) (string, error) {
	yamlDump, err := yaml.Marshal(modelDump)
	return string(yamlDump), err
}

func toModelInfo(modelInfo base.ModelInfo) jujuparams.ModelInfo {
	mi := jujuparams.ModelInfo{
		Name:               modelInfo.Name,
		UUID:               modelInfo.UUID,
		ControllerUUID:     modelInfo.ControllerUUID,
		IsController:       modelInfo.IsController,
		Type:               modelInfo.Type.String(),
		DefaultSeries:      modelInfo.DefaultSeries,
		CloudTag:           names.NewCloudTag(modelInfo.Cloud).String(),
		CloudRegion:        modelInfo.CloudRegion,
		CloudCredentialTag: names.NewCloudCredentialTag(modelInfo.CloudCredential).String(),
		OwnerTag:           names.NewUserTag(modelInfo.Owner).String(),
		Life:               modelInfo.Life,
		ProviderType:       modelInfo.ProviderType,
		DefaultBase:        modelInfo.DefaultSeries,
		AgentVersion:       modelInfo.AgentVersion,
		Status: jujuparams.EntityStatus{
			Status: modelInfo.Status.Status,
			Info:   modelInfo.Status.Info,
			Data:   modelInfo.Status.Data,
			Since:  modelInfo.Status.Since,
		},
	}
	for _, user := range modelInfo.Users {
		mi.Users = append(mi.Users, jujuparams.ModelUserInfo{
			UserName:       user.UserName,
			DisplayName:    user.DisplayName,
			LastConnection: user.LastConnection,
			Access:         jujuparams.UserAccessPermission(user.Access),
		})
	}
	for _, machine := range modelInfo.Machines {
		hardwareInfo := &jujuparams.MachineHardware{}
		if machine.Hardware != nil {
			hardwareInfo = &jujuparams.MachineHardware{
				Arch:             machine.Hardware.Arch,
				Cores:            machine.Hardware.CpuCores,
				Mem:              machine.Hardware.Mem,
				RootDisk:         machine.Hardware.RootDisk,
				CpuPower:         machine.Hardware.CpuPower,
				Tags:             machine.Hardware.Tags,
				AvailabilityZone: machine.Hardware.AvailabilityZone,
				VirtType:         machine.Hardware.VirtType,
			}
		}
		mi.Machines = append(mi.Machines, jujuparams.ModelMachineInfo{
			Id:          machine.Id,
			InstanceId:  machine.InstanceId,
			DisplayName: machine.DisplayName,
			Status:      machine.Status,
			Message:     machine.Message,
			Hardware:    hardwareInfo,
			HAPrimary:   machine.HAPrimary,
			HasVote:     machine.HasVote,
			WantsVote:   machine.WantsVote,
		})
	}
	return mi
}

func toFullModelInfo(modelInfo jujuclient.ModelInfo) jujuparams.ModelInfo {
	modelInfoParams := toModelInfo(modelInfo.ModelInfo)

	if modelInfo.MigrationStatus != nil {
		modelInfoParams.Migration = &jujuparams.ModelMigrationStatus{
			Status: modelInfo.MigrationStatus.Status,
			Start:  modelInfo.MigrationStatus.Start,
			End:    modelInfo.MigrationStatus.End,
		}
	}
	if modelInfo.CloudCredentialValidity != nil {
		modelInfoParams.CloudCredentialValidity = modelInfo.CloudCredentialValidity
	}

	var supportedFeatures []jujuparams.SupportedFeature
	for _, f := range modelInfo.SupportedFeatures {
		supportedFeatures = append(supportedFeatures, jujuparams.SupportedFeature{
			Name:        f.Name,
			Description: f.Description,
			Version:     f.Version,
		})
	}
	modelInfoParams.SupportedFeatures = supportedFeatures

	var secretBackendResults []jujuparams.SecretBackendResult
	for _, sb := range modelInfo.SecretBackends {
		res := jujuparams.SecretBackendResult{
			Result: jujuparams.SecretBackend{
				Name:                sb.Result.Name,
				BackendType:         sb.Result.BackendType,
				TokenRotateInterval: sb.Result.TokenRotateInterval,
				Config:              sb.Result.Config,
			},
			ID:         sb.ID,
			NumSecrets: sb.NumSecrets,
			Status:     sb.Status,
			Message:    sb.Message,
		}
		if sb.Error != nil {
			res.Error = &jujuparams.Error{
				Message: sb.Error.Error(),
				Code:    string(errors.ErrorCode(sb.Error)),
				Info:    errors.ErrorInfo(sb.Error),
			}
		}
		secretBackendResults = append(secretBackendResults, res)
	}
	modelInfoParams.SecretBackends = secretBackendResults

	return modelInfoParams
}

func toJobInfoParams(jobInfo jobs.JobInfo) params.JobInfoResponse {
	var jobErrors []params.JobError
	for _, err := range jobInfo.Errors {
		jobErrors = append(jobErrors, params.JobError{
			Error:   err.Error,
			At:      err.At,
			Attempt: err.Attempt,
		})
	}
	return params.JobInfoResponse{
		ID:             jobInfo.ID,
		Status:         jobInfo.Status,
		Kind:           jobInfo.Kind,
		CurrentAttempt: jobInfo.CurrentAttempt,
		MaxAttempts:    jobInfo.MaxAttempts,
		FinishedAt:     jobInfo.FinishedAt,
		Errors:         jobErrors,
	}
}
