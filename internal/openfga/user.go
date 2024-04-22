// Copyright 2023 canonical.

package openfga

import (
	"context"
	"strings"

	"github.com/juju/names/v5"
	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"

	"github.com/canonical/jimm/internal/dbmodel"
	"github.com/canonical/jimm/internal/errors"
	ofganames "github.com/canonical/jimm/internal/openfga/names"
	jimmnames "github.com/canonical/jimm/pkg/names"
	"github.com/canonical/ofga"
)

// NewUser returns a new user structure that can be used to check
// user's access rights to various resources.
func NewUser(u *dbmodel.Identity, client *OFGAClient) *User {
	return &User{
		Identity: u,
		client:   client,
	}
}

// User wraps dbmodel.User and implements methods that enable us
// to check user's access rights to various resources.
type User struct {
	*dbmodel.Identity
	client    *OFGAClient
	JimmAdmin bool
}

// IsAllowedAddModed returns true if the user is allowed to add a model on the
// specified cloud.
func (u *User) IsAllowedAddModel(ctx context.Context, resource names.CloudTag) (bool, error) {
	allowed, err := checkRelation(ctx, u, resource, ofganames.CanAddModelRelation)
	if err != nil {
		return false, errors.E(err)
	}
	return allowed, nil
}

// IsApplicationOfferConsumer returns true if user has consumer relation to the application offer.
func (u *User) IsApplicationOfferConsumer(ctx context.Context, resource names.ApplicationOfferTag) (bool, error) {
	isConsumer, err := checkRelation(ctx, u, resource, ofganames.ConsumerRelation)
	if err != nil {
		return false, errors.E(err)
	}
	return isConsumer, nil
}

// IsApplicationOfferReader returns true if user has reader relation to the application offer.
func (u *User) IsApplicationOfferReader(ctx context.Context, resource names.ApplicationOfferTag) (bool, error) {
	isReader, err := checkRelation(ctx, u, resource, ofganames.ReaderRelation)
	if err != nil {
		return false, errors.E(err)
	}
	return isReader, nil
}

// IsModelReader returns true if user has reader relation to the model.
func (u *User) IsModelReader(ctx context.Context, resource names.ModelTag) (bool, error) {
	isReader, err := checkRelation(ctx, u, resource, ofganames.ReaderRelation)
	if err != nil {
		return false, errors.E(err)
	}
	return isReader, nil
}

// IsModelWriter returns true if user has writer relation to the model.
func (u *User) IsModelWriter(ctx context.Context, resource names.ModelTag) (bool, error) {
	isWriter, err := checkRelation(ctx, u, resource, ofganames.WriterRelation)
	if err != nil {
		return false, errors.E(err)
	}
	return isWriter, nil
}

// IsServiceAccountAdmin returns true if the user has administrator relation to the service account.
func (u *User) IsServiceAccountAdmin(ctx context.Context, clientID jimmnames.ServiceAccountTag) (bool, error) {
	isAdmin, err := checkRelation(ctx, u, clientID, ofganames.AdministratorRelation)
	if err != nil {
		return false, errors.E(err)
	}
	return isAdmin, nil
}

// GetCloudAccess returns the relation the user has to the specified cloud.
func (u *User) GetCloudAccess(ctx context.Context, resource names.CloudTag) Relation {
	isCloudAdmin, err := IsAdministrator(ctx, u, resource)
	if err != nil {
		zapctx.Error(ctx, "openfga check failed", zap.Error(err))
		return ofganames.NoRelation
	}
	if isCloudAdmin {
		return ofganames.AdministratorRelation
	}
	userAccess, err := u.IsAllowedAddModel(ctx, resource)
	if err != nil {
		zapctx.Error(ctx, "openfga check failed", zap.Error(err))
		return ofganames.NoRelation
	}
	if userAccess {
		return ofganames.CanAddModelRelation
	}

	return ofganames.NoRelation
}

// GetAuditLogViewerAccess returns if the user has audit log viewer relation with the given controller.
func (u *User) GetAuditLogViewerAccess(ctx context.Context, resource names.ControllerTag) Relation {
	hasAccess, err := checkRelation(ctx, u, resource, ofganames.AuditLogViewerRelation)
	if err != nil {
		zapctx.Error(ctx, "openfga check failed", zap.Error(err))
		return ofganames.NoRelation
	}
	if hasAccess {
		return ofganames.AuditLogViewerRelation
	}
	return ofganames.NoRelation
}

