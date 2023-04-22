// Copyright 2020 Canonical Ltd.

package jimm_test

import (
	"context"
	"database/sql"
	"sort"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/google/uuid"
	"github.com/juju/charm/v10"
	"github.com/juju/juju/core/crossmodel"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v4"
	"gopkg.in/macaroon.v2"
	"gorm.io/gorm"

	"github.com/CanonicalLtd/jimm/internal/db"
	"github.com/CanonicalLtd/jimm/internal/dbmodel"
	"github.com/CanonicalLtd/jimm/internal/errors"
	"github.com/CanonicalLtd/jimm/internal/jimm"
	"github.com/CanonicalLtd/jimm/internal/jimmtest"
	"github.com/CanonicalLtd/jimm/internal/openfga"
	ofganames "github.com/CanonicalLtd/jimm/internal/openfga/names"
)

type environment struct {
	users             []dbmodel.User
	clouds            []dbmodel.Cloud
	credentials       []dbmodel.CloudCredential
	controllers       []dbmodel.Controller
	models            []dbmodel.Model
	applicationOffers []dbmodel.ApplicationOffer
}

var initializeEnvironment = func(c *qt.C, ctx context.Context, db *db.Database, client *openfga.OFGAClient, jimmUUID string) *environment {
	env := environment{}

	// Alice is a model admin, but not a superuser or offer admin.
	u := dbmodel.User{
		Username: "alice@external",
	}
	c.Assert(db.DB.Create(&u).Error, qt.IsNil)

	u1 := dbmodel.User{
		Username: "eve@external",
	}
	c.Assert(db.DB.Create(&u1).Error, qt.IsNil)

	u2 := dbmodel.User{
		Username: "bob@external",
	}
	c.Assert(db.DB.Create(&u2).Error, qt.IsNil)

	u3 := dbmodel.User{
		Username: "fred@external",
	}
	c.Assert(db.DB.Create(&u3).Error, qt.IsNil)

	u4 := dbmodel.User{
		Username: "grant@external",
	}
	c.Assert(db.DB.Create(&u4).Error, qt.IsNil)

	// Jane is an offer admin, but not a superuser or model admin.
	u5 := dbmodel.User{
		Username: "jane@external",
	}
	c.Assert(db.DB.Create(&u5).Error, qt.IsNil)

	// Joe is a superuser, but not a model or offer admin.
	u6 := dbmodel.User{
		Username:         "joe@external",
		ControllerAccess: "superuser",
	}
	c.Assert(db.DB.Create(&u6).Error, qt.IsNil)

	err := openfga.NewUser(&u6, client).SetControllerAccess(ctx, names.NewControllerTag(jimmUUID), ofganames.AdministratorRelation)
	c.Assert(err, qt.IsNil)

	env.users = []dbmodel.User{u, u1, u2, u3, u4, u5, u6}

	cloud := dbmodel.Cloud{
		Name: "test-cloud",
		Type: "test-provider",
		Regions: []dbmodel.CloudRegion{{
			Name: "test-region-1",
		}},
	}
	c.Assert(db.DB.Create(&cloud).Error, qt.IsNil)
	env.clouds = []dbmodel.Cloud{cloud}

	// user u is administrator of the test-cloud
	err = openfga.NewUser(&u, client).SetCloudAccess(ctx, cloud.ResourceTag(), ofganames.AdministratorRelation)
	c.Assert(err, qt.IsNil)

	controller := dbmodel.Controller{
		Name:          "test-controller-1",
		UUID:          "00000000-0000-0000-0000-0000-0000000000001",
		PublicAddress: "test-public-address",
		CACertificate: "test-ca-cert",
		CloudName:     cloud.Name,
		CloudRegion:   cloud.Regions[0].Name,
		CloudRegions: []dbmodel.CloudRegionControllerPriority{{
			Priority:      0,
			CloudRegionID: cloud.Regions[0].ID,
		}},
	}
	err = db.AddController(ctx, &controller)
	c.Assert(err, qt.IsNil)
	env.controllers = []dbmodel.Controller{controller}

	err = client.AddCloudController(context.Background(), cloud.ResourceTag(), controller.ResourceTag())
	c.Assert(err, qt.IsNil)

	err = client.AddController(context.Background(), names.NewControllerTag(jimmUUID), controller.ResourceTag())
	c.Assert(err, qt.IsNil)

	cred := dbmodel.CloudCredential{
		Name:          "test-credential-1",
		CloudName:     cloud.Name,
		OwnerUsername: u.Username,
		AuthType:      "empty",
	}
	err = db.SetCloudCredential(ctx, &cred)
	c.Assert(err, qt.IsNil)
	env.credentials = []dbmodel.CloudCredential{cred}

	model := dbmodel.Model{
		Name: "test-model",
		UUID: sql.NullString{
			String: "00000000-0000-0000-0000-0000-0000000000003",
			Valid:  true,
		},
		OwnerUsername:     u.Username,
		ControllerID:      controller.ID,
		CloudRegionID:     cloud.Regions[0].ID,
		CloudCredentialID: cred.ID,
		Users: []dbmodel.UserModelAccess{{
			Username: u.Username,
			Access:   "admin",
		}},
	}
	err = db.AddModel(ctx, &model)
	c.Assert(err, qt.IsNil)
	env.models = []dbmodel.Model{model}

	// user u is administrator of the test-model
	err = openfga.NewUser(&u, client).SetModelAccess(ctx, model.ResourceTag(), ofganames.AdministratorRelation)
	c.Assert(err, qt.IsNil)

	err = client.AddControllerModel(context.Background(), controller.ResourceTag(), model.ResourceTag())
	c.Assert(err, qt.IsNil)

	offer := dbmodel.ApplicationOffer{
		ID:              1,
		UUID:            "00000000-0000-0000-0000-0000-0000000000011",
		URL:             "test-offer-url",
		Name:            "test-offer",
		ModelID:         model.ID,
		Model:           model,
		ApplicationName: "test-app",
		CharmURL:        "cs:test-app:17",
		Users: []dbmodel.UserApplicationOfferAccess{{
			Username: u1.Username,
			Access:   string(jujuparams.OfferAdminAccess),
		}, {
			Username: u2.Username,
			Access:   string(jujuparams.OfferConsumeAccess),
		}, {
			Username: u3.Username,
			Access:   string(jujuparams.OfferReadAccess),
		}, {
			Username: u5.Username,
			Access:   string(jujuparams.OfferAdminAccess),
		}},
	}
	err = db.AddApplicationOffer(ctx, &offer)
	c.Assert(err, qt.IsNil)
	env.applicationOffers = []dbmodel.ApplicationOffer{offer}

	err = client.AddModelApplicationOffer(context.Background(), model.ResourceTag(), offer.ResourceTag())
	c.Assert(err, qt.IsNil)

	// user u1 is administrator of the test-offer
	err = openfga.NewUser(&u1, client).SetApplicationOfferAccess(ctx, offer.ResourceTag(), ofganames.AdministratorRelation)
	c.Assert(err, qt.IsNil)

	// user u2 is consumer of the test-offer
	err = openfga.NewUser(&u2, client).SetApplicationOfferAccess(ctx, offer.ResourceTag(), ofganames.ConsumerRelation)
	c.Assert(err, qt.IsNil)

	// user u3 is reader of the test-offer
	err = openfga.NewUser(&u3, client).SetApplicationOfferAccess(ctx, offer.ResourceTag(), ofganames.ReaderRelation)
	c.Assert(err, qt.IsNil)

	// user u5 is administrator of the test-offer
	err = openfga.NewUser(&u5, client).SetApplicationOfferAccess(ctx, offer.ResourceTag(), ofganames.AdministratorRelation)
	c.Assert(err, qt.IsNil)

	return &env
}

