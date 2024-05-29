// Copyright 2023 canonical.

package openfga

import (
	"context"

	ofganames "github.com/canonical/jimm/internal/openfga/names"
)

// This is just for exporting the private function for testing. Since the private
// function is a generic one, we cannot use a simple file-scoped "var" statement.
func UnsetMultipleResourceAccesses[T ofganames.ResourceTagger](ctx context.Context, user *User, resource T, relations []Relation, pageSize int32) error {
	return unsetMultipleResourceAccesses(ctx, user, resource, relations, pageSize)
}

func (o *OFGAClient) RemoveTuples(ctx context.Context, tuple Tuple) error {
	return o.removeTuples(ctx, tuple)
}
