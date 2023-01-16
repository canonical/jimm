package names

import (
	"fmt"
	"regexp"
)

const (
	GroupTagKind = "group"
)

var (
	validGroupIdSnippet = `^[1-9][0-9]*(#|\z)[a-z]*$`
	validGroupId        = regexp.MustCompile(validGroupIdSnippet)
)

// GroupTag represents a group.
// Implements juju names.Tag
type GroupTag struct {
	id string
}

// Id implements juju names.Tag
func (t GroupTag) Id() string { return t.id }

// Kind implements juju names.Tag
func (t GroupTag) Kind() string { return GroupTagKind }

// String implements juju names.Tag
func (t GroupTag) String() string { return GroupTagKind + "-" + t.Id() }

// NewGroupTag creates a valid GroupTag if it is possible to parse
// the provided tag.
func NewGroupTag(groupId string) GroupTag {
	id := validGroupId.FindString(groupId)

	if id == "" {
		panic(fmt.Sprintf("invalid group tag %q", groupId))
	}

	return GroupTag{id: id}
}

// ParseGroupTag parses a user group string.
func ParseGroupTag(tag string) (GroupTag, error) {
	t, err := ParseTag(tag)
	if err != nil {
		return GroupTag{}, err
	}
	gt, ok := t.(GroupTag)
	if !ok {
		return GroupTag{}, invalidTagError(tag, GroupTagKind)
	}
	return gt, nil
}

// IsValidGroupId verifies the id of the tag is valid according to a regex internally.
func IsValidGroupId(id string) bool {
	return validGroupId.MatchString(id)
}
