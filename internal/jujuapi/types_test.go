// Copyright 2026 Canonical.

package jujuapi

import (
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
	"github.com/google/go-cmp/cmp"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/life"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v5"
	"github.com/juju/version/v2"

	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/jimm/juju"
	"github.com/canonical/jimm/v3/internal/jujuclient"
)

func TestModelCreateArgs(t *testing.T) {
	c := qt.New(t)

	authenticatedUser := names.NewUserTag("vorbis@canonical.com")

	tests := []struct {
		about         string
		args          jujuparams.ModelCreateArgs
		expectedArgs  *juju.ModelCreateArgs
		expectedError string
	}{{
		about: "all ok",
		args: jujuparams.ModelCreateArgs{
			Name:               "test-model",
			OwnerTag:           names.NewUserTag("alice@canonical.com").String(),
			CloudTag:           names.NewCloudTag("test-cloud").String(),
			CloudCredentialTag: names.NewCloudCredentialTag("test-cloud/alice@canonical.com/test-credential-1").String(),
		},
		expectedArgs: &juju.ModelCreateArgs{
			Name:            "test-model",
			Owner:           names.NewUserTag("alice@canonical.com"),
			Cloud:           names.NewCloudTag("test-cloud"),
			CloudCredential: names.NewCloudCredentialTag("test-cloud/alice@canonical.com/test-credential-1"),
		},
	}, {
		about: "name not specified",
		args: jujuparams.ModelCreateArgs{
			OwnerTag:           names.NewUserTag("alice@canonical.com").String(),
			CloudTag:           names.NewCloudTag("test-cloud").String(),
			CloudCredentialTag: names.NewCloudCredentialTag("test-cloud/alice/test-credential-1").String(),
		},
		expectedError: "name not specified",
	}, {
		about: "invalid owner tag",
		args: jujuparams.ModelCreateArgs{
			Name:               "test-model",
			OwnerTag:           "alice@canonical.com",
			CloudTag:           names.NewCloudTag("test-cloud").String(),
			CloudCredentialTag: names.NewCloudCredentialTag("test-cloud/alice/test-credential-1").String(),
		},
		expectedError: `"alice@canonical.com" is not a valid tag`,
	}, {
		about: "invalid cloud tag",
		args: jujuparams.ModelCreateArgs{
			Name:               "test-model",
			OwnerTag:           names.NewUserTag("alice@canonical.com").String(),
			CloudTag:           "test-cloud",
			CloudCredentialTag: names.NewCloudCredentialTag("test-cloud/alice/test-credential-1").String(),
		},
		expectedError: `"test-cloud" is not a valid tag`,
	}, {
		about: "invalid cloud credential tag",
		args: jujuparams.ModelCreateArgs{
			Name:               "test-model",
			OwnerTag:           names.NewUserTag("alice@canonical.com").String(),
			CloudTag:           names.NewCloudTag("test-cloud").String(),
			CloudCredentialTag: "test-credential-1",
		},
		expectedError: `invalid cloud credential tag: "test-credential-1" is not a valid tag`,
	}, {
		about: "cloud does not match cloud credential cloud",
		args: jujuparams.ModelCreateArgs{
			Name:               "test-model",
			OwnerTag:           names.NewUserTag("alice@canonical.com").String(),
			CloudTag:           names.NewCloudTag("test-cloud").String(),
			CloudCredentialTag: names.NewCloudCredentialTag("another-cloud/alice/test-credential-1").String(),
		},
		expectedError: "cloud credential cloud mismatch",
	}, {
		about: "owner tag not specified",
		args: jujuparams.ModelCreateArgs{
			Name:     "test-model",
			CloudTag: names.NewCloudTag("test-cloud").String(),
		},
		expectedArgs: &juju.ModelCreateArgs{
			Name:  "test-model",
			Owner: names.NewUserTag("vorbis@canonical.com"),
			Cloud: names.NewCloudTag("test-cloud"),
		},
	}}

	opts := []cmp.Option{
		cmp.Comparer(func(t1, t2 names.UserTag) bool {
			return t1.String() == t2.String()
		}),
		cmp.Comparer(func(t1, t2 names.CloudTag) bool {
			return t1.String() == t2.String()
		}),
		cmp.Comparer(func(t1, t2 names.CloudCredentialTag) bool {
			return t1.String() == t2.String()
		}),
	}
	for _, test := range tests {
		c.Run(test.about, func(c *qt.C) {
			a, err := toAddModelArgs(test.args, authenticatedUser)
			if test.expectedError == "" {
				c.Assert(err, qt.IsNil)
				c.Assert(a, qt.CmpEquals(opts...), test.expectedArgs)
			} else {
				c.Assert(err, qt.ErrorMatches, test.expectedError)
			}
		})
	}
}

