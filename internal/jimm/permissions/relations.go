// Copyright 2025 Canonical.

package permissions

import (
	"context"
	"fmt"
	"strconv"

	"github.com/canonical/ofga"
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

// resourceAdminRelations are the access-grant relations a resource administrator
// (a non-JIMM-admin who administers the target) is permitted to manage. The
// structural relations that define the resource hierarchy (controller, model)
// are deliberately excluded — only JIMM admins may change those, since they are
// set by JIMM internally and are not part of the user-facing access-grant API.
// Using an allowlist (rather than a denylist) keeps any relation added to the
// OpenFGA model in the future JIMM-admin-only until explicitly opted in.
var resourceAdminRelations = map[ofga.Relation]bool{
	ofganames.AdministratorRelation:  true,
	ofganames.ReaderRelation:         true,
	ofganames.WriterRelation:         true,
	ofganames.ConsumerRelation:       true,
	ofganames.CanAddModelRelation:    true,
	ofganames.AuditLogViewerRelation: true,
}

// grantableObjectKinds are the object kinds an access grant may be made to.
// Service accounts and public-access (user:*) are both kind `user`. Structural
// relations are the only ones whose object is a controller/model/cloud, so
// restricting grantees to these kinds independently prevents a non-admin from
// writing a structural tuple.
var grantableObjectKinds = map[openfga.Kind]bool{
	openfga.UserType:     true,
	openfga.GroupType:    true,
	openfga.IdPGroupType: true,
	openfga.RoleType:     true,
}

// authorizeRelationTargetAdmin authorizes a non-JIMM-admin to manage the given
// relation tuple. JIMM admins may manage any tuple. A non-admin may only grant
// or revoke an access relation (resourceAdminRelations) to a grantee of an
// allowed kind (grantableObjectKinds) on a resource target they administer.
//
// E.g. a non-jimm-admin who is a model admin can grant 'user-alice' with 'write' permission
// to a model but cannot remove the relation between a controller and that model.
func (j *PermissionManager) authorizeRelationTargetAdmin(ctx context.Context, user *openfga.User, tuple openfga.Tuple) error {
	if user.JimmAdmin {
		return nil
	}

	// Non-admins may only manage access-grant relations, never the structural
	// (controller/model) relations that define the resource hierarchy.
	if !resourceAdminRelations[tuple.Relation] {
		return errors.Codef(errors.CodeUnauthorized, "unauthorized")
	}

	// ...and only grant them to a user, service account, group or role.
	if tuple.Object == nil {
		return errors.Codef(errors.CodeBadRequest, "object not specified")
	}
	if !grantableObjectKinds[tuple.Object.Kind] {
		return errors.Codef(errors.CodeUnauthorized, "unauthorized")
	}

	switch tuple.Target.Kind {
	case openfga.ControllerType, openfga.ModelType, openfga.ApplicationOfferType, openfga.CloudType:
		contextualTuples, err := user.ContextualTuples()
		if err != nil {
			return err
		}
		allowed, err := j.authSvc.CheckRelation(ctx, openfga.Tuple{
			Object:   ofganames.ConvertTag(user.ResourceTag()),
			Relation: ofganames.AdministratorRelation,
			Target:   tuple.Target,
		}, false, contextualTuples...)
		if err != nil {
			return errors.Codef(errors.CodeOpenFGARequestFailed, "%w", err)
		}
		if allowed {
			return nil
		}
	case openfga.GroupType, openfga.RoleType:
		// Membership changes for groups and roles are restricted to JIMM admins.
	default:
		// Unsupported relation-management targets are rejected for non-admins.
	}

	return errors.Codef(errors.CodeUnauthorized, "unauthorized")
}

// AddRelation checks user permission and adds the given relation tuples.
// JIMM admins can update any relation, while resource administrators can update
// relations on supported resource targets.
func (j *PermissionManager) AddRelation(ctx context.Context, user *openfga.User, tuples []apiparams.RelationshipTuple) error {
	parsedTuples, err := j.parseTuples(ctx, tuples)
	if err != nil {
		return err
	}
	for _, tuple := range parsedTuples {
		if err := j.authorizeRelationTargetAdmin(ctx, user, tuple); err != nil {
			return err
		}
	}
	for i := 0; i < len(parsedTuples); i += BATCH_SIZE_OPENFGA {
		end := min(i+BATCH_SIZE_OPENFGA, len(parsedTuples))
		batch := parsedTuples[i:end]

		err = j.authSvc.AddRelation(ctx, batch...)
		if err != nil {
			return errors.Codef(errors.CodeOpenFGARequestFailed, "%w", err)
		}
		j.logUserUpdates(ctx, user, batch, true)
	}
	return nil
}

// RemoveRelation checks user permission and removes the given relation tuples.
// JIMM admins can update any relation, while resource administrators can update
// relations on supported resource targets.
func (j *PermissionManager) RemoveRelation(ctx context.Context, user *openfga.User, tuples []apiparams.RelationshipTuple) error {
	parsedTuples, err := j.parseTuples(ctx, tuples)
	if err != nil {
		return err
	}
	for _, tuple := range parsedTuples {
		if err := j.authorizeRelationTargetAdmin(ctx, user, tuple); err != nil {
			return err
		}
	}
	for i := 0; i < len(parsedTuples); i += BATCH_SIZE_OPENFGA {
		end := min(i+BATCH_SIZE_OPENFGA, len(parsedTuples))
		batch := parsedTuples[i:end]

		err = j.authSvc.RemoveRelation(ctx, batch...)
		if err != nil {
			return errors.Codef(errors.CodeOpenFGARequestFailed, "%w", err)
		}
		j.logUserUpdates(ctx, user, batch, false)
	}
	return nil
}

// CheckRelation checks user permission and returns true if the given tuple exists.
// JIMM admins can inspect any relation, while non-admins can inspect their own
// relations or relations on supported resources they administer.
func (j *PermissionManager) CheckRelation(ctx context.Context, user *openfga.User, tuple apiparams.RelationshipTuple, trace bool) (_ bool, err error) {
	parsedTuple, err := j.parseAndAuthorizeRelationCheck(ctx, user, tuple)
	if err != nil {
		return false, err
	}

	contextualTuples, err := user.ContextualTuples()
	if err != nil {
		return false, err
	}
	allowed, err := j.authSvc.CheckRelation(ctx, *parsedTuple, trace, contextualTuples...)
	if err != nil {
		return allowed, errors.Codef(errors.CodeOpenFGARequestFailed, "%w", err)
	}
	return allowed, nil
}

// parseAndAuthorizeRelationCheck parses a relation check and verifies that the
// authenticated user is allowed to inspect it. JIMM administrators can inspect
// any relation; other users can inspect their own relation or a relation on a
// resource they administer.
func (j *PermissionManager) parseAndAuthorizeRelationCheck(ctx context.Context, user *openfga.User, tuple apiparams.RelationshipTuple) (*openfga.Tuple, error) {
	parsedTuple, err := j.parseTuple(ctx, tuple)
	if err != nil {
		return nil, err
	}
	userCheckingSelf := parsedTuple.Object != nil && parsedTuple.Object.Kind == openfga.UserType && parsedTuple.Object.ID == user.Name
	if !userCheckingSelf {
		if err := j.authorizeRelationTargetAdmin(ctx, user, *parsedTuple); err != nil {
			return nil, err
		}
	}
	return parsedTuple, nil
}

// CheckRelations checks user permissions and returns a slice of CheckResult for each tuple.
// Valid and authorized tuples are checked in batches; malformed or unauthorized
// tuples retain an error in their corresponding result.
func (j *PermissionManager) CheckRelations(ctx context.Context, user *openfga.User, tuples []apiparams.RelationshipTuple) ([]openfga.CheckResult, error) {
	results := make([]openfga.CheckResult, len(tuples))
	parsedTuples := make([]openfga.TupleWithCorrelationId, 0, len(tuples))
	resultIndex := make(map[string]int, len(tuples))
	var contextualTuples []openfga.Tuple
	if len(tuples) > 0 {
		var err error
		contextualTuples, err = user.ContextualTuples()
		if err != nil {
			return nil, err
		}
	}
	for i, tuple := range tuples {
		parsedTuple, err := j.parseAndAuthorizeRelationCheck(ctx, user, tuple)
		if err != nil {
			results[i].Error = err
			continue
		}

		correlationID := strconv.Itoa(i)
		resultIndex[correlationID] = i
		check := openfga.TupleWithCorrelationId{
			Tuple:            parsedTuple,
			CorrelationId:    correlationID,
			ContextualTuples: contextualTuples,
		}
		parsedTuples = append(parsedTuples, check)
	}

	if len(parsedTuples) == 0 {
		return results, nil
	}
	allowed, err := j.authSvc.BatchCheckRelations(ctx, parsedTuples)
	if err != nil {
		for _, tuple := range parsedTuples {
			index := resultIndex[tuple.CorrelationId]
			results[index].Error = errors.Codef(errors.CodeOpenFGARequestFailed, "%w", err)
		}
		return results, nil
	}
	for _, tuple := range parsedTuples {
		index := resultIndex[tuple.CorrelationId]
		results[index].Allowed = allowed[tuple.CorrelationId]
	}

	return results, nil
}

// ListRelationshipTuples checks user permission and lists relationship tuples based of tuple struct with pagination.
// Listing filters can be relaxed: optionally exclude tuple.Relation or tuple.Object or specify only tuple.TargetObject.Kind.
func (j *PermissionManager) ListRelationshipTuples(ctx context.Context, user *openfga.User, tuple apiparams.RelationshipTuple, pageSize int32, continuationToken string) ([]openfga.Tuple, string, error) {

	if !user.JimmAdmin {
		return nil, "", errors.Codef(errors.CodeUnauthorized, "unauthorized")
	}
	// if targetObject is not specified returns all tuples.
	parsedTuple := &openfga.Tuple{}
	var err error
	if tuple.TargetObject != "" {
		parsedTuple, err = j.parseTuple(ctx, tuple)
		if err != nil {
			return nil, "", err
		}
	} else if tuple.Object != "" {
		return nil, "", errors.Codef(errors.CodeBadRequest, "it is invalid to pass an object without a target object.")
	}

	responseTuples, ct, err := j.authSvc.ReadRelatedObjects(ctx, *parsedTuple, pageSize, continuationToken)
	if err != nil {
		return nil, "", err
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
		return nil, e, errors.Codef(errors.CodeUnauthorized, "unauthorized")
	}
	responseTuples, nextToken, err := j.getObjectRelationsPage(ctx, object, pageSize, entitlementToken)
	if err != nil {
		return nil, e, err
	}
	// verify next page contains some entries. Otherwise return empty nextToken.
	if len(responseTuples) == int(pageSize) && nextToken.String() != "" {
		responseTuples, _, err := j.getObjectRelationsPage(ctx, object, 1, nextToken)
		if err != nil {
			return nil, e, fmt.Errorf("error getting next page to verify it cointains something: %w", err)
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
		return nil, errors.Codef(errors.CodeUnauthorized, "unauthorized")
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
			return nil, err
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
		return nil, errors.Codef(errors.CodeBadRequest, "%w", err)
	}
	t := openfga.Tuple{
		Relation: relation,
	}

	// Wraps the general error that will be sent for both
	// the object and target object, but changing the message and key
	// to be specific to the erroneous offender.
	parseTagError := func(msg string, key string, err error) error {
		zapctx.Debug(ctx, msg, zap.String("key", key), zap.Error(err))
		return errors.Codef(errors.CodeFailedToParseTupleKey, "%s %s: %s", msg, key, err.Error())
	}

	if tuple.TargetObject == "" {
		return nil, errors.Codef(errors.CodeBadRequest, "target object not specified")
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
