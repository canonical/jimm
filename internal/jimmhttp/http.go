// Copyright 2024 Canonical.

package jimmhttp

import (
	"context"
	"net/http"

	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"
)

type HTTPHandler struct {
	HTTPProxier HTTPProxier
}

func (h *HTTPHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	if h.HTTPProxier == nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	authErr := h.HTTPProxier.AuthenticateAndAuthorize(ctx, w, req)
	if authErr != nil {
		zapctx.Error(ctx, "authentication error", zap.Error(authErr))
		w.WriteHeader(http.StatusUnauthorized)
		_, err := w.Write([]byte(authErr.Error()))
		zapctx.Error(ctx, "failed to write authentication error", zap.Error(err))
		return
	}
	h.HTTPProxier.ServeHTTP(ctx, w, req)
}

// HTTPProxier is an interface to proxy HTTP requests to controllers.
//
// AuthenticateAndAuthorize handles authentication and authorization.
// ServeHTTP proxies the request.
type HTTPProxier interface {
	AuthenticateAndAuthorize(ctx context.Context, w http.ResponseWriter, req *http.Request) error
	ServeHTTP(ctx context.Context, w http.ResponseWriter, req *http.Request)
}
