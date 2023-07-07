// Copyright 2023 CanonicalLtd.

package names

func NewTag(id, kind, relation string) *Tag {
	return &Tag{
		ID:       id,
		Relation: Relation(relation),
		Kind:     Kind(kind),
	}
}
