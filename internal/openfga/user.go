// Copyright 2023 CanonicalLtd.

package openfga

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/juju/names/v4"
	"github.com/juju/zaputil/zapctx"
	openfga "github.com/openfga/go-sdk"
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
	allowed, _, err := checkRelation(ctx, u, resource, ofganames.CanAddModelRelation)
	if err != nil {
		return false, errors.E(err)
	}
	return allowed, nil
}

// IsApplicationOfferConsumer returns true if user has consumer relation to the application offer.
func (u *User) IsApplicationOfferConsumer(ctx context.Context, resource names.ApplicationOfferTag) (bool, error) {
	isConsumer, _, err := checkRelation(ctx, u, resource, ofganames.ConsumerRelation)
	if err != nil {
		return false, errors.E(err)
	}
	return isConsumer, nil
}

// IsApplicationOfferReader returns true if user has reader relation to the application offer.
func (u *User) IsApplicationOfferReader(ctx context.Context, resource names.ApplicationOfferTag) (bool, error) {
	isReader, _, err := checkRelation(ctx, u, resource, ofganames.ReaderRelation)
	if err != nil {
		return false, errors.E(err)
	}
	return isReader, nil
}

// IsModelReader returns true if user has reader relation to the model.
func (u *User) IsModelReader(ctx context.Context, resource names.ModelTag) (bool, error) {
	isReader, _, err := checkRelation(ctx, u, resource, ofganames.ReaderRelation)
	if err != nil {
		return false, errors.E(err)
	}
	return isReader, nil
}

// IsModelWriter returns true if user has writer relation to the model.
func (u *User) IsModelWriter(ctx context.Context, resource names.ModelTag) (bool, error) {
	isWriter, _, err := checkRelation(ctx, u, resource, ofganames.WriterRelation)
	if err != nil {
		return false, errors.E(err)
	}
	return isWriter, nil
}

