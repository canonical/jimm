// Package jemerror knows how to map from errors to
// HTTP error responses.
package jemerror

import (
	"context"
	"net/http"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/httpbakery"
	"gopkg.in/errgo.v1"
	"gopkg.in/httprequest.v1"

	"github.com/CanonicalLtd/jimm/internal/zapctx"
	"github.com/CanonicalLtd/jimm/internal/zaputil"
	"github.com/CanonicalLtd/jimm/params"
)

// Mapper holds an ErrorMapper that can
// translate from Go errors to standard JEM
// error responses.
var Mapper = errToResp

type errorCoder interface {
	ErrorCode() params.ErrorCode
}

func errToResp(ctx context.Context, err error) (int, interface{}) {
	zapctx.Info(ctx, "HTTP error response", zaputil.Error(err))
	// Allow bakery errors to be returned as the bakery would
	// like them, so that httpbakery.Client.Do will work.
	if err, ok := errgo.Cause(err).(*httpbakery.Error); ok {
		return httpbakery.ErrorToResponse(ctx, err)
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
