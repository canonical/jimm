// Copyright 2025 Canonical.

package jujuapi_test

import (
	"context"
	"strconv"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
	"github.com/juju/juju/api/base"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v5"

	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/jimm/bootstrap"
	"github.com/canonical/jimm/v3/internal/jimm/jobs"
	"github.com/canonical/jimm/v3/internal/jimm/juju"
	"github.com/canonical/jimm/v3/internal/jujuapi"
	"github.com/canonical/jimm/v3/internal/openfga"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest/mocks"
	apiparams "github.com/canonical/jimm/v3/pkg/api/params"
)

func TestPrepareModelMigration_UnauthorizedUser(t *testing.T) {
	c := qt.New(t)

	ctx := c.Context()
	jimm := &jimmtest.JIMM{
		JujuManager_: func() jujuapi.JujuManager {
			return &mocks.JujuManager{}
		},
	}
	root := newTestControllerRoot(jimm, "alice@canonical.com", false)

	_, err := root.PrepareModelMigration(ctx, apiparams.PrepareModelMigrationRequest{})

	c.Assert(err, qt.ErrorMatches, "unauthorized")
}

func TestAddController_UnauthorizedUser(t *testing.T) {
	c := qt.New(t)

	ctx := c.Context()
	jimm := &jimmtest.JIMM{
		JujuManager_: func() jujuapi.JujuManager {
			return &mocks.JujuManager{}
		},
	}
	root := newTestControllerRoot(jimm, "alice@canonical.com", false)

	_, err := root.AddController(ctx, apiparams.AddControllerRequest{})

	c.Assert(errors.ErrorCode(err), qt.Equals, errors.CodeUnauthorized)
	c.Assert(err, qt.ErrorMatches, "unauthorized")
}

func TestAddController_Success(t *testing.T) {
	c := qt.New(t)

	ctx := c.Context()

	called := false
	jimm := &jimmtest.JIMM{
		JujuManager_: func() jujuapi.JujuManager {
			return &mocks.JujuManager{
				ControllerService: mocks.ControllerService{
					AddController_: func(ctx context.Context, user *openfga.User, ctl *dbmodel.Controller, creds juju.ControllerCreds) error {
						called = true
						c.Assert(user.JimmAdmin, qt.Equals, true)
						c.Assert(ctl.Name, qt.Equals, "controller-2")
						c.Assert(ctl.UUID, qt.Equals, "982b16d9-a945-4762-b684-fd4fd885aa11")
						c.Assert(ctl.PublicAddress, qt.Equals, "controller.example.com:443")
						c.Assert(ctl.CACertificate, qt.Equals, "ca-cert")
						c.Assert(ctl.TLSHostname, qt.Equals, "juju-apiserver")
						c.Assert(creds.AdminIdentityName, qt.Equals, "admin")
						c.Assert(creds.AdminPassword, qt.Equals, "super-secret")

						// Simulate the JujuManager filling in extra data during the add.
						ctl.CloudName = "aws"
						ctl.CloudRegion = "eu-west-1"
						ctl.AgentVersion = "3.6.9"
						return nil
					},
				},
			}
		},
	}
	root := newTestControllerRoot(jimm, "alice@canonical.com", true)

	req := apiparams.AddControllerRequest{
		UUID:          "982b16d9-a945-4762-b684-fd4fd885aa11",
		Name:          "controller-2",
		PublicAddress: "controller.example.com:443",
		TLSHostname:   "juju-apiserver",
		APIAddresses:  []string{"127.0.0.1:17070"},
		CACertificate: "ca-cert",
		Username:      "admin",
		Password:      "super-secret",
	}

	info, err := root.AddController(ctx, req)
	c.Assert(err, qt.IsNil)
	c.Assert(called, qt.Equals, true)
	c.Assert(info.Name, qt.Equals, req.Name)
	c.Assert(info.UUID, qt.Equals, req.UUID)
	c.Assert(info.PublicAddress, qt.Equals, req.PublicAddress)
	c.Assert(info.CACertificate, qt.Equals, req.CACertificate)
	c.Assert(info.APIAddresses, qt.DeepEquals, req.APIAddresses)
	c.Assert(info.CloudTag, qt.Equals, names.NewCloudTag("aws").String())
	c.Assert(info.CloudRegion, qt.Equals, "eu-west-1")
	c.Assert(info.AgentVersion, qt.Equals, "3.6.9")
}

