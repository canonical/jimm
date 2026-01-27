// Copyright 2026 Canonical.

package main

import (
	"flag"
	"fmt"
	"os"
	"slices"
	"sort"
	"strings"

	"github.com/juju/juju/api"
	"github.com/juju/juju/core/facades"

	"github.com/canonical/jimm/v3/internal/jujuapi"
)

func compare(args []string) {
	fs := flag.NewFlagSet("compare", flag.ExitOnError)
	check := fs.Bool("check", false, "exit non-zero if any differences are found")
	errorOnVersionLag := fs.Bool("error-on-version-lag", false, "exit non-zero if JIMM's highest facade version lags behind Juju's")
	_ = fs.Parse(args)

	// Note we have confusing package names here: "api" is Juju's api package,
	// while "jujuapi" is JIMM's internal/jujuapi package.

	jimm := normalize(jujuapi.SupportedFacades())

	juju := normalize(convertJujuFacadeVersions(api.SupportedFacadeVersions()))

	if *errorOnVersionLag {
		lag := versionLag(juju, jimm)
		if len(lag) == 0 {
			return
		}
		for _, line := range lag {
			fmt.Println(line)
		}
		os.Exit(1)
	}

	d := diff(juju, jimm)
	printDiff(d)
	if *check && d.hasChanges() {
		os.Exit(1)
	}
}

// versionLag returns a sorted list of output lines for facades where the highest
// version supported by JIMM is lower than Juju's highest version for that facade.
func versionLag(juju, jimm map[string][]int) []string {
	common := make([]string, 0)
	for name := range juju {
		if _, ok := jimm[name]; ok {
			common = append(common, name)
		}
	}
	sort.Strings(common)

	var out []string
	for _, name := range common {
		jv := juju[name]
		iv := jimm[name]
		if len(jv) == 0 || len(iv) == 0 {
			continue
		}
		jujuMax := jv[len(jv)-1]
		jimmMax := iv[len(iv)-1]
		if jimmMax < jujuMax {
			out = append(out, fmt.Sprintf("%s: jimm=%d juju=%d", name, jimmMax, jujuMax))
		}
	}
	return out
}

func convertJujuFacadeVersions(in facades.FacadeVersions) map[string][]int {
	out := make(map[string][]int, len(in))
	for name, versions := range in {
		out[name] = append([]int(nil), versions...)
	}
	return out
}

type diffResult struct {
	OnlyInJuju   []string
	OnlyInJimm   []string
	VersionDiffs []string // already formatted lines
}

func (d diffResult) hasChanges() bool {
	return len(d.OnlyInJuju) > 0 || len(d.OnlyInJimm) > 0 || len(d.VersionDiffs) > 0
}

func diff(juju, jimm map[string][]int) diffResult {
	var r diffResult

	for name := range juju {
		if _, ok := jimm[name]; !ok {
			r.OnlyInJuju = append(r.OnlyInJuju, name)
		}
	}
	for name := range jimm {
		if _, ok := juju[name]; !ok {
			r.OnlyInJimm = append(r.OnlyInJimm, name)
		}
	}
	sort.Strings(r.OnlyInJuju)
	sort.Strings(r.OnlyInJimm)

	common := make([]string, 0)
	for name := range juju {
		if _, ok := jimm[name]; ok {
			common = append(common, name)
		}
	}
	sort.Strings(common)

	for _, name := range common {
		missing, extra := intSliceDiff(juju[name], jimm[name])
		if len(missing) == 0 && len(extra) == 0 {
			continue
		}
		line := name + ":"
		if len(missing) > 0 {
			line += " missing " + formatInts(missing)
		}
		if len(extra) > 0 {
			if len(missing) > 0 {
				line += ";"
			}
			line += " extra " + formatInts(extra)
		}
		r.VersionDiffs = append(r.VersionDiffs, line)
	}

	return r
}

func printDiff(d diffResult) {
	if !d.hasChanges() {
		fmt.Println("Juju and JIMM facade versions match (for shared facades).")
		return
	}

	if len(d.OnlyInJuju) > 0 {
		fmt.Println("Facades only in Juju:")
		for _, n := range d.OnlyInJuju {
			fmt.Println("  -", n)
		}
	}
	if len(d.OnlyInJimm) > 0 {
		fmt.Println("Facades only in JIMM:")
		for _, n := range d.OnlyInJimm {
			fmt.Println("  -", n)
		}
	}
	if len(d.VersionDiffs) > 0 {
		fmt.Println("Version differences (Juju vs JIMM):")
		for _, line := range d.VersionDiffs {
			fmt.Println("  -", line)
		}
	}
}

// normalise sorts and deduplicates the slices in the map.
func normalize(m map[string][]int) map[string][]int {
	out := make(map[string][]int, len(m))
	for k, vs := range m {
		vs2 := append([]int(nil), vs...)
		slices.Sort(vs2)
		vs2 = slices.Compact(vs2)
		out[k] = vs2
	}
	return out
}

// intSliceDiff returns (missingInJimm, extraInJimm) relative to juju.
func intSliceDiff(juju, jimm []int) (missing []int, extra []int) {
	jm := make(map[int]bool, len(juju))
	im := make(map[int]bool, len(jimm))
	for _, v := range juju {
		jm[v] = true
	}
	for _, v := range jimm {
		im[v] = true
	}
	for v := range jm {
		if !im[v] {
			missing = append(missing, v)
		}
	}
	for v := range im {
		if !jm[v] {
			extra = append(extra, v)
		}
	}
	sort.Ints(missing)
	sort.Ints(extra)
	return missing, extra
}

func formatInts(vs []int) string {
	parts := make([]string, len(vs))
	for i, v := range vs {
		parts[i] = fmt.Sprintf("%d", v)
	}
	return "[" + strings.Join(parts, ", ") + "]"
}
