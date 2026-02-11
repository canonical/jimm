// Copyright 2026 Canonical.

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

// PermissionManager is a type alias to export PermissionManager for use in tests.
type PermissionManager = permissionManager

func (j *permissionManager) ParseAndValidateTag(ctx context.Context, key string) (*ofganames.Tag, error) {
	return j.parseAndValidateTag(ctx, key)
}