func TestToModelInfo(t *testing.T) {
	c := qt.New(t)

	now := time.Now().UTC().Truncate(time.Second)
	lastConnection := now.Add(-5 * time.Minute)
	arch := "amd64"
	cores := uint64(8)
	mem := uint64(16384)
	rootDisk := uint64(512000)
	cpuPower := uint64(4000)
	availabilityZone := "eu-west-1a"
	virtType := "kvm"
	tags := []string{"tag1", "tag2"}
	haPrimary := true
	hasVote := true
	wantsVote := true
	agentVersion := version.MustParse("3.6.0")

	modelInfo := base.ModelInfo{
		Name:            "test-model",
		UUID:            "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
		ControllerUUID:  "11111111-2222-3333-4444-555555555555",
		IsController:    false,
		Type:            "iaas",
		DefaultSeries:   "jammy",
		Cloud:           "aws",
		CloudRegion:     "eu-west-1",
		CloudCredential: "aws/alice@canonical.com/main-cred",
		Owner:           "alice@canonical.com",
		Life:            life.Value("alive"),
		ProviderType:    "ec2",
		AgentVersion:    &agentVersion,
		Status: base.Status{
			Status: "available",
			Info:   "ready",
			Data: map[string]any{
				"k": "v",
			},
			Since: &now,
		},
		Users: []base.UserInfo{{
			UserName:       "alice@canonical.com",
			DisplayName:    "Alice",
			Access:         "admin",
			LastConnection: &lastConnection,
		}},
		Machines: []base.Machine{{
			Id:          "0",
			InstanceId:  "i-000001",
			DisplayName: "machine-0",
			Status:      "running",
			Message:     "ok",
			HAPrimary:   &haPrimary,
			HasVote:     hasVote,
			WantsVote:   wantsVote,
			Hardware: &instance.HardwareCharacteristics{
				Arch:             &arch,
				CpuCores:         &cores,
				Mem:              &mem,
				RootDisk:         &rootDisk,
				CpuPower:         &cpuPower,
				Tags:             &tags,
				AvailabilityZone: &availabilityZone,
				VirtType:         &virtType,
			},
		}, {
			Id:          "1",
			InstanceId:  "i-000002",
			DisplayName: "machine-1",
			Status:      "pending",
			Message:     "provisioning",
		}},
	}

	got := toModelInfo(modelInfo)

	c.Assert(got.Name, qt.Equals, modelInfo.Name)
	c.Assert(got.UUID, qt.Equals, modelInfo.UUID)
	c.Assert(got.ControllerUUID, qt.Equals, modelInfo.ControllerUUID)
	c.Assert(got.IsController, qt.Equals, modelInfo.IsController)
	c.Assert(got.Type, qt.Equals, modelInfo.Type.String())
	c.Assert(got.DefaultSeries, qt.Equals, modelInfo.DefaultSeries)
	c.Assert(got.DefaultBase, qt.Equals, modelInfo.DefaultSeries)
	c.Assert(got.CloudTag, qt.Equals, names.NewCloudTag(modelInfo.Cloud).String())
	c.Assert(got.CloudRegion, qt.Equals, modelInfo.CloudRegion)
	c.Assert(got.CloudCredentialTag, qt.Equals, names.NewCloudCredentialTag(modelInfo.CloudCredential).String())
	c.Assert(got.OwnerTag, qt.Equals, names.NewUserTag(modelInfo.Owner).String())
	c.Assert(got.Life, qt.Equals, modelInfo.Life)
	c.Assert(got.ProviderType, qt.Equals, modelInfo.ProviderType)
	c.Assert(got.AgentVersion, qt.Equals, modelInfo.AgentVersion)
	c.Assert(got.Status, qt.DeepEquals, jujuparams.EntityStatus{
		Status: modelInfo.Status.Status,
		Info:   modelInfo.Status.Info,
		Data:   modelInfo.Status.Data,
		Since:  modelInfo.Status.Since,
	})

	c.Assert(got.Users, qt.DeepEquals, []jujuparams.ModelUserInfo{{
		UserName:       "alice@canonical.com",
		DisplayName:    "Alice",
		LastConnection: &lastConnection,
		Access:         jujuparams.UserAccessPermission("admin"),
	}})

	c.Assert(got.Machines, qt.HasLen, 2)
	c.Assert(got.Machines[0], qt.DeepEquals, jujuparams.ModelMachineInfo{
		Id:          "0",
		InstanceId:  "i-000001",
		DisplayName: "machine-0",
		Status:      "running",
		Message:     "ok",
		Hardware: &jujuparams.MachineHardware{
			Arch:             &arch,
			Cores:            &cores,
			Mem:              &mem,
			RootDisk:         &rootDisk,
			CpuPower:         &cpuPower,
			Tags:             &tags,
			AvailabilityZone: &availabilityZone,
			VirtType:         &virtType,
		},
		HAPrimary: &haPrimary,
		HasVote:   hasVote,
		WantsVote: wantsVote,
	})
	c.Assert(got.Machines[1], qt.DeepEquals, jujuparams.ModelMachineInfo{
		Id:          "1",
		InstanceId:  "i-000002",
		DisplayName: "machine-1",
		Status:      "pending",
		Message:     "provisioning",
		Hardware:    &jujuparams.MachineHardware{},
	})
}