// GetControllerAccess returns the relation the user has with the specified controller.
func (u *User) GetControllerAccess(ctx context.Context, resource names.ControllerTag) Relation {
	isControllerAdmin, err := IsAdministrator(ctx, u, resource)
	if err != nil {
		zapctx.Error(ctx, "openfga check failed", zap.Error(err))
		return ofganames.NoRelation
	}
	if isControllerAdmin {
		return ofganames.AdministratorRelation
	}
	return ofganames.NoRelation
}

// GetModelAccess returns the relation the user has with the specified model.
func (u *User) GetModelAccess(ctx context.Context, resource names.ModelTag) Relation {
	isModelAdmin, err := IsAdministrator(ctx, u, resource)
	if err != nil {
		zapctx.Error(ctx, "openfga check failed", zap.Error(err))
		return ofganames.NoRelation
	}
	if isModelAdmin {
		return ofganames.AdministratorRelation
	}
	isModelWriter, err := u.IsModelWriter(ctx, resource)
	if err != nil {
		zapctx.Error(ctx, "openfga check failed", zap.Error(err))
		return ofganames.NoRelation
	}
	if isModelWriter {
		return ofganames.WriterRelation
	}
	isModelReader, err := u.IsModelReader(ctx, resource)
	if err != nil {
		zapctx.Error(ctx, "openfga check failed", zap.Error(err))
		return ofganames.NoRelation
	}
	if isModelReader {
		return ofganames.ReaderRelation
	}

	return ofganames.NoRelation
}

// GetApplicationOfferAccess returns the relation the user has with the specified application offer.
func (u *User) GetApplicationOfferAccess(ctx context.Context, resource names.ApplicationOfferTag) Relation {
	isOfferAdmin, err := IsAdministrator(ctx, u, resource)
	if err != nil {
		zapctx.Error(ctx, "openfga check failed", zap.Error(err))
		return ofganames.NoRelation
	}
	if isOfferAdmin {
		return ofganames.AdministratorRelation
	}
	isConsumer, err := u.IsApplicationOfferConsumer(ctx, resource)
	if err != nil {
		zapctx.Error(ctx, "openfga check failed", zap.Error(err))
		return ofganames.NoRelation
	}
	if isConsumer {
		return ofganames.ConsumerRelation
	}
	isReader, err := u.IsApplicationOfferReader(ctx, resource)
	if err != nil {
		zapctx.Error(ctx, "openfga check failed", zap.Error(err))
		return ofganames.NoRelation
	}
	if isReader {
		return ofganames.ReaderRelation
	}

	return ofganames.NoRelation
}

// SetModelAccess adds a direct relation between the user and the model.
// Note that the action is idempotent (does not return error if the relation already exists).
func (u *User) SetModelAccess(ctx context.Context, resource names.ModelTag, relation Relation) error {
	return setResourceAccess(ctx, u, resource, relation)
}

// UnsetModelAccess removes direct relations between the user and the model.
// Note that the action is idempotent (i.e., does not return error if the relation does not exist).
func (u *User) UnsetModelAccess(ctx context.Context, resource names.ModelTag, relations ...Relation) error {
	return unsetMultipleResourceAccesses(ctx, u, resource, relations)
}

// SetControllerAccess adds a direct relation between the user and the controller.
// Note that the action is idempotent (does not return error if the relation already exists).
func (u *User) SetControllerAccess(ctx context.Context, resource names.ControllerTag, relation Relation) error {
	return setResourceAccess(ctx, u, resource, relation)
}

// UnsetAuditLogViewerAccess removes a direct audit log viewer relation between the user and a controller.
// Note that the action is idempotent (i.e., does not return error if the relation does not exist).
func (u *User) UnsetAuditLogViewerAccess(ctx context.Context, resource names.ControllerTag) error {
	return unsetResourceAccess(ctx, u, resource, ofganames.AuditLogViewerRelation, true)
}

// SetCloudAccess adds a direct relation between the user and the cloud.
// Note that the action is idempotent (does not return error if the relation already exists).
func (u *User) SetCloudAccess(ctx context.Context, resource names.CloudTag, relation Relation) error {
	return setResourceAccess(ctx, u, resource, relation)
}

