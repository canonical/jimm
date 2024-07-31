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

	// test with default page size and tot
	page, nextPage, pag := utils.CreatePagination(nil, 100)
	defPag := pagination.NewOffsetFilter(-1, 0)
	c.Assert(page, qt.Equals, 0)
	c.Assert(*nextPage, qt.Equals, 1)
	c.Assert(pag.Limit(), qt.Equals, defPag.Limit())
	c.Assert(pag.Offset(), qt.Equals, defPag.Offset())
	c.Assert(pag.Offset(), qt.Equals, defPag.Offset())

	// test with set page size
	pagSize := 100
	pageNum := 1
	page, nextPage, pag = utils.CreatePagination(&resources.GetIdentitiesParams{
		Size: &pagSize,
		Page: &pageNum,
	}, 1000)

	c.Assert(page, qt.Equals, pageNum)
	c.Assert(*nextPage, qt.Equals, pageNum+1)
	c.Assert(pag.Limit(), qt.Equals, 100)
	c.Assert(pag.Offset(), qt.Equals, 100)

	// test with set page size number 2
	pagSize = 100
	pageNum = 5
	page, nextPage, pag = utils.CreatePagination(&resources.GetIdentitiesParams{
		Size: &pagSize,
		Page: &pageNum,
	}, 1000)

	c.Assert(page, qt.Equals, pageNum)
	c.Assert(*nextPage, qt.Equals, pageNum+1)
	c.Assert(pag.Limit(), qt.Equals, 100)
	c.Assert(pag.Offset(), qt.Equals, 500)

	// test with last current page and nextPage not present
	pagSize = 10
	pageNum = 0
	page, nextPage, pag = utils.CreatePagination(&resources.GetIdentitiesParams{
		Size: &pagSize,
		Page: &pageNum,
	}, 10)

	c.Assert(page, qt.Equals, pageNum)
	c.Assert(nextPage, qt.IsNil)
	c.Assert(pag.Limit(), qt.Equals, 10)
	c.Assert(pag.Offset(), qt.Equals, 0)

	// test with current page over the total
	pagSize = 10
	pageNum = 2
	page, nextPage, _ = utils.CreatePagination(&resources.GetIdentitiesParams{
		Size: &pagSize,
		Page: &pageNum,
	}, 10)

	c.Assert(page, qt.Equals, pageNum)
	c.Assert(nextPage, qt.IsNil)
}
