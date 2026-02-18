// Copyright 2025 Canonical.

package permissions

import (
	"context"
	"fmt"

	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"

	"github.com/canonical/jimm/v3/internal/common/pagination"
	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/logger"
	"github.com/canonical/jimm/v3/internal/openfga"
	ofganames "github.com/canonical/jimm/v3/internal/openfga/names"
	apiparams "github.com/canonical/jimm/v3/pkg/api/params"
)

// BATCH_SIZE_OPENFGA defines the maximum number of tuples to process in a single batch operation.
// This is the default for maxTuplesPerWrite
// See: https://openfga.dev/docs/getting-started/setup-openfga/configuration
// TODO: this value should be received from the OpenFGA charm's relation, so we make sure that we batch
// requests according to the deployed OpenFGA instance configuration.
const BATCH_SIZE_OPENFGA = 100

// AddRelation checks user permission and add given relations tuples.
// At the moment user is required be admin.
func (j *PermissionManager) AddRelation(ctx context.Context, user *openfga.User, tuples []apiparams.RelationshipTuple) error {

	if !user.JimmAdmin {
		return errors.E(errors.CodeUnauthorized, "unauthorized")
	}
	parsedTuples, err := j.parseTuples(ctx, tuples)
	if err != nil {
		return errors.E(err)
	}
	for i := 0; i < len(parsedTuples); i += BATCH_SIZE_OPENFGA {
		end := i + BATCH_SIZE_OPENFGA
		if end > len(parsedTuples) {
			end = len(parsedTuples)
		}
		batch := parsedTuples[i:end]

		err = j.authSvc.AddRelation(ctx, batch...)
		if err != nil {
			return errors.E(errors.CodeOpenFGARequestFailed, err)
		}
		j.logUserUpdates(ctx, user, batch, true)
	}
	return nil
}

// RemoveRelation checks user permission and remove given relations tuples.
// At the moment user is required be admin.
func (j *PermissionManager) RemoveRelation(ctx context.Context, user *openfga.User, tuples []apiparams.RelationshipTuple) error {

	if !user.JimmAdmin {
		return errors.E(errors.CodeUnauthorized, "unauthorized")
	}
	parsedTuples, err := j.parseTuples(ctx, tuples)
	if err != nil {
		return errors.E(err)
	}
	for i := 0; i < len(parsedTuples); i += BATCH_SIZE_OPENFGA {
		end := i + BATCH_SIZE_OPENFGA
		if end > len(parsedTuples) {
			end = len(parsedTuples)
		}
		batch := parsedTuples[i:end]

		err = j.authSvc.RemoveRelation(ctx, batch...)
		if err != nil {
			return errors.E(errors.CodeOpenFGARequestFailed, err)
		}
		j.logUserUpdates(ctx, user, batch, true)
	}
	return nil
}

// CheckRelation checks user permission and return true if the given tuple exists.
// At the moment user is required be admin or checking its own relations.
func (j *PermissionManager) CheckRelation(ctx context.Context, user *openfga.User, tuple apiparams.RelationshipTuple, trace bool) (_ bool, err error) {

	allowed := false
	parsedTuple, err := j.parseTuple(ctx, tuple)
	if err != nil {
		return false, errors.E(err)
	}
	userCheckingSelf := parsedTuple.Object.Kind == openfga.UserType && parsedTuple.Object.ID == user.Name
	// Admins can check any relation, non-admins can only check their own.
	if !user.JimmAdmin && !userCheckingSelf {
		return allowed, errors.E(errors.CodeUnauthorized, "unauthorized")
	}

	allowed, err = j.authSvc.CheckRelation(ctx, *parsedTuple, trace)
	if err != nil {
		return allowed, errors.E(errors.CodeOpenFGARequestFailed, err)
	}
	return allowed, nil
}

// CheckRelations checks user permissions and returns a slice of CheckResult for each tuple.
// At the moment the implementation is a simple loop around CheckRelation.
// TODO(simonedutto): this is a temporary implementation, once canonical/openfga supports BatchCheck
// we can use that to improve performance.
func (j *PermissionManager) CheckRelations(ctx context.Context, user *openfga.User, tuples []apiparams.RelationshipTuple) ([]openfga.CheckResult, error) {
	var results []openfga.CheckResult
	var err error
	for _, tuple := range tuples {
		var result openfga.CheckResult
		result.Allowed, err = j.CheckRelation(ctx, user, tuple, false)
		if err != nil {
			result.Error = err
		}
		results = append(results, result)
	}

	return results, nil
}

// ListRelationshipTuples checks user permission and lists relationship tuples based of tuple struct with pagination.
// Listing filters can be relaxed: optionally exclude tuple.Relation or tuple.Object or specify only tuple.TargetObject.Kind.
func (j *PermissionManager) ListRelationshipTuples(ctx context.Context, user *openfga.User, tuple apiparams.RelationshipTuple, pageSize int32, continuationToken string) ([]openfga.Tuple, string, error) {

	if !user.JimmAdmin {
		return nil, "", errors.E(errors.CodeUnauthorized, "unauthorized")
	}
	// if targetObject is not specified returns all tuples.
	parsedTuple := &openfga.Tuple{}
	var err error
	if tuple.TargetObject != "" {
		parsedTuple, err = j.parseTuple(ctx, tuple)
		if err != nil {
			return nil, "", errors.E(err)
		}
	} else if tuple.Object != "" {
		return nil, "", errors.E(errors.CodeBadRequest, "it is invalid to pass an object without a target object.")
	}

	responseTuples, ct, err := j.authSvc.ReadRelatedObjects(ctx, *parsedTuple, pageSize, continuationToken)
	if err != nil {
		return nil, "", errors.E(err)
	}
	return responseTuples, ct, nil
}

