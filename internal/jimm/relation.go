package jimm

import (
	"context"

	"github.com/canonical/jimm/internal/errors"
	"github.com/canonical/jimm/internal/openfga"
)

// AddRelation checks user permission and add given relations tuples.
// At the moment user is required be admin.
func (j *JIMM) AddRelation(ctx context.Context, user *openfga.User, tuples []openfga.Tuple) error {
	const op = errors.Op("jimm.AddRelation")
	if !user.JimmAdmin {
		return errors.E(op, errors.CodeUnauthorized, "unauthorized")
	}
	err := j.OpenFGAClient.AddRelation(ctx, tuples...)
	if err != nil {
		return errors.E(op, errors.CodeOpenFGARequestFailed, err)
	}
	return nil
}

// RemoveRelation checks user permission and remove given relations tuples.
// At the moment user is required be admin.
func (j *JIMM) RemoveRelation(ctx context.Context, user *openfga.User, tuples []openfga.Tuple) error {
	const op = errors.Op("jimm.RemoveRelation")
	if !user.JimmAdmin {
		return errors.E(op, errors.CodeUnauthorized, "unauthorized")
	}
	err := j.OpenFGAClient.RemoveRelation(ctx, tuples...)
	if err != nil {
		return errors.E(op, errors.CodeOpenFGARequestFailed, err)
	}
	return nil
}

// CheckRelation checks user permission and return if the given tuple exists.
// At the moment user is required be admin or checking its own relations
func (j *JIMM) CheckRelation(ctx context.Context, user *openfga.User, tuple openfga.Tuple, trace bool) (_ bool, err error) {
	const op = errors.Op("jimm.CheckRelation")
	allowed := false
	userCheckingSelf := tuple.Object.Kind == openfga.UserType && tuple.Object.ID == user.Name
	// Admins can check any relation, non-admins can only check their own.
	if !(user.JimmAdmin || userCheckingSelf) {
		return allowed, errors.E(op, errors.CodeUnauthorized, "unauthorized")
	}
	allowed, err = j.OpenFGAClient.CheckRelation(ctx, tuple, trace)
	if err != nil {
		return allowed, errors.E(op, errors.CodeOpenFGARequestFailed, err)
	}
	return allowed, nil
}

// ListRelationshipTuples checks user permission and remove given relations tuples.
// At the moment user is required be admin
func (j *JIMM) ListRelationshipTuples(ctx context.Context, user *openfga.User, tuple openfga.Tuple, pageSize int32, continuationToken string) ([]openfga.Tuple, string, error) {
	const op = errors.Op("jujuapi.ListRelationshipTuples")
	if !user.JimmAdmin {
		return []openfga.Tuple{}, "", errors.E(op, errors.CodeUnauthorized, "unauthorized")
	}

	responseTuples, ct, err := j.OpenFGAClient.ReadRelatedObjects(ctx, tuple, pageSize, continuationToken)
	if err != nil {
		return []openfga.Tuple{}, "", errors.E(op, err)
	}
	return responseTuples, ct, nil
}
