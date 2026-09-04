// Copyright 2025 Canonical.

package jujuclient

import (
	"context"

	cloudapi "github.com/juju/juju/api/client/cloud"
	jujucloud "github.com/juju/juju/cloud"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v6"
)

// CheckCredentialModels checks that the given credential would be
// accepted as a valid credential by all models currently using that
// credential. This method uses the CheckCredentialsModel procedure on
// the Cloud.
//
// If the same cloud is on many controllers, we need to know
// ahead of time that the credential will work for all models on all controllers
// under this cloud. Once we are sure via CheckCredentialModels that the credential
// will work for all models, only then can we safely update the credential.
// For this reason, UpdateCredentialsCheckModels alone is insufficient as it
// checks a single credential is safe and is is not suitable when updating
// the cloud's credential across many controllers.
func (c Connection) CheckCredentialModels(ctx context.Context, cred jujuparams.TaggedCredential) ([]jujuparams.UpdateCredentialResult, error) {
	return cloudapi.NewClient(&c).CheckCredentialsModels(ctx, jujuparams.TaggedCredentials{Credentials: []jujuparams.TaggedCredential{cred}})
}

// UpdateCloudsCredentialForce updates the given credential on the controller.
// It calls UpdateCloudsCredentials, and adapts it to:
// 1. Take a single credential
// 2. Always force the update (force=true).
// As such, the credential will always be upgraded if possible irrespective
// of whether it will break existing models (this is a forced update).
// If the caller wants to check that a credential will work with existing models
// then CheckCredentialModels should be used first.
func (c Connection) UpdateCloudsCredentialForce(ctx context.Context, cred jujuparams.TaggedCredential) ([]jujuparams.UpdateCredentialResult, error) {
	return cloudapi.NewClient(&c).UpdateCloudsCredentials(
		ctx,
		map[string]jujucloud.Credential{
			cred.Tag: jujucloud.NewCredential(jujucloud.AuthType(cred.Credential.AuthType), cred.Credential.Attributes),
		},
		true,
	)
}

// RevokeCredential removes the given credential on the controller. The
// credential will always be removed irrespective of whether it will
// break existing models (this is a forced revoke). If the caller wants
// to check that removing the credential will break existing models then
// CheckCredentialModels should be used first.
func (c Connection) RevokeCredential(ctx context.Context, cred names.CloudCredentialTag) error {
	return cloudapi.NewClient(&c).RevokeCredential(ctx, cred, true)
}

// Cloud retrieves information about the given cloud. Cloud uses the
// Cloud procedure on the Cloud facade.
func (c Connection) Cloud(ctx context.Context, tag names.CloudTag) (jujucloud.Cloud, error) {
	return cloudapi.NewClient(&c).Cloud(ctx, tag)
}

// Clouds retrieves information about all available clouds. Clouds uses the
// Clouds procedure on the Cloud facade.
func (c Connection) Clouds(ctx context.Context) (map[names.CloudTag]jujucloud.Cloud, error) {
	return cloudapi.NewClient(&c).Clouds(ctx)
}

// AddCloud adds the given cloud to a controller with the given name.
// AddCloud uses the AddCloud procedure on the Cloud facade.
func (c Connection) AddCloud(ctx context.Context, tag names.CloudTag, cloud jujucloud.Cloud, force bool) error {
	return cloudapi.NewClient(&c).AddCloud(ctx, cloud, force)
}

// RemoveCloud removes the given cloud from the controller. RemoveCloud
// uses the RemoveClouds procedure on the Cloud facade.
func (c Connection) RemoveCloud(ctx context.Context, tag names.CloudTag) error {
	return cloudapi.NewClient(&c).RemoveCloud(ctx, tag.Id())
}

// ModelConfigSchema returns the model config schema for the given provider
// type. The schema describes the configuration attributes, including
// provider specific ones, that may be set on a model hosted by that type of
// cloud. ModelConfigSchema uses the ModelConfigSchema procedure on the Cloud
// facade, which requires the controller to support at least version 8 of the
// Cloud facade.
func (c Connection) ModelConfigSchema(ctx context.Context, providerType string) (map[string]jujuparams.ModelConfigSchemaField, error) {
	return cloudapi.NewClient(&c).ModelConfigSchema(ctx, providerType)
}

// UpdateCloud updates the given cloud with the given cloud definition.
// UpdateCloud uses the UpdateCloud procedure on the cloud facade.
func (c Connection) UpdateCloud(ctx context.Context, tag names.CloudTag, cloud jujucloud.Cloud) error {
	return cloudapi.NewClient(&c).UpdateCloud(ctx, cloud)
}

// CredentialContents returns contents of the credential values for the specified
// cloud and credential name. Secrets will be included if requested.
func (c Connection) CredentialContents(ctx context.Context, cloud string, credential string, withSecrets bool) ([]jujuparams.CredentialContentResult, error) {
	return cloudapi.NewClient(&c).CredentialContents(ctx, cloud, credential, withSecrets)
}
