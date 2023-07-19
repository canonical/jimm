// Copyright 2016 Canonical Ltd.

// Package jujuapi implements API endpoints for the juju API.
package jujuapi

import (
	"context"
	"net/http"

	"github.com/canonical/jimm/internal/jimm"
	"github.com/canonical/jimm/internal/jimmhttp"
)

// A Params object holds the paramaters needed to configure the API
// servers.
type Params struct {
	// ControllerUUID is the UUID of the JIMM controller.
	ControllerUUID string

	// IdentityLocation holds the URL of the third-party identity
	// provider.
	IdentityLocation string

	// PublicDNSName is the name to advertise as the public address of
	// the juju controller.
	PublicDNSName string
}

// APIHandler returns an http Handler for the /api endpoint.
func APIHandler(ctx context.Context, jimm *jimm.JIMM, p Params) http.Handler {
	return &jimmhttp.WSHandler{
		Upgrader: websocketUpgrader,
		Server: &apiServer{
			jimm:   jimm,
			params: p,
		},
	}
}

// ModelHandler creates an http.Handler for "/model" endpoints.
func ModelHandler(ctx context.Context, jimm *jimm.JIMM, p Params) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/", &jimmhttp.WSHandler{
		Upgrader: websocketUpgrader,
		Server:   modelProxyServer{jimm: jimm},
	})
	return mux
}
