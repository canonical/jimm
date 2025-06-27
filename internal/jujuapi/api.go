// Copyright 2025 Canonical.

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

	// PublicDNSName is returned to Juju clients on login.
	// It is the hostname that will be used during TLS verification.
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
	mux.Handle("/{uuid}/api", &jimmhttp.WSHandler{
		Upgrader: websocketUpgrader,
		Server: &apiModelProxier{apiServer: apiServer{
			jimm: jimm,
		}},
	})
	mux.Handle("/{uuid}/log", &jimmhttp.WSHandler{
		Upgrader: websocketUpgrader,
		Server: &streamModelProxier{apiServer: apiServer{
			jimm: jimm,
		}},
	})
	return mux
}

// LogTransferHandler creates an http.Handler for the "/migrate/logtransfer" endpoint.
func LogTransferHandler(ctx context.Context, jimm *jimm.JIMM, p Params) http.Handler {
	return &jimmhttp.WSHandler{
		Upgrader: websocketUpgrader,
		Server:   &streamControllerProxier{jimm: jimm},
	}
}