/*
func TestRevokeOfferAccess(t *testing.T) {
	c := qt.New(t)

	ctx := context.Background()
	now := time.Now().UTC().Round(time.Millisecond)

	revokeErrorsChannel := make(chan error, 1)

	tests := []struct {
		about              string
		parameterFunc      func(*environment) (dbmodel.User, dbmodel.User, string, jujuparams.OfferAccessPermission)
		revokeError        string
		expectedError      string
		expectedAccesLevel string
	}{{
		about:       "controller returns an error",
		revokeError: "a silly error",
		parameterFunc: func(env *environment) (dbmodel.User, dbmodel.User, string, jujuparams.OfferAccessPermission) {
			return env.users[0], env.users[1], "test-offer-url", jujuparams.OfferAdminAccess
		},
		expectedError: "a silly error",
	}, {
		about: "model admin revokes an admin user admin access - user keeps consume access",
		parameterFunc: func(env *environment) (dbmodel.User, dbmodel.User, string, jujuparams.OfferAccessPermission) {
			return env.users[0], env.users[1], "test-offer-url", jujuparams.OfferAdminAccess
		},
		expectedAccesLevel: "consume",
	}, {
		about: "admin revokes an admin user admin access - user keeps consume access",
		parameterFunc: func(env *environment) (dbmodel.User, dbmodel.User, string, jujuparams.OfferAccessPermission) {
			return env.users[5], env.users[1], "test-offer-url", jujuparams.OfferAdminAccess
		},
		expectedAccesLevel: "consume",
	}, {
		about: "superuser revokes an admin user admin access - user keeps consume access",
		parameterFunc: func(env *environment) (dbmodel.User, dbmodel.User, string, jujuparams.OfferAccessPermission) {
			return env.users[6], env.users[1], "test-offer-url", jujuparams.OfferAdminAccess
		},
		expectedAccesLevel: "consume",
	}, {
		about: "admin revokes an admin user consume access - user keeps read access",
		parameterFunc: func(env *environment) (dbmodel.User, dbmodel.User, string, jujuparams.OfferAccessPermission) {
			return env.users[0], env.users[1], "test-offer-url", jujuparams.OfferConsumeAccess
		},
		expectedAccesLevel: "read",
	}, {
		about: "admin revokes an admin user read access - user has no access",
		parameterFunc: func(env *environment) (dbmodel.User, dbmodel.User, string, jujuparams.OfferAccessPermission) {
			return env.users[0], env.users[1], "test-offer-url", jujuparams.OfferReadAccess
		},
		expectedAccesLevel: "",
	}, {
		about: "admin revokes a consume user admin access - user keeps consume access",
		parameterFunc: func(env *environment) (dbmodel.User, dbmodel.User, string, jujuparams.OfferAccessPermission) {
			return env.users[0], env.users[2], "test-offer-url", jujuparams.OfferAdminAccess
		},
		expectedAccesLevel: "consume",
	}, {
		about: "admin revokes a consume user consume access - user keeps read access",
		parameterFunc: func(env *environment) (dbmodel.User, dbmodel.User, string, jujuparams.OfferAccessPermission) {
			return env.users[0], env.users[2], "test-offer-url", jujuparams.OfferConsumeAccess
		},
		expectedAccesLevel: "read",
	}, {
		about: "admin revokes a consume user read access - user has no access",
		parameterFunc: func(env *environment) (dbmodel.User, dbmodel.User, string, jujuparams.OfferAccessPermission) {
			return env.users[0], env.users[2], "test-offer-url", jujuparams.OfferReadAccess
		},
		expectedAccesLevel: "",
	}, {
		about: "admin revokes a read user admin access - user keeps read access",
		parameterFunc: func(env *environment) (dbmodel.User, dbmodel.User, string, jujuparams.OfferAccessPermission) {
			return env.users[0], env.users[3], "test-offer-url", jujuparams.OfferAdminAccess
		},
		expectedAccesLevel: "read",
	}, {
		about: "admin revokes a read user consume access - user keeps read access",
		parameterFunc: func(env *environment) (dbmodel.User, dbmodel.User, string, jujuparams.OfferAccessPermission) {
			return env.users[0], env.users[3], "test-offer-url", jujuparams.OfferConsumeAccess
		},
		expectedAccesLevel: "read",
	}, {
		about: "admin revokes a read user read access - user has no access",
		parameterFunc: func(env *environment) (dbmodel.User, dbmodel.User, string, jujuparams.OfferAccessPermission) {
			return env.users[0], env.users[3], "test-offer-url", jujuparams.OfferReadAccess
		},
		expectedAccesLevel: "",
	}, {
		about: "admin tries to revoke access to use that does not have access",
		parameterFunc: func(env *environment) (dbmodel.User, dbmodel.User, string, jujuparams.OfferAccessPermission) {
			return env.users[0], env.users[4], "test-offer-url", jujuparams.OfferReadAccess
		},
		expectedAccesLevel: "",
	}, {
		about: "user with consume access cannot revoke access",
		parameterFunc: func(env *environment) (dbmodel.User, dbmodel.User, string, jujuparams.OfferAccessPermission) {
			return env.users[2], env.users[3], "test-offer-url", jujuparams.OfferReadAccess
		},
		expectedError: "unauthorized",
	}, {
		about: "user with read access cannot revoke access",
		parameterFunc: func(env *environment) (dbmodel.User, dbmodel.User, string, jujuparams.OfferAccessPermission) {
			return env.users[3], env.users[3], "test-offer-url", jujuparams.OfferReadAccess
		},
		expectedError: "unauthorized",
	}, {
		about: "no such offer",
		parameterFunc: func(env *environment) (dbmodel.User, dbmodel.User, string, jujuparams.OfferAccessPermission) {
			return env.users[3], env.users[3], "no-such-offer", jujuparams.OfferReadAccess
		},
		expectedError: "application offer not found",
	}}

	for _, test := range tests {
		c.Run(test.about, func(c *qt.C) {

			db := db.Database{
				DB: jimmtest.MemoryDB(c, func() time.Time { return now }),
			}
			err := db.Migrate(ctx, false)
			c.Assert(err, qt.IsNil)

			environment := initializeEnvironment(c, ctx, &db)
			authenticatedUser, offerUser, offerURL, revokeAccessLevel := test.parameterFunc(environment)

			j := &jimm.JIMM{
				Database: db,
				Dialer: &jimmtest.Dialer{
					API: &jimmtest.API{
						RevokeApplicationOfferAccess_: func(context.Context, string, names.UserTag, jujuparams.OfferAccessPermission) error {
							select {
							case err := <-revokeErrorsChannel:
								return err
							default:
								return nil
							}
						},
					},
				},
			}

			if test.revokeError != "" {
				select {
				case revokeErrorsChannel <- errors.E(test.revokeError):
				default:
				}
			}
			err = j.RevokeOfferAccess(ctx, &authenticatedUser, offerURL, offerUser.ResourceTag(), revokeAccessLevel)
			if test.expectedError == "" {
				c.Assert(err, qt.IsNil)

				offer := dbmodel.ApplicationOffer{
					URL: offerURL,
				}
				err = db.GetApplicationOffer(ctx, &offer)
				c.Assert(err, qt.IsNil)
				c.Assert(offer.UserAccess(&offerUser), qt.Equals, test.expectedAccesLevel)
			} else {
				c.Assert(err, qt.ErrorMatches, test.expectedError)
			}
		})
	}
}

func TestGrantOfferAccess(t *testing.T) {
	c := qt.New(t)

	ctx := context.Background()
	now := time.Now().UTC().Round(time.Millisecond)

	grantErrorsChannel := make(chan error, 1)

	tests := []struct {
		about              string
		parameterFunc      func(*environment) (dbmodel.User, dbmodel.User, string, jujuparams.OfferAccessPermission)
		grantError         string
		expectedError      string
		expectedAccesLevel string
	}{{
		about:      "controller returns an error",
		grantError: "a silly error",
		parameterFunc: func(env *environment) (dbmodel.User, dbmodel.User, string, jujuparams.OfferAccessPermission) {
			return env.users[0], env.users[1], "test-offer-url", jujuparams.OfferAdminAccess
		},
		expectedError: "a silly error",
	}, {
		about: "model admin grants an admin user admin access - admin user keeps admin",
		parameterFunc: func(env *environment) (dbmodel.User, dbmodel.User, string, jujuparams.OfferAccessPermission) {
			return env.users[0], env.users[1], "test-offer-url", jujuparams.OfferAdminAccess
		},
		expectedAccesLevel: "admin",
	}, {
		about: "model admin grants an admin user consume access - admin user keeps admin",
		parameterFunc: func(env *environment) (dbmodel.User, dbmodel.User, string, jujuparams.OfferAccessPermission) {
			return env.users[0], env.users[1], "test-offer-url", jujuparams.OfferConsumeAccess
		},
		expectedAccesLevel: "admin",
	}, {
		about: "model admin grants an admin user read access - admin user keeps admin",
		parameterFunc: func(env *environment) (dbmodel.User, dbmodel.User, string, jujuparams.OfferAccessPermission) {
			return env.users[0], env.users[1], "test-offer-url", jujuparams.OfferReadAccess
		},
		expectedAccesLevel: "admin",
	}, {
		about: "model admin grants a consume user admin access - user gets admin access",
		parameterFunc: func(env *environment) (dbmodel.User, dbmodel.User, string, jujuparams.OfferAccessPermission) {
			return env.users[0], env.users[2], "test-offer-url", jujuparams.OfferAdminAccess
		},
		expectedAccesLevel: "admin",
	}, {
		about: "admin grants a consume user admin access - user gets admin access",
		parameterFunc: func(env *environment) (dbmodel.User, dbmodel.User, string, jujuparams.OfferAccessPermission) {
			return env.users[5], env.users[2], "test-offer-url", jujuparams.OfferAdminAccess
		},
		expectedAccesLevel: "admin",
	}, {
		about: "superuser grants a consume user admin access - user gets admin access",
		parameterFunc: func(env *environment) (dbmodel.User, dbmodel.User, string, jujuparams.OfferAccessPermission) {
			return env.users[6], env.users[2], "test-offer-url", jujuparams.OfferAdminAccess
		},
		expectedAccesLevel: "admin",
	}, {
		about: "admin grants a consume user consume access - user keeps consume access",
		parameterFunc: func(env *environment) (dbmodel.User, dbmodel.User, string, jujuparams.OfferAccessPermission) {
			return env.users[0], env.users[2], "test-offer-url", jujuparams.OfferConsumeAccess
		},
		expectedAccesLevel: "consume",
	}, {
		about: "admin grants a consume user read access - use keeps consume access",
		parameterFunc: func(env *environment) (dbmodel.User, dbmodel.User, string, jujuparams.OfferAccessPermission) {
			return env.users[0], env.users[2], "test-offer-url", jujuparams.OfferReadAccess
		},
		expectedAccesLevel: "consume",
	}, {
		about: "admin grants a read user admin access - user gets admin access",
		parameterFunc: func(env *environment) (dbmodel.User, dbmodel.User, string, jujuparams.OfferAccessPermission) {
			return env.users[0], env.users[3], "test-offer-url", jujuparams.OfferAdminAccess
		},
		expectedAccesLevel: "admin",
	}, {
		about: "admin grants a read user consume access - user gets consume access",
		parameterFunc: func(env *environment) (dbmodel.User, dbmodel.User, string, jujuparams.OfferAccessPermission) {
			return env.users[0], env.users[3], "test-offer-url", jujuparams.OfferConsumeAccess
		},
		expectedAccesLevel: "consume",
	}, {
		about: "admin grants a read user read access - user keeps read access",
		parameterFunc: func(env *environment) (dbmodel.User, dbmodel.User, string, jujuparams.OfferAccessPermission) {
			return env.users[0], env.users[3], "test-offer-url", jujuparams.OfferReadAccess
		},
		expectedAccesLevel: "read",
	}, {
		about: "no such offer",
		parameterFunc: func(env *environment) (dbmodel.User, dbmodel.User, string, jujuparams.OfferAccessPermission) {
			return env.users[0], env.users[3], "no-such-offer", jujuparams.OfferReadAccess
		},
		expectedError: "application offer not found",
	}, {
		about: "user with consume rights cannot grant any rights",
		parameterFunc: func(env *environment) (dbmodel.User, dbmodel.User, string, jujuparams.OfferAccessPermission) {
			return env.users[2], env.users[4], "test-offer-url", jujuparams.OfferConsumeAccess
		},
		expectedError: "unauthorized",
	}, {
		about: "user with read rights cannot grant any rights",
		parameterFunc: func(env *environment) (dbmodel.User, dbmodel.User, string, jujuparams.OfferAccessPermission) {
			return env.users[3], env.users[4], "test-offer-url", jujuparams.OfferConsumeAccess
		},
		expectedError: "unauthorized",
	}, {
		about: "admin grants new user admin access - new user has admin access",
		parameterFunc: func(env *environment) (dbmodel.User, dbmodel.User, string, jujuparams.OfferAccessPermission) {
			return env.users[0], env.users[4], "test-offer-url", jujuparams.OfferAdminAccess
		},
		expectedAccesLevel: "admin",
	}, {
		about: "admin grants new user consume access - new user has consume access",
		parameterFunc: func(env *environment) (dbmodel.User, dbmodel.User, string, jujuparams.OfferAccessPermission) {
			return env.users[0], env.users[4], "test-offer-url", jujuparams.OfferConsumeAccess
		},
		expectedAccesLevel: "consume",
	}, {
		about: "admin grants new user read access - new user has read access",
		parameterFunc: func(env *environment) (dbmodel.User, dbmodel.User, string, jujuparams.OfferAccessPermission) {
			return env.users[0], env.users[4], "test-offer-url", jujuparams.OfferReadAccess
		},
		expectedAccesLevel: "read",
	}}

	for _, test := range tests {
		c.Run(test.about, func(c *qt.C) {

			db := db.Database{
				DB: jimmtest.MemoryDB(c, func() time.Time { return now }),
			}
			err := db.Migrate(ctx, false)
			c.Assert(err, qt.IsNil)

			environment := initializeEnvironment(c, ctx, &db)
			authenticatedUser, offerUser, offerURL, grantAccessLevel := test.parameterFunc(environment)

			j := &jimm.JIMM{
				Database: db,
				Dialer: &jimmtest.Dialer{
					API: &jimmtest.API{
						GrantApplicationOfferAccess_: func(context.Context, string, names.UserTag, jujuparams.OfferAccessPermission) error {
							select {
							case err := <-grantErrorsChannel:
								return err
							default:
								return nil
							}
						},
					},
				},
			}

			if test.grantError != "" {
				select {
				case grantErrorsChannel <- errors.E(test.grantError):
				default:
				}
			}
			err = j.GrantOfferAccess(ctx, &authenticatedUser, offerURL, offerUser.ResourceTag(), grantAccessLevel)
			if test.expectedError == "" {
				c.Assert(err, qt.IsNil)

				offer := dbmodel.ApplicationOffer{
					URL: offerURL,
				}
				err = db.GetApplicationOffer(ctx, &offer)
				c.Assert(err, qt.IsNil)
				c.Assert(offer.UserAccess(&offerUser), qt.Equals, test.expectedAccesLevel)
			} else {
				c.Assert(err, qt.ErrorMatches, test.expectedError)
			}
		})
	}
}
*/

func TestGetApplicationOfferConsumeDetails(t *testing.T) {
	c := qt.New(t)

	_, client, _, err := jimmtest.SetupTestOFGAClient(c.Name())
	c.Assert(err, qt.IsNil)

	ctx := context.Background()
	now := time.Now().UTC().Round(time.Millisecond)

	db := db.Database{
		DB: jimmtest.MemoryDB(c, func() time.Time { return now }),
	}
	err = db.Migrate(ctx, false)
	c.Assert(err, qt.IsNil)

	u := dbmodel.User{
		Username:         "alice@external",
		ControllerAccess: "superuser",
	}
	c.Assert(db.DB.Create(&u).Error, qt.IsNil)

	u1 := dbmodel.User{
		Username:         "eve@external",
		ControllerAccess: "superuser",
	}
	c.Assert(db.DB.Create(&u1).Error, qt.IsNil)

	u2 := dbmodel.User{
		Username:         "bob@external",
		ControllerAccess: "superuser",
	}
	c.Assert(db.DB.Create(&u2).Error, qt.IsNil)

	cloud := dbmodel.Cloud{
		Name: "test-cloud",
		Type: "test-provider",
		Regions: []dbmodel.CloudRegion{{
			Name: "test-region-1",
		}},
	}
	c.Assert(db.DB.Create(&cloud).Error, qt.IsNil)

	// user u is administrator of the test-model
	err = openfga.NewUser(&u, client).SetCloudAccess(ctx, cloud.ResourceTag(), ofganames.AdministratorRelation)
	c.Assert(err, qt.IsNil)

	controller := dbmodel.Controller{
		Name:          "test-controller-1",
		UUID:          "00000000-0000-0000-0000-0000-0000000000001",
		PublicAddress: "test-public-address",
		CACertificate: "test-ca-cert",
		CloudName:     "test-cloud",
		CloudRegion:   "test-region-1",
		CloudRegions: []dbmodel.CloudRegionControllerPriority{{
			Priority:      0,
			CloudRegionID: cloud.Regions[0].ID,
		}},
	}
	err = db.AddController(ctx, &controller)
	c.Assert(err, qt.IsNil)

	cred := dbmodel.CloudCredential{
		Name:          "test-credential-1",
		CloudName:     cloud.Name,
		OwnerUsername: u.Username,
		AuthType:      "empty",
	}
	err = db.SetCloudCredential(ctx, &cred)
	c.Assert(err, qt.IsNil)

	model := dbmodel.Model{
		Name: "test-model",
		UUID: sql.NullString{
			String: "00000000-0000-0000-0000-0000-0000000000003",
			Valid:  true,
		},
		OwnerUsername:     u.Username,
		ControllerID:      controller.ID,
		CloudRegionID:     cloud.Regions[0].ID,
		CloudCredentialID: cred.ID,
	}
	err = db.AddModel(ctx, &model)
	c.Assert(err, qt.IsNil)

	offer := dbmodel.ApplicationOffer{
		ID:              1,
		UUID:            uuid.NewString(),
		URL:             "test-offer-url",
		ModelID:         model.ID,
		Model:           model,
		ApplicationName: "test-app",
		CharmURL:        "cs:test-app:17",
	}
	err = db.AddApplicationOffer(ctx, &offer)
	c.Assert(err, qt.IsNil)

	// user u is administrator of the test offer
	err = openfga.NewUser(&u, client).SetApplicationOfferAccess(ctx, offer.ResourceTag(), ofganames.AdministratorRelation)
	c.Assert(err, qt.IsNil)

	// user u1 is reader of the test offer
	err = openfga.NewUser(&u1, client).SetApplicationOfferAccess(ctx, offer.ResourceTag(), ofganames.ReaderRelation)
	c.Assert(err, qt.IsNil)

	// user u2 is consumer of the test offer
	err = openfga.NewUser(&u2, client).SetApplicationOfferAccess(ctx, offer.ResourceTag(), ofganames.ConsumerRelation)
	c.Assert(err, qt.IsNil)

	j := &jimm.JIMM{
		UUID:          uuid.NewString(),
		OpenFGAClient: client,
		Database:      db,
		Dialer: &jimmtest.Dialer{
			UUID: "00000000-0000-0000-0000-0000-0000000000001",
			API: &jimmtest.API{
				GetApplicationOfferConsumeDetails_: func(ctx context.Context, user names.UserTag, details *jujuparams.ConsumeOfferDetails, v bakery.Version) error {
					details.Offer = &jujuparams.ApplicationOfferDetails{
						SourceModelTag:         names.NewModelTag(model.UUID.String).String(),
						OfferUUID:              offer.UUID,
						OfferURL:               offer.URL,
						OfferName:              offer.Name,
						ApplicationDescription: offer.ApplicationDescription,
						Endpoints: []jujuparams.RemoteEndpoint{{
							Name:      "test-endpoint",
							Role:      "requirer",
							Interface: "unknown",
							Limit:     1,
						}},
						Bindings: map[string]string{
							"key1": "value1",
							"key2": "value2",
						},
						Users: []jujuparams.OfferUserDetails{{
							UserName: "alice@external",
							Access:   "admin",
						}, {
							UserName: "eve@external",
							Access:   "read",
						}, {
							UserName: "bob@external",
							Access:   "consume",
						}},
						Spaces: []jujuparams.RemoteSpace{{
							CloudType:  "test-cloud-type",
							Name:       "test-remote-space",
							ProviderId: "test-provider-id",
							ProviderAttributes: map[string]interface{}{
								"attr1": "value1",
								"attr2": "value2",
							},
						}},
					}
					details.Macaroon = &macaroon.Macaroon{}
					details.ControllerInfo = &jujuparams.ExternalControllerInfo{
						ControllerTag: names.NewControllerTag(controller.UUID).String(),
					}
					return nil
				},
			},
		},
	}

	tests := []struct {
		about                string
		user                 *dbmodel.User
		details              jujuparams.ConsumeOfferDetails
		expectedOfferDetails jujuparams.ConsumeOfferDetails
		expectedError        string
	}{{
		about: "admin can get the application offer consume details ",
		user:  &u,
		details: jujuparams.ConsumeOfferDetails{
			Offer: &jujuparams.ApplicationOfferDetails{
				OfferURL: "test-offer-url",
			},
		},
		expectedOfferDetails: jujuparams.ConsumeOfferDetails{
			ControllerInfo: &jujuparams.ExternalControllerInfo{
				ControllerTag: names.NewControllerTag(controller.UUID).String(),
				Alias:         "test-controller-1",
				Addrs:         []string{"test-public-address"},
			},
			Macaroon: &macaroon.Macaroon{},
			Offer: &jujuparams.ApplicationOfferDetails{
				SourceModelTag:         names.NewModelTag(model.UUID.String).String(),
				OfferUUID:              offer.UUID,
				OfferURL:               offer.URL,
				OfferName:              offer.Name,
				ApplicationDescription: offer.ApplicationDescription,
				Endpoints: []jujuparams.RemoteEndpoint{{
					Name:      "test-endpoint",
					Role:      "requirer",
					Interface: "unknown",
					Limit:     1,
				}},
				Bindings: map[string]string{
					"key1": "value1",
					"key2": "value2",
				},
				Users: []jujuparams.OfferUserDetails{{
					UserName: "alice@external",
					Access:   "admin",
				}, {
					UserName: "bob@external",
					Access:   "consume",
				}, {
					UserName: "eve@external",
					Access:   "read",
				}},
				Spaces: []jujuparams.RemoteSpace{{
					CloudType:  "test-cloud-type",
					Name:       "test-remote-space",
					ProviderId: "test-provider-id",
					ProviderAttributes: map[string]interface{}{
						"attr1": "value1",
						"attr2": "value2",
					},
				}},
			},
		},
	}, {
		about: "users with consume access can get the application offer consume details with filtered users",
		user:  &u2,
		details: jujuparams.ConsumeOfferDetails{
			Offer: &jujuparams.ApplicationOfferDetails{
				OfferURL: "test-offer-url",
			},
		},
		expectedOfferDetails: jujuparams.ConsumeOfferDetails{
			ControllerInfo: &jujuparams.ExternalControllerInfo{
				ControllerTag: names.NewControllerTag(controller.UUID).String(),
				Alias:         "test-controller-1",
				Addrs:         []string{"test-public-address"},
			},
			Macaroon: &macaroon.Macaroon{},
			Offer: &jujuparams.ApplicationOfferDetails{
				SourceModelTag:         names.NewModelTag(model.UUID.String).String(),
				OfferUUID:              offer.UUID,
				OfferURL:               offer.URL,
				OfferName:              offer.Name,
				ApplicationDescription: offer.ApplicationDescription,
				Endpoints: []jujuparams.RemoteEndpoint{{
					Name:      "test-endpoint",
					Role:      "requirer",
					Interface: "unknown",
					Limit:     1,
				}},
				Bindings: map[string]string{
					"key1": "value1",
					"key2": "value2",
				},
				Users: []jujuparams.OfferUserDetails{{
					UserName: "bob@external",
					Access:   "consume",
				}},
				Spaces: []jujuparams.RemoteSpace{{
					CloudType:  "test-cloud-type",
					Name:       "test-remote-space",
					ProviderId: "test-provider-id",
					ProviderAttributes: map[string]interface{}{
						"attr1": "value1",
						"attr2": "value2",
					},
				}},
			},
		},
	}, {
		about: "user with read access cannot get application offer consume details",
		user:  &u1,
		details: jujuparams.ConsumeOfferDetails{
			Offer: &jujuparams.ApplicationOfferDetails{
				OfferURL: "test-offer-url",
			},
		},
		expectedError: "unauthorized",
	}, {
		about: "no such offer",
		user:  &u,
		details: jujuparams.ConsumeOfferDetails{
			Offer: &jujuparams.ApplicationOfferDetails{
				OfferURL: "no-such-offer",
			},
		},
		expectedError: "application offer not found",
	}}

	for _, test := range tests {
		c.Run(test.about, func(c *qt.C) {
			err := j.GetApplicationOfferConsumeDetails(ctx, openfga.NewUser(test.user, client), &test.details, bakery.Version3)
			if test.expectedError == "" {
				c.Assert(err, qt.IsNil)
				sort.Slice(test.details.Offer.Users, func(i, j int) bool {
					return test.details.Offer.Users[i].UserName < test.details.Offer.Users[j].UserName
				})
				c.Assert(test.details, qt.CmpEquals(cmpopts.EquateEmpty(), cmpopts.IgnoreTypes(time.Time{})), test.expectedOfferDetails)
			} else {
				c.Assert(err, qt.ErrorMatches, test.expectedError)
			}
		})
	}
}

