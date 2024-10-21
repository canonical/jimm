// Copyright 2024 Canonical.

package jujuapi_test

import (
	"context"
	"fmt"
	"testing"

	qt "github.com/frankban/quicktest"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v5"
	gc "gopkg.in/check.v1"

	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/jimm"
	"github.com/canonical/jimm/v3/internal/jujuapi"
	"github.com/canonical/jimm/v3/internal/openfga"
	ofganames "github.com/canonical/jimm/v3/internal/openfga/names"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
	"github.com/canonical/jimm/v3/pkg/api/params"
	jimmnames "github.com/canonical/jimm/v3/pkg/names"
)

// Unit tests (see below for integration tests).

func TestAddServiceAccount(t *testing.T) {
	c := qt.New(t)

	tests := []struct {
		about         string
		args          params.AddServiceAccountRequest
		addedClientId string
		expectedError string
	}{{
		about: "Valid client ID without domain",
		args: params.AddServiceAccountRequest{
			ClientID: "fca1f605-736e-4d1f-bcd2-aecc726923be",
		},
		addedClientId: "fca1f605-736e-4d1f-bcd2-aecc726923be@serviceaccount",
	}, {
		about: "Valid client ID with correct domain",
		args: params.AddServiceAccountRequest{
			ClientID: "fca1f605-736e-4d1f-bcd2-aecc726923be@serviceaccount",
		},
		addedClientId: "fca1f605-736e-4d1f-bcd2-aecc726923be@serviceaccount",
	}, {
		about: "Valid client ID with wrong domain",
		args: params.AddServiceAccountRequest{
			ClientID: "fca1f605-736e-4d1f-bcd2-aecc726923be@not-serviceaccount",
		},
		expectedError: "invalid client ID",
	}, {
		about: "Invalid Client ID without domain",
		args: params.AddServiceAccountRequest{
			ClientID: "_123_",
		},
		expectedError: "invalid client ID",
	}, {
		about: "Invalid Client ID with wrong domain",
		args: params.AddServiceAccountRequest{
			ClientID: "_123_@not-serviceaccount",
		},
		expectedError: "invalid client ID",
	}}

	for _, test := range tests {
		test := test
		c.Run(test.about, func(c *qt.C) {
			jimm := &jimmtest.JIMM{
				AddServiceAccount_: func(_ context.Context, _ *openfga.User, clientID string) error {
					c.Assert(clientID, qt.Equals, test.addedClientId)
					return nil
				},
			}
			cr := jujuapi.NewControllerRoot(jimm, jujuapi.Params{})

			err := cr.AddServiceAccount(context.Background(), test.args)
			if test.expectedError == "" {
				c.Assert(err, qt.IsNil)
			} else {
				c.Assert(err, qt.ErrorMatches, test.expectedError)
			}
		})
	}
}

