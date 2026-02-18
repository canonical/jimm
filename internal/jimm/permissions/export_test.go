// Copyright 2025 Canonical.

package permissions

import (
	"context"

	ofganames "github.com/canonical/jimm/v3/internal/openfga/names"
)

var (
	ResolveTag                     = resolveTag
	DetermineAccessLevelAfterGrant = determineAccessLevelAfterGrant
	BatchSizeOpenfga               = BATCH_SIZE_OPENFGA
)

func (j *PermissionManager) ParseAndValidateTag(ctx context.Context, key string) (*ofganames.Tag, error) {
	return j.parseAndValidateTag(ctx, key)
}
