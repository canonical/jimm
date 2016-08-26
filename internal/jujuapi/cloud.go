// Copyright 2016 Canonical Ltd.

package jujuapi

import (
	"sort"
	"strings"

	jujuparams "github.com/juju/juju/apiserver/params"

	"github.com/CanonicalLtd/jem/internal/mongodoc"
)

// makeCloud converts a cloud description in a mongodoc.Cloud into a jujuparams.Cloud.
func makeCloud(cloud mongodoc.Cloud) jujuparams.Cloud {
	sort.Strings(cloud.AuthTypes)
	return jujuparams.Cloud{
		Type:             cloud.ProviderType,
		AuthTypes:        cloud.AuthTypes,
		Endpoint:         cloud.Endpoint,
		IdentityEndpoint: cloud.IdentityEndpoint,
		StorageEndpoint:  cloud.StorageEndpoint,
		Regions:          makeRegions(cloud.Regions),
	}
}

// makeRegions creates a slice of jujuparams.Region in alphabetical order
// by name from a slice of mongodoc.Region.
func makeRegions(regs []mongodoc.Region) []jujuparams.CloudRegion {
	rs := make(regions, len(regs))
	for i, r := range regs {
		rs[i] = jujuparams.CloudRegion{
			Name:             r.Name,
			Endpoint:         r.Endpoint,
			IdentityEndpoint: r.IdentityEndpoint,
			StorageEndpoint:  r.StorageEndpoint,
		}
	}
	sort.Sort(rs)
	return []jujuparams.CloudRegion(rs)
}

type regions []jujuparams.CloudRegion

func (r regions) Len() int {
	return len(r)
}

func (r regions) Less(i, j int) bool {
	return r[i].Name < r[j].Name
}

func (r regions) Swap(i, j int) {
	r[i], r[j] = r[j], r[i]
}

func mergeClouds(c1, c2 jujuparams.Cloud) jujuparams.Cloud {
	c := c1
	c.AuthTypes = mergeStrings(c.AuthTypes, c2.AuthTypes)
	if c.Endpoint == "" {
		c.Endpoint = c2.Endpoint
	}
	if c.IdentityEndpoint == "" {
		c.Endpoint = c2.IdentityEndpoint
	}
	if c.StorageEndpoint == "" {
		c.Endpoint = c2.StorageEndpoint
	}
	c.Regions = mergeRegions(c.Regions, c2.Regions)
	return c
}

// merger is an interface that can be used to merge together two lists of
// sorted items.
type merger interface {
	// Len1 returns the number of elements in the first input list.
	Len1() int

	// Len2 returns the number of elements in the second input list.
	Len2() int

	// Cmp compares the the ith element from the first input list
	// with the jth element from the second input list and returns -1
	// if it is lower, 0 if they are equal or 1 if it is greater.
	Cmp(i, j int) int

	// Append1 appends the ith element of the first input list onto
	// the output.
	Append1(i int)

	// Append2 appends the ith element of the second input list onto
	// the output.
	Append2(i int)
}

// union merges the given two lists of sorted, uniqued items.
func union(m merger) {
	p1, p2 := 0, 0
	for p1 < m.Len1() && p2 < m.Len2() {
		switch m.Cmp(p1, p2) {
		case -1:
			m.Append1(p1)
			p1++
		case 0:
			m.Append1(p1)
			p1++
			p2++
		case 1:
			m.Append2(p2)
			p2++
		}
	}
	for p1 < m.Len1() {
		m.Append1(p1)
		p1++
	}
	for p2 < m.Len2() {
		m.Append2(p2)
		p2++
	}
}

// mergeStrings merges two sorted string slices and ensures that each
// string appears once in the resutl.
func mergeStrings(s1, s2 []string) []string {
	m := stringsMerger{
		in1: s1,
		in2: s2,
	}
	union(&m)
	return m.out
}

// stringsMerger is a merger that can merge sorted lists of strings.
type stringsMerger struct {
	in1, in2 []string
	out      []string
}

func (m *stringsMerger) Len1() int {
	return len(m.in1)
}

func (m *stringsMerger) Len2() int {
	return len(m.in2)
}

func (m *stringsMerger) Cmp(i, j int) int {
	return strings.Compare(m.in1[i], m.in2[j])
}

func (m *stringsMerger) Append1(i int) {
	m.out = append(m.out, m.in1[i])
}

func (m *stringsMerger) Append2(i int) {
	m.out = append(m.out, m.in2[i])
}

func mergeRegions(r1, r2 []jujuparams.CloudRegion) []jujuparams.CloudRegion {
	m := regionsMerger{
		in1: r1,
		in2: r2,
	}
	union(&m)
	return m.out
}

// regionsMerger is a merger that can merge sorted lists of
// jujuparams.CloudRegion.
type regionsMerger struct {
	in1, in2 []jujuparams.CloudRegion
	out      []jujuparams.CloudRegion
}

func (m *regionsMerger) Len1() int {
	return len(m.in1)
}

func (m *regionsMerger) Len2() int {
	return len(m.in2)
}

func (m *regionsMerger) Cmp(i, j int) int {
	return strings.Compare(m.in1[i].Name, m.in2[j].Name)
}

func (m *regionsMerger) Append1(i int) {
	m.out = append(m.out, m.in1[i])
}

func (m *regionsMerger) Append2(i int) {
	m.out = append(m.out, m.in2[i])
}
