// Copyright 2025 Canonical.

package jujuapi_test

import (
	"context"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
	"github.com/google/uuid"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v5"
	gc "gopkg.in/check.v1"

	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/jimm"
	"github.com/canonical/jimm/v3/internal/jimm/bootstrap"
	"github.com/canonical/jimm/v3/internal/jimm/juju"
	"github.com/canonical/jimm/v3/internal/jujuapi"
	"github.com/canonical/jimm/v3/internal/openfga"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest/mocks"
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
				GetJobInfo_: func(ctx context.Context, user *openfga.User, jobId uuid.UUID, offset int) (apiparams.GetJobInfoResponse, error) {
					if jobId != uuidGenerated {
						return apiparams.GetJobInfoResponse{}, errors.E(errors.CodeNotFound, "job not found")
					}
					return apiparams.GetJobInfoResponse{
						Status: "running",
						Logs:   []string{"bootstrap logs"},
					}, nil
				},
			}
		},
	}
	root := newTestControllerRoot(jimm, "alice@canonical.com", true)

	response, err := root.GetJobInfo(ctx, apiparams.GetJobInfoRequest{
		JobID:     uuidGenerated.String(),
		Watermark: 0,
	})

	c.Assert(err, gc.IsNil)
	c.Assert(response.Status, gc.Equals, apiparams.StatusRunning)
	c.Assert(response.Logs, gc.DeepEquals, []string{"bootstrap logs"})

	// Test job not found
	_, err = root.GetJobInfo(ctx, apiparams.GetJobInfoRequest{
		JobID:     uuid.New().String(),
		Watermark: 0,
	})
	c.Assert(errors.ErrorCode(err), gc.Equals, errors.CodeNotFound)

	// Test unauthorized user
	root = newTestControllerRoot(jimm, "alice@canonical.com", false)
	_, err = root.GetJobInfo(ctx, apiparams.GetJobInfoRequest{
		JobID:     uuidGenerated.String(),
		Watermark: 0,
	})
	c.Assert(errors.ErrorCode(err), gc.Equals, errors.CodeUnauthorized)
}

func (s *jimmUnitTestSuite) TestBootstrapStart_RejectsBuiltinClouds(c *gc.C) {
	ctx := context.Background()

	jimm := &jimmtest.JIMM{}
	root := newTestControllerRoot(jimm, "alice@canonical.com", true)

	params := apiparams.BootstrapParams{
		CloudName: "localhost",
	}

	_, err := root.StartBootstrapJob(ctx, params)
	c.Assert(err, gc.ErrorMatches, `.*bootstrap via JIMM does not support built-in clouds like "localhost"`)
}

func (s *jimmUnitTestSuite) TestBootstrapStart(c *gc.C) {
	ctx := context.Background()
	var startBootstrapErr error

	jimm := &jimmtest.JIMM{
		BootstapManager_: func() jimm.BootstrapManager {
			return &mocks.BootstapManager{
				StartBootstrapJob_: func(ctx context.Context, user *openfga.User, params bootstrap.BootstrapParams) (string, error) {
					if startBootstrapErr != nil {
						return "", startBootstrapErr
					}
					return uuid.New().String(), nil
				},
			}
		},
	}
	root := newTestControllerRoot(jimm, "alice@canonical.com", true)

	params := apiparams.BootstrapParams{
		ControllerName:    "controller",
		CloudName:         "cloud",
		RegionName:        "region",
		Config:            map[string]string{},
		Cloud:             jujuparams.Cloud{},
		Credential:        jujuparams.CloudCredential{},
		ControllerVersion: "3.6.9",
	}

	response, err := root.StartBootstrapJob(ctx, params)
	c.Assert(err, gc.IsNil)
	c.Assert(response.JobID, gc.Not(gc.Equals), "")

	// Test start bootstrap fails
	startBootstrapErr = errors.E("foo")
	_, err = root.StartBootstrapJob(ctx, params)
	c.Assert(err, gc.NotNil)
	c.Assert(err.Error(), gc.Matches, "failed to start bootstrap job: foo")

	startBootstrapErr = nil
	// Test unauthorized user
	root = newTestControllerRoot(jimm, "alice@canonical.com", false)
	_, err = root.StartBootstrapJob(ctx, params)
	c.Assert(errors.ErrorCode(err), gc.Equals, errors.CodeUnauthorized)
}

