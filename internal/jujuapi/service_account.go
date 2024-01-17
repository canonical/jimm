// Copyright 2024 canonical.

package jujuapi

import (
	"context"

	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v4"

	apiparams "github.com/canonical/jimm/api/params"
	"github.com/canonical/jimm/internal/dbmodel"
	"github.com/canonical/jimm/internal/errors"
	"github.com/canonical/jimm/internal/jimm"
	"github.com/canonical/jimm/internal/openfga"
	jimmnames "github.com/canonical/jimm/pkg/names"
)

// service_acount contains the primary RPC commands for handling service accounts within JIMM via the JIMM facade itself.

// AddGroup creates a group within JIMMs DB for reference by OpenFGA.
func (r *controllerRoot) AddServiceAccount(ctx context.Context, req apiparams.AddServiceAccountRequest) error {
	const op = errors.Op("jujuapi.AddGroup")

	if !jimmnames.IsValidServiceAccountId(req.ClientID) {
		return errors.E(op, errors.CodeBadRequest, "invalid client ID")
	}

	return r.jimm.AddServiceAccount(ctx, r.user, req.ClientID)
}

// getServiceAccount validates the incoming identity has administrator permission
// on the service account and returns the service account identity.
func (r *controllerRoot) getServiceAccount(ctx context.Context, clientID string) (*openfga.User, error) {
	if !jimmnames.IsValidServiceAccountId(clientID) {
		return nil, errors.E(errors.CodeBadRequest, "invalid client ID")
	}

	ok, err := r.user.IsServiceAccountAdmin(ctx, jimmnames.NewServiceAccountTag(clientID))
	if err != nil {
		return nil, errors.E(err)
	}
	if !ok {
		return nil, errors.E("unauthorized")
	}

	var targetIdentityModel dbmodel.Identity
	targetIdentityModel.SetTag(names.NewUserTag(clientID))
	if err := r.jimm.DB().GetIdentity(ctx, &targetIdentityModel); err != nil {
		return nil, errors.E(err)
	}
	return openfga.NewUser(&targetIdentityModel, r.jimm.AuthorizationClient()), nil
}

// UpdateServiceAccountCredentialsCheckModels updates a set of cloud credentials' content.
// If there are any models that are using a credential and these models
// are not going to be visible with updated credential content,
// there will be detailed validation errors per model.
//
// This method checks that the authenticated user has permission to manage the service account.
func (r *controllerRoot) UpdateServiceAccountCredentials(ctx context.Context, req apiparams.UpdateServiceAccountCredentialsRequest) (jujuparams.UpdateCredentialResults, error) {
	const op = errors.Op("jujuapi.UpdateServiceAccountCredentials")

	targetUser, err := r.getServiceAccount(ctx, req.ClientID)
	if err != nil {
		return jujuparams.UpdateCredentialResults{}, errors.E(op, err)
	}

	results := jujuparams.UpdateCredentialResults{
		Results: make([]jujuparams.UpdateCredentialResult, len(req.Credentials)),
	}
	for i, credential := range req.Credentials {
		var res []jujuparams.UpdateCredentialModelResult
		var err error
		var tag names.CloudCredentialTag
		tag, err = names.ParseCloudCredentialTag(credential.Tag)
		if err == nil {
			res, err = r.jimm.UpdateCloudCredential(ctx, targetUser, jimm.UpdateCloudCredentialArgs{
				CredentialTag: tag,
				Credential:    credential.Credential,
				// Check that all credentials are valid.
				SkipCheck: false,
				// Update all credentials on target controllers.
				SkipUpdate: false,
			})
		}
		results.Results[i] = jujuparams.UpdateCredentialResult{
			CredentialTag: credential.Tag,
			Error:         mapError(err),
			Models:        res,
		}
		results.Results[i].CredentialTag = credential.Tag
	}
	return results, nil
}

func (r *controllerRoot) ListServiceAccountCredentials(ctx context.Context, req apiparams.ListServiceAccountCredentialsRequest) (jujuparams.CredentialContentResults, error) {
	const op = errors.Op("jujuapi.UpdateServiceAccountCredentials")

	targetUser, err := r.getServiceAccount(ctx, req.ClientID)
	if err != nil {
		return jujuparams.CredentialContentResults{}, errors.E(op, err)
	}
	return getIdentityCredentials(ctx, targetUser, r.jimm, req.CloudCredentialArgs)
}
