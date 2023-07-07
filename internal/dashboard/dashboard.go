// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package dashboard contains a single function that creates a handler for
// serving the JAAS Dashboard.
package dashboard

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"text/template"
	"time"

	"github.com/juju/version/v2"
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
func Handler(ctx context.Context, loc string, publicDNSname string) http.Handler {
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
			root := "/" + de.Name() + "/"
			mux.Handle(root, http.StripPrefix(root, http.FileServer(http.Dir(fn))))
			continue
		}
		hnd := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			http.ServeFile(w, req, fn)
		})
		switch de.Name() {
		case "index.html":
			// serve index.html if there is nothing better to serve.
			mux.Handle("/", hnd)
			mux.Handle(dashboardPath, http.RedirectHandler("/", http.StatusSeeOther))
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
			configParams["baseControllerURL"] = publicDNSname
			if err := t.Execute(&w, configParams); err != nil {
				zapctx.Error(ctx, "error executing config.js.go", zap.Error(err))
				continue
			}
			content := bytes.NewReader(w.Bytes())
			mux.Handle("/config.js", http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				http.ServeContent(w, req, "config.js", modTime, content)
			}))
		case "version.json":
			modTime := time.Now()
			buf, err := os.ReadFile(fn)
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
			type guiArchiveVersion struct {
				// Version holds the Juju GUI version number.
				Version version.Number `json:"version"`
				// SHA256 holds the SHA256 hash of the GUI tar.bz2 archive.
				SHA256 string `json:"sha256"`
				// Current holds whether this specific version is the current one served
				// by the controller.
				Current bool `json:"current"`
			}
			type guiArchiveResponse struct {
				Versions []guiArchiveVersion `json:"versions"`
			}

			versions := guiArchiveResponse{
				Versions: []guiArchiveVersion{{
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
			mux.Handle("/gui-archive", http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				http.ServeContent(w, req, "gui-archive.json", modTime, content)
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