func TestCopyServiceAccountCredential(t *testing.T) {
	c := qt.New(t)

	tests := []struct {
		about         string
		args          params.CopyServiceAccountCredentialRequest
		addTuples     []openfga.Tuple
		username      string
		expectedError string
	}{{
		about: "Valid request",
		args: params.CopyServiceAccountCredentialRequest{
			ClientID: "fca1f605-736e-4d1f-bcd2-aecc726923be",
			CloudCredentialArg: jujuparams.CloudCredentialArg{
				CloudName:      "aws",
				CredentialName: "my-cred",
			},
		},
		username: "alice",
		addTuples: []openfga.Tuple{{
			Object:   ofganames.ConvertTag(names.NewUserTag("alice")),
			Relation: ofganames.AdministratorRelation,
			Target:   ofganames.ConvertTag(jimmnames.NewServiceAccountTag("fca1f605-736e-4d1f-bcd2-aecc726923be@serviceaccount")),
		}},
	}, {
		about: "Valid request with @serviceaccount domain",
		args: params.CopyServiceAccountCredentialRequest{
			ClientID: "fca1f605-736e-4d1f-bcd2-aecc726923be@serviceaccount",
			CloudCredentialArg: jujuparams.CloudCredentialArg{
				CloudName:      "aws",
				CredentialName: "my-cred",
			},
		},
		username: "alice",
		addTuples: []openfga.Tuple{{
			Object:   ofganames.ConvertTag(names.NewUserTag("alice")),
			Relation: ofganames.AdministratorRelation,
			Target:   ofganames.ConvertTag(jimmnames.NewServiceAccountTag("fca1f605-736e-4d1f-bcd2-aecc726923be@serviceaccount")),
		}},
	}, {
		about: "Invalid cloud credential args",
		args: params.CopyServiceAccountCredentialRequest{
			ClientID: "fca1f605-736e-4d1f-bcd2-aecc726923be",
			CloudCredentialArg: jujuparams.CloudCredentialArg{
				CloudName:      "123aws",
				CredentialName: "123my-cred",
			},
		},
		username: "alice",
		addTuples: []openfga.Tuple{{
			Object:   ofganames.ConvertTag(names.NewUserTag("alice")),
			Relation: ofganames.AdministratorRelation,
			Target:   ofganames.ConvertTag(jimmnames.NewServiceAccountTag("fca1f605-736e-4d1f-bcd2-aecc726923be@serviceaccount")),
		}},
		expectedError: ".* is not a valid cloud credential tag",
	}, {
		about: "Missing permissions",
		args: params.CopyServiceAccountCredentialRequest{
			ClientID: "fca1f605-736e-4d1f-bcd2-aecc726923be",
			CloudCredentialArg: jujuparams.CloudCredentialArg{
				CloudName:      "aws",
				CredentialName: "my-cred",
			},
		},
		username:      "alice",
		expectedError: "unauthorized",
	}}

	for _, test := range tests {
		test := test
		c.Run(test.about, func(c *qt.C) {
			ofgaClient, _, _, err := jimmtest.SetupTestOFGAClient(c.Name())
			c.Assert(err, qt.IsNil)
			pgDb := db.Database{
				DB: jimmtest.PostgresDB(c, nil),
			}
			err = pgDb.Migrate(context.Background(), false)
			c.Assert(err, qt.IsNil)
			clientIdWithDomain, err := jimmnames.EnsureValidServiceAccountId(test.args.ClientID)
			c.Assert(err, qt.IsNil)
			jimm := &jimmtest.JIMM{
				CopyServiceAccountCredential_: func(ctx context.Context, u, svcAcc *openfga.User, cloudCredentialTag names.CloudCredentialTag) (names.CloudCredentialTag, []jujuparams.UpdateCredentialModelResult, error) {
					c.Assert(cloudCredentialTag.Cloud().Id(), qt.Equals, test.args.CloudCredentialArg.CloudName)
					c.Assert(cloudCredentialTag.Owner().Id(), qt.Equals, u.Name)
					c.Assert(cloudCredentialTag.Name(), qt.Equals, test.args.CloudCredentialArg.CredentialName)
					c.Assert(svcAcc.Name, qt.Equals, clientIdWithDomain)
					newCredTag := names.NewCloudCredentialTag(fmt.Sprintf("%s/%s/%s", test.args.CloudName, svcAcc.Name, test.args.CredentialName))
					return newCredTag, nil, nil
				},
				UserLogin_: func(ctx context.Context, email string) (*openfga.User, error) {
					var u dbmodel.Identity
					u.SetTag(names.NewUserTag(email))
					return openfga.NewUser(&u, ofgaClient), nil
				},
			}
			var u dbmodel.Identity
			u.SetTag(names.NewUserTag(test.username))
			user := openfga.NewUser(&u, ofgaClient)
			cr := jujuapi.NewControllerRoot(jimm, jujuapi.Params{})
			jujuapi.SetUser(cr, user)
			if len(test.addTuples) > 0 {
				err = ofgaClient.AddRelation(context.Background(), test.addTuples...)
				c.Assert(err, qt.IsNil)
			}
			res, err := cr.CopyServiceAccountCredential(context.Background(), test.args)
			if test.expectedError == "" {
				c.Assert(err, qt.IsNil)
				c.Assert(res.CredentialTag, qt.Equals, fmt.Sprintf("cloudcred-%s_%s_%s", test.args.CloudName, clientIdWithDomain, test.args.CredentialName))
			} else {
				c.Assert(err, qt.ErrorMatches, test.expectedError)
			}
		})
	}
}

