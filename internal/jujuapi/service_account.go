// Copyright 2024 canonical.

package jujuapi

import (
	"context"

	jujuparams "github.com/juju/juju/rpc/params"
)

// service_acount contains the primary RPC commands for handling service accounts within JIMM via the JIMM facade itself.

// AddServiceAccountCredentials adds cloud credentials to a service account.
// It serves merely as a wrapper
func (r *controllerRoot) AddServiceAccountCredentials(ctx context.Context, args jujuparams.TaggedCredentials) (jujuparams.ErrorResults, error) {
	return r.AddCredentials(ctx, args)
}

// UpdateServiceAccountCredentialsCheckModels updates a set of cloud credentials' content.
// If there are any models that are using a credential and these models
// are not going to be visible with updated credential content,
// there will be detailed validation errors per model.
//
// This method is identical to the user variant of UpdateCredentialsCheckModels, however,
// it is explicitly targeting service accounts. The underlying updates to the local
// database and controllers where the credential is used are the same.
func (r *controllerRoot) UpdateServiceAccountCredentialsCheckModels(ctx context.Context, args jujuparams.UpdateCredentialArgs) (jujuparams.UpdateCredentialResults, error) {
	return r.updateCredentials(ctx, args.Credentials, args.Force, false)
}
