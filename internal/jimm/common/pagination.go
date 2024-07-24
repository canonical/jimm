// Copyright 2024 Canonical Ltd.

// common holds structs and variables that are common to JIMM.
package common

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
