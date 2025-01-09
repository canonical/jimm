// Copyright 2025 Canonical.

package permissions

import (
	"context"
	"fmt"

	"github.com/canonical/ofga"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v5"
	"github.com/juju/zaputil"
	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/openfga"
	ofganames "github.com/canonical/jimm/v3/internal/openfga/names"
	jimmnames "github.com/canonical/jimm/v3/pkg/names"
)

// ToOfferAccessString maps relation to an application offer access string.
func ToOfferAccessString(relation openfga.Relation) string {
	switch relation {
	case ofganames.AdministratorRelation:
		return string(jujuparams.OfferAdminAccess)
	case ofganames.ConsumerRelation:
		return string(jujuparams.OfferConsumeAccess)
	case ofganames.ReaderRelation:
		return string(jujuparams.OfferReadAccess)
	default:
		return ""
	}
}

// ToCloudAccessString maps relation to a cloud access string.
func ToCloudAccessString(relation openfga.Relation) string {
	switch relation {
	case ofganames.AdministratorRelation:
		return "admin"
	case ofganames.CanAddModelRelation:
		return "add-model"
	default:
		return ""
	}
}

// ToModelAccessString maps relation to a model access string.
func ToModelAccessString(relation openfga.Relation) string {
	switch relation {
	case ofganames.AdministratorRelation:
		return "admin"
	case ofganames.WriterRelation:
		return "write"
	case ofganames.ReaderRelation:
		return "read"
	default:
		return ""
	}
}

// ToModelAccessString maps relation to a controller access string.
func ToControllerAccessString(relation openfga.Relation) string {
	switch relation {
	case ofganames.AdministratorRelation:
		return "superuser"
	default:
		return "login"
	}
}

// ToCloudRelation returns a valid relation for the cloud. Access level
// string can be either "admin", in which case the administrator relation
// is returned, or "add-model", in which case the can_addmodel relation is
// returned.
func ToCloudRelation(accessLevel string) (openfga.Relation, error) {
	switch accessLevel {
	case "admin":
		return ofganames.AdministratorRelation, nil
	case "add-model":
		return ofganames.CanAddModelRelation, nil
	default:
		return ofganames.NoRelation, errors.E("unknown cloud access")
	}
}

// ToModelRelation returns a valid relation for the model.
func ToModelRelation(accessLevel string) (openfga.Relation, error) {
	switch accessLevel {
	case "admin":
		return ofganames.AdministratorRelation, nil
	case "write":
		return ofganames.WriterRelation, nil
	case "read":
		return ofganames.ReaderRelation, nil
	default:
		return ofganames.NoRelation, errors.E("unknown model access")
	}
}

// ToOfferRelation returns a valid relation for the application offer.
func ToOfferRelation(accessLevel string) (openfga.Relation, error) {
	switch accessLevel {
	case "":
		return ofganames.NoRelation, nil
	case string(jujuparams.OfferAdminAccess):
		return ofganames.AdministratorRelation, nil
	case string(jujuparams.OfferConsumeAccess):
		return ofganames.ConsumerRelation, nil
	case string(jujuparams.OfferReadAccess):
		return ofganames.ReaderRelation, nil
	default:
		return ofganames.NoRelation, errors.E("unknown application offer access")
	}
}

// GetUserControllerAccess returns the user's level of access to the desired controller.
func (j *permissionManager) GetUserControllerAccess(ctx context.Context, user *openfga.User, controller names.ControllerTag) (string, error) {
	accessLevel := user.GetControllerAccess(ctx, controller)
	return ToControllerAccessString(accessLevel), nil
}

// GetUserCloudAccess returns users access level for the specified cloud.
func (j *permissionManager) GetUserCloudAccess(ctx context.Context, user *openfga.User, cloud names.CloudTag) (string, error) {
	accessLevel := user.GetCloudAccess(ctx, cloud)
	return ToCloudAccessString(accessLevel), nil
}

