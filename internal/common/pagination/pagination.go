// Copyright 2024 Canonical Ltd.

// pagination holds common pagination patterns.
package pagination

const (
	defaultOffsetFilterPageSize = 50
	maxOffsetFilterPageSize     = 200
	// OpenFGA has internal limits on its page size
	// See https://openfga.dev/docs/interacting/read-tuple-changes
	defaultOpenFGAPageSize = 50
	maxOpenFGAPageSize     = 100
)

type LimitOffsetPagination struct {
	limit  int
	offset int
}

// NewOffsetFilter creates a filter for limit/offset pagination.
// If limit or offset are out of bounds, defaults will be used instead.
func NewOffsetFilter(limit int, offset int) LimitOffsetPagination {
	if limit < 0 {
		limit = defaultOffsetFilterPageSize
	}
	if limit > maxOffsetFilterPageSize {
		limit = maxOffsetFilterPageSize
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

// CreatePagination returns the current page, the next page if exists, and the pagination.LimitOffsetPagination.
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
	if (page+1)*pageSize < total {
		nPage := page + 1
		nextPage = &nPage
	}
	return page, nextPage, NewOffsetFilter(pageSize, offset)
}

type OpenFGAPagination struct {
	limit int
	token string
}

// NewOpenFGAFilter creates a filter for token pagination.
func NewOpenFGAFilter(limit int, token string) OpenFGAPagination {
	if limit < 0 {
		limit = defaultOpenFGAPageSize
	}
	if limit > maxOpenFGAPageSize {
		limit = maxOpenFGAPageSize
	}
	return OpenFGAPagination{
		limit: limit,
		token: token,
	}
}

func (l OpenFGAPagination) Limit() int {
	return l.limit
}

func (l OpenFGAPagination) Token() string {
	return l.token
}
