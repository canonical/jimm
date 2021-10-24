// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package dashboard contains a single method that
// registers a simple file server that serves files
// for the Juju Dashboard.
package dashboard

import (
	"bytes"
	"context"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"text/template"
	"time"

	"github.com/julienschmidt/httprouter"
	"gopkg.in/errgo.v1"
)

const (
	dashboardPath = "/dashboard"
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
	f, err := os.Open(dashboardLocation)
	if err != nil {
		return errgo.Mask(err)
	}
	defer f.Close()
	fis, err := f.Readdir(0)
	if err != nil {
		return errgo.Mask(err)
	}
	for _, fi := range fis {
		path := filepath.Join(dashboardLocation, fi.Name())
		if fi.IsDir() {
			router.ServeFiles("/"+fi.Name()+"/*filepath", http.Dir(path))
			continue
		}
		serveFile := func(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
			http.ServeFile(w, req, path)
		}
		switch fi.Name() {
		case "index.html":
			// Use index.html to serve anything we don't otherwise know about.
			router.GET(dashboardPath, serveFile)
			router.NotFound = http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				http.ServeFile(w, req, path)
			})
		case "config.js":
			// Ignore any config.js file, use a rendered one instead.
		case "config.js.go":
			tmpl, err := template.ParseFiles(path)
			if err != nil {
				return errgo.Notef(err, "cannot parse dashboard configuration template")
			}
			var buf bytes.Buffer
			err = tmpl.Execute(&buf, map[string]interface{}{
				"baseAppURL":                "/",
				"identityProviderAvailable": true,
				"isJuju":                    false,
			})
			if err != nil {
				return errgo.Notef(err, "cannot execute dashboard configuration template")
			}
			serveConfig := func(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
				http.ServeContent(w, req, "config.js", time.Now(), bytes.NewReader(buf.Bytes()))
			}
			router.GET("/config.js", serveConfig)
		default:
			router.GET("/"+fi.Name(), serveFile)
		}
	}
	return nil
}
