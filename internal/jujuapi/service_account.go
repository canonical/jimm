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
	ofganames "github.com/canonical/jimm/internal/openfga/names"
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

// UpdateServiceAccountCredentialsCheckModels updates a set of cloud credentials' content.
// If there are any models that are using a credential and these models
// are not going to be visible with updated credential content,
// there will be detailed validation errors per model.
//
// This method checks that the authenticated user has permission to manage the service account.
func (r *controllerRoot) UpdateServiceAccountCredentials(ctx context.Context, req apiparams.UpdateServiceAccountCredentialsRequest) (jujuparams.UpdateCredentialResults, error) {
	const op = errors.Op("jujuapi.UpdateServiceAccountCredentials")

	if !jimmnames.IsValidServiceAccountId(req.ID) {
		return jujuparams.UpdateCredentialResults{}, errors.E(op, errors.CodeBadRequest, "invalid client ID")
	}

	tuple := openfga.Tuple{
		Object:   ofganames.ConvertTag(r.user.ResourceTag()),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(jimmnames.NewServiceAccountTag(req.ID)),
	}
	ok, err := r.jimm.AuthorizationClient().CheckRelation(ctx, tuple, false)
	if err != nil {
		return jujuparams.UpdateCredentialResults{}, errors.E(op, errors.CodeOpenFGARequestFailed, "unable to determine permissions")
	}
	if !ok {
		return jujuparams.UpdateCredentialResults{}, errors.E(op, errors.CodeUnauthorized, "unauthorized")
	}

	var targetUserModel dbmodel.User
	targetUserModel.SetTag(names.NewUserTag(req.ID))
	targetUser := openfga.NewUser(&targetUserModel, r.jimm.AuthorizationClient())

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
