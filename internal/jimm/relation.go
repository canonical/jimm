// Copyright 2020 Canonical Ltd.

package jimm

import (
	"context"
	"fmt"

	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"

	apiparams "github.com/canonical/jimm/api/params"
	"github.com/canonical/jimm/internal/errors"
	"github.com/canonical/jimm/internal/openfga"
	ofganames "github.com/canonical/jimm/internal/openfga/names"
)

// AddRelation checks user permission and add given relations tuples.
// At the moment user is required be admin.
func (j *JIMM) AddRelation(ctx context.Context, user *openfga.User, tuples []apiparams.RelationshipTuple) error {
	const op = errors.Op("jimm.AddRelation")
	if !user.JimmAdmin {
		return errors.E(op, errors.CodeUnauthorized, "unauthorized")
	}
	parsedTuples, err := j.parseTuples(ctx, tuples)
	if err != nil {
		return errors.E(err)
	}
	err = j.OpenFGAClient.AddRelation(ctx, parsedTuples...)
	if err != nil {
		return errors.E(op, errors.CodeOpenFGARequestFailed, err)
	}
	return nil
}

// RemoveRelation checks user permission and remove given relations tuples.
// At the moment user is required be admin.
func (j *JIMM) RemoveRelation(ctx context.Context, user *openfga.User, tuples []apiparams.RelationshipTuple) error {
	const op = errors.Op("jimm.RemoveRelation")
	if !user.JimmAdmin {
		return errors.E(op, errors.CodeUnauthorized, "unauthorized")
	}
	parsedTuples, err := j.parseTuples(ctx, tuples)
	if err != nil {
		return errors.E(op, err)
	}
	err = j.OpenFGAClient.RemoveRelation(ctx, parsedTuples...)
	if err != nil {
		return errors.E(op, errors.CodeOpenFGARequestFailed, err)
	}
	return nil
}

// CheckRelation checks user permission and return true if the given tuple exists.
// At the moment user is required be admin or checking its own relations
func (j *JIMM) CheckRelation(ctx context.Context, user *openfga.User, tuple apiparams.RelationshipTuple, trace bool) (_ bool, err error) {
	const op = errors.Op("jimm.CheckRelation")
	allowed := false
	parsedTuple, err := j.parseTuple(ctx, tuple)
	if err != nil {
		return false, errors.E(op, err)
	}
	userCheckingSelf := parsedTuple.Object.Kind == openfga.UserType && parsedTuple.Object.ID == user.Name
	// Admins can check any relation, non-admins can only check their own.
	if !(user.JimmAdmin || userCheckingSelf) {
		return allowed, errors.E(op, errors.CodeUnauthorized, "unauthorized")
	}

	allowed, err = j.OpenFGAClient.CheckRelation(ctx, *parsedTuple, trace)
	if err != nil {
		return allowed, errors.E(op, errors.CodeOpenFGARequestFailed, err)
	}
	return allowed, nil
}

// ListRelationshipTuples checks user permission and remove given relations tuples.
// At the moment user is required be admin
func (j *JIMM) ListRelationshipTuples(ctx context.Context, user *openfga.User, tuple apiparams.RelationshipTuple, pageSize int32, continuationToken string) ([]openfga.Tuple, string, error) {
	const op = errors.Op("jujuapi.ListRelationshipTuples")
	if !user.JimmAdmin {
		return []openfga.Tuple{}, "", errors.E(op, errors.CodeUnauthorized, "unauthorized")
	}
	parsedTuple := &openfga.Tuple{}
	var err error
	if tuple.TargetObject != "" {
		parsedTuple, err = j.parseTuple(ctx, tuple)
		if err != nil {
			return []openfga.Tuple{}, "", errors.E(op, err)
		}
	}

	responseTuples, ct, err := j.OpenFGAClient.ReadRelatedObjects(ctx, *parsedTuple, pageSize, continuationToken)
	if err != nil {
		return []openfga.Tuple{}, "", errors.E(op, err)
	}
	return responseTuples, ct, nil
}

// parseTuples translate the api request struct containing tuples to a slice of openfga tuple keys.
// This method utilises the parseTuple method which does all the heavy lifting.
func (j *JIMM) parseTuples(ctx context.Context, tuples []apiparams.RelationshipTuple) ([]openfga.Tuple, error) {
	keys := make([]openfga.Tuple, 0, len(tuples))
	for _, tuple := range tuples {
		key, err := j.parseTuple(ctx, tuple)
		if err != nil {
			return nil, errors.E(err)
		}
		keys = append(keys, *key)
	}
	return keys, nil
}

// parseTuple takes the initial tuple from a relational request and ensures that
// whatever format, be it JAAS or Juju tag, is resolved to the correct identifier
// to be persisted within OpenFGA.
func (j *JIMM) parseTuple(ctx context.Context, tuple apiparams.RelationshipTuple) (*openfga.Tuple, error) {
	const op = errors.Op("jujuapi.parseTuple")

	relation, err := ofganames.ParseRelation(tuple.Relation)
	if err != nil {
		return nil, errors.E(op, err, errors.CodeBadRequest)
	}
	t := openfga.Tuple{
		Relation: relation,
	}

	// Wraps the general error that will be sent for both
	// the object and target object, but changing the message and key
	// to be specific to the erroneous offender.
	parseTagError := func(msg string, key string, err error) error {
		zapctx.Debug(ctx, msg, zap.String("key", key), zap.Error(err))
		return errors.E(op, errors.CodeFailedToParseTupleKey, fmt.Sprintf("%s %s: %s", msg, key, err.Error()))
	}

	if tuple.TargetObject == "" {
		return nil, errors.E(op, errors.CodeBadRequest, "target object not specified")
	}
	if tuple.TargetObject != "" {
		targetTag, err := j.parseAndValidateTag(ctx, tuple.TargetObject)
		if err != nil {
			return nil, parseTagError("failed to parse tuple target object key", tuple.TargetObject, err)
		}
		t.Target = targetTag
	}
	if tuple.Object != "" {
		objectTag, err := j.parseAndValidateTag(ctx, tuple.Object)
		if err != nil {
			return nil, parseTagError("failed to parse tuple object key", tuple.Object, err)
		}
		t.Object = objectTag
	}

	return &t, nil
}
