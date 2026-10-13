// Copyright 2026 Canonical.

package idpgroupfetcher_test

import (
	"context"
	"testing"

	qt "github.com/frankban/quicktest"
	"golang.org/x/oauth2"

	"github.com/canonical/jimm/v3/internal/jimm/idpgroupfetcher"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
)

// TestHookServiceFetchGroups tests the HookService fetcher against a
// hook-service instance (docker compose up -d hook-service). Run with
// -short to skip.
func TestHookServiceFetchGroups(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	c := qt.New(t)
	ctx := context.Background()

	fetcher, err := idpgroupfetcher.NewHookService(jimmtest.HookServiceAddress(), jimmtest.HookServiceTokenSource(ctx))
	c.Assert(err, qt.IsNil)

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

// TestHookServiceFetchGroupsInvalidToken verifies the hook-service rejects
// a token that is not a JWT issued by the identity provider.
func TestHookServiceFetchGroupsInvalidToken(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	c := qt.New(t)

	fetcher, err := idpgroupfetcher.NewHookService(jimmtest.HookServiceAddress(), oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "invalid-token"}))
	c.Assert(err, qt.IsNil)

	_, err = fetcher.FetchGroups(context.Background(), "jimm-group-user@canonical.com")
	c.Assert(err, qt.ErrorMatches, `failed to fetch groups for user .*: .*Unauthenticated.*`)
}