func TestGetServiceAccount(t *testing.T) {
	c := qt.New(t)

	tests := []struct {
		about            string
		clientID         string
		addTuples        []openfga.Tuple
		username         string
		expectedClientID string
		expectedError    string
	}{{
		about:    "Valid request without domain",
		clientID: "fca1f605-736e-4d1f-bcd2-aecc726923be",
		username: "alice",
		addTuples: []openfga.Tuple{{
			Object:   ofganames.ConvertTag(names.NewUserTag("alice")),
			Relation: ofganames.AdministratorRelation,
			Target:   ofganames.ConvertTag(jimmnames.NewServiceAccountTag("fca1f605-736e-4d1f-bcd2-aecc726923be@serviceaccount")),
		}},
		expectedClientID: "fca1f605-736e-4d1f-bcd2-aecc726923be@serviceaccount",
	}, {
		about:    "Valid request with domain",
		clientID: "fca1f605-736e-4d1f-bcd2-aecc726923be@serviceaccount",
		username: "alice",
		addTuples: []openfga.Tuple{{
			Object:   ofganames.ConvertTag(names.NewUserTag("alice")),
			Relation: ofganames.AdministratorRelation,
			Target:   ofganames.ConvertTag(jimmnames.NewServiceAccountTag("fca1f605-736e-4d1f-bcd2-aecc726923be@serviceaccount")),
		}},
		expectedClientID: "fca1f605-736e-4d1f-bcd2-aecc726923be@serviceaccount",
	}, {
		about:         "Invalid request: wrong domain",
		clientID:      "fca1f605-736e-4d1f-bcd2-aecc726923be@not-serviceaccount",
		username:      "alice",
		expectedError: "invalid client ID",
	}, {
		about:         "Missing service account administrator permission",
		username:      "alice",
		clientID:      "fca1f605-736e-4d1f-bcd2-aecc726923be@serviceaccount",
		expectedError: "unauthorized",
	}, {
		about:         "Invalid Client ID",
		username:      "alice",
		clientID:      "_123_",
		expectedError: "invalid client ID",
	}}

	for _, test := range tests {
		test := test
		c.Run(test.about, func(c *qt.C) {
			ofgaClient, _, _, err := jimmtest.SetupTestOFGAClient(c.Name())
			c.Assert(err, qt.IsNil)
			pgDb := db.Database{
				DB: jimmtest.PostgresDB(c, nil),
			}
			err = pgDb.Migrate(context.Background(), false)
			c.Assert(err, qt.IsNil)
			jimm := &jimmtest.JIMM{
				UserLogin_: func(ctx context.Context, email string) (*openfga.User, error) {
					var u dbmodel.Identity
					u.SetTag(names.NewUserTag(email))
					return openfga.NewUser(&u, ofgaClient), nil
				},
			}
			var u dbmodel.Identity
			u.SetTag(names.NewUserTag(test.username))
			user := openfga.NewUser(&u, ofgaClient)
			cr := jujuapi.NewControllerRoot(jimm, jujuapi.Params{})
			jujuapi.SetUser(cr, user)

			if len(test.addTuples) > 0 {
				err = ofgaClient.AddRelation(context.Background(), test.addTuples...)
				c.Assert(err, qt.IsNil)
			}

			res, err := cr.GetServiceAccount(context.Background(), test.clientID)
			if test.expectedError == "" {
				c.Assert(err, qt.IsNil)
				c.Assert(res.Identity.Name, qt.Equals, test.expectedClientID)
			} else {
				c.Assert(err, qt.ErrorMatches, test.expectedError)
			}
		})
	}
}