// GetUserModelAccess returns the access level a user has against a specific model.
func (j *permissionManager) GetUserModelAccess(ctx context.Context, user *openfga.User, model names.ModelTag) (string, error) {
	accessLevel := user.GetModelAccess(ctx, model)
	return ToModelAccessString(accessLevel), nil
}

// GrantAuditLogAccess grants audit log access for the target user.
func (j *permissionManager) GrantAuditLogAccess(ctx context.Context, user *openfga.User, targetUserTag names.UserTag) error {
	const op = errors.Op("jimm.GrantAuditLogAccess")

	access := user.GetControllerAccess(ctx, j.jimmTag)
	if access != ofganames.AdministratorRelation {
		return errors.E(op, errors.CodeUnauthorized, "unauthorized")
	}

	targetUser := &dbmodel.Identity{}
	targetUser.SetTag(targetUserTag)
	err := j.store.GetIdentity(ctx, targetUser)
	if err != nil {
		return errors.E(op, err)
	}

	err = openfga.NewUser(targetUser, j.authSvc).SetControllerAccess(ctx, j.jimmTag, ofganames.AuditLogViewerRelation)
	if err != nil {
		return errors.E(op, err)
	}
	return nil
}

// RevokeAuditLogAccess revokes audit log access for the target user.
func (j *permissionManager) RevokeAuditLogAccess(ctx context.Context, user *openfga.User, targetUserTag names.UserTag) error {
	const op = errors.Op("jimm.RevokeAuditLogAccess")

	access := user.GetControllerAccess(ctx, j.jimmTag)
	if access != ofganames.AdministratorRelation {
		return errors.E(op, errors.CodeUnauthorized, "unauthorized")
	}

	targetUser := &dbmodel.Identity{}
	targetUser.SetTag(targetUserTag)
	err := j.store.GetIdentity(ctx, targetUser)
	if err != nil {
		return errors.E(op, err)
	}

	err = openfga.NewUser(targetUser, j.authSvc).UnsetAuditLogViewerAccess(ctx, j.jimmTag)
	if err != nil {
		return errors.E(op, err)
	}
	return nil
}

// GrantServiceAccountAccess creates an administrator relation between the tags provided
// and the service account. The provided tags must be users or groups (with the member relation)
// otherwise OpenFGA will report an error.
func (j *permissionManager) GrantServiceAccountAccess(ctx context.Context, u *openfga.User, svcAccTag jimmnames.ServiceAccountTag, entities []string) error {
	op := errors.Op("jimm.GrantServiceAccountAccess")
	tags := make([]*ofganames.Tag, 0, len(entities))
	// Validate tags
	for _, val := range entities {
		tag, err := j.parseAndValidateTag(ctx, val)
		if err != nil {
			return errors.E(op, err)
		}
		if tag.Kind != openfga.UserType && tag.Kind != openfga.GroupType {
			return errors.E(op, "invalid entity - not user or group")
		}
		if tag.Kind == openfga.GroupType {
			tag.Relation = ofganames.MemberRelation
		}
		tags = append(tags, tag)
	}
	tuples := make([]openfga.Tuple, 0, len(tags))
	svcAccEntity := ofganames.ConvertTag(svcAccTag)
	for _, tag := range tags {
		tuple := openfga.Tuple{
			Object:   tag,
			Relation: ofganames.AdministratorRelation,
			Target:   svcAccEntity,
		}
		tuples = append(tuples, tuple)
	}
	err := j.authSvc.AddRelation(ctx, tuples...)
	if err != nil {
		zapctx.Error(ctx, "failed to add tuple(s)", zap.NamedError("add-relation-error", err))
		return errors.E(op, errors.CodeOpenFGARequestFailed, err)
	}
	return nil
}