func TestToModelInfoNilMachineInfo(t *testing.T) {
	c := qt.New(t)

	modelInfo := base.ModelInfo{
		Name:            "test-model",
		Cloud:           "aws",
		CloudCredential: "aws/alice@canonical.com/main-cred",
		Owner:           "alice@canonical.com",
	}

	got := toModelInfo(modelInfo)

	c.Assert(got.Name, qt.Equals, "test-model")
	c.Assert(got.Machines, qt.IsNil)
}

func TestToFullModelInfo(t *testing.T) {
	c := qt.New(t)

	start := time.Now().UTC().Truncate(time.Second)
	end := start.Add(15 * time.Minute)
	rotateInterval := 30 * time.Minute
	validity := true
	agentVersion := version.MustParse("3.6.0")

	modelInfo := jujuclient.ModelInfo{
		ModelInfo: base.ModelInfo{
			Name:            "test-model",
			UUID:            "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
			ControllerUUID:  "11111111-2222-3333-4444-555555555555",
			IsController:    false,
			Type:            "iaas",
			DefaultSeries:   "jammy",
			Cloud:           "aws",
			CloudRegion:     "eu-west-1",
			CloudCredential: "aws/alice@canonical.com/main-cred",
			Owner:           "alice@canonical.com",
			Life:            life.Value("alive"),
			ProviderType:    "ec2",
			AgentVersion:    &agentVersion,
		},
		MigrationStatus: &jujuclient.ModelMigrationStatus{
			Status: "running",
			Start:  &start,
			End:    &end,
		},
		CloudCredentialValidity: &validity,
		SupportedFeatures: []jujuclient.SupportedFeature{{
			Name:        "feature-a",
			Description: "test feature",
			Version:     "v1",
		}},
		SecretBackends: []jujuclient.SecretBackendResult{{
			Result: jujuclient.SecretBackend{
				Name:                "vault",
				BackendType:         "vault",
				TokenRotateInterval: &rotateInterval,
				Config: map[string]interface{}{
					"endpoint": "https://vault.example.com",
				},
			},
			ID:         "backend-1",
			NumSecrets: 7,
			Status:     "available",
			Message:    "ready",
			Error:      errors.Codef(errors.CodeBadRequest, "backend warning"),
		}},
	}

	got := toFullModelInfo(modelInfo)

	c.Assert(got.Name, qt.Equals, modelInfo.Name)
	c.Assert(got.CloudCredentialValidity, qt.DeepEquals, &validity)
	c.Assert(got.Migration, qt.DeepEquals, &jujuparams.ModelMigrationStatus{
		Status: "running",
		Start:  &start,
		End:    &end,
	})
	c.Assert(got.SupportedFeatures, qt.DeepEquals, []jujuparams.SupportedFeature{{
		Name:        "feature-a",
		Description: "test feature",
		Version:     "v1",
	}})
	c.Assert(got.SecretBackends, qt.DeepEquals, []jujuparams.SecretBackendResult{{
		Result: jujuparams.SecretBackend{
			Name:                "vault",
			BackendType:         "vault",
			TokenRotateInterval: &rotateInterval,
			Config: map[string]interface{}{
				"endpoint": "https://vault.example.com",
			},
		},
		ID:         "backend-1",
		NumSecrets: 7,
		Status:     "available",
		Message:    "ready",
		Error: &jujuparams.Error{
			Message: "backend warning",
			Code:    string(errors.CodeBadRequest),
			Info:    nil,
		},
	}})
}

