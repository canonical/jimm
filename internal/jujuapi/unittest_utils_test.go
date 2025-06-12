// Copyright 2025 Canonical.

package jujuapi_test

import (
	"github.com/juju/names/v5"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/jimm"
	"github.com/canonical/jimm/v3/internal/jujuapi"
	"github.com/canonical/jimm/v3/internal/openfga"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest/mocks"
)

func newTestControllerRoot(jujuManager mocks.JujuManager, userEmail string, admin bool) *jujuapi.ControllerRoot {
	jimm := &jimmtest.JIMM{
		JujuManager_: func() jimm.JujuManager {
			return &jujuManager
		},
	}
	var u dbmodel.Identity
	u.SetTag(names.NewUserTag(userEmail))
	user := openfga.NewUser(&u, nil)

	user.JimmAdmin = admin

	root := jujuapi.NewControllerRoot(jimm, jujuapi.Params{})
	jujuapi.SetUser(root, user)

	return root
}
