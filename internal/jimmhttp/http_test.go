// Copyright 2024 Canonical.

package jimmhttp_test

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	qt "github.com/frankban/quicktest"

	"github.com/canonical/jimm/v3/internal/jimmhttp"
)

func TestHTTPHandler(t *testing.T) {
	c := qt.New(t)

	hnd := &jimmhttp.HTTPHandler{
		HTTPProxier: testHTTPServer{t: c},
	}

	srv := httptest.NewServer(hnd)
	c.Cleanup(srv.Close)

	client := http.Client{}

	// test ok
	resp, err := client.Get(srv.URL)
	c.Assert(err, qt.IsNil)
	defer resp.Body.Close()
	c.Assert(err, qt.IsNil)
	c.Assert(resp.StatusCode, qt.Equals, http.StatusOK)

	// test unauthorized
	req, err := http.NewRequest("POST", srv.URL, nil)
	c.Assert(err, qt.IsNil)
	req.Header.Set("Authorization", "wrong")
	resp, err = client.Do(req)
	c.Assert(err, qt.IsNil)
	defer resp.Body.Close()
	c.Assert(err, qt.IsNil)
	c.Assert(resp.StatusCode, qt.Equals, http.StatusUnauthorized)
}

type testHTTPServer struct {
	t testing.TB
}

func (s testHTTPServer) AuthenticateAndAuthorize(ctx context.Context, w http.ResponseWriter, req *http.Request) error {
	if auth := req.Header.Get("Authorization"); auth == "wrong" {
		return errors.New("error")
	}
	return nil
}

func (s testHTTPServer) ServeHTTP(ctx context.Context, w http.ResponseWriter, req *http.Request) {
	var buf bytes.Buffer
	_ = req.Write(&buf)
	_, _ = w.Write(buf.Bytes())
}
