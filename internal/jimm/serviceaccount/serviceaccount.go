// Copyright 2025 Canonical.

package serviceaccount

import (
	"context"

	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v5"

	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/openfga"
	ofganames "github.com/canonical/jimm/v3/internal/openfga/names"
	jimmnames "github.com/canonical/jimm/v3/pkg/names"
)

// CredentialCopier defines how the service account manager can copy
// a user's own credentials to a service account.
type CredentialCopier interface {
	CopyCredential(ctx context.Context, originalUser *openfga.User, newUser *openfga.User, cred names.CloudCredentialTag) (names.CloudCredentialTag, []jujuparams.UpdateCredentialModelResult, error)
}

// serviceAccountManager provides a means to manage service
// accounts within JIMM.
type serviceAccountManager struct {
	store      *db.Database
	authSvc    *openfga.OFGAClient
	credCopier CredentialCopier
}

// NewServiceAccountManager returns a new serviceAccountManager that
// provides methods to manage service accounts.
func NewServiceAccountManager(store *db.Database, authSvc *openfga.OFGAClient, cc CredentialCopier) (*serviceAccountManager, error) {
	if store == nil {
		return nil, errors.E("role store cannot be nil")
	}
	if authSvc == nil {
		return nil, errors.E("role authorisation service cannot be nil")
	}
	if cc == nil {
		return nil, errors.E("credential copier service cannot be nil")
	}
	return &serviceAccountManager{store, authSvc, cc}, nil
}

// AddServiceAccount checks that no one owns the service account yet
// and then adds a relation between the logged in user and the service account.
func (s *serviceAccountManager) AddServiceAccount(ctx context.Context, u *openfga.User, clientId string) error {
	op := errors.Op("jimm.AddServiceAccount")

	svcTag := jimmnames.NewServiceAccountTag(clientId)
	key := openfga.Tuple{
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(svcTag),
	}
	keyWithUser := key
	keyWithUser.Object = ofganames.ConvertTag(u.ResourceTag())

	ok, err := s.authSvc.CheckRelation(ctx, keyWithUser, false)
	if err != nil {
		return errors.E(op, err)
	}
	// If the user already has administration permission over the
	// service account then return early.
	if ok {
		return nil
	}

	tuples, _, err := s.authSvc.ReadRelatedObjects(ctx, key, 10, "")
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
	err = s.authSvc.AddRelation(ctx, addTuple)
	if err != nil {
		return errors.E(op, err)
	}
	return nil
}

// CopyServiceAccountCredential attempts to create a copy of a user's cloud-credential
// for a service account.
func (s *serviceAccountManager) CopyServiceAccountCredential(ctx context.Context, u *openfga.User, svcAcc *openfga.User, cred names.CloudCredentialTag) (names.CloudCredentialTag, []jujuparams.UpdateCredentialModelResult, error) {
	return s.credCopier.CopyCredential(ctx, u, svcAcc, cred)
}