func TestGetApplicationOffer(t *testing.T) {
	c := qt.New(t)

	ctx := context.Background()
	now := time.Now().UTC().Round(time.Millisecond)

	_, client, _, err := jimmtest.SetupTestOFGAClient(c.Name())
	c.Assert(err, qt.IsNil)

	j := &jimm.JIMM{
		UUID:          uuid.NewString(),
		OpenFGAClient: client,
		Database: db.Database{
			DB: jimmtest.MemoryDB(c, func() time.Time { return now }),
		},
		Dialer: &jimmtest.Dialer{
			API: &jimmtest.API{
				GetApplicationOffer_: func(_ context.Context, details *jujuparams.ApplicationOfferAdminDetails) error {
					details.ApplicationName = "test-app"
					details.CharmURL = "cs:test-app:17"
					details.Connections = []jujuparams.OfferConnection{{
						SourceModelTag: "test-model-src",
						RelationId:     1,
						Username:       "unknown",
						Endpoint:       "test-endpoint",
					}}
					details.ApplicationOfferDetails = jujuparams.ApplicationOfferDetails{
						SourceModelTag:         names.NewModelTag("00000000-0000-0000-0000-0000-0000000000003").String(),
						OfferUUID:              "00000000-0000-0000-0000-0000-0000000000011",
						OfferURL:               "test-offer-url",
						ApplicationDescription: "changed offer description",
						Endpoints: []jujuparams.RemoteEndpoint{{
							Name:      "test-endpoint",
							Role:      charm.RoleRequirer,
							Interface: "unknown",
							Limit:     1,
						}},
						Spaces: []jujuparams.RemoteSpace{{
							CloudType:  "test-cloud-type",
							Name:       "test-remote-space",
							ProviderId: "test-provider-id",
							ProviderAttributes: map[string]interface{}{
								"attr1": "value1",
								"attr2": "value2",
							},
							Subnets: []jujuparams.Subnet{{
								SpaceTag: "test-remote-space",
								VLANTag:  1024,
								Status:   "dead",
							}},
						}},
						Bindings: map[string]string{
							"key1": "value4",
							"key2": "value5",
						},
						Users: []jujuparams.OfferUserDetails{{
							UserName: "alice@external",
							Access:   string(jujuparams.OfferAdminAccess),
						}, {
							UserName: "eve@external",
							Access:   "read",
						}, {
							UserName: "admin",
							Access:   "admin",
						}},
					}
					return nil
				},
			},
		},
	}

	err = j.Database.Migrate(ctx, false)
	c.Assert(err, qt.IsNil)

	u := dbmodel.User{
		Username:         "alice@external",
		ControllerAccess: "superuser",
	}
	c.Assert(j.Database.DB.Create(&u).Error, qt.IsNil)

	u1 := dbmodel.User{
		Username:         "eve@external",
		ControllerAccess: "superuser",
	}
	c.Assert(j.Database.DB.Create(&u1).Error, qt.IsNil)

	u2 := dbmodel.User{
		Username:         "bob@external",
		ControllerAccess: "superuser",
	}
	c.Assert(j.Database.DB.Create(&u2).Error, qt.IsNil)

	cloud := dbmodel.Cloud{
		Name: "test-cloud",
		Type: "test-provider",
		Regions: []dbmodel.CloudRegion{{
			Name: "test-region-1",
		}},
		Users: []dbmodel.UserCloudAccess{{
			Username: u.Username,
		}},
	}
	c.Assert(j.Database.DB.Create(&cloud).Error, qt.IsNil)

	controller := dbmodel.Controller{
		Name:        "test-controller-1",
		UUID:        "00000000-0000-0000-0000-0000-0000000000001",
		CloudName:   "test-cloud",
		CloudRegion: "test-region-1",
		CloudRegions: []dbmodel.CloudRegionControllerPriority{{
			Priority:      0,
			CloudRegionID: cloud.Regions[0].ID,
		}},
	}
	err = j.Database.AddController(ctx, &controller)
	c.Assert(err, qt.IsNil)

	cred := dbmodel.CloudCredential{
		Name:          "test-credential-1",
		CloudName:     cloud.Name,
		OwnerUsername: u.Username,
		AuthType:      "empty",
	}
	err = j.Database.SetCloudCredential(ctx, &cred)
	c.Assert(err, qt.IsNil)

	model := dbmodel.Model{
		Name: "test-model",
		UUID: sql.NullString{
			String: "00000000-0000-0000-0000-0000-0000000000003",
			Valid:  true,
		},
		OwnerUsername:     u.Username,
		ControllerID:      controller.ID,
		CloudRegionID:     cloud.Regions[0].ID,
		CloudCredentialID: cred.ID,
	}
	err = j.Database.AddModel(ctx, &model)
	c.Assert(err, qt.IsNil)

	offer := dbmodel.ApplicationOffer{
		ID:                     1,
		ModelID:                1,
		ApplicationName:        "test-app",
		CharmURL:               "cs:test-app:17",
		ApplicationDescription: "a test app offering",
		Name:                   "test-application-offer",
		UUID:                   "00000000-0000-0000-0000-0000-0000000000004",
		URL:                    "test-offer-url",
		Endpoints: []dbmodel.ApplicationOfferRemoteEndpoint{{
			ApplicationOfferID: 1,
			Name:               "test-endpoint",
			Role:               "requirer",
			Interface:          "unknown",
			Limit:              1,
		}},
		Spaces: []dbmodel.ApplicationOfferRemoteSpace{{
			ApplicationOfferID: 1,
			CloudType:          "test-cloud-type",
			Name:               "test-remote-space",
			ProviderID:         "test-provider-id",
			ProviderAttributes: dbmodel.Map{
				"attr1": "value1",
				"attr2": "value2",
			},
		}},
		Bindings: dbmodel.StringMap{
			"key1": "value1",
			"key2": "value2",
		},
		Connections: []dbmodel.ApplicationOfferConnection{{
			ApplicationOfferID: 1,
			SourceModelTag:     "test-model-src",
			RelationID:         1,
			Username:           "unknown",
			Endpoint:           "test-endpoint",
		}},
		Users: []dbmodel.UserApplicationOfferAccess{{
			Username: u.Username,
			Access:   string(jujuparams.OfferAdminAccess),
		}, {
			Username: u1.Username,
			Access:   string(jujuparams.OfferReadAccess),
		}},
	}
	err = j.Database.AddApplicationOffer(ctx, &offer)
	c.Assert(err, qt.IsNil)

	// user u is administrator of the test offer
	err = openfga.NewUser(&u, client).SetApplicationOfferAccess(ctx, offer.ResourceTag(), ofganames.AdministratorRelation)
	c.Assert(err, qt.IsNil)

	// user u1 is reader of the test offer
	err = openfga.NewUser(&u1, client).SetApplicationOfferAccess(ctx, offer.ResourceTag(), ofganames.ReaderRelation)
	c.Assert(err, qt.IsNil)

	tests := []struct {
		about                string
		user                 *dbmodel.User
		offerURL             string
		expectedOfferDetails jujuparams.ApplicationOfferAdminDetails
		expectedError        string
	}{{
		about:    "admin can get the application offer",
		user:     &u,
		offerURL: "test-offer-url",
		expectedOfferDetails: jujuparams.ApplicationOfferAdminDetails{
			ApplicationOfferDetails: jujuparams.ApplicationOfferDetails{
				SourceModelTag:         names.NewModelTag(model.UUID.String).String(),
				OfferUUID:              "00000000-0000-0000-0000-0000-0000000000011",
				OfferURL:               "test-offer-url",
				ApplicationDescription: "changed offer description",
				Endpoints: []jujuparams.RemoteEndpoint{{
					Name:      "test-endpoint",
					Role:      "requirer",
					Interface: "unknown",
					Limit:     1,
				}},
				Bindings: map[string]string{
					"key1": "value4",
					"key2": "value5",
				},
				Users: []jujuparams.OfferUserDetails{{
					UserName: "alice@external",
					Access:   "admin",
				}, {
					UserName: "eve@external",
					Access:   "read",
				}},
				Spaces: []jujuparams.RemoteSpace{{
					CloudType:  "test-cloud-type",
					Name:       "test-remote-space",
					ProviderId: "test-provider-id",
					ProviderAttributes: map[string]interface{}{
						"attr1": "value1",
						"attr2": "value2",
					},
					Subnets: []jujuparams.Subnet{{
						SpaceTag: "test-remote-space",
						VLANTag:  1024,
						Status:   "dead",
					}},
				}},
			},
			ApplicationName: "test-app",
			CharmURL:        "cs:test-app:17",
			Connections: []jujuparams.OfferConnection{{
				SourceModelTag: "test-model-src",
				RelationId:     1,
				Username:       "unknown",
				Endpoint:       "test-endpoint",
			}},
		},
	}, {
		about:    "user with read access can get the application offer, but users and connections are filtered",
		user:     &u1,
		offerURL: "test-offer-url",
		expectedOfferDetails: jujuparams.ApplicationOfferAdminDetails{
			ApplicationOfferDetails: jujuparams.ApplicationOfferDetails{
				SourceModelTag:         names.NewModelTag(model.UUID.String).String(),
				OfferUUID:              "00000000-0000-0000-0000-0000-0000000000011",
				OfferURL:               "test-offer-url",
				ApplicationDescription: "changed offer description",
				Endpoints: []jujuparams.RemoteEndpoint{{
					Name:      "test-endpoint",
					Role:      "requirer",
					Interface: "unknown",
					Limit:     1,
				}},
				Bindings: map[string]string{
					"key1": "value4",
					"key2": "value5",
				},
				Users: []jujuparams.OfferUserDetails{{
					UserName: "eve@external",
					Access:   "read",
				}},
				Spaces: []jujuparams.RemoteSpace{{
					CloudType:  "test-cloud-type",
					Name:       "test-remote-space",
					ProviderId: "test-provider-id",
					ProviderAttributes: map[string]interface{}{
						"attr1": "value1",
						"attr2": "value2",
					},
					Subnets: []jujuparams.Subnet{{
						SpaceTag: "test-remote-space",
						VLANTag:  1024,
						Status:   "dead",
					}},
				}},
			},
			ApplicationName: "test-app",
			CharmURL:        "cs:test-app:17",
		},
	}, {
		about:         "user without access cannot get the application offer",
		user:          &u2,
		offerURL:      "test-offer-url",
		expectedError: "application offer not found",
	}, {
		about:         "not found",
		user:          &u1,
		offerURL:      "offer-not-found",
		expectedError: "application offer not found",
	}}

	for _, test := range tests {
		c.Run(test.about, func(c *qt.C) {
			details, err := j.GetApplicationOffer(ctx, openfga.NewUser(test.user, client), test.offerURL)
			if test.expectedError == "" {
				c.Assert(err, qt.IsNil)
				sort.Slice(details.Users, func(i, j int) bool {
					return details.Users[i].UserName < details.Users[j].UserName
				})
				c.Assert(details, qt.CmpEquals(cmpopts.EquateEmpty(), cmpopts.IgnoreTypes(time.Time{})), &test.expectedOfferDetails)
			} else {
				c.Assert(err, qt.ErrorMatches, test.expectedError)
			}
		})
	}
}

