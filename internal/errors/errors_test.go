// Copyright 2026 Canonical.

package errors_test

import (
	stderr "errors"
	"fmt"
	"testing"

	qt "github.com/frankban/quicktest"
	"github.com/juju/juju/rpc"

	"github.com/canonical/jimm/v3/internal/errors"
)

func TestCodefWrapsError(t *testing.T) {
	c := qt.New(t)

	code := errors.Code("test code")
	wrapped := errors.New("an error happened")
	err := errors.Codef(code, "%w", wrapped)
	c.Check(err, qt.ErrorMatches, `an error happened`)
	c.Check(errors.ErrorCode(err), qt.Equals, code)
	errValue, ok := err.(*errors.Error)
	c.Assert(ok, qt.IsTrue)
	c.Check(errValue.Code, qt.Equals, code)
	c.Assert(errValue.Err, qt.IsNotNil)
	c.Check(errValue.Err.Error(), qt.Equals, wrapped.Error())
	c.Check(stderr.Is(errValue.Err, wrapped), qt.IsTrue)
	c.Check(stderr.Is(err, wrapped), qt.IsTrue)
}

func TestCodefFormatsString(t *testing.T) {
	c := qt.New(t)

	code := errors.Code("test code")
	err := errors.Codef(code, "an error happened")
	c.Check(err, qt.ErrorMatches, `an error happened`)
	c.Check(errors.ErrorCode(err), qt.Equals, code)
	errValue, ok := err.(*errors.Error)
	c.Assert(ok, qt.IsTrue)
	c.Check(errValue.Code, qt.Equals, code)
	c.Assert(errValue.Err, qt.IsNotNil)
	c.Check(errValue.Err.Error(), qt.Equals, "an error happened")
	c.Check(errValue.Message, qt.Equals, "")

	err = errors.Codef(code, "formatted %s", "message")
	c.Check(err, qt.ErrorMatches, `formatted message`)
	c.Check(errors.ErrorCode(err), qt.Equals, code)
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
