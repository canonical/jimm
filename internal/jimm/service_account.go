// Copyright 2024 Canonical Ltd.

package jimm

import (
	"context"

	"github.com/canonical/jimm/internal/errors"
	"github.com/canonical/jimm/internal/openfga"
	ofganames "github.com/canonical/jimm/internal/openfga/names"
	"github.com/canonical/jimm/pkg/names"
)

// AddServiceAccount checks that a user doesn't already own the service account
// and then adds a relation between the user and the service account.
func (j *JIMM) AddServiceAccount(ctx context.Context, u *openfga.User, clientId string) (err error) {
	op := errors.Op("jimm.AddServiceAccount")
	svcTag := names.NewServiceAccountTag(clientId)
	key := openfga.Tuple{
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(svcTag),
	}
	tuples, _, err := j.AuthorizationClient().ReadRelatedObjects(ctx, key, 10, "")
	if err != nil {
		return errors.E(op, err)
	}
	if len(tuples) > 0 {
		return errors.E(op, "service account already owned by another user")
	}
	addTuple := openfga.Tuple{
		Object:   ofganames.ConvertTag(u.ResourceTag()),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(svcTag),
	}
	err = j.AuthorizationClient().AddRelation(ctx, addTuple)
	if err != nil {
		return errors.E(op, err)
	}
	return nil
}