func TestOffer(t *testing.T) {
	c := qt.New(t)

	now := time.Now().UTC().Round(time.Millisecond)
	tests := []struct {
		about                       string
		getApplicationOffer         func(context.Context, *jujuparams.ApplicationOfferAdminDetails) error
		grantApplicationOfferAccess func(context.Context, string, names.UserTag, jujuparams.OfferAccessPermission) error
		offer                       func(context.Context, crossmodel.OfferURL, jujuparams.AddApplicationOffer) error
		createEnv                   func(*qt.C, db.Database, *openfga.OFGAClient) (dbmodel.User, jimm.AddApplicationOfferParams, dbmodel.ApplicationOffer, func(*qt.C, error))
	}{{
		about: "all ok",
		getApplicationOffer: func(_ context.Context, details *jujuparams.ApplicationOfferAdminDetails) error {
			details.ApplicationName = "test-app"
			details.CharmURL = "cs:test-app:17"
			details.Connections = []jujuparams.OfferConnection{{
				SourceModelTag: "test-model-src",
				RelationId:     1,
				Username:       "unknown",
				Endpoint:       "test-endpoint",
			}}
			details.ApplicationOfferDetails = jujuparams.ApplicationOfferDetails{
				OfferUUID:              "00000000-0000-0000-0000-0000-0000000000004",
				OfferURL:               "test-offer-url",
				ApplicationDescription: "a test app offering",
				Endpoints: []jujuparams.RemoteEndpoint{{
					Name:      "test-endpoint",
					Role:      charm.RoleRequirer,
					Interface: "unknown",
					Limit:     1,
				}},
				Spaces: []jujuparams.RemoteSpace{{
					CloudType:  "test-cloud-type",
					Name:       "test-remote-space",
					ProviderId: "test-provider-id",
					ProviderAttributes: map[string]interface{}{
						"attr1": "value1",
						"attr2": "value2",
					},
					Subnets: []jujuparams.Subnet{{
						SpaceTag: "test-remote-space",
						VLANTag:  1024,
						Status:   "alive",
					}},
				}},
				Bindings: map[string]string{
					"key1": "value1",
					"key2": "value2",
				},
				Users: []jujuparams.OfferUserDetails{{
					UserName:    "alice",
					DisplayName: "alice, sister of eve",
					Access:      string(jujuparams.OfferAdminAccess),
				}},
			}
			return nil
		},
		grantApplicationOfferAccess: func(context.Context, string, names.UserTag, jujuparams.OfferAccessPermission) error {
			return nil
		},
		offer: func(context.Context, crossmodel.OfferURL, jujuparams.AddApplicationOffer) error {
			return nil
		},
		createEnv: func(c *qt.C, db db.Database, client *openfga.OFGAClient) (dbmodel.User, jimm.AddApplicationOfferParams, dbmodel.ApplicationOffer, func(*qt.C, error)) {
			ctx := context.Background()

			u := dbmodel.User{
				Username:         "alice@external",
				ControllerAccess: "superuser",
			}
			c.Assert(db.DB.Create(&u).Error, qt.IsNil)

			cloud := dbmodel.Cloud{
				Name: "test-cloud",
				Type: "test-provider",
				Regions: []dbmodel.CloudRegion{{
					Name: "test-region-1",
				}},
				Users: []dbmodel.UserCloudAccess{{
					Username: u.Username,
				}},
			}
			c.Assert(db.DB.Create(&cloud).Error, qt.IsNil)

			// user u is administrator of the test-cloud
			err := openfga.NewUser(&u, client).SetCloudAccess(ctx, cloud.ResourceTag(), ofganames.AdministratorRelation)
			c.Assert(err, qt.IsNil)

			controller := dbmodel.Controller{
				Name:        "test-controller-1",
				UUID:        "00000000-0000-0000-0000-0000-0000000000001",
				CloudName:   "test-cloud",
				CloudRegion: "test-region-1",
				CloudRegions: []dbmodel.CloudRegionControllerPriority{{
					Priority:      0,
					CloudRegionID: cloud.Regions[0].ID,
				}},
			}
			err = db.AddController(ctx, &controller)
			c.Assert(err, qt.IsNil)

			cred := dbmodel.CloudCredential{
				Name:          "test-credential-1",
				CloudName:     cloud.Name,
				OwnerUsername: u.Username,
				AuthType:      "empty",
			}
			err = db.SetCloudCredential(ctx, &cred)
			c.Assert(err, qt.IsNil)

			model := dbmodel.Model{
				Name: "test-model",
				UUID: sql.NullString{
					String: "00000000-0000-0000-0000-0000-0000000000003",
					Valid:  true,
				},
				OwnerUsername:     u.Username,
				ControllerID:      controller.ID,
				CloudRegionID:     cloud.Regions[0].ID,
				CloudCredentialID: cred.ID,
				Users: []dbmodel.UserModelAccess{{
					User:   u,
					Access: "admin",
				}},
			}
			err = db.AddModel(ctx, &model)
			c.Assert(err, qt.IsNil)

			// user u is administrator of the test-model
			err = openfga.NewUser(&u, client).SetModelAccess(ctx, model.ResourceTag(), ofganames.AdministratorRelation)
			c.Assert(err, qt.IsNil)

			offerParams := jimm.AddApplicationOfferParams{
				ModelTag:               model.ResourceTag(),
				OfferName:              "test-app-offer",
				ApplicationName:        "test-app",
				ApplicationDescription: "a test app offering",
				Endpoints: map[string]string{
					"endpoint1": "url1",
				},
			}

			offer := dbmodel.ApplicationOffer{
				ID:                     1,
				ModelID:                1,
				ApplicationName:        "test-app",
				CharmURL:               "cs:test-app:17",
				ApplicationDescription: "a test app offering",
				UUID:                   "00000000-0000-0000-0000-0000-0000000000004",
				URL:                    "test-offer-url",
				Endpoints: []dbmodel.ApplicationOfferRemoteEndpoint{{
					ApplicationOfferID: 1,
					Name:               "test-endpoint",
					Role:               "requirer",
					Interface:          "unknown",
					Limit:              1,
				}},
				Spaces: []dbmodel.ApplicationOfferRemoteSpace{{
					ApplicationOfferID: 1,
					CloudType:          "test-cloud-type",
					Name:               "test-remote-space",
					ProviderID:         "test-provider-id",
					ProviderAttributes: dbmodel.Map{
						"attr1": "value1",
						"attr2": "value2",
					},
				}},
				Bindings: dbmodel.StringMap{
					"key1": "value1",
					"key2": "value2",
				},
				Connections: []dbmodel.ApplicationOfferConnection{{
					ApplicationOfferID: 1,
					SourceModelTag:     "test-model-src",
					RelationID:         1,
					Username:           "unknown",
					Endpoint:           "test-endpoint",
				}},
			}

			return u, offerParams, offer, nil
		},
	}, {
		about: "controller returns an error when creating an offer",
		getApplicationOffer: func(_ context.Context, details *jujuparams.ApplicationOfferAdminDetails) error {
			return nil
		},
		grantApplicationOfferAccess: func(context.Context, string, names.UserTag, jujuparams.OfferAccessPermission) error {
			return nil
		},
		offer: func(context.Context, crossmodel.OfferURL, jujuparams.AddApplicationOffer) error {
			return errors.E("a silly error")
		},
		createEnv: func(c *qt.C, db db.Database, client *openfga.OFGAClient) (dbmodel.User, jimm.AddApplicationOfferParams, dbmodel.ApplicationOffer, func(*qt.C, error)) {
			ctx := context.Background()

			u := dbmodel.User{
				Username:         "alice@external",
				ControllerAccess: "superuser",
			}
			c.Assert(db.DB.Create(&u).Error, qt.IsNil)

			cloud := dbmodel.Cloud{
				Name: "test-cloud",
				Type: "test-provider",
				Regions: []dbmodel.CloudRegion{{
					Name: "test-region-1",
				}},
				Users: []dbmodel.UserCloudAccess{{
					Username: u.Username,
				}},
			}
			c.Assert(db.DB.Create(&cloud).Error, qt.IsNil)

			// user u is administrator of the test-cloud
			err := openfga.NewUser(&u, client).SetCloudAccess(ctx, cloud.ResourceTag(), ofganames.AdministratorRelation)
			c.Assert(err, qt.IsNil)

			controller := dbmodel.Controller{
				Name:        "test-controller-1",
				UUID:        "00000000-0000-0000-0000-0000-0000000000001",
				CloudName:   "test-cloud",
				CloudRegion: "test-region-1",
				CloudRegions: []dbmodel.CloudRegionControllerPriority{{
					Priority:      0,
					CloudRegionID: cloud.Regions[0].ID,
				}},
			}
			err = db.AddController(ctx, &controller)
			c.Assert(err, qt.IsNil)

			cred := dbmodel.CloudCredential{
				Name:          "test-credential-1",
				CloudName:     cloud.Name,
				OwnerUsername: u.Username,
				AuthType:      "empty",
			}
			err = db.SetCloudCredential(ctx, &cred)
			c.Assert(err, qt.IsNil)

			model := dbmodel.Model{
				Name: "test-model",
				UUID: sql.NullString{
					String: "00000000-0000-0000-0000-0000-0000000000003",
					Valid:  true,
				},
				OwnerUsername:     u.Username,
				ControllerID:      controller.ID,
				CloudRegionID:     cloud.Regions[0].ID,
				CloudCredentialID: cred.ID,
				Users: []dbmodel.UserModelAccess{{
					User:   u,
					Access: "admin",
				}},
			}
			err = db.AddModel(ctx, &model)
			c.Assert(err, qt.IsNil)

			// user u is administrator of the test-model
			err = openfga.NewUser(&u, client).SetModelAccess(ctx, model.ResourceTag(), ofganames.AdministratorRelation)
			c.Assert(err, qt.IsNil)

			offerParams := jimm.AddApplicationOfferParams{
				ModelTag:               model.ResourceTag(),
				OfferName:              "test-app-offer",
				ApplicationName:        "test-app",
				ApplicationDescription: "a test app offering",
				Endpoints: map[string]string{
					"endpoint1": "url1",
				},
			}

			offer := dbmodel.ApplicationOffer{}

			return u, offerParams, offer, func(c *qt.C, err error) {
				c.Assert(err, qt.ErrorMatches, "a silly error")
			}
		},
	}, {
		about: "model not found",
		getApplicationOffer: func(_ context.Context, details *jujuparams.ApplicationOfferAdminDetails) error {
			return nil
		},
		grantApplicationOfferAccess: func(context.Context, string, names.UserTag, jujuparams.OfferAccessPermission) error {
			return nil
		},
		offer: func(context.Context, crossmodel.OfferURL, jujuparams.AddApplicationOffer) error {
			return nil
		},
		createEnv: func(c *qt.C, db db.Database, client *openfga.OFGAClient) (dbmodel.User, jimm.AddApplicationOfferParams, dbmodel.ApplicationOffer, func(*qt.C, error)) {
			u := dbmodel.User{
				Username:         "alice@external",
				ControllerAccess: "superuser",
			}

			c.Assert(db.DB.Create(&u).Error, qt.IsNil)
			offerParams := jimm.AddApplicationOfferParams{
				ModelTag:               names.NewModelTag("model-not-found"),
				OfferName:              "test-app-offer",
				ApplicationName:        "test-app",
				ApplicationDescription: "a test app offering",
				Endpoints: map[string]string{
					"endpoint1": "url1",
				},
			}

			offer := dbmodel.ApplicationOffer{}

			return u, offerParams, offer, func(c *qt.C, err error) {
				c.Assert(err, qt.ErrorMatches, "model not found")
			}
		},
	}, {
		about: "application not found",
		getApplicationOffer: func(_ context.Context, details *jujuparams.ApplicationOfferAdminDetails) error {
			return nil
		},
		grantApplicationOfferAccess: func(context.Context, string, names.UserTag, jujuparams.OfferAccessPermission) error {
			return nil
		},
		offer: func(context.Context, crossmodel.OfferURL, jujuparams.AddApplicationOffer) error {
			return errors.E(errors.CodeNotFound, "application test-app")
		},
		createEnv: func(c *qt.C, db db.Database, client *openfga.OFGAClient) (dbmodel.User, jimm.AddApplicationOfferParams, dbmodel.ApplicationOffer, func(*qt.C, error)) {
			ctx := context.Background()

			u := dbmodel.User{
				Username:         "alice@external",
				ControllerAccess: "superuser",
			}
			c.Assert(db.DB.Create(&u).Error, qt.IsNil)

			cloud := dbmodel.Cloud{
				Name: "test-cloud",
				Type: "test-provider",
				Regions: []dbmodel.CloudRegion{{
					Name: "test-region-1",
				}},
				Users: []dbmodel.UserCloudAccess{{
					Username: u.Username,
				}},
			}
			c.Assert(db.DB.Create(&cloud).Error, qt.IsNil)

			// user u is administrator of the test-cloud
			err := openfga.NewUser(&u, client).SetCloudAccess(ctx, cloud.ResourceTag(), ofganames.AdministratorRelation)
			c.Assert(err, qt.IsNil)

			controller := dbmodel.Controller{
				Name:        "test-controller-1",
				UUID:        "00000000-0000-0000-0000-0000-0000000000001",
				CloudName:   "test-cloud",
				CloudRegion: "test-region-1",
				CloudRegions: []dbmodel.CloudRegionControllerPriority{{
					Priority:      0,
					CloudRegionID: cloud.Regions[0].ID,
				}},
			}
			err = db.AddController(ctx, &controller)
			c.Assert(err, qt.IsNil)

			cred := dbmodel.CloudCredential{
				Name:          "test-credential-1",
				CloudName:     cloud.Name,
				OwnerUsername: u.Username,
				AuthType:      "empty",
			}
			err = db.SetCloudCredential(ctx, &cred)
			c.Assert(err, qt.IsNil)

			model := dbmodel.Model{
				Name: "test-model",
				UUID: sql.NullString{
					String: "00000000-0000-0000-0000-0000-0000000000003",
					Valid:  true,
				},
				OwnerUsername:     u.Username,
				ControllerID:      controller.ID,
				CloudRegionID:     cloud.Regions[0].ID,
				CloudCredentialID: cred.ID,
				Users: []dbmodel.UserModelAccess{{
					User:   u,
					Access: "admin",
				}},
			}
			err = db.AddModel(ctx, &model)
			c.Assert(err, qt.IsNil)

			// user u is administrator of the test-model
			err = openfga.NewUser(&u, client).SetModelAccess(ctx, model.ResourceTag(), ofganames.AdministratorRelation)
			c.Assert(err, qt.IsNil)

			offerParams := jimm.AddApplicationOfferParams{
				ModelTag:               model.ResourceTag(),
				OfferName:              "test-app-offer",
				ApplicationName:        "test-app",
				ApplicationDescription: "a test app offering",
				Endpoints: map[string]string{
					"endpoint1": "url1",
				},
			}

			offer := dbmodel.ApplicationOffer{}

			return u, offerParams, offer, func(c *qt.C, err error) {
				c.Assert(errors.ErrorCode(err), qt.Equals, errors.CodeNotFound)
				c.Assert(err, qt.ErrorMatches, "application test-app")
			}
		},
	}, {
		about: "user not model admin",
		getApplicationOffer: func(_ context.Context, details *jujuparams.ApplicationOfferAdminDetails) error {
			return nil
		},
		grantApplicationOfferAccess: func(context.Context, string, names.UserTag, jujuparams.OfferAccessPermission) error {
			return nil
		},
		offer: func(context.Context, crossmodel.OfferURL, jujuparams.AddApplicationOffer) error {
			return nil
		},
		createEnv: func(c *qt.C, db db.Database, client *openfga.OFGAClient) (dbmodel.User, jimm.AddApplicationOfferParams, dbmodel.ApplicationOffer, func(*qt.C, error)) {
			ctx := context.Background()

			u := dbmodel.User{
				Username:         "alice@external",
				ControllerAccess: "superuser",
			}
			c.Assert(db.DB.Create(&u).Error, qt.IsNil)

			u1 := dbmodel.User{
				Username:         "eve@external",
				ControllerAccess: "superuser",
			}
			c.Assert(db.DB.Create(&u1).Error, qt.IsNil)

			cloud := dbmodel.Cloud{
				Name: "test-cloud",
				Type: "test-provider",
				Regions: []dbmodel.CloudRegion{{
					Name: "test-region-1",
				}},
				Users: []dbmodel.UserCloudAccess{{
					Username: u.Username,
				}},
			}
			c.Assert(db.DB.Create(&cloud).Error, qt.IsNil)

			// user u is administrator of the test-cloud
			err := openfga.NewUser(&u, client).SetCloudAccess(ctx, cloud.ResourceTag(), ofganames.AdministratorRelation)
			c.Assert(err, qt.IsNil)

			controller := dbmodel.Controller{
				Name:        "test-controller-1",
				UUID:        "00000000-0000-0000-0000-0000-0000000000001",
				CloudName:   "test-cloud",
				CloudRegion: "test-region",
				CloudRegions: []dbmodel.CloudRegionControllerPriority{{
					Priority:      0,
					CloudRegionID: cloud.Regions[0].ID,
				}},
			}
			err = db.AddController(ctx, &controller)
			c.Assert(err, qt.IsNil)

			cred := dbmodel.CloudCredential{
				Name:          "test-credential-1",
				CloudName:     cloud.Name,
				OwnerUsername: u.Username,
				AuthType:      "empty",
			}
			err = db.SetCloudCredential(ctx, &cred)
			c.Assert(err, qt.IsNil)

			model := dbmodel.Model{
				Name: "test-model",
				UUID: sql.NullString{
					String: "00000000-0000-0000-0000-0000-0000000000003",
					Valid:  true,
				},
				OwnerUsername:     u.Username,
				ControllerID:      controller.ID,
				CloudRegionID:     cloud.Regions[0].ID,
				CloudCredentialID: cred.ID,
				Users: []dbmodel.UserModelAccess{{
					User:   u,
					Access: "admin",
				}},
			}
			err = db.AddModel(ctx, &model)
			c.Assert(err, qt.IsNil)

			// user u is administrator of the test-model
			err = openfga.NewUser(&u, client).SetModelAccess(ctx, model.ResourceTag(), ofganames.AdministratorRelation)
			c.Assert(err, qt.IsNil)

			offerParams := jimm.AddApplicationOfferParams{
				ModelTag:               model.ResourceTag(),
				OfferName:              "test-app-offer",
				ApplicationName:        "test-app",
				ApplicationDescription: "a test app offering",
				Endpoints: map[string]string{
					"endpoint1": "url1",
				},
			}

			offer := dbmodel.ApplicationOffer{}

			return u1, offerParams, offer, func(c *qt.C, err error) {
				c.Assert(err, qt.ErrorMatches, "unauthorized")
			}
		},
	}, {
		about: "fail to fetch application offer details",
		getApplicationOffer: func(_ context.Context, details *jujuparams.ApplicationOfferAdminDetails) error {
			return errors.E("a silly error")
		},
		grantApplicationOfferAccess: func(context.Context, string, names.UserTag, jujuparams.OfferAccessPermission) error {
			return nil
		},
		offer: func(context.Context, crossmodel.OfferURL, jujuparams.AddApplicationOffer) error {
			return nil
		},
		createEnv: func(c *qt.C, db db.Database, client *openfga.OFGAClient) (dbmodel.User, jimm.AddApplicationOfferParams, dbmodel.ApplicationOffer, func(*qt.C, error)) {
			ctx := context.Background()

			u := dbmodel.User{
				Username:         "alice@external",
				ControllerAccess: "superuser",
			}
			c.Assert(db.DB.Create(&u).Error, qt.IsNil)

			cloud := dbmodel.Cloud{
				Name: "test-cloud",
				Type: "test-provider",
				Regions: []dbmodel.CloudRegion{{
					Name: "test-region-1",
				}},
				Users: []dbmodel.UserCloudAccess{{
					Username: u.Username,
				}},
			}
			c.Assert(db.DB.Create(&cloud).Error, qt.IsNil)

			// user u is administrator of the test-cloud
			err := openfga.NewUser(&u, client).SetCloudAccess(ctx, cloud.ResourceTag(), ofganames.AdministratorRelation)
			c.Assert(err, qt.IsNil)

			controller := dbmodel.Controller{
				Name:        "test-controller-1",
				UUID:        "00000000-0000-0000-0000-0000-0000000000001",
				CloudName:   "test-cloud",
				CloudRegion: "test-region-1",
				CloudRegions: []dbmodel.CloudRegionControllerPriority{{
					Priority:      0,
					CloudRegionID: cloud.Regions[0].ID,
				}},
			}
			err = db.AddController(ctx, &controller)
			c.Assert(err, qt.IsNil)

			cred := dbmodel.CloudCredential{
				Name:          "test-credential-1",
				CloudName:     cloud.Name,
				OwnerUsername: u.Username,
				AuthType:      "empty",
			}
			err = db.SetCloudCredential(ctx, &cred)
			c.Assert(err, qt.IsNil)

			model := dbmodel.Model{
				Name: "test-model",
				UUID: sql.NullString{
					String: "00000000-0000-0000-0000-0000-0000000000003",
					Valid:  true,
				},
				OwnerUsername:     u.Username,
				ControllerID:      controller.ID,
				CloudRegionID:     cloud.Regions[0].ID,
				CloudCredentialID: cred.ID,
				Users: []dbmodel.UserModelAccess{{
					User:   u,
					Access: "admin",
				}},
			}
			err = db.AddModel(ctx, &model)
			c.Assert(err, qt.IsNil)

			// user u is administrator of the test-model
			err = openfga.NewUser(&u, client).SetModelAccess(ctx, model.ResourceTag(), ofganames.AdministratorRelation)
			c.Assert(err, qt.IsNil)

			offerParams := jimm.AddApplicationOfferParams{
				ModelTag:               model.ResourceTag(),
				OfferName:              "test-app-offer",
				ApplicationName:        "test-app",
				ApplicationDescription: "a test app offering",
				Endpoints: map[string]string{
					"endpoint1": "url1",
				},
			}

			offer := dbmodel.ApplicationOffer{}

			return u, offerParams, offer, func(c *qt.C, err error) {
				c.Assert(err, qt.ErrorMatches, "a silly error")
			}
		},
	}, {
		about: "controller returns `application offer already exists`",
		getApplicationOffer: func(_ context.Context, details *jujuparams.ApplicationOfferAdminDetails) error {
			return nil
		},
		grantApplicationOfferAccess: func(context.Context, string, names.UserTag, jujuparams.OfferAccessPermission) error {
			return nil
		},
		offer: func(context.Context, crossmodel.OfferURL, jujuparams.AddApplicationOffer) error {
			return errors.E("application offer already exists")
		},
		createEnv: func(c *qt.C, db db.Database, client *openfga.OFGAClient) (dbmodel.User, jimm.AddApplicationOfferParams, dbmodel.ApplicationOffer, func(*qt.C, error)) {
			ctx := context.Background()

			u := dbmodel.User{
				Username:         "alice@external",
				ControllerAccess: "superuser",
			}
			c.Assert(db.DB.Create(&u).Error, qt.IsNil)

			cloud := dbmodel.Cloud{
				Name: "test-cloud",
				Type: "test-provider",
				Regions: []dbmodel.CloudRegion{{
					Name: "test-region-1",
				}},
				Users: []dbmodel.UserCloudAccess{{
					Username: u.Username,
				}},
			}
			c.Assert(db.DB.Create(&cloud).Error, qt.IsNil)

			// user u is administrator of the test-cloud
			err := openfga.NewUser(&u, client).SetCloudAccess(ctx, cloud.ResourceTag(), ofganames.AdministratorRelation)
			c.Assert(err, qt.IsNil)

			controller := dbmodel.Controller{
				Name:        "test-controller-1",
				UUID:        "00000000-0000-0000-0000-0000-0000000000001",
				CloudName:   "test-cloud",
				CloudRegion: "test-region-1",
				CloudRegions: []dbmodel.CloudRegionControllerPriority{{
					Priority:      0,
					CloudRegionID: cloud.Regions[0].ID,
				}},
			}
			err = db.AddController(ctx, &controller)
			c.Assert(err, qt.IsNil)

			cred := dbmodel.CloudCredential{
				Name:          "test-credential-1",
				CloudName:     cloud.Name,
				OwnerUsername: u.Username,
				AuthType:      "empty",
			}
			err = db.SetCloudCredential(ctx, &cred)
			c.Assert(err, qt.IsNil)

			model := dbmodel.Model{
				Name: "test-model",
				UUID: sql.NullString{
					String: "00000000-0000-0000-0000-0000-0000000000003",
					Valid:  true,
				},
				OwnerUsername:     u.Username,
				ControllerID:      controller.ID,
				CloudRegionID:     cloud.Regions[0].ID,
				CloudCredentialID: cred.ID,
				Users: []dbmodel.UserModelAccess{{
					User:   u,
					Access: "admin",
				}},
			}
			err = db.AddModel(ctx, &model)
			c.Assert(err, qt.IsNil)

			// user u is administrator of the test-model
			err = openfga.NewUser(&u, client).SetModelAccess(ctx, model.ResourceTag(), ofganames.AdministratorRelation)
			c.Assert(err, qt.IsNil)

			offerParams := jimm.AddApplicationOfferParams{
				ModelTag:               model.ResourceTag(),
				OfferName:              "test-app-offer",
				ApplicationName:        "test-app",
				ApplicationDescription: "a test app offering",
				Endpoints: map[string]string{
					"endpoint1": "url1",
				},
			}

			offer := dbmodel.ApplicationOffer{}

			return u, offerParams, offer, func(c *qt.C, err error) {
				c.Assert(err, qt.ErrorMatches, "application offer already exists")
				c.Assert(errors.ErrorCode(err), qt.Equals, errors.CodeAlreadyExists)
			}
		},
	}}

	for _, test := range tests {
		c.Run(test.about, func(c *qt.C) {
			api := &jimmtest.API{
				GetApplicationOffer_:         test.getApplicationOffer,
				GrantApplicationOfferAccess_: test.grantApplicationOfferAccess,
				Offer_:                       test.offer,
			}

			_, client, _, err := jimmtest.SetupTestOFGAClient(c.Name(), test.about)
			c.Assert(err, qt.IsNil)

			j := &jimm.JIMM{
				Database: db.Database{
					DB: jimmtest.MemoryDB(c, func() time.Time { return now }),
				},
				Dialer: &jimmtest.Dialer{
					API: api,
				},
				OpenFGAClient: client,
			}

			ctx := context.Background()
			err = j.Database.Migrate(ctx, false)
			c.Assert(err, qt.IsNil)

			u, offerArgs, expectedOffer, errorAssertion := test.createEnv(c, j.Database, client)

			err = j.Offer(context.Background(), openfga.NewUser(&u, client), offerArgs)
			if errorAssertion == nil {
				c.Assert(err, qt.IsNil)

				offer := dbmodel.ApplicationOffer{
					URL: expectedOffer.URL,
				}
				err = j.Database.GetApplicationOffer(ctx, &offer)
				c.Assert(err, qt.IsNil)
				c.Assert(offer, qt.CmpEquals(cmpopts.EquateEmpty(), cmpopts.IgnoreTypes(time.Time{}, gorm.Model{}), cmpopts.IgnoreTypes(dbmodel.Model{})), expectedOffer)
			} else {
				errorAssertion(c, err)
			}
		})
	}

}

