// Copyright 2024 Canonical.

package kubetest

import (
	"io"
	"net/http"
	"net/http/httptest"

	gc "gopkg.in/check.v1"
)

const (
	Username = "test-kubernetes-user"
	//nolint:gosec // Thinks it's an exposed secret.
	Password = "test-kubernetes-password"
)

func NewFakeKubernetes(c *gc.C) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if username, password, ok := req.BasicAuth(); !ok || username != Username || password != Password {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		switch req.URL.Path {
		case "/version":
			w.Header().Set("Content-Type", "application/json")
			_, err := w.Write([]byte(`{"major":"1","minor":"21","gitVersion":"v1.21.0"}`))
			c.Assert(err, gc.IsNil)
		case "/api/v1/namespaces":
			if req.Method != "POST" {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			w.Header().Set("Content-Type", req.Header.Get("Content-Type"))
			_, err := io.Copy(w, req.Body)
			c.Assert(err, gc.IsNil)
		case "/api":
			w.Header().Set("Content-Type", "application/json")
			_, err := w.Write([]byte(`{"versions":["v1"]}`))
			c.Assert(err, gc.IsNil)
		case "/apis":
			w.Header().Set("Content-Type", "application/json")
			_, err := w.Write([]byte(`{"groups":[{"name":"apps","versions":[{"groupVersion":"apps/v1","version":"v1"}]}]}`))
			c.Assert(err, gc.IsNil)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	return srv
}
