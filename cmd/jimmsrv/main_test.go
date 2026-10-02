// Copyright 2025 Canonical.

package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	qt "github.com/frankban/quicktest"

	"github.com/canonical/jimm/v3/internal/middleware"
)

func TestCorsAllowedOriginsEmptyListAllowsAllOrigins(t *testing.T) {
	c := qt.New(t)

	splitOrigins := strings.Split("", " ")
	c.Assert(splitOrigins, qt.DeepEquals, []string{""})

	fieldsOrigins := strings.Fields("")
	c.Assert(fieldsOrigins, qt.HasLen, 0)

	serveWithOrigins := func(allowedOrigins []string) int {
		cors := middleware.NewWebsocketCors(allowedOrigins)
		handler := cors.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Origin", "https://jaas.dev.ps7.canonical.com")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		return w.Code
	}

	c.Assert(serveWithOrigins(splitOrigins), qt.Equals, http.StatusForbidden)
	c.Assert(serveWithOrigins(fieldsOrigins), qt.Equals, http.StatusOK)
}
