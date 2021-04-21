// Copyright 2020 Canonical Ltd.

// Package errors contains types to help handle errors in the system.
package errors

import (
	"fmt"

	jujuparams "github.com/juju/juju/apiserver/params"
	"go.uber.org/zap"

	"github.com/CanonicalLtd/jimm/internal/zapctx"
)

// An Error is an error in the JIMM system.
type Error struct {
	// Op is the operation that errored.
	Op Op

	// Code is a code attached to the error.
	Code Code

	// Message is a human-readable error description.
	Message string

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

// E constructs errors for use throughout the JIMM application. An error
// is constructed by processing the given arguments. The meaning of the
// arguments is as follows:
//
//     errors.Op   - string representation of the operation being
//                   performed.
//     errors.Code - string code classifying the error.
//     error       - underlying error that caused the new error.
//     string      - A human readable message describing the error.
//
// E will panic if no arguments are provided.
func E(args ...interface{}) error {
	if len(args) == 0 {
		panic("call to errors.E with no arguments")
	}
	var setCode bool
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
		default:
			zapctx.Default.DPanic("unknown type passed to errors.E", zap.String("type", fmt.Sprintf("%T", arg)), zap.Any("value", arg))
			return fmt.Errorf("unknown type (%T) passed to errors.E", arg)
		}
	}
	if setCode {
		return &e
	}
	// the caller didn't explicitely set the code for this error, attempt
	// to copy the code from the wrapped error. The interface used to
	// extract error codes is compatible with both the Error type and juju
	// API Error types.
	if ec, ok := e.Err.(interface{ ErrorCode() string }); ok {
		e.Code = Code(ec.ErrorCode())
	}
	return &e
}

// An Op describes the operation being performed that caused the error.
type Op string

// A Code is a code which describes the class of error. Where possible
// these codes are identical to the codes returned in the juju API.
type Code string

const (
	CodeAlreadyExists       Code = jujuparams.CodeAlreadyExists
	CodeBadRequest          Code = jujuparams.CodeBadRequest
	CodeConnectionFailed    Code = "connection failed"
	CodeDatabaseLocked      Code = "database locked"
	CodeIncompatibleClouds  Code = jujuparams.CodeIncompatibleClouds
	CodeNotFound            Code = jujuparams.CodeNotFound
	CodeNotImplemented      Code = jujuparams.CodeNotImplemented
	CodeNotSupported        Code = jujuparams.CodeNotSupported
	CodeServerConfiguration Code = "server configuration"
	CodeUnauthorized        Code = jujuparams.CodeUnauthorized
	CodeUpgradeInProgress   Code = jujuparams.CodeUpgradeInProgress
)

// ErrorCode returns the error code from the given error.
func ErrorCode(err error) Code {
	e, ok := err.(*Error)
	if !ok {
		return ""
	}
	return e.Code
}
