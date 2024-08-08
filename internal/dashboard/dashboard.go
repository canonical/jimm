// Copyright 2024 Canonical.

// Package dashboard contains a single function that creates a handler for
// serving the JAAS Dashboard.
package dashboard

import (
	"context"
	"net/http"
	"net/url"

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
	} else {
		zapctx.Warn(ctx, "not redirecting to the dashboard", zap.String("dashboard_location", loc))
	}
	return mux
}
