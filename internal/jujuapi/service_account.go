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
	const op = errors.Op("jujuapi.AddServiceAccount")

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
		return nil, errors.E(errors.CodeUnauthorized, "unauthorized")
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

	targetIdentity, err := r.getServiceAccount(ctx, req.ClientID)
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
			res, err = r.jimm.UpdateCloudCredential(ctx, targetIdentity, jimm.UpdateCloudCredentialArgs{
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

// ListServiceAccountCredentials lists the cloud credentials available for a service account.
func (r *controllerRoot) ListServiceAccountCredentials(ctx context.Context, req apiparams.ListServiceAccountCredentialsRequest) (jujuparams.CredentialContentResults, error) {
	const op = errors.Op("jujuapi.ListServiceAccountCredentials")

	targetIdentity, err := r.getServiceAccount(ctx, req.ClientID)
	if err != nil {
		return jujuparams.CredentialContentResults{}, errors.E(op, err)
	}
	return getIdentityCredentials(ctx, targetIdentity, r.jimm, req.CloudCredentialArgs)
}

// GrantServiceAccountAccess is the method handler for granting new users/groups with access
// to service accounts.
func (r *controllerRoot) GrantServiceAccountAccess(ctx context.Context, req apiparams.GrantServiceAccountAccess) error {
	const op = errors.Op("jujuapi.GrantServiceAccountAccess")

	targetUser, err := r.getServiceAccount(ctx, req.ClientID)
	if err != nil {
		return errors.E(op, err)
	}
	tags := make([]*ofganames.Tag, 0, len(req.Entities))
	// Validate tags
	for _, val := range req.Entities {
		tag, err := parseTag(ctx, r.jimm.ResourceTag().Id(), r.jimm.DB(), val)
		if err != nil {
			return errors.E(op, err)
		}
		if tag.Kind != openfga.UserType && tag.Kind != openfga.GroupType {
			return errors.E(op, "invalid entity - not user or group")
		}
		if tag.Kind == openfga.GroupType {
			tag.Relation = ofganames.MemberRelation
		}
		tags = append(tags, tag)
	}
	svcAccTag := jimmnames.NewServiceAccountTag(req.ClientID)

	return r.jimm.GrantServiceAccountAccess(ctx, targetUser, svcAccTag, tags)
}
