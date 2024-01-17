// Copyright 2024 Canonical Ltd.

package jimm_test

import (
	"context"
	"testing"

	qt "github.com/frankban/quicktest"

	"github.com/canonical/jimm/internal/dbmodel"
	"github.com/canonical/jimm/internal/jimm"
	"github.com/canonical/jimm/internal/jimmtest"
	"github.com/canonical/jimm/internal/openfga"
)

func TestAddServiceAccount(t *testing.T) {
	c := qt.New(t)

	ctx := context.Background()
	client, _, _, err := jimmtest.SetupTestOFGAClient(c.Name())
	c.Assert(err, qt.IsNil)
	j := &jimm.JIMM{
		OpenFGAClient: client,
	}
	c.Assert(err, qt.IsNil)
	user := openfga.NewUser(
		&dbmodel.Identity{
			Name:        "bob@external",
			DisplayName: "Bob",
		},
		client,
	)
	clientID := "39caae91-b914-41ae-83f8-c7b86ca5ad5a"
	err = j.AddServiceAccount(ctx, user, clientID)
	c.Assert(err, qt.IsNil)
	err = j.AddServiceAccount(ctx, user, clientID)
	c.Assert(err, qt.IsNil)
	userAlice := openfga.NewUser(
		&dbmodel.Identity{
			Name:        "alive@external",
			DisplayName: "Alice",
		},
		client,
	)
	err = j.AddServiceAccount(ctx, userAlice, clientID)
	c.Assert(err, qt.ErrorMatches, "service account already owned")
}
