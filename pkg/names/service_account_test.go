package names

import (
	"testing"

	qt "github.com/frankban/quicktest"
	"github.com/stretchr/testify/assert"
)

func TestParseServiceAccountID(t *testing.T) {
	tests := []struct {
		about      string
		tag        string
		expectedID string
		err        string
	}{{
		about:      "Valid svc account tag",
		tag:        "serviceaccount-1e654457-a195-4a41-8360-929c7f455d43@serviceaccount",
		expectedID: "1e654457-a195-4a41-8360-929c7f455d43@serviceaccount",
		err:        "",
	}, {
		about: "Invalid svc account tag (no domain)",
		tag:   "serviceaccount-1e654457-a195-4a41-8360-929c7f455d43",
		err:   "is not a valid serviceaccount tag",
	}, {
		about: "Invalid svc account tag (serviceaccounts)",
		tag:   "serviceaccounts-1e654457-a195-4a41-8360-929c7f455d43@serviceaccount",
		err:   "is not a valid tag",
	}, {
		about: "Invalid svc account tag (no prefix)",
		tag:   "1e654457-a195-4a41-8360-929c7f455d43@serviceaccount",
		err:   "is not a valid tag",
	}, {
		about: "Invalid svc account tag (missing ID)",
		tag:   "serviceaccounts-",
		err:   "is not a valid tag",
	}}
	for _, test := range tests {
		t.Run(test.about, func(t *testing.T) {
			gt, err := ParseServiceAccountTag(test.tag)
			if test.err == "" {
				assert.NoError(t, err)
				assert.Equal(t, test.expectedID, gt.id)
				assert.Equal(t, test.expectedID, gt.Id())
				assert.Equal(t, "serviceaccount", gt.Kind())
				assert.Equal(t, test.tag, gt.String())
			} else {
				assert.ErrorContains(t, err, test.err)
			}
		})
	}
}

func TestIsValidServiceAccountId(t *testing.T) {
	assert.True(t, IsValidServiceAccountId("1e654457-a195-4a41-8360-929c7f455d43@serviceaccount"))
	assert.True(t, IsValidServiceAccountId("12345@serviceaccount"))
	assert.True(t, IsValidServiceAccountId("abc123@serviceaccount"))
	assert.True(t, IsValidServiceAccountId("ABC123@serviceaccount"))
	assert.True(t, IsValidServiceAccountId("ABC123@serviceaccount"))
	assert.False(t, IsValidServiceAccountId("ABC123"))
	assert.False(t, IsValidServiceAccountId("abc 123"))
	assert.False(t, IsValidServiceAccountId(""))
	assert.False(t, IsValidServiceAccountId("  "))
	assert.False(t, IsValidServiceAccountId("@"))
	assert.False(t, IsValidServiceAccountId("@serviceaccount"))
	assert.False(t, IsValidServiceAccountId("abc123@some-other-domain"))
	assert.False(t, IsValidServiceAccountId("abc123@"))
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

	for _, t := range tests {
		tt := t
		c.Run(tt.name, func(c *qt.C) {
			result, err := EnsureValidServiceAccountId(tt.id)
			if tt.expectedError {
				c.Assert(err, qt.ErrorMatches, "invalid client ID")
				c.Assert(result, qt.Equals, "")
			} else {
				c.Assert(err, qt.IsNil)
				c.Assert(result, qt.Equals, tt.expectedId)
			}
		})
	}
}
