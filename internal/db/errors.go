// Copyright 2020 Canonical Ltd.

package db

import (
	sqlite3 "github.com/mattn/go-sqlite3"
	"gorm.io/gorm"

	"github.com/CanonicalLtd/jimm/internal/errors"
)

// errorCode determines the error code from the database error returned.
func errorCode(err error) errors.Code {
	switch err {
	case gorm.ErrRecordNotFound:
		return errors.CodeNotFound
	}
	if e, ok := err.(sqlite3.Error); ok {
		switch e.ExtendedCode {
		case sqlite3.ErrConstraintUnique:
			return errors.CodeAlreadyExists
		}
	}
	return ""
}