func TestListControllers_AdminIncludesPendingBootstraps(t *testing.T) {
	c := qt.New(t)

	ctx := c.Context()
	jimm := &jimmtest.JIMM{
		JujuManager_: func() jujuapi.JujuManager {
			return &mocks.JujuManager{
				ControllerService: mocks.ControllerService{
					ListControllers_: func(ctx context.Context, user *openfga.User) ([]dbmodel.Controller, error) {
						c.Assert(user.JimmAdmin, qt.Equals, true)
						return []dbmodel.Controller{{
							Name:      "active-controller",
							UUID:      "982b16d9-a945-4762-b684-fd4fd885aa11",
							CloudName: "aws",
						}}, nil
					},
					ListControllerBootstraps_: func(ctx context.Context) ([]dbmodel.ControllerBootstrap, error) {
						return []dbmodel.ControllerBootstrap{{
							Name:        "bootstrapping-controller",
							CloudName:   "aws",
							CloudRegion: "eu-west-1",
						}}, nil
					},
				},
			}
		},
	}
	root := newTestControllerRoot(jimm, "alice@canonical.com", true)

	resp, err := root.ListControllers(ctx)
	c.Assert(err, qt.IsNil)
	c.Assert(resp.Controllers, qt.DeepEquals, []apiparams.ControllerInfo{{
		Name:     "active-controller",
		UUID:     "982b16d9-a945-4762-b684-fd4fd885aa11",
		CloudTag: names.NewCloudTag("aws").String(),
		Status:   jujuparams.EntityStatus{Status: "available"},
	}, {
		Name:        "bootstrapping-controller",
		CloudTag:    names.NewCloudTag("aws").String(),
		CloudRegion: "eu-west-1",
		Status:      jujuparams.EntityStatus{Status: "bootstrapping"},
	}})
}

func TestRemoveController_UnauthorizedUser(t *testing.T) {
	c := qt.New(t)

	ctx := c.Context()
	jimm := &jimmtest.JIMM{}
	root := newTestControllerRoot(jimm, "alice@canonical.com", false)

	_, err := root.RemoveController(ctx, apiparams.RemoveControllerRequest{})

	c.Assert(errors.ErrorCode(err), qt.Equals, errors.CodeUnauthorized)
	c.Assert(err, qt.ErrorMatches, "unauthorized")
}

func TestRemoveController_Success(t *testing.T) {
	c := qt.New(t)

	ctx := c.Context()

	removeCalled := false
	infoCalled := false
	testControllerName := "test-controller"
	jimm := &jimmtest.JIMM{

		JujuManager_: func() jujuapi.JujuManager {
			return &mocks.JujuManager{
				ControllerService: mocks.ControllerService{
					ControllerInfo_: func(ctx context.Context, name string) (*dbmodel.Controller, error) {
						infoCalled = true
						return &dbmodel.Controller{
							Name:          testControllerName,
							UUID:          "982b16d9-a945-4762-b684-fd4fd885aa11",
							PublicAddress: "controller.example.com:443",
							CACertificate: "ca-cert",
							TLSHostname:   "juju-apiserver",
							CloudName:     "openstack",
						}, nil
					},
					RemoveController_: func(ctx context.Context, user *openfga.User, controllerName string, force bool) error {
						removeCalled = true
						c.Check(controllerName, qt.Equals, testControllerName)
						c.Check(user.JimmAdmin, qt.Equals, true)
						return nil
					},
				},
			}
		},
	}
	root := newTestControllerRoot(jimm, "alice@canonical.com", true)

	req := apiparams.RemoveControllerRequest{
		Name: testControllerName,
	}

	info, err := root.RemoveController(ctx, req)
	c.Assert(err, qt.IsNil)
	c.Assert(removeCalled, qt.Equals, true)
	c.Assert(infoCalled, qt.Equals, true)
	c.Assert(info.Name, qt.Equals, req.Name)
	c.Assert(info.UUID, qt.Equals, "982b16d9-a945-4762-b684-fd4fd885aa11")
	c.Assert(info.PublicAddress, qt.Equals, "controller.example.com:443")
	c.Assert(info.CACertificate, qt.Equals, "ca-cert")
	c.Assert(info.CloudTag, qt.Equals, names.NewCloudTag("openstack").String())
}

