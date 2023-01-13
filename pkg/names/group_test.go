package names

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseGroupTagAcceptsGroups(t *testing.T) {
	gt, err := ParseGroupTag("group-1")
	assert.NoError(t, err)
	assert.Equal(t, "1", gt.id)
	assert.Equal(t, "1", gt.Id())
	assert.Equal(t, "group", gt.Kind())
	assert.Equal(t, "group-1", gt.String())
}

func TestParseGroupTagAcceptsGroupsWithRelationSpecifier(t *testing.T) {
	gt, err := ParseGroupTag("group-1#member")
	assert.NoError(t, err)
	assert.Equal(t, "1#member", gt.id)
	assert.Equal(t, "1#member", gt.Id())
	assert.Equal(t, "group", gt.Kind())
	assert.Equal(t, "group-1#member", gt.String())
}

func TestParseGroupTagDeniesBadKinds(t *testing.T) {
	_, err := ParseGroupTag("pokemon-diglett")
	assert.Error(t, err)
	assert.ErrorContains(t, err, "\"pokemon-diglett\" is not a valid tag")
}

func TestIsValidGroupId(t *testing.T) {
	r := regexp.MustCompile(`^[1-9][0-9]*(#|\z)[a-z]*$`)
	assert.False(t, r.MatchString("0#hi"))
	assert.False(t, r.MatchString("0"))
	assert.False(t, r.MatchString("a#hi"))
	assert.False(t, r.MatchString("01"))
	assert.True(t, r.MatchString("1"))
	assert.True(t, r.MatchString("1#"))
	assert.True(t, r.MatchString("1#hi"))

	s := r.FindString("1010")
	assert.Equal(t, "1010", s)

	s = r.FindString("01010")
	assert.Equal(t, "", s)

	s = r.FindString("01010#")
	assert.Equal(t, "", s)

	s = r.FindString("1010#member")
	assert.Equal(t, "1010#member", s)
}
