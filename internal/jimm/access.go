// Copyright 2024 Canonical.

package jimm

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"strings"

	"github.com/canonical/ofga"
	"github.com/google/uuid"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v5"
	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"

	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/openfga"
	ofganames "github.com/canonical/jimm/v3/internal/openfga/names"
	"github.com/canonical/jimm/v3/internal/servermon"
	jimmnames "github.com/canonical/jimm/v3/pkg/names"
)

const (
	jimmControllerName = "jimm"
)

var (
	// Matches juju uris, jimm user/group tags and UUIDs
	// Performs a single match and breaks the juju URI into 4 groups.
	// The groups are:
	// [0] - Entire match
	// [1] - tag
	// [2] - trailer (i.e. resource identifier)
	// [3] - Relation specifier (i.e., #member)
	// A complete matcher example would look like so with square-brackets denoting groups and paranthsis denoting index:
	// (1)[controller][-](2)[myFavoriteController][#](3)[relation-specifier]"
	// An example without a relation: `user-alice@wonderland`:
	// (1)[user][-](2)[alice@wonderland]
	// An example with a relaton `group-alices-wonderland#member`:
	// (1)[group][-](2)[alices-wonderland][#](3)[member]
	jujuURIMatcher = regexp.MustCompile(`([a-zA-Z0-9]*)(?:-)([^#]+)(?:#([a-zA-Z]+)|\z)`)

	// modelOwnerAndNameMatcher matches a string based on the
	// the expected form <model-owner>/<model-name>
	modelOwnerAndNameMatcher = regexp.MustCompile(`(.+)/(.+)`)
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

// CheckPermission loops over the desired permissions in desiredPerms and adds these permissions
// to cachedPerms if they exist. If the user does not have any of the desired permissions then an
// error is returned.
// Note that cachedPerms map is modified and returned.
func (j *JIMM) CheckPermission(ctx context.Context, user *openfga.User, cachedPerms map[string]string, desiredPerms map[string]interface{}) (map[string]string, error) {
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

// GrantAuditLogAccess grants audit log access for the target user.
func (j *JIMM) GrantAuditLogAccess(ctx context.Context, user *openfga.User, targetUserTag names.UserTag) error {
	const op = errors.Op("jimm.GrantAuditLogAccess")

	access := user.GetControllerAccess(ctx, j.ResourceTag())
	if access != ofganames.AdministratorRelation {
		return errors.E(op, errors.CodeUnauthorized, "unauthorized")
	}

	targetUser := &dbmodel.Identity{}
	targetUser.SetTag(targetUserTag)
	err := j.Database.GetIdentity(ctx, targetUser)
	if err != nil {
		return errors.E(op, err)
	}

	err = openfga.NewUser(targetUser, j.OpenFGAClient).SetControllerAccess(ctx, j.ResourceTag(), ofganames.AuditLogViewerRelation)
	if err != nil {
		return errors.E(op, err)
	}
	return nil
}

// RevokeAuditLogAccess revokes audit log access for the target user.
func (j *JIMM) RevokeAuditLogAccess(ctx context.Context, user *openfga.User, targetUserTag names.UserTag) error {
	const op = errors.Op("jimm.RevokeAuditLogAccess")

	access := user.GetControllerAccess(ctx, j.ResourceTag())
	if access != ofganames.AdministratorRelation {
		return errors.E(op, errors.CodeUnauthorized, "unauthorized")
	}

	targetUser := &dbmodel.Identity{}
	targetUser.SetTag(targetUserTag)
	err := j.Database.GetIdentity(ctx, targetUser)
	if err != nil {
		return errors.E(op, err)
	}

	err = openfga.NewUser(targetUser, j.OpenFGAClient).UnsetAuditLogViewerAccess(ctx, j.ResourceTag())
	if err != nil {
		return errors.E(op, err)
	}
	return nil
}

// ToJAASTag converts a tag used in OpenFGA authorization model to a
// tag used in JAAS.
func (j *JIMM) ToJAASTag(ctx context.Context, tag *ofganames.Tag, resolveUUIDs bool) (string, error) {
	if !resolveUUIDs {
		res := tag.Kind.String() + "-" + tag.ID
		if tag.Relation.String() != "" {
			res = res + "#" + tag.Relation.String()
		}
		return res, nil
	}

	tagToString := func(kind, id string) string {
		res := kind + "-" + id
		if tag.Relation.String() != "" {
			res += "#" + tag.Relation.String()
		}
		return res
	}

	switch tag.Kind {
	case names.UserTagKind:
		return tagToString(names.UserTagKind, tag.ID), nil
	case jimmnames.ServiceAccountTagKind:
		return jimmnames.ServiceAccountTagKind + "-" + tag.ID, nil
	case names.ControllerTagKind:
		if tag.ID == j.ResourceTag().Id() {
			return "controller-jimm", nil
		}
		controller := dbmodel.Controller{
			UUID: tag.ID,
		}
		err := j.Database.GetController(ctx, &controller)
		if err != nil {
			return "", errors.E(err, fmt.Sprintf("failed to fetch controller information: %s", controller.UUID))
		}
		return tagToString(names.ControllerTagKind, controller.Name), nil
	case names.ModelTagKind:
		model := dbmodel.Model{
			UUID: sql.NullString{
				String: tag.ID,
				Valid:  true,
			},
		}
		err := j.Database.GetModel(ctx, &model)
		if err != nil {
			return "", errors.E(err, fmt.Sprintf("failed to fetch model information: %s", model.UUID.String))
		}
		modelUserID := model.OwnerIdentityName + "/" + model.Name
		return tagToString(names.ModelTagKind, modelUserID), nil
	case names.ApplicationOfferTagKind:
		ao := dbmodel.ApplicationOffer{
			UUID: tag.ID,
		}
		err := j.Database.GetApplicationOffer(ctx, &ao)
		if err != nil {
			return "", errors.E(err, fmt.Sprintf("failed to fetch application offer information: %s", ao.UUID))
		}
		return tagToString(names.ApplicationOfferTagKind, ao.URL), nil
	case jimmnames.GroupTagKind:
		group := dbmodel.GroupEntry{
			UUID: tag.ID,
		}
		err := j.Database.GetGroup(ctx, &group)
		if err != nil {
			return "", errors.E(err, fmt.Sprintf("failed to fetch group information: %s", group.UUID))
		}
		return tagToString(jimmnames.GroupTagKind, group.Name), nil
	case jimmnames.RoleTagKind:
		role := dbmodel.RoleEntry{
			UUID: tag.ID,
		}
		err := j.Database.GetRole(ctx, &role)
		if err != nil {
			return "", errors.E(err, fmt.Sprintf("failed to fetch role information: %s", role.UUID))
		}
		return tagToString(jimmnames.RoleTagKind, role.Name), nil
	case names.CloudTagKind:
		cloud := dbmodel.Cloud{
			Name: tag.ID,
		}
		err := j.Database.GetCloud(ctx, &cloud)
		if err != nil {
			return "", errors.E(err, fmt.Sprintf("failed to fetch cloud information: %s", cloud.Name))
		}
		return tagToString(names.CloudTagKind, cloud.Name), nil
	default:
		return "", errors.E(fmt.Sprintf("unexpected tag kind: %v", tag.Kind))
	}
}

type tagResolver struct {
	resourceUUID string
	trailer      string
	relation     ofga.Relation
}

func newTagResolver(tag string) (*tagResolver, string, error) {
	matches := jujuURIMatcher.FindStringSubmatch(tag)
	if len(matches) != 4 {
		return nil, "", errors.E("tag is not properly formatted", errors.CodeBadRequest)
	}
	tagKind := matches[1]
	resourceUUID := ""
	trailer := ""
	// We first attempt to see if group2 is a uuid
	if _, err := uuid.Parse(matches[2]); err == nil {
		// We know it's a UUID
		resourceUUID = matches[2]
	} else {
		// We presume the information the matcher needs is in the trailer
		trailer = matches[2]
	}

	relation, err := ofganames.ParseRelation(matches[3])
	if err != nil {
		return nil, "", errors.E("failed to parse relation", errors.CodeBadRequest)
	}
	return &tagResolver{
		resourceUUID: resourceUUID,
		trailer:      trailer,
		relation:     relation,
	}, tagKind, nil
}

func (t *tagResolver) userTag(ctx context.Context) (*ofga.Entity, error) {
	zapctx.Debug(
		ctx,
		"Resolving JIMM tags to Juju tags for tag kind: user",
		zap.String("user-name", t.trailer),
	)

	valid := names.IsValidUser(t.trailer)
	if !valid {
		// TODO(ale8k): Return custom error for validation check at JujuAPI
		return nil, errors.E("invalid user")
	}
	return ofganames.ConvertTagWithRelation(names.NewUserTag(t.trailer), t.relation), nil
}

func (t *tagResolver) groupTag(ctx context.Context, db *db.Database) (*ofga.Entity, error) {
	zapctx.Debug(
		ctx,
		"Resolving JIMM tags to Juju tags for tag kind: group",
		zap.String("group-name", t.trailer),
	)
	if t.resourceUUID != "" {
		return ofganames.ConvertTagWithRelation(jimmnames.NewGroupTag(t.resourceUUID), t.relation), nil
	}
	entry := dbmodel.GroupEntry{Name: t.trailer}

	err := db.GetGroup(ctx, &entry)
	if err != nil {
		return nil, errors.E(fmt.Sprintf("group %s not found", t.trailer))
	}

	return ofganames.ConvertTagWithRelation(entry.ResourceTag(), t.relation), nil
}

func (t *tagResolver) roleTag(ctx context.Context, db *db.Database) (*ofga.Entity, error) {
	zapctx.Debug(
		ctx,
		"Resolving JIMM tags to Juju tags for tag kind: role",
		zap.String("role-name", t.trailer),
	)
	if t.resourceUUID != "" {
		return ofganames.ConvertTagWithRelation(jimmnames.NewRoleTag(t.resourceUUID), t.relation), nil
	}
	entry := dbmodel.RoleEntry{Name: t.trailer}

	err := db.GetRole(ctx, &entry)
	if err != nil {
		return nil, errors.E(fmt.Sprintf("role %s not found", t.trailer))
	}

	return ofganames.ConvertTagWithRelation(entry.ResourceTag(), t.relation), nil
}

func (t *tagResolver) controllerTag(ctx context.Context, jimmUUID string, db *db.Database) (*ofga.Entity, error) {
	zapctx.Debug(
		ctx,
		"Resolving JIMM tags to Juju tags for tag kind: controller",
	)

	if t.resourceUUID != "" {
		return ofganames.ConvertTagWithRelation(names.NewControllerTag(t.resourceUUID), t.relation), nil
	}
	if t.trailer == jimmControllerName {
		return ofganames.ConvertTagWithRelation(names.NewControllerTag(jimmUUID), t.relation), nil
	}
	controller := dbmodel.Controller{Name: t.trailer}

	err := db.GetController(ctx, &controller)
	if err != nil {
		return nil, errors.E("controller not found")
	}
	return ofganames.ConvertTagWithRelation(controller.ResourceTag(), t.relation), nil
}

func (t *tagResolver) modelTag(ctx context.Context, db *db.Database) (*ofga.Entity, error) {
	zapctx.Debug(
		ctx,
		"Resolving JIMM tags to Juju tags for tag kind: model",
	)

	if t.resourceUUID != "" {
		return ofganames.ConvertTagWithRelation(names.NewModelTag(t.resourceUUID), t.relation), nil
	}

	model := dbmodel.Model{}
	matches := modelOwnerAndNameMatcher.FindStringSubmatch(t.trailer)
	if len(matches) != 3 {
		return nil, errors.E("model name format incorrect, expected <model-owner>/<model-name>")
	}
	model.OwnerIdentityName = matches[1]
	model.Name = matches[2]

	err := db.GetModel(ctx, &model)
	if err != nil {
		return nil, errors.E("model not found")
	}

	return ofganames.ConvertTagWithRelation(model.ResourceTag(), t.relation), nil
}

func (t *tagResolver) applicationOfferTag(ctx context.Context, db *db.Database) (*ofga.Entity, error) {
	zapctx.Debug(
		ctx,
		"Resolving JIMM tags to Juju tags for tag kind: applicationoffer",
	)

	if t.resourceUUID != "" {
		return ofganames.ConvertTagWithRelation(names.NewApplicationOfferTag(t.resourceUUID), t.relation), nil
	}
	offer := dbmodel.ApplicationOffer{URL: t.trailer}

	err := db.GetApplicationOffer(ctx, &offer)
	if err != nil {
		return nil, errors.E("application offer not found")
	}

	return ofganames.ConvertTagWithRelation(offer.ResourceTag(), t.relation), nil
}

func (t *tagResolver) cloudTag(ctx context.Context, db *db.Database) (*ofga.Entity, error) {
	zapctx.Debug(
		ctx,
		"Resolving JIMM tags to Juju tags for tag kind: cloud",
	)

	if t.resourceUUID != "" {
		return ofganames.ConvertTagWithRelation(names.NewCloudTag(t.resourceUUID), t.relation), nil
	}
	cloud := dbmodel.Cloud{Name: t.trailer}

	err := db.GetCloud(ctx, &cloud)
	if err != nil {
		return nil, errors.E("cloud not found")
	}

	return ofganames.ConvertTagWithRelation(cloud.ResourceTag(), t.relation), nil
}

func (t *tagResolver) serviceAccountTag(ctx context.Context) (*ofga.Entity, error) {
	zapctx.Debug(
		ctx,
		"Resolving JIMM tags to Juju tags for tag kind: serviceaccount",
		zap.String("serviceaccount-name", t.trailer),
	)
	if !jimmnames.IsValidServiceAccountId(t.trailer) {
		// TODO(ale8k): Return custom error for validation check at JujuAPI
		return nil, errors.E("invalid service account id")
	}

	return ofganames.ConvertTagWithRelation(jimmnames.NewServiceAccountTag(t.trailer), t.relation), nil
}

// resolveTag resolves JIMM tag [of any kind available] (i.e., controller-mycontroller:alex@canonical.com/mymodel.myoffer)
// into a juju string tag (i.e., controller-<controller uuid>).
//
// If the JIMM tag is aleady of juju string tag form, the transformation is left alone.
//
// In both cases though, the resource the tag pertains to is validated to exist within the database.
func resolveTag(jimmUUID string, db *db.Database, tag string) (*ofganames.Tag, error) {
	ctx := context.Background()
	resolver, tagKind, err := newTagResolver(tag)
	if err != nil {
		return nil, errors.E(fmt.Errorf("failed to setup tag resolver: %w", err))
	}

	switch tagKind {
	case names.UserTagKind:
		return resolver.userTag(ctx)
	case jimmnames.GroupTagKind:
		return resolver.groupTag(ctx, db)
	case jimmnames.RoleTagKind:
		return resolver.roleTag(ctx, db)
	case names.ControllerTagKind:
		return resolver.controllerTag(ctx, jimmUUID, db)
	case names.ModelTagKind:
		return resolver.modelTag(ctx, db)
	case names.ApplicationOfferTagKind:
		return resolver.applicationOfferTag(ctx, db)
	case names.CloudTagKind:
		return resolver.cloudTag(ctx, db)
	case jimmnames.ServiceAccountTagKind:
		return resolver.serviceAccountTag(ctx)
	}
	return nil, errors.E(errors.CodeBadRequest, fmt.Sprintf("failed to map tag, unknown kind: %s", tagKind))
}

// parseAndValidateTag attempts to parse the provided key into a tag whilst additionally
// ensuring the resource exists for said tag.
//
// This key may be in the form of either a JIMM tag string or Juju tag string.
func (j *JIMM) parseAndValidateTag(ctx context.Context, key string) (*ofganames.Tag, error) {
	op := errors.Op("jimm.parseAndValidateTag")
	tupleKeySplit := strings.SplitN(key, "-", 2)
	if len(tupleKeySplit) == 1 {
		tag, err := ofganames.BlankKindTag(tupleKeySplit[0])
		if err != nil {
			return nil, errors.E(op, errors.CodeFailedToParseTupleKey, err)
		}
		return tag, nil
	}
	tagString := key
	tag, err := resolveTag(j.UUID, j.Database, tagString)
	if err != nil {
		zapctx.Debug(ctx, "failed to resolve tuple object", zap.Error(err))
		return nil, errors.E(op, errors.CodeFailedToResolveTupleResource, err)
	}
	zapctx.Debug(ctx, "resolved JIMM tag", zap.String("tag", tag.String()))

	return tag, nil
}

// OpenFGACleanup queries OpenFGA for all existing tuples, tries to resolve each tuple and removes those
// that JIMM cannot resolved - orphaned tuples. JIMM not being able to resolve a tuple means that the
// corresponding entity has been removed from JIMM's database.
//
// This approach to cleaning up tuples is intended to be temporary while we implement
// a better approach to eventual consistency of JIMM's database objects and OpenFGA tuples.
func (j *JIMM) OpenFGACleanup(ctx context.Context) (err error) {
	const op = errors.Op("jimm.CleanupDyingModels")
	zapctx.Info(ctx, string(op))
	durationObserver := servermon.DurationObserver(servermon.JimmMethodsDurationHistogram, string(op))
	defer durationObserver()
	var (
		continuationToken string
		tuples            []ofga.Tuple
	)
	for {
		tuples, continuationToken, err = j.OpenFGAClient.ReadRelatedObjects(ctx, openfga.Tuple{}, 20, continuationToken)
		if err != nil {
			zapctx.Error(ctx, "reading all tuples", zap.Error(err))
			return err
		}

		orphanedTuples := j.orphanedTuples(ctx, tuples...)
		if len(orphanedTuples) > 0 {
			zapctx.Debug(ctx, "removing orphaned tuples", zap.Any("tuples", orphanedTuples))
			err = j.OpenFGAClient.RemoveRelation(ctx, orphanedTuples...)
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

func (j *JIMM) orphanedTuples(ctx context.Context, tuples ...openfga.Tuple) []openfga.Tuple {
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