func TestUpdateServiceAccountCredentials(t *testing.T) {
	c := qt.New(t)

	tests := []struct {
		about                 string
		updateCloudCredential func(ctx context.Context, u *openfga.User, args jimm.UpdateCloudCredentialArgs) ([]jujuparams.UpdateCredentialModelResult, error)
		args                  params.UpdateServiceAccountCredentialsRequest
		username              string
		addTuples             []openfga.Tuple
		expectedResult        jujuparams.UpdateCredentialResults
		expectedError         string
	}{{
		about: "Valid request without domain",
		updateCloudCredential: func(ctx context.Context, u *openfga.User, args jimm.UpdateCloudCredentialArgs) ([]jujuparams.UpdateCredentialModelResult, error) {
			return nil, nil
		},
		expectedResult: jujuparams.UpdateCredentialResults{
			Results: []jujuparams.UpdateCredentialResult{
				{
					CredentialTag: "cloudcred-aws/1cbe5066-ea80-4979-8633-048d32f46cf8/cred-name",
					Error:         nil,
					Models:        nil,
				},
				{
					CredentialTag: "cloudcred-azure/1cbe5066-ea80-4979-8633-048d32f46cf8/cred-name2",
					Error:         nil,
					Models:        nil,
				},
			}},
		args: params.UpdateServiceAccountCredentialsRequest{
			ClientID: "fca1f605-736e-4d1f-bcd2-aecc726923be",
			UpdateCredentialArgs: jujuparams.UpdateCredentialArgs{
				Credentials: []jujuparams.TaggedCredential{
					{
						Tag:        "cloudcred-aws/1cbe5066-ea80-4979-8633-048d32f46cf8/cred-name",
						Credential: jujuparams.CloudCredential{Attributes: map[string]string{"foo": "bar"}},
					},
					{
						Tag:        "cloudcred-azure/1cbe5066-ea80-4979-8633-048d32f46cf8/cred-name2",
						Credential: jujuparams.CloudCredential{Attributes: map[string]string{"wolf": "low"}},
					},
				}},
		},
		username: "alice",
		addTuples: []openfga.Tuple{{
			Object:   ofganames.ConvertTag(names.NewUserTag("alice")),
			Relation: ofganames.AdministratorRelation,
			Target:   ofganames.ConvertTag(jimmnames.NewServiceAccountTag("fca1f605-736e-4d1f-bcd2-aecc726923be@serviceaccount")),
		}},
	}, {
		about: "Valid request with domain",
		updateCloudCredential: func(ctx context.Context, u *openfga.User, args jimm.UpdateCloudCredentialArgs) ([]jujuparams.UpdateCredentialModelResult, error) {
			return nil, nil
		},
		expectedResult: jujuparams.UpdateCredentialResults{
			Results: []jujuparams.UpdateCredentialResult{
				{
					CredentialTag: "cloudcred-aws/1cbe5066-ea80-4979-8633-048d32f46cf8/cred-name",
					Error:         nil,
					Models:        nil,
				},
				{
					CredentialTag: "cloudcred-azure/1cbe5066-ea80-4979-8633-048d32f46cf8/cred-name2",
					Error:         nil,
					Models:        nil,
				},
			}},
		args: params.UpdateServiceAccountCredentialsRequest{
			ClientID: "fca1f605-736e-4d1f-bcd2-aecc726923be@serviceaccount",
			UpdateCredentialArgs: jujuparams.UpdateCredentialArgs{
				Credentials: []jujuparams.TaggedCredential{
					{
						Tag:        "cloudcred-aws/1cbe5066-ea80-4979-8633-048d32f46cf8/cred-name",
						Credential: jujuparams.CloudCredential{Attributes: map[string]string{"foo": "bar"}},
					},
					{
						Tag:        "cloudcred-azure/1cbe5066-ea80-4979-8633-048d32f46cf8/cred-name2",
						Credential: jujuparams.CloudCredential{Attributes: map[string]string{"wolf": "low"}},
					},
				}},
		},
		username: "alice",
		addTuples: []openfga.Tuple{{
			Object:   ofganames.ConvertTag(names.NewUserTag("alice")),
			Relation: ofganames.AdministratorRelation,
			Target:   ofganames.ConvertTag(jimmnames.NewServiceAccountTag("fca1f605-736e-4d1f-bcd2-aecc726923be@serviceaccount")),
		}},
	}, {
		about: "Invalid Credential Tag",
		updateCloudCredential: func(ctx context.Context, u *openfga.User, args jimm.UpdateCloudCredentialArgs) ([]jujuparams.UpdateCredentialModelResult, error) {
			return nil, nil
		},
		expectedResult: jujuparams.UpdateCredentialResults{
			Results: []jujuparams.UpdateCredentialResult{
				{
					CredentialTag: "invalid-cred-name",
					Error: &jujuparams.Error{
						Message: `"invalid-cred-name" is not a valid tag`,
					},
					Models: nil,
				},
			}},
		args: params.UpdateServiceAccountCredentialsRequest{
			ClientID: "fca1f605-736e-4d1f-bcd2-aecc726923be",
			UpdateCredentialArgs: jujuparams.UpdateCredentialArgs{
				Credentials: []jujuparams.TaggedCredential{
					{
						Tag:        "invalid-cred-name",
						Credential: jujuparams.CloudCredential{Attributes: map[string]string{"foo": "bar"}},
					},
				}},
		},
		username: "alice",
		addTuples: []openfga.Tuple{{
			Object:   ofganames.ConvertTag(names.NewUserTag("alice")),
			Relation: ofganames.AdministratorRelation,
			Target:   ofganames.ConvertTag(jimmnames.NewServiceAccountTag("fca1f605-736e-4d1f-bcd2-aecc726923be@serviceaccount")),
		}},
	}, {
		about: "Invalid Service account ID",
		updateCloudCredential: func(ctx context.Context, u *openfga.User, args jimm.UpdateCloudCredentialArgs) ([]jujuparams.UpdateCredentialModelResult, error) {
			return nil, nil
		},
		args: params.UpdateServiceAccountCredentialsRequest{
			ClientID: "_123_",
			UpdateCredentialArgs: jujuparams.UpdateCredentialArgs{
				Credentials: []jujuparams.TaggedCredential{
					{
						Tag:        "cloudcred-aws/1cbe5066-ea80-4979-8633-048d32f46cf8/cred-name",
						Credential: jujuparams.CloudCredential{Attributes: map[string]string{"foo": "bar"}},
					},
				}},
		},
		username:      "alice",
		expectedError: "invalid client ID",
	}, {
		about: "Missing service account administrator permission",
		updateCloudCredential: func(ctx context.Context, u *openfga.User, args jimm.UpdateCloudCredentialArgs) ([]jujuparams.UpdateCredentialModelResult, error) {
			return nil, nil
		},
		args: params.UpdateServiceAccountCredentialsRequest{
			ClientID: "fca1f605-736e-4d1f-bcd2-aecc726923be",
			UpdateCredentialArgs: jujuparams.UpdateCredentialArgs{
				Credentials: []jujuparams.TaggedCredential{
					{
						Tag:        "cloudcred-aws/1cbe5066-ea80-4979-8633-048d32f46cf8/cred-name",
						Credential: jujuparams.CloudCredential{Attributes: map[string]string{"foo": "bar"}},
					},
				}},
		},
		username:      "alice",
		expectedError: "unauthorized",
	}}

	for _, test := range tests {
		test := test
		c.Run(test.about, func(c *qt.C) {
			ofgaClient, _, _, err := jimmtest.SetupTestOFGAClient(c.Name())
			c.Assert(err, qt.IsNil)
			pgDb := db.Database{
				DB: jimmtest.PostgresDB(c, nil),
			}
			err = pgDb.Migrate(context.Background(), false)
			c.Assert(err, qt.IsNil)
			jimm := &jimmtest.JIMM{
				UpdateCloudCredential_: test.updateCloudCredential,
				UserLogin_:             func(ctx context.Context, email string) (*openfga.User, error) { return nil, nil },
			}
			var u dbmodel.Identity
			u.SetTag(names.NewUserTag(test.username))
			user := openfga.NewUser(&u, ofgaClient)
			cr := jujuapi.NewControllerRoot(jimm, jujuapi.Params{})
			jujuapi.SetUser(cr, user)

			if len(test.addTuples) > 0 {
				err = ofgaClient.AddRelation(context.Background(), test.addTuples...)
				c.Assert(err, qt.IsNil)
			}

			res, err := cr.UpdateServiceAccountCredentials(context.Background(), test.args)
			if test.expectedError == "" {
				c.Assert(err, qt.IsNil)
				c.Assert(res, qt.DeepEquals, test.expectedResult)
			} else {
				c.Assert(err, qt.ErrorMatches, test.expectedError)
			}
		})
	}
}

