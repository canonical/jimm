// Copyright 2025 Canonical.

// Package errors contains types to help handle errors in the system.
package errors

import (
	stderr "errors"
	"fmt"

	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"

	apiparams "github.com/canonical/jimm/v3/pkg/api/params"
)

// An Error is an error in the JIMM system.
type Error struct {
	// Op is the operation that errored.
	Op Op

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

// E constructs errors for use throughout the JIMM application. An error
// is constructed by processing the given arguments. The meaning of the
// arguments is as follows:
//
//	errors.Op   - string representation of the operation being
//	              performed.
//	errors.Code - string code classifying the error.
//	error       - underlying error that caused the new error.
//	string      - A human readable message describing the error.
//
// E will panic if no arguments are provided.
func E(args ...interface{}) error {
	if len(args) == 0 {
		panic("call to errors.E with no arguments")
	}
	var setCode bool
	var setInfo bool
	var e Error
	for _, arg := range args {
		switch v := arg.(type) {
		case Op:
			e.Op = v
		case Code:
			setCode = true
			e.Code = v
		case error:
			e.Err = v
		case string:
			e.Message = v
		case map[string]any:
			setInfo = true
			e.Info = v
		default:
			zapctx.Default.DPanic("unknown type passed to errors.E", zap.String("type", fmt.Sprintf("%T", arg)), zap.Any("value", arg))
			return fmt.Errorf("unknown type (%T) passed to errors.E", arg)
		}
	}
	if setCode {
		return &e
	}

	// If the caller didn't explicitly set the code/info for this error, attempt
	// to copy the code/info from the wrapped error. The interface used to
	// extract the details is compatible with both the Error type and juju
	// API Error types.
	if !setCode {
		if ec, ok := e.Err.(interface{ ErrorCode() string }); ok {
			e.Code = Code(ec.ErrorCode())
		}
	}
	if !setInfo {
		if ei, ok := e.Err.(interface{ ErrorInfo() map[string]any }); ok {
			e.Info = ei.ErrorInfo()
		}
	}
	return &e
}

// An Op describes the operation being performed that caused the error.
type Op string

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
	CodeFailedToParseTupleKey        Code = "failed to parse tuple"
	CodeFailedToResolveTupleResource Code = "failed resolve resource"
	CodeOpenFGARequestFailed         Code = "failed request to OpenFGA"
	CodeJWKSRetrievalFailed          Code = "jwks retrieval failure"
)

// ErrorCode returns the error code from the given error.
func ErrorCode(err error) Code {
	var errCode interface{ ErrorCode() string }
	if stderr.As(err, &errCode) {
		return Code(errCode.ErrorCode())
	}
	return ""
}

// ErrorInfo returns additional information about the error.
func ErrorInfo(err error) map[string]any {
	var errInfo interface{ ErrorInfo() map[string]any }
	if stderr.As(err, &errInfo) {
		return errInfo.ErrorInfo()
	}
	return nil
}
