// Copyright 2026 Canonical.

package dbmodel_test

import (
	"testing"

	qt "github.com/frankban/quicktest"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v5"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	apiparams "github.com/canonical/jimm/v3/pkg/api/params"
)

func TestControllerBootstrapToAPIControllerInfo(t *testing.T) {
	c := qt.New(t)

	ci := dbmodel.ControllerBootstrap{
		Name:        "test-controller",
		CloudName:   "test-cloud",
		CloudRegion: "test-region",
	}.ToControllerInfo()

	c.Check(ci, qt.DeepEquals, apiparams.ControllerInfo{
		Name:        "test-controller",
		CloudTag:    names.NewCloudTag("test-cloud").String(),
		CloudRegion: "test-region",
		Status: jujuparams.EntityStatus{
			Status: "bootstrapping",
		},
	})
}
