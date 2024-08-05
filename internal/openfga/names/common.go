package names

// WithMemberRelation is a convenience function to return an ID with a member
// relation, commonly used with group types when they need a #member relation.
func WithMemberRelation(id string) string {
	return id + "#" + MemberRelation.String()
}
