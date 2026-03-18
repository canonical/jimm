// Copyright 2025 Canonical.

package errors_test

import (
	stderr "errors"
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
	err := errors.E(code, "an error happened")
	c.Check(err, qt.ErrorMatches, `an error happened`)
	c.Check(errors.ErrorCode(err), qt.Equals, code)

	err = errors.E(err)
	c.Check(err, qt.ErrorMatches, `an error happened`)
	c.Check(errors.ErrorCode(err), qt.Equals, code)
}

func TestEWithInfo(t *testing.T) {
	c := qt.New(t)

	code := errors.Code("test code")
	info := map[string]any{"key": "value"}
	err := errors.E(code, "an error happened", info)
	c.Check(err, qt.ErrorMatches, `an error happened`)
	c.Check(errors.ErrorCode(err), qt.Equals, code)
	c.Check(err.(errors.Error).Info, qt.DeepEquals, info)
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

func TestNew(t *testing.T) {
	c := qt.New(t)

	err := errors.New("test error")
	c.Check(err, qt.ErrorMatches, `test error`)
	c.Check(err.Message, qt.Equals, "test error")
}

func TestNewf(t *testing.T) {
	c := qt.New(t)

	baseErr := fmt.Errorf("base error")
	err := errors.Newf("wrapped error: %v", baseErr)
	c.Check(err.Error(), qt.Matches, `wrapped error: base error`)
	c.Check(err.Message, qt.Equals, "wrapped error: base error")
	c.Check(err.Unwrap(), qt.IsNil)
	c.Check(stderr.Is(err, baseErr), qt.IsFalse)

	err = errors.Newf("formatted %s", "message")
	c.Check(err.Error(), qt.Equals, "formatted message")
	c.Check(err.Message, qt.Equals, "formatted message")
	c.Check(err.Unwrap(), qt.IsNil)
}

func TestWrap(t *testing.T) {
	c := qt.New(t)

	baseErr := fmt.Errorf("base error")
	err := errors.Wrap(baseErr)
	c.Check(err.Error(), qt.Equals, "base error")
	c.Check(err.Unwrap(), qt.Equals, baseErr)
}

func TestWrapBheaviour(t *testing.T) {
	c := qt.New(t)

	info := map[string]any{"key": "value"}
	baseErr := errors.New("base error").WithCode(errors.CodeUnauthorized).WithInfo(info)

	err := errors.Wrap(baseErr)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeUnauthorized)
	c.Check(errors.ErrorInfo(err), qt.DeepEquals, info)

	err = errors.New("request failed").Wrap(baseErr)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeUnauthorized)
	c.Check(errors.ErrorInfo(err), qt.DeepEquals, info)

	override := map[string]any{"override": "value"}
	err = errors.New("request failed").WithCode(errors.CodeBadRequest).WithInfo(override).Wrap(baseErr)
	c.Check(err.Code, qt.Equals, errors.CodeBadRequest)
	c.Check(err.Info, qt.DeepEquals, override)
}

func TestWithCode(t *testing.T) {
	c := qt.New(t)

	err := errors.New("test error").WithCode(errors.CodeNotFound)
	c.Check(err.Error(), qt.Equals, "test error")
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeNotFound)
}

func TestWithInfo(t *testing.T) {
	c := qt.New(t)

	info := map[string]any{"key": "value"}
	err := errors.New("test error").WithInfo(info)
	c.Check(err.Error(), qt.Equals, "test error")
	c.Check(err.Info, qt.DeepEquals, info)
	c.Check(errors.ErrorInfo(err), qt.DeepEquals, info)
}

