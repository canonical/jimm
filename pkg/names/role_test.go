// Copyright 2024 Canonical.

package names_test

import (
	"fmt"
	"testing"

	qt "github.com/frankban/quicktest"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"github.com/canonical/jimm/v3/pkg/names"
)

func TestParseRoleTag(t *testing.T) {
	c := qt.New(t)
	uuid := uuid.NewString()

	tests := []struct {
		tag           string
		expectedError string
		expectedTag   string
		expectedId    string
	}{{
		tag:         fmt.Sprintf("role-%s", uuid),
		expectedId:  uuid,
		expectedTag: fmt.Sprintf("role-%s", uuid),
	}, {
		tag:         fmt.Sprintf("role-%s#member", uuid),
		expectedId:  fmt.Sprintf("%s#member", uuid),
		expectedTag: fmt.Sprintf("role-%s#member", uuid),
	}, {
		tag:           "pokemon-diglett",
		expectedError: "\"pokemon-diglett\" is not a valid tag",
	}}

	for i, test := range tests {
		test := test
		c.Run(fmt.Sprintf("test case %d", i), func(c *qt.C) {
			gt, err := names.ParseRoleTag(test.tag)
			if test.expectedError == "" {
				c.Assert(err, qt.IsNil)
				c.Assert(gt.Id(), qt.Equals, test.expectedId)
				c.Assert(gt.Kind(), qt.Equals, "role")
				c.Assert(gt.String(), qt.Equals, test.expectedTag)
			} else {
				c.Assert(err, qt.ErrorMatches, test.expectedError)
			}
		})
	}
}

func TestParseRoleTagDeniesBadKinds(t *testing.T) {
	_, err := names.ParseRoleTag("pokemon-diglett")
	assert.Error(t, err)
	assert.ErrorContains(t, err, "\"pokemon-diglett\" is not a valid tag")
}

func TestIsValidRoleId(t *testing.T) {
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
		assert.Equal(t, names.IsValidRoleId(test.id), test.expectedValid)
	}
}

func TestIsValidRoleName(t *testing.T) {
	tests := []struct {
		name             string
		expectedValidity bool
	}{{
		name:             "role-1",
		expectedValidity: true,
	}, {
		name:             "Role1",
		expectedValidity: true,
	}, {
		name:             "1role",
		expectedValidity: false,
	}, {
		name:             ".role",
		expectedValidity: false,
	}, {
		name:             "role.A",
		expectedValidity: true,
	}, {
		name:             "role.A1",
		expectedValidity: true,
	}, {
		name:             "role_test_a_1",
		expectedValidity: true,
	}, {
		name:             "role+a",
		expectedValidity: false,
	}, {
		name:             "Test.Role.1.A",
		expectedValidity: true,
	}, {
		name:             "",
		expectedValidity: false,
	}, {
		name:             "no",
		expectedValidity: false,
	}, {
		name:             "foo",
		expectedValidity: true,
	}, {
		name:             "short",
		expectedValidity: true,
	}, {
		name:             "short1",
		expectedValidity: true,
	}, {
		name:             "short_",
		expectedValidity: false,
	}, {
		name:             "role.A#member",
		expectedValidity: false,
	}}

	for _, test := range tests {
		t.Logf("testing role name %q, expected validity %v", test.name, test.expectedValidity)

		valid := names.IsValidRoleName(test.name)
		assert.Equal(t, valid, test.expectedValidity)
	}
}
