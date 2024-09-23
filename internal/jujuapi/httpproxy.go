// Copyright 2024 Canonical.
package jujuapi

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/juju/names/v5"
	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"

	"github.com/canonical/jimm/v3/internal/rpc"
)

type HTTPProxier struct {
	JIMM JIMM // interface
}

// ServeHTTP extract the model uuid from the path to retrieve necessary information to proxy the request to the right controller.
func (s *HTTPProxier) ServeHTTP(ctx context.Context, w http.ResponseWriter, req *http.Request) {
	writeError := func(msg string, code int) {
		w.WriteHeader(code)
		_, err := w.Write([]byte(msg))
		if err != nil {
			zapctx.Error(ctx, "cannot write to connection", zap.Error(err))
		}
	}
	modelUUID := chi.URLParam(req, "uuid")
	if modelUUID == "" {
		writeError("cannot parse path", http.StatusUnprocessableEntity)
		return
	}
	// retrieving credentials from controller
	model, err := s.JIMM.GetModel(ctx, modelUUID)
	if err != nil {
		writeError("cannot get model", http.StatusNotFound)
		return
	}
	u, p, err := s.JIMM.GetCredentialStore().GetControllerCredentials(ctx, model.Controller.Name)
	if err != nil {
		writeError("cannot retrieve credentials", http.StatusNotFound)
		return
	}
	req.SetBasicAuth(names.NewUserTag(u).String(), p)

	// proxy request
	rpc.ProxyHTTP(ctx, &model.Controller, w, req)
}