func TestMigrationPatterns(t *testing.T) {
	tests := []struct {
		name   string
		oldErr error
		newErr error
	}{
		{
			name:   "errors.E(err) -> err",
			oldErr: errors.E(stderr.New("base error")),
			newErr: stderr.New("base error"),
		},
		{
			name:   "errors.E(msg) -> fmt.Errorf(msg)",
			oldErr: errors.E("plain error"),
			newErr: fmt.Errorf("plain error"),
		},
		{
			name:   "errors.E(code) -> errors.New(\"\").WithCode(code)",
			oldErr: errors.E(errors.CodeNotFound),
			newErr: errors.New("").WithCode(errors.CodeNotFound),
		},
		{
			name:   "errors.E(code, msg) -> errors.New(msg).WithCode(code)",
			oldErr: errors.E(errors.CodeNotFound, "not found"),
			newErr: errors.New("not found").WithCode(errors.CodeNotFound),
		},
		{
			name:   "errors.E(err, msg) -> errors.New(msg).Wrap(err)",
			oldErr: errors.E(stderr.New("base error"), "request failed"),
			newErr: errors.New("request failed").Wrap(stderr.New("base error")),
		},
		{
			name:   "errors.E(err, code) -> errors.Wrap(err).WithCode(code)",
			oldErr: errors.E(stderr.New("base error"), errors.CodeBadRequest),
			newErr: errors.Wrap(stderr.New("base error")).WithCode(errors.CodeBadRequest),
		},
		{
			name:   "errors.E(code, msg, info) -> errors.New(msg).WithCode(code).WithInfo(info)",
			oldErr: errors.E(errors.CodeNotFound, "not found", map[string]any{"detail": "missing"}),
			newErr: errors.New("not found").WithCode(errors.CodeNotFound).WithInfo(map[string]any{"detail": "missing"}),
		},
		{
			name:   "errors.E(fmt.Errorf(\"msg: %w\", err)) -> fmt.Errorf(\"msg: %w\", err)",
			oldErr: errors.E(fmt.Errorf("request failed: %w", stderr.New("base error"))),
			newErr: fmt.Errorf("request failed: %w", stderr.New("base error")),
		},
		{
			name:   "errors.E(err, fmt.Sprintf(...)) -> errors.Newf(...).Wrap(err)",
			oldErr: errors.E(stderr.New("base error"), fmt.Sprintf("request failed %s", "later")),
			newErr: errors.Newf("request failed %s", "later").Wrap(stderr.New("base error")),
		},
		{
			name:   "wrapped code and info remain discoverable through the chain",
			oldErr: errors.E("request failed", errors.E("base error", errors.CodeUnauthorized, map[string]any{"scope": "admin"})),
			newErr: errors.New("request failed").Wrap(errors.New("base error").WithCode(errors.CodeUnauthorized).WithInfo(map[string]any{"scope": "admin"})),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			c := qt.New(t)
			c.Check(test.newErr.Error(), qt.Equals, test.oldErr.Error())
			c.Check(errors.ErrorCode(test.newErr), qt.Equals, errors.ErrorCode(test.oldErr))
			c.Check(errors.ErrorInfo(test.newErr), qt.DeepEquals, errors.ErrorInfo(test.oldErr))
		})
	}
}

func TestMigrationPreservesWrappedErrorMatching(t *testing.T) {
	c := qt.New(t)

	cause := stderr.New("base error")
	var oldErr = errors.E(cause, "request failed")
	var newErr error = errors.New("request failed").Wrap(cause)
	c.Check(stderr.Is(oldErr, cause), qt.IsTrue)
	c.Check(stderr.Is(newErr, cause), qt.IsTrue)

	cause = stderr.New("base error")
	oldErr = errors.E(cause, errors.CodeBadRequest)
	newErr = errors.Wrap(cause).WithCode(errors.CodeBadRequest)
	c.Check(stderr.Is(oldErr, cause), qt.IsTrue)
	c.Check(stderr.Is(newErr, cause), qt.IsTrue)

	cause = stderr.New("base error")
	oldErr = errors.E(fmt.Errorf("request failed: %w", cause))
	newErr = fmt.Errorf("request failed: %w", cause)
	c.Check(stderr.Is(oldErr, cause), qt.IsTrue)
	c.Check(stderr.Is(newErr, cause), qt.IsTrue)
}
