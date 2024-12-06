// Copyright 2024 Canonical.

package jimm_test

import (
	"context"
	"testing"

	qt "github.com/frankban/quicktest"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v5"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/jimm"
	"github.com/canonical/jimm/v3/internal/openfga"
	ofganames "github.com/canonical/jimm/v3/internal/openfga/names"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
	jimmnames "github.com/canonical/jimm/v3/pkg/names"
)

func TestAddServiceAccount(t *testing.T) {
	c := qt.New(t)

	ctx := context.Background()

	j := jimmtest.NewJIMM(c, nil)

	bob, err := dbmodel.NewIdentity("bob@canonical.com")
	c.Assert(err, qt.IsNil)
	user := openfga.NewUser(
		bob,
		j.OpenFGAClient,
	)
	clientID := "39caae91-b914-41ae-83f8-c7b86ca5ad5a@serviceaccount"
	err = j.AddServiceAccount(ctx, user, clientID)
	c.Assert(err, qt.IsNil)
	err = j.AddServiceAccount(ctx, user, clientID)
	c.Assert(err, qt.IsNil)

	alive, err := dbmodel.NewIdentity("alive@canonical.com")
	c.Assert(err, qt.IsNil)
	userAlice := openfga.NewUser(
		alive,
		j.OpenFGAClient,
	)
	err = j.AddServiceAccount(ctx, userAlice, clientID)
	c.Assert(err, qt.ErrorMatches, "service account already owned")
}

func TestCopyServiceAccountCredential(t *testing.T) {
	c := qt.New(t)

	ctx := context.Background()

	api := &jimmtest.API{
		CheckCredentialModels_: func(context.Context, jujuparams.TaggedCredential) ([]jujuparams.UpdateCredentialModelResult, error) {
			return []jujuparams.UpdateCredentialModelResult{}, nil
		},
		UpdateCredential_: func(context.Context, jujuparams.TaggedCredential) ([]jujuparams.UpdateCredentialModelResult, error) {
			return []jujuparams.UpdateCredentialModelResult{}, nil
		},
	}

	j := jimmtest.NewJIMM(c, &jimm.Parameters{
		Dialer: &jimmtest.Dialer{
			API: api,
		},
	})

	svcAccId, err := dbmodel.NewIdentity("39caae91-b914-41ae-83f8-c7b86ca5ad5a@serviceaccount")
	c.Assert(err, qt.IsNil)
	c.Assert(j.Database.DB.Create(&svcAccId).Error, qt.IsNil)
	svcAcc := openfga.NewUser(svcAccId, j.OpenFGAClient)
	u, err := dbmodel.NewIdentity("alice@canonical.com")
	c.Assert(err, qt.IsNil)

	c.Assert(j.Database.DB.Create(&u).Error, qt.IsNil)

	user := openfga.NewUser(u, j.OpenFGAClient)

	err = user.SetControllerAccess(context.Background(), j.ResourceTag(), ofganames.AdministratorRelation)
	c.Assert(err, qt.IsNil)

	// Create cloud, controller and cloud-credential as setup for test.
	cloud := dbmodel.Cloud{
		Name: "test-cloud",
		Type: "test-provider",
		Regions: []dbmodel.CloudRegion{{
			Name: "test-region-1",
		}},
	}
	c.Assert(j.Database.DB.Create(&cloud).Error, qt.IsNil)

	err = user.SetCloudAccess(context.Background(), cloud.ResourceTag(), ofganames.AdministratorRelation)
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
		Name:              "test-credential-1",
		CloudName:         cloud.Name,
		OwnerIdentityName: u.Name,
		AuthType:          "empty",
	}
	err = j.Database.SetCloudCredential(context.Background(), &cred)
	c.Assert(err, qt.Equals, nil)

	_, res, err := j.CopyServiceAccountCredential(ctx, user, svcAcc, cred.ResourceTag())
	c.Assert(err, qt.Equals, nil)
	newCred := dbmodel.CloudCredential{
		Name:              "test-credential-1",
		CloudName:         cloud.Name,
		OwnerIdentityName: svcAcc.Name,
	}
	c.Assert(len(res), qt.Equals, 0)
	err = j.Database.GetCloudCredential(context.Background(), &newCred)
	c.Assert(err, qt.Equals, nil)
}