func TestOfferAssertOpenFGARelationsExist(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()
	now := time.Now().UTC().Round(time.Millisecond)

	createEnv := func(c *qt.C, db db.Database, client *openfga.OFGAClient) (dbmodel.User, jimm.AddApplicationOfferParams, dbmodel.ApplicationOffer, func(*qt.C, error)) {
		ctx := context.Background()

		u := dbmodel.User{
			Username:         "alice@external",
			ControllerAccess: "superuser",
		}
		c.Assert(db.DB.Create(&u).Error, qt.IsNil)

		cloud := dbmodel.Cloud{
			Name: "test-cloud",
			Type: "test-provider",
			Regions: []dbmodel.CloudRegion{{
				Name: "test-region-1",
			}},
			Users: []dbmodel.UserCloudAccess{{
				Username: u.Username,
			}},
		}
		c.Assert(db.DB.Create(&cloud).Error, qt.IsNil)

		// user u is administrator of the test-cloud
		err := openfga.NewUser(&u, client).SetCloudAccess(ctx, cloud.ResourceTag(), ofganames.AdministratorRelation)
		c.Assert(err, qt.IsNil)

		controller := dbmodel.Controller{
			Name:        "test-controller",
			UUID:        "00000000-0000-0000-0000-0000-0000000000001",
			CloudName:   "test-cloud",
			CloudRegion: "test-region-1",
			CloudRegions: []dbmodel.CloudRegionControllerPriority{{
				Priority:      0,
				CloudRegionID: cloud.Regions[0].ID,
			}},
		}
		err = db.AddController(ctx, &controller)
		c.Assert(err, qt.IsNil)

		cred := dbmodel.CloudCredential{
			Name:          "test-credential-1",
			CloudName:     cloud.Name,
			OwnerUsername: u.Username,
			AuthType:      "empty",
		}
		err = db.SetCloudCredential(ctx, &cred)
		c.Assert(err, qt.IsNil)

		model := dbmodel.Model{
			Name: "test-model",
			UUID: sql.NullString{
				String: "00000000-0000-0000-0000-0000-0000000000003",
				Valid:  true,
			},
			OwnerUsername:     u.Username,
			ControllerID:      controller.ID,
			CloudRegionID:     cloud.Regions[0].ID,
			CloudCredentialID: cred.ID,
			Users: []dbmodel.UserModelAccess{{
				User:   u,
				Access: "admin",
			}},
		}
		err = db.AddModel(ctx, &model)
		c.Assert(err, qt.IsNil)

		// user u is administrator of the test-model
		err = openfga.NewUser(&u, client).SetModelAccess(ctx, model.ResourceTag(), ofganames.AdministratorRelation)
		c.Assert(err, qt.IsNil)

		offerParams := jimm.AddApplicationOfferParams{
			ModelTag:               model.ResourceTag(),
			OfferName:              "test-app-offer",
			ApplicationName:        "test-app",
			ApplicationDescription: "a test app offering",
			Endpoints: map[string]string{
				"endpoint1": "url1",
			},
		}

		offer := dbmodel.ApplicationOffer{
			ID:                     1,
			ModelID:                model.ID,
			ApplicationName:        "test-app",
			CharmURL:               "cs:test-app:17",
			ApplicationDescription: "a test app offering",
			UUID:                   "00000000-0000-0000-0000-0000-0000000000004",
			URL:                    "test-offer-url",
			Endpoints: []dbmodel.ApplicationOfferRemoteEndpoint{{
				ApplicationOfferID: 1,
				Name:               "test-endpoint",
				Role:               "requirer",
				Interface:          "unknown",
				Limit:              1,
			}},
			Spaces: []dbmodel.ApplicationOfferRemoteSpace{{
				ApplicationOfferID: 1,
				CloudType:          "test-cloud-type",
				Name:               "test-remote-space",
				ProviderID:         "test-provider-id",
				ProviderAttributes: dbmodel.Map{
					"attr1": "value1",
					"attr2": "value2",
				},
			}},
			Bindings: dbmodel.StringMap{
				"key1": "value1",
				"key2": "value2",
			},
			Connections: []dbmodel.ApplicationOfferConnection{{
				ApplicationOfferID: 1,
				SourceModelTag:     "test-model-src",
				RelationID:         1,
				Username:           "unknown",
				Endpoint:           "test-endpoint",
			}},
		}

		return u, offerParams, offer, nil
	}

	api := &jimmtest.API{
		GetApplicationOffer_: func(_ context.Context, details *jujuparams.ApplicationOfferAdminDetails) error {
			details.ApplicationName = "test-app"
			details.CharmURL = "cs:test-app:17"
			details.Connections = []jujuparams.OfferConnection{{
				SourceModelTag: "test-model-src",
				RelationId:     1,
				Username:       "unknown",
				Endpoint:       "test-endpoint",
			}}
			details.ApplicationOfferDetails = jujuparams.ApplicationOfferDetails{
				OfferUUID:              "00000000-0000-0000-0000-0000-0000000000004",
				OfferURL:               "test-offer-url",
				ApplicationDescription: "a test app offering",
				Endpoints: []jujuparams.RemoteEndpoint{{
					Name:      "test-endpoint",
					Role:      charm.RoleRequirer,
					Interface: "unknown",
					Limit:     1,
				}},
				Spaces: []jujuparams.RemoteSpace{{
					CloudType:  "test-cloud-type",
					Name:       "test-remote-space",
					ProviderId: "test-provider-id",
					ProviderAttributes: map[string]interface{}{
						"attr1": "value1",
						"attr2": "value2",
					},
					Subnets: []jujuparams.Subnet{{
						SpaceTag: "test-remote-space",
						VLANTag:  1024,
						Status:   "alive",
					}},
				}},
				Bindings: map[string]string{
					"key1": "value1",
					"key2": "value2",
				},
				Users: []jujuparams.OfferUserDetails{{
					UserName:    "alice",
					DisplayName: "alice, sister of eve",
					Access:      string(jujuparams.OfferAdminAccess),
				}},
			}
			return nil
		},
		GrantApplicationOfferAccess_: func(context.Context, string, names.UserTag, jujuparams.OfferAccessPermission) error {
			return nil
		},
		Offer_: func(context.Context, crossmodel.OfferURL, jujuparams.AddApplicationOffer) error {
			return nil
		},
	}

	_, client, _, err := jimmtest.SetupTestOFGAClient(c.Name())
	c.Assert(err, qt.IsNil)

	j := &jimm.JIMM{
		UUID: uuid.NewString(),
		Database: db.Database{
			DB: jimmtest.MemoryDB(c, func() time.Time { return now }),
		},
		Dialer: &jimmtest.Dialer{
			API:  api,
			UUID: "00000000-0000-0000-0000-0000-0000000000001",
		},
		OpenFGAClient: client,
	}

	err = j.Database.Migrate(ctx, false)
	c.Assert(err, qt.IsNil)

	u, offerArgs, expectedOffer, _ := createEnv(c, j.Database, client)

	err = j.Offer(context.Background(), openfga.NewUser(&u, client), offerArgs)
	c.Assert(err, qt.IsNil)

	offer := dbmodel.ApplicationOffer{
		URL: expectedOffer.URL,
	}
	err = j.Database.GetApplicationOffer(ctx, &offer)
	c.Assert(err, qt.IsNil)
	c.Assert(offer, qt.CmpEquals(cmpopts.EquateEmpty(), cmpopts.IgnoreTypes(time.Time{}, gorm.Model{}), cmpopts.IgnoreTypes(dbmodel.Model{})), expectedOffer)

	// check the controller relation was created
	exists, _, err := client.CheckRelation(
		context.Background(),
		openfga.Tuple{
			Object:   ofganames.ConvertTag(offerArgs.ModelTag),
			Relation: ofganames.ModelRelation,
			Target:   ofganames.ConvertTag(offer.ResourceTag()),
		},
		false,
	)
	c.Assert(err, qt.IsNil)
	c.Assert(exists, qt.IsTrue)

	// check the user has administrator rights on the offer
	exists, _, err = client.CheckRelation(
		context.Background(),
		openfga.Tuple{
			Object:   ofganames.ConvertTag(u.ResourceTag()),
			Relation: ofganames.AdministratorRelation,
			Target:   ofganames.ConvertTag(offer.ResourceTag()),
		},
		false,
	)
	c.Assert(err, qt.IsNil)
	c.Assert(exists, qt.IsTrue)
}