// UnsetCloudAccess removes direct relations between the user and the cloud.
// Note that the action is idempotent (i.e., does not return error if the relation does not exist).
func (u *User) UnsetCloudAccess(ctx context.Context, resource names.CloudTag, relations ...Relation) error {
	return unsetMultipleResourceAccesses(ctx, u, resource, relations)
}

// SetApplicationOfferAccess adds a direct relation between the user and the application offer.
// Note that the action is idempotent (does not return error if the relation already exists).
func (u *User) SetApplicationOfferAccess(ctx context.Context, resource names.ApplicationOfferTag, relation Relation) error {
	return setResourceAccess(ctx, u, resource, relation)
}

// UnsetApplicationOfferAccess removes a direct relation between the user and the application offer.
// Note that if the `ignoreMissingRelation` is set to `true`, then the action will be idempotent (i.e., does not return
// error if the relation does not exist).
func (u *User) UnsetApplicationOfferAccess(ctx context.Context, resource names.ApplicationOfferTag, relation Relation, ignoreMissingRelation bool) error {
	return unsetResourceAccess(ctx, u, resource, relation, ignoreMissingRelation)
}

// ListModels returns a slice of model UUIDs that this user has the relation <relation> to.
func (u *User) ListModels(ctx context.Context, relation ofga.Relation) ([]string, error) {
	entities, err := u.client.ListObjects(ctx, ofganames.ConvertTag(u.ResourceTag()), relation, ModelType, nil)
	if err != nil {
		return nil, err
	}
	modelUUIDs := make([]string, len(entities))
	for i, model := range entities {
		modelUUIDs[i] = model.ID
	}
	return modelUUIDs, err
}

// ListApplicationOffers returns a slice of application offer UUIDs that a user has the relation <relation> to.
func (u *User) ListApplicationOffers(ctx context.Context, relation ofga.Relation) ([]string, error) {
	entities, err := u.client.ListObjects(ctx, ofganames.ConvertTag(u.ResourceTag()), relation, ApplicationOfferType, nil)
	if err != nil {
		return nil, err
	}
	appOfferUUIDs := make([]string, len(entities))
	for i, offer := range entities {
		appOfferUUIDs[i] = offer.ID
	}
	return appOfferUUIDs, err
}

type administratorT interface {
	names.ControllerTag | names.ModelTag | names.ApplicationOfferTag | names.CloudTag

	Id() string
	Kind() string
	String() string
}

func checkRelation[T ofganames.ResourceTagger](ctx context.Context, u *User, resource T, relation Relation) (bool, error) {
	isAllowed, err := u.client.CheckRelation(
		ctx,
		Tuple{
			Object:   ofganames.ConvertTag(u.ResourceTag()),
			Relation: relation,
			Target:   ofganames.ConvertTag(resource),
		},
		true,
	)
	if err != nil {
		return false, errors.E(err)
	}

	return isAllowed, nil
}

// CheckRelation accepts a resource as a tag and checks if the user has the specified relation to the resource.
// The resource string will be converted to a tag. In cases where one already has a resource tag, consider using
// the convenience functions like `IsModelWriter` or `IsApplicationOfferConsumer`.
func CheckRelation(ctx context.Context, u *User, resource names.Tag, relation Relation) (bool, error) {
	var tag *ofganames.Tag
	var err error
	tag = ofganames.ConvertGenericTag(resource)
	isAllowed, err := u.client.CheckRelation(
		ctx,
		Tuple{
			Object:   ofganames.ConvertTag(u.ResourceTag()),
			Relation: relation,
			Target:   tag,
		},
		true,
	)
	if err != nil {
		return false, errors.E(err)
	}

	return isAllowed, nil
}

// IsAdministrator returns true if user has administrator access to the resource.
func IsAdministrator[T administratorT](ctx context.Context, u *User, resource T) (bool, error) {
	isAdmin, err := checkRelation(ctx, u, resource, ofganames.AdministratorRelation)
	if err != nil {
		zapctx.Error(
			ctx,
			"openfga administrator check failed",
			zap.Error(err),
			zap.String("user", u.Name),
			zap.String("resource", resource.String()),
		)
		return false, errors.E(err)
	}
	if isAdmin {
		zapctx.Info(
			ctx,
			"user is resource administrator",
			zap.String("user", u.Tag().String()),
			zap.String("resource", resource.String()),
		)
	}
	return isAdmin, nil
}

