// Copyright 2024 Canonical.

package jimmhttp

import (
	"context"
	"net/http"
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
	h.HTTPProxier.ServeHTTP(ctx, w, req)
}

// HTTPProxier is an interface to proxy HTTP requests to controllers.
//
// ServeHTTP proxies the request.
type HTTPProxier interface {
	ServeHTTP(ctx context.Context, w http.ResponseWriter, req *http.Request)
}
