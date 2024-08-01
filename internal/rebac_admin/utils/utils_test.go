package utils_test

import (
	"testing"

	qt "github.com/frankban/quicktest"

	"github.com/canonical/jimm/v3/internal/common/pagination"
	"github.com/canonical/jimm/v3/internal/rebac_admin/utils"
)

func TestCreatePaginationFilter(t *testing.T) {
	c := qt.New(t)
	page, pag := utils.CreatePaginationFilter(nil, nil)
	defPag := pagination.NewOffsetFilter(-1, 0)
	c.Assert(page, qt.Equals, 0)
	c.Assert(pag.Limit(), qt.Equals, defPag.Limit())
	c.Assert(pag.Offset(), qt.Equals, defPag.Offset())

	pageSize := 100
	pageNum := 1

	page, pag = utils.CreatePaginationFilter(&pageSize, &pageNum)

	c.Assert(page, qt.Equals, pageNum)
	c.Assert(pag.Limit(), qt.Equals, 100)
	c.Assert(pag.Offset(), qt.Equals, 100)

	pageSize = 100
	pageNum = 5

	page, pag = utils.CreatePaginationFilter(&pageSize, &pageNum)

	c.Assert(page, qt.Equals, pageNum)
	c.Assert(pag.Limit(), qt.Equals, 100)
	c.Assert(pag.Offset(), qt.Equals, 500)
}
