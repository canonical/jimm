// Copyright 2026 Canonical.

package juju

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/google/go-github/v69/github"
	"github.com/juju/version/v2"

	"github.com/canonical/jimm/v3/pkg/api/params"
)

// GitHubClient is the interface used to list GitHub releases for a repository.
type GitHubClient interface {
	ListReleases(ctx context.Context, owner, repo string, opts *github.ListOptions) ([]*github.RepositoryRelease, *github.Response, error)
}

const minSupportedVersion = "3.6.5"

// blacklistedVersions lists specific releases that should be excluded even if they
// otherwise meet the stable release criteria.
var blacklistedVersions = []version.Number{
	version.MustParse("4.0.0"),
	version.MustParse("4.0.1"),
	version.MustParse("4.0.2"),
}

// SupportedVersions returns a list of supported Juju versions.
// When minVersion is non-nil, only versions strictly greater than minVersion are returned;
// versions equal to or below minVersion are excluded.
func (j *JujuManager) SupportedVersions(ctx context.Context, minVersion *string) (params.SupportedJujuVersionsResponse, error) {
	var parsedMinVersion *version.Number
	if minVersion != nil {
		v, err := version.Parse(*minVersion)
		if err != nil {
			return params.SupportedJujuVersionsResponse{}, fmt.Errorf("invalid min version %q: %w", *minVersion, err)
		}
		parsedMinVersion = &v
	}

	client := j.GitHubClient
	if client == nil {
		client = github.NewClient(nil).Repositories
	}

	releases, err := fetchReleasesFromGitHub(ctx, client, parsedMinVersion)
	if err != nil {
		return params.SupportedJujuVersionsResponse{}, err
	}
	return params.SupportedJujuVersionsResponse{Versions: releases}, nil
}

// fetchReleasesFromGitHub queries the GitHub API for juju/juju releases and returns
// a filtered, sorted slice of stable releases >= minSupportedVersion.
// If minVersion is non-nil, only releases strictly greater than it are included.
func fetchReleasesFromGitHub(ctx context.Context, client GitHubClient, minVersion *version.Number) ([]params.VersionElem, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	minV := version.MustParse(minSupportedVersion)

	type keyedRelease struct {
		v    version.Number
		elem params.VersionElem
	}

	var keyed []keyedRelease
	opts := &github.ListOptions{PerPage: 100, Page: 1}

	for {
		repoReleases, resp, err := client.ListReleases(ctx, "juju", "juju", opts)
		if err != nil {
			return nil, fmt.Errorf("fetching juju releases from GitHub: %w", err)
		}

		for _, release := range repoReleases {
			if release == nil || release.GetDraft() || release.GetPrerelease() {
				continue
			}
			v, ok := stableVersion(release.GetTagName())
			if !ok {
				continue
			}
			if v.Compare(minV) < 0 {
				continue
			}
			if slices.Contains(blacklistedVersions, v) {
				continue
			}
			if minVersion != nil && v.Compare(*minVersion) != 1 {
				continue
			}
			keyed = append(keyed, keyedRelease{
				v: v,
				elem: params.VersionElem{
					Version:       v.String(),
					Date:          release.GetPublishedAt().UTC().Format("2006-01-02"),
					LinkToRelease: release.GetHTMLURL(),
				},
			})
		}

		if resp == nil || resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	sort.Slice(keyed, func(i, j int) bool {
		return keyed[i].v.Compare(keyed[j].v) > 0
	})

	result := make([]params.VersionElem, len(keyed))
	for i, k := range keyed {
		result[i] = k.elem
	}
	return result, nil
}

// stableVersion parses a git tag and returns the version number if it represents
// a stable (non-pre-release) release.
func stableVersion(tag string) (version.Number, bool) {
	tag = strings.TrimPrefix(strings.TrimSpace(tag), "v")
	v, err := version.Parse(tag)
	if err != nil {
		return version.Number{}, false
	}
	if v.Tag != "" {
		return version.Number{}, false
	}
	return v, true
}
