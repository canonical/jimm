package jujuapi

import (
	"context"

	apiparams "github.com/canonical/jimm/api/params"
	"github.com/canonical/jimm/internal/openfga"
)

// RelationService is an interface to collect methods of the JIMM interface who interact with OpenFGA relations
type RelationService interface {
	AddRelation(ctx context.Context, user *openfga.User, tuples []apiparams.RelationshipTuple) error
	RemoveRelation(ctx context.Context, user *openfga.User, tuples []apiparams.RelationshipTuple) error
	CheckRelation(ctx context.Context, user *openfga.User, tuple apiparams.RelationshipTuple, trace bool) (_ bool, err error)
	ListRelationshipTuples(ctx context.Context, user *openfga.User, tuple apiparams.RelationshipTuple, pageSize int32, continuationToken string) ([]openfga.Tuple, string, error)
}
