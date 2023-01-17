package openfga

import (
	"context"

	openfga "github.com/openfga/go-sdk"
)

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

// ReadResponse takes what is necessary from the underlying OFGA ReadResponse and simplifies it
// into a safe ready-to-use response.
type ReadResponse struct {
	Keys            []openfga.TupleKey
	PaginationToken string
}

// NewOpenFGAClient returns a wrapped OpenFGA API client ensuring all calls are made to the provided
// authorisation model (id) and returns what is necessary.
func NewOpenFGAClient(a openfga.OpenFgaApi, authModelId string) *OFGAClient {
	return &OFGAClient{api: a, AuthModelId: authModelId}
}

// addRelation adds user(s) to the specified object by the specified relation within the tuple keys given.
func (o *OFGAClient) addRelation(ctx context.Context, t ...openfga.TupleKey) error {
	wr := openfga.NewWriteRequest()
	wr.SetAuthorizationModelId(o.AuthModelId)
	keys := openfga.NewTupleKeys(t)
	wr.SetWrites(*keys)
	_, _, err := o.api.Write(ctx).Body(*wr).Execute()
	if err != nil {
		return err
	}
	return nil
}

// getRelatedObjects returns all objects where the user has a valid relation to them.
// Such as all the groups a user resides in.
//
// The underlying tuple is managed by this method and as such you need only provide the "tuple_key" segment. See CreateTupleKey
//
// The results may be paginated via a pageSize and the initial returned pagination token from the first request.
func (o *OFGAClient) getRelatedObjects(ctx context.Context, t openfga.TupleKey, pageSize int32, paginationToken string) (*openfga.ReadResponse, error) {
	rr := openfga.NewReadRequest()

	if pageSize != 0 {
		rr.SetPageSize(pageSize)
	}

	if paginationToken != "" {
		rr.SetContinuationToken(paginationToken)
	}

	rr.SetAuthorizationModelId(o.AuthModelId)
	rr.SetTupleKey(t)
	readres, _, err := o.api.Read(ctx).Body(*rr).Execute()
	if err != nil {
		return nil, err
	}
	return &readres, nil
}

// checkRelation verifies that object a, is reachable, via unions or direct relations to object b
func (o *OFGAClient) checkRelation(ctx context.Context, t openfga.TupleKey, trace bool) (bool, string, error) {
	cr := openfga.NewCheckRequest()
	cr.SetAuthorizationModelId(o.AuthModelId)
	cr.SetTupleKey(t)
	cr.SetTrace(trace)
	checkres, _, err := o.api.Check(ctx).Body(*cr).Execute()
	if err != nil {
		return false, "", err
	}
	allowed := checkres.GetAllowed()
	resolution := checkres.GetResolution()
	return allowed, resolution, nil

}

// CreateTuple wraps the underlying ofga tuple into a convenient ease-of-use method
func (o *OFGAClient) CreateTupleKey(object string, relation string, targetObject string) openfga.TupleKey {
	k := openfga.NewTupleKey()
	k.SetUser(object)
	k.SetRelation(relation)
	k.SetObject(targetObject)
	return *k
}

// AddRelations creates a tuple(s) from the provided keys. See CreateTupleKey for creating keys.
func (o *OFGAClient) AddRelations(ctx context.Context, keys ...openfga.TupleKey) error {
	return o.addRelation(ctx, keys...)
}

// ReadRelations reads a relation(s) from the provided key where a match can be found.
//
// See: https://openfga.dev/api/service#/Relationship%20Tuples/Read
//
// See: CreateTupleKey for creating keys.
//
// You may read via pagination utilising the token returned from the request.
func (o *OFGAClient) ReadRelatedObjects(ctx context.Context, key openfga.TupleKey, pageSize int32, paginationToken string) (*ReadResponse, error) {
	keys := []openfga.TupleKey{}
	res, err := o.getRelatedObjects(ctx, key, pageSize, paginationToken)
	if err != nil {
		return nil, err
	}
	tupes, ok := res.GetTuplesOk()
	if ok {
		t := *tupes
		for i := 0; i < len(t); i++ {
			key, ok := t[0].GetKeyOk()
			if ok {
				keys = append(keys, *key)
			}
		}
	}

	token := ""
	t, ok := res.GetContinuationTokenOk()
	if ok {
		token = *t
	}

	return &ReadResponse{Keys: keys, PaginationToken: token}, nil
}

// CheckRelation verifies that a user (or object) is allowed to access the target object by the specified relation.
//
// It will return:
// - A bool of simply true or false, denoting authorisation
// - A string (if trace is true) explaining WHY this is true [in the case the check succeeds, otherwise an empty string]
// - An error in the event something is wrong when contacting OpenFGA
func (o *OFGAClient) CheckRelation(ctx context.Context, key openfga.TupleKey, trace bool) (bool, string, error) {
	return o.checkRelation(ctx, key, trace)
}