func TestCopyServiceAccountCredentialWithMissingCredential(t *testing.T) {
	c := qt.New(t)

	ctx := context.Background()

	j := jimmtest.NewJIMM(c, nil)

	svcAccId, err := dbmodel.NewIdentity("39caae91-b914-41ae-83f8-c7b86ca5ad5a@serviceaccount")
	c.Assert(err, qt.IsNil)
	c.Assert(j.Database.DB.Create(&svcAccId).Error, qt.IsNil)
	svcAcc := openfga.NewUser(svcAccId, j.OpenFGAClient)
	u, err := dbmodel.NewIdentity("alice@canonical.com")
	c.Assert(err, qt.IsNil)
	c.Assert(j.Database.DB.Create(&u).Error, qt.IsNil)
	user := openfga.NewUser(u, j.OpenFGAClient)

	cred := dbmodel.CloudCredential{
		Name:              "test-credential-1",
		CloudName:         "fake-cloud",
		OwnerIdentityName: u.Name,
		AuthType:          "empty",
	}
	_, _, err = j.CopyServiceAccountCredential(ctx, user, svcAcc, cred.ResourceTag())
	c.Assert(err, qt.ErrorMatches, "cloudcredential .* not found")
}

func TestGrantServiceAccountAccess(t *testing.T) {
	c := qt.New(t)

	tests := []struct {
		about                     string
		grantServiceAccountAccess func(ctx context.Context, user *openfga.User, tags []string) error
		clientID                  string
		tags                      []string
		username                  string
		addGroups                 []string
		expectedError             string
	}{{
		about: "Valid request",
		grantServiceAccountAccess: func(ctx context.Context, user *openfga.User, tags []string) error {
			return nil
		},
		addGroups: []string{"1"},
		tags: []string{
			"user-alice",
			"user-bob",
			"group-1#member",
		},
		clientID: "fca1f605-736e-4d1f-bcd2-aecc726923be@serviceaccount",
		username: "alice",
	}, {
		about: "Group that doesn't exist",
		grantServiceAccountAccess: func(ctx context.Context, user *openfga.User, tags []string) error {
			return nil
		},
		tags: []string{
			"user-alice",
			"user-bob",
			// This group doesn't exist.
			"group-bar",
		},
		clientID:      "fca1f605-736e-4d1f-bcd2-aecc726923be@serviceaccount",
		username:      "alice",
		expectedError: "group bar not found",
	}, {
		about: "Invalid tags",
		grantServiceAccountAccess: func(ctx context.Context, user *openfga.User, tags []string) error {
			return nil
		},
		tags: []string{
			"user-alice",
			"user-bob",
			"controller-jimm",
		},
		clientID:      "fca1f605-736e-4d1f-bcd2-aecc726923be@serviceaccount",
		username:      "alice",
		expectedError: "invalid entity - not user or group",
	}}

	for _, test := range tests {
		test := test
		c.Run(test.about, func(c *qt.C) {
			j := jimmtest.NewJIMM(c, nil)

			var u dbmodel.Identity
			u.SetTag(names.NewUserTag(test.clientID))
			svcAccountIdentity := openfga.NewUser(&u, j.OpenFGAClient)
			svcAccountIdentity.JimmAdmin = true
			if len(test.addGroups) > 0 {
				for _, name := range test.addGroups {
					_, err := j.GroupManager().AddGroup(context.Background(), svcAccountIdentity, name)
					c.Assert(err, qt.IsNil)
				}
			}
			svcAccountTag := jimmnames.NewServiceAccountTag(test.clientID)

			err := j.GrantServiceAccountAccess(context.Background(), svcAccountIdentity, svcAccountTag, test.tags)
			if test.expectedError == "" {
				c.Assert(err, qt.IsNil)
				for _, tag := range test.tags {
					parsedTag, err := j.ParseAndValidateTag(context.Background(), tag)
					c.Assert(err, qt.IsNil)
					tuple := openfga.Tuple{
						Object:   parsedTag,
						Relation: ofganames.AdministratorRelation,
						Target:   ofganames.ConvertTag(jimmnames.NewServiceAccountTag(test.clientID)),
					}
					ok, err := j.OpenFGAClient.CheckRelation(context.Background(), tuple, false)
					c.Assert(err, qt.IsNil)
					c.Assert(ok, qt.IsTrue)
				}
			} else {
				c.Assert(err, qt.ErrorMatches, test.expectedError)
			}
		})
	}
}
