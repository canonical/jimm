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
	"path"
	"path/filepath"
	"text/template"
	"time"

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
	mux := http.NewServeMux()

	u, err := url.Parse(loc)
	if err != nil {
		zapctx.Warn(ctx, "cannot parse location", zap.Error(err))
	}
	if u != nil && u.IsAbs() {
		mux.Handle(dashboardPath, http.RedirectHandler(loc, http.StatusPermanentRedirect))
		return mux
	}

	f, err := os.Open(loc)
	if err != nil {
		zapctx.Warn(ctx, "error reading dashboard path", zap.Error(err))
		return mux
	}
	defer f.Close()
	des, err := f.ReadDir(0)
	if err != nil {
		zapctx.Warn(ctx, "error reading dashboard path", zap.Error(err))
		return mux
	}

	for _, de := range des {
		fn := filepath.Join(loc, de.Name())
		if de.IsDir() {
			mux.Handle(path.Join("/", de.Name(), "/"), http.FileServer(http.Dir(fn)))
			continue
		}
		hnd := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			http.ServeFile(w, req, fn)
		})
		switch de.Name() {
		case "index.html":
			mux.Handle(dashboardPath, hnd)
			// serve index.html if there is nothing better to serve.
			mux.Handle("/", hnd)
		case "config.js":
			continue
		case "config.js.go":
			modTime := time.Now()
			buf, err := os.ReadFile(fn)
			if err != nil {
				zapctx.Error(ctx, "error reading config.js.go", zap.Error(err))
				continue
			}
			t, err := template.New("").Parse(string(buf))
			if err != nil {
				zapctx.Error(ctx, "error parsing config.js.go", zap.Error(err))
				continue
			}
			var w bytes.Buffer
			if err := t.Execute(&w, configParams); err != nil {
				zapctx.Error(ctx, "error executing config.js.go", zap.Error(err))
				continue
			}
			content := bytes.NewReader(w.Bytes())
			mux.Handle("/config.js", http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				http.ServeContent(w, req, "config.js", modTime, content)
			}))
		default:
			mux.Handle(path.Join("/", de.Name()), hnd)
		}
	}

	return mux
}

// configParams holds the parameters that need to be provided to the
// config.js.go template for a JAAS dashboard deployement.
var configParams = map[string]interface{}{
	"baseAppURL":                "/",
	"identityProviderAvailable": true,
	"isJuju":                    false,
}
