// Copyright 2023 Canonical Ltd.

// Package constants contains constants and enums used throughout JIMM.
package constants

// The constants below can be split out once we have more.

// ModelLife values specify model life status
type ModelLife int

// Enumerate all possible migration phases.
const (
	UNKNOWN ModelLife = iota
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
	"migrating-away",
}

// String returns the name of an model life constant.
func (p ModelLife) String() string {
	i := int(p)
	if i >= 0 && i < len(lifeNames) {
		return lifeNames[i]
	}
	return "unknown"
}

// Parselife converts a string model life name
// to its constant value.
func ParseModelLife(target string) (ModelLife, bool) {
	for p, name := range lifeNames {
		if target == name {
			return ModelLife(p), true
		}
	}
	return UNKNOWN, false
}
