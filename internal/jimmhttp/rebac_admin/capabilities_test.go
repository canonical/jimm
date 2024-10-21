// Copyright 2024 Canonical.

package rebac_admin_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	qt "github.com/frankban/quicktest"

	"github.com/canonical/jimm/v3/internal/jimmhttp/rebac_admin"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
)

// test capabilities are reachable
func TestCapabilities(t *testing.T) {
	c := qt.New(t)
	jimm := jimmtest.JIMM{}
	ctx := context.Background()
	handlers, err := rebac_admin.SetupBackend(ctx, &jimm)
	c.Assert(err, qt.IsNil)
	testServer := httptest.NewServer(handlers.Handler(""))
	defer testServer.Close()

	// test not found endpoint
	url := fmt.Sprintf("%s/v1%s", testServer.URL, "/not-found")
	req, err := http.NewRequest("GET", url, nil)
	c.Assert(err, qt.IsNil)
	resp, err := http.DefaultClient.Do(req)
	c.Assert(err, qt.IsNil)
	defer resp.Body.Close()
	c.Assert(resp.StatusCode, qt.Equals, 404)

	// test endpoints in capabilities are found
	for _, cap := range rebac_admin.Capabilities {
		for _, m := range cap.Methods {
			c.Run(fmt.Sprintf("%s %s", m, cap.Endpoint), func(c *qt.C) {
				url := fmt.Sprintf("%s/v1%s", testServer.URL, cap.Endpoint)
				req, err := http.NewRequest(string(m), url, nil)
				c.Assert(err, qt.IsNil)
				resp, err := http.DefaultClient.Do(req)
				c.Assert(err, qt.IsNil)
				defer resp.Body.Close()
				// 404 is for not found endpoints and 501 is for "not implemented" endpoints in the rebac-admin-ui-handlers library
				isNotFound := resp.StatusCode == 404 || resp.StatusCode == 501
				c.Assert(isNotFound, qt.IsFalse)
			})

		}
	}

}
