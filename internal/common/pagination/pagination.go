// Copyright 2024 Canonical.

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
func CreatePagination(sizeP, pageP *int, total int) (currentPage int, nextPage *int, _ LimitOffsetPagination) {
	pageSize := -1
	offset := 0

	if sizeP != nil && pageP != nil {
		pageSize = *sizeP
		currentPage = *pageP
		offset = pageSize * currentPage
	}
	if (currentPage+1)*pageSize < total {
		nPage := currentPage + 1
		nextPage = &nPage
	}
	return currentPage, nextPage, NewOffsetFilter(pageSize, offset)
}

// CreatePagination returns the current page, the expected page size, and the pagination.LimitOffsetPagination.
// This method is different approach to the method `CreatePagination` when we don't have the total number of records.
// We return the expectedPageSize, which is pageSize +1, so we fetch one record more from the db.
// We then check the resulting records are enough to advice the consumers to ask for one more page or not.
func CreatePaginationWithoutTotal(sizeP, pageP *int) (currentPage int, expectedPageSize int, _ LimitOffsetPagination) {
	pageSize := -1
	offset := 0

	if sizeP != nil && pageP != nil {
		pageSize = *sizeP
		currentPage = *pageP
		offset = pageSize * currentPage
	}
	expectedPageSize = pageSize + 1
	return currentPage, expectedPageSize, NewOffsetFilter(pageSize+1, offset)
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
