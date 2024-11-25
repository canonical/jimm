// Copyright 2024 Canonical.

package names

import (
	"github.com/canonical/jimm/v3/pkg/names"
)

// WithMemberRelation is a convenience function for group tags to return the tag's string
// with a member relation, commonly used when assigning group relations.
func WithMemberRelation(groupTag names.GroupTag) string {
	return groupTag.String() + "#" + MemberRelation.String()
}

// WithAssigneeRelation is a convenience function for role tags to return the tag's string
// with an assignee relation, commonly used when assigning role relations.
func WithAssigneeRelation(groupTag names.RoleTag) string {
	return groupTag.String() + "#" + AssigneeRelation.String()
}
