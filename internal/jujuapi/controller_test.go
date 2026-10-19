// Copyright 2025 Canonical.

package jujuapi_test

import (
	"context"
	"testing"

	qt "github.com/frankban/quicktest"
	"github.com/google/uuid"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v6"
	gossh "golang.org/x/crypto/ssh"

	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/jimm/config"
	"github.com/canonical/jimm/v3/internal/jujuapi"
	"github.com/canonical/jimm/v3/internal/openfga"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest/mocks"
)

func TestSSHServerHostKey(t *testing.T) {
	c := qt.New(t)
	privateKey, err := jimmtest.TestRSAKey()
	c.Assert(err, qt.IsNil)
	publicKey, err := gossh.NewPublicKey(privateKey.Public())
	c.Assert(err, qt.IsNil)
	configManager, err := config.NewConfigManager(config.ControllerConfig{
		SSHPublicHostKey: string(gossh.MarshalAuthorizedKey(publicKey)),
	})
	c.Assert(err, qt.IsNil)

	cr := jujuapi.NewControllerRoot(&jimmtest.JIMM{
		ConfigManager_: func() jujuapi.ConfigManager {
			return configManager
		},
	}, jujuapi.Params{})

	result, err := cr.SSHServerHostKey(context.Background())
	c.Assert(err, qt.IsNil)
	parsedKey, err := gossh.ParsePublicKey(result.PublicKey)
	c.Assert(err, qt.IsNil)
	c.Check(parsedKey.Marshal(), qt.DeepEquals, publicKey.Marshal())
}

func TestSSHServerPort(t *testing.T) {
	c := qt.New(t)
	configManager, err := config.NewConfigManager(config.ControllerConfig{
		SSHPort: 17022,
	})
	c.Assert(err, qt.IsNil)

	cr := jujuapi.NewControllerRoot(&jimmtest.JIMM{
		ConfigManager_: func() jujuapi.ConfigManager {
			return configManager
		},
	}, jujuapi.Params{})

	result, err := cr.SSHServerPort(context.Background())
	c.Assert(err, qt.IsNil)
	c.Check(result.Port, qt.Equals, 17022)
}

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
