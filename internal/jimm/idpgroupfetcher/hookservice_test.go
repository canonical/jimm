// Copyright 2026 Canonical.

package idpgroupfetcher_test

import (
	"context"
	"testing"

	qt "github.com/frankban/quicktest"

	"github.com/canonical/jimm/v3/internal/jimm/idpgroupfetcher"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
)

// TestHookServiceFetchGroups tests the HookService fetcher against a
// hook-service instance (docker compose up -d hook-service).
func TestHookServiceFetchGroups(t *testing.T) {
	c := qt.New(t)

	fetcher, err := idpgroupfetcher.NewHookService(jimmtest.HookServiceAddress(), "dummy-token")
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

// TestNewNoOp verifies the factory returns the NoOp fetcher when no
// hook-service address is configured.
func TestNewNoOp(t *testing.T) {
	c := qt.New(t)

	fetcher, err := idpgroupfetcher.New(context.Background(), idpgroupfetcher.Params{})
	c.Assert(err, qt.IsNil)
	groups, err := fetcher.FetchGroups(context.Background(), "alice@canonical.com")
	c.Assert(err, qt.IsNil)
	c.Assert(groups, qt.HasLen, 0)
}