func TestDetermineAccessLevelAfterGrant(t *testing.T) {
	/*
		c := qt.New(t)

		tests := []struct {
			about               string
			currentAccessLevel  string
			grantAccessLevel    string
			expectedAccessLevel string
		}{{
			about:               "user has no access - grant admin",
			currentAccessLevel:  "",
			grantAccessLevel:    string(jujuparams.OfferAdminAccess),
			expectedAccessLevel: "admin",
		}, {
			about:               "user has no access - grant consume",
			currentAccessLevel:  "",
			grantAccessLevel:    string(jujuparams.OfferConsumeAccess),
			expectedAccessLevel: "consume",
		}, {
			about:               "user has no access - grant read",
			currentAccessLevel:  "",
			grantAccessLevel:    string(jujuparams.OfferReadAccess),
			expectedAccessLevel: "read",
		}, {
			about:               "user has read access - grant admin",
			currentAccessLevel:  "read",
			grantAccessLevel:    string(jujuparams.OfferAdminAccess),
			expectedAccessLevel: "admin",
		}, {
			about:               "user has read access - grant consume",
			currentAccessLevel:  "read",
			grantAccessLevel:    string(jujuparams.OfferConsumeAccess),
			expectedAccessLevel: "consume",
		}, {
			about:               "user has read access - grant read",
			currentAccessLevel:  "read",
			grantAccessLevel:    string(jujuparams.OfferReadAccess),
			expectedAccessLevel: "read",
		}, {
			about:               "user has consume access - grant admin",
			currentAccessLevel:  "consume",
			grantAccessLevel:    string(jujuparams.OfferAdminAccess),
			expectedAccessLevel: "admin",
		}, {
			about:               "user has consume access - grant consume",
			currentAccessLevel:  "consume",
			grantAccessLevel:    string(jujuparams.OfferConsumeAccess),
			expectedAccessLevel: "consume",
		}, {
			about:               "user has consume access - grant read",
			currentAccessLevel:  "consume",
			grantAccessLevel:    string(jujuparams.OfferReadAccess),
			expectedAccessLevel: "consume",
		}, {
			about:               "user has admin access - grant admin",
			currentAccessLevel:  "admin",
			grantAccessLevel:    string(jujuparams.OfferAdminAccess),
			expectedAccessLevel: "admin",
		}, {
			about:               "user has admin access - grant consume",
			currentAccessLevel:  "admin",
			grantAccessLevel:    string(jujuparams.OfferConsumeAccess),
			expectedAccessLevel: "admin",
		}, {
			about:               "user has admin access - grant read",
			currentAccessLevel:  "admin",
			grantAccessLevel:    string(jujuparams.OfferReadAccess),
			expectedAccessLevel: "admin",
		}}

		for _, test := range tests {
			c.Run(test.about, func(c *qt.C) {
				level := jimm.DetermineAccessLevelAfterGrant(test.currentAccessLevel, test.grantAccessLevel)
				c.Assert(level, qt.Equals, test.expectedAccessLevel)
			})
		}
	*/
}

func TestDetermineAccessLevelAfterRevoke(t *testing.T) {
	/*
		c := qt.New(t)

		tests := []struct {
			about               string
			currentAccessLevel  string
			revokeAccessLevel   string
			expectedAccessLevel string
		}{{
			about:               "user has no access - revoke admin",
			currentAccessLevel:  "",
			revokeAccessLevel:   string(jujuparams.OfferAdminAccess),
			expectedAccessLevel: "",
		}, {
			about:               "user has no access - revoke consume",
			currentAccessLevel:  "",
			revokeAccessLevel:   string(jujuparams.OfferConsumeAccess),
			expectedAccessLevel: "",
		}, {
			about:               "user has no access - revoke read",
			currentAccessLevel:  "",
			revokeAccessLevel:   string(jujuparams.OfferReadAccess),
			expectedAccessLevel: "",
		}, {
			about:               "user has no access - revoke all",
			currentAccessLevel:  "",
			revokeAccessLevel:   "",
			expectedAccessLevel: "",
		}, {
			about:               "user has read access - revoke admin",
			currentAccessLevel:  string(jujuparams.OfferReadAccess),
			revokeAccessLevel:   string(jujuparams.OfferAdminAccess),
			expectedAccessLevel: string(jujuparams.OfferReadAccess),
		}, {
			about:               "user has read access - revoke consume",
			currentAccessLevel:  string(jujuparams.OfferReadAccess),
			revokeAccessLevel:   string(jujuparams.OfferConsumeAccess),
			expectedAccessLevel: string(jujuparams.OfferReadAccess),
		}, {
			about:               "user has read access - revoke read",
			currentAccessLevel:  string(jujuparams.OfferReadAccess),
			revokeAccessLevel:   string(jujuparams.OfferReadAccess),
			expectedAccessLevel: "",
		}, {
			about:               "user has read access - revoke all",
			currentAccessLevel:  string(jujuparams.OfferReadAccess),
			revokeAccessLevel:   string(jujuparams.OfferReadAccess),
			expectedAccessLevel: "",
		}, {
			about:               "user has consume access - revoke admin",
			currentAccessLevel:  string(jujuparams.OfferConsumeAccess),
			revokeAccessLevel:   string(jujuparams.OfferAdminAccess),
			expectedAccessLevel: string(jujuparams.OfferConsumeAccess),
		}, {
			about:               "user has consume access - revoke consume",
			currentAccessLevel:  string(jujuparams.OfferConsumeAccess),
			revokeAccessLevel:   string(jujuparams.OfferConsumeAccess),
			expectedAccessLevel: string(jujuparams.OfferReadAccess),
		}, {
			about:               "user has consume access - revoke read",
			currentAccessLevel:  string(jujuparams.OfferConsumeAccess),
			revokeAccessLevel:   string(jujuparams.OfferReadAccess),
			expectedAccessLevel: "",
		}, {
			about:               "user has consume access - revoke all",
			currentAccessLevel:  string(jujuparams.OfferConsumeAccess),
			revokeAccessLevel:   string(jujuparams.OfferReadAccess),
			expectedAccessLevel: "",
		}, {
			about:               "user has admin access - revoke admin",
			currentAccessLevel:  string(jujuparams.OfferAdminAccess),
			revokeAccessLevel:   string(jujuparams.OfferAdminAccess),
			expectedAccessLevel: string(jujuparams.OfferConsumeAccess),
		}, {
			about:               "user has admin access - revoke consume",
			currentAccessLevel:  string(jujuparams.OfferAdminAccess),
			revokeAccessLevel:   string(jujuparams.OfferConsumeAccess),
			expectedAccessLevel: string(jujuparams.OfferReadAccess),
		}, {
			about:               "user has admin access - revoke read",
			currentAccessLevel:  string(jujuparams.OfferAdminAccess),
			revokeAccessLevel:   string(jujuparams.OfferReadAccess),
			expectedAccessLevel: "",
		}, {
			about:               "user has admin access - revoke all",
			currentAccessLevel:  string(jujuparams.OfferAdminAccess),
			revokeAccessLevel:   string(jujuparams.OfferReadAccess),
			expectedAccessLevel: "",
		}}

		for _, test := range tests {
			c.Run(test.about, func(c *qt.C) {
				level := jimm.DetermineAccessLevelAfterRevoke(test.currentAccessLevel, test.revokeAccessLevel)
				c.Assert(level, qt.Equals, test.expectedAccessLevel)
			})
		}
	*/
}