func TestListServiceAccountCredentials(t *testing.T) {
	c := qt.New(t)

	tests := []struct {
		about                        string
		getCloudCredential           func(ctx context.Context, user *openfga.User, tag names.CloudCredentialTag) (*dbmodel.CloudCredential, error)
		getCloudCredentialAttributes func(ctx context.Context, u *openfga.User, cred *dbmodel.CloudCredential, hidden bool) (attrs map[string]string, redacted []string, err error)
		ForEachUserCloudCredential   func(ctx context.Context, u *dbmodel.Identity, ct names.CloudTag, f func(cred *dbmodel.CloudCredential) error) error
		args                         params.ListServiceAccountCredentialsRequest
		username                     string
		addTuples                    []openfga.Tuple
		expectedResult               jujuparams.CredentialContentResults
		expectedError                string
	}{{
		about: "Valid request without domain",
		ForEachUserCloudCredential: func(ctx context.Context, u *dbmodel.Identity, ct names.CloudTag, f func(cred *dbmodel.CloudCredential) error) error {
			return nil
		},
		expectedResult: jujuparams.CredentialContentResults{
			Results: []jujuparams.CredentialContentResult{}},
		args: params.ListServiceAccountCredentialsRequest{
			ClientID: "fca1f605-736e-4d1f-bcd2-aecc726923be",
		},
		getCloudCredential: func(ctx context.Context, user *openfga.User, tag names.CloudCredentialTag) (*dbmodel.CloudCredential, error) {
			cred := &dbmodel.CloudCredential{}
			return cred, nil
		},
		getCloudCredentialAttributes: func(ctx context.Context, u *openfga.User, cred *dbmodel.CloudCredential, hidden bool) (attrs map[string]string, redacted []string, err error) {
			return nil, nil, nil
		},
		username: "alice",
		addTuples: []openfga.Tuple{{
			Object:   ofganames.ConvertTag(names.NewUserTag("alice")),
			Relation: ofganames.AdministratorRelation,
			Target:   ofganames.ConvertTag(jimmnames.NewServiceAccountTag("fca1f605-736e-4d1f-bcd2-aecc726923be@serviceaccount")),
		}},
	}, {
		about: "Valid request with domain",
		ForEachUserCloudCredential: func(ctx context.Context, u *dbmodel.Identity, ct names.CloudTag, f func(cred *dbmodel.CloudCredential) error) error {
			return nil
		},
		expectedResult: jujuparams.CredentialContentResults{
			Results: []jujuparams.CredentialContentResult{}},
		args: params.ListServiceAccountCredentialsRequest{
			ClientID: "fca1f605-736e-4d1f-bcd2-aecc726923be@serviceaccount",
		},
		getCloudCredential: func(ctx context.Context, user *openfga.User, tag names.CloudCredentialTag) (*dbmodel.CloudCredential, error) {
			cred := &dbmodel.CloudCredential{}
			return cred, nil
		},
		getCloudCredentialAttributes: func(ctx context.Context, u *openfga.User, cred *dbmodel.CloudCredential, hidden bool) (attrs map[string]string, redacted []string, err error) {
			return nil, nil, nil
		},
		username: "alice",
		addTuples: []openfga.Tuple{{
			Object:   ofganames.ConvertTag(names.NewUserTag("alice")),
			Relation: ofganames.AdministratorRelation,
			Target:   ofganames.ConvertTag(jimmnames.NewServiceAccountTag("fca1f605-736e-4d1f-bcd2-aecc726923be@serviceaccount")),
		}},
	}, {
		about: "Invalid Service account ID",
		ForEachUserCloudCredential: func(ctx context.Context, u *dbmodel.Identity, ct names.CloudTag, f func(cred *dbmodel.CloudCredential) error) error {
			return nil
		},
		args: params.ListServiceAccountCredentialsRequest{
			ClientID: "_123_",
		},
		getCloudCredential: func(ctx context.Context, user *openfga.User, tag names.CloudCredentialTag) (*dbmodel.CloudCredential, error) {
			cred := &dbmodel.CloudCredential{}
			return cred, nil
		},
		getCloudCredentialAttributes: func(ctx context.Context, u *openfga.User, cred *dbmodel.CloudCredential, hidden bool) (attrs map[string]string, redacted []string, err error) {
			return nil, nil, nil
		},
		username:      "alice",
		expectedError: "invalid client ID",
	}, {
		about: "Missing service account administrator permission",
		ForEachUserCloudCredential: func(ctx context.Context, u *dbmodel.Identity, ct names.CloudTag, f func(cred *dbmodel.CloudCredential) error) error {
			return nil
		},
		args: params.ListServiceAccountCredentialsRequest{
			ClientID: "fca1f605-736e-4d1f-bcd2-aecc726923be",
		},
		getCloudCredential: func(ctx context.Context, user *openfga.User, tag names.CloudCredentialTag) (*dbmodel.CloudCredential, error) {
			cred := &dbmodel.CloudCredential{}
			return cred, nil
		},
		getCloudCredentialAttributes: func(ctx context.Context, u *openfga.User, cred *dbmodel.CloudCredential, hidden bool) (attrs map[string]string, redacted []string, err error) {
			return nil, nil, nil
		},
		username:      "alice",
		expectedError: "unauthorized",
	}}

	for _, test := range tests {
		test := test
		c.Run(test.about, func(c *qt.C) {
			ofgaClient, _, _, err := jimmtest.SetupTestOFGAClient(c.Name())
			c.Assert(err, qt.IsNil)
			pgDb := db.Database{
				DB: jimmtest.PostgresDB(c, nil),
			}
			err = pgDb.Migrate(context.Background(), false)
			c.Assert(err, qt.IsNil)
			jimm := &jimmtest.JIMM{
				GetCloudCredential_:           test.getCloudCredential,
				GetCloudCredentialAttributes_: test.getCloudCredentialAttributes,
				ForEachUserCloudCredential_:   test.ForEachUserCloudCredential,
				UserLogin_: func(ctx context.Context, email string) (*openfga.User, error) {
					var u dbmodel.Identity
					u.SetTag(names.NewUserTag(email))
					return openfga.NewUser(&u, ofgaClient), nil
				},
			}
			var u dbmodel.Identity
			u.SetTag(names.NewUserTag(test.username))
			user := openfga.NewUser(&u, ofgaClient)
			cr := jujuapi.NewControllerRoot(jimm, jujuapi.Params{})
			jujuapi.SetUser(cr, user)

			if len(test.addTuples) > 0 {
				err = ofgaClient.AddRelation(context.Background(), test.addTuples...)
				c.Assert(err, qt.IsNil)
			}

			res, err := cr.ListServiceAccountCredentials(context.Background(), test.args)
			if test.expectedError == "" {
				c.Assert(err, qt.IsNil)
				c.Assert(res, qt.DeepEquals, test.expectedResult)
			} else {
				c.Assert(err, qt.ErrorMatches, test.expectedError)
			}
		})
	}
}

