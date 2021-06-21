// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package dashboard contains a single function that creates a handler for
// serving the JAAS Dashboard.
package dashboard

import (
	"bytes"
	"context"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"text/template"

	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"
)

const (
	dashboardPath = "/dashboard"
)

// Handler returns an http.Handler that serves the JAAS dashboard from the
// specified location. If the location is an absolute URL then the returned
// handler will redirect to that URL. If the location is a path on the disk
// then the handler will serve the files from that path. Otherwise a
// NotFoundHandler will be returned.
func Handler(ctx context.Context, loc string) http.Handler {
	if loc == "" {
		// If the location isn't configured then don't serve any
		// content.
		return http.NotFoundHandler()
	}

	hnd := redirectHandler(ctx, loc)
	if hnd == nil {
		hnd = pathHandler(ctx, loc)
	}
	if hnd == nil {
		hnd = http.NotFoundHandler()
	}
	return hnd
}

func redirectHandler(ctx context.Context, loc string) http.Handler {
	u, err := url.Parse(loc)
	if err != nil {
		zapctx.Warn(ctx, "cannot parse location", zap.Error(err))
		return nil
	}
	if u.IsAbs() {
		return http.RedirectHandler(loc, http.StatusPermanentRedirect)
	}
	return nil
}

func pathHandler(ctx context.Context, loc string) http.Handler {
	info, err := os.Stat(loc)
	if err != nil {
		zapctx.Warn(ctx, "cannot load dashboard files", zap.Error(err))
		return nil
	}
	if !info.IsDir() {
		return nil
	}
	mux := http.NewServeMux()
	t, err := template.ParseFiles(filepath.Join(loc, "config.js.go"))
	if err == nil {
		var buf bytes.Buffer
		if err := t.Execute(&buf, configParams); err == nil {
			content := buf.Bytes()
			mux.HandleFunc("/config.js", func(w http.ResponseWriter, _ *http.Request) {
				w.Write(content)
			})
		}
	} else {
		zapctx.Warn(ctx, "cannot parse template", zap.Error(err))
	}
	mux.Handle("/", http.FileServer(http.Dir(loc)))
	return http.StripPrefix(dashboardPath, mux)
}

// configParams holds the parameters that need to be provided to the
// config.js.go template for a JAAS dashboard deployement.
var configParams = map[string]interface{}{
	"baseAppURL":                dashboardPath + "/",
	"identityProviderAvailable": true,
	"isJuju":                    false,
}
