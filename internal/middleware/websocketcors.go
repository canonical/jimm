// Copyright 2024 Canonical.
package middleware

import (
	"net/http"

	"github.com/rs/cors"
)

// WebsocketCors provides middleware for handling CORS on websockets.
type WebsocketCors struct {
	cors *cors.Cors
}

// NewWebsocketCors returns a new WebsocketCors object.
// If no allowedOrigins are provided, all origins will be allowed.
func NewWebsocketCors(allowedOrigins []string) *WebsocketCors {
	corsOpts := cors.New(cors.Options{
		AllowedOrigins: allowedOrigins,
	})
	return &WebsocketCors{cors: corsOpts}
}

// Handler implements CORS validation for websocket handlers.
// Any methods beside GET are rejected as a bad request.
// If the origin is not in the allow list, a status forbidden is returned.
func (c *WebsocketCors) Handler(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte("unexpected method"))
			return
		}

		// We can allow cross-origin requests for those requests that don't
		// include cookies. For now we enforce CORS protection on all requests
		// because the only client that is expected to connect cross-origin
		// is the Juju dashboard which uses cookies for auth.
		originPresent := r.Header.Get("Origin") != ""
		if originPresent && !c.cors.OriginAllowed(r) {
			w.WriteHeader(http.StatusForbidden)
			return
		}

		h.ServeHTTP(w, r)
	})
}
