// Copyright 2023 CanonicalLtd.

package openfga

import (
	"context"
	"strings"

	"github.com/juju/names/v4"
	"github.com/juju/zaputil/zapctx"
	openfga "github.com/openfga/go-sdk"
	"go.uber.org/zap"

	"github.com/CanonicalLtd/jimm/internal/errors"
	ofganames "github.com/CanonicalLtd/jimm/internal/openfga/names"
	jimmnames "github.com/CanonicalLtd/jimm/pkg/names"
)

var (
	// resourceTypes contains a list of all resource kinds (i.e. tags) used throughout JIMM.
	resourceTypes = [...]string{names.UserTagKind, names.ModelTagKind, names.ControllerTagKind, names.ApplicationOfferTagKind, jimmnames.GroupTagKind}
)

// Tuple represents a relation between an object and a target.
type Tuple struct {
	Object   *ofganames.Tag
	Relation ofganames.Relation
	Target   *ofganames.Tag
}

func (t Tuple) toOpenFGATuple() openfga.TupleKey {
	return createTupleKey(t)
}

// ReadResponse takes what is necessary from the underlying OFGA ReadResponse and simplifies it
// into a safe ready-to-use response.
type ReadResponse struct {
	Tuples          []Tuple
	PaginationToken string
}

// OFGAClient contains convenient utility methods for interacting
// with OpenFGA from OUR usecase. It wraps the provided pre-generated client
// from OpenFGA.
//
// It makes no promises as to whether the underlying client is "correctly configured" however.
//
// It is worth noting that any time the term 'User' is used, this COULD represent ANOTHER object, for example:
// a group can relate to a user as a 'member', if a user is a 'member' of that group, and that group
// is an administrator of the controller, a byproduct of this is that the flow will look like so:
//
// user:alex -> member -> group:yellow -> administrator -> controller:<uuid>
//
// In the above scenario, alex becomes an administrator due the the 'user' aka group:yellow being
// an administrator.
type OFGAClient struct {
	api         openfga.OpenFgaApi
	AuthModelId string
}

// NewOpenFGAClient returns a wrapped OpenFGA API client ensuring all calls are made to the provided
// authorisation model (id) and returns what is necessary.
func NewOpenFGAClient(a openfga.OpenFgaApi, authModelId string) *OFGAClient {
	return &OFGAClient{api: a, AuthModelId: authModelId}
}

// addRelation adds user(s) to the specified object by the specified relation within the tuple keys given.
func (o *OFGAClient) addRelation(ctx context.Context, tuples ...Tuple) error {
	wr := openfga.NewWriteRequest()
	wr.SetAuthorizationModelId(o.AuthModelId)

	tupleKeys := make([]openfga.TupleKey, len(tuples))
	for i, tuple := range tuples {
		tupleKeys[i] = tuple.toOpenFGATuple()
	}

	keys := openfga.NewTupleKeys(tupleKeys)
	wr.SetWrites(*keys)
	_, _, err := o.api.Write(ctx).Body(*wr).Execute()
	if err != nil {
		return err
	}
	return nil
}

// removeRelation deletes user(s) from the specified object by the specified relation within the tuple keys given.
func (o *OFGAClient) removeRelation(ctx context.Context, tuples ...Tuple) error {
	wr := openfga.NewWriteRequest()
	wr.SetAuthorizationModelId(o.AuthModelId)

	tupleKeys := make([]openfga.TupleKey, len(tuples))
	for i, tuple := range tuples {
		tupleKeys[i] = tuple.toOpenFGATuple()
	}

	keys := openfga.NewTupleKeys(tupleKeys)
	wr.SetDeletes(*keys)
	_, _, err := o.api.Write(ctx).Body(*wr).Execute()
	if err != nil {
		return errors.E(err)
	}
	return nil
}

// getRelatedObjects returns all objects where the user has a valid relation to them.
// Such as all the groups a user resides in.
//
// The underlying tuple is managed by this method and as such you need only provide the "tuple_key" segment. See CreateTupleKey
//
// The results may be paginated via a pageSize and the initial returned pagination token from the first request.
func (o *OFGAClient) getRelatedObjects(ctx context.Context, tuple *Tuple, pageSize int32, paginationToken string) (*openfga.ReadResponse, error) {
	rr := openfga.NewReadRequest()

	if pageSize != 0 {
		rr.SetPageSize(pageSize)
	}

	if paginationToken != "" {
		rr.SetContinuationToken(paginationToken)
	}

	if tuple != nil {
		rr.SetTupleKey(tuple.toOpenFGATuple())
	}
	readres, _, err := o.api.Read(ctx).Body(*rr).Execute()
	if err != nil {
		return nil, err
	}
	return &readres, nil
}

