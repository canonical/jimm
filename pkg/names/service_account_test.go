// Copyright 2025 Canonical.

package names_test

import (
	"fmt"
	"testing"

	qt "github.com/frankban/quicktest"

	"github.com/canonical/jimm/v3/pkg/names"
)

func TestIsValidServiceAccountId(t *testing.T) {
	c := qt.New(t)
	tests := []struct {
		id            string
		expectedValid bool
	}{{
		id:            "1e654457-a195-4a41-8360-929c7f455d43@serviceaccount",
		expectedValid: true,
	}, {
		id:            "12345@serviceaccount",
		expectedValid: true,
	}, {
		id:            "abc123@serviceaccount",
		expectedValid: true,
	}, {
		id:            "ABC123@serviceaccount",
		expectedValid: true,
	}, {
		id:            "ABC123@serviceaccount",
		expectedValid: true,
	}, {
		id:            "ABC123",
		expectedValid: false,
	}, {
		id:            "abc 123",
		expectedValid: false,
	}, {
		id:            "",
		expectedValid: false,
	}, {
		id:            "  ",
		expectedValid: false,
	}, {
		id:            "@",
		expectedValid: false,
	}, {
		id:            "@serviceaccount",
		expectedValid: false,
	}, {
		id:            "abc123@some-other-domain",
		expectedValid: false,
	}, {
		id:            "abc123@",
		expectedValid: false,
	}}
	for i, test := range tests {
		c.Run(fmt.Sprintf("test case %d", i), func(c *qt.C) {
			c.Assert(names.IsValidServiceAccountId(test.id), qt.Equals, test.expectedValid)
		})
	}

}

func TestEnsureValidClientIdWithDomain(t *testing.T) {
	c := qt.New(t)

	tests := []struct {
		name          string
		id            string
		expectedError bool
		expectedId    string
	}{{
		name:       "uuid, no domain",
		id:         "00000000-0000-0000-0000-000000000000",
		expectedId: "00000000-0000-0000-0000-000000000000@serviceaccount",
	}, {
		name:       "uuid, with domain",
		id:         "00000000-0000-0000-0000-000000000000@serviceaccount",
		expectedId: "00000000-0000-0000-0000-000000000000@serviceaccount",
	}, {
		name:          "empty",
		id:            "",
		expectedError: true,
	}, {
		name:          "empty id, with correct domain",
		id:            "@serviceaccount",
		expectedError: true,
	}, {
		name:          "uuid, with wrong domain",
		id:            "00000000-0000-0000-0000-000000000000@some-domain",
		expectedError: true,
	}, {
		name:          "invalid format",
		id:            "_123_",
		expectedError: true,
	},
	}

	for _, test := range tests {
		c.Run(test.name, func(c *qt.C) {
			result, err := names.EnsureValidServiceAccountId(test.id)
			if test.expectedError {
				c.Assert(err, qt.ErrorMatches, "invalid client ID")
				c.Assert(result, qt.Equals, "")
			} else {
				c.Assert(err, qt.IsNil)
				c.Assert(result, qt.Equals, test.expectedId)
			}
		})
	}
}
