// Copyright 2023 canonical.

package openfga

import "context"

func (o *OFGAClient) RemoveTuples(ctx context.Context, tuple Tuple) error {
	return o.removeTuples(ctx, tuple)
}