// ListObjectRelations lists all the tuples that an object has a direct relation with.
// Useful for listing all the resources that a group or user have access to.
//
// This functions provides a slightly higher-level abstraction in favor of ListRelationshipTuples.
func (j *PermissionManager) ListObjectRelations(ctx context.Context, user *openfga.User, object string, pageSize int32, entitlementToken pagination.EntitlementToken) ([]openfga.Tuple, pagination.EntitlementToken, error) {

	var e pagination.EntitlementToken
	if !user.JimmAdmin {
		return nil, e, errors.E(errors.CodeUnauthorized, "unauthorized")
	}
	responseTuples, nextToken, err := j.getObjectRelationsPage(ctx, object, pageSize, entitlementToken)
	if err != nil {
		return nil, e, errors.E(err)
	}
	// verify next page contains some entries. Otherwise return empty nextToken.
	if len(responseTuples) == int(pageSize) && nextToken.String() != "" {
		responseTuples, _, err := j.getObjectRelationsPage(ctx, object, 1, nextToken)
		if err != nil {
			return nil, e, errors.E("error getting next page to verify it cointains something", err)
		}
		if len(responseTuples) == 0 {
			nextToken = pagination.EntitlementToken{}
		}
	}
	return responseTuples, nextToken, nil
}

// ListResources returns a list of resources known to JIMM with a pagination filter.
func (j *PermissionManager) ListResources(ctx context.Context, user *openfga.User, filter pagination.LimitOffsetPagination, namePrefixFilter, typeFilter string) ([]db.Resource, error) {

	if !user.JimmAdmin {
		return nil, errors.E(errors.CodeUnauthorized, "unauthorized")
	}

	return j.store.ListResources(ctx, filter.Limit(), filter.Offset(), namePrefixFilter, typeFilter)
}

func (j *PermissionManager) getObjectRelationsPage(ctx context.Context, object string, pageSize int32, entitlementToken pagination.EntitlementToken) ([]openfga.Tuple, pagination.EntitlementToken, error) {
	var err error
	var e pagination.EntitlementToken
	tuple := &openfga.Tuple{}
	tuple.Object, err = j.parseAndValidateTag(ctx, object)
	if err != nil {
		return nil, e, err
	}
	var responseTuples []openfga.Tuple
	nextToken := entitlementToken
	// loop around entity kinds, each with a different continuation token.
	for {
		nextContinuationToken, kind, err := pagination.DecodeEntitlementToken(nextToken)
		if err != nil {
			return nil, e, err
		}
		tuple.Target, err = j.parseAndValidateTag(ctx, kind.String())
		if err != nil {
			return nil, e, err
		}
		t, nextContinuationToken, err := j.authSvc.ReadRelatedObjects(ctx, *tuple, pageSize, nextContinuationToken)
		if err != nil {
			return nil, e, err
		}
		responseTuples = append(responseTuples, t...)
		// nolint:gosec
		pageSize -= int32(len(t))
		nextToken, err = pagination.NextEntitlementToken(kind, nextContinuationToken)
		if err != nil {
			return nil, e, err
		}
		// break on a full page or no other entries.
		if pageSize <= 0 || nextToken.String() == "" {
			break
		}
	}
	return responseTuples, nextToken, nil
}

// parseTuples translate the api request struct containing tuples to a slice of openfga tuple keys.
// This method utilises the parseTuple method which does all the heavy lifting.
func (j *PermissionManager) parseTuples(ctx context.Context, tuples []apiparams.RelationshipTuple) ([]openfga.Tuple, error) {
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
func (j *PermissionManager) parseTuple(ctx context.Context, tuple apiparams.RelationshipTuple) (*openfga.Tuple, error) {

	relation, err := ofganames.ParseRelation(tuple.Relation)
	if err != nil {
		return nil, errors.E(err, errors.CodeBadRequest)
	}
	t := openfga.Tuple{
		Relation: relation,
	}

	// Wraps the general error that will be sent for both
	// the object and target object, but changing the message and key
	// to be specific to the erroneous offender.
	parseTagError := func(msg string, key string, err error) error {
		zapctx.Debug(ctx, msg, zap.String("key", key), zap.Error(err))
		return errors.E(errors.CodeFailedToParseTupleKey, fmt.Sprintf("%s %s: %s", msg, key, err.Error()))
	}

	if tuple.TargetObject == "" {
		return nil, errors.E(errors.CodeBadRequest, "target object not specified")
	}
	t.Target, err = j.parseAndValidateTag(ctx, tuple.TargetObject)
	if err != nil {
		return nil, parseTagError("failed to parse tuple target object key", tuple.TargetObject, err)
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

// logUserUpdates logs tuple relation changes if they are for a user.
// This should be the closest equivalent of logging an RBAC role in our Zanzibar-style openfga graph.
func (j *PermissionManager) logUserUpdates(ctx context.Context, user *openfga.User, tuples []openfga.Tuple, isAddition bool) {
	for _, tuple := range tuples {
		if tuple.Object.Kind.String() == openfga.UserType.String() {
			logger.LogUserUpdated(ctx, user.Name, tuple.Object.ID, tuple.Relation.String(), tuple.Target.ID, isAddition)
		}
	}
}
