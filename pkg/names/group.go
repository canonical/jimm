package names

import (
	"fmt"
	"regexp"
)

const (
	GroupTagKind = "group"
)

var (
	validGroupNameSnippet = "[a-zA-Z0-9][a-zA-Z0-9.+-]*[a-zA-Z0-9]"
	validGroupName        = regexp.MustCompile("^" + validGroupNameSnippet + "$")
)

// GroupTag represents a group.
// Implement juju names.Tag
type GroupTag struct {
	name string
}

func (t GroupTag) Id() string     { return t.name }
func (t GroupTag) Kind() string   { return GroupTagKind }
func (t GroupTag) String() string { return GroupTagKind + "-" + t.Id() }

func NewGroupTag(groupName string) GroupTag {
	parts := validGroupName.FindStringSubmatch(groupName)
	if len(parts) != 1 {
		panic(fmt.Sprintf("invalid group tag %q", groupName))
	}

	return GroupTag{name: parts[0]}
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

func IsValidGroup(id string) bool {
	return validGroupName.MatchString(id)
}
