// Copyright 2024 Canonical Ltd.

package jimm

import (
	"context"

	"github.com/canonical/jimm/internal/errors"
	"github.com/canonical/jimm/internal/openfga"
	ofganames "github.com/canonical/jimm/internal/openfga/names"
	jimmnames "github.com/canonical/jimm/pkg/names"
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
