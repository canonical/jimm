// Copyright 2024 Canonical.
package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	qt "github.com/frankban/quicktest"

	"github.com/canonical/jimm/v3/internal/middleware"
)

func TestWebsocketCors(t *testing.T) {
	tests := []struct {
		name           string
		allowedOrigins []string
		origin         string
		method         string
		expectedStatus int
	}{
		{
			name:           "success with no origin header",
			method:         http.MethodGet,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "success with allowed origin",
			method:         http.MethodGet,
			allowedOrigins: []string{"jaas.com"},
			origin:         "jaas.com",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "success with missing origin header and configured allowed origins",
			method:         http.MethodGet,
			allowedOrigins: []string{"jaas.com"},
			origin:         "",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "success with wildcard",
			method:         http.MethodGet,
			allowedOrigins: []string{"jaas.com/foo/*"},
			origin:         "jaas.com/foo/bar",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "failure with forbidden origin",
			method:         http.MethodGet,
			allowedOrigins: []string{"jaas.com"},
			origin:         "my-host.com",
			expectedStatus: http.StatusForbidden,
		},
		{
			name:           "get method returns bad request",
			method:         http.MethodConnect,
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		c := qt.New(t)
		c.Run(tt.name, func(c *qt.C) {

			req := httptest.NewRequest(tt.method, "/", nil)
			if tt.origin != "" {
				req.Header.Add("Origin", tt.origin)
			}
			w := httptest.NewRecorder()

			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("the-body"))
			})

			cors := middleware.NewWebsocketCors(tt.allowedOrigins)
			handlerWithCors := cors.Handler(handler)
			handlerWithCors.ServeHTTP(w, req)

			c.Assert(w.Code, qt.Equals, tt.expectedStatus)
		})
	}
}
