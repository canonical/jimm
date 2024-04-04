package names

import (
	"testing"

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
		tag:        "serviceaccount-1e654457-a195-4a41-8360-929c7f455d43@canonical.com",
		expectedID: "1e654457-a195-4a41-8360-929c7f455d43@canonical.com",
		err:        "",
	}, {
		about: "Invalid svc account tag (no domain)",
		tag:   "serviceaccount-1e654457-a195-4a41-8360-929c7f455d43",
		err:   "is not a valid serviceaccount tag",
	}, {
		about: "Invalid svc account tag (serviceaccounts)",
		tag:   "serviceaccounts-1e654457-a195-4a41-8360-929c7f455d43@canonical.com",
		err:   "is not a valid tag",
	}, {
		about: "Invalid svc account tag (no prefix)",
		tag:   "1e654457-a195-4a41-8360-929c7f455d43@canonical.com",
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
	assert.True(t, IsValidServiceAccountId("1e654457-a195-4a41-8360-929c7f455d43@canonical.com"))
	assert.True(t, IsValidServiceAccountId("12345@canonical.com"))
	assert.True(t, IsValidServiceAccountId("abc123@canonical.com"))
	assert.True(t, IsValidServiceAccountId("ABC123@canonical.com"))
	assert.True(t, IsValidServiceAccountId("ABC123@canonical.com"))
	assert.False(t, IsValidServiceAccountId("ABC123"))
	assert.False(t, IsValidServiceAccountId("abc 123"))
	assert.False(t, IsValidServiceAccountId(""))
	assert.False(t, IsValidServiceAccountId("  "))
}