func TestPrepareModelMigration_InvalidModelTag(t *testing.T) {
	c := qt.New(t)

	ctx := c.Context()

	jimm := &jimmtest.JIMM{
		JujuManager_: func() jujuapi.JujuManager {
			return &mocks.JujuManager{}
		},
	}
	root := newTestControllerRoot(jimm, "alice@canonical.com", true)

	_, err := root.PrepareModelMigration(ctx, apiparams.PrepareModelMigrationRequest{
		ModelTag: "blah",
	})

	c.Assert(err, qt.ErrorMatches, `invalid model tag: "blah" is not a valid tag`)
}

func TestPrepareModelMigration_InvalidControllerName(t *testing.T) {
	c := qt.New(t)

	ctx := c.Context()

	jimm := &jimmtest.JIMM{
		JujuManager_: func() jujuapi.JujuManager {
			return &mocks.JujuManager{}
		},
	}
	root := newTestControllerRoot(jimm, "alice@canonical.com", true)

	_, err := root.PrepareModelMigration(ctx, apiparams.PrepareModelMigrationRequest{
		ModelTag:              names.NewModelTag("5650ac3f-8332-437f-874f-089e0e447e7f").String(),
		BackingControllerName: "---bad wolf---",
	})

	c.Assert(err, qt.ErrorMatches, "invalid controller name")
}