func TestGrantServiceAccountAccess(t *testing.T) {
	c := qt.New(t)

	tests := []struct {
		about                     string
		grantServiceAccountAccess func(ctx context.Context, user *openfga.User, svcAccTag jimmnames.ServiceAccountTag, entities []string) error
		params                    params.GrantServiceAccountAccess
		tags                      []string
		username                  string
		addTuples                 []openfga.Tuple
		expectedError             string
	}{{
		about: "Valid request without domain",
		grantServiceAccountAccess: func(ctx context.Context, user *openfga.User, svcAccTag jimmnames.ServiceAccountTag, entities []string) error {
			return nil
		},
		params: params.GrantServiceAccountAccess{
			Entities: []string{
				"user-alice",
				"user-bob",
			},
			ClientID: "fca1f605-736e-4d1f-bcd2-aecc726923be",
		},
		username: "alice",
		addTuples: []openfga.Tuple{{
			Object:   ofganames.ConvertTag(names.NewUserTag("alice")),
			Relation: ofganames.AdministratorRelation,
			Target:   ofganames.ConvertTag(jimmnames.NewServiceAccountTag("fca1f605-736e-4d1f-bcd2-aecc726923be@serviceaccount")),
		}},
	}, {
		about: "Valid request with domain",
		grantServiceAccountAccess: func(ctx context.Context, user *openfga.User, svcAccTag jimmnames.ServiceAccountTag, entities []string) error {
			return nil
		},
		params: params.GrantServiceAccountAccess{
			Entities: []string{
				"user-alice",
				"user-bob",
			},
			ClientID: "fca1f605-736e-4d1f-bcd2-aecc726923be@serviceaccount",
		},
		username: "alice",
		addTuples: []openfga.Tuple{{
			Object:   ofganames.ConvertTag(names.NewUserTag("alice")),
			Relation: ofganames.AdministratorRelation,
			Target:   ofganames.ConvertTag(jimmnames.NewServiceAccountTag("fca1f605-736e-4d1f-bcd2-aecc726923be@serviceaccount")),
		}},
	}, {
		about: "Invalid Service account ID",
		grantServiceAccountAccess: func(ctx context.Context, user *openfga.User, svcAccTag jimmnames.ServiceAccountTag, entities []string) error {
			return nil
		},
		params: params.GrantServiceAccountAccess{
			Entities: []string{
				"user-alice",
				"user-bob",
			},
			ClientID: "_123_",
		},
		username:      "alice",
		expectedError: "invalid client ID",
	}, {
		about: "Missing service account administrator permission",
		grantServiceAccountAccess: func(ctx context.Context, user *openfga.User, svcAccTag jimmnames.ServiceAccountTag, entities []string) error {
			return nil
		},
		params: params.GrantServiceAccountAccess{
			Entities: []string{
				"user-alice",
				"user-bob",
			},
			ClientID: "fca1f605-736e-4d1f-bcd2-aecc726923be@serviceaccount",
		},
		username:      "alice",
		expectedError: "unauthorized",
	}}

	for _, test := range tests {
		test := test
		c.Run(test.about, func(c *qt.C) {
			ofgaClient, _, _, err := jimmtest.SetupTestOFGAClient(c.Name())
			c.Assert(err, qt.IsNil)
			pgDb := db.Database{
				DB: jimmtest.PostgresDB(c, nil),
			}
			err = pgDb.Migrate(context.Background(), false)
			c.Assert(err, qt.IsNil)
			jimm := &jimmtest.JIMM{
				UserLogin_:                 func(ctx context.Context, email string) (*openfga.User, error) { return nil, nil },
				GrantServiceAccountAccess_: test.grantServiceAccountAccess,
			}
			var u dbmodel.Identity
			u.SetTag(names.NewUserTag(test.username))
			user := openfga.NewUser(&u, ofgaClient)
			cr := jujuapi.NewControllerRoot(jimm, jujuapi.Params{})
			jujuapi.SetUser(cr, user)

			if len(test.addTuples) > 0 {
				err = ofgaClient.AddRelation(context.Background(), test.addTuples...)
				c.Assert(err, qt.IsNil)
			}

			err = cr.GrantServiceAccountAccess(context.Background(), test.params)
			if test.expectedError == "" {
				c.Assert(err, qt.IsNil)
			} else {
				c.Assert(err, qt.ErrorMatches, test.expectedError)
			}
		})
	}
}