func TestToFullModelInfoNilSecretBackendError(t *testing.T) {
	c := qt.New(t)

	modelInfo := jujuclient.ModelInfo{
		ModelInfo: base.ModelInfo{
			Name:            "test-model",
			Cloud:           "aws",
			CloudCredential: "aws/alice@canonical.com/main-cred",
			Owner:           "alice@canonical.com",
		},
		SecretBackends: []jujuclient.SecretBackendResult{{
			Result: jujuclient.SecretBackend{
				Name:        "vault",
				BackendType: "vault",
			},
			ID:         "backend-1",
			NumSecrets: 7,
			Status:     "available",
			Message:    "ready",
			Error:      nil,
		}},
	}

	got := toFullModelInfo(modelInfo)

	c.Assert(got.SecretBackends, qt.DeepEquals, []jujuparams.SecretBackendResult{{
		Result: jujuparams.SecretBackend{
			Name:        "vault",
			BackendType: "vault",
		},
		ID:         "backend-1",
		NumSecrets: 7,
		Status:     "available",
		Message:    "ready",
		Error:      nil,
	}})
}

func TestToFullModelInfoNonNilSecretBackendError(t *testing.T) {
	c := qt.New(t)

	modelInfo := jujuclient.ModelInfo{
		ModelInfo: base.ModelInfo{
			Name:            "test-model",
			Cloud:           "aws",
			CloudCredential: "aws/alice@canonical.com/main-cred",
			Owner:           "alice@canonical.com",
		},
		SecretBackends: []jujuclient.SecretBackendResult{{
			Result: jujuclient.SecretBackend{
				Name:        "vault",
				BackendType: "vault",
			},
			Error: &errors.Error{Code: errors.CodeNotFound, Message: "an error", Info: map[string]any{"detail": "not found"}},
		}},
	}

	got := toFullModelInfo(modelInfo)

	c.Assert(got.SecretBackends, qt.HasLen, 1)
	c.Assert(got.SecretBackends[0].Error, qt.IsNotNil)
	c.Assert(got.SecretBackends[0].Error.Error(), qt.Equals, "an error")
	c.Assert(got.SecretBackends[0].Error.Code, qt.Equals, string(errors.CodeNotFound))
	c.Assert(got.SecretBackends[0].Error.Info, qt.DeepEquals, map[string]any{"detail": "not found"})
}
