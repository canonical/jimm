// Copyright 2025 Canonical.

package jujuapi_test

import (
	"context"

	"github.com/google/uuid"
	jujucloud "github.com/juju/juju/cloud"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v5"
	gc "gopkg.in/check.v1"

	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/jimm"
	"github.com/canonical/jimm/v3/internal/jimm/bootstrap"
	"github.com/canonical/jimm/v3/internal/openfga"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest/mocks"
	"github.com/canonical/jimm/v3/pkg/api/params"
	apiparams "github.com/canonical/jimm/v3/pkg/api/params"
)

type jimmUnitTestSuite struct{}

var _ = gc.Suite(&jimmUnitTestSuite{})

func (s *jimmUnitTestSuite) TestPrepareModelMigration_UnauthorizedUser(c *gc.C) {
	ctx := context.Background()
	jimm := &jimmtest.JIMM{
		JujuManager_: func() jimm.JujuManager {
			return &mocks.JujuManager{}
		},
	}
	root := newTestControllerRoot(jimm, "alice@canonical.com", false)

	_, err := root.PrepareModelMigration(ctx, apiparams.PrepareModelMigrationRequest{})

	c.Assert(err, gc.ErrorMatches, "unauthorized")
}

func (s *jimmUnitTestSuite) TestPrepareModelMigration_InvalidModelTag(c *gc.C) {
	ctx := context.Background()

	jimm := &jimmtest.JIMM{
		JujuManager_: func() jimm.JujuManager {
			return &mocks.JujuManager{}
		},
	}
	root := newTestControllerRoot(jimm, "alice@canonical.com", true)

	_, err := root.PrepareModelMigration(ctx, apiparams.PrepareModelMigrationRequest{
		ModelTag: "blah",
	})

	c.Assert(err, gc.ErrorMatches, "invalid model tag")
}

func (s *jimmUnitTestSuite) TestPrepareModelMigration_InvalidControllerName(c *gc.C) {
	ctx := context.Background()

	jimm := &jimmtest.JIMM{
		JujuManager_: func() jimm.JujuManager {
			return &mocks.JujuManager{}
		},
	}
	root := newTestControllerRoot(jimm, "alice@canonical.com", true)

	_, err := root.PrepareModelMigration(ctx, apiparams.PrepareModelMigrationRequest{
		ModelTag:              names.NewModelTag("5650ac3f-8332-437f-874f-089e0e447e7f").String(),
		BackingControllerName: "---bad wolf---",
	})

	c.Assert(err, gc.ErrorMatches, "invalid controller name")
}

func (s *jimmUnitTestSuite) TestPrepareModelMigration_EmptyExternalUser(c *gc.C) {
	ctx := context.Background()

	jimm := &jimmtest.JIMM{
		JujuManager_: func() jimm.JujuManager {
			return &mocks.JujuManager{
				PrepareModelMigration_: func(ctx context.Context, user *openfga.User, modelUUID, targetControllerName string, userMapping map[string]string) (string, error) {
					return "foo", nil
				},
			}
		},
	}
	root := newTestControllerRoot(jimm, "alice@canonical.com", true)

	_, err := root.PrepareModelMigration(ctx, apiparams.PrepareModelMigrationRequest{
		ModelTag:              names.NewModelTag("5650ac3f-8332-437f-874f-089e0e447e7f").String(),
		BackingControllerName: "test-controller",
		UserMapping:           map[string]string{"alice": "alice@canonical.com", "skipped-local-user": ""},
	})

	c.Assert(err, gc.IsNil)
}

func (s *jimmUnitTestSuite) TestPrepareModelMigration_InvalidUserMapping(c *gc.C) {
	ctx := context.Background()

	jimm := &jimmtest.JIMM{
		JujuManager_: func() jimm.JujuManager {
			return &mocks.JujuManager{}
		},
	}
	root := newTestControllerRoot(jimm, "alice@canonical.com", true)

	_, err := root.PrepareModelMigration(ctx, apiparams.PrepareModelMigrationRequest{
		ModelTag:              names.NewModelTag("5650ac3f-8332-437f-874f-089e0e447e7f").String(),
		BackingControllerName: "controller",
		UserMapping:           map[string]string{"--bad local--": "alice@canonical.com"},
	})

	c.Assert(err, gc.ErrorMatches, `--bad local-- is not a valid local user name`)

	_, err = root.PrepareModelMigration(ctx, apiparams.PrepareModelMigrationRequest{
		ModelTag:              names.NewModelTag("5650ac3f-8332-437f-874f-089e0e447e7f").String(),
		BackingControllerName: "controller",
		UserMapping:           map[string]string{"alice": "alice"},
	})

	c.Assert(err, gc.ErrorMatches, `alice is not a valid external user name`)

	_, err = root.PrepareModelMigration(ctx, apiparams.PrepareModelMigrationRequest{
		ModelTag:              names.NewModelTag("5650ac3f-8332-437f-874f-089e0e447e7f").String(),
		BackingControllerName: "controller",
		UserMapping:           map[string]string{"alice": "--badwolf--@canonical.com"},
	})

	c.Assert(err, gc.ErrorMatches, `--badwolf--@canonical.com is not a valid external user name`)
}