// CheckPermission loops over the desired permissions in desiredPerms and adds these permissions
// to cachedPerms if they exist. If the user does not have any of the desired permissions then an
// error is returned.
// Note that cachedPerms map is modified and returned.
func (j *permissionManager) CheckPermission(ctx context.Context, user *openfga.User, cachedPerms map[string]string, desiredPerms map[string]interface{}) (map[string]string, error) {
	const op = errors.Op("jimm.CheckPermission")
	for key, val := range desiredPerms {
		if _, ok := cachedPerms[key]; !ok {
			stringVal, ok := val.(string)
			if !ok {
				return nil, errors.E(op, fmt.Sprintf("failed to get permission assertion: expected %T, got %T", stringVal, val))
			}
			tag, err := names.ParseTag(key)
			if err != nil {
				return cachedPerms, errors.E(op, fmt.Sprintf("failed to parse tag %s", key))
			}
			relation, err := ofganames.ConvertJujuRelation(stringVal)
			if err != nil {
				return cachedPerms, errors.E(op, fmt.Sprintf("failed to parse relation %s", stringVal), err)
			}
			check, err := openfga.CheckRelation(ctx, user, tag, relation)
			if err != nil {
				return cachedPerms, errors.E(op, err)
			}
			if !check {
				return cachedPerms, errors.E(op, fmt.Sprintf("Missing permission for %s:%s", key, val))
			}
			cachedPerms[key] = stringVal
		}
	}
	return cachedPerms, nil
}

// GetJimmControllerAccess returns the JIMM controller access level for the
// requested user.
func (j *permissionManager) GetJimmControllerAccess(ctx context.Context, user *openfga.User, tag names.UserTag) (string, error) {
	const op = errors.Op("jimm.GetJIMMControllerAccess")

	// If the authenticated user is requesting the access level
	// for him/her-self then we return that - either the user
	// is a JIMM admin (aka "superuser"), or they have a "login"
	// access level.
	if user.Name == tag.Id() {
		if user.JimmAdmin {
			return "superuser", nil
		}
		return "login", nil
	}

	// Only JIMM administrators are allowed to see the access
	// level of somebody else.
	if !user.JimmAdmin {
		return "", errors.E(op, errors.CodeUnauthorized, "unauthorized")
	}

	var targetUser dbmodel.Identity
	targetUser.SetTag(tag)
	targetUserTag := openfga.NewUser(&targetUser, j.authSvc)

	// Check if the user is jimm administrator.
	isAdmin, err := openfga.IsAdministrator(ctx, targetUserTag, j.jimmTag)
	if err != nil {
		zapctx.Error(ctx, "failed to check access rights", zap.Error(err))
		return "", errors.E(op, err)
	}
	if isAdmin {
		return "superuser", nil
	}

	return "login", nil
}

// GrantCloudAccess grants the given access level on the given cloud to the
// given user. If the cloud is not found then an error with the code
// CodeNotFound is returned. If the authenticated user does not have admin
// access to the cloud then an error with the code CodeUnauthorized is
// returned.
func (j *permissionManager) GrantCloudAccess(ctx context.Context, user *openfga.User, ct names.CloudTag, ut names.UserTag, access string) error {
	const op = errors.Op("jimm.GrantCloudAccess")

	targetRelation, err := ToCloudRelation(access)
	if err != nil {
		zapctx.Debug(
			ctx,
			"failed to recognize given access",
			zaputil.Error(err),
			zap.String("access", string(access)),
		)
		return errors.E(op, errors.CodeBadRequest, fmt.Sprintf("failed to recognize given access: %q", access), err)
	}

	isCloudAdministrator, err := openfga.IsAdministrator(ctx, user, ct)
	if err != nil {
		return errors.E(op, err)
	}
	if !isCloudAdministrator {
		// If the user doesn't have admin access on the cloud return
		// an unauthorized error.
		return errors.E(op, errors.CodeUnauthorized, "unauthorized")
	}

	targetUser := &dbmodel.Identity{}
	targetUser.SetTag(ut)
	if err := j.store.GetIdentity(ctx, targetUser); err != nil {
		return err
	}
	targetOfgaUser := openfga.NewUser(targetUser, j.authSvc)

	currentRelation := targetOfgaUser.GetCloudAccess(ctx, ct)
	switch targetRelation {
	case ofganames.CanAddModelRelation:
		switch currentRelation {
		case ofganames.NoRelation:
			break
		default:
			return nil
		}
	case ofganames.AdministratorRelation:
		switch currentRelation {
		case ofganames.NoRelation, ofganames.CanAddModelRelation:
			break
		default:
			return nil
		}
	}

	err = targetOfgaUser.SetCloudAccess(ctx, ct, targetRelation)
	if err != nil {
		zapctx.Error(
			ctx,
			"failed to grant cloud access",
			zaputil.Error(err),
			zap.String("targetUser", string(ut.Id())),
			zap.String("cloud", string(ct.Id())),
			zap.String("access", string(access)),
		)
		return errors.E(op, fmt.Errorf("failed to set cloud access: %w", err))
	}
	return nil
}

