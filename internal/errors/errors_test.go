// Copyright 2025 Canonical.

package errors_test

import (
	"fmt"
	"testing"

	qt "github.com/frankban/quicktest"
	"github.com/juju/juju/rpc"

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

func TestEWithInfo(t *testing.T) {
	c := qt.New(t)

	code := errors.Code("test code")
	info := map[string]any{"key": "value"}
	err := errors.E(errors.Op("test.op"), code, "an error happened", info)
	c.Check(err, qt.ErrorMatches, `an error happened`)
	c.Check(errors.ErrorCode(err), qt.Equals, code)
	c.Check(err.(*errors.Error).Info, qt.DeepEquals, info)
	c.Check(errors.ErrorInfo(err), qt.DeepEquals, info)

	err = errors.E("plain-error")
	c.Check(err, qt.ErrorMatches, `plain-error`)
	c.Check(errors.ErrorInfo(err), qt.DeepEquals, map[string]any(nil))
}

func TestErrorCodeWithJujuRPC(t *testing.T) {
	c := qt.New(t)

	err := rpc.RequestError{
		Code: "my-code",
		Info: map[string]any{"key": "value"},
	}
	c.Check(string(errors.ErrorCode(&err)), qt.Equals, "my-code")
	c.Check(errors.ErrorInfo(&err), qt.DeepEquals, map[string]any{"key": "value"})

	wrappedErr := fmt.Errorf("wrapped: %w", &err)
	c.Check(string(errors.ErrorCode(wrappedErr)), qt.Equals, "my-code")
	c.Check(errors.ErrorInfo(wrappedErr), qt.DeepEquals, map[string]any{"key": "value"})
}
