// Copyright 2023 Canonical Ltd.

// Package constants contains constants and enums used throughout JIMM.
package constants

// The constants below can be split out once we have more.

// Life values specify model life status
type Life int

// Enumerate all possible migration phases.
const (
	UNKNOWN Life = iota
	ALIVE
	DEAD
	DYING
	MIGRATING_INTERNAL
	MIGRATING_AWAY
)

var lifeNames = []string{
	"unknown",
	"alive",
	"dead",
	"dying",
	"migrating-internal",
	"migrating-way",
}

// String returns the name of an model life constant.
func (p Life) String() string {
	i := int(p)
	if i >= 0 && i < len(lifeNames) {
		return lifeNames[i]
	}
	return "unknown"
}

// Parselife converts a string model life name
// to its constant value.
func ParseLife(target string) (Life, bool) {
	for p, name := range lifeNames {
		if target == name {
			return Life(p), true
		}
	}
	return UNKNOWN, false
}
