// Copyright 2023 CanonicalLtd.

package names

import cofga "github.com/canonical/ofga"

func NewTag(id, kind, relation string) *Tag {
	return &Tag{
		ID:       id,
		Relation: cofga.Relation(relation),
		Kind:     cofga.Kind(kind),
	}
}
