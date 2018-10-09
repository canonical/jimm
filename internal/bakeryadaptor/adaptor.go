// Copyright 2018 Canonical Ltd.

package bakeryadaptor

import (
	"io"
	"net/http"

	"gopkg.in/errgo.v1"
	"gopkg.in/macaroon-bakery.v2-unstable/httpbakery"
)

// Doer wraps a gopkg.in/macaroon-bakery.v2-unstable/httpbakey.Client so
// that it behaves correctly as a gopkg.in/httprequest.v1.Doer.
type Doer struct {
	Client *httpbakery.Client
}

func (d Doer) Do(req *http.Request) (*http.Response, error) {
	if req.Body == nil {
		return d.Client.Do(req)
	}
	body, ok := req.Body.(io.ReadSeeker)
	if !ok {
		return nil, errgo.New("unsupported request body type")
	}
	req1 := *req
	req1.Body = nil
	return d.Client.DoWithBody(&req1, body)
}
