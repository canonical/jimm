// Copyright 2024 Canonical Ltd.

package common_test

import (
	"testing"

	"github.com/canonical/jimm/internal/jimm/common"
	qt "github.com/frankban/quicktest"
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
			wantLimit:  common.DefaultPageSize,
			wantOffset: 0,
		},
		{
			desc:       "Very large limit is reduced",
			limit:      2000,
			offset:     5,
			wantLimit:  common.DefaultPageSize,
			wantOffset: 5,
		},
	}
	c := qt.New(t)
	for _, tC := range testCases {
		c.Run(tC.desc, func(c *qt.C) {
			filter := common.NewOffsetFilter(tC.limit, tC.offset)
			c.Assert(filter.Limit(), qt.Equals, tC.wantLimit)
			c.Assert(filter.Offset(), qt.Equals, tC.wantOffset)
		})
	}
}