func (s *jimmUnitTestSuite) TestBootstrapStatus(c *gc.C) {
	ctx := context.Background()
	uuidGenerated := uuid.New()
	jimm := &jimmtest.JIMM{
		BootstapManager_: func() jimm.BootstrapManager {
			return &mocks.BootstapManager{
				GetBootstrapStatusAndLogs_: func(ctx context.Context, user *openfga.User, jobId uuid.UUID, offset int) (params.BootstrapStatusResponse, error) {
					if jobId != uuidGenerated {
						return params.BootstrapStatusResponse{}, errors.E(errors.CodeNotFound, "job not found")
					}
					return params.BootstrapStatusResponse{
						Status: "running",
						Logs:   []string{"bootstrap logs"},
					}, nil
				},
			}
		},
	}
	root := newTestControllerRoot(jimm, "alice@canonical.com", true)

	response, err := root.BootstrapStatus(ctx, params.BootstrapStatusRequest{
		JobID:     uuidGenerated.String(),
		Watermark: 0,
	})

	c.Assert(err, gc.IsNil)
	c.Assert(response.Status, gc.Equals, params.StatusRunning)
	c.Assert(response.Logs, gc.DeepEquals, []string{"bootstrap logs"})

	// Test job not found
	_, err = root.BootstrapStatus(ctx, params.BootstrapStatusRequest{
		JobID:     uuid.New().String(),
		Watermark: 0,
	})
	c.Assert(errors.ErrorCode(err), gc.Equals, errors.CodeNotFound)

	// Test unauthorized user
	root = newTestControllerRoot(jimm, "alice@canonical.com", false)
	_, err = root.BootstrapStatus(ctx, params.BootstrapStatusRequest{
		JobID:     uuidGenerated.String(),
		Watermark: 0,
	})
	c.Assert(errors.ErrorCode(err), gc.Equals, errors.CodeUnauthorized)
}

func (s *jimmUnitTestSuite) TestBootstrapStart(c *gc.C) {
	ctx := context.Background()
	var startBootstrapErr error

	jimm := &jimmtest.JIMM{
		BootstapManager_: func() jimm.BootstrapManager {
			return &mocks.BootstapManager{
				StartBootstrap_: func(ctx context.Context, user *openfga.User, params bootstrap.BootstrapParams) (string, error) {
					if startBootstrapErr != nil {
						return "", startBootstrapErr
					}
					return uuid.New().String(), nil
				},
			}
		},
	}
	root := newTestControllerRoot(jimm, "alice@canonical.com", true)

	params := params.BootstrapStartParams{
		ControllerName: "controller",
		CloudName:      "cloud",
		RegionName:     "region",
		Flags: params.BootstrapFlags{
			AgentVersion: "1.0.0",
			Timeout:      3600,
		},
		Cloud:             jujuparams.Cloud{},
		Credential:        jujucloud.CloudCredential{},
		ControllerVersion: "3.6.8",
	}

	response, err := root.BootstrapStart(ctx, params)
	c.Assert(err, gc.IsNil)
	c.Assert(response.JobID, gc.Not(gc.Equals), "")

	// Test start bootstrap fails
	startBootstrapErr = errors.E("foo")
	_, err = root.BootstrapStart(ctx, params)
	c.Assert(err, gc.NotNil)
	c.Assert(err.Error(), gc.Matches, "failed to start bootstrap job: foo")

	startBootstrapErr = nil
	// Test unauthorized user
	root = newTestControllerRoot(jimm, "alice@canonical.com", false)
	_, err = root.BootstrapStart(ctx, params)
	c.Assert(errors.ErrorCode(err), gc.Equals, errors.CodeUnauthorized)
}

func (s *jimmUnitTestSuite) TestBootstrapStop(c *gc.C) {
	ctx := context.Background()
	uuidGenerated := uuid.New()

	jimm := &jimmtest.JIMM{
		BootstapManager_: func() jimm.BootstrapManager {
			return &mocks.BootstapManager{
				StopBootstrap_: func(ctx context.Context, user *openfga.User, jobId uuid.UUID) error {
					if jobId != uuidGenerated {
						return errors.E(errors.CodeNotFound, "job not found")
					}
					return nil
				},
			}
		},
	}
	root := newTestControllerRoot(jimm, "alice@canonical.com", false)
	// Test stop bootstrap job unauthorized user
	err := root.BootstrapStop(ctx, params.BootstrapStopRequest{
		JobID: uuidGenerated.String(),
	})
	c.Assert(errors.ErrorCode(err), gc.Equals, errors.CodeUnauthorized)

	// Test stop bootstrap job
	root = newTestControllerRoot(jimm, "alice@canonical.com", true)
	err = root.BootstrapStop(ctx, params.BootstrapStopRequest{
		JobID: uuidGenerated.String(),
	})
	c.Assert(err, gc.IsNil)

	// Test job not found
	err = root.BootstrapStop(ctx, params.BootstrapStopRequest{
		JobID: uuid.New().String(),
	})
	c.Assert(err, gc.ErrorMatches, ".*job not found.*")

	// Test stop bootstrap not valid job ID
	err = root.BootstrapStop(ctx, params.BootstrapStopRequest{
		JobID: "not-a-valid-uuid",
	})
	c.Assert(err, gc.NotNil)
	c.Assert(err.Error(), gc.Matches, "invalid job ID")
}
