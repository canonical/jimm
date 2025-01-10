// Copyright 2025 Canonical.

package jimm

import (
	"context"
	"fmt"

	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v5"

	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/openfga"
	ofganames "github.com/canonical/jimm/v3/internal/openfga/names"
	jimmnames "github.com/canonical/jimm/v3/pkg/names"
)

// AddServiceAccount checks that no one owns the service account yet
// and then adds a relation between the logged in user and the service account.
func (j *JIMM) AddServiceAccount(ctx context.Context, u *openfga.User, clientId string) error {
	op := errors.Op("jimm.AddServiceAccount")

	svcTag := jimmnames.NewServiceAccountTag(clientId)
	key := openfga.Tuple{
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(svcTag),
	}
	keyWithUser := key
	keyWithUser.Object = ofganames.ConvertTag(u.ResourceTag())

	ok, err := j.OpenFGAClient.CheckRelation(ctx, keyWithUser, false)
	if err != nil {
		return errors.E(op, err)
	}
	// If the user already has administration permission over the
	// service account then return early.
	if ok {
		return nil
	}

	tuples, _, err := j.OpenFGAClient.ReadRelatedObjects(ctx, key, 10, "")
	if err != nil {
		return errors.E(op, err)
	}
	if len(tuples) > 0 {
		return errors.E(op, "service account already owned")
	}
	addTuple := openfga.Tuple{
		Object:   ofganames.ConvertTag(u.ResourceTag()),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(svcTag),
	}
	err = j.OpenFGAClient.AddRelation(ctx, addTuple)
	if err != nil {
		return errors.E(op, err)
	}
	return nil
}

// CopyServiceAccountCredential attempts to create a copy of a user's cloud-credential
// for a service account.
func (j *JIMM) CopyServiceAccountCredential(ctx context.Context, u *openfga.User, svcAcc *openfga.User, cred names.CloudCredentialTag) (names.CloudCredentialTag, []jujuparams.UpdateCredentialModelResult, error) {
	op := errors.Op("jimm.AddServiceAccountCredential")

	credential, err := j.GetCloudCredential(ctx, u, cred)
	if err != nil {
		return names.CloudCredentialTag{}, nil, errors.E(op, err)
	}
	attr, err := j.getCloudCredentialAttributes(ctx, credential)
	if err != nil {
		return names.CloudCredentialTag{}, nil, errors.E(op, err)
	}
	newCredID := fmt.Sprintf("%s/%s/%s", cred.Cloud().Id(), svcAcc.Name, cred.Name())
	if !names.IsValidCloudCredential(newCredID) {
		return names.CloudCredentialTag{}, nil, errors.E(op, fmt.Sprintf("new credential ID %s is not a valid cloud credential tag", newCredID))
	}
	newCredential := jujuparams.CloudCredential{
		AuthType:   credential.AuthType,
		Attributes: attr,
	}
	newTag := names.NewCloudCredentialTag(newCredID)
	modelRes, err := j.UpdateCloudCredential(ctx, svcAcc, UpdateCloudCredentialArgs{
		CredentialTag: names.NewCloudCredentialTag(newCredID),
		Credential:    newCredential,
		SkipCheck:     false,
		SkipUpdate:    false,
	})
	return newTag, modelRes, err
}
