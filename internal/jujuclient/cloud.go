// Copyright 2020 Canonical Ltd.

package jujuclient

import (
	"context"

	jujuerrors "github.com/juju/errors"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v4"
	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"

	"github.com/canonical/jimm/internal/errors"
)

// SupportsCheckCredentialModels reports whether the controller supports
// the Cloud.CheckCredentialsModels, Cloud.RevokeCredentialsCheckModels,
// and Cloud.UpdateCredentialsCheckModels methods.
func (c Connection) SupportsCheckCredentialModels() bool {
	return c.hasFacadeVersion("Cloud", 3) || c.hasFacadeVersion("Cloud", 7)
}

// CheckCredentialModels checks that the given credential would be
// accepted as a valid credential by all models currently using that
// credential. This method uses the CheckCredentialsModel procedure on
// the Cloud. Any error that represents a Juju API
// failure will be of type *APIError.
func (c Connection) CheckCredentialModels(ctx context.Context, cred jujuparams.TaggedCredential) ([]jujuparams.UpdateCredentialModelResult, error) {
	const op = errors.Op("jujuclient.CheckCredentialModels")
	in := jujuparams.TaggedCredentials{
		Credentials: []jujuparams.TaggedCredential{cred},
	}

	out := jujuparams.UpdateCredentialResults{
		Results: make([]jujuparams.UpdateCredentialResult, 1),
	}
	if err := c.CallHighestFacadeVersion(ctx, "Cloud", []int{7, 3}, "", "CheckCredentialsModels", &in, &out); err != nil {
		return nil, errors.E(op, jujuerrors.Cause(err))
	}
	if out.Results[0].Error != nil {
		return out.Results[0].Models, errors.E(op, out.Results[0].Error)
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
	const op = errors.Op("jujuclient.UpdateCredential")
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
	if err := c.CallHighestFacadeVersion(ctx, "Cloud", []int{7, 3, 1}, "", "UpdateCredentialsCheckModels", &update, &out); err != nil {
		return nil, errors.E(op, jujuerrors.Cause(err))
	}
	if out.Results[0].Error != nil {
		return out.Results[0].Models, errors.E(op, out.Results[0].Error)
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
	const op = errors.Op("jujuclient.RevokeCredential")
	out := jujuparams.ErrorResults{
		Results: make([]jujuparams.ErrorResult, 1),
	}
	if c.SupportsCheckCredentialModels() {
		in := jujuparams.RevokeCredentialArgs{
			Credentials: []jujuparams.RevokeCredentialArg{{
				Tag:   cred.String(),
				Force: true,
			}},
		}
		if err := c.CallHighestFacadeVersion(ctx, "Cloud", []int{7, 3}, "", "RevokeCredentialsCheckModels", &in, &out); err != nil {
			return errors.E(op, jujuerrors.Cause(err))
		}
	} else {
		in := jujuparams.Entities{
			Entities: []jujuparams.Entity{{
				Tag: cred.String(),
			}},
		}
		if err := c.Call(ctx, "Cloud", 1, "", "RevokeCredentials", &in, &out); err != nil {
			return errors.E(op, jujuerrors.Cause(err))
		}
	}
	if out.Results[0].Error != nil {
		return errors.E(op, out.Results[0].Error)
	}
	return nil
}

// Cloud retrieves information about the given cloud. Cloud uses the
// Cloud procedure on the Cloud facade.
func (c Connection) Cloud(ctx context.Context, tag names.CloudTag, cloud *jujuparams.Cloud) error {
	const op = errors.Op("jujuclient.Cloud")
	args := jujuparams.Entities{
		Entities: []jujuparams.Entity{{
			Tag: tag.String(),
		}},
	}
	resp := jujuparams.CloudResults{
		Results: []jujuparams.CloudResult{{
			Cloud: cloud,
		}},
	}
	if err := c.CallHighestFacadeVersion(ctx, "Cloud", []int{7, 1}, "", "Cloud", &args, &resp); err != nil {
		return errors.E(op, jujuerrors.Cause(err))
	}
	if resp.Results[0].Error != nil {
		errors.E(op, resp.Results[0].Error)
	}
	return nil
}

// Clouds retrieves information about all available clouds. Clouds uses the
// Clouds procedure on the Cloud facade.
func (c Connection) Clouds(ctx context.Context) (map[names.CloudTag]jujuparams.Cloud, error) {
	const op = errors.Op("jujuclient.Clouds")
	var resp jujuparams.CloudsResult
	if err := c.CallHighestFacadeVersion(ctx, "Cloud", []int{7, 1}, "", "Clouds", nil, &resp); err != nil {
		return nil, errors.E(op, jujuerrors.Cause(err))
	}

	clouds := make(map[names.CloudTag]jujuparams.Cloud, len(resp.Clouds))
	for cloudTag, cloud := range resp.Clouds {
		tag, err := names.ParseCloudTag(cloudTag)
		if err != nil {
			zapctx.Warn(ctx, "controller returned invalid cloud tag", zap.String("tag", cloudTag))
			continue
		}
		clouds[tag] = cloud
	}
	return clouds, nil
}

// AddCloud adds the given cloud to a controller with the given name.
// AddCloud uses the AddCloud procedure on the Cloud facade.
func (c Connection) AddCloud(ctx context.Context, tag names.CloudTag, cloud jujuparams.Cloud, force bool) error {
	const op = errors.Op("jujuclient.AddCloud")
	args := jujuparams.AddCloudArgs{
		Cloud: cloud,
		Name:  tag.Id(),
		Force: &force,
	}
	if err := c.CallHighestFacadeVersion(ctx, "Cloud", []int{7, 2}, "", "AddCloud", &args, nil); err != nil {
		return errors.E(op, jujuerrors.Cause(err))
	}
	return nil
}

// RemoveCloud removes the given cloud from the controller. RemoveCloud
// uses the RemoveClouds procedure on the Cloud facade.
func (c Connection) RemoveCloud(ctx context.Context, tag names.CloudTag) error {
	const op = errors.Op("jujuclient.RemoveCloud")
	args := jujuparams.Entities{
		Entities: []jujuparams.Entity{{
			Tag: tag.String(),
		}},
	}
	resp := jujuparams.ErrorResults{
		Results: make([]jujuparams.ErrorResult, 1),
	}
	if err := c.CallHighestFacadeVersion(ctx, "Cloud", []int{7, 2}, "", "RemoveClouds", &args, &resp); err != nil {
		return errors.E(op, jujuerrors.Cause(err))
	}
	if resp.Results[0].Error != nil {
		return errors.E(op, resp.Results[0].Error)
	}
	return nil
}

// GrantCloudAccess gives the given user the given access level on the
// given cloud. GrantCloudAccess uses the ModifyCloudAccess procedure on
// the Cloud facade.
func (c Connection) GrantCloudAccess(ctx context.Context, cloudTag names.CloudTag, userTag names.UserTag, access string) error {
	const op = errors.Op("jujuclient.GrantCloudAccess")
	args := jujuparams.ModifyCloudAccessRequest{
		Changes: []jujuparams.ModifyCloudAccess{{
			UserTag:  userTag.String(),
			Action:   jujuparams.GrantCloudAccess,
			Access:   access,
			CloudTag: cloudTag.String(),
		}},
	}

	resp := jujuparams.ErrorResults{
		Results: make([]jujuparams.ErrorResult, 1),
	}
	err := c.CallHighestFacadeVersion(ctx, "Cloud", []int{7, 3}, "", "ModifyCloudAccess", &args, &resp)
	if err != nil {
		return errors.E(op, jujuerrors.Cause(err))
	}
	if resp.Results[0].Error != nil {
		return errors.E(op, resp.Results[0].Error)
	}
	return nil
}

// RevokeCloudAccess revokes the given access level on the given cloud from
// the given user. RevokeCloudAccess uses the ModifyCloudAccess procedure
// on the Cloud facade.
func (c Connection) RevokeCloudAccess(ctx context.Context, cloudTag names.CloudTag, userTag names.UserTag, access string) error {
	const op = errors.Op("jujuclient.RevokeCloudAccess")
	args := jujuparams.ModifyCloudAccessRequest{
		Changes: []jujuparams.ModifyCloudAccess{{
			UserTag:  userTag.String(),
			Action:   jujuparams.RevokeCloudAccess,
			Access:   access,
			CloudTag: cloudTag.String(),
		}},
	}

	resp := jujuparams.ErrorResults{
		Results: make([]jujuparams.ErrorResult, 1),
	}
	err := c.CallHighestFacadeVersion(ctx, "Cloud", []int{7, 3}, "", "ModifyCloudAccess", &args, &resp)
	if err != nil {
		return errors.E(op, jujuerrors.Cause(err))
	}
	if resp.Results[0].Error != nil {
		return errors.E(op, resp.Results[0].Error)
	}
	return nil
}

// CloudInfo retrieves information about the cloud with the given name.
// CloudInfo uses the CloudInfo procedure on the Cloud facade.
func (c Connection) CloudInfo(ctx context.Context, tag names.CloudTag, ci *jujuparams.CloudInfo) error {
	const op = errors.Op("jujuclient.CloudInfo")
	args := jujuparams.Entities{
		Entities: []jujuparams.Entity{{Tag: tag.String()}},
	}

	resp := jujuparams.CloudInfoResults{
		Results: []jujuparams.CloudInfoResult{{
			Result: ci,
		}},
	}
	err := c.CallHighestFacadeVersion(ctx, "Cloud", []int{7, 2}, "", "CloudInfo", &args, &resp)
	if err != nil {
		return errors.E(op, jujuerrors.Cause(err))
	}
	if resp.Results[0].Error != nil {
		return errors.E(op, resp.Results[0].Error)
	}
	return nil
}

// UpdateCloud updates the given cloud with the given cloud definition.
// UpdateCloud uses the UpdateCloud procedure on the cloud facade.
func (c Connection) UpdateCloud(ctx context.Context, tag names.CloudTag, cloud jujuparams.Cloud) error {
	const op = errors.Op("jujuclient.UpdateCloud")

	args := jujuparams.UpdateCloudArgs{
		Clouds: []jujuparams.AddCloudArgs{{
			Cloud: cloud,
			Name:  tag.Id(),
		}},
	}
	resp := jujuparams.ErrorResults{
		Results: make([]jujuparams.ErrorResult, 1),
	}
	err := c.CallHighestFacadeVersion(ctx, "Cloud", []int{7, 4}, "", "UpdateCloud", &args, &resp)
	if err != nil {
		return errors.E(op, jujuerrors.Cause(err))
	}
	if resp.Results[0].Error != nil {
		return errors.E(op, resp.Results[0].Error)
	}
	return nil
}
