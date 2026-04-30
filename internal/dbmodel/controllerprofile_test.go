// Copyright 2026 Canonical.

package dbmodel_test

import (
	"testing"

	qt "github.com/frankban/quicktest"

	"github.com/canonical/jimm/v3/internal/dbmodel"
)

func TestPartialJujuVersionPrefixes(t *testing.T) {
	c := qt.New(t)

	for _, tc := range []struct {
		about   string
		version string
		want    []string
	}{
		{
			about:   "empty version",
			version: "",
			want:    nil,
		},
		{
			about:   "major only",
			version: "3",
			want:    []string{"3"},
		},
		{
			about:   "major minor",
			version: "3.6",
			want:    []string{"3", "3.6"},
		},
		{
			about:   "major minor patch",
			version: "3.6.4",
			want:    []string{"3", "3.6", "3.6.4"},
		},
	} {
		c.Run(tc.about, func(c *qt.C) {
			c.Assert(dbmodel.PartialJujuVersionPrefixes(tc.version), qt.DeepEquals, tc.want)
		})
	}
}
