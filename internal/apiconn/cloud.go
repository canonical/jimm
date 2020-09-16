// Copyright 2020 Canonical Ltd.

package apiconn

import (
	"context"

	jujuparams "github.com/juju/juju/apiserver/params"
	"github.com/juju/names/v4"
	"go.uber.org/zap"
	"gopkg.in/errgo.v1"

	"github.com/CanonicalLtd/jimm/internal/conv"
	"github.com/CanonicalLtd/jimm/internal/mongodoc"
	"github.com/CanonicalLtd/jimm/internal/zapctx"
	"github.com/CanonicalLtd/jimm/params"
)

// SupportsCheckCredentialModels reports whether the controller supports
// the Cloud.CheckCredentialsModels, Cloud.RevokeCredentialsCheckModels,
// and Cloud.UpdateCredentialsCheckModels methods.
func (c *Conn) SupportsCheckCredentialModels() bool {
	return c.HasFacadeVersion("Cloud", 3)
}

// CheckCredentialModels checks that the given credential would be
// accepted as a valid credential by all models currently using that
// credential. This method uses the CheckCredentialsModel procedure on
// the Cloud facade version 3. Any error that represents a Juju API
// failure will be of type *APIError.
func (c *Conn) CheckCredentialModels(_ context.Context, cred *mongodoc.Credential) ([]jujuparams.UpdateCredentialModelResult, error) {
	in := jujuparams.TaggedCredentials{
		Credentials: []jujuparams.TaggedCredential{
			conv.ToTaggedCredential(cred),
		},
	}

	var out jujuparams.UpdateCredentialResults
	if err := c.APICall("Cloud", 3, "", "CheckCredentialsModels", &in, &out); err != nil {
		return nil, newAPIError(err)
	}
	if len(out.Results) != 1 {
		return nil, errgo.Newf("unexpected number of results (expected 1, got %d)", len(out.Results))
	}
	return out.Results[0].Models, newAPIError(out.Results[0].Error)
}

// UpdateCredential updates the given credential on the controller. The
// credential will always be upgraded if possible irrespective of whether
// it will break existing models (this is a forced update). If the caller
// wants to check that a credential will work with existing models then
// CheckCredentialModels should be used first.
//
// This method will call the first available procedure from:
//     - Cloud(7).UpdateCredentialsCheckModels
//     - Cloud(3).UpdateCredentialsCheckModels
//     - Cloud(1).UpdateCredentials
//
// Any error that represents a Juju API failure will be of type
// *APIError.
func (c *Conn) UpdateCredential(_ context.Context, cred *mongodoc.Credential) ([]jujuparams.UpdateCredentialModelResult, error) {
	creds := jujuparams.TaggedCredentials{
		Credentials: []jujuparams.TaggedCredential{
			conv.ToTaggedCredential(cred),
		},
	}

	update := jujuparams.UpdateCredentialArgs{
		Credentials: creds.Credentials,
		Force:       true,
	}

	var out jujuparams.UpdateCredentialResults
	if c.HasFacadeVersion("Cloud", 7) {
		if err := c.APICall("Cloud", 7, "", "UpdateCredentialsCheckModels", &update, &out); err != nil {
			return nil, newAPIError(err)
		}
	} else if c.HasFacadeVersion("Cloud", 3) {
		if err := c.APICall("Cloud", 3, "", "UpdateCredentialsCheckModels", &update, &out); err != nil {
			return nil, newAPIError(err)
		}
	} else {
		// Cloud(1).UpdateCredentials actually returns
		// jujuparams.ErrorResults rather than
		// jujuparams.UpdateCredentialsResults, but the former will still
		// unmarshal correctly into the latter so there is no need to use
		// a different response type.
		if err := c.APICall("Cloud", 1, "", "UpdateCredentials", &creds, &out); err != nil {
			return nil, newAPIError(err)
		}
	}
	if len(out.Results) != 1 {
		return nil, errgo.Newf("unexpected number of results (expected 1, got %d)", len(out.Results))
	}
	return out.Results[0].Models, newAPIError(out.Results[0].Error)
}

// RevokeCredential removes the given credential on the controller. The
// credential will always be removed irrespective of whether it will
// break existing models (this is a forced revoke). If the caller wants
// to check that removing the credential will break existing models then
// CheckCredentialModels should be used first.
//
// This method will call the first available procedure from:
//     - Cloud(3).RevokeCredentialsCheckModels
//     - Cloud(1).RevokeCredentials
//
// Any error that represents a Juju API failure will be of type
// *APIError.
func (c *Conn) RevokeCredential(_ context.Context, path params.CredentialPath) error {
	var out jujuparams.ErrorResults
	if c.SupportsCheckCredentialModels() {
		in := jujuparams.RevokeCredentialArgs{
			Credentials: []jujuparams.RevokeCredentialArg{{
				Tag:   conv.ToCloudCredentialTag(path).String(),
				Force: true,
			}},
		}
		if err := c.APICall("Cloud", 3, "", "RevokeCredentialsCheckModels", &in, &out); err != nil {
			return newAPIError(err)
		}
	} else {
		in := jujuparams.Entities{
			Entities: []jujuparams.Entity{{
				Tag: conv.ToCloudCredentialTag(path).String(),
			}},
		}
		if err := c.APICall("Cloud", 1, "", "RevokeCredentials", &in, &out); err != nil {
			return newAPIError(err)
		}
	}
	if len(out.Results) != 1 {
		return errgo.Newf("unexpected number of results (expected 1, got %d)", len(out.Results))
	}
	return newAPIError(out.Results[0].Error)
}