// checkRelation verifies that object a, is reachable, via unions or direct relations to object b
func (o *OFGAClient) checkRelation(ctx context.Context, tuple Tuple, trace bool) (bool, string, error) {
	zapctx.Debug(
		ctx,
		"check request internal",
		zap.String("tuple object", tuple.Object.String()),
		zap.String("tuple relation", tuple.Relation.String()),
		zap.String("tuple target object", tuple.Target.String()),
	)
	cr := openfga.NewCheckRequest(tuple.toOpenFGATuple())
	cr.SetAuthorizationModelId(o.AuthModelId)

	cr.SetTrace(trace)
	checkres, httpres, err := o.api.Check(ctx).Body(*cr).Execute()
	if err != nil {
		return false, "", err
	}
	zapctx.Debug(ctx, "check request internal resp code", zap.Int("code", httpres.StatusCode))
	allowed := checkres.GetAllowed()
	resolution := checkres.GetResolution()
	return allowed, resolution, nil
}

// listObjects lists all the object IDs a user has access to via the specified relation
// It is experimental and subject to context deadlines. See: https://openfga.dev/docs/interacting/relationship-queries#summary
//
// The arguments for this call also slightly differ, as it is not an ordinary tuple to be set. The target object
// must NOT include the ID, i.e.,
//
//   - "group:" vs "group:mygroup", where "mygroup" is the ID and the correct objType would be "group".
func (o *OFGAClient) listObjects(ctx context.Context, user string, relation string, objType string, contextualTuples []Tuple) (objectIds []string, err error) {
	zapctx.Debug(
		ctx,
		"list request internal",
	)
	lor := openfga.NewListObjectsRequest(objType, relation, user)
	lor.SetAuthorizationModelId(o.AuthModelId)

	if len(contextualTuples) > 0 {
		keys := make([]openfga.TupleKey, len(contextualTuples))

		for i, ct := range contextualTuples {
			keys[i] = ct.toOpenFGATuple()
		}

		lor.SetContextualTuples(*openfga.NewContextualTupleKeys(keys))
	}

	listres, httpres, err := o.api.ListObjects(ctx).Body(*lor).Execute()
	if err != nil {
		return
	}

	zapctx.Debug(ctx, "list request internal resp code", zap.Int("code", httpres.StatusCode))
	objectIds = listres.GetObjects()
	return
}

// createTuple wraps the underlying ofga tuple into a convenient ease-of-use method
func createTupleKey(tuple Tuple) openfga.TupleKey {
	k := openfga.NewTupleKey()
	// in some cases specifying the object is not required
	if tuple.Object != nil {
		k.SetUser(tuple.Object.String())
	}
	// in some cases specifying the relation is not required
	if tuple.Relation != "" {
		k.SetRelation(string(tuple.Relation))
	}
	k.SetObject(tuple.Target.String())
	return *k
}

// AddRelations creates a tuple(s) from the provided keys. See CreateTupleKey for creating keys.
func (o *OFGAClient) AddRelations(ctx context.Context, tuples ...Tuple) error {
	return o.addRelation(ctx, tuples...)
}

// RemoveRelation creates a tuple(s) from the provided keys. See CreateTupleKey for creating keys.
func (o *OFGAClient) RemoveRelation(ctx context.Context, tuples ...Tuple) error {
	return o.removeRelation(ctx, tuples...)
}

// ListObjects returns all object IDs of <objType> that a user has the relation <relation> to.
func (o *OFGAClient) ListObjects(ctx context.Context, user string, relation string, objType string, contextualTuples []Tuple) ([]string, error) {
	return o.listObjects(ctx, user, relation, objType, contextualTuples)
}

// ReadRelations reads a relation(s) from the provided key where a match can be found.
//
// See: https://openfga.dev/api/service#/Relationship%20Tuples/Read
//
// See: CreateTupleKey for creating keys.
//
// You may read via pagination utilising the token returned from the request.
func (o *OFGAClient) ReadRelatedObjects(ctx context.Context, tuple *Tuple, pageSize int32, paginationToken string) (*ReadResponse, error) {
	keys := []Tuple{}
	res, err := o.getRelatedObjects(ctx, tuple, pageSize, paginationToken)
	if err != nil {
		return nil, errors.E(err)
	}
	tuples, ok := res.GetTuplesOk()
	if ok {
		t := *tuples
		for i := 0; i < len(t); i++ {
			key, ok := t[i].GetKeyOk()
			if ok {
				user, err := ofganames.TagFromString(key.GetUser())
				if err != nil {
					return nil, errors.E(err)
				}
				target, err := ofganames.TagFromString(key.GetObject())
				if err != nil {
					return nil, errors.E(err)
				}
				keys = append(keys, Tuple{
					Object:   user,
					Relation: ofganames.Relation(key.GetRelation()),
					Target:   target,
				})
			}
		}
	}

	token := ""
	t, ok := res.GetContinuationTokenOk()
	if ok {
		token = *t
	}

	return &ReadResponse{Tuples: keys, PaginationToken: token}, nil
}

// CheckRelation verifies that a user (or object) is allowed to access the target object by the specified relation.
//
// It will return:
// - A bool of simply true or false, denoting authorisation
// - A string (if trace is true) explaining WHY this is true [in the case the check succeeds, otherwise an empty string]
// - An error in the event something is wrong when contacting OpenFGA
func (o *OFGAClient) CheckRelation(ctx context.Context, tuple Tuple, trace bool) (bool, string, error) {
	return o.checkRelation(ctx, tuple, trace)
}