// RevokeCloudAccess revokes the given access level on the given cloud from
// the given user. If the cloud is not found then an error with the code
// CodeNotFound is returned. If the authenticated user does not have admin
// access to the cloud then an error with the code CodeUnauthorized is
// returned.
func (j *permissionManager) RevokeCloudAccess(ctx context.Context, user *openfga.User, ct names.CloudTag, ut names.UserTag, access string) error {
	const op = errors.Op("jimm.RevokeCloudAccess")

	targetRelation, err := ToCloudRelation(access)
	if err != nil {
		zapctx.Debug(
			ctx,
			"failed to recognize given access",
			zaputil.Error(err),
			zap.String("access", string(access)),
		)
		return errors.E(op, errors.CodeBadRequest, fmt.Sprintf("failed to recognize given access: %q", access), err)
	}

	isCloudAdministrator, err := openfga.IsAdministrator(ctx, user, ct)
	if err != nil {
		return errors.E(op, err)
	}
	if !isCloudAdministrator {
		// If the user doesn't have admin access on the cloud return
		// an unauthorized error.
		return errors.E(op, errors.CodeUnauthorized, "unauthorized")
	}

	targetUser := &dbmodel.Identity{}
	targetUser.SetTag(ut)
	if err := j.store.GetIdentity(ctx, targetUser); err != nil {
		return err
	}
	targetOfgaUser := openfga.NewUser(targetUser, j.authSvc)

	currentRelation := targetOfgaUser.GetCloudAccess(ctx, ct)

	var relationsToRevoke []openfga.Relation
	switch targetRelation {
	case ofganames.CanAddModelRelation:
		switch currentRelation {
		case ofganames.NoRelation:
			return nil
		default:
			// If we're revoking "add-model" access, in addition to the "add-model" relation, we should also revoke the
			// "admin" relation. That's because having an "admin" relation indirectly grants the "add-model" permission
			// to the user.
			relationsToRevoke = []openfga.Relation{
				ofganames.CanAddModelRelation,
				ofganames.AdministratorRelation,
			}
		}
	case ofganames.AdministratorRelation:
		switch currentRelation {
		case ofganames.NoRelation, ofganames.CanAddModelRelation:
			return nil
		default:
			relationsToRevoke = []openfga.Relation{
				ofganames.AdministratorRelation,
			}
		}
	}

	err = targetOfgaUser.UnsetCloudAccess(ctx, ct, relationsToRevoke...)
	if err != nil {
		zapctx.Error(
			ctx,
			"failed to revoke cloud access",
			zaputil.Error(err),
			zap.String("targetUser", string(ut.Id())),
			zap.String("cloud", string(ct.Id())),
			zap.String("access", string(access)),
		)
		return errors.E(op, fmt.Errorf("failed to unset cloud access: %w", err))
	}

	return nil
}