func (s *jimmUnitTestSuite) TestBootstrapStop(c *gc.C) {
	ctx := context.Background()
	uuidGenerated := uuid.New()

	jimm := &jimmtest.JIMM{
		BootstapManager_: func() jimm.BootstrapManager {
			return &mocks.BootstapManager{
				StopJob_: func(ctx context.Context, user *openfga.User, jobId uuid.UUID) error {
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
	err := root.StopJob(ctx, apiparams.StopJobRequest{
		JobID: uuidGenerated.String(),
	})
	c.Assert(errors.ErrorCode(err), gc.Equals, errors.CodeUnauthorized)

	// Test stop bootstrap job
	root = newTestControllerRoot(jimm, "alice@canonical.com", true)
	err = root.StopJob(ctx, apiparams.StopJobRequest{
		JobID: uuidGenerated.String(),
	})
	c.Assert(err, gc.IsNil)

	// Test job not found
	err = root.StopJob(ctx, apiparams.StopJobRequest{
		JobID: uuid.New().String(),
	})
	c.Assert(err, gc.ErrorMatches, ".*job not found.*")

	// Test stop bootstrap not valid job ID
	err = root.StopJob(ctx, apiparams.StopJobRequest{
		JobID: "not-a-valid-uuid",
	})
	c.Assert(err, gc.NotNil)
	c.Assert(err.Error(), gc.Matches, "invalid job ID")
}

func (s *jimmUnitTestSuite) TestStartDestroyControllerJob(c *gc.C) {
	ctx := context.Background()

	ctrlInfo := &dbmodel.Controller{
		Name:          "moribund",
		CloudName:     jimmtest.TestCloudName,
		CloudRegion:   jimmtest.TestCloudRegionName,
		UUID:          "not-a-uuid",
		AgentVersion:  "not-a-version",
		PublicAddress: "not-an-address",
		CACertificate: "not-even-close",
		Models:        []dbmodel.Model{},
	}

	jimm := &jimmtest.JIMM{
		BootstapManager_: func() jimm.BootstrapManager {
			return &mocks.BootstapManager{
				StartDestroyControllerJob_: func(ctx context.Context, user *openfga.User, params bootstrap.DestroyControllerParams) (string, error) {
					return uuid.New().String(), nil
				},
			}
		},
		JujuManager_: func() jimm.JujuManager {
			return &mocks.JujuManager{
				ControllerService: mocks.ControllerService{
					ControllerInfo_: func(ctx context.Context, name string) (*dbmodel.Controller, error) {
						return ctrlInfo, nil
					},
				},
			}
		},
	}
	root := newTestControllerRoot(jimm, "alice@canonical.com", true)
	req := apiparams.DestroyControllerRequest{}

	// OK to destroy controller without models
	_, err := root.StartDestroyControllerJob(ctx, req)
	c.Assert(err, gc.IsNil)

	// Refuse to destroy controller with models
	ctrlInfo.Models = append(ctrlInfo.Models, dbmodel.Model{})
	_, err = root.StartDestroyControllerJob(ctx, req)
	c.Assert(err, gc.ErrorMatches, "cannot destroy controller with models")
}

func (s *jimmUnitTestSuite) TestAddModelToController(c *gc.C) {
	ctx := context.Background()

	jimm := &jimmtest.JIMM{
		JujuManager_: func() jimm.JujuManager {
			return &mocks.JujuManager{
				ModelManager: mocks.ModelManager{
					AddModel_: func(ctx context.Context, u *openfga.User, args *juju.ModelCreateArgs) (*jujuparams.ModelInfo, error) {
						c.Check(args.Name, gc.Equals, "mymodel")
						c.Check(args.Owner.String(), gc.Equals, "user-alice@canonical.com")
						c.Check(args.Cloud.String(), gc.Equals, "cloud-openstack")
						c.Check(args.CloudRegion, gc.Equals, "region-1")
						c.Check(args.CloudCredential.String(), gc.Equals, "cloudcred-openstack_alice_mycred")
						c.Check(args.ControllerName, gc.Equals, "controller-1")
						return &jujuparams.ModelInfo{}, nil
					},
				},
			}
		},
	}

	root := newTestControllerRoot(jimm, "alice@canonical.com", true)
	req := apiparams.AddModelToControllerRequest{
		ModelCreateArgs: jujuparams.ModelCreateArgs{
			Name:               "mymodel",
			OwnerTag:           "user-alice@canonical.com",
			CloudTag:           "cloud-openstack",
			CloudRegion:        "region-1",
			CloudCredentialTag: "cloudcred-openstack/alice/mycred",
		},
		ControllerName: "controller-1",
	}

	_, err := root.AddModelToController(ctx, req)
	c.Assert(err, gc.IsNil)
}

// TestAuditLogAPIParamsConversion tests the conversion of API params to a AuditLogFilter struct.
// Note that this test doesn't require a running Juju/JIMM controller so it doesn't use gc + the jimmSuite.
func TestAuditLogAPIParamsConversion(t *testing.T) {
	c := qt.New(t)
	testCases := []struct {
		about   string
		request apiparams.FindAuditEventsRequest
		result  db.AuditLogFilter
		err     error
	}{
		{
			about: "Test basic conversion",
			request: apiparams.FindAuditEventsRequest{
				After:    "2023-08-14T00:00:00Z",
				Before:   "2023-08-14T00:00:00Z",
				UserTag:  "user-alice",
				Model:    "123",
				Method:   "Deploy",
				Offset:   10,
				Limit:    10,
				SortTime: false,
			},
			result: db.AuditLogFilter{
				Start:       time.Date(2023, 8, 14, 0, 0, 0, 0, time.UTC),
				End:         time.Date(2023, 8, 14, 0, 0, 0, 0, time.UTC),
				IdentityTag: "user-alice",
				Model:       "123",
				Method:      "Deploy",
				Offset:      10,
				Limit:       10,
				SortTime:    false,
			},
		}, {
			about: "Test limit lower bound",
			request: apiparams.FindAuditEventsRequest{
				Limit: 0,
			},
			result: db.AuditLogFilter{
				Limit: jujuapi.AuditLogDefaultLimit,
			},
		}, {
			about: "Test limit upper bound",
			request: apiparams.FindAuditEventsRequest{
				Limit: jujuapi.AuditLogUpperLimit + 1,
			},
			result: db.AuditLogFilter{
				Limit: jujuapi.AuditLogUpperLimit,
			},
		},
	}
	for _, test := range testCases {
		c.Log(test.about)
		res, err := jujuapi.AuditParamsToFilter(test.request)
		if test.err == nil {
			c.Assert(err, qt.IsNil)
			c.Assert(res, qt.DeepEquals, test.result)
		} else {
			c.Assert(err, qt.ErrorMatches, test.err)
		}
	}
}
