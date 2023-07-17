// Copyright 2020 Canonical Ltd.

package db

import (
	dqlitedriver "github.com/canonical/go-dqlite/driver"
	"github.com/jackc/pgconn"
	sqlite3 "github.com/mattn/go-sqlite3"
	"gorm.io/gorm"

	"github.com/canonical/jimm/internal/errors"
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
	switch e := err.(type) {
	case sqlite3.Error:
		if e.ExtendedCode == sqlite3.ErrConstraintUnique {
			code = errors.CodeAlreadyExists
		}
		if e.Code == sqlite3.ErrLocked {
			code = errors.CodeDatabaseLocked
		}
	case dqlitedriver.Error:
		// TODO(mhilton) work out how to decode dqlite errors.
	case *pgconn.PgError:
		if e.Code == pgUniqueViolation {
			code = errors.CodeAlreadyExists
		}
	}

	return errors.E(code, err)
}