// GetCloudAccess returns the relation the user has to the specified cloud.
func (u *User) GetCloudAccess(ctx context.Context, resource names.CloudTag) ofganames.Relation {
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

func (u *User) GetControllerAuditLogViewerAccess(ctx context.Context, resource names.ControllerTag) ofganames.Relation {
	hasAccess, _, err := checkRelation(ctx, u, resource, ofganames.AuditLogViewerRelation)
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
func (u *User) GetControllerAccess(ctx context.Context, resource names.ControllerTag) ofganames.Relation {
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
func (u *User) GetModelAccess(ctx context.Context, resource names.ModelTag) ofganames.Relation {
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

// SetModelAccess adds a direct relation between the user and the model.
func (u *User) SetModelAccess(ctx context.Context, resource names.ModelTag, relation ofganames.Relation) error {
	return setResourceAccess(ctx, u, resource, relation)
}

// SetControllerAccess adds a direct relation between the user and the controller.
func (u *User) SetControllerAccess(ctx context.Context, resource names.ControllerTag, relation ofganames.Relation) error {
	return setResourceAccess(ctx, u, resource, relation)
}

// UnsetControllerAccess removes a direct relation between the user and a controller.
func (u *User) UnsetControllerAccess(ctx context.Context, resource names.ControllerTag, relation ofganames.Relation) error {
	return unsetResourceAccess(ctx, u, resource, relation)
}

// SetCloudAccess adds a direct relation between the user and the cloud.
func (u *User) SetCloudAccess(ctx context.Context, resource names.CloudTag, relation ofganames.Relation) error {
	return setResourceAccess(ctx, u, resource, relation)
}

// SetApplicationOfferAccess adds a direct relation between the user and the application offer.
func (u *User) SetApplicationOfferAccess(ctx context.Context, resource names.ApplicationOfferTag, relation ofganames.Relation) error {
	return setResourceAccess(ctx, u, resource, relation)
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

func checkRelation[T ofganames.ResourceTagger](ctx context.Context, u *User, resource T, relation ofganames.Relation) (bool, string, error) {
	isAllowed, resolution, err := u.client.checkRelation(
		ctx,
		Tuple{
			Object:   ofganames.ConvertTag(u.ResourceTag()),
			Relation: relation,
			Target:   ofganames.ConvertTag(resource),
		},
		true,
	)
	if err != nil {
		return false, "", errors.E(err)
	}

	return isAllowed, resolution, nil
}

// CheckRelation accepts a resource as a tag and checks if the user has the specified relation to the resource.
// The resource string will be converted to a tag. In cases where one already has a resource tag, consider using
// the convenience functions like `IsModelWriter` or `IsApplicationOfferConsumer`.
func CheckRelation(ctx context.Context, u *User, resource names.Tag, relation ofganames.Relation) (bool, string, error) {
	var tag *ofganames.Tag
	var err error
	tag = ofganames.ConvertGenericTag(resource)
	isAllowed, resolution, err := u.client.checkRelation(
		ctx,
		Tuple{
			Object:   ofganames.ConvertTag(u.ResourceTag()),
			Relation: relation,
			Target:   tag,
		},
		true,
	)
	if err != nil {
		return false, "", errors.E(err)
	}

	return isAllowed, resolution, nil
}

// IsAdministrator returns true if user has administrator access to the resource.
func IsAdministrator[T administratorT](ctx context.Context, u *User, resource T) (bool, error) {
	isAdmin, resolution, err := checkRelation(ctx, u, resource, ofganames.AdministratorRelation)
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
			zap.Any("resolution", resolution),
		)
	}
	return isAdmin, nil
}

func setResourceAccess[T ofganames.ResourceTagger](ctx context.Context, user *User, resource T, relation ofganames.Relation) error {
	err := user.client.addRelation(ctx, Tuple{
		Object:   ofganames.ConvertTag(user.ResourceTag()),
		Relation: relation,
		Target:   ofganames.ConvertTag(resource),
	})
	if err != nil {
		// if the tuple already exist we don't return an error.
		if strings.Contains(err.Error(), "cannot write a tuple which already exists") {
			return nil
		}
		return errors.E(err)
	}

	return nil
}

func unsetResourceAccess[T ofganames.ResourceTagger](ctx context.Context, user *User, resource T, relation ofganames.Relation) error {
	err := user.client.removeRelation(ctx, Tuple{
		Object:   ofganames.ConvertTag(user.ResourceTag()),
		Relation: relation,
		Target:   ofganames.ConvertTag(resource),
	})
	if err != nil {
		// if the tuple does not exist we don't return an error.
		if strings.Contains(err.Error(), "cannot delete a tuple which does not exist") {
			return nil
		}
		return errors.E(err)
	}

	return nil
}

// ListUsersWithAccess lists all users that have the specified relation to the resource.
func ListUsersWithAccess[T ofganames.ResourceTagger](ctx context.Context, client *OFGAClient, resource T, relation ofganames.Relation) ([]*User, error) {
	t := createTupleKey(Tuple{
		Relation: relation,
		Target:   ofganames.ConvertTag(resource),
	})

	list, err := listUsersWithAccess(ctx, client, t)
	if err != nil {
		zapctx.Error(ctx, "failed to list related users", zap.Error(err))
		return nil, errors.E(err)
	}

	users := make([]*User, len(list))
	for i, user := range list {
		users[i] = NewUser(
			&dbmodel.User{
				Username: strings.TrimPrefix(user, "user:"),
			},
			client,
		)

	}
	return users, nil

}

func listUsersWithAccess(ctx context.Context, client *OFGAClient, tuple openfga.TupleKey) ([]string, error) {
	// we create an expand request
	er := openfga.NewExpandRequest(tuple)
	er.SetAuthorizationModelId(client.AuthModelId)

	res, _, err := client.api.Expand(ctx).Body(*er).Execute()
	if err != nil {
		zapctx.Error(ctx, "failed to query for related object", zap.Error(err))
		return nil, err
	}

	// and process the returned tree
	tree := res.GetTree()
	if !tree.HasRoot() {
		return nil, errors.E("unexpected tree structure")
	}
	root := tree.GetRoot()
	rootUsers, err := traverseTree(ctx, client, &root)
	if err != nil {
		return nil, errors.E(err)
	}
	var users []string
	for username, _ := range rootUsers {
		users = append(users, username)
	}
	return users, nil
}

// traverseTree will explore the tree returned by openfga Expand call
// to find all users that have access to the resource.
func traverseTree(ctx context.Context, client *OFGAClient, node *openfga.Node) (map[string]bool, error) {
	logError := func(message, nodeType string, n interface{}) {
		data, _ := json.Marshal(n)
		zapctx.Error(ctx, message, zap.String(nodeType, string(data)))
	}

	// If this is a union node, we traverse all child nodes and
	// join the results.
	if node.HasUnion() {
		union := node.GetUnion()
		users := make(map[string]bool)
		for _, childNode := range union.GetNodes() {
			nodeUsers, err := traverseTree(ctx, client, &childNode)
			if err != nil {
				return nil, errors.E(err)
			}
			for nodeUser, _ := range nodeUsers {
				users[nodeUser] = true
			}
		}
		return users, nil
	}
	// A-ha! a leaf node!
	if node.HasLeaf() {
		leaf := node.GetLeaf()

		// A leaf node may list users: in this case we still need
		// to run the list through expandList.
		if leaf.HasUsers() {
			leafUsers, err := expandList(ctx, client, *leaf.Users.Users)
			if err != nil {
				return nil, errors.E(err)
			}
			return leafUsers, nil
		} else if leaf.HasComputed() {
			computed := leaf.GetComputed()
			// A computed leaf node needs to have a userset
			// (e.g. applicationoffer:a8513c54-6eb0-4058-84c7-225e03c4d0b5#consumer)
			if computed.HasUserset() {
				userset := computed.GetUserset()
				computedUsers, err := expandUserset(ctx, client, userset)
				if err != nil {
					return nil, errors.E(err)
				}
				return computedUsers, nil
			} else {
				logError("missing userset", "leaf", leaf)
				return nil, errors.E("missing userset")
			}
		} else if leaf.HasTupleToUserset() {
			tupleToUserset := leaf.GetTupleToUserset()
			if tupleToUserset.HasComputed() {
				computedList := tupleToUserset.GetComputed()
				users := make(map[string]bool)
				// We're interested in the list of computed nodes
				// this TupleToUserset contains -  we need to get a list
				// of users that have access to each of these and join the list.
				for _, computed := range computedList {
					if computed.HasUserset() {
						userset := computed.GetUserset()
						computedUsers, err := expandUserset(ctx, client, userset)
						if err != nil {
							return nil, errors.E(err)
						}
						for computedUser, _ := range computedUsers {
							users[computedUser] = true
						}
					} else {
						logError("tupleToUserset: missing userset", "leaf", leaf)
						return nil, errors.E("missing userset")
					}
				}
				return users, nil
			}
		} else {
			logError("unknown leaf type", "leaf", leaf)
			return nil, errors.E("unknown leaf type")
		}
	}
	logError("unknown node type", "node", node)
	return nil, errors.E("unknown node type")
}

// expandUserset expects to receive a userset of the
// "entity#relation" format and will recursively call listUsersWithAccess
// to expand the userset get the users that have the specified
// relation to the entity contained in the userset.
func expandUserset(ctx context.Context, client *OFGAClient, userset string) (map[string]bool, error) {
	tokens := strings.Split(userset, "#")
	if len(tokens) != 2 {
		zapctx.Error(ctx, "unexpected userset", zap.String("userset", userset))
		return nil, errors.E("unexpected userset")
	}
	computedTuple := openfga.NewTupleKey()
	computedTuple.SetRelation(tokens[1])
	computedTuple.SetObject(tokens[0])
	users, err := listUsersWithAccess(ctx, client, *computedTuple)
	if err != nil {
		return nil, errors.E(err)
	}
	computedUsers, err := expandList(ctx, client, users)
	if err != nil {
		return nil, errors.E(err)
	}
	return computedUsers, nil
}

// expandList checks all entities in the list
//   - an entity may be a username, in that case it is direcly added to the list
//   - or it may tell us that users with a certain kind of relation
//     to the specified entity have access to the resource: e.g. group#members or
//     model#administrator
func expandList(ctx context.Context, client *OFGAClient, entities []string) (map[string]bool, error) {
	users := make(map[string]bool)
	for _, entity := range entities {
		tokens := strings.Split(entity, "#")
		switch len(tokens) {
		case 1:
			// entity does not contain a # so it is a username
			// - we add it to the map
			users[entity] = true
		case 2:
			// ok, we need to check users that have relation
			// specified in tokens[1] to resource specified
			// in tokens[0]
			t := openfga.NewTupleKey()
			t.SetRelation(tokens[1])
			t.SetObject(tokens[0])
			newUsers, err := listUsersWithAccess(ctx, client, *t)
			if err != nil {
				return nil, errors.E(err)
			}
			for _, username := range newUsers {
				users[username] = true
			}
		default:
			zapctx.Error(ctx, "unknown entity type", zap.String("entity", entity))
			return nil, errors.E("unknown entity type")
		}
	}
	return users, nil
}
