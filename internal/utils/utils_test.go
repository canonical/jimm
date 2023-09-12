package utils_test

import (
	"testing"

	qt "github.com/frankban/quicktest"

	"github.com/canonical/jimm/internal/utils"
)

func TestNewConversationID(t *testing.T) {
	c := qt.New(t)
	res := utils.NewConversationID()
	c.Assert(res, qt.HasLen, 16)
}
