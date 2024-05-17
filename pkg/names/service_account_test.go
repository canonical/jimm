// Copyright 2024 Canonical Ltd.

package names_test

import (
	"fmt"
	"testing"

	qt "github.com/frankban/quicktest"

	"github.com/canonical/jimm/pkg/names"
)

func TestParseServiceAccountID(t *testing.T) {
	c := qt.New(t)
	tests := []struct {
		about      string
		tag        string
		expectedID string
		err        string
	}{{
		about:      "Valid svc account tag",
		tag:        "serviceaccount-1e654457-a195-4a41-8360-929c7f455d43@serviceaccount",
		expectedID: "1e654457-a195-4a41-8360-929c7f455d43@serviceaccount",
	}, {
		about: "Invalid svc account tag (no domain)",
		tag:   "serviceaccount-1e654457-a195-4a41-8360-929c7f455d43",
		err:   ".*is not a valid serviceaccount tag",
	}, {
		about: "Invalid svc account tag (serviceaccounts)",
		tag:   "serviceaccounts-1e654457-a195-4a41-8360-929c7f455d43@serviceaccount",
		err:   ".*is not a valid tag",
	}, {
		about: "Invalid svc account tag (no prefix)",
		tag:   "1e654457-a195-4a41-8360-929c7f455d43@serviceaccount",
		err:   ".*is not a valid tag",
	}, {
		about: "Invalid svc account tag (missing ID)",
		tag:   "serviceaccounts-",
		err:   ".*is not a valid tag",
	}}
	for _, test := range tests {
		test := test
		c.Run(test.about, func(c *qt.C) {
			gt, err := names.ParseServiceAccountTag(test.tag)
			if test.err == "" {
				c.Assert(err, qt.IsNil)
				c.Assert(gt.Id(), qt.Equals, test.expectedID)
				c.Assert(gt.Kind(), qt.Equals, "serviceaccount")
				c.Assert(gt.String(), qt.Equals, test.tag)
			} else {
				c.Assert(err, qt.ErrorMatches, test.err)
			}
		})
	}
}

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
		test := test
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
		test := test
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
