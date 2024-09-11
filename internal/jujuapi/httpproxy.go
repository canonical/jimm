// Copyright 2024 Canonical.
package jujuapi

import (
	"context"
	"net/http"
	"strings"

	"github.com/juju/names/v5"
	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"

	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/rpc"
)

type httpProxier struct {
	jimm JIMM // interface
}

func (s *httpProxier) Authenticate(ctx context.Context, w http.ResponseWriter, req *http.Request) error {
	// extract auth token
	_, password, ok := req.BasicAuth()
	if !ok {
		return errors.E(errors.CodeUnauthorized, "authentication missing")
	}
	user, err := s.jimm.LoginWithSessionToken(ctx, password)
	if err != nil {
		return errors.E(errors.CodeUnauthorized, err)
	}
	// extract model uuid and check permission
	sPath, _ := strings.CutPrefix(req.URL.EscapedPath(), "/model")
	uuid, finalPath, err := modelInfoFromPath(sPath)
	if err != nil {
		return errors.E(errors.CodeUnauthorized, "cannot parse path")
	}
	if ok, err := checkModelAccessForUser(ctx, finalPath, user, names.NewModelTag(uuid)); err != nil {
		return errors.E(errors.CodeUnauthorized, "unauthorized")
	} else if !ok {
		return errors.E(errors.CodeUnauthorized, "unauthorized")
	}
	return nil
}

func (s *httpProxier) ServeHTTP(ctx context.Context, w http.ResponseWriter, req *http.Request) {
	writeError := func(msg string, code int) {
		w.WriteHeader(code)
		_, err := w.Write([]byte(msg))
		if err != nil {
			zapctx.Error(ctx, "cannot write to connection", zap.Error(err))
		}
	}
	sPath, _ := strings.CutPrefix(req.URL.EscapedPath(), "/model")
	uuid, _, err := modelInfoFromPath(sPath)
	if err != nil {
		writeError("cannot parse path", http.StatusUnprocessableEntity)
		return
	}
	// retrieving credentials from controller
	model, err := s.jimm.GetModel(ctx, uuid)
	if err != nil {
		writeError("cannot get model", http.StatusNotFound)
		return
	}
	u, p, err := s.jimm.GetCredentialStore().GetControllerCredentials(ctx, model.Controller.Name)
	if err != nil {
		writeError("cannot retrieve credentials", http.StatusNotFound)
		return
	}
	req.SetBasicAuth(names.NewUserTag(u).String(), p)

	// proxy request
	rpc.ProxyHTTP(ctx, &model.Controller, w, req)
}
