// Copyright 2023 Canonical Ltd.

package apiconn

import (
	"context"

	jujuparams "github.com/juju/juju/apiserver/params"
	"gopkg.in/errgo.v1"
)

// Offer creates a new ApplicationOffer on the controller. Offer uses the
// Offer procedure on the ApplicationOffers facade version 2.
func (c *Conn) InitiateMigration(ctx context.Context, spec jujuparams.MigrationSpec) (
	*jujuparams.InitiateMigrationResult, error,
) {
	args := jujuparams.InitiateMigrationArgs{
		Specs: []jujuparams.MigrationSpec{spec},
	}

	var results jujuparams.InitiateMigrationResults
	err := c.APICall("Controller", 9, "", "InitiateMigration", &args, &results)
	if err != nil {
		return nil, errgo.Mask(err)
	}
	if len(results.Results) != 1 {
		return nil, errgo.Newf("unexpected number of results (expected 1, got %d)", len(results.Results))
	}
	return &results.Results[0], nil
}
