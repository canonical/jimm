// Copyright 2025 Canonical.

package jujuclient

import (
	"context"

	jujuerrors "github.com/juju/errors"
	cloudapi "github.com/juju/juju/api/client/cloud"
	jujucloud "github.com/juju/juju/cloud"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v5"

	"github.com/canonical/jimm/v3/internal/errors"
)

// CheckCredentialModels checks that the given credential would be
// accepted as a valid credential by all models currently using that
// credential. This method uses the CheckCredentialsModel procedure on
// the Cloud. Any error that represents a Juju API
// failure will be of type *APIError.
func (c Connection) CheckCredentialModels(ctx context.Context, cred jujuparams.TaggedCredential) ([]jujuparams.UpdateCredentialModelResult, error) {

	in := jujuparams.TaggedCredentials{
		Credentials: []jujuparams.TaggedCredential{cred},
	}

	out := jujuparams.UpdateCredentialResults{
		Results: make([]jujuparams.UpdateCredentialResult, 1),
	}
	if err := c.CallHighestFacadeVersion(ctx, "Cloud", []int{7}, "", "CheckCredentialsModels", &in, &out); err != nil {
		return nil, errors.E(jujuerrors.Cause(err))
	}
	if out.Results[0].Error != nil {
		return out.Results[0].Models, errors.E(out.Results[0].Error)
	}
	return out.Results[0].Models, nil
}

// UpdateCredential updates the given credential on the controller. The
// credential will always be upgraded if possible irrespective of whether
// it will break existing models (this is a forced update). If the caller
// wants to check that a credential will work with existing models then
// CheckCredentialModels should be used first.
//
// This method will call the first available procedure from:
//   - Cloud(7).UpdateCredentialsCheckModels
//   - Cloud(3).UpdateCredentialsCheckModels
//   - Cloud(1).UpdateCredentials
//
// Any error that represents a Juju API failure will be of type
// *APIError.
func (c Connection) UpdateCredential(ctx context.Context, cred jujuparams.TaggedCredential) ([]jujuparams.UpdateCredentialModelResult, error) {

	creds := jujuparams.TaggedCredentials{
		Credentials: []jujuparams.TaggedCredential{cred},
	}

	update := jujuparams.UpdateCredentialArgs{
		Credentials: creds.Credentials,
		Force:       true,
	}

	out := jujuparams.UpdateCredentialResults{
		Results: make([]jujuparams.UpdateCredentialResult, 1),
	}

	// Cloud(1).UpdateCredentials actually returns
	// jujuparams.ErrorResults rather than
	// jujuparams.UpdateCredentialsResults, but the former will still
	// unmarshal correctly into the latter so there is no need to use
	// a different response type.
	if err := c.CallHighestFacadeVersion(ctx, "Cloud", []int{7}, "", "UpdateCredentialsCheckModels", &update, &out); err != nil {
		return nil, errors.E(jujuerrors.Cause(err))
	}
	if out.Results[0].Error != nil {
		return out.Results[0].Models, errors.E(out.Results[0].Error)
	}
	return out.Results[0].Models, nil
}

// RevokeCredential removes the given credential on the controller. The
// credential will always be removed irrespective of whether it will
// break existing models (this is a forced revoke). If the caller wants
// to check that removing the credential will break existing models then
// CheckCredentialModels should be used first.
//
// This method will call the first available procedure from:
//   - Cloud(3).RevokeCredentialsCheckModels
//   - Cloud(1).RevokeCredentials
//
// Any error that represents a Juju API failure will be of type
// *APIError.
func (c Connection) RevokeCredential(ctx context.Context, cred names.CloudCredentialTag) error {

	out := jujuparams.ErrorResults{
		Results: make([]jujuparams.ErrorResult, 1),
	}
	in := jujuparams.RevokeCredentialArgs{
		Credentials: []jujuparams.RevokeCredentialArg{{
			Tag:   cred.String(),
			Force: true,
		}},
	}
	if err := c.CallHighestFacadeVersion(ctx, "Cloud", []int{7}, "", "RevokeCredentialsCheckModels", &in, &out); err != nil {
		return errors.E(jujuerrors.Cause(err))
	}

	if out.Results[0].Error != nil {
		return errors.E(out.Results[0].Error)
	}
	return nil
}

// Cloud retrieves information about the given cloud. Cloud uses the
// Cloud procedure on the Cloud facade.
func (c Connection) Cloud(tag names.CloudTag, cloud *jujucloud.Cloud) error {
	cloudAPI := cloudapi.NewClient(&c)
	res, err := cloudAPI.Cloud(tag)
	if err != nil {
		return err
	}
	*cloud = res
	return nil
}

// Clouds retrieves information about all available clouds. Clouds uses the
// Clouds procedure on the Cloud facade.
func (c Connection) Clouds() (map[names.CloudTag]jujucloud.Cloud, error) {
	cloudAPI := cloudapi.NewClient(&c)
	return cloudAPI.Clouds()
}

// AddCloud adds the given cloud to a controller with the given name.
// AddCloud uses the AddCloud procedure on the Cloud facade.
func (c Connection) AddCloud(tag names.CloudTag, cloud jujucloud.Cloud, force bool) error {
	cloudAPI := cloudapi.NewClient(&c)
	return cloudAPI.AddCloud(cloud, force)
}

// RemoveCloud removes the given cloud from the controller. RemoveCloud
// uses the RemoveClouds procedure on the Cloud facade.
func (c Connection) RemoveCloud(tag names.CloudTag) error {
	cloudAPI := cloudapi.NewClient(&c)
	return cloudAPI.RemoveCloud(tag.Id())
}

// UpdateCloud updates the given cloud with the given cloud definition.
// UpdateCloud uses the UpdateCloud procedure on the cloud facade.
func (c Connection) UpdateCloud(tag names.CloudTag, cloud jujucloud.Cloud) error {
	cloudAPI := cloudapi.NewClient(&c)
	return cloudAPI.UpdateCloud(cloud)
}

// CredentialContents returns contents of the credential values for the specified
// cloud and credential name. Secrets will be included if requested.
func (c Connection) CredentialContents(cloud string, credential string, withSecrets bool) ([]jujuparams.CredentialContentResult, error) {
	cloudAPI := cloudapi.NewClient(&c)
	return cloudAPI.CredentialContents(cloud, credential, withSecrets)
}
