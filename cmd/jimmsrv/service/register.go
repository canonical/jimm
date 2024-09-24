// Copyright (\d{4}) Canonical.

package service

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/canonical/jimm/v3/internal/jimm"
	"github.com/canonical/jimm/v3/internal/jimmhttp"
	"github.com/canonical/jimm/v3/internal/jujuapi"
	"github.com/canonical/jimm/v3/internal/middleware"
	ofganames "github.com/canonical/jimm/v3/internal/openfga/names"
)

// RegisterModelHTTPEndpoints register the group of http endpoints for models, with their respective middlewares
func RegisterModelHTTPEndpoints(mux *chi.Mux, jimm *jimm.JIMM) {
	mux.Group(func(r chi.Router) {
		r.Use(func(h http.Handler) http.Handler { return middleware.AuthenticateWithSessionTokenViaBasicAuth(h, jimm) })
		r.Use(func(h http.Handler) http.Handler {
			return middleware.AuthorizeUserForModelAccess(h, jimm, ofganames.WriterRelation)
		})
		r.Handle("/model/{uuid}/charms", &jimmhttp.HTTPHandler{
			HTTPProxier: &jujuapi.HTTPProxier{JIMM: jimm},
		})
		mux.Handle("/model/{uuid}/applications/*", &jimmhttp.HTTPHandler{
			HTTPProxier: &jujuapi.HTTPProxier{JIMM: jimm},
		})
	})

}