// Clouds retrieves information about all available clouds. Clouds uses the
// Clouds procedure on the Cloud facade version 1.
func (c *Conn) Clouds(ctx context.Context) (map[params.Cloud]jujuparams.Cloud, error) {
	var resp jujuparams.CloudsResult
	if err := c.APICall("Cloud", 1, "", "Clouds", nil, &resp); err != nil {
		return nil, newAPIError(err)
	}

	clouds := make(map[params.Cloud]jujuparams.Cloud, len(resp.Clouds))
	for cloudTag, cloud := range resp.Clouds {
		tag, err := names.ParseCloudTag(cloudTag)
		if err != nil {
			zapctx.Warn(ctx, "controller returned invalid cloud tag", zap.String("tag", cloudTag))
			continue
		}
		clouds[conv.FromCloudTag(tag)] = cloud
	}
	return clouds, nil
}

// AddCloud adds the given cloud to a controller with the given name.
// AddCloud uses the AddCloud procedure on the Cloud facade version 2.
func (c *Conn) AddCloud(ctx context.Context, name params.Cloud, cloud jujuparams.Cloud) error {
	args := jujuparams.AddCloudArgs{
		Cloud: cloud,
		Name:  string(name),
	}
	if err := c.APICall("Cloud", 2, "", "AddCloud", &args, nil); err != nil {
		return newAPIError(err)
	}
	return nil
}

// RemoveCloud removes the given cloud from the controller. RemoveCloud
// uses the RemoveClouds procedure on the Cloud facade version 2.
func (c *Conn) RemoveCloud(ctx context.Context, cloud params.Cloud) error {
	args := jujuparams.Entities{
		Entities: []jujuparams.Entity{{
			Tag: conv.ToCloudTag(cloud).String(),
		}},
	}
	var resp jujuparams.ErrorResults
	if err := c.APICall("Cloud", 2, "", "RemoveClouds", &args, &resp); err != nil {
		return newAPIError(err)
	}
	if len(resp.Results) != 1 {
		return errgo.Newf("unexpected number of results (expected 1, got %d)", len(resp.Results))
	}
	return newAPIError(resp.Results[0].Error)
}

// GrantCloudAccess gives the given user the given access level on the
// given cloud. GrantCloudAccess uses the ModifyCloudAccess procedure on
// the Cloud facade version 3.
func (c *Conn) GrantCloudAccess(ctx context.Context, cloud params.Cloud, user params.User, access string) error {
	args := jujuparams.ModifyCloudAccessRequest{
		Changes: []jujuparams.ModifyCloudAccess{{
			UserTag:  conv.ToUserTag(user).String(),
			Action:   jujuparams.GrantCloudAccess,
			Access:   access,
			CloudTag: conv.ToCloudTag(cloud).String(),
		}},
	}

	var resp jujuparams.ErrorResults
	err := c.APICall("Cloud", 3, "", "ModifyCloudAccess", &args, &resp)
	if err != nil {
		return newAPIError(err)
	}
	if len(resp.Results) != 1 {
		return errgo.Newf("unexpected number of results (expected 1, got %d)", len(resp.Results))
	}
	return newAPIError(resp.Results[0].Error)
}

// RevokeCloudAccess revokes the given access level on the given cloud from
// the given user. RevokeCloudAccess uses the ModifyCloudAccess procedure
// on the Cloud facade version 3.
func (c *Conn) RevokeCloudAccess(ctx context.Context, cloud params.Cloud, user params.User, access string) error {
	args := jujuparams.ModifyCloudAccessRequest{
		Changes: []jujuparams.ModifyCloudAccess{{
			UserTag:  conv.ToUserTag(user).String(),
			Action:   jujuparams.RevokeCloudAccess,
			Access:   access,
			CloudTag: conv.ToCloudTag(cloud).String(),
		}},
	}

	var resp jujuparams.ErrorResults
	err := c.APICall("Cloud", 3, "", "ModifyCloudAccess", &args, &resp)
	if err != nil {
		return newAPIError(err)
	}
	if len(resp.Results) != 1 {
		return errgo.Newf("unexpected number of results (expected 1, got %d)", len(resp.Results))
	}
	return newAPIError(resp.Results[0].Error)
}
