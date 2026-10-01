// Copyright 2026 Canonical.

package idpgroupfetcher

import (
	"context"
	"testing"

	qt "github.com/frankban/quicktest"
)

// TestHookServiceFetchGroups tests the HookService fetcher against a
// hook-service instance (docker compose up -d hook-service). Run with
// -short to skip.
func TestHookServiceFetchGroups(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	c := qt.New(t)

	fetcher, err := NewHookService(testHookServiceAddress, "dummy-token")
	c.Assert(err, qt.IsNil)

	ctx := context.Background()

	// Member of the "canonical" group (see local/hook-service/entrypoint.sh).
	groups, err := fetcher.FetchGroups(ctx, "jimm-group-user@canonical.com")
	c.Assert(err, qt.IsNil)
	c.Assert(groups, qt.DeepEquals, []string{"canonical"})

	groups, err = fetcher.FetchGroups(ctx, "nobody@canonical.com")
	c.Assert(err, qt.IsNil)
	c.Assert(groups, qt.HasLen, 0)

	// The @serviceaccount suffix is stripped before lookup.
	groups, err = fetcher.FetchGroups(ctx, "jimm-group-client@serviceaccount")
	c.Assert(err, qt.IsNil)
	c.Assert(groups, qt.DeepEquals, []string{"canonical"})
}
