// Copyright 2024 Canonical.

package jimm

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"strings"
	"sync"

	"github.com/canonical/ofga"
	"github.com/google/uuid"
	"github.com/juju/juju/core/crossmodel"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v5"
	"github.com/juju/zaputil"
	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"

	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/jimmjwx"
	"github.com/canonical/jimm/v3/internal/openfga"
	ofganames "github.com/canonical/jimm/v3/internal/openfga/names"
	jimmnames "github.com/canonical/jimm/v3/pkg/names"
)

const (
	jimmControllerName = "jimm"
)

var (
	// Matches juju uris, jimm user/group tags and UUIDs
	// Performs a single match and breaks the juju URI into 10 groups, each successive group is XORD to ensure we can run
	// this just once.
	// The groups are as so:
	// [0] - Entire match
	// [1] - tag
	// [2] - A single "-", ignored
	// [3] - Controller name OR user name OR group name
	// [4] - A single ":", ignored
	// [5] - Controller user / model owner
	// [6] - A single "/", ignored
	// [7] - Model name
	// [8] - A single ".", ignored
	// [9] - Application offer name
	// [10] - Relation specifier (i.e., #member)
	// A complete matcher example would look like so with square-brackets denoting groups and paranthsis denoting index:
	// (1)[controller](2)[-](3)[controller-1](4)[:](5)[alice@canonical.com-place](6)[/](7)[model-1](8)[.](9)[offer-1](10)[#relation-specifier]"
	// In the case of something like: user-alice@wonderland or group-alices-wonderland#member, it would look like so:
	// (1)[user](2)[-](3)[alices@wonderland]
	// (1)[group](2)[-](3)[alices-wonderland](10)[#member]
	// So if a group, user, UUID, controller name comes in, it will always be index 3 for them
	// and if a relation specifier is present, it will always be index 10
	jujuURIMatcher = regexp.MustCompile(`([a-zA-Z0-9]*)(\-|\z)([a-zA-Z0-9-@.]*)(\:|)([a-zA-Z0-9-@.]*)(\/|)([a-zA-Z0-9-]*)(\.|)([a-zA-Z0-9-]*)([a-zA-Z#]*|\z)\z`)
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

// JWTGeneratorDatabase specifies the database interface used by the
// JWT generator.
type JWTGeneratorDatabase interface {
	GetController(ctx context.Context, controller *dbmodel.Controller) error
}

// JWTGeneratorAccessChecker specifies the access checker used by the JWT
// generator to obtain user's access rights to various entities.
type JWTGeneratorAccessChecker interface {
	GetUserModelAccess(context.Context, *openfga.User, names.ModelTag) (string, error)
	GetUserControllerAccess(context.Context, *openfga.User, names.ControllerTag) (string, error)
	GetUserCloudAccess(context.Context, *openfga.User, names.CloudTag) (string, error)
	CheckPermission(context.Context, *openfga.User, map[string]string, map[string]interface{}) (map[string]string, error)
}

// JWTService specifies the service JWT generator uses to generate JWTs.
type JWTService interface {
	NewJWT(context.Context, jimmjwx.JWTParams) ([]byte, error)
}

// JWTGenerator provides the necessary state and methods to authorize a user and generate JWT tokens.
type JWTGenerator struct {
	database      JWTGeneratorDatabase
	accessChecker JWTGeneratorAccessChecker
	jwtService    JWTService

	mu             sync.Mutex
	accessMapCache map[string]string
	mt             names.ModelTag
	ct             names.ControllerTag
	user           *openfga.User
	callCount      int
}

// NewJWTGenerator returns a new JwtAuthorizer struct
func NewJWTGenerator(database JWTGeneratorDatabase, accessChecker JWTGeneratorAccessChecker, jwtService JWTService) JWTGenerator {
	return JWTGenerator{
		database:      database,
		accessChecker: accessChecker,
		jwtService:    jwtService,
	}
}

// SetTags implements TokenGenerator
func (auth *JWTGenerator) SetTags(mt names.ModelTag, ct names.ControllerTag) {
	auth.mt = mt
	auth.ct = ct
}

// SetTags implements TokenGenerator
func (auth *JWTGenerator) GetUser() names.UserTag {
	if auth.user != nil {
		return auth.user.ResourceTag()
	}
	return names.UserTag{}
}

// MakeLoginToken authorizes the user based on the provided login requests and returns
// a JWT containing claims about user's access to the controller, model (if applicable)
// and all clouds that the controller knows about.
func (auth *JWTGenerator) MakeLoginToken(ctx context.Context, user *openfga.User) ([]byte, error) {
	const op = errors.Op("jimm.MakeLoginToken")

	auth.mu.Lock()
	defer auth.mu.Unlock()

	if user == nil {
		return nil, errors.E(op, "user not specified")
	}
	auth.user = user

	// Recreate the accessMapCache to prevent leaking permissions across multiple login requests.
	auth.accessMapCache = make(map[string]string)
	var authErr error

	var modelAccess string
	if auth.mt.Id() == "" {
		return nil, errors.E(op, "model not set")
	}
	modelAccess, authErr = auth.accessChecker.GetUserModelAccess(ctx, auth.user, auth.mt)
	if authErr != nil {
		zapctx.Error(ctx, "model access check failed", zap.Error(authErr))
		return nil, authErr
	}
	auth.accessMapCache[auth.mt.String()] = modelAccess

	if auth.ct.Id() == "" {
		return nil, errors.E(op, "controller not set")
	}
	var controllerAccess string
	controllerAccess, authErr = auth.accessChecker.GetUserControllerAccess(ctx, auth.user, auth.ct)
	if authErr != nil {
		return nil, authErr
	}
	auth.accessMapCache[auth.ct.String()] = controllerAccess

	var ctl dbmodel.Controller
	ctl.SetTag(auth.ct)
	err := auth.database.GetController(ctx, &ctl)
	if err != nil {
		zapctx.Error(ctx, "failed to fetch controller", zap.Error(err))
		return nil, errors.E(op, "failed to fetch controller", err)
	}
	clouds := make(map[names.CloudTag]bool)
	for _, cloudRegion := range ctl.CloudRegions {
		clouds[cloudRegion.CloudRegion.Cloud.ResourceTag()] = true
	}
	for cloudTag := range clouds {
		accessLevel, err := auth.accessChecker.GetUserCloudAccess(ctx, auth.user, cloudTag)
		if err != nil {
			zapctx.Error(ctx, "cloud access check failed", zap.Error(err))
			return nil, errors.E(op, "failed to check user's cloud access", err)
		}
		auth.accessMapCache[cloudTag.String()] = accessLevel
	}

	return auth.jwtService.NewJWT(ctx, jimmjwx.JWTParams{
		Controller: auth.ct.Id(),
		User:       auth.user.Tag().String(),
		Access:     auth.accessMapCache,
	})
}

// MakeToken assumes MakeLoginToken has already been called and checks the permissions
// specified in the permissionMap. If the logged in user has all those permissions
// a JWT will be returned with assertions confirming all those permissions.
func (auth *JWTGenerator) MakeToken(ctx context.Context, permissionMap map[string]interface{}) ([]byte, error) {
	const op = errors.Op("jimm.MakeToken")

	auth.mu.Lock()
	defer auth.mu.Unlock()

	if auth.callCount >= 10 {
		return nil, errors.E(op, "Permission check limit exceeded")
	}
	auth.callCount++
	if auth.user == nil {
		return nil, errors.E(op, "User authorization missing.")
	}
	if permissionMap != nil {
		var err error
		auth.accessMapCache, err = auth.accessChecker.CheckPermission(ctx, auth.user, auth.accessMapCache, permissionMap)
		if err != nil {
			return nil, err
		}
	}
	jwt, err := auth.jwtService.NewJWT(ctx, jimmjwx.JWTParams{
		Controller: auth.ct.Id(),
		User:       auth.user.Tag().String(),
		Access:     auth.accessMapCache,
	})
	if err != nil {
		return nil, err
	}
	return jwt, nil
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

	switch tag.Kind {
	case names.UserTagKind:
		return names.UserTagKind + "-" + tag.ID, nil
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
		controllerString := names.ControllerTagKind + "-" + controller.Name
		if tag.Relation.String() != "" {
			controllerString = controllerString + "#" + tag.Relation.String()
		}
		return controllerString, nil
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
		modelString := names.ModelTagKind + "-" + model.Controller.Name + ":" + model.OwnerIdentityName + "/" + model.Name
		if tag.Relation.String() != "" {
			modelString = modelString + "#" + tag.Relation.String()
		}
		return modelString, nil
	case names.ApplicationOfferTagKind:
		ao := dbmodel.ApplicationOffer{
			UUID: tag.ID,
		}
		err := j.Database.GetApplicationOffer(ctx, &ao)
		if err != nil {
			return "", errors.E(err, fmt.Sprintf("failed to fetch application offer information: %s", ao.UUID))
		}
		aoString := names.ApplicationOfferTagKind + "-" + ao.Model.Controller.Name + ":" + ao.Model.OwnerIdentityName + "/" + ao.Model.Name + "." + ao.Name
		if tag.Relation.String() != "" {
			aoString = aoString + "#" + tag.Relation.String()
		}
		return aoString, nil
	case jimmnames.GroupTagKind:
		group := dbmodel.GroupEntry{
			UUID: tag.ID,
		}
		err := j.Database.GetGroup(ctx, &group)
		if err != nil {
			return "", errors.E(err, fmt.Sprintf("failed to fetch group information: %s", group.UUID))
		}
		groupString := jimmnames.GroupTagKind + "-" + group.Name
		if tag.Relation.String() != "" {
			groupString = groupString + "#" + tag.Relation.String()
		}
		return groupString, nil
	case names.CloudTagKind:
		cloud := dbmodel.Cloud{
			Name: tag.ID,
		}
		err := j.Database.GetCloud(ctx, &cloud)
		if err != nil {
			return "", errors.E(err, fmt.Sprintf("failed to fetch cloud information: %s", cloud.Name))
		}
		cloudString := names.CloudTagKind + "-" + cloud.Name
		if tag.Relation.String() != "" {
			cloudString = cloudString + "#" + tag.Relation.String()
		}
		return cloudString, nil
	default:
		return "", errors.E(fmt.Sprintf("unexpected tag kind: %v", tag.Kind))
	}
}

type tagResolver struct {
	resourceUUID   string
	trailer        string
	controllerName string
	userName       string
	modelName      string
	offerName      string
	relation       ofga.Relation
}

func newTagResolver(tag string) (*tagResolver, string, error) {
	matches := jujuURIMatcher.FindStringSubmatch(tag)
	tagKind := matches[1]
	resourceUUID := ""
	trailer := ""
	// We first attempt to see if group3 is a uuid
	if _, err := uuid.Parse(matches[3]); err == nil {
		// We know it's a UUID
		resourceUUID = matches[3]
	} else {
		// We presume it's a user or a group
		trailer = matches[3]
	}

	// Matchers along the way to determine segments of the string, they'll be empty
	// if the match has failed
	controllerName := matches[3]
	userName := matches[5]
	modelName := matches[7]
	offerName := matches[9]
	relationString := strings.TrimLeft(matches[10], "#")
	relation, err := ofganames.ParseRelation(relationString)
	if err != nil {
		return nil, "", errors.E("failed to parse relation", errors.CodeBadRequest)
	}
	return &tagResolver{
		resourceUUID:   resourceUUID,
		trailer:        trailer,
		controllerName: controllerName,
		userName:       userName,
		modelName:      modelName,
		offerName:      offerName,
		relation:       relation,
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
	var entry dbmodel.GroupEntry
	if t.resourceUUID != "" {
		entry.UUID = t.resourceUUID
	} else if t.trailer != "" {
		entry.Name = t.trailer
	}

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
	controller := dbmodel.Controller{}

	if t.resourceUUID != "" {
		controller.UUID = t.resourceUUID
	} else if t.controllerName != "" {
		if t.controllerName == jimmControllerName {
			return ofganames.ConvertTagWithRelation(names.NewControllerTag(jimmUUID), t.relation), nil
		}
		controller.Name = t.controllerName
	}

	// NOTE (alesstimec) Do we need to special-case the
	// controller-jimm case - jimm controller does not exist
	// in the database, but has a clearly defined UUID?

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
	model := dbmodel.Model{}

	if t.resourceUUID != "" {
		model.UUID = sql.NullString{String: t.resourceUUID, Valid: true}
	} else if t.controllerName != "" && t.userName != "" && t.modelName != "" {
		controller := dbmodel.Controller{Name: t.controllerName}
		err := db.GetController(ctx, &controller)
		if err != nil {
			return nil, errors.E("controller not found")
		}
		model.ControllerID = controller.ID
		model.OwnerIdentityName = t.userName
		model.Name = t.modelName
	}

	err := db.GetModel(ctx, &model)
	if err != nil {
		return nil, errors.E("model not found")
	}

	return ofganames.ConvertTagWithRelation(names.NewModelTag(model.UUID.String), t.relation), nil
}

func (t *tagResolver) applicationOfferTag(ctx context.Context, db *db.Database) (*ofga.Entity, error) {
	zapctx.Debug(
		ctx,
		"Resolving JIMM tags to Juju tags for tag kind: applicationoffer",
	)
	offer := dbmodel.ApplicationOffer{}

	if t.resourceUUID != "" {
		offer.UUID = t.resourceUUID
	} else if t.controllerName != "" && t.userName != "" && t.modelName != "" && t.offerName != "" {
		offerURL, err := crossmodel.ParseOfferURL(fmt.Sprintf("%s:%s/%s.%s", t.controllerName, t.userName, t.modelName, t.offerName))
		if err != nil {
			zapctx.Debug(
				ctx,
				"failed to parse application offer url",
				zap.String("url", fmt.Sprintf("%s:%s/%s.%s", t.controllerName, t.userName, t.modelName, t.offerName)),
				zaputil.Error(err),
			)
			return nil, errors.E("failed to parse offer url", err)
		}
		offer.URL = offerURL.String()
	}

	err := db.GetApplicationOffer(ctx, &offer)
	if err != nil {
		return nil, errors.E("application offer not found")
	}

	return ofganames.ConvertTagWithRelation(offer.ResourceTag(), t.relation), nil
}
func (t *tagResolver) serviceAccountTag(ctx context.Context) (*ofga.Entity, error) {
	zapctx.Debug(
		ctx,
		"Resolving JIMM tags to Juju tags for tag kind: serviceaccount",
		zap.String("serviceaccount-name", t.trailer),
	)
	st, err := jimmnames.ParseServiceAccountTag(t.trailer)
	if err != nil {
		return nil, errors.E(err)
	}
	return ofganames.ConvertTagWithRelation(st, t.relation), nil
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
		return nil, errors.E("failed to setup tag resolver", err)
	}

	switch tagKind {
	case names.UserTagKind:
		return resolver.userTag(ctx)
	case jimmnames.GroupTagKind:
		return resolver.groupTag(ctx, db)
	case names.ControllerTagKind:
		return resolver.controllerTag(ctx, jimmUUID, db)
	case names.ModelTagKind:
		return resolver.modelTag(ctx, db)
	case names.ApplicationOfferTagKind:
		return resolver.applicationOfferTag(ctx, db)
	case jimmnames.ServiceAccountTagKind:
		return resolver.serviceAccountTag(ctx)
	}
	return nil, errors.E("failed to map tag " + tagKind)
}

// ParseTag attempts to parse the provided key into a tag whilst additionally
// ensuring the resource exists for said tag.
//
// This key may be in the form of either a JIMM tag string or Juju tag string.
func (j *JIMM) ParseTag(ctx context.Context, key string) (*ofganames.Tag, error) {
	op := errors.Op("jimm.ParseTag")
	tupleKeySplit := strings.SplitN(key, "-", 2)
	if len(tupleKeySplit) < 2 {
		return nil, errors.E(op, errors.CodeFailedToParseTupleKey, "tag does not have tuple key delimiter")
	}
	tagString := key
	tag, err := resolveTag(j.UUID, &j.Database, tagString)
	if err != nil {
		zapctx.Debug(ctx, "failed to resolve tuple object", zap.Error(err))
		return nil, errors.E(op, errors.CodeFailedToResolveTupleResource, err)
	}
	zapctx.Debug(ctx, "resolved JIMM tag", zap.String("tag", tag.String()))

	return tag, nil
}

// AddGroup creates a group within JIMMs DB for reference by OpenFGA.
func (j *JIMM) AddGroup(ctx context.Context, user *openfga.User, name string) (*dbmodel.GroupEntry, error) {
	const op = errors.Op("jimm.AddGroup")

	if !user.JimmAdmin {
		return nil, errors.E(op, errors.CodeUnauthorized, "unauthorized")
	}

	ge, err := j.Database.AddGroup(ctx, name)
	if err != nil {
		return nil, errors.E(op, err)
	}
	return ge, nil
}

// RenameGroup renames a group in JIMM's DB.
func (j *JIMM) RenameGroup(ctx context.Context, user *openfga.User, oldName, newName string) error {
	const op = errors.Op("jimm.RenameGroup")

	if !user.JimmAdmin {
		return errors.E(op, errors.CodeUnauthorized, "unauthorized")
	}

	group := &dbmodel.GroupEntry{
		Name: oldName,
	}
	err := j.Database.GetGroup(ctx, group)
	if err != nil {
		return errors.E(op, err)
	}
	group.Name = newName

	if err := j.Database.UpdateGroup(ctx, group); err != nil {
		return errors.E(op, err)
	}
	return nil
}

// RemoveGroup removes a group within JIMMs DB for reference by OpenFGA.
func (j *JIMM) RemoveGroup(ctx context.Context, user *openfga.User, name string) error {
	const op = errors.Op("jimm.RemoveGroup")

	if !user.JimmAdmin {
		return errors.E(op, errors.CodeUnauthorized, "unauthorized")
	}

	group := &dbmodel.GroupEntry{
		Name: name,
	}
	err := j.Database.GetGroup(ctx, group)
	if err != nil {
		return errors.E(op, err)
	}
	err = j.OpenFGAClient.RemoveGroup(ctx, group.ResourceTag())
	if err != nil {
		return errors.E(op, err)
	}

	if err := j.Database.RemoveGroup(ctx, group); err != nil {
		return errors.E(op, err)
	}
	return nil
}

// ListGroups returns a list of groups known to JIMM.
func (j *JIMM) ListGroups(ctx context.Context, user *openfga.User) ([]dbmodel.GroupEntry, error) {
	const op = errors.Op("jimm.ListGroups")

	if !user.JimmAdmin {
		return nil, errors.E(op, errors.CodeUnauthorized, "unauthorized")
	}

	var groups []dbmodel.GroupEntry
	err := j.Database.ForEachGroup(ctx, func(ge *dbmodel.GroupEntry) error {
		groups = append(groups, *ge)
		return nil
	})
	if err != nil {
		return nil, errors.E(op, err)
	}
	return groups, nil
}
