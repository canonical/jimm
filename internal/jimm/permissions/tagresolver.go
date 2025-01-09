// Copyright 2025 Canonical.

package permissions

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"strings"

	"github.com/canonical/ofga"
	"github.com/google/uuid"
	"github.com/juju/names/v5"
	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"

	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	ofganames "github.com/canonical/jimm/v3/internal/openfga/names"
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

// ToJAASTag converts a tag used in OpenFGA authorization model to a
// tag used in JAAS.
func (j *permissionManager) ToJAASTag(ctx context.Context, tag *ofganames.Tag, resolveUUIDs bool) (string, error) {
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
		if tag.ID == j.jimmTag.Id() {
			return "controller-jimm", nil
		}
		controller := dbmodel.Controller{
			UUID: tag.ID,
		}
		err := j.store.GetController(ctx, &controller)
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
		err := j.store.GetModel(ctx, &model)
		if err != nil {
			return "", errors.E(err, fmt.Sprintf("failed to fetch model information: %s", model.UUID.String))
		}
		modelUserID := model.OwnerIdentityName + "/" + model.Name
		return tagToString(names.ModelTagKind, modelUserID), nil
	case names.ApplicationOfferTagKind:
		ao := dbmodel.ApplicationOffer{
			UUID: tag.ID,
		}
		err := j.store.GetApplicationOffer(ctx, &ao)
		if err != nil {
			return "", errors.E(err, fmt.Sprintf("failed to fetch application offer information: %s", ao.UUID))
		}
		return tagToString(names.ApplicationOfferTagKind, ao.URL), nil
	case jimmnames.GroupTagKind:
		group := dbmodel.GroupEntry{
			UUID: tag.ID,
		}
		err := j.store.GetGroup(ctx, &group)
		if err != nil {
			return "", errors.E(err, fmt.Sprintf("failed to fetch group information: %s", group.UUID))
		}
		return tagToString(jimmnames.GroupTagKind, group.Name), nil
	case jimmnames.RoleTagKind:
		role := dbmodel.RoleEntry{
			UUID: tag.ID,
		}
		err := j.store.GetRole(ctx, &role)
		if err != nil {
			return "", errors.E(err, fmt.Sprintf("failed to fetch role information: %s", role.UUID))
		}
		return tagToString(jimmnames.RoleTagKind, role.Name), nil
	case names.CloudTagKind:
		cloud := dbmodel.Cloud{
			Name: tag.ID,
		}
		err := j.store.GetCloud(ctx, &cloud)
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
func (j *permissionManager) parseAndValidateTag(ctx context.Context, key string) (*ofganames.Tag, error) {
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
	tag, err := resolveTag(j.jimmUUID, j.store, tagString)
	if err != nil {
		zapctx.Debug(ctx, "failed to resolve tuple object", zap.Error(err))
		return nil, errors.E(op, errors.CodeFailedToResolveTupleResource, err)
	}
	zapctx.Debug(ctx, "resolved JIMM tag", zap.String("tag", tag.String()))

	return tag, nil
}
