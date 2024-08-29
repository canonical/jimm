// Copyright 2024 Canonical.
package jujuapi

import (
	"context"
	"strings"

	"github.com/juju/names/v5"

	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/openfga"
)

func checkPermission(ctx context.Context, path string, u *openfga.User, mt names.ModelTag) (bool, error) {
	path = strings.Split(path, "/")[0] // extract just the first segment
	switch path {
	case "log":
		return u.IsModelReader(ctx, mt)
	case "charms":
		return u.IsModelWriter(ctx, mt)
	case "applications":
		return u.IsModelWriter(ctx, mt)
	default:
		return false, errors.E("unknown endpoint " + path)
	}
}