func TestDestroyOffer(t *testing.T) {
	c := qt.New(t)

	ctx := context.Background()
	now := time.Now().UTC().Round(time.Millisecond)

	destroyErrorsChannel := make(chan error, 1)

	tests := []struct {
		about         string
		parameterFunc func(*environment) (dbmodel.User, string)
		destroyError  string
		expectedError string
	}{{
		about: "admin allowed to destroy an offer",
		parameterFunc: func(env *environment) (dbmodel.User, string) {
			return env.users[0], "test-offer-url"
		},
	}, {
		about: "user with consume access not allowed to destroy an offer",
		parameterFunc: func(env *environment) (dbmodel.User, string) {
			return env.users[2], "test-offer-url"
		},
		expectedError: "unauthorized",
	}, {
		about: "user with read access not allowed to destroy an offer",
		parameterFunc: func(env *environment) (dbmodel.User, string) {
			return env.users[3], "test-offer-url"
		},
		expectedError: "unauthorized",
	}, {
		about: "user without access not allowed to destroy an offer",
		parameterFunc: func(env *environment) (dbmodel.User, string) {
			return env.users[4], "test-offer-url"
		},
		expectedError: "unauthorized",
	}, {
		about: "offer not found",
		parameterFunc: func(env *environment) (dbmodel.User, string) {
			return env.users[0], "no-such-offer"
		},
		expectedError: "application offer not found",
	}, {
		about:        "controller returns an error",
		destroyError: "a silly error",
		parameterFunc: func(env *environment) (dbmodel.User, string) {
			return env.users[0], "test-offer-url"
		},
		expectedError: "a silly error",
	}}

	for _, test := range tests {
		c.Run(test.about, func(c *qt.C) {

			db := db.Database{
				DB: jimmtest.MemoryDB(c, func() time.Time { return now }),
			}
			err := db.Migrate(ctx, false)
			c.Assert(err, qt.IsNil)

			_, client, _, err := jimmtest.SetupTestOFGAClient(c.Name(), test.about)
			c.Assert(err, qt.IsNil)

			jimmUUID := uuid.NewString()

			environment := initializeEnvironment(c, ctx, &db, client, jimmUUID)
			authenticatedUser, offerURL := test.parameterFunc(environment)

			j := &jimm.JIMM{
				UUID:     jimmUUID,
				Database: db,
				Dialer: &jimmtest.Dialer{
					API: &jimmtest.API{
						DestroyApplicationOffer_: func(context.Context, string, bool) error {
							select {
							case err := <-destroyErrorsChannel:
								return err
							default:
								return nil
							}
						},
					},
				},
				OpenFGAClient: client,
			}

			if test.destroyError != "" {
				select {
				case destroyErrorsChannel <- errors.E(test.destroyError):
				default:
				}
			}
			err = j.DestroyOffer(ctx, openfga.NewUser(&authenticatedUser, client), offerURL, true)
			if test.expectedError == "" {
				c.Assert(err, qt.IsNil)

				offer := dbmodel.ApplicationOffer{
					URL: offerURL,
				}
				err = db.GetApplicationOffer(ctx, &offer)
				c.Assert(errors.ErrorCode(err), qt.Equals, errors.CodeNotFound)
			} else {
				c.Assert(err, qt.ErrorMatches, test.expectedError)
			}
		})
	}
}

func TestUpdateOffer(t *testing.T) {
	c := qt.New(t)

	ctx := context.Background()
	now := time.Now().UTC().Round(time.Millisecond)

	tests := []struct {
		about         string
		parameterFunc func(*environment) (string, bool)
		expectedError string
		expectedOffer dbmodel.ApplicationOffer
	}{{
		about: "update works",
		parameterFunc: func(env *environment) (string, bool) {
			return env.applicationOffers[0].UUID, false
		},
		expectedOffer: dbmodel.ApplicationOffer{
			ID:                     1,
			UUID:                   "00000000-0000-0000-0000-0000-0000000000011",
			URL:                    "test-offer-url",
			ModelID:                1,
			ApplicationName:        "test-app",
			CharmURL:               "cs:test-app:17",
			ApplicationDescription: "changed offer description",
			Spaces: []dbmodel.ApplicationOfferRemoteSpace{{
				ApplicationOfferID: 1,
				CloudType:          "test-cloud-type",
				Name:               "test-remote-space",
				ProviderID:         "test-provider-id",
				ProviderAttributes: dbmodel.Map{
					"attr1": "value3",
					"attr2": "value4"},
			}},
			Bindings: dbmodel.StringMap{
				"key1": "value4",
				"key2": "value5",
			},
			Connections: []dbmodel.ApplicationOfferConnection{{
				ApplicationOfferID: 1,
				SourceModelTag:     "test-model-src",
				RelationID:         1,
				Username:           "unknown",
				Endpoint:           "test-endpoint",
			}},
			Endpoints: []dbmodel.ApplicationOfferRemoteEndpoint{{
				ApplicationOfferID: 1,
				Name:               "test-endpoint",
				Role:               "requirer",
				Interface:          "unknown",
				Limit:              1,
			}},
			Users: []dbmodel.UserApplicationOfferAccess{{
				Username: "eve@external",
				User: dbmodel.User{
					Username:         "eve@external",
					ControllerAccess: "login",
				},
				ApplicationOfferID: 1,
				Access:             "admin",
			}, {
				Username: "bob@external",
				User: dbmodel.User{
					Username:         "bob@external",
					ControllerAccess: "login",
				},
				ApplicationOfferID: 1,
				Access:             "consume",
			}, {
				Username: "fred@external",
				User: dbmodel.User{
					Username:         "fred@external",
					ControllerAccess: "login",
				},
				ApplicationOfferID: 1,
				Access:             "read",
			}, {
				Username: "jane@external",
				User: dbmodel.User{
					Username:         "jane@external",
					ControllerAccess: "login",
				},
				ApplicationOfferID: 1,
				Access:             "admin",
			}},
		},
	}, {
		about: "offer removed",
		parameterFunc: func(env *environment) (string, bool) {
			return env.applicationOffers[0].UUID, true
		},
	}, {
		about: "offer not found",
		parameterFunc: func(env *environment) (string, bool) {
			return "no-such-uuid", false
		},
		expectedError: "application offer not found",
	}}

	for _, test := range tests {
		c.Run(test.about, func(c *qt.C) {

			db := db.Database{
				DB: jimmtest.MemoryDB(c, func() time.Time { return now }),
			}
			err := db.Migrate(ctx, false)
			c.Assert(err, qt.IsNil)

			_, client, _, err := jimmtest.SetupTestOFGAClient(c.Name(), test.about)
			c.Assert(err, qt.IsNil)

			jimmUUID := uuid.NewString()

			environment := initializeEnvironment(c, ctx, &db, client, jimmUUID)
			offerUUID, removed := test.parameterFunc(environment)

			j := &jimm.JIMM{
				UUID:          jimmUUID,
				OpenFGAClient: client,
				Database:      db,
				Dialer: &jimmtest.Dialer{
					API: &jimmtest.API{
						GetApplicationOffer_: func(_ context.Context, details *jujuparams.ApplicationOfferAdminDetails) error {
							details.ApplicationName = "test-app"
							details.CharmURL = "cs:test-app:17"
							details.Connections = []jujuparams.OfferConnection{{
								SourceModelTag: "test-model-src",
								RelationId:     1,
								Username:       "unknown",
								Endpoint:       "test-endpoint",
							}}
							details.ApplicationOfferDetails = jujuparams.ApplicationOfferDetails{
								OfferUUID:              "00000000-0000-0000-0000-0000-0000000000011",
								OfferURL:               "test-offer-url",
								ApplicationDescription: "changed offer description",
								Endpoints: []jujuparams.RemoteEndpoint{{
									Name:      "test-endpoint",
									Role:      charm.RoleRequirer,
									Interface: "unknown",
									Limit:     1,
								}},
								Spaces: []jujuparams.RemoteSpace{{
									CloudType:  "test-cloud-type",
									Name:       "test-remote-space",
									ProviderId: "test-provider-id",
									ProviderAttributes: map[string]interface{}{
										"attr1": "value3",
										"attr2": "value4",
									},
									Subnets: []jujuparams.Subnet{{
										SpaceTag: "test-remote-space",
										VLANTag:  1024,
										Status:   "dead",
									}},
								}},
								Bindings: map[string]string{
									"key1": "value4",
									"key2": "value5",
								},
								Users: []jujuparams.OfferUserDetails{{
									UserName:    "alice",
									DisplayName: "alice, sister of eve",
									Access:      string(jujuparams.OfferAdminAccess),
								}},
							}
							return nil
						},
					},
				},
			}

			err = j.UpdateApplicationOffer(ctx, &environment.controllers[0], offerUUID, removed)
			if test.expectedError == "" {
				c.Assert(err, qt.IsNil)

				offer := dbmodel.ApplicationOffer{
					UUID: offerUUID,
				}
				err = db.GetApplicationOffer(ctx, &offer)
				if removed {
					c.Assert(errors.ErrorCode(err), qt.Equals, errors.CodeNotFound)
				} else {
					c.Assert(err, qt.IsNil)
					c.Assert(
						offer,
						qt.CmpEquals(
							cmpopts.EquateEmpty(),
							cmpopts.IgnoreTypes(time.Time{}),
							cmpopts.IgnoreTypes(gorm.Model{}),
							cmpopts.IgnoreTypes(dbmodel.Model{}),
						),
						test.expectedOffer,
					)
				}
			} else {
				c.Assert(err, qt.ErrorMatches, test.expectedError)
			}
		})
	}
}

func TestFindApplicationOffers(t *testing.T) {
	c := qt.New(t)

	ctx := context.Background()
	now := time.Now().UTC().Round(time.Millisecond)

	expectedOffer := dbmodel.ApplicationOffer{
		ID:      1,
		UUID:    "00000000-0000-0000-0000-0000-0000000000011",
		URL:     "test-offer-url",
		Name:    "test-offer",
		ModelID: 1,
		Model: dbmodel.Model{
			UUID: sql.NullString{
				String: "00000000-0000-0000-0000-0000-0000000000003",
				Valid:  true,
			},
		},
		ApplicationName: "test-app",
		CharmURL:        "cs:test-app:17",
	}

	tests := []struct {
		about         string
		parameterFunc func(*environment) (dbmodel.User, string, []jujuparams.OfferFilter)
		expectedError string
		expectedOffer *dbmodel.ApplicationOffer
	}{{
		about: "find an offer as model admin",
		parameterFunc: func(env *environment) (dbmodel.User, string, []jujuparams.OfferFilter) {
			return env.users[0], "admin", []jujuparams.OfferFilter{{
				OfferName: "test-offer",
			}}
		},
		expectedOffer: &expectedOffer,
	}, {
		about: "find an offer as offer admin",
		parameterFunc: func(env *environment) (dbmodel.User, string, []jujuparams.OfferFilter) {
			return env.users[5], "admin", []jujuparams.OfferFilter{{
				OfferName: "test-offer",
			}}
		},
		expectedOffer: &expectedOffer,
	}, {
		about: "find an offer as superuser",
		parameterFunc: func(env *environment) (dbmodel.User, string, []jujuparams.OfferFilter) {
			return env.users[6], "admin", []jujuparams.OfferFilter{{
				OfferName: "test-offer",
			}}
		},
		expectedOffer: &expectedOffer,
	}, {
		about: "offer not found",
		parameterFunc: func(env *environment) (dbmodel.User, string, []jujuparams.OfferFilter) {
			return env.users[0], "admin", []jujuparams.OfferFilter{{
				OfferName: "no-such-offer",
			}}
		},
	}, {
		about: "user without access cannot find offers",
		parameterFunc: func(env *environment) (dbmodel.User, string, []jujuparams.OfferFilter) {
			return env.users[4], "", []jujuparams.OfferFilter{{
				OfferName: "test-offer",
			}}
		},
	}}

	for _, test := range tests {
		c.Run(test.about, func(c *qt.C) {

			db := db.Database{
				DB: jimmtest.MemoryDB(c, func() time.Time { return now }),
			}
			err := db.Migrate(ctx, false)
			c.Assert(err, qt.IsNil)

			_, client, _, err := jimmtest.SetupTestOFGAClient(c.Name(), test.about)
			c.Assert(err, qt.IsNil)

			jimmUUID := uuid.NewString()

			environment := initializeEnvironment(c, ctx, &db, client, jimmUUID)
			user, accessLevel, filters := test.parameterFunc(environment)

			j := &jimm.JIMM{
				UUID:     jimmUUID,
				Database: db,
				Dialer: &jimmtest.Dialer{
					API: &jimmtest.API{},
				},
				OpenFGAClient: client,
			}

			offers, err := j.FindApplicationOffers(ctx, openfga.NewUser(&user, client), filters...)
			if test.expectedError == "" {
				c.Assert(err, qt.IsNil)
				if test.expectedOffer != nil {
					details := test.expectedOffer.ToJujuApplicationOfferDetails()
					if accessLevel != string(jujuparams.OfferAdminAccess) {
						details.Users = []jujuparams.OfferUserDetails{{
							UserName: user.Username,
							Access:   accessLevel,
						}}
					} else {
						details.Users = []jujuparams.OfferUserDetails{{
							UserName: "alice@external",
							Access:   "admin",
						}, {
							UserName: "bob@external",
							Access:   "consume",
						}, {
							UserName: "eve@external",
							Access:   "admin",
						}, {
							UserName: "fred@external",
							Access:   "read",
						}, {
							UserName: "jane@external",
							Access:   "admin",
						}, {
							// joe is jimm admin
							UserName: "joe@external",
							Access:   "admin",
						}}
					}
					for i, _ := range offers {
						users := offers[i].Users
						sort.Slice(users, func(i, j int) bool {
							return users[i].UserName < users[j].UserName
						})
						offers[i].Users = users
					}

					c.Assert(
						offers,
						qt.CmpEquals(
							cmpopts.EquateEmpty(),
							cmpopts.IgnoreTypes(time.Time{}),
							cmpopts.IgnoreTypes(gorm.Model{}),
							cmpopts.IgnoreTypes(dbmodel.Model{}),
						),
						[]jujuparams.ApplicationOfferAdminDetails{details},
					)
				} else {
					c.Assert(offers, qt.HasLen, 0)
				}
			} else {
				c.Assert(err, qt.ErrorMatches, test.expectedError)
			}
		})
	}
}

