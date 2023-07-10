// Copyright 2023 CanonicalLtd.

package openfga

import (
	"context"
	"strings"

	"github.com/juju/names/v4"
	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"

	"github.com/CanonicalLtd/jimm/internal/dbmodel"
	"github.com/CanonicalLtd/jimm/internal/errors"
	ofganames "github.com/CanonicalLtd/jimm/internal/openfga/names"
)

// NewUser returns a new user structure that can be used to check
// user's access rights to various resources.
func NewUser(u *dbmodel.User, client *OFGAClient) *User {
	return &User{
		User:   u,
		client: client,
	}
}

// User wraps dbmodel.User and implements methods that enable us
// to check user's access rights to various resources.
type User struct {
	*dbmodel.User
	client *OFGAClient
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
	isAdmin, err := IsAdministrator(ctx, u, resource)
	if err != nil {
		zapctx.Error(ctx, "openfga check failed", zap.Error(err))
		return ofganames.NoRelation
	}
	if isAdmin {
		return ofganames.AdministratorRelation
	}
	return ofganames.NoRelation
}

// GetModelAccess returns the relation the user has with the specified model.
func (u *User) GetModelAccess(ctx context.Context, resource names.ModelTag) Relation {
	isAdmin, err := IsAdministrator(ctx, u, resource)
	if err != nil {
		zapctx.Error(ctx, "openfga check failed", zap.Error(err))
		return ofganames.NoRelation
	}
	if isAdmin {
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
	isAdmin, err := IsAdministrator(ctx, u, resource)
	if err != nil {
		zapctx.Error(ctx, "openfga check failed", zap.Error(err))
		return ofganames.NoRelation
	}
	if isAdmin {
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
func (u *User) SetModelAccess(ctx context.Context, resource names.ModelTag, relation Relation) error {
	return setResourceAccess(ctx, u, resource, relation)
}

// SetControllerAccess adds a direct relation between the user and the controller.
func (u *User) SetControllerAccess(ctx context.Context, resource names.ControllerTag, relation Relation) error {
	return setResourceAccess(ctx, u, resource, relation)
}

// UnsetAuditLogViewerAccess removes a direct audit log viewer relation between the user and a controller.
func (u *User) UnsetAuditLogViewerAccess(ctx context.Context, resource names.ControllerTag) error {
	return unsetResourceAccess(ctx, u, resource, ofganames.AuditLogViewerRelation, true)
}

// SetCloudAccess adds a direct relation between the user and the cloud.
func (u *User) SetCloudAccess(ctx context.Context, resource names.CloudTag, relation Relation) error {
	return setResourceAccess(ctx, u, resource, relation)
}

// SetApplicationOfferAccess adds a direct relation between the user and the application offer.
func (u *User) SetApplicationOfferAccess(ctx context.Context, resource names.ApplicationOfferTag, relation Relation) error {
	return setResourceAccess(ctx, u, resource, relation)
}

// UnsetApplicationOfferAccess removes a direct relation between the user and the application offer.
func (u *User) UnsetApplicationOfferAccess(ctx context.Context, resource names.ApplicationOfferTag, relation Relation, ignoreMissingRelation bool) error {
	return unsetResourceAccess(ctx, u, resource, relation, ignoreMissingRelation)
}

// ListModels returns a slice of model UUIDs this user has at least reader access to.
func (u *User) ListModels(ctx context.Context) ([]string, error) {
	return u.client.ListObjects(ctx, ofganames.ConvertTag(u.ResourceTag()).String(), ofganames.ReaderRelation.String(), "model", nil)
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
			zap.String("user", u.Username),
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

func setResourceAccess[T ofganames.ResourceTagger](ctx context.Context, user *User, resource T, relation Relation) error {
	err := user.client.AddRelations(ctx, Tuple{
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
		users[i] = NewUser(&dbmodel.User{Username: entity.ID}, client)
	}
	return users, nil
}