// GrantModelAccess grants the given access level on the given model to
// the given user. If the model is not found then an error with the code
// CodeNotFound is returned. If the authenticated user does not have
// admin access to the model then an error with the code CodeUnauthorized
// is returned.
func (j *permissionManager) GrantModelAccess(ctx context.Context, user *openfga.User, mt names.ModelTag, ut names.UserTag, access jujuparams.UserAccessPermission) error {
	const op = errors.Op("jimm.GrantModelAccess")
	zapctx.Info(ctx, string(op))

	targetRelation, err := ToModelRelation(string(access))
	if err != nil {
		zapctx.Debug(
			ctx,
			"failed to recognize given access",
			zaputil.Error(err),
			zap.String("access", string(access)),
		)
		return errors.E(op, errors.CodeBadRequest, fmt.Sprintf("failed to recognize given access: %q", access), err)
	}

	modelAdmin, err := user.HasModelRelation(ctx, mt, ofganames.AdministratorRelation)
	if err != nil {
		return errors.E(op, err)
	}
	if !modelAdmin {
		return errors.E(op, errors.CodeUnauthorized, "unauthorized")
	}

	targetUser := &dbmodel.Identity{}
	targetUser.SetTag(ut)
	if err := j.store.GetIdentity(ctx, targetUser); err != nil {
		return err
	}
	targetOfgaUser := openfga.NewUser(targetUser, j.authSvc)

	currentRelation := targetOfgaUser.GetModelAccess(ctx, mt)
	switch targetRelation {
	case ofganames.ReaderRelation:
		switch currentRelation {
		case ofganames.NoRelation:
			break
		default:
			return nil
		}
	case ofganames.WriterRelation:
		switch currentRelation {
		case ofganames.NoRelation, ofganames.ReaderRelation:
			break
		default:
			return nil
		}
	case ofganames.AdministratorRelation:
		switch currentRelation {
		case ofganames.NoRelation, ofganames.ReaderRelation, ofganames.WriterRelation:
			break
		default:
			return nil
		}
	}

	err = targetOfgaUser.SetModelAccess(ctx, mt, targetRelation)
	if err != nil {
		zapctx.Error(
			ctx,
			"failed to grant model access",
			zaputil.Error(err),
			zap.String("targetUser", string(ut.Id())),
			zap.String("model", string(mt.Id())),
			zap.String("access", string(access)),
		)
		return errors.E(op, fmt.Errorf("failed to set model access: %w", err))
	}
	return nil
}

// RevokeModelAccess revokes the given access level on the given model from
// the given user. If the model is not found then an error with the code
// CodeNotFound is returned. If the authenticated user does not have admin
// access to the model, and is not attempting to revoke their own access,
// then an error with the code CodeUnauthorized is returned.
func (j *permissionManager) RevokeModelAccess(ctx context.Context, user *openfga.User, mt names.ModelTag, ut names.UserTag, access jujuparams.UserAccessPermission) error {
	const op = errors.Op("jimm.RevokeModelAccess")
	zapctx.Info(ctx, string(op))

	targetRelation, err := ToModelRelation(string(access))
	if err != nil {
		zapctx.Debug(
			ctx,
			"failed to recognize given access",
			zaputil.Error(err),
			zap.String("access", string(access)),
		)
		return errors.E(op, errors.CodeBadRequest, fmt.Sprintf("failed to recognize given access: %q", access), err)
	}

	requiredAccess := ofganames.AdministratorRelation
	if user.Tag() == ut {
		// If the user is attempting to revoke their own access.
		requiredAccess = ofganames.ReaderRelation
	}

	modelAdmin, err := user.HasModelRelation(ctx, mt, requiredAccess)
	if err != nil {
		return errors.E(op, err)
	}
	if !modelAdmin {
		return errors.E(op, errors.CodeUnauthorized, "unauthorized")
	}

	targetUser := &dbmodel.Identity{}
	targetUser.SetTag(ut)
	if err := j.store.GetIdentity(ctx, targetUser); err != nil {
		return err
	}
	targetOfgaUser := openfga.NewUser(targetUser, j.authSvc)

	currentRelation := targetOfgaUser.GetModelAccess(ctx, mt)

	var relationsToRevoke []openfga.Relation
	switch targetRelation {
	case ofganames.ReaderRelation:
		switch currentRelation {
		case ofganames.NoRelation:
			return nil
		default:
			relationsToRevoke = []openfga.Relation{
				ofganames.ReaderRelation,
				ofganames.WriterRelation,
				ofganames.AdministratorRelation,
			}
		}
	case ofganames.WriterRelation:
		switch currentRelation {
		case ofganames.NoRelation, ofganames.ReaderRelation:
			return nil
		default:
			relationsToRevoke = []openfga.Relation{
				ofganames.WriterRelation,
				ofganames.AdministratorRelation,
			}
		}
	case ofganames.AdministratorRelation:
		switch currentRelation {
		case ofganames.NoRelation, ofganames.ReaderRelation, ofganames.WriterRelation:
			return nil
		default:
			relationsToRevoke = []openfga.Relation{
				ofganames.AdministratorRelation,
			}
		}
	}

	err = targetOfgaUser.UnsetModelAccess(ctx, mt, relationsToRevoke...)
	if err != nil {
		zapctx.Error(
			ctx,
			"failed to revoke model access",
			zaputil.Error(err),
			zap.String("targetUser", string(ut.Id())),
			zap.String("model", string(mt.Id())),
			zap.String("access", string(access)),
		)
		return errors.E(op, fmt.Errorf("failed to unset model access: %w", err))
	}
	return nil
}

