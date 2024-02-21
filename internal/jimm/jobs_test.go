// Copyright 2023 Canonical Ltd.

package jimm_test

import (
	"context"
	"testing"

	qt "github.com/frankban/quicktest"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/google/uuid"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/status"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v4"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	apiparams "github.com/canonical/jimm/api/params"
	"github.com/canonical/jimm/internal/db"
	"github.com/canonical/jimm/internal/dbmodel"
	"github.com/canonical/jimm/internal/jimm"
	"github.com/canonical/jimm/internal/jimmtest"
	"github.com/canonical/jimm/internal/openfga"
	ofganames "github.com/canonical/jimm/internal/openfga/names"
)

func TestJobListParams(t *testing.T) {
	c := qt.New(t)
	tests := []struct {
		about             string
		request           apiparams.FindJobsRequest
		state             rivertype.JobState
		expectedJobParams river.JobListParams
	}{{
		about: "get failed jobs, negative limit so it will replace it with 1, and sort ascending",
		request: apiparams.FindJobsRequest{
			Limit:   -1,
			SortAsc: true,
		},
		state:             rivertype.JobStateDiscarded,
		expectedJobParams: *river.NewJobListParams().First(1).OrderBy(river.JobListOrderByTime, river.SortOrderAsc).State(river.JobStateDiscarded),
	}, {
		about: "get cancelled jobs, > 10000 limit so it will be capped at 10000, and sort descending",
		request: apiparams.FindJobsRequest{
			Limit:   15000,
			SortAsc: false,
		},
		state:             rivertype.JobStateCancelled,
		expectedJobParams: *river.NewJobListParams().First(10000).OrderBy(river.JobListOrderByTime, river.SortOrderDesc).State(river.JobStateCancelled),
	}, {
		about: "get completed jobs, 2000 limit, and sort ascending",
		request: apiparams.FindJobsRequest{
			Limit:   2000,
			SortAsc: false,
		},
		state:             rivertype.JobStateCancelled,
		expectedJobParams: *river.NewJobListParams().First(2000).OrderBy(river.JobListOrderByTime, river.SortOrderDesc).State(river.JobStateCancelled),
	}}

	for _, test := range tests {
		c.Run(test.about, func(c *qt.C) {
			params := jimm.GetJobListParams(test.request, test.state)
			c.Assert(*params, qt.CmpEquals(cmpopts.IgnoreUnexported(river.JobListParams{})), test.expectedJobParams)
		})
	}
}

