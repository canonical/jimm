package utils_test

import (
	"testing"

	qt "github.com/frankban/quicktest"

	"github.com/canonical/jimm/internal/common/pagination"
	"github.com/canonical/jimm/internal/rebac_admin/utils"
	"github.com/canonical/rebac-admin-ui-handlers/v1/resources"
)

func TestNewConversationID(t *testing.T) {
	c := qt.New(t)
	page, pag := utils.CreatePagination(nil)
	defPag := pagination.NewOffsetFilter(-1, 0)
	c.Assert(page, qt.Equals, 0)
	c.Assert(pag.Limit(), qt.Equals, defPag.Limit())
	c.Assert(pag.Offset(), qt.Equals, defPag.Offset())

	pagSize := 100
	pageNum := 1

	page, pag = utils.CreatePagination(&resources.GetIdentitiesParams{
		Size: &pagSize,
		Page: &pageNum,
	})

	c.Assert(page, qt.Equals, pageNum)
	c.Assert(pag.Limit(), qt.Equals, 100)
	c.Assert(pag.Offset(), qt.Equals, 100)

	pagSize = 100
	pageNum = 5

	page, pag = utils.CreatePagination(&resources.GetIdentitiesParams{
		Size: &pagSize,
		Page: &pageNum,
	})

	c.Assert(page, qt.Equals, pageNum)
	c.Assert(pag.Limit(), qt.Equals, 100)
	c.Assert(pag.Offset(), qt.Equals, 500)
}
