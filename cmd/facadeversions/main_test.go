// Copyright 2026 Canonical.

package main

import (
	"testing"

	qt "github.com/frankban/quicktest"
	"github.com/juju/juju/core/facades"
)

func TestNormalizeSortsAndDedups(t *testing.T) {
	c := qt.New(t)

	in := map[string][]int{
		"A": {2, 1, 2, 3, 1},
		"B": {},
	}
	got := normalize(in)
	want := map[string][]int{
		"A": {1, 2, 3},
		"B": nil,
	}
	c.Assert(got, qt.DeepEquals, want)
}

func TestIntSliceDiff(t *testing.T) {
	c := qt.New(t)

	missing, extra := intSliceDiff([]int{1, 2, 4}, []int{2, 3, 4})
	c.Assert(missing, qt.DeepEquals, []int{1})
	c.Assert(extra, qt.DeepEquals, []int{3})
}

func TestDiff(t *testing.T) {
	c := qt.New(t)

	juju := normalize(map[string][]int{
		"A": {1, 2},
		"B": {1},
	})
	jimm := normalize(map[string][]int{
		"A": {2},
		"C": {1},
	})

	got := diff(juju, jimm)
	want := diffResult{
		OnlyInJuju:   []string{"B"},
		OnlyInJimm:   []string{"C"},
		VersionDiffs: []string{"A: missing [1]"},
	}
	c.Assert(got, qt.DeepEquals, want)
}

func TestVersionLag(t *testing.T) {
	c := qt.New(t)

	juju := normalize(map[string][]int{
		"A":        {1, 3, 2},
		"B":        {1, 2},
		"OnlyJuju": {9},
		"Empty":    {},
	})
	jimm := normalize(map[string][]int{
		"A":        {1},
		"B":        {1},
		"OnlyJimm": {9},
		"Empty":    {},
	})

	got := versionLag(juju, jimm)
	want := []string{
		"A: jimm=1 juju=3",
		"B: jimm=1 juju=2",
	}
	c.Assert(got, qt.DeepEquals, want)
}

func TestConvertJujuFacadeVersionsCopiesSlices(t *testing.T) {
	c := qt.New(t)

	in := facades.FacadeVersions{
		"X": facades.FacadeVersion{1, 2},
	}
	out := convertJujuFacadeVersions(in)

	out["X"][0] = 99
	c.Assert(in["X"][0], qt.Equals, 1)
}
