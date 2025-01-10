// Copyright 2025 Canonical.

package mocks

import (
	"context"

	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v5"

	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/openfga"
)

// ServiceAccountManager is an implementation of the jimm.ServiceAccountManager interface.
type ServiceAccountManager struct {
	AddServiceAccount_            func(ctx context.Context, u *openfga.User, clientId string) error
	CopyServiceAccountCredential_ func(ctx context.Context, u *openfga.User, svcAcc *openfga.User, cred names.CloudCredentialTag) (names.CloudCredentialTag, []jujuparams.UpdateCredentialModelResult, error)
}

func (s *ServiceAccountManager) AddServiceAccount(ctx context.Context, u *openfga.User, clientId string) error {
	if s.AddServiceAccount_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return s.AddServiceAccount_(ctx, u, clientId)
}
func (s *ServiceAccountManager) CopyServiceAccountCredential(ctx context.Context, u *openfga.User, svcAcc *openfga.User, cred names.CloudCredentialTag) (names.CloudCredentialTag, []jujuparams.UpdateCredentialModelResult, error) {
	if s.CopyServiceAccountCredential_ == nil {
		return names.CloudCredentialTag{}, nil, errors.E(errors.CodeNotImplemented)
	}
	return s.CopyServiceAccountCredential_(ctx, u, svcAcc, cred)
}
