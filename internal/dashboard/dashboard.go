// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package dashboard contains a single method that
// registers a simple file server that serves files
// for the Juju Dashboard.
package dashboard

import (
	"context"
	"net/http"
	"net/url"

	"github.com/julienschmidt/httprouter"
	"gopkg.in/errgo.v1"
)

const (
	dashboardPathPrefix = "dashboard"
)

// Register registers a http handler the serves Juju Dashboard
// files.
func Register(ctx context.Context, router *httprouter.Router, dashboardLocation string) error {
	u, err := url.Parse(dashboardLocation)
	if err != nil {
		return errgo.Mask(err)
	}
	if u.IsAbs() {
		router.GET("/dashboard", func(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
			http.Redirect(w, req, dashboardLocation, http.StatusPermanentRedirect)
		})
		return nil
	}
	router.ServeFiles("/"+dashboardPathPrefix+"/*filepath", http.Dir(dashboardLocation))
	router.ServeFiles("/static/*filepath", http.Dir(dashboardLocation+"/static"))
	router.GET("/config.js", func(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
		http.ServeFile(w, req, dashboardLocation+"/config.js")
	})
	return nil
}
