// Copyright 2018 Canonical Ltd.

package kubetest

import (
	"io"
	"net/http"
	"net/http/httptest"

	gc "gopkg.in/check.v1"
)

const (
	Username = "test-kubernetes-user"
	Password = "test-kubernetes-password"
)

// NewFakeKubernetes creates a minimal kubernetes API server which
// response to just the API calls required by the tests.
func NewFakeKubernetes(c *gc.C) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path != "/api/v1/namespaces" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if req.Method != "POST" {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if username, password, ok := req.BasicAuth(); !ok || username != Username || password != Password {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", req.Header.Get("Content-Type"))
		io.Copy(w, req.Body)
	}))
	return srv
}
