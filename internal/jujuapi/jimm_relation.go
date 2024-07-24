package jujuapi

import (
	"context"

	"github.com/canonical/jimm/internal/openfga"
)

type JIMMRelation interface {
	AddRelation(ctx context.Context, user *openfga.User, tuples []openfga.Tuple) error
	RemoveRelation(ctx context.Context, user *openfga.User, tuples []openfga.Tuple) error
	CheckRelation(ctx context.Context, user *openfga.User, tuple openfga.Tuple, trace bool) (_ bool, err error)
	ListRelationshipTuples(ctx context.Context, user *openfga.User, tuple openfga.Tuple, pageSize int32, continuationToken string) ([]openfga.Tuple, string, error)
}
