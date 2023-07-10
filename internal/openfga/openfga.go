// Copyright 2023 CanonicalLtd.

package openfga

import (
	"context"
	"strings"

	"github.com/CanonicalLtd/jimm/internal/errors"
	ofganames "github.com/CanonicalLtd/jimm/internal/openfga/names"
	jimmnames "github.com/CanonicalLtd/jimm/pkg/names"
	cofga "github.com/canonical/ofga"
	"github.com/juju/names/v4"
)

var (
	// resourceTypes contains a list of all resource kinds (i.e. tags) used throughout JIMM.
	resourceTypes = [...]string{names.UserTagKind, names.ModelTagKind, names.ControllerTagKind, names.ApplicationOfferTagKind, jimmnames.GroupTagKind}
)

// Tuple represents a relation between an object and a target.
type Tuple = cofga.Tuple

type Tag = ofganames.Tag
type Kind = ofganames.Kind

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
	cofgaClient *cofga.Client
}

// NewOpenFGAClient returns a new JIMM-specific client that wraps the given core OpenFGA client.
func NewOpenFGAClient(cofgaClient *cofga.Client) *OFGAClient {
	return &OFGAClient{cofgaClient: cofgaClient}
}

// getRelatedObjects returns all objects where the user has a valid relation to them.
// Such as all the groups a user resides in.
//
// The underlying tuple is managed by this method and as such you need only provide the "tuple_key" segment. See CreateTupleKey
//
// The results may be paginated via a pageSize and the initial returned continuation token from the first request.
func (o *OFGAClient) getRelatedObjects(ctx context.Context, tuple Tuple, pageSize int32, continuationToken string) ([]Tuple, string, error) {
	timestampedTuples, ct, err := o.cofgaClient.FindMatchingTuples(ctx, tuple, pageSize, continuationToken)
	if err != nil {
		return nil, "", err
	}
	tuples := make([]Tuple, len(timestampedTuples))
	for i, tt := range timestampedTuples {
		tuples[i] = tt.Tuple
	}
	return tuples, ct, nil
}

// listObjects lists all the object IDs a user has access to via the specified relation
// It is experimental and subject to context deadlines. See: https://openfga.dev/docs/interacting/relationship-queries#summary
//
// The arguments for this call also slightly differ, as it is not an ordinary tuple to be set. The target object
// must NOT include the ID, i.e.,
//
//   - "group:" vs "group:mygroup", where "mygroup" is the ID and the correct objType would be "group".
func (o *OFGAClient) listObjects(ctx context.Context, user string, relation string, objType string, contextualTuples []Tuple) (objectIds []string, err error) {
	userEntity, err := cofga.ParseEntity(user)
	if err != nil {
		return nil, err
	}
	entities, err := o.cofgaClient.FindAccessibleObjectsByRelation(ctx, Tuple{
		Object:   &userEntity,
		Relation: cofga.Relation(relation),
		Target:   &cofga.Entity{Kind: cofga.Kind(objType)},
	}, contextualTuples...)
	if err != nil {
		return nil, err
	}
	result := make([]string, len(entities))
	for i, entity := range entities {
		result[i] = entity.String()
	}
	return result, nil
}

// AddRelations creates a tuple(s) from the provided keys. See CreateTupleKey for creating keys.
func (o *OFGAClient) AddRelations(ctx context.Context, tuples ...Tuple) error {
	return o.cofgaClient.AddRelation(ctx, tuples...)
}

// RemoveRelation creates a tuple(s) from the provided keys. See CreateTupleKey for creating keys.
func (o *OFGAClient) RemoveRelation(ctx context.Context, tuples ...Tuple) error {
	return o.cofgaClient.RemoveRelation(ctx, tuples...)
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
// You may read via pagination utilising the continuation token returned from the request.
func (o *OFGAClient) ReadRelatedObjects(ctx context.Context, tuple Tuple, pageSize int32, continuationToken string) ([]Tuple, string, error) {
	return o.getRelatedObjects(ctx, tuple, pageSize, continuationToken)
}

// CheckRelation verifies that a user (or object) is allowed to access the target object by the specified relation.
//
// It will return a bool of simply true or false, denoting authorisation, and an error.
func (o *OFGAClient) CheckRelation(ctx context.Context, tuple Tuple, trace bool) (bool, error) {
	if trace {
		return o.cofgaClient.CheckRelationWithTracing(ctx, tuple)
	}
	return o.cofgaClient.CheckRelation(ctx, tuple)
}

// removeTuples iteratively reads through all the tuples with the parameters as supplied by key and deletes them.
func (o *OFGAClient) removeTuples(ctx context.Context, tuple Tuple) error {
	pageSize := 50
	for {
		// Since we're deleting the returned tuples, it's best to avoid pagination,
		// and fresh query for the relations.
		tuples, ct, err := o.ReadRelatedObjects(ctx, tuple, int32(pageSize), "")
		if err != nil {
			return err
		}
		if len(tuples) > 0 {
			err = o.RemoveRelation(ctx, tuples...)
			if err != nil {
				return err
			}
		}
		if ct == "" {
			return nil
		}
	}
}

// AddControllerModel adds a relation between a controller and a model.
func (o *OFGAClient) AddControllerModel(ctx context.Context, controller names.ControllerTag, model names.ModelTag) error {
	if err := o.AddRelations(
		ctx,
		Tuple{
			Object:   ofganames.ConvertTag(controller),
			Relation: ofganames.ControllerRelation,
			Target:   ofganames.ConvertTag(model),
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
		Tuple{
			Target: ofganames.ConvertTag(model),
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
			Object:   ofganames.ConvertTag(model),
			Relation: ofganames.ModelRelation,
			Target:   ofganames.ConvertTag(offer),
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
		Tuple{
			Target: ofganames.ConvertTag(offer),
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
		Tuple{
			Relation: ofganames.MemberRelation,
			Target:   ofganames.ConvertTag(group),
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
		newKey := Tuple{
			Object: ofganames.ConvertTagWithRelation(group, ofganames.MemberRelation),
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
		Tuple{
			Target: ofganames.ConvertTag(cloud),
		},
	); err != nil {
		return errors.E(err)
	}
	return nil
}

// AddCloudController adds a controller relation between a controller and
// a cloud.
func (o *OFGAClient) AddCloudController(ctx context.Context, cloud names.CloudTag, controller names.ControllerTag) error {
	if err := o.AddRelations(ctx, Tuple{
		Object:   ofganames.ConvertTag(controller),
		Relation: ofganames.ControllerRelation,
		Target:   ofganames.ConvertTag(cloud),
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
	if err := o.AddRelations(ctx, Tuple{
		Object:   ofganames.ConvertTag(jimm),
		Relation: ofganames.ControllerRelation,
		Target:   ofganames.ConvertTag(controller),
	}); err != nil {
		// if the tuple already exist we don't return an error.
		if strings.Contains(err.Error(), "cannot write a tuple which already exists") {
			return nil
		}
		return errors.E(err)
	}
	return nil
}