// removeTuples iteratively reads through all the tuples with the parameters as supplied by key and deletes them.
func (o *OFGAClient) removeTuples(ctx context.Context, tuple *Tuple) error {
	pageSize := 50
	var token string
	var resp *ReadResponse
	var err error
	for ok := true; ok; ok = (token != resp.PaginationToken) {
		if resp != nil {
			token = resp.PaginationToken
		}
		resp, err = o.ReadRelatedObjects(ctx, tuple, int32(pageSize), token)
		if err != nil {
			return err
		}
		if len(resp.Tuples) > 0 {
			err = o.RemoveRelation(ctx, resp.Tuples...)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// AddControllerModel adds a relation between a controller and a model.
func (o *OFGAClient) AddControllerModel(ctx context.Context, controller names.ControllerTag, model names.ModelTag) error {
	if err := o.AddRelations(
		ctx,
		Tuple{
			Object:   ofganames.FromTag(controller),
			Relation: ofganames.ControllerRelation,
			Target:   ofganames.FromTag(model),
		},
	); err != nil {
		return errors.E(err)
	}
	return nil
}

// RemoveModel removes a model.
func (o *OFGAClient) RemoveModel(ctx context.Context, model names.ModelTag) error {
	if err := o.removeTuples(
		ctx,
		&Tuple{
			Target: ofganames.FromTag(model),
		},
	); err != nil {
		return errors.E(err)
	}
	return nil
}

// AddModelApplicationOffer adds a relation between a model and an application offer.
func (o *OFGAClient) AddModelApplicationOffer(ctx context.Context, model names.ModelTag, offer names.ApplicationOfferTag) error {
	if err := o.AddRelations(
		ctx,
		Tuple{
			Object:   ofganames.FromTag(model),
			Relation: ofganames.ModelRelation,
			Target:   ofganames.FromTag(offer),
		},
	); err != nil {
		return errors.E(err)
	}
	return nil
}

// RemoveApplicationOffer removes an application offer.
func (o *OFGAClient) RemoveApplicationOffer(ctx context.Context, offer names.ApplicationOfferTag) error {
	if err := o.removeTuples(
		ctx,
		&Tuple{
			Target: ofganames.FromTag(offer),
		},
	); err != nil {
		return errors.E(err)
	}
	return nil
}

// RemoveGroup removes a group.
func (o *OFGAClient) RemoveGroup(ctx context.Context, group jimmnames.GroupTag) error {
	if err := o.removeTuples(
		ctx,
		&Tuple{
			Relation: ofganames.MemberRelation,
			Target:   ofganames.FromTag(group),
		},
	); err != nil {
		return errors.E(err)
	}
	// We need to loop through all resource types because the OpenFGA Read API does not provide
	// means for only specifying a user resource, it must be paired with an object type.
	for _, kind := range resourceTypes {
		kt, err := ofganames.BlankKindTag(kind)
		if err != nil {
			return errors.E(err)
		}
		newKey := &Tuple{
			Object: ofganames.FromTagWithRelation(group, ofganames.MemberRelation),
			Target: kt,
		}
		err = o.removeTuples(ctx, newKey)
		if err != nil {
			return errors.E(err)
		}
	}
	return nil
}

// RemoveCloud removes a cloud.
func (o *OFGAClient) RemoveCloud(ctx context.Context, cloud names.CloudTag) error {
	if err := o.removeTuples(
		ctx,
		&Tuple{
			Target: ofganames.FromTag(cloud),
		},
	); err != nil {
		return errors.E(err)
	}
	return nil
}

// AddCloudController adds a controller relation between a controller and
// a cloud.
func (o *OFGAClient) AddCloudController(ctx context.Context, cloud names.CloudTag, controller names.ControllerTag) error {
	if err := o.addRelation(ctx, Tuple{
		Object:   ofganames.FromTag(controller),
		Relation: ofganames.ControllerRelation,
		Target:   ofganames.FromTag(cloud),
	}); err != nil {
		// if the tuple already exist we don't return an error.
		if strings.Contains(err.Error(), "cannot write a tuple which already exists") {
			return nil
		}
		return errors.E(err)
	}
	return nil
}

// AddController adds a controller relation between JIMM and the added controller. Meaning
// JIMM admins also have administrator access to the added controller
func (o *OFGAClient) AddController(ctx context.Context, jimm names.ControllerTag, controller names.ControllerTag) error {
	if err := o.addRelation(ctx, Tuple{
		Object:   ofganames.FromTag(jimm),
		Relation: ofganames.ControllerRelation,
		Target:   ofganames.FromTag(controller),
	}); err != nil {
		// if the tuple already exist we don't return an error.
		if strings.Contains(err.Error(), "cannot write a tuple which already exists") {
			return nil
		}
		return errors.E(err)
	}
	return nil
}
