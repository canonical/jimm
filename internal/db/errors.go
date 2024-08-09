// Copyright 2024 Canonical.

package db

import (
	"github.com/jackc/pgconn"
	"gorm.io/gorm"

	"github.com/canonical/jimm/v3/internal/errors"
)

// postgresql error codes from
// https://www.postgresql.org/docs/11/errcodes-appendix.html.
const pgUniqueViolation = "23505"

// dbError translates an error returned from the database into the error
// form understood by the JIMM system.
func dbError(err error) error {
	code := errors.Code(errors.ErrorCode(err))

	if err == gorm.ErrRecordNotFound {
		code = errors.CodeNotFound
	}

	if e, ok := err.(*pgconn.PgError); ok {
		if e.Code == pgUniqueViolation {
			code = errors.CodeAlreadyExists
		}
	}

	return errors.E(code, err)
}