func TestPrepareModelMigration_EmptyExternalUser(t *testing.T) {
	c := qt.New(t)

	ctx := c.Context()

	jimm := &jimmtest.JIMM{
		JujuManager_: func() jujuapi.JujuManager {
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

	c.Assert(err, qt.IsNil)
}

func TestPrepareModelMigration_InvalidUserMapping(t *testing.T) {
	c := qt.New(t)

	ctx := c.Context()

	jimm := &jimmtest.JIMM{
		JujuManager_: func() jujuapi.JujuManager {
			return &mocks.JujuManager{}
		},
	}
	root := newTestControllerRoot(jimm, "alice@canonical.com", true)

	_, err := root.PrepareModelMigration(ctx, apiparams.PrepareModelMigrationRequest{
		ModelTag:              names.NewModelTag("5650ac3f-8332-437f-874f-089e0e447e7f").String(),
		BackingControllerName: "controller",
		UserMapping:           map[string]string{"--bad local--": "alice@canonical.com"},
	})

	c.Assert(err, qt.ErrorMatches, `--bad local-- is not a valid local user name`)

	_, err = root.PrepareModelMigration(ctx, apiparams.PrepareModelMigrationRequest{
		ModelTag:              names.NewModelTag("5650ac3f-8332-437f-874f-089e0e447e7f").String(),
		BackingControllerName: "controller",
		UserMapping:           map[string]string{"alice": "alice"},
	})

	c.Assert(err, qt.ErrorMatches, `alice is not a valid external user name`)

	_, err = root.PrepareModelMigration(ctx, apiparams.PrepareModelMigrationRequest{
		ModelTag:              names.NewModelTag("5650ac3f-8332-437f-874f-089e0e447e7f").String(),
		BackingControllerName: "controller",
		UserMapping:           map[string]string{"alice": "--badwolf--@canonical.com"},
	})

	c.Assert(err, qt.ErrorMatches, `--badwolf--@canonical.com is not a valid external user name`)
}

func TestBootstrapStatus(t *testing.T) {
	c := qt.New(t)

	ctx := c.Context()
	expectedJobID := int64(1)
	jimm := &jimmtest.JIMM{
		BootstapManager_: func() jujuapi.BootstrapManager {
			return &mocks.BootstapManager{
				GetJobInfo_: func(ctx context.Context, user *openfga.User, jobId int64, offset int) (apiparams.GetBootstrapInfoResponse, error) {
					if jobId != expectedJobID {
						return apiparams.GetBootstrapInfoResponse{}, errors.Codef(errors.CodeNotFound, "job not found")
					}
					return apiparams.GetBootstrapInfoResponse{
						Status: "running",
						Logs:   []string{"bootstrap logs"},
					}, nil
				},
			}
		},
	}
	root := newTestControllerRoot(jimm, "alice@canonical.com", true)

	response, err := root.GetBootstrapInfo(ctx, apiparams.GetBootstrapInfoRequest{
		JobID:     strconv.FormatInt(expectedJobID, 10),
		Watermark: 0,
	})

	c.Assert(err, qt.IsNil)
	c.Assert(response.Status, qt.Equals, apiparams.StatusRunning)
	c.Assert(response.Logs, qt.DeepEquals, []string{"bootstrap logs"})

	// Test job not found
	_, err = root.GetBootstrapInfo(ctx, apiparams.GetBootstrapInfoRequest{
		JobID:     "999",
		Watermark: 0,
	})
	c.Assert(errors.ErrorCode(err), qt.Equals, errors.CodeNotFound)

	// Test unauthorized user
	root = newTestControllerRoot(jimm, "alice@canonical.com", false)
	_, err = root.GetBootstrapInfo(ctx, apiparams.GetBootstrapInfoRequest{
		JobID:     strconv.FormatInt(expectedJobID, 10),
		Watermark: 0,
	})
	c.Assert(errors.ErrorCode(err), qt.Equals, errors.CodeUnauthorized)
}

func TestBootstrapStart_RejectsBuiltinClouds(t *testing.T) {
	c := qt.New(t)

	ctx := c.Context()

	jimm := &jimmtest.JIMM{
		BootstapManager_: func() jujuapi.BootstrapManager {
			return &mocks.BootstapManager{}
		},
	}
	root := newTestControllerRoot(jimm, "alice@canonical.com", true)

	params := apiparams.BootstrapParams{
		Cloud: apiparams.BootstrapCloud{
			Name: "localhost",
		},
	}

	_, err := root.StartBootstrap(ctx, params)
	c.Assert(err, qt.ErrorMatches, `.*bootstrap via JIMM does not support built-in clouds like "localhost"`)
}

func TestBootstrapStart(t *testing.T) {
	c := qt.New(t)

	ctx := c.Context()
	var startBootstrapErr error
	var gotParams bootstrap.BootstrapParams

	jimm := &jimmtest.JIMM{
		BootstapManager_: func() jujuapi.BootstrapManager {
			return &mocks.BootstapManager{
				StartBootstrapJob_: func(ctx context.Context, user *openfga.User, params bootstrap.BootstrapParams) (int64, error) {
					gotParams = params
					if startBootstrapErr != nil {
						return 0, startBootstrapErr
					}
					return 1, nil
				},
			}
		},
	}
	root := newTestControllerRoot(jimm, "alice@canonical.com", true)

	params := apiparams.BootstrapParams{
		ControllerName: "controller",
		Cloud: apiparams.BootstrapCloud{
			Name: "cloud",
			Region: apiparams.BootstrapCloudRegion{
				Name: "region",
			},
		},
		BootstrapOptions: apiparams.BootstrapOptions{
			BootstrapBase:        "ubuntu@24.04",
			BootstrapConstraints: map[string]string{"mem": "8G"},
			ModelConstraints:     map[string]string{"arch": "amd64"},
			ModelDefault:         map[string]string{"logging-config": "<root>=INFO"},
			StoragePool: &apiparams.BootstrapStoragePool{
				Name:       "controller-pool",
				Type:       "ebs",
				Attributes: map[string]string{"volume-type": "gp3"},
			},
			BootstrapConfig:       map[string]string{"bootstrap-timeout": "20m"},
			ControllerConfig:      map[string]string{"audit-log-enabled": "true"},
			ControllerModelConfig: map[string]string{"image-stream": "released"},
		},
		Credential:        jujuparams.CloudCredential{},
		ControllerVersion: "3.6.9",
	}

	response, err := root.StartBootstrap(ctx, params)
	c.Assert(err, qt.IsNil)
	c.Assert(response.JobID, qt.Not(qt.Equals), "")
	c.Assert(gotParams.CLIVersion, qt.Equals, params.ControllerVersion)
	c.Assert(gotParams.CloudNameAndRegion, qt.Equals, "cloud/region")
	c.Assert(gotParams.ControllerName, qt.Equals, params.ControllerName)
	c.Assert(gotParams.BootstrapOptions.BootstrapBase, qt.Equals, params.BootstrapOptions.BootstrapBase)
	c.Assert(gotParams.BootstrapOptions.BootstrapConstraints, qt.DeepEquals, params.BootstrapOptions.BootstrapConstraints)
	c.Assert(gotParams.BootstrapOptions.ModelConstraints, qt.DeepEquals, params.BootstrapOptions.ModelConstraints)
	c.Assert(gotParams.BootstrapOptions.ModelDefault, qt.DeepEquals, params.BootstrapOptions.ModelDefault)
	c.Assert(gotParams.BootstrapOptions.BootstrapConfig, qt.DeepEquals, params.BootstrapOptions.BootstrapConfig)
	c.Assert(gotParams.BootstrapOptions.ControllerConfig, qt.DeepEquals, params.BootstrapOptions.ControllerConfig)
	c.Assert(gotParams.BootstrapOptions.ControllerModelConfig, qt.DeepEquals, params.BootstrapOptions.ControllerModelConfig)
	c.Assert(gotParams.BootstrapOptions.StoragePool, qt.DeepEquals, &bootstrap.StoragePool{
		Name:       "controller-pool",
		Type:       "ebs",
		Attributes: map[string]string{"volume-type": "gp3"},
	})

	// Test start bootstrap fails
	startBootstrapErr = errors.New("foo")
	_, err = root.StartBootstrap(ctx, params)
	c.Assert(err, qt.Not(qt.IsNil))
	c.Assert(err.Error(), qt.Matches, "failed to start bootstrap job: foo")

	startBootstrapErr = nil
	// Test unauthorized user
	root = newTestControllerRoot(jimm, "alice@canonical.com", false)
	_, err = root.StartBootstrap(ctx, params)
	c.Assert(errors.ErrorCode(err), qt.Equals, errors.CodeUnauthorized)
}

func TestBootstrapStop(t *testing.T) {
	c := qt.New(t)

	ctx := c.Context()
	expectedJobID := int64(1)

	jimm := &jimmtest.JIMM{
		BootstapManager_: func() jujuapi.BootstrapManager {
			return &mocks.BootstapManager{
				StopJob_: func(ctx context.Context, user *openfga.User, jobId int64) error {
					if jobId != expectedJobID {
						return errors.Codef(errors.CodeNotFound, "job not found")
					}
					return nil
				},
			}
		},
	}
	root := newTestControllerRoot(jimm, "alice@canonical.com", false)
	// Test stop bootstrap job unauthorized user
	err := root.StopBootstrap(ctx, apiparams.StopBootstrapRequest{
		JobID: strconv.FormatInt(expectedJobID, 10),
	})
	c.Assert(errors.ErrorCode(err), qt.Equals, errors.CodeUnauthorized)

	// Test stop bootstrap job
	root = newTestControllerRoot(jimm, "alice@canonical.com", true)
	err = root.StopBootstrap(ctx, apiparams.StopBootstrapRequest{
		JobID: strconv.FormatInt(expectedJobID, 10),
	})
	c.Assert(err, qt.IsNil)

	// Test job not found
	err = root.StopBootstrap(ctx, apiparams.StopBootstrapRequest{
		JobID: "999",
	})
	c.Assert(err, qt.ErrorMatches, ".*job not found.*")

	// Test stop bootstrap not valid job ID
	err = root.StopBootstrap(ctx, apiparams.StopBootstrapRequest{
		JobID: "random-string",
	})
	c.Assert(err, qt.Not(qt.IsNil))
	c.Assert(err.Error(), qt.Matches, "invalid job ID.*")
}

func TestStartDestroyControllerJob(t *testing.T) {
	c := qt.New(t)

	ctx := c.Context()

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
		BootstapManager_: func() jujuapi.BootstrapManager {
			return &mocks.BootstapManager{
				StartDestroyControllerJob_: func(ctx context.Context, user *openfga.User, params bootstrap.DestroyControllerParams) (int64, error) {
					return 1, nil
				},
			}
		},
		JujuManager_: func() jujuapi.JujuManager {
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
	_, err := root.StartDestroyController(ctx, req)
	c.Assert(err, qt.IsNil)

	// Refuse to destroy controller with models
	ctrlInfo.Models = append(ctrlInfo.Models, dbmodel.Model{})
	_, err = root.StartDestroyController(ctx, req)
	c.Assert(err, qt.ErrorMatches, "cannot destroy controller with models")
}

func TestAddModelToController(t *testing.T) {
	c := qt.New(t)

	ctx := c.Context()

	jimm := &jimmtest.JIMM{
		JujuManager_: func() jujuapi.JujuManager {
			return &mocks.JujuManager{
				ModelManager: mocks.ModelManager{
					AddModel_: func(ctx context.Context, u *openfga.User, args *juju.ModelCreateArgs) (base.ModelInfo, error) {
						c.Check(args.Name, qt.Equals, "mymodel")
						c.Check(args.Owner.String(), qt.Equals, "user-alice@canonical.com")
						c.Check(args.Cloud.String(), qt.Equals, "cloud-openstack")
						c.Check(args.CloudRegion, qt.Equals, "region-1")
						c.Check(args.CloudCredential.String(), qt.Equals, "cloudcred-openstack_alice_mycred")
						c.Check(args.ControllerName, qt.Equals, "controller-1")
						return base.ModelInfo{
							Cloud:           "openstack",
							Owner:           "alice@canonical.com",
							CloudCredential: "openstack/alice@canonical.com/mycred",
						}, nil
					},
				},
			}
		},
	}

	root := newTestControllerRoot(jimm, "alice@canonical.com", true)
	// #nosec G101 No secrets in ModelCreateArgs.
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
	c.Assert(err, qt.IsNil)
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

func TestModelControllerInfo(t *testing.T) {
	c := qt.New(t)

	ctx := c.Context()

	testCases := []struct {
		about          string
		admin          bool
		modelQualifier string
		jujuManager    func(*qt.C) jujuapi.JujuManager
		expectedInfo   apiparams.ModelControllerInfo
		expectedError  string
	}{{
		about:          "non-admin user with model uuid",
		admin:          false,
		modelQualifier: "12345678-1234-1234-1234-123456789abc",
		jujuManager: func(c *qt.C) jujuapi.JujuManager {
			return &mocks.JujuManager{}
		},
		expectedError: "unauthorized",
	}, {
		about:          "invalid model uuid",
		admin:          true,
		modelQualifier: "invalid-model-uuid",
		jujuManager: func(c *qt.C) jujuapi.JujuManager {
			return &mocks.JujuManager{}
		},
		expectedError: `invalid model UUID: invalid UUID length: 18`,
	}, {
		about:          "model not found with model tag",
		admin:          true,
		modelQualifier: "12345678-1234-1234-1234-123456789abc",
		jujuManager: func(c *qt.C) jujuapi.JujuManager {
			return &mocks.JujuManager{
				ModelControllerInfo_: func(ctx context.Context, user *openfga.User, qualifier juju.ModelControllerInfoQualifier) (*apiparams.ModelControllerInfo, error) {
					return nil, errors.New("model not found")
				},
			}
		},
		expectedError: "model not found",
	}, {
		about:          "success with model tag",
		admin:          true,
		modelQualifier: "12345678-1234-1234-1234-123456789abc",
		jujuManager: func(c *qt.C) jujuapi.JujuManager {
			return &mocks.JujuManager{
				ModelControllerInfo_: func(ctx context.Context, user *openfga.User, qualifier juju.ModelControllerInfoQualifier) (*apiparams.ModelControllerInfo, error) {
					c.Assert(user.JimmAdmin, qt.Equals, true)
					return &apiparams.ModelControllerInfo{
						ModelName:      "test-model",
						ModelUUID:      "12345678-1234-1234-1234-123456789abc",
						ControllerName: "test-controller",
						ControllerUUID: "87654321-4321-4321-4321-cba987654321",
					}, nil
				},
			}
		},
		expectedInfo: apiparams.ModelControllerInfo{
			ModelName:      "test-model",
			ModelUUID:      "12345678-1234-1234-1234-123456789abc",
			ControllerName: "test-controller",
			ControllerUUID: "87654321-4321-4321-4321-cba987654321",
		},
	}, {
		about:          "success with owner and model name",
		admin:          true,
		modelQualifier: "alice@canonical.com/test-model",
		jujuManager: func(c *qt.C) jujuapi.JujuManager {
			return &mocks.JujuManager{
				ModelControllerInfo_: func(ctx context.Context, user *openfga.User, qualifier juju.ModelControllerInfoQualifier) (*apiparams.ModelControllerInfo, error) {
					c.Assert(user.JimmAdmin, qt.Equals, true)
					return &apiparams.ModelControllerInfo{
						ModelName:      "test-model",
						ModelUUID:      "12345678-1234-1234-1234-123456789abc",
						ControllerName: "test-controller",
						ControllerUUID: "87654321-4321-4321-4321-cba987654321",
					}, nil
				},
			}
		},
		expectedInfo: apiparams.ModelControllerInfo{
			ModelName:      "test-model",
			ModelUUID:      "12345678-1234-1234-1234-123456789abc",
			ControllerName: "test-controller",
			ControllerUUID: "87654321-4321-4321-4321-cba987654321",
		},
	}, {
		about:          "error with a partial model qualifier (only owner provided)",
		admin:          true,
		modelQualifier: "alice@canonical.com/",
		jujuManager:    func(c *qt.C) jujuapi.JujuManager { return &mocks.JujuManager{} },
		expectedError:  `invalid model UUID: invalid UUID length: 20`,
	}, {
		about:          "error with a partial model qualifier (only model name provided)",
		admin:          true,
		modelQualifier: "/test-model",
		jujuManager:    func(c *qt.C) jujuapi.JujuManager { return &mocks.JujuManager{} },
		expectedError:  `invalid model UUID: invalid UUID length: 11`,
	}, {
		about:          "non-admin user with owner and model name",
		admin:          false,
		modelQualifier: "alice@canonical.com/test-model",
		jujuManager:    func(c *qt.C) jujuapi.JujuManager { return &mocks.JujuManager{} },
		expectedError:  "unauthorized",
	}, {
		about:          "model not found with owner and model name",
		admin:          true,
		modelQualifier: "bob@canonical.com/non-existent",
		jujuManager: func(c *qt.C) jujuapi.JujuManager {
			return &mocks.JujuManager{
				ModelControllerInfo_: func(ctx context.Context, user *openfga.User, qualifier juju.ModelControllerInfoQualifier) (*apiparams.ModelControllerInfo, error) {
					return nil, errors.New("model not found")
				},
			}
		},
		expectedError: "model not found",
	}}

	for _, test := range testCases {
		c.Log(test.about)
		jimm := &jimmtest.JIMM{
			JujuManager_: func() jujuapi.JujuManager {
				return test.jujuManager(c)
			},
		}
		root := newTestControllerRoot(jimm, "alice@canonical.com", test.admin)

		req := apiparams.ModelControllerInfoRequest{ModelQualifier: test.modelQualifier}
		info, err := root.ModelControllerInfo(ctx, req)

		if test.expectedError != "" {
			c.Assert(err, qt.ErrorMatches, test.expectedError)
		} else {
			c.Assert(err, qt.IsNil)
			c.Assert(info, qt.DeepEquals, test.expectedInfo)
		}
	}
}

func TestJobInfo(t *testing.T) {
	c := qt.New(t)

	ctx := c.Context()

	finishedTime := time.Date(2023, 8, 14, 0, 0, 0, 0, time.UTC)
	jimm := &jimmtest.JIMM{
		JobManager_: func() jujuapi.JobManager {
			return &mocks.JobManager{
				GetJobInfo_: func(ctx context.Context, jobID int64) (jobs.JobInfo, error) {
					if jobID != int64(1) {
						return jobs.JobInfo{}, errors.Codef(errors.CodeNotFound, "job not found")
					}
					c.Check(jobID, qt.Equals, int64(1))
					return jobs.JobInfo{
						ID:             1,
						Kind:           "bootstrap",
						Status:         apiparams.StatusRunning,
						CurrentAttempt: 1,
						MaxAttempts:    3,
						FinishedAt:     &finishedTime,
						Errors: []jobs.JobError{
							{
								At:      finishedTime,
								Attempt: 1,
								Error:   "some error",
							},
						},
					}, nil
				},
			}
		},
	}

	root := newTestControllerRoot(jimm, "alice@canonical.com", true)
	req := apiparams.JobInfoRequest{JobID: "1"}

	resp, err := root.JobInfo(ctx, req)
	c.Assert(err, qt.IsNil)
	c.Assert(resp.ID, qt.Equals, int64(1))
	c.Assert(resp.Kind, qt.Equals, "bootstrap")
	c.Assert(resp.Status, qt.Equals, apiparams.StatusRunning)
	c.Assert(resp.CurrentAttempt, qt.Equals, 1)
	c.Assert(resp.MaxAttempts, qt.Equals, 3)
	c.Assert(resp.FinishedAt, qt.IsNotNil)
	c.Assert(*resp.FinishedAt, qt.DeepEquals, finishedTime)
	c.Assert(len(resp.Errors), qt.Equals, 1)
	c.Assert(resp.Errors[0].At, qt.Equals, finishedTime)
	c.Assert(resp.Errors[0].Attempt, qt.Equals, 1)
	c.Assert(resp.Errors[0].Error, qt.Equals, "some error")

	// Test non-existent job ID
	_, err = root.JobInfo(ctx, apiparams.JobInfoRequest{JobID: "invalid"})
	c.Assert(errors.ErrorCode(err), qt.Equals, errors.CodeBadRequest)
	c.Assert(err.Error(), qt.Matches, "invalid job ID: invalid")
}

func TestJobInfo_RequiresAdmin(t *testing.T) {
	c := qt.New(t)

	ctx := c.Context()

	root := newTestControllerRoot(nil, "alice@canonical.com", false)
	req := apiparams.JobInfoRequest{JobID: "1"}

	_, err := root.JobInfo(ctx, req)
	c.Assert(err, qt.ErrorMatches, "unauthorized")
	c.Assert(errors.ErrorCode(err), qt.Equals, errors.CodeUnauthorized)
}

func TestListJobs(t *testing.T) {
	c := qt.New(t)

	ctx := c.Context()

	jimm := &jimmtest.JIMM{
		JobManager_: func() jujuapi.JobManager {
			return &mocks.JobManager{
				ListJobs_: func(ctx context.Context, params apiparams.ListJobsRequest) (apiparams.ListJobsResponse, error) {
					c.Check(params.Statuses, qt.DeepEquals, []apiparams.JobStatus{apiparams.StatusRunning})
					c.Check(params.Kinds, qt.DeepEquals, []string{"bootstrap-controller"})
					c.Check(params.Count, qt.Equals, 10)
					return apiparams.ListJobsResponse{
						Jobs: []apiparams.ListJobInfo{
							{ID: 1, Status: apiparams.StatusRunning, Kind: "bootstrap-controller", MaxAttempts: 3},
							{ID: 2, Status: apiparams.StatusRunning, Kind: "bootstrap-controller", MaxAttempts: 3},
							{ID: 3, Status: apiparams.StatusRunning, Kind: "bootstrap-controller", MaxAttempts: 3},
						},
						NextCursor: "test-cursor",
					}, nil
				},
			}
		},
	}

	root := newTestControllerRoot(jimm, "alice@canonical.com", true)
	req := apiparams.ListJobsRequest{
		Statuses: []apiparams.JobStatus{apiparams.StatusRunning},
		Kinds:    []string{"bootstrap-controller"},
		Count:    10,
	}

	resp, err := root.ListJobs(ctx, req)
	c.Assert(err, qt.IsNil)
	c.Assert(resp.Jobs, qt.HasLen, 3)
	c.Assert(resp.Jobs[0].ID, qt.Equals, int64(1))
	c.Assert(resp.Jobs[0].Status, qt.Equals, apiparams.StatusRunning)
	c.Assert(resp.Jobs[0].Kind, qt.Equals, "bootstrap-controller")
	c.Assert(resp.Jobs[0].MaxAttempts, qt.Equals, 3)
	c.Assert(resp.NextCursor, qt.Equals, "test-cursor")

	// Test error case
	jimm2 := &jimmtest.JIMM{
		JobManager_: func() jujuapi.JobManager {
			return &mocks.JobManager{
				ListJobs_: func(ctx context.Context, params apiparams.ListJobsRequest) (apiparams.ListJobsResponse, error) {
					return apiparams.ListJobsResponse{}, errors.Codef(errors.CodeNotFound, "no jobs found")
				},
			}
		},
	}
	root2 := newTestControllerRoot(jimm2, "alice@canonical.com", true)
	_, err = root2.ListJobs(ctx, req)
	c.Assert(errors.ErrorCode(err), qt.Equals, errors.CodeNotFound)
}

func TestListJobs_RequiresAdmin(t *testing.T) {
	c := qt.New(t)

	ctx := c.Context()

	root := newTestControllerRoot(nil, "alice@canonical.com", false)
	req := apiparams.ListJobsRequest{
		Statuses: []apiparams.JobStatus{apiparams.StatusRunning},
		Kinds:    []string{"bootstrap-controller"},
		Count:    10,
	}

	_, err := root.ListJobs(ctx, req)
	c.Assert(err, qt.ErrorMatches, "unauthorized")
	c.Assert(errors.ErrorCode(err), qt.Equals, errors.CodeUnauthorized)
}

func TestListMigrationTargets_Unauthorized(t *testing.T) {
	c := qt.New(t)

	ctx := c.Context()

	root := newTestControllerRoot(nil, "alice@canonical.com", false)
	req := apiparams.ListMigrationTargetsRequest{ModelTag: "123"}

	_, err := root.ListMigrationTargets(ctx, req)
	c.Assert(err, qt.ErrorMatches, "unauthorized")
	c.Assert(errors.ErrorCode(err), qt.Equals, errors.CodeUnauthorized)
}
