package names

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseServiceAccountID(t *testing.T) {
	gt, err := ParseServiceAccountTag("serviceaccount-1e654457-a195-4a41-8360-929c7f455d43")
	assert.NoError(t, err)
	assert.Equal(t, "1e654457-a195-4a41-8360-929c7f455d43", gt.id)
	assert.Equal(t, "1e654457-a195-4a41-8360-929c7f455d43", gt.Id())
	assert.Equal(t, "serviceaccount", gt.Kind())
	assert.Equal(t, "serviceaccount-1e654457-a195-4a41-8360-929c7f455d43", gt.String())
}

func TestIsValidServiceAccountId(t *testing.T) {
	assert.True(t, IsValidServiceAccountId("1e654457-a195-4a41-8360-929c7f455d43"))
	assert.True(t, IsValidServiceAccountId("12345"))
	assert.True(t, IsValidServiceAccountId("abc123"))
	assert.True(t, IsValidServiceAccountId("ABC123"))
	assert.False(t, IsValidServiceAccountId("abc 123"))
	assert.False(t, IsValidServiceAccountId(""))
	assert.False(t, IsValidServiceAccountId("  "))
}