// setResourceAccess creates a relation to model the requested resource access.
// Note that the action is idempotent (does not return error if the relation already exists).
func setResourceAccess[T ofganames.ResourceTagger](ctx context.Context, user *User, resource T, relation Relation) error {
	err := user.client.AddRelation(ctx, Tuple{
		Object:   ofganames.ConvertTag(user.ResourceTag()),
		Relation: relation,
		Target:   ofganames.ConvertTag(resource),
	})
	if err != nil {
		// if the tuple already exist we don't return an error.
		// TODO we should opt to check against specific errors via checking their code/metadata.
		if strings.Contains(err.Error(), "cannot write a tuple which already exists") {
			return nil
		}
		return errors.E(err)
	}

	return nil
}

// unsetMultipleResourceAccesses deletes relations that correspond to the requested resource access, atomically.
// Note that the action is idempotent (i.e., does not return error if any of the relations does not exist).
func unsetMultipleResourceAccesses[T ofganames.ResourceTagger](ctx context.Context, user *User, resource T, relations []Relation) error {
	tupleObject := ofganames.ConvertTag(user.ResourceTag())
	tupleTarget := ofganames.ConvertTag(resource)

	lastContinuationToken := ""
	existingRelations := map[Relation]interface{}{}
	for {
		timestampedTuples, continuationToken, err := user.client.cofgaClient.FindMatchingTuples(ctx, Tuple{
			Object: tupleObject,
			Target: tupleTarget,
		}, 0, lastContinuationToken)

		if err != nil {
			return errors.E(err, "failed to retrieve existing relations")
		}

		for _, timestampedTuple := range timestampedTuples {
			existingRelations[timestampedTuple.Tuple.Relation] = nil
		}

		if continuationToken == lastContinuationToken {
			break
		}
		lastContinuationToken = continuationToken
	}

	tuplesToRemove := make([]Tuple, 0, len(relations))
	for _, relation := range relations {
		if _, ok := existingRelations[relation]; !ok {
			continue
		}
		tuplesToRemove = append(tuplesToRemove, Tuple{
			Object:   tupleObject,
			Relation: relation,
			Target:   tupleTarget,
		})
	}

	err := user.client.RemoveRelation(ctx, tuplesToRemove...)
	if err != nil {
		return errors.E(err, "failed to remove relations")
	}
	return nil
}

// unsetResourceAccess deletes a relation that corresponds to the requested resource access.
// Note that if the `ignoreMissingRelation` argument is set to `true`, then the action will be idempotent (i.e., does
// not return error if the relation does not exist).
func unsetResourceAccess[T ofganames.ResourceTagger](ctx context.Context, user *User, resource T, relation Relation, ignoreMissingRelation bool) error {
	err := user.client.RemoveRelation(ctx, Tuple{
		Object:   ofganames.ConvertTag(user.ResourceTag()),
		Relation: relation,
		Target:   ofganames.ConvertTag(resource),
	})
	if err != nil {
		if ignoreMissingRelation {
			// if the tuple does not exist we don't return an error.
			// TODO we should opt to check against specific errors via checking their code/metadata.
			if strings.Contains(err.Error(), "cannot delete a tuple which does not exist") {
				return nil
			}
		}
		return errors.E(err)
	}

	return nil
}

// ListUsersWithAccess lists all users that have the specified relation to the resource.
func ListUsersWithAccess[T ofganames.ResourceTagger](ctx context.Context, client *OFGAClient, resource T, relation Relation) ([]*User, error) {
	entities, err := client.cofgaClient.FindUsersByRelation(ctx, Tuple{
		Relation: relation,
		Target:   ofganames.ConvertTag(resource),
	}, 999)

	if err != nil {
		return nil, err
	}

	users := make([]*User, len(entities))
	for i, entity := range entities {
		if entity.ID == "*" {
			entity.ID = ofganames.EveryoneUser
		}
		identity, err := dbmodel.NewIdentity(entity.ID)
		if err != nil {
			zapctx.Error(ctx, "failed to return user with access", zap.Error(err), zap.String("id", entity.ID))
		}
		users[i] = NewUser(identity, client)
	}
	return users, nil
}
