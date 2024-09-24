// Copyright 2024 Canonical.

package jimmhttp_test

import (
	"bytes"
	"context"
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

	// test ok
	resp, err := http.Get(srv.URL)
	c.Assert(err, qt.IsNil)
	defer resp.Body.Close()
	c.Assert(err, qt.IsNil)
	c.Assert(resp.StatusCode, qt.Equals, http.StatusOK)
}

type testHTTPServer struct {
	t testing.TB
}

func (s testHTTPServer) ServeHTTP(ctx context.Context, w http.ResponseWriter, req *http.Request) {
	var buf bytes.Buffer
	_ = req.Write(&buf)
	_, _ = w.Write(buf.Bytes())
}
