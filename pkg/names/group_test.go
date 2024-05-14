package names

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestParseGroupTagAcceptsGroups(t *testing.T) {
	uuid := uuid.NewString()
	gt, err := ParseGroupTag(fmt.Sprintf("group-%s", uuid))
	assert.NoError(t, err)
	assert.Equal(t, uuid, gt.id)
	assert.Equal(t, uuid, gt.Id())
	assert.Equal(t, "group", gt.Kind())
	assert.Equal(t, fmt.Sprintf("group-%s", uuid), gt.String())
}

func TestParseGroupTagAcceptsGroupsWithRelationSpecifier(t *testing.T) {
	uuid := uuid.NewString()
	gt, err := ParseGroupTag(fmt.Sprintf("group-%s#member", uuid))
	assert.NoError(t, err)
	assert.Equal(t, fmt.Sprintf("%s#member", uuid), gt.id)
	assert.Equal(t, fmt.Sprintf("%s#member", uuid), gt.Id())
	assert.Equal(t, "group", gt.Kind())
	assert.Equal(t, fmt.Sprintf("group-%s#member", uuid), gt.String())
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

func TestIsValidGroupName(t *testing.T) {
	tests := []struct {
		name             string
		expectedValidity bool
	}{{
		name:             "group-1",
		expectedValidity: true,
	}, {
		name:             "Group1",
		expectedValidity: true,
	}, {
		name:             "1group",
		expectedValidity: false,
	}, {
		name:             ".group",
		expectedValidity: false,
	}, {
		name:             "group.A",
		expectedValidity: true,
	}, {
		name:             "group.A1",
		expectedValidity: true,
	}, {
		name:             "group_test_a_1",
		expectedValidity: true,
	}, {
		name:             "group+a",
		expectedValidity: false,
	}, {
		name:             "Test.Group.1.A",
		expectedValidity: true,
	}, {
		name:             "",
		expectedValidity: false,
	}, {
		name:             "short",
		expectedValidity: false,
	}, {
		name:             "short1",
		expectedValidity: true,
	}, {
		name:             "short_",
		expectedValidity: false,
	}, {
		name:             "group.A#member",
		expectedValidity: false,
	}}

	for _, test := range tests {
		t.Logf("testing group name %q, expected validity %v", test.name, test.expectedValidity)

		valid := IsValidGroupName(test.name)
		assert.Equal(t, valid, test.expectedValidity)
	}
}