// GrantOfferAccess grants rights for an application offer.
func (j *permissionManager) GrantOfferAccess(ctx context.Context, user *openfga.User, offerURL string, ut names.UserTag, access jujuparams.OfferAccessPermission) error {
	const op = errors.Op("jimm.GrantOfferAccess")

	identity, err := dbmodel.NewIdentity(ut.Id())
	if err != nil {
		return errors.E(op, err)
	}

	offer := dbmodel.ApplicationOffer{
		URL: offerURL,
	}
	if err := j.store.GetApplicationOffer(ctx, &offer); err != nil {
		// If the offer is not found, we leak information about the existence of offers that do exist.
		return errors.E(op, err)
	}

	isOfferAdmin, err := openfga.IsAdministrator(ctx, user, offer.ResourceTag())
	if err != nil {
		return errors.E(op, err)
	}
	if !isOfferAdmin {
		return errors.E(op, errors.CodeUnauthorized, "unauthorized")
	}

	targetUser := openfga.NewUser(identity, j.authSvc)
	currentRelation := targetUser.GetApplicationOfferAccess(ctx, offer.ResourceTag())
	currentAccessLevel := ToOfferAccessString(currentRelation)
	targetAccessLevel := determineAccessLevelAfterGrant(currentAccessLevel, string(access))

	// NOTE (alesstimec) not removing the current access level as it might be an
	// indirect relation.
	if targetAccessLevel != currentAccessLevel {
		relation, err := ToOfferRelation(targetAccessLevel)
		if err != nil {
			return errors.E(op, err)
		}
		err = targetUser.SetApplicationOfferAccess(ctx, offer.ResourceTag(), relation)
		if err != nil {
			return errors.E(op, err)
		}
	}

	return nil
}

func determineAccessLevelAfterGrant(currentAccessLevel, grantAccessLevel string) string {
	switch currentAccessLevel {
	case string(jujuparams.OfferAdminAccess):
		return string(jujuparams.OfferAdminAccess)
	case string(jujuparams.OfferConsumeAccess):
		switch grantAccessLevel {
		case string(jujuparams.OfferAdminAccess):
			return string(jujuparams.OfferAdminAccess)
		default:
			return string(jujuparams.OfferConsumeAccess)
		}
	case string(jujuparams.OfferReadAccess):
		switch grantAccessLevel {
		case string(jujuparams.OfferAdminAccess):
			return string(jujuparams.OfferAdminAccess)
		case string(jujuparams.OfferConsumeAccess):
			return string(jujuparams.OfferConsumeAccess)
		default:
			return string(jujuparams.OfferReadAccess)
		}
	default:
		return grantAccessLevel
	}
}

