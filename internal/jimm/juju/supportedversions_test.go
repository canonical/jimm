// Copyright 2026 Canonical.

package juju_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"testing/synctest"
	"time"

	qt "github.com/frankban/quicktest"
	"github.com/google/go-github/v69/github"
	"github.com/juju/version/v2"
	"go.uber.org/mock/gomock"

	"github.com/canonical/jimm/v3/internal/jimm/juju"
	"github.com/canonical/jimm/v3/internal/jimm/juju/mocks"
)

// makeRelease builds a RepositoryRelease pointer suitable for use in tests.
func makeRelease(tag string, draft, prerelease bool, publishedAt time.Time, htmlURL string) *github.RepositoryRelease {
	return &github.RepositoryRelease{
		TagName:     new(tag),
		Draft:       new(draft),
		Prerelease:  new(prerelease),
		PublishedAt: &github.Timestamp{Time: publishedAt},
		HTMLURL:     new(htmlURL),
	}
}

// noNextPage returns a *github.Response indicating no further pages.
func noNextPage() *github.Response {
	return &github.Response{Response: &http.Response{}, NextPage: 0}
}

// TestSupportedVersions_HappyPath checks that stable releases are returned
// sorted descending, while drafts, prereleases, blacklisted versions, versions
// below the minimum, and invalid tags are all excluded.
func TestSupportedVersions_HappyPath(t *testing.T) {
	c := qt.New(t)
	ctrl := gomock.NewController(t)

	published := time.Date(2025, 1, 15, 0, 0, 0, 0, time.UTC)
	releases := []*github.RepositoryRelease{
		makeRelease("v3.6.7", false, false, published, "https://github.com/juju/juju/releases/tag/v3.6.7"),
		makeRelease("v3.6.6", false, false, published, "https://github.com/juju/juju/releases/tag/v3.6.6"),
		makeRelease("v3.6.5", false, false, published, "https://github.com/juju/juju/releases/tag/v3.6.5"),
		makeRelease("v3.6.4", false, false, published, "https://example.com"),        // below min — excluded
		makeRelease("v4.0.0", false, false, published, "https://example.com"),        // blacklisted — excluded
		makeRelease("v3.6.7", true, false, published, "https://example.com"),         // draft — excluded
		makeRelease("v3.6.7", false, true, published, "https://example.com"),         // prerelease — excluded
		makeRelease("v3.6.5-beta.1", false, false, published, "https://example.com"), // non-stable tag — excluded
		makeRelease("not-a-version", false, false, published, "https://example.com"), // unparseable — excluded
	}

	mockGH := mocks.NewMockGitHubClient(ctrl)
	mockGH.EXPECT().
		ListReleases(gomock.Any(), "juju", "juju", gomock.Any()).
		Return(releases, noNextPage(), nil)

	j := &juju.JujuManager{GitHubClient: mockGH}

	resp, err := j.SupportedVersions(context.Background(), nil)
	c.Assert(err, qt.IsNil)
	c.Assert(len(resp.Versions), qt.Equals, 3)
	// Results must be sorted descending.
	c.Assert(resp.Versions[0].Version, qt.Equals, "3.6.7")
	c.Assert(resp.Versions[1].Version, qt.Equals, "3.6.6")
	c.Assert(resp.Versions[2].Version, qt.Equals, "3.6.5")
	for _, v := range resp.Versions {
		c.Assert(v.Date, qt.Not(qt.Equals), "")
		c.Assert(v.LinkToRelease, qt.Not(qt.Equals), "")
	}
}

func TestSupportedVersions_MinVersion(t *testing.T) {
	c := qt.New(t)

	c.Run("filters to versions strictly greater than minVersion", func(c *qt.C) {
		ctrl := gomock.NewController(t)
		published := time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC)
		releases := []*github.RepositoryRelease{
			makeRelease("v3.6.8", false, false, published, "https://github.com/juju/juju/releases/tag/v3.6.8"),
			makeRelease("v3.6.7", false, false, published, "https://github.com/juju/juju/releases/tag/v3.6.7"),
			makeRelease("v3.6.6", false, false, published, "https://github.com/juju/juju/releases/tag/v3.6.6"),
		}
		mockGH := mocks.NewMockGitHubClient(ctrl)
		mockGH.EXPECT().
			ListReleases(gomock.Any(), "juju", "juju", gomock.Any()).
			Return(releases, noNextPage(), nil)

		minVersion := "3.6.7"
		j := &juju.JujuManager{GitHubClient: mockGH}
		resp, err := j.SupportedVersions(context.Background(), &minVersion)
		c.Assert(err, qt.IsNil)
		c.Assert(len(resp.Versions), qt.Equals, 1)
		parsed := version.MustParse(resp.Versions[0].Version)
		c.Assert(parsed.Compare(version.MustParse(minVersion)) > 0, qt.IsTrue)
	})

	t.Run("invalid min version returns error", func(t *testing.T) {
		minVersion := "not-a-version"
		j := &juju.JujuManager{}
		_, err := j.SupportedVersions(context.Background(), &minVersion)
		c.Assert(err, qt.ErrorMatches, `invalid min version.*`)
	})
}

// TestSupportedVersions_GitHubAPIError checks that errors from the GitHub API
// are propagated to the caller.
func TestSupportedVersions_GitHubAPIError(t *testing.T) {
	c := qt.New(t)
	ctrl := gomock.NewController(t)

	mockGH := mocks.NewMockGitHubClient(ctrl)
	mockGH.EXPECT().
		ListReleases(gomock.Any(), "juju", "juju", gomock.Any()).
		Return(nil, nil, fmt.Errorf("rate limit exceeded"))

	j := &juju.JujuManager{GitHubClient: mockGH}
	_, err := j.SupportedVersions(context.Background(), nil)
	c.Assert(err, qt.ErrorMatches, `.*rate limit exceeded.*`)
}

// TestSupportedVersions_Caching verifies that GitHub is only called once within
// the TTL window and again after the clock advances past it.
func TestSupportedVersions_Caching(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := qt.New(t)
		ctrl := gomock.NewController(t)

		published := time.Date(2025, 1, 15, 0, 0, 0, 0, time.UTC)
		releases := []*github.RepositoryRelease{
			makeRelease("v3.6.7", false, false, published, "https://github.com/juju/juju/releases/tag/v3.6.7"),
		}

		mockGH := mocks.NewMockGitHubClient(ctrl)
		// Expect exactly two fetches: once on the cold start and once after TTL expiry.
		mockGH.EXPECT().
			ListReleases(gomock.Any(), "juju", "juju", gomock.Any()).
			Return(releases, noNextPage(), nil).
			Times(2)

		j := &juju.JujuManager{GitHubClient: mockGH}

		// First call: cold cache, hits GitHub.
		resp, err := j.SupportedVersions(context.Background(), nil)
		c.Assert(err, qt.IsNil)
		c.Assert(len(resp.Versions), qt.Equals, 1)

		// Second call within TTL: served from cache, no additional GitHub call.
		resp, err = j.SupportedVersions(context.Background(), nil)
		c.Assert(err, qt.IsNil)
		c.Assert(len(resp.Versions), qt.Equals, 1)

		// Advance the fake clock past the TTL.
		time.Sleep(juju.ReleasesCacheTTL + time.Second)

		// Third call: cache expired, hits GitHub again.
		resp, err = j.SupportedVersions(context.Background(), nil)
		c.Assert(err, qt.IsNil)
		c.Assert(len(resp.Versions), qt.Equals, 1)
	})
}