const listApplicationsTestEnv = `clouds:
- name: test-cloud
  type: test-provider
  regions:
  - name: test-cloud-region
cloud-credentials:
- owner: alice@external
  name: cred-1
  cloud: test-cloud
controllers:
- name: controller-1
  uuid: 00000001-0000-0000-0000-000000000001
  cloud: test-cloud
  region: test-cloud-region
models:
- name: model-1
  type: iaas
  uuid: 00000002-0000-0000-0000-000000000001
  controller: controller-1
  default-series: warty
  cloud: test-cloud
  region: test-cloud-region
  cloud-credential: cred-1
  owner: bob@external
  life: alive
  status:
    status: available
    info: "OK!"
    since: 2020-02-20T20:02:20Z
  users:
  - user: alice@external
    access: admin
  - user: bob@external
    access: admin
  - user: charlie@external
    access: read
  sla:
    level: unsupported
  agent-version: 1.2.3
- name: model-2
  type: iaas
  uuid: 00000002-0000-0000-0000-000000000002
  controller: controller-1
  default-series: warty
  cloud: test-cloud
  region: test-cloud-region
  cloud-credential: cred-1
  owner: alice@external
  life: alive
  status:
    status: available
    info: "OK!"
    since: 2020-02-20T20:02:20Z
  users:
  - user: alice@external
    access: admin
  - user: bob@external
    access: write
  - user: charlie@external
    access: read
  sla:
    level: unsupported
  agent-version: 1.2.3
`

func TestListApplicationOffers(t *testing.T) {
	c := qt.New(t)

	ctx := context.Background()
	now := time.Now().UTC().Round(time.Millisecond)

	_, client, _, err := jimmtest.SetupTestOFGAClient(c.Name())
	c.Assert(err, qt.IsNil)

	db := db.Database{
		DB: jimmtest.MemoryDB(c, func() time.Time { return now }),
	}
	err = db.Migrate(ctx, false)
	c.Assert(err, qt.IsNil)
	env := jimmtest.ParseEnvironment(c, listApplicationsTestEnv)
	env.PopulateDB(c, db, client)

	j := &jimm.JIMM{
		UUID:          uuid.NewString(),
		OpenFGAClient: client,
		Database:      db,
		Dialer: &jimmtest.Dialer{
			API: &jimmtest.API{
				ListApplicationOffers_: func(_ context.Context, filters []jujuparams.OfferFilter) ([]jujuparams.ApplicationOfferAdminDetails, error) {
					switch filters[0].ModelName {
					case "model-1":
						return []jujuparams.ApplicationOfferAdminDetails{{
							ApplicationOfferDetails: jujuparams.ApplicationOfferDetails{
								SourceModelTag:         "00000011-0000-0000-0000-000000000001",
								OfferUUID:              "00000012-0000-0000-0000-000000000001",
								OfferURL:               "test-offer-url",
								OfferName:              "offer-1",
								ApplicationDescription: "app description 1",
								Endpoints: []jujuparams.RemoteEndpoint{{
									Name:      "test-endpoint",
									Role:      "requirer",
									Interface: "unknown",
									Limit:     1,
								}},
								Bindings: map[string]string{
									"key1": "value1",
									"key2": "value2",
								},
								Users: []jujuparams.OfferUserDetails{{
									UserName: "alice@external",
									Access:   "admin",
								}, {
									UserName: "eve@external",
									Access:   "read",
								}, {
									UserName: "bob@external",
									Access:   "consume",
								}},
								Spaces: []jujuparams.RemoteSpace{{
									CloudType:  "test-cloud-type",
									Name:       "test-remote-space",
									ProviderId: "test-provider-id",
									ProviderAttributes: map[string]interface{}{
										"attr1": "value1",
										"attr2": "value2",
									},
								}},
							},
							ApplicationName: "application-1",
							CharmURL:        "charm-1",
							Connections: []jujuparams.OfferConnection{{
								SourceModelTag: "00000011-0000-0000-0000-000000000001",
								RelationId:     1,
								Username:       "charlie@external",
								Endpoint:       "an-endpoint",
							}},
						}, {
							ApplicationOfferDetails: jujuparams.ApplicationOfferDetails{
								SourceModelTag:         "00000011-0000-0000-0000-000000000002",
								OfferUUID:              "00000012-0000-0000-0000-000000000002",
								OfferURL:               "test-offer-url",
								OfferName:              "offer-2",
								ApplicationDescription: "app description 2",
								Endpoints: []jujuparams.RemoteEndpoint{{
									Name:      "test-endpoint",
									Role:      "requirer",
									Interface: "unknown",
									Limit:     1,
								}},
								Bindings: map[string]string{
									"key1": "value1",
									"key2": "value2",
								},
								Users: []jujuparams.OfferUserDetails{{
									UserName: "alice@external",
									Access:   "admin",
								}, {
									UserName: "eve@external",
									Access:   "read",
								}, {
									UserName: "bob@external",
									Access:   "consume",
								}},
								Spaces: []jujuparams.RemoteSpace{{
									CloudType:  "test-cloud-type",
									Name:       "test-remote-space",
									ProviderId: "test-provider-id",
									ProviderAttributes: map[string]interface{}{
										"attr1": "value1",
										"attr2": "value2",
									},
								}},
							},
							ApplicationName: "application-2",
							CharmURL:        "charm-2",
							Connections: []jujuparams.OfferConnection{{
								SourceModelTag: "00000011-0000-0000-0000-000000000002",
								RelationId:     2,
								Username:       "charlie@external",
								Endpoint:       "an-endpoint",
							}},
						}}, nil
					case "model-2":
						return []jujuparams.ApplicationOfferAdminDetails{{
							ApplicationOfferDetails: jujuparams.ApplicationOfferDetails{
								SourceModelTag:         "00000011-0000-0000-0000-000000000003",
								OfferUUID:              "00000012-0000-0000-0000-000000000003",
								OfferURL:               "test-offer-url",
								OfferName:              "offer-3",
								ApplicationDescription: "app description 3",
								Endpoints: []jujuparams.RemoteEndpoint{{
									Name:      "test-endpoint",
									Role:      "requirer",
									Interface: "unknown",
									Limit:     1,
								}},
								Bindings: map[string]string{
									"key1": "value1",
									"key2": "value2",
								},
								Users: []jujuparams.OfferUserDetails{{
									UserName: "alice@external",
									Access:   "admin",
								}, {
									UserName: "eve@external",
									Access:   "read",
								}, {
									UserName: "bob@external",
									Access:   "consume",
								}},
								Spaces: []jujuparams.RemoteSpace{{
									CloudType:  "test-cloud-type",
									Name:       "test-remote-space",
									ProviderId: "test-provider-id",
									ProviderAttributes: map[string]interface{}{
										"attr1": "value1",
										"attr2": "value2",
									},
								}},
							},
							ApplicationName: "application-3",
							CharmURL:        "charm-3",
							Connections: []jujuparams.OfferConnection{{
								SourceModelTag: "00000011-0000-0000-0000-000000000003",
								RelationId:     3,
								Username:       "charlie@external",
								Endpoint:       "an-endpoint",
							}},
						}}, nil
					}
					return nil, nil
				},
			},
		},
	}

	tuples := []openfga.Tuple{{
		Object:   ofganames.ConvertTag(names.NewUserTag("alice@external")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewApplicationOfferTag("00000012-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("eve@external")),
		Relation: ofganames.ReaderRelation,
		Target:   ofganames.ConvertTag(names.NewApplicationOfferTag("00000012-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("bob@external")),
		Relation: ofganames.ConsumerRelation,
		Target:   ofganames.ConvertTag(names.NewApplicationOfferTag("00000012-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("alice@external")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewApplicationOfferTag("00000012-0000-0000-0000-000000000002")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("eve@external")),
		Relation: ofganames.ReaderRelation,
		Target:   ofganames.ConvertTag(names.NewApplicationOfferTag("00000012-0000-0000-0000-000000000002")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("bob@external")),
		Relation: ofganames.ConsumerRelation,
		Target:   ofganames.ConvertTag(names.NewApplicationOfferTag("00000012-0000-0000-0000-000000000002")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("alice@external")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewApplicationOfferTag("00000012-0000-0000-0000-000000000003")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("eve@external")),
		Relation: ofganames.ReaderRelation,
		Target:   ofganames.ConvertTag(names.NewApplicationOfferTag("00000012-0000-0000-0000-000000000003")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("bob@external")),
		Relation: ofganames.ConsumerRelation,
		Target:   ofganames.ConvertTag(names.NewApplicationOfferTag("00000012-0000-0000-0000-000000000003")),
	}}
	err = client.AddRelations(context.Background(), tuples...)
	c.Assert(err, qt.IsNil)

	u := env.User("alice@external").DBObject(c, db, client)
	_, err = j.ListApplicationOffers(ctx, openfga.NewUser(&u, client))
	c.Assert(err, qt.ErrorMatches, `at least one filter must be specified`)

	_, err = j.ListApplicationOffers(ctx, openfga.NewUser(&u, client), jujuparams.OfferFilter{})
	c.Assert(err, qt.ErrorMatches, `application offer filter must specify a model name`)

	filters := []jujuparams.OfferFilter{{
		OwnerName: "bob@external",
		ModelName: "model-1",
	}, {
		ModelName: "model-2",
	}}

	offers, err := j.ListApplicationOffers(ctx, openfga.NewUser(&u, client), filters...)
	c.Assert(err, qt.IsNil)

	for i, _ := range offers {
		sort.Slice(offers[i].Users, func(j, k int) bool {
			return offers[i].Users[j].UserName < offers[i].Users[k].UserName
		})
	}
	c.Check(offers, qt.DeepEquals, []jujuparams.ApplicationOfferAdminDetails{{
		ApplicationOfferDetails: jujuparams.ApplicationOfferDetails{
			SourceModelTag:         "00000011-0000-0000-0000-000000000003",
			OfferUUID:              "00000012-0000-0000-0000-000000000003",
			OfferURL:               "test-offer-url",
			OfferName:              "offer-3",
			ApplicationDescription: "app description 3",
			Endpoints: []jujuparams.RemoteEndpoint{{
				Name:      "test-endpoint",
				Role:      "requirer",
				Interface: "unknown",
				Limit:     1,
			}},
			Bindings: map[string]string{
				"key1": "value1",
				"key2": "value2",
			},
			Users: []jujuparams.OfferUserDetails{{
				UserName: "alice@external",
				Access:   "admin",
			}, {
				UserName: "bob@external",
				Access:   "consume",
			}, {
				UserName: "eve@external",
				Access:   "read",
			}},
			Spaces: []jujuparams.RemoteSpace{{
				CloudType:  "test-cloud-type",
				Name:       "test-remote-space",
				ProviderId: "test-provider-id",
				ProviderAttributes: map[string]interface{}{
					"attr1": "value1",
					"attr2": "value2",
				},
			}},
		},
		ApplicationName: "application-3",
		CharmURL:        "charm-3",
		Connections: []jujuparams.OfferConnection{{
			SourceModelTag: "00000011-0000-0000-0000-000000000003",
			RelationId:     3,
			Username:       "charlie@external",
			Endpoint:       "an-endpoint",
		}},
	}, {
		ApplicationOfferDetails: jujuparams.ApplicationOfferDetails{
			SourceModelTag:         "00000011-0000-0000-0000-000000000001",
			OfferUUID:              "00000012-0000-0000-0000-000000000001",
			OfferURL:               "test-offer-url",
			OfferName:              "offer-1",
			ApplicationDescription: "app description 1",
			Endpoints: []jujuparams.RemoteEndpoint{{
				Name:      "test-endpoint",
				Role:      "requirer",
				Interface: "unknown",
				Limit:     1,
			}},
			Bindings: map[string]string{
				"key1": "value1",
				"key2": "value2",
			},
			Users: []jujuparams.OfferUserDetails{{
				UserName: "alice@external",
				Access:   "admin",
			}, {
				UserName: "bob@external",
				Access:   "consume",
			}, {
				UserName: "eve@external",
				Access:   "read",
			}},
			Spaces: []jujuparams.RemoteSpace{{
				CloudType:  "test-cloud-type",
				Name:       "test-remote-space",
				ProviderId: "test-provider-id",
				ProviderAttributes: map[string]interface{}{
					"attr1": "value1",
					"attr2": "value2",
				},
			}},
		},
		ApplicationName: "application-1",
		CharmURL:        "charm-1",
		Connections: []jujuparams.OfferConnection{{
			SourceModelTag: "00000011-0000-0000-0000-000000000001",
			RelationId:     1,
			Username:       "charlie@external",
			Endpoint:       "an-endpoint",
		}},
	}, {
		ApplicationOfferDetails: jujuparams.ApplicationOfferDetails{
			SourceModelTag:         "00000011-0000-0000-0000-000000000002",
			OfferUUID:              "00000012-0000-0000-0000-000000000002",
			OfferURL:               "test-offer-url",
			OfferName:              "offer-2",
			ApplicationDescription: "app description 2",
			Endpoints: []jujuparams.RemoteEndpoint{{
				Name:      "test-endpoint",
				Role:      "requirer",
				Interface: "unknown",
				Limit:     1,
			}},
			Bindings: map[string]string{
				"key1": "value1",
				"key2": "value2",
			},
			Users: []jujuparams.OfferUserDetails{{
				UserName: "alice@external",
				Access:   "admin",
			}, {
				UserName: "bob@external",
				Access:   "consume",
			}, {
				UserName: "eve@external",
				Access:   "read",
			}},
			Spaces: []jujuparams.RemoteSpace{{
				CloudType:  "test-cloud-type",
				Name:       "test-remote-space",
				ProviderId: "test-provider-id",
				ProviderAttributes: map[string]interface{}{
					"attr1": "value1",
					"attr2": "value2",
				},
			}},
		},
		ApplicationName: "application-2",
		CharmURL:        "charm-2",
		Connections: []jujuparams.OfferConnection{{
			SourceModelTag: "00000011-0000-0000-0000-000000000002",
			RelationId:     2,
			Username:       "charlie@external",
			Endpoint:       "an-endpoint",
		}},
	}})
}
