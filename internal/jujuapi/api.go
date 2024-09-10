// Copyright 2024 Canonical.

// Package jujuapi implements API endpoints for the juju API.
package jujuapi

import (
	"context"
	"net/http"

	"github.com/canonical/jimm/v3/internal/jimm"
	"github.com/canonical/jimm/v3/internal/jimmhttp"
)

// A Params object holds the parameters needed to configure the API
// servers.
type Params struct {
	// ControllerUUID is the UUID of the JIMM controller.
	ControllerUUID string

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
	mux.Handle("/model/{uuid}/api", http.StripPrefix("/model", &jimmhttp.WSHandler{
		Upgrader: websocketUpgrader,
		Server: &apiProxier{apiServer: apiServer{
			jimm: jimm,
		}},
	}))
	// model is not stripped from the URL because it is needed in the proxied request
	mux.Handle("/model/{uuid}/charms", &jimmhttp.HTTPHandler{
		HTTPProxier: &httpProxier{jimm: jimm},
	})
	// model is not stripped from the URL because it is needed in the proxied request
	mux.Handle("/model/{uuid}/applications/*", &jimmhttp.HTTPHandler{
		HTTPProxier: &httpProxier{jimm: jimm},
	})
	mux.Handle("/model/{uuid}/log", http.StripPrefix("/model", &jimmhttp.WSHandler{
		Upgrader: websocketUpgrader,
		Server: &streamProxier{apiServer: apiServer{
			jimm: jimm,
		}},
	}))

	return mux
}
