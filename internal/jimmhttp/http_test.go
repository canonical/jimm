// Copyright 2021 Canonical Ltd.

package jimmhttp_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	qt "github.com/frankban/quicktest"

	"github.com/CanonicalLtd/jimm/internal/jimmhttp"
)

var stripPathElementTests = []struct {
	name       string
	url        string
	expectVar  string
	expectPath string
}{{
	name:       "simple",
	url:        "/foo/bar",
	expectVar:  "foo",
	expectPath: "/bar",
}, {
	name:       "no_suffix",
	url:        "/foo",
	expectVar:  "foo",
	expectPath: "",
}, {
	name:       "empty",
	url:        "",
	expectVar:  "",
	expectPath: "",
}, {
	name:       "root",
	url:        "/",
	expectVar:  "",
	expectPath: "",
}, {
	name:       "escaped",
	url:        "/foo%2fbar",
	expectVar:  "foo",
	expectPath: "/bar",
}}

func TestStripPathElement(t *testing.T) {
	c := qt.New(t)

	for _, test := range stripPathElementTests {
		c.Run(test.name, func(c *qt.C) {
			var hnd http.Handler = http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				c.Check(jimmhttp.PathElementFromContext(req.Context(), "key"), qt.Equals, test.expectVar)
				c.Check(req.URL.Path, qt.Equals, test.expectPath)
				c.Check(req.URL.RawPath, qt.Equals, "")
			})
			hnd = jimmhttp.StripPathElement("key", hnd)

			req, err := http.NewRequest("GET", test.url, nil)
			c.Assert(err, qt.IsNil)
			rr := httptest.NewRecorder()

			hnd.ServeHTTP(rr, req)
			resp := rr.Result()
			c.Check(resp.StatusCode, qt.Equals, http.StatusOK)
		})
	}
}
