// Copyright 2024 Canonical Ltd.

package names_test

import (
	"fmt"
	"testing"

	qt "github.com/frankban/quicktest"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"github.com/canonical/jimm/pkg/names"
)

func TestParseGroupTag(t *testing.T) {
	c := qt.New(t)
	uuid := uuid.NewString()

	tests := []struct {
		tag           string
		expectedError string
		expectedTag   string
		expectedId    string
	}{{
		tag:         fmt.Sprintf("group-%s", uuid),
		expectedId:  uuid,
		expectedTag: fmt.Sprintf("group-%s", uuid),
	}, {
		tag:         fmt.Sprintf("group-%s#member", uuid),
		expectedId:  fmt.Sprintf("%s#member", uuid),
		expectedTag: fmt.Sprintf("group-%s#member", uuid),
	}, {
		tag:           "pokemon-diglett",
		expectedError: "\"pokemon-diglett\" is not a valid tag",
	}}

	for i, test := range tests {
		test := test
		c.Run(fmt.Sprintf("test case %d", i), func(c *qt.C) {
			gt, err := names.ParseGroupTag(test.tag)
			if test.expectedError == "" {
				c.Assert(err, qt.IsNil)
				c.Assert(gt.Id(), qt.Equals, test.expectedId)
				c.Assert(gt.Kind(), qt.Equals, "group")
				c.Assert(gt.String(), qt.Equals, test.expectedTag)
			} else {
				c.Assert(err, qt.ErrorMatches, test.expectedError)
			}
		})
	}
}

func TestParseGroupTagDeniesBadKinds(t *testing.T) {
	_, err := names.ParseGroupTag("pokemon-diglett")
	assert.Error(t, err)
	assert.ErrorContains(t, err, "\"pokemon-diglett\" is not a valid tag")
}

func TestIsValidGroupId(t *testing.T) {
	uuid := uuid.NewString()
	tests := []struct {
		id            string
		expectedValid bool
	}{{
		id:            uuid,
		expectedValid: true,
	}, {
		id:            fmt.Sprintf("%s#member", uuid),
		expectedValid: true,
	}, {
		id:            fmt.Sprintf("%s#member#member", uuid),
		expectedValid: false,
	}, {
		id:            fmt.Sprintf("%s#", uuid),
		expectedValid: false,
	}, {
		id:            "0#member",
		expectedValid: false,
	}, {
		id:            "0",
		expectedValid: false,
	}}
	for _, test := range tests {
		assert.Equal(t, names.IsValidGroupId(test.id), test.expectedValid)
	}
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

		valid := names.IsValidGroupName(test.name)
		assert.Equal(t, valid, test.expectedValidity)
	}
}