// RevokeOfferAccess revokes rights for an application offer.
func (j *permissionManager) RevokeOfferAccess(ctx context.Context, user *openfga.User, offerURL string, ut names.UserTag, access jujuparams.OfferAccessPermission) (err error) {
	const op = errors.Op("jimm.RevokeOfferAccess")

	identity, err := dbmodel.NewIdentity(ut.Id())
	if err != nil {
		return errors.E(op, err)
	}

	offer := dbmodel.ApplicationOffer{
		URL: offerURL,
	}
	if err := j.store.GetApplicationOffer(ctx, &offer); err != nil {
		// If the offer is not found, we leak information about the existence of offers that do exist.
		return errors.E(op, err)
	}

	isOfferAdmin, err := openfga.IsAdministrator(ctx, user, offer.ResourceTag())
	if err != nil {
		return errors.E(op, err)
	}
	if !isOfferAdmin {
		return errors.E(op, errors.CodeUnauthorized, "unauthorized")
	}

	targetUser := openfga.NewUser(identity, j.authSvc)
	targetRelation, err := ToOfferRelation(string(access))
	if err != nil {
		return errors.E(op, err)
	}
	err = targetUser.UnsetApplicationOfferAccess(ctx, offer.ResourceTag(), targetRelation)
	if err != nil {
		return errors.E(op, err, "failed to unset given access")
	}

	// Checking if the target user still has the given access to the
	// application offer (which is possible because of indirect relations),
	// and if so, returning an informative error.
	currentRelation := targetUser.GetApplicationOfferAccess(ctx, offer.ResourceTag())
	stillHasAccess := false
	switch targetRelation {
	case ofganames.AdministratorRelation:
		if currentRelation == ofganames.AdministratorRelation {
			stillHasAccess = true
		}
	case ofganames.ConsumerRelation:
		switch currentRelation {
		case ofganames.AdministratorRelation, ofganames.ConsumerRelation:
			stillHasAccess = true
		}
	case ofganames.ReaderRelation:
		switch currentRelation {
		case ofganames.AdministratorRelation, ofganames.ConsumerRelation, ofganames.ReaderRelation:
			stillHasAccess = true
		}
	}

	if stillHasAccess {
		return errors.E(op, "unable to completely revoke given access due to other relations; try to remove them as well, or use 'jimmctl' for more control")
	}
	return nil
}

// OpenFGACleanup queries OpenFGA for all existing tuples, tries to resolve each tuple and removes those
// that JIMM cannot resolved - orphaned tuples. JIMM not being able to resolve a tuple means that the
// corresponding entity has been removed from JIMM's database.
//
// This approach to cleaning up tuples is intended to be temporary while we implement
// a better approach to eventual consistency of JIMM's database objects and OpenFGA tuples.
func (j *permissionManager) OpenFGACleanup(ctx context.Context) error {
	var (
		continuationToken string
		err               error
		tuples            []ofga.Tuple
	)
	for {
		tuples, continuationToken, err = j.authSvc.ReadRelatedObjects(ctx, openfga.Tuple{}, 20, continuationToken)
		if err != nil {
			zapctx.Error(ctx, "reading all tuples", zap.Error(err))
			return err
		}

		orphanedTuples := j.orphanedTuples(ctx, tuples...)
		if len(orphanedTuples) > 0 {
			zapctx.Debug(ctx, "removing orphaned tuples", zap.Any("tuples", orphanedTuples))
			err = j.authSvc.RemoveRelation(ctx, orphanedTuples...)
			if err != nil {
				zapctx.Warn(ctx, "failed to clean up orphaned tuples", zap.Error(err))
			}
		}
		if continuationToken == "" {
			return nil
		}
		select {
		case <-ctx.Done():
			return nil
		default:
		}
	}
}

func (j *permissionManager) orphanedTuples(ctx context.Context, tuples ...openfga.Tuple) []openfga.Tuple {
	orphanedTuples := []openfga.Tuple{}
	for _, tuple := range tuples {
		_, err := j.ToJAASTag(ctx, tuple.Object, true)
		if err != nil {
			if errors.ErrorCode(err) == errors.CodeNotFound {
				orphanedTuples = append(orphanedTuples, tuple)
				continue
			}
		}
		_, err = j.ToJAASTag(ctx, tuple.Target, true)
		if err != nil {
			if errors.ErrorCode(err) == errors.CodeNotFound {
				orphanedTuples = append(orphanedTuples, tuple)
				continue
			}
		}
	}
	return orphanedTuples
}
