package names

import (
	"errors"
	"fmt"
	"strings"

	"github.com/juju/names/v4"
)

// validKinds holds the valid tags for JIMM
func validKinds(kind string) bool {
	switch kind {
	case GroupTagKind:
		return true
	case ApplicationOfferTagKind:
		return true
	}
	return false
}

// TagKind returns one of the *TagKind constants for the given tag, or
// an error if none matches.
func TagKind(tag string) (string, error) {
	i := strings.Index(tag, "-")
	if i <= 0 || !validKinds(tag[:i]) {
		return "", fmt.Errorf("%q is not a valid tag", tag)
	}
	return tag[:i], nil
}

// splitTag splits the tag based on its kind (i.e., group-yellow would become [group, yellow]) but we only care for the elements
// AFTER the kind
func splitTag(tag string) (string, string, error) {
	kind, err := TagKind(tag)
	if err != nil {
		return "", "", err
	}
	return kind, tag[len(kind)+1:], nil
}

// ParseTag parses a string representation into a Tag.
func ParseTag(tag string) (names.Tag, error) {
	kind, id, err := splitTag(tag)
	if err != nil {
		return nil, invalidTagError(tag, "")
	}

	switch kind {
	case GroupTagKind:
		if !IsValidGroupId(id) {
			return nil, invalidTagError(tag, kind)
		}
		return NewGroupTag(id), nil
	case ApplicationOfferTagKind:
		if !IsValidApplicationOfferTag(id) {
			return nil, invalidTagError(tag, kind)
		}
		return NewApplicationOfferTag(id), nil
	}
	return nil, errors.New("couldn't parse tag")
}

func invalidTagError(tag, kind string) error {
	if kind != "" {
		return fmt.Errorf("%q is not a valid %s tag", tag, kind)
	}
	return fmt.Errorf("%q is not a valid tag", tag)
}
