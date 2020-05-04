// Copyright 2020 Canonical Ltd.

package apiconn

import (
	jujuparams "github.com/juju/juju/apiserver/params"
	"gopkg.in/errgo.v1"
)

// An APIError wraps an error that was returned by the Juju API.
type APIError struct {
	errgo.Err
}

// ErrorCode returns the result of jujuparams.ErrorCode on the underlying
// error.
func (e *APIError) ErrorCode() string {
	return jujuparams.ErrCode(e.Underlying_)
}

// ParamsError returns this error as a *jujuparams.Error. If the
// underlying error is already a jujuparams.Error then return that,
// otherwise create one from the underlying error.
func (e *APIError) ParamsError() *jujuparams.Error {
	if perr, ok := e.Underlying_.(*jujuparams.Error); ok {
		return perr
	}
	return &jujuparams.Error{
		Code:    jujuparams.ErrCode(e.Underlying_),
		Message: e.Underlying_.Error(),
	}
}

// IsAPIError returns whether the given error is an APIError.
func IsAPIError(err error) bool {
	_, ok := err.(*APIError)
	return ok
}

// newAPIError creates a new APIError wrapping the given error, which
// should have been returned from the Juju API client. If the given error
// represents a nil jujuparams.Error then the returned error will be nil.
func newAPIError(err error) error {
	if err1, ok := err.(*jujuparams.Error); ok && err1 == nil {
		return nil
	}
	apierr := &APIError{
		Err: errgo.Err{
			Message_:    "api error",
			Underlying_: err,
		},
	}
	apierr.SetLocation(1)
	return apierr
}
