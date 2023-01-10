package names

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseGroupTagAcceptsGroups(t *testing.T) {
	gt, err := ParseGroupTag("group-yellow")
	assert.NoError(t, err)
	assert.Equal(t, "yellow", gt.name)
	assert.Equal(t, "yellow", gt.Id())
	assert.Equal(t, "group", gt.Kind())
	assert.Equal(t, "group-yellow", gt.String())
}

func TestParseGroupTagAcceptsGroupsWithRelationSpecifier(t *testing.T) {
	gt, err := ParseGroupTag("group-yellow#member")
	assert.NoError(t, err)
	assert.Equal(t, "yellow#member", gt.name)
	assert.Equal(t, "yellow#member", gt.Id())
	assert.Equal(t, "group", gt.Kind())
	assert.Equal(t, "group-yellow#member", gt.String())
}

func TestParseGroupTagDeniesBadKinds(t *testing.T) {
	_, err := ParseGroupTag("pokemon-diglett")
	assert.Error(t, err)
	assert.ErrorContains(t, err, "\"pokemon-diglett\" is not a valid tag")
}
