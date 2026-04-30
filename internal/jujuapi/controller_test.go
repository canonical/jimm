// Copyright 2025 Canonical.

package jujuapi_test

import (
	"context"
	"testing"

	qt "github.com/frankban/quicktest"
	"github.com/google/uuid"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v5"

	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/jujuapi"
	"github.com/canonical/jimm/v3/internal/openfga"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest/mocks"
)

func TestInitiateMigration(t *testing.T) {
	c := qt.New(t)

	mt := names.NewModelTag(uuid.New().String())
	migrationID := uuid.New().String()

	tests := []struct {
		about             string
		initiateMigration func(ctx context.Context, user *openfga.User, spec jujuparams.MigrationSpec) (jujuparams.InitiateMigrationResult, error)
		args              jujuparams.InitiateMigrationArgs
		expectedError     string
		expectedResult    jujuparams.InitiateMigrationResults
	}{{
		about: "model migration initiated successfully",
		initiateMigration: func(ctx context.Context, user *openfga.User, spec jujuparams.MigrationSpec) (jujuparams.InitiateMigrationResult, error) {
			return jujuparams.InitiateMigrationResult{
				ModelTag:    mt.String(),
				MigrationId: migrationID,
			}, nil
		},
		args: jujuparams.InitiateMigrationArgs{
			Specs: []jujuparams.MigrationSpec{{
				ModelTag: mt.String(),
			}},
		},
		expectedResult: jujuparams.InitiateMigrationResults{
			Results: []jujuparams.InitiateMigrationResult{{
				ModelTag:    mt.String(),
				MigrationId: migrationID,
			}},
		},
	}, {
		about: "controller returns an error",
		initiateMigration: func(ctx context.Context, user *openfga.User, spec jujuparams.MigrationSpec) (jujuparams.InitiateMigrationResult, error) {
			return jujuparams.InitiateMigrationResult{}, errors.New("a silly error")
		},
		args: jujuparams.InitiateMigrationArgs{
			Specs: []jujuparams.MigrationSpec{{
				ModelTag: mt.String(),
			}},
		},
		expectedResult: jujuparams.InitiateMigrationResults{
			Results: []jujuparams.InitiateMigrationResult{{
				Error: &jujuparams.Error{
					Message: "a silly error",
				},
			}},
		},
	}}

	for _, test := range tests {
		c.Run(test.about, func(c *qt.C) {
			jujuManager := mocks.JujuManager{
				InitiateMigration_: test.initiateMigration,
			}
			jimm := &jimmtest.JIMM{
				JujuManager_: func() jujuapi.JujuManager {
					return &jujuManager
				},
			}
			cr := jujuapi.NewControllerRoot(jimm, jujuapi.Params{})

			result, err := cr.InitiateMigration(context.Background(), test.args)
			if test.expectedError == "" {
				c.Assert(err, qt.IsNil)
				c.Assert(result, qt.DeepEquals, test.expectedResult)
			} else {
				c.Assert(err, qt.ErrorMatches, test.expectedError)
			}
		})
	}
}
