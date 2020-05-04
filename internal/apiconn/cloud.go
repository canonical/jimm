// Copyright 2020 Canonical Ltd.

package apiconn

import (
	"context"

	jujuparams "github.com/juju/juju/apiserver/params"
	"gopkg.in/errgo.v1"

	"github.com/CanonicalLtd/jimm/internal/conv"
	"github.com/CanonicalLtd/jimm/internal/mongodoc"
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