// Integration tests below.
type serviceAccountSuite struct {
	websocketSuite
}

var _ = gc.Suite(&serviceAccountSuite{})

func (s *serviceAccountSuite) TestUpdateServiceAccountCredentialsIntegration(c *gc.C) {
	conn := s.open(c, nil, "bob")
	defer conn.Close()

	serviceAccount := jimmnames.NewServiceAccountTag("fca1f605-736e-4d1f-bcd2-aecc726923be@serviceaccount")

	tuple := openfga.Tuple{
		Object:   ofganames.ConvertTag(names.NewUserTag("bob@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(serviceAccount),
	}

	err := s.JIMM.OpenFGAClient.AddRelation(context.Background(), tuple)
	c.Assert(err, gc.IsNil)
	cloud := &dbmodel.Cloud{
		Name: "aws",
	}
	err = s.JIMM.Database.AddCloud(context.Background(), cloud)
	c.Assert(err, gc.IsNil)

	var credResults jujuparams.UpdateCredentialResults
	err = conn.APICall("JIMM", 4, "", "UpdateServiceAccountCredentials", params.UpdateServiceAccountCredentialsRequest{
		ClientID: "fca1f605-736e-4d1f-bcd2-aecc726923be@serviceaccount",
		UpdateCredentialArgs: jujuparams.UpdateCredentialArgs{
			Credentials: []jujuparams.TaggedCredential{
				{
					Tag:        "cloudcred-aws/fca1f605-736e-4d1f-bcd2-aecc726923be@serviceaccount/cred-name",
					Credential: jujuparams.CloudCredential{Attributes: map[string]string{"foo": "bar"}},
				},
				{
					Tag:        "cloudcred-aws/fca1f605-736e-4d1f-bcd2-aecc726923be@serviceaccount/cred-name2",
					Credential: jujuparams.CloudCredential{Attributes: map[string]string{"wolf": "low"}},
				},
			}},
	}, &credResults)

	expectedResult := jujuparams.UpdateCredentialResults{
		Results: []jujuparams.UpdateCredentialResult{
			{
				CredentialTag: "cloudcred-aws/fca1f605-736e-4d1f-bcd2-aecc726923be@serviceaccount/cred-name",
				Error:         nil,
				Models:        nil,
			},
			{
				CredentialTag: "cloudcred-aws/fca1f605-736e-4d1f-bcd2-aecc726923be@serviceaccount/cred-name2",
				Error:         nil,
				Models:        nil,
			},
		}}
	c.Assert(err, gc.Equals, nil)
	c.Assert(credResults, gc.DeepEquals, expectedResult)
}
