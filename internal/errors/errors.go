// Copyright 2026 Canonical.

// Package errors contains types to help handle errors in the system.
package errors

import (
	stderr "errors"
	"fmt"

	jujuparams "github.com/juju/juju/rpc/params"

	apiparams "github.com/canonical/jimm/v3/pkg/api/params"
)

// An Error is an error in the JIMM system.
type Error struct {
	// Code is a code attached to the error.
	Code Code

	// Message is a human-readable error description.
	Message string

	// Info is additional information about the error
	// that clients need to use.
	Info map[string]any

	// Err contains the underlying error, if there is one.
	Err error
}

// Error implements the error interface.
func (e *Error) Error() string {
	if e.Message != "" {
		return e.Message
	}
	if e.Err != nil {
		return e.Err.Error()
	}
	if e.Code != "" {
		return string(e.Code)
	}
	return "unknown error"
}

// Unwrap implements the Unwrap method used by errors.Unwrap.
func (e *Error) Unwrap() error {
	return e.Err
}

// ErrorCode returns the value of this error's Code.
func (e *Error) ErrorCode() string {
	return string(e.Code)
}

// ErrorInfo returns the value of this error's Info.
func (e *Error) ErrorInfo() map[string]any {
	return e.Info
}

// New constructs a new error with the given message.
// It is a wrapper around the stdlib errors.New function.
func New(text string) error {
	return stderr.New(text)
}

// Codef constructs an error with a formatted message and an explicit code.
//
// The message is formatted using fmt.Errorf with the provided format and args.
// The code is attached to the error and can be retrieved using the ErrorCode function.
//
// If the format string includes a %w verb and the corresponding argument is an error,
// that error will be wrapped and can be retrieved using errors.Unwrap.
//
// To attach a code to an error without adding context use %w or %v as necessary, i.e.:
// `errors.Codef(code, "%w", err)`
// or
// `errors.Codef(code, "%v", err)`
func Codef(code Code, format string, args ...any) error {
	return &Error{
		Code: code,
		Err:  fmt.Errorf(format, args...),
	}
}

// A Code is a code which describes the class of error. Where possible
// these codes are identical to the codes returned in the juju API.
type Code string

const (
	CodeAlreadyExists                Code = jujuparams.CodeAlreadyExists
	CodeBadRequest                   Code = jujuparams.CodeBadRequest
	CodeCloudRegionRequired          Code = jujuparams.CodeCloudRegionRequired
	CodeConnectionFailed             Code = "connection failed"
	CodeDatabaseLocked               Code = "database locked"
	CodeForbidden                    Code = jujuparams.CodeForbidden
	CodeIncompatibleClouds           Code = jujuparams.CodeIncompatibleClouds
	CodeModelNotFound                Code = jujuparams.CodeModelNotFound
	CodeModelMigrating               Code = "model migrating"
	CodeNotFound                     Code = jujuparams.CodeNotFound
	CodeNotImplemented               Code = jujuparams.CodeNotImplemented
	CodeNotSupported                 Code = jujuparams.CodeNotSupported
	CodeRedirect                     Code = jujuparams.CodeRedirect
	CodeServerConfiguration          Code = "server configuration"
	CodeStillAlive                   Code = apiparams.CodeStillAlive
	CodeUnauthorized                 Code = jujuparams.CodeUnauthorized
	CodeServerError                  Code = "server error"
	CodeSessionTokenInvalid          Code = jujuparams.CodeSessionTokenInvalid
	CodeUpgradeInProgress            Code = jujuparams.CodeUpgradeInProgress
	CodeInProgress                   Code = "in progress"
	CodeFailedToParseTupleKey        Code = "failed to parse tuple"
	CodeFailedToResolveTupleResource Code = "failed resolve resource"
	CodeOpenFGARequestFailed         Code = "failed request to OpenFGA"
	CodeJWKSRetrievalFailed          Code = "jwks retrieval failure"
	CodeFatalLoginError              Code = "fatal login error"
)

// ErrorCode returns the error code from the given error.
// It unwraps the error chain to find the first error that implements
// the ErrorCode() string method that also has a non-empty code. If no
// such error is found, an empty Code is returned.
func ErrorCode(err error) Code {
	for err != nil {
		if v, ok := err.(interface{ ErrorCode() string }); ok {
			if code := v.ErrorCode(); code != "" {
				return Code(code)
			}
		}
		err = stderr.Unwrap(err)
	}
	return ""
}

// ErrorInfo returns additional information about the error.
// It unwraps the error chain to find the first error that implements
// the ErrorInfo() map[string]any method that also has non-nil info.
// If no such error is found, nil is returned.
func ErrorInfo(err error) map[string]any {
	for err != nil {
		if v, ok := err.(interface{ ErrorInfo() map[string]any }); ok {
			if info := v.ErrorInfo(); info != nil {
				return info
			}
		}
		err = stderr.Unwrap(err)
	}
	return nil
}
