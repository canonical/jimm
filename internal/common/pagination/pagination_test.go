// Copyright 2024 Canonical.

package pagination_test

import (
	"testing"

	qt "github.com/frankban/quicktest"

	"github.com/canonical/jimm/v3/internal/common/pagination"
	"github.com/canonical/jimm/v3/internal/common/utils"
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
			wantLimit:  pagination.MaxPageSize,
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

	testCases := []struct {
		desc         string
		size         *int
		page         *int
		total        int
		wantPage     int
		wantNextPage *int
		wantOffset   int
		wantLimit    int
	}{
		{
			desc:         "test with default values",
			size:         nil,
			page:         nil,
			wantPage:     0,
			wantNextPage: utils.IntToPointer(1),
			wantOffset:   0,
			wantLimit:    pagination.DefaultPageSize,
		},
		{
			desc:         "test with set page size",
			size:         utils.IntToPointer(100),
			page:         utils.IntToPointer(1),
			total:        1000,
			wantPage:     1,
			wantNextPage: utils.IntToPointer(2),
			wantOffset:   100,
			wantLimit:    100,
		},
		{
			desc:         "test with set page size number 2",
			size:         utils.IntToPointer(100),
			page:         utils.IntToPointer(5),
			total:        1000,
			wantPage:     5,
			wantNextPage: utils.IntToPointer(6),
			wantOffset:   500,
			wantLimit:    100,
		},
		{
			desc:         "test with last current page and nextPage not present",
			size:         utils.IntToPointer(10),
			page:         utils.IntToPointer(0),
			total:        10,
			wantPage:     0,
			wantNextPage: nil,
			wantOffset:   0,
			wantLimit:    10,
		},
		{
			desc:         "test with current page over the total",
			size:         utils.IntToPointer(10),
			page:         utils.IntToPointer(2),
			total:        10,
			wantPage:     2,
			wantNextPage: nil,
			wantOffset:   20,
			wantLimit:    10,
		},
	}

	for _, tC := range testCases {
		c.Run(tC.desc, func(c *qt.C) {
			page, nextPage, pag := pagination.CreatePagination(tC.size, tC.page, tC.total)
			c.Assert(page, qt.Equals, tC.wantPage)
			if tC.wantNextPage == nil {
				c.Assert(nextPage, qt.IsNil)
			} else {
				c.Assert(*nextPage, qt.Equals, *tC.wantNextPage)
			}
			c.Assert(pag.Limit(), qt.Equals, tC.wantLimit)
			c.Assert(pag.Offset(), qt.Equals, tC.wantOffset)
		})
	}
}

// test the requested size is 1 more than then page size.
func TestCreatePaginationWithoutTotal(t *testing.T) {
	c := qt.New(t)
	pPage := utils.IntToPointer(0)
	pSize := utils.IntToPointer(10)
	page, size, pag := pagination.CreatePaginationWithoutTotal(pSize, pPage)
	c.Assert(page, qt.Equals, 0)
	c.Assert(pag.Limit(), qt.Equals, 11)
	c.Assert(pag.Offset(), qt.Equals, 0)
	c.Assert(size, qt.Equals, 11)
}

func TestTokenFilter(t *testing.T) {
	testToken := "test-token"
	testCases := []struct {
		desc      string
		limit     int
		token     string
		wantLimit int
	}{
		{
			desc:      "Valid value are not changed",
			limit:     10,
			token:     testToken,
			wantLimit: 10,
		},
		{
			desc:      "Negative values are corrected",
			limit:     -1,
			token:     testToken,
			wantLimit: pagination.DefaultOpenFGAPageSize,
		},
		{
			desc:      "Very large limit is reduced",
			limit:     2000,
			token:     testToken,
			wantLimit: pagination.MaxOpenFGAPageSize,
		},
	}
	c := qt.New(t)
	for _, tC := range testCases {
		c.Run(tC.desc, func(c *qt.C) {
			filter := pagination.NewOpenFGAFilter(tC.limit, tC.token)
			c.Assert(filter.Limit(), qt.Equals, tC.wantLimit)
			c.Assert(filter.Token(), qt.Equals, testToken)
		})
	}
}
