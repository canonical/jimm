// Copyright 2025 Canonical.

package errors_test

import (
	stderr "errors"
	"testing"

	qt "github.com/frankban/quicktest"

	"github.com/canonical/jimm/v3/internal/errors"
)

func TestEEmptyArguments(t *testing.T) {
	c := qt.New(t)

	c.Assert(func() {
		_ = errors.E()
	}, qt.PanicMatches, `call to errors.E with no arguments`)
}

func TestEUnknownType(t *testing.T) {
	c := qt.New(t)
	c.Check(errors.E(42), qt.ErrorMatches, `unknown type \(int\) passed to errors.E`)
}

func TestE(t *testing.T) {
	c := qt.New(t)

	code := errors.Code("test code")
	err := errors.E(errors.Op("test.op"), code, "an error happened")
	c.Check(err, qt.ErrorMatches, `an error happened`)
	c.Check(errors.ErrorCode(err), qt.Equals, code)

	err = errors.E(errors.Op("test.op2"), err)
	c.Check(err, qt.ErrorMatches, `an error happened`)
	c.Check(errors.ErrorCode(err), qt.Equals, code)
}

func TestWrappedErrors(t *testing.T) {
	c := qt.New(t)

	err := stderr.New("foo")

	code := errors.Code("test code")
	err = errors.E(errors.Op("test.op"), code, "reason", err)
	c.Check(err.Error(), qt.Equals, `reason: foo`)
	c.Check(errors.ErrorCode(err), qt.Equals, code)

	err = errors.E(errors.Op("test.op2"), err, "more info")
	c.Check(err.Error(), qt.Equals, `more info: reason: foo`)
	c.Check(errors.ErrorCode(err), qt.Equals, code)
}
