// Copyright 2024 Canonical.

// Package jimmhttp contains utilities for HTTP connections.
package jimmhttp

import (
	"context"
	"net/http"
	"net/url"
	"strings"

	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"
)

type contextPathKey string

// PathElementFromContext returns the value of the path element previously
// extracted in a StripPathElement handler.
func PathElementFromContext(ctx context.Context, key string) string {
	s, _ := ctx.Value(contextPathKey(key)).(string)
	return s
}

// StripPathElement returns a handler that serves HTTP requests by removing
// the first element from the request path and invoking the handler h.
//
// If a key is specified the removed element will be stored in the context
// attached to the request such that it can be retrieved using
// PathElementFromContext.
//
// If the request URL contains a RawPath field then it will be cleared
// before calling h.
func StripPathElement(key string, h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		v, p := splitPath(req.URL.Path)
		req2 := new(http.Request)
		*req2 = *req
		req2.URL = new(url.URL)
		*req2.URL = *req.URL
		req2.URL.Path = p
		// clear the RawPath if it was previously set.
		req2.URL.RawPath = ""
		if key != "" {
			req2 = req2.WithContext(context.WithValue(req2.Context(), contextPathKey(key), v))
		}
		h.ServeHTTP(w, req2)
	})
}

func splitPath(s string) (elem, remain string) {
	if len(s) > 0 && s[0] == '/' {
		s = s[1:]
	}
	if n := strings.IndexByte(s, '/'); n >= 0 {
		return s[:n], s[n:]
	}
	return s, ""
}

// writeError writes an error and logs the message. It is expected that the status code
// is an erroneous status code.
func writeError(ctx context.Context, w http.ResponseWriter, status int, err error, logMessage string) {
	zapctx.Error(ctx, logMessage, zap.Error(err))
	w.WriteHeader(status)
	errMsg := ""
	if err != nil {
		errMsg = " - " + err.Error()
	}
	_, err = w.Write([]byte(http.StatusText(status) + errMsg))
	if err != nil {
		zapctx.Error(ctx, "failed to write status text error", zap.Error(err))
	}
}
