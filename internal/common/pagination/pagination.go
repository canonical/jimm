// Copyright 2024 Canonical Ltd.

// pagination holds common pagination patterns.
package pagination

const (
	defaultPageSize = 50
	maxPageSize     = 200
)

type LimitOffsetPagination struct {
	limit  int
	offset int
}

// NewOffsetFilter creates a filter for limit/offset pagination.
// If limit or offset are out of bounds, defaults will be used instead.
func NewOffsetFilter(limit int, offset int) LimitOffsetPagination {
	if limit < 0 {
		limit = defaultPageSize
	}
	if limit > maxPageSize {
		limit = defaultPageSize
	}
	if offset < 0 {
		offset = 0
	}
	return LimitOffsetPagination{
		limit:  limit,
		offset: offset,
	}
}

func (l LimitOffsetPagination) Limit() int {
	return l.limit
}

func (l LimitOffsetPagination) Offset() int {
	return l.offset
}

// CreatePagination returns the current page, the next page if exists, and the pagination.LimitOffsetPagination from the *resources.GetIdentitiesParams, a .
func CreatePagination(sizeP, pageP *int, total int) (int, *int, LimitOffsetPagination) {
	pageSize := -1
	offset := 0
	page := 0
	var nextPage *int

	if sizeP != nil && pageP != nil {
		pageSize = *sizeP
		page = *pageP
		offset = pageSize * page
	}
	if (page+1)*pageSize >= total {
		nextPage = nil
	} else {
		nPage := page + 1
		nextPage = &nPage
	}
	return page, nextPage, NewOffsetFilter(pageSize, offset)
}
