// Copyright 2026 Canonical.

package errors_test

import (
	"fmt"
	"testing"

	qt "github.com/frankban/quicktest"
	"github.com/juju/juju/rpc"

	"github.com/canonical/jimm/v3/internal/errors"
)

func TestErrWithCode(t *testing.T) {
	c := qt.New(t)

	code := errors.Code("test code")
	wrapped := errors.New("an error happened")
	err := errors.ErrWithCode(wrapped, code)
	c.Check(err, qt.ErrorMatches, `an error happened`)
	c.Check(errors.ErrorCode(err), qt.Equals, code)
	errValue, ok := err.(*errors.Error)
	c.Assert(ok, qt.IsTrue)
	c.Check(errValue.Code, qt.Equals, code)
	c.Check(errValue.Err, qt.Equals, wrapped)
}

func TestMsgWithCode(t *testing.T) {
	c := qt.New(t)

	code := errors.Code("test code")
	err := errors.MsgWithCode("an error happened", code)
	c.Check(err, qt.ErrorMatches, `an error happened`)
	c.Check(errors.ErrorCode(err), qt.Equals, code)
	errValue, ok := err.(*errors.Error)
	c.Assert(ok, qt.IsTrue)
	c.Check(errValue.Code, qt.Equals, code)
	c.Check(errValue.Message, qt.Equals, "an error happened")

	err = errors.New("plain-error")
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
