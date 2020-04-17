// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package dashboard contains a single method that
// registers a simple file server that serves files
// for the Juju Dashboard.
package dashboard

import (
	"context"
	"net/http"

	"github.com/julienschmidt/httprouter"
)

const (
	dashboardPathPrefix = "dashboard"
)

// Register registers a http handler the serves Juju Dashboard
// files.
func Register(ctx context.Context, router *httprouter.Router, dataDir string) {
	router.ServeFiles("/"+dashboardPathPrefix+"/*filepath", http.Dir(dataDir))
	router.ServeFiles("/static/*filepath", http.Dir(dataDir+"/static"))
	router.GET("/config.js", func(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
		http.ServeFile(w, req, dataDir+"/config.js")
	})
}
