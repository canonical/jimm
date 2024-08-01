// Copyright 2024 Canonical Ltd.

package pagination_test

import (
	"testing"

	qt "github.com/frankban/quicktest"

	"github.com/canonical/jimm/v3/internal/common/pagination"
	"github.com/canonical/rebac-admin-ui-handlers/v1/resources"
)

func TestOffsetFilter(t *testing.T) {
	testCases := []struct {
		desc       string
		limit      int
		offset     int
		wantLimit  int
		wantOffset int
	}{
		{
			desc:       "Valid value are not changed",
			limit:      10,
			offset:     5,
			wantLimit:  10,
			wantOffset: 5,
		},
		{
			desc:       "Negative values are corrected",
			limit:      -1,
			offset:     -1,
			wantLimit:  pagination.DefaultPageSize,
			wantOffset: 0,
		},
		{
			desc:       "Very large limit is reduced",
			limit:      2000,
			offset:     5,
			wantLimit:  pagination.DefaultPageSize,
			wantOffset: 5,
		},
	}
	c := qt.New(t)
	for _, tC := range testCases {
		c.Run(tC.desc, func(c *qt.C) {
			filter := pagination.NewOffsetFilter(tC.limit, tC.offset)
			c.Assert(filter.Limit(), qt.Equals, tC.wantLimit)
			c.Assert(filter.Offset(), qt.Equals, tC.wantOffset)
		})
	}
}

func TestCreatePagination(t *testing.T) {
	c := qt.New(t)

	// test with default page size and tot
	page, nextPage, pag := pagination.CreatePagination(nil, 100)
	defPag := pagination.NewOffsetFilter(-1, 0)
	c.Assert(page, qt.Equals, 0)
	c.Assert(*nextPage, qt.Equals, 1)
	c.Assert(pag.Limit(), qt.Equals, defPag.Limit())
	c.Assert(pag.Offset(), qt.Equals, defPag.Offset())
	c.Assert(pag.Offset(), qt.Equals, defPag.Offset())

	// test with set page size
	pagSize := 100
	pageNum := 1
	page, nextPage, pag = pagination.CreatePagination(&resources.GetIdentitiesParams{
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
	page, nextPage, pag = pagination.CreatePagination(&resources.GetIdentitiesParams{
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
	page, nextPage, pag = pagination.CreatePagination(&resources.GetIdentitiesParams{
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
	page, nextPage, _ = pagination.CreatePagination(&resources.GetIdentitiesParams{
		Size: &pagSize,
		Page: &pageNum,
	}, 10)

	c.Assert(page, qt.Equals, pageNum)
	c.Assert(nextPage, qt.IsNil)
}
