// Package jemerror knows how to map from errors to
// HTTP error responses.
package jemerror

import (
	"context"
	"net/http"

	"github.com/juju/httprequest"
	"gopkg.in/errgo.v1"
	"gopkg.in/macaroon-bakery.v2-unstable/httpbakery"

	"github.com/CanonicalLtd/jem/internal/zapctx"
	"github.com/CanonicalLtd/jem/internal/zaputil"
	"github.com/CanonicalLtd/jem/params"
)

// Mapper holds an ErrorMapper that can
// translate from Go errors to standard JEM
// error responses.
var Mapper httprequest.ErrorMapper = errToResp

type errorCoder interface {
	ErrorCode() params.ErrorCode
}

func errToResp(err error) (int, interface{}) {
	zapctx.Info(context.TODO(), "HTTP error response", zaputil.Error(err))
	// Allow bakery errors to be returned as the bakery would
	// like them, so that httpbakery.Client.Do will work.
	if err, ok := errgo.Cause(err).(*httpbakery.Error); ok {
		return httpbakery.ErrorToResponse(err)
	}
	errorBody := errorResponseBody(err)
	status := http.StatusInternalServerError
	switch errorBody.Code {
	case params.ErrNotFound,
		params.ErrAmbiguousChoice:

		status = http.StatusNotFound
	case params.ErrForbidden,
		params.ErrStillAlive,
		params.ErrAlreadyExists:

		status = http.StatusForbidden
	case params.ErrBadRequest,
		httprequest.ErrUnmarshal:

		status = http.StatusBadRequest
	case params.ErrUnauthorized:
		status = http.StatusUnauthorized

	case params.ErrMethodNotAllowed:
		status = http.StatusMethodNotAllowed
	}
	return status, errorBody
}

// errorResponse returns an appropriate error response for the provided error.
func errorResponseBody(err error) *params.Error {
	errResp := &params.Error{
		Message: err.Error(),
	}
	cause := errgo.Cause(err)
	if coder, ok := cause.(errorCoder); ok {
		errResp.Code = coder.ErrorCode()
	} else if errgo.Cause(err) == httprequest.ErrUnmarshal {
		errResp.Code = params.ErrBadRequest
	}
	return errResp
}