func TestFindJobs(t *testing.T) {
	c := qt.New(t)

	client, _, _, err := jimmtest.SetupTestOFGAClient(c.Name())
	c.Assert(err, qt.IsNil)

	ctx := context.Background()
	jimmDb := db.Database{
		DB: jimmtest.PostgresDB(c, nil),
	}
	j := &jimm.JIMM{
		UUID:          uuid.NewString(),
		Database:      jimmDb,
		OpenFGAClient: client,
	}
	j.River = jimmtest.NewRiver(c, nil, client, &jimmDb, j)

	err = j.Database.Migrate(ctx, true)
	c.Assert(err, qt.Equals, nil)

	admin := openfga.NewUser(&dbmodel.User{Username: "alice@external"}, client)
	u := dbmodel.User{
		Username: "alice@external",
	}
	c.Assert(j.Database.DB.Create(&u).Error, qt.IsNil)
	err = admin.SetControllerAccess(ctx, j.ResourceTag(), ofganames.AdministratorRelation)
	c.Assert(err, qt.IsNil)

	cloud := dbmodel.Cloud{
		Name: "test-cloud",
		Type: "test-provider",
		Regions: []dbmodel.CloudRegion{{
			Name: "test-region-1",
		}},
	}
	c.Assert(j.Database.DB.Create(&cloud).Error, qt.IsNil)
	err = admin.SetCloudAccess(context.Background(), cloud.ResourceTag(), ofganames.AdministratorRelation)
	c.Assert(err, qt.IsNil)

	controller1 := dbmodel.Controller{
		Name:        "test-controller-1",
		UUID:        "00000000-0000-0000-0000-0000-0000000000001",
		CloudName:   "test-cloud",
		CloudRegion: "test-region-1",
		CloudRegions: []dbmodel.CloudRegionControllerPriority{{
			Priority:      0,
			CloudRegionID: cloud.Regions[0].ID,
		}},
	}
	err = j.Database.AddController(context.Background(), &controller1)
	c.Assert(err, qt.Equals, nil)
	cred := dbmodel.CloudCredential{
		Name:          "test-credential-1",
		CloudName:     cloud.Name,
		OwnerUsername: u.Username,
		AuthType:      "empty",
	}
	err = j.Database.SetCloudCredential(context.Background(), &cred)
	c.Assert(err, qt.Equals, nil)

	_, err = j.AddModel(context.Background(), admin, &jimm.ModelCreateArgs{
		Name:            "test-model",
		Owner:           names.NewUserTag(u.Username),
		Cloud:           names.NewCloudTag(cloud.Name),
		CloudRegion:     "test-region-1",
		CloudCredential: names.NewCloudCredentialTag("test-cloud/alice@external/test-credential-1"),
	})
	c.Assert(err, qt.IsNotNil)
	// the above fails because no dialer is configured on JIMM
	api := &jimmtest.API{
		UpdateCredential_: func(context.Context, jujuparams.TaggedCredential) ([]jujuparams.UpdateCredentialModelResult, error) {
			return []jujuparams.UpdateCredentialModelResult{{
				ModelUUID: "00000001-0000-0000-0000-0000-000000000001",
				ModelName: "test-model",
			}}, nil
		},
		GrantJIMMModelAdmin_: func(_ context.Context, _ names.ModelTag) error {
			return nil
		},
		ModelInfo_: func(ctx context.Context, mi *jujuparams.ModelInfo) error { return nil },
		CreateModel_: func(ctx context.Context, args *jujuparams.ModelCreateArgs, mi *jujuparams.ModelInfo) error {
			mi.Name = args.Name
			mi.UUID = "00000001-0000-0000-0000-0000-000000000001"
			mi.CloudTag = args.CloudTag
			mi.CloudCredentialTag = args.CloudCredentialTag
			mi.CloudRegion = args.CloudRegion
			mi.OwnerTag = args.OwnerTag
			mi.Status = jujuparams.EntityStatus{
				Status: status.Started,
				Info:   "running a test",
			}
			mi.Life = life.Alive
			mi.Users = []jujuparams.ModelUserInfo{{
				UserName: "alice@external",
				Access:   jujuparams.ModelAdminAccess,
			}, {
				// "bob" is a local user
				UserName: "bob",
				Access:   jujuparams.ModelReadAccess,
			}}
			mi.Machines = []jujuparams.ModelMachineInfo{{
				Id:          "test-machine-id",
				DisplayName: "a test machine",
				Status:      "running",
				Message:     "a test message",
				HasVote:     true,
				WantsVote:   false,
			}}
			return nil
		},
	}
	j.Dialer = &jimmtest.Dialer{
		API: api,
	}

	_, err = j.AddModel(context.Background(), admin, &jimm.ModelCreateArgs{
		Name:            "test-model",
		Owner:           names.NewUserTag(u.Username),
		Cloud:           names.NewCloudTag(cloud.Name),
		CloudRegion:     "test-region-1",
		CloudCredential: names.NewCloudCredentialTag("test-cloud/alice@external/test-credential-1"),
	})
	c.Assert(err, qt.IsNil)

	jobs, err := j.FindJobs(context.Background(), apiparams.FindJobsRequest{IncludeCancelled: true, IncludeCompleted: true, IncludeFailed: true, SortAsc: true})
	expectedJobs := apiparams.Jobs{
		FailedJobs:    []apiparams.Job{{Kind: "AddModel", ID: 1, Attempt: 1, MaxAttempts: 1, State: string(rivertype.JobStateDiscarded)}},
		CompletedJobs: []apiparams.Job{{Kind: "AddModel", ID: 2, Attempt: 1, MaxAttempts: 1, State: string(rivertype.JobStateCompleted)}},
		CancelledJobs: []apiparams.Job{},
	}
	c.Assert(err, qt.Equals, nil)
	jobComparator := qt.CmpEquals(cmpopts.IgnoreFields(apiparams.Job{}, "FinalizedAt", "Errors", "EncodedArgs", "CreatedAt", "AttemptedAt"), cmpopts.EquateEmpty())
	c.Assert(jobs.FailedJobs, jobComparator, expectedJobs.FailedJobs)
	c.Assert(len(jobs.CompletedJobs), qt.DeepEquals, len(expectedJobs.CompletedJobs))
}
