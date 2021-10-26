// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package dashboard contains a single method that
// registers a simple file server that serves files
// for the Juju Dashboard.
package dashboard

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"text/template"
	"time"

	"github.com/CanonicalLtd/jimm/internal/zapctx"
	jujuparams "github.com/juju/juju/apiserver/params"
	"github.com/juju/version/v2"
	"github.com/julienschmidt/httprouter"
	"go.uber.org/zap"
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
		router.GET(dashboardPath, func(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
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
			// Temporary redirect from /dashboard to /.
			router.GET(dashboardPath, func(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
				http.Redirect(w, req, "/", http.StatusTemporaryRedirect)
			})
			// Serve index.html on /.
			router.GET("/", serveFile)
			// Use index.html to serve anything we don't otherwise know about.
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
		case "version.json":
			buf, err := os.ReadFile(path)
			if err != nil {
				zapctx.Error(ctx, "error reading version.json", zap.Error(err))
				continue
			}
			var fVersion struct {
				Version string `json:"version`
				GitSHA  string `json:"git-sha"`
			}
			err = json.Unmarshal(buf, &fVersion)
			if err != nil {
				zapctx.Error(ctx, "failed to unmarshal version.json", zap.Error(err))
				continue
			}
			ver, err := version.Parse(fVersion.Version)
			if err != nil {
				zapctx.Error(ctx,
					"invalid dashboard version number",
					zap.Error(err),
					zap.String("version", fVersion.Version),
				)
				continue
			}
			versions := jujuparams.GUIArchiveResponse{
				Versions: []jujuparams.GUIArchiveVersion{{
					Version: ver,
					SHA256:  fVersion.GitSHA,
					Current: true,
				}},
			}
			data, err := json.Marshal(versions)
			if err != nil {
				zapctx.Error(ctx, "failed to marshal gui archive versions", zap.Error(err))
				continue
			}
			content := bytes.NewReader(data)
			serveContent := func(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
				http.ServeContent(w, req, "gui-archive.json", time.Now(), content)
			}
			router.GET("/gui-archive", serveContent)
		default:
			router.GET("/"+fi.Name(), serveFile)
		}
	}
	return nil
}
