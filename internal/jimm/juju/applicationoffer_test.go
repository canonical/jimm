// Copyright 2026 Canonical.

package juju_test

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
	"github.com/juju/charm/v12"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/permission"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v5"
	"gopkg.in/macaroon.v2"
	"gorm.io/gorm"

	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/jimm/juju"
	"github.com/canonical/jimm/v3/internal/jujuclient"
	"github.com/canonical/jimm/v3/internal/openfga"
	ofganames "github.com/canonical/jimm/v3/internal/openfga/names"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
)

type environment struct {
	users             []dbmodel.Identity
	clouds            []dbmodel.Cloud
	credentials       []dbmodel.CloudCredential
	controllers       []dbmodel.Controller
	models            []dbmodel.Model
	applicationOffers []dbmodel.ApplicationOffer
}

var initializeEnvironment = func(c *qt.C, ctx context.Context, db *db.Database, client *openfga.OFGAClient, jimmTag names.ControllerTag) *environment {
	env := environment{}

	// Alice is a model admin, but not a superuser or offer admin.
	u, err := dbmodel.NewIdentity("alice@canonical.com")
	c.Assert(err, qt.IsNil)
	c.Assert(db.DB.Create(u).Error, qt.IsNil)

	u1, err := dbmodel.NewIdentity("eve@canonical.com")
	c.Assert(err, qt.IsNil)
	c.Assert(db.DB.Create(u1).Error, qt.IsNil)

	u2, err := dbmodel.NewIdentity("bob@canonical.com")
	c.Assert(err, qt.IsNil)
	c.Assert(db.DB.Create(u2).Error, qt.IsNil)

	u3, err := dbmodel.NewIdentity("fred@canonical.com")
	c.Assert(err, qt.IsNil)
	c.Assert(db.DB.Create(u3).Error, qt.IsNil)

	u4, err := dbmodel.NewIdentity("grant@canonical.com")
	c.Assert(err, qt.IsNil)
	c.Assert(db.DB.Create(u4).Error, qt.IsNil)

	// Jane is an offer admin, but not a superuser or model admin.
	u5, err := dbmodel.NewIdentity("jane@canonical.com")
	c.Assert(err, qt.IsNil)
	c.Assert(db.DB.Create(u5).Error, qt.IsNil)

	// Joe is a superuser, but not a model or offer admin.
	u6, err := dbmodel.NewIdentity("joe@canonical.com")
	c.Assert(err, qt.IsNil)
	c.Assert(db.DB.Create(u6).Error, qt.IsNil)

	err = openfga.NewUser(u6, client).SetControllerAccess(ctx, jimmTag, ofganames.AdministratorRelation)
	c.Assert(err, qt.IsNil)

	env.users = []dbmodel.Identity{*u, *u1, *u2, *u3, *u4, *u5, *u6}

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
	err = openfga.NewUser(u, client).SetCloudAccess(ctx, cloud.ResourceTag(), ofganames.AdministratorRelation)
	c.Assert(err, qt.IsNil)

	controller := dbmodel.Controller{
		Name:          "test-controller-1",
		UUID:          "00000000-0000-0000-0000-000000000001",
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

	err = client.AddController(context.Background(), jimmTag, controller.ResourceTag())
	c.Assert(err, qt.IsNil)

	cred := dbmodel.CloudCredential{
		Name:              "test-credential-1",
		CloudName:         cloud.Name,
		OwnerIdentityName: u.Name,
		AuthType:          "empty",
	}
	err = db.SetCloudCredential(ctx, &cred)
	c.Assert(err, qt.IsNil)
	env.credentials = []dbmodel.CloudCredential{cred}

	model := dbmodel.Model{
		Name: "test-model",
		UUID: sql.NullString{
			String: "00000000-0000-0000-0000-000000000003",
			Valid:  true,
		},
		OwnerIdentityName: u.Name,
		ControllerID:      controller.ID,
		CloudRegionID:     cloud.Regions[0].ID,
		CloudCredentialID: cred.ID,
	}
	err = db.AddModel(ctx, &model)
	c.Assert(err, qt.IsNil)
	env.models = []dbmodel.Model{model}

	// user u is administrator of the test-model
	err = openfga.NewUser(u, client).SetModelAccess(ctx, model.ResourceTag(), ofganames.AdministratorRelation)
	c.Assert(err, qt.IsNil)

	err = client.AddControllerModel(context.Background(), controller.ResourceTag(), model.ResourceTag())
	c.Assert(err, qt.IsNil)

	offer := dbmodel.ApplicationOffer{
		ID:      1,
		UUID:    "00000012-0000-0000-0000-000000000001",
		URL:     "test-offer-url",
		Name:    "test-offer",
		ModelID: model.ID,
		Model:   model,
	}
	err = db.AddApplicationOffer(ctx, &offer)
	c.Assert(err, qt.IsNil)
	env.applicationOffers = []dbmodel.ApplicationOffer{offer}

	err = client.AddModelApplicationOffer(context.Background(), model.ResourceTag(), offer.ResourceTag())
	c.Assert(err, qt.IsNil)

	// user u1 is administrator of the test-offer
	err = openfga.NewUser(u1, client).SetApplicationOfferAccess(ctx, offer.ResourceTag(), ofganames.AdministratorRelation)
	c.Assert(err, qt.IsNil)

	// user u2 is consumer of the test-offer
	err = openfga.NewUser(u2, client).SetApplicationOfferAccess(ctx, offer.ResourceTag(), ofganames.ConsumerRelation)
	c.Assert(err, qt.IsNil)

	// user u3 is reader of the test-offer
	err = openfga.NewUser(u3, client).SetApplicationOfferAccess(ctx, offer.ResourceTag(), ofganames.ReaderRelation)
	c.Assert(err, qt.IsNil)

	// user u5 is administrator of the test-offer
	err = openfga.NewUser(u5, client).SetApplicationOfferAccess(ctx, offer.ResourceTag(), ofganames.AdministratorRelation)
	c.Assert(err, qt.IsNil)

	return &env
}

func TestGetApplicationOfferConsumeDetails(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	api := jimmtest.API{}
	j := newTestJujuManager(c, &parameters{
		Dialer: &jimmtest.Dialer{
			UUID: "00000000-0000-0000-0000-000000000001",
			API:  &api,
		},
	})

	db := j.Database
	client := j.OpenFGAClient

	u, err := dbmodel.NewIdentity("alice@canonical.com")
	c.Assert(err, qt.IsNil)
	c.Assert(db.DB.Create(u).Error, qt.IsNil)

	u1, err := dbmodel.NewIdentity("eve@canonical.com")
	c.Assert(err, qt.IsNil)
	c.Assert(db.DB.Create(u1).Error, qt.IsNil)

	u2, err := dbmodel.NewIdentity("bob@canonical.com")
	c.Assert(err, qt.IsNil)
	c.Assert(db.DB.Create(u2).Error, qt.IsNil)

	cloud := dbmodel.Cloud{
		Name: "test-cloud",
		Type: "test-provider",
		Regions: []dbmodel.CloudRegion{{
			Name: "test-region-1",
		}},
	}
	c.Assert(db.DB.Create(&cloud).Error, qt.IsNil)

	// user u is administrator of the test-model
	err = openfga.NewUser(u, client).SetCloudAccess(ctx, cloud.ResourceTag(), ofganames.AdministratorRelation)
	c.Assert(err, qt.IsNil)

	controller := dbmodel.Controller{
		Name:          "test-controller-1",
		UUID:          "00000000-0000-0000-0000-000000000001",
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
		Name:              "test-credential-1",
		CloudName:         cloud.Name,
		OwnerIdentityName: u.Name,
		AuthType:          "empty",
	}
	err = db.SetCloudCredential(ctx, &cred)
	c.Assert(err, qt.IsNil)

	model := dbmodel.Model{
		Name: "test-model",
		UUID: sql.NullString{
			String: "00000000-0000-0000-0000-000000000003",
			Valid:  true,
		},
		OwnerIdentityName: u.Name,
		ControllerID:      controller.ID,
		CloudRegionID:     cloud.Regions[0].ID,
		CloudCredentialID: cred.ID,
	}
	err = db.AddModel(ctx, &model)
	c.Assert(err, qt.IsNil)

	offer := dbmodel.ApplicationOffer{
		ID:      1,
		UUID:    uuid.NewString(),
		URL:     "test-offer-url",
		ModelID: model.ID,
		Model:   model,
	}
	err = db.AddApplicationOffer(ctx, &offer)
	c.Assert(err, qt.IsNil)

	// user u is administrator of the test offer
	err = openfga.NewUser(u, client).SetApplicationOfferAccess(ctx, offer.ResourceTag(), ofganames.AdministratorRelation)
	c.Assert(err, qt.IsNil)

	// user u1 is reader of the test offer
	err = openfga.NewUser(u1, client).SetApplicationOfferAccess(ctx, offer.ResourceTag(), ofganames.ReaderRelation)
	c.Assert(err, qt.IsNil)

	// user u2 is consumer of the test offer
	err = openfga.NewUser(u2, client).SetApplicationOfferAccess(ctx, offer.ResourceTag(), ofganames.ConsumerRelation)
	c.Assert(err, qt.IsNil)

	everyoneTag := names.NewUserTag(ofganames.EveryoneUser)
	uAll, err := dbmodel.NewIdentity(everyoneTag.Id())
	c.Assert(err, qt.IsNil)
	c.Assert(db.DB.Create(uAll).Error, qt.IsNil)
	// user uAll is reader of the test offer
	err = openfga.NewUser(uAll, client).SetApplicationOfferAccess(ctx, offer.ResourceTag(), ofganames.ReaderRelation)
	c.Assert(err, qt.IsNil)

	api.GetApplicationOfferConsumeDetails_ = func(ctx context.Context, s string) (jujuparams.ConsumeOfferDetails, error) {
		return jujuparams.ConsumeOfferDetails{
			Offer: &jujuparams.ApplicationOfferDetailsV5{
				SourceModelTag: names.NewModelTag(model.UUID.String).String(),
				OfferUUID:      offer.UUID,
				OfferURL:       offer.URL,
				OfferName:      offer.Name,
				Endpoints: []jujuparams.RemoteEndpoint{{
					Name:      "test-endpoint",
					Role:      "requirer",
					Interface: "unknown",
					Limit:     1,
				}},
				Users: []jujuparams.OfferUserDetails{{
					UserName: "alice@canonical.com",
					Access:   "admin",
				}, {
					UserName: "eve@canonical.com",
					Access:   "read",
				}, {
					UserName: "bob@canonical.com",
					Access:   "consume",
				}},
			},
			Macaroon: &macaroon.Macaroon{},
			ControllerInfo: &jujuparams.ExternalControllerInfo{
				ControllerTag: names.NewControllerTag(controller.UUID).String(),
			},
		}, nil
	}

	tests := []struct {
		about                string
		user                 *dbmodel.Identity
		details              jujuparams.ConsumeOfferDetails
		expectedOfferDetails jujuparams.ConsumeOfferDetails
		expectedError        string
	}{{
		about: "admin can get the application offer consume details ",
		user:  u,
		details: jujuparams.ConsumeOfferDetails{
			Offer: &jujuparams.ApplicationOfferDetailsV5{
				OfferUUID: "00000012-0000-0000-0000-000000000001",
				OfferURL:  "test-offer-url",
			},
		},
		expectedOfferDetails: jujuparams.ConsumeOfferDetails{
			ControllerInfo: &jujuparams.ExternalControllerInfo{
				ControllerTag: names.NewControllerTag(controller.UUID).String(),
				Alias:         "test-controller-1",
				Addrs:         []string{"test-public-address"},
			},
			Macaroon: &macaroon.Macaroon{},
			Offer: &jujuparams.ApplicationOfferDetailsV5{
				SourceModelTag: names.NewModelTag(model.UUID.String).String(),
				OfferUUID:      offer.UUID,
				OfferURL:       offer.URL,
				OfferName:      offer.Name,
				Endpoints: []jujuparams.RemoteEndpoint{{
					Name:      "test-endpoint",
					Role:      "requirer",
					Interface: "unknown",
					Limit:     1,
				}},
				Users: []jujuparams.OfferUserDetails{{
					UserName: "alice@canonical.com",
					Access:   "admin",
				}, {
					UserName: "bob@canonical.com",
					Access:   "consume",
				}, {
					UserName: "eve@canonical.com",
					Access:   "read",
				}, {
					UserName: "everyone@external",
					Access:   "read",
				}},
			},
		},
	}, {
		about: "users with consume access can get the application offer consume details with filtered users",
		user:  u2,
		details: jujuparams.ConsumeOfferDetails{
			Offer: &jujuparams.ApplicationOfferDetailsV5{
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
			Offer: &jujuparams.ApplicationOfferDetailsV5{
				SourceModelTag: names.NewModelTag(model.UUID.String).String(),
				OfferUUID:      offer.UUID,
				OfferURL:       offer.URL,
				OfferName:      offer.Name,
				Endpoints: []jujuparams.RemoteEndpoint{{
					Name:      "test-endpoint",
					Role:      "requirer",
					Interface: "unknown",
					Limit:     1,
				}},
				Users: []jujuparams.OfferUserDetails{{
					UserName: "bob@canonical.com",
					Access:   "consume",
				}, {
					UserName: "everyone@external",
					Access:   "read",
				}},
			},
		},
	}, {
		about: "user with read access cannot get application offer consume details",
		user:  u1,
		details: jujuparams.ConsumeOfferDetails{
			Offer: &jujuparams.ApplicationOfferDetailsV5{
				OfferURL: "test-offer-url",
			},
		},
		expectedError: "unauthorized",
	}, {
		about: "no such offer",
		user:  u,
		details: jujuparams.ConsumeOfferDetails{
			Offer: &jujuparams.ApplicationOfferDetailsV5{
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

	j := newTestJujuManager(c, &parameters{
		Dialer: &jimmtest.Dialer{
			API: &jimmtest.API{
				GetApplicationOffer_: func(ctx context.Context, s string) (*crossmodel.ApplicationOfferDetails, error) {
					return &crossmodel.ApplicationOfferDetails{
						OfferUUID:              "00000000-0000-0000-0000-000000000004",
						OfferName:              "test-offer",
						ApplicationName:        "test-app",
						ApplicationDescription: "changed offer description",
						OfferURL:               "test-offer-url",
						CharmURL:               "cs:test-app:17",
						Endpoints: []charm.Relation{
							{
								Name:      "test-endpoint",
								Role:      charm.RoleRequirer,
								Interface: "unknown",
								Limit:     1,
							},
						},
						Connections: []crossmodel.OfferConnection{
							{
								SourceModelUUID: "test-model-src",
								Username:        "unknown",
								Endpoint:        "test-endpoint",
								RelationId:      1,
							},
						},
						Users: []crossmodel.OfferUserDetails{
							{
								UserName: "alice@canonical.com",
								Access:   permission.AdminAccess,
							},
							{
								UserName: "eve@canonical.com",
								Access:   permission.ReadAccess,
							},
							{
								UserName: "admin",
								Access:   permission.AdminAccess,
							},
						},
					}, nil
				},
			},
		},
	})

	u, err := dbmodel.NewIdentity("alice@canonical.com")
	c.Assert(err, qt.IsNil)
	c.Assert(j.Database.DB.Create(&u).Error, qt.IsNil)

	u1, err := dbmodel.NewIdentity("eve@canonical.com")
	c.Assert(err, qt.IsNil)
	c.Assert(j.Database.DB.Create(&u1).Error, qt.IsNil)

	u2, err := dbmodel.NewIdentity("bob@canonical.com")
	c.Assert(err, qt.IsNil)
	c.Assert(j.Database.DB.Create(&u2).Error, qt.IsNil)

	cloud := dbmodel.Cloud{
		Name: "test-cloud",
		Type: "test-provider",
		Regions: []dbmodel.CloudRegion{{
			Name: "test-region-1",
		}},
	}
	c.Assert(j.Database.DB.Create(&cloud).Error, qt.IsNil)

	controller := dbmodel.Controller{
		Name:        "test-controller-1",
		UUID:        "00000000-0000-0000-0000-000000000001",
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
		Name:              "test-credential-1",
		CloudName:         cloud.Name,
		OwnerIdentityName: u.Name,
		AuthType:          "empty",
	}
	err = j.Database.SetCloudCredential(ctx, &cred)
	c.Assert(err, qt.IsNil)

	model := dbmodel.Model{
		Name: "test-model",
		UUID: sql.NullString{
			String: "00000000-0000-0000-0000-000000000003",
			Valid:  true,
		},
		OwnerIdentityName: u.Name,
		ControllerID:      controller.ID,
		CloudRegionID:     cloud.Regions[0].ID,
		CloudCredentialID: cred.ID,
	}
	err = j.Database.AddModel(ctx, &model)
	c.Assert(err, qt.IsNil)

	offer := dbmodel.ApplicationOffer{
		ID:      1,
		ModelID: 1,
		Name:    "test-application-offer",
		UUID:    "00000000-0000-0000-0000-000000000004",
		URL:     "test-offer-url",
	}
	err = j.Database.AddApplicationOffer(ctx, &offer)
	c.Assert(err, qt.IsNil)

	// user u is administrator of the test offer
	err = openfga.NewUser(u, j.OpenFGAClient).SetApplicationOfferAccess(ctx, offer.ResourceTag(), ofganames.AdministratorRelation)
	c.Assert(err, qt.IsNil)

	// user u1 is reader of the test offer
	err = openfga.NewUser(u1, j.OpenFGAClient).SetApplicationOfferAccess(ctx, offer.ResourceTag(), ofganames.ReaderRelation)
	c.Assert(err, qt.IsNil)

	tests := []struct {
		about                string
		user                 *dbmodel.Identity
		offerURL             string
		expectedOfferDetails crossmodel.ApplicationOfferDetails
		expectedError        string
	}{{
		about:    "admin can get the application offer",
		user:     u,
		offerURL: "test-offer-url",
		expectedOfferDetails: crossmodel.ApplicationOfferDetails{
			OfferUUID:              "00000000-0000-0000-0000-000000000004",
			OfferName:              "test-offer",
			OfferURL:               "test-offer-url",
			ApplicationDescription: "changed offer description",
			Endpoints: []charm.Relation{{
				Name:      "test-endpoint",
				Role:      "requirer",
				Interface: "unknown",
				Limit:     1,
			}},
			Users: []crossmodel.OfferUserDetails{{
				UserName: "alice@canonical.com",
				Access:   "admin",
			}, {
				UserName: "eve@canonical.com",
				Access:   "read",
			}},
			ApplicationName: "test-app",
			CharmURL:        "cs:test-app:17",
			Connections: []crossmodel.OfferConnection{{
				SourceModelUUID: "test-model-src",
				RelationId:      1,
				Username:        "unknown",
				Endpoint:        "test-endpoint",
			}},
		},
	}, {
		about:    "user with read access can get the application offer, but users and connections are filtered",
		user:     u1,
		offerURL: "test-offer-url",
		expectedOfferDetails: crossmodel.ApplicationOfferDetails{
			OfferUUID:              "00000000-0000-0000-0000-000000000004",
			OfferName:              "test-offer",
			OfferURL:               "test-offer-url",
			ApplicationName:        "test-app",
			ApplicationDescription: "changed offer description",
			CharmURL:               "cs:test-app:17",
			Endpoints: []charm.Relation{{
				Name:      "test-endpoint",
				Role:      "requirer",
				Interface: "unknown",
				Limit:     1,
			}},
			Users: []crossmodel.OfferUserDetails{{
				UserName: "eve@canonical.com",
				Access:   "read",
			}},
		},
	}, {
		about:         "user without access cannot get the application offer",
		user:          u2,
		offerURL:      "test-offer-url",
		expectedError: "application offer not found",
	}, {
		about:         "not found",
		user:          u1,
		offerURL:      "offer-not-found",
		expectedError: "application offer not found",
	}}

	for _, test := range tests {
		c.Run(test.about, func(c *qt.C) {
			details, err := j.GetApplicationOffer(ctx, openfga.NewUser(test.user, j.OpenFGAClient), test.offerURL)
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

	tests := []struct {
		about               string
		getApplicationOffer func(context.Context, string) (*crossmodel.ApplicationOfferDetails, error)
		offer               func(context.Context, jujuclient.OfferParams) error
		createEnv           func(*qt.C, *db.Database, *openfga.OFGAClient) (dbmodel.Identity, juju.AddApplicationOfferParams, dbmodel.ApplicationOffer, func(*qt.C, error))
	}{{
		about: "all ok",
		getApplicationOffer: func(_ context.Context, _ string) (*crossmodel.ApplicationOfferDetails, error) {
			return &crossmodel.ApplicationOfferDetails{
				ApplicationName:        "test-app",
				CharmURL:               "cs:test-app:17",
				OfferName:              "test-app-offer",
				OfferURL:               "test-offer-url",
				OfferUUID:              "00000000-0000-0000-0000-000000000004",
				ApplicationDescription: "a test app offering",
				Endpoints: []charm.Relation{{
					Name:      "test-endpoint",
					Role:      charm.RoleRequirer,
					Interface: "unknown",
					Limit:     1,
				}},
				Connections: []crossmodel.OfferConnection{{
					SourceModelUUID: "test-model-src",
					RelationId:      1,
					Username:        "unknown",
					Endpoint:        "test-endpoint",
				}},
				Users: []crossmodel.OfferUserDetails{{
					UserName:    "alice",
					DisplayName: "alice, sister of eve",
					Access:      permission.AdminAccess,
				}},
			}, nil
		},
		offer: func(context.Context, jujuclient.OfferParams) error {
			return nil
		},
		createEnv: func(c *qt.C, db *db.Database, client *openfga.OFGAClient) (dbmodel.Identity, juju.AddApplicationOfferParams, dbmodel.ApplicationOffer, func(*qt.C, error)) {
			ctx := context.Background()

			u, err := dbmodel.NewIdentity("alice@canonical.com")
			c.Assert(err, qt.IsNil)
			c.Assert(db.DB.Create(u).Error, qt.IsNil)

			cloud := dbmodel.Cloud{
				Name: "test-cloud",
				Type: "test-provider",
				Regions: []dbmodel.CloudRegion{{
					Name: "test-region-1",
				}},
			}
			c.Assert(db.DB.Create(&cloud).Error, qt.IsNil)

			// user u is administrator of the test-cloud
			err = openfga.NewUser(u, client).SetCloudAccess(ctx, cloud.ResourceTag(), ofganames.AdministratorRelation)
			c.Assert(err, qt.IsNil)

			controller := dbmodel.Controller{
				Name:        "test-controller-1",
				UUID:        "00000000-0000-0000-0000-000000000001",
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
				Name:              "test-credential-1",
				CloudName:         cloud.Name,
				OwnerIdentityName: u.Name,
				AuthType:          "empty",
			}
			err = db.SetCloudCredential(ctx, &cred)
			c.Assert(err, qt.IsNil)

			model := dbmodel.Model{
				Name: "test-model",
				UUID: sql.NullString{
					String: "00000000-0000-0000-0000-000000000003",
					Valid:  true,
				},
				OwnerIdentityName: u.Name,
				ControllerID:      controller.ID,
				CloudRegionID:     cloud.Regions[0].ID,
				CloudCredentialID: cred.ID,
			}
			err = db.AddModel(ctx, &model)
			c.Assert(err, qt.IsNil)

			// user u is administrator of the test-model
			err = openfga.NewUser(u, client).SetModelAccess(ctx, model.ResourceTag(), ofganames.AdministratorRelation)
			c.Assert(err, qt.IsNil)

			offerParams := juju.AddApplicationOfferParams{
				ModelTag:               model.ResourceTag(),
				OfferName:              "test-app-offer",
				ApplicationName:        "test-app",
				ApplicationDescription: "a test app offering",
				Endpoints: map[string]string{
					"endpoint1": "url1",
				},
			}

			offer := dbmodel.ApplicationOffer{
				ID:      1,
				Name:    offerParams.OfferName,
				ModelID: 1,
				UUID:    "00000000-0000-0000-0000-000000000004",
				URL:     "test-offer-url",
			}

			return *u, offerParams, offer, nil
		},
	}, {
		about: "controller returns an error when creating an offer",
		getApplicationOffer: func(_ context.Context, _ string) (*crossmodel.ApplicationOfferDetails, error) {
			return nil, nil
		},
		offer: func(context.Context, jujuclient.OfferParams) error {
			return errors.New("a silly error")
		},
		createEnv: func(c *qt.C, db *db.Database, client *openfga.OFGAClient) (dbmodel.Identity, juju.AddApplicationOfferParams, dbmodel.ApplicationOffer, func(*qt.C, error)) {
			ctx := context.Background()

			u, err := dbmodel.NewIdentity("alice@canonical.com")
			c.Assert(err, qt.IsNil)
			c.Assert(db.DB.Create(u).Error, qt.IsNil)

			cloud := dbmodel.Cloud{
				Name: "test-cloud",
				Type: "test-provider",
				Regions: []dbmodel.CloudRegion{{
					Name: "test-region-1",
				}},
			}
			c.Assert(db.DB.Create(&cloud).Error, qt.IsNil)

			// user u is administrator of the test-cloud
			err = openfga.NewUser(u, client).SetCloudAccess(ctx, cloud.ResourceTag(), ofganames.AdministratorRelation)
			c.Assert(err, qt.IsNil)

			controller := dbmodel.Controller{
				Name:        "test-controller-1",
				UUID:        "00000000-0000-0000-0000-000000000001",
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
				Name:              "test-credential-1",
				CloudName:         cloud.Name,
				OwnerIdentityName: u.Name,
				AuthType:          "empty",
			}
			err = db.SetCloudCredential(ctx, &cred)
			c.Assert(err, qt.IsNil)

			model := dbmodel.Model{
				Name: "test-model",
				UUID: sql.NullString{
					String: "00000000-0000-0000-0000-000000000003",
					Valid:  true,
				},
				OwnerIdentityName: u.Name,
				ControllerID:      controller.ID,
				CloudRegionID:     cloud.Regions[0].ID,
				CloudCredentialID: cred.ID,
			}
			err = db.AddModel(ctx, &model)
			c.Assert(err, qt.IsNil)

			// user u is administrator of the test-model
			err = openfga.NewUser(u, client).SetModelAccess(ctx, model.ResourceTag(), ofganames.AdministratorRelation)
			c.Assert(err, qt.IsNil)

			offerParams := juju.AddApplicationOfferParams{
				ModelTag:               model.ResourceTag(),
				OfferName:              "test-app-offer",
				ApplicationName:        "test-app",
				ApplicationDescription: "a test app offering",
				Endpoints: map[string]string{
					"endpoint1": "url1",
				},
			}

			offer := dbmodel.ApplicationOffer{}

			return *u, offerParams, offer, func(c *qt.C, err error) {
				c.Assert(err, qt.ErrorMatches, "a silly error")
			}
		},
	}, {
		about: "model not found",
		getApplicationOffer: func(ctx context.Context, s string) (*crossmodel.ApplicationOfferDetails, error) {
			return nil, nil
		},
		offer: func(context.Context, jujuclient.OfferParams) error {
			return nil
		},
		createEnv: func(c *qt.C, db *db.Database, client *openfga.OFGAClient) (dbmodel.Identity, juju.AddApplicationOfferParams, dbmodel.ApplicationOffer, func(*qt.C, error)) {
			u, err := dbmodel.NewIdentity("alice@canonical.com")
			c.Assert(err, qt.IsNil)

			c.Assert(db.DB.Create(u).Error, qt.IsNil)
			offerParams := juju.AddApplicationOfferParams{
				ModelTag:               names.NewModelTag("model-not-found"),
				OfferName:              "test-app-offer",
				ApplicationName:        "test-app",
				ApplicationDescription: "a test app offering",
				Endpoints: map[string]string{
					"endpoint1": "url1",
				},
			}

			offer := dbmodel.ApplicationOffer{}

			return *u, offerParams, offer, func(c *qt.C, err error) {
				c.Assert(err, qt.ErrorMatches, "model not found")
			}
		},
	}, {
		about: "application not found",
		getApplicationOffer: func(ctx context.Context, s string) (*crossmodel.ApplicationOfferDetails, error) {
			return nil, nil
		},
		offer: func(context.Context, jujuclient.OfferParams) error {
			return errors.E(errors.CodeNotFound, "application test-app")
		},
		createEnv: func(c *qt.C, db *db.Database, client *openfga.OFGAClient) (dbmodel.Identity, juju.AddApplicationOfferParams, dbmodel.ApplicationOffer, func(*qt.C, error)) {
			ctx := context.Background()

			u, err := dbmodel.NewIdentity("alice@canonical.com")
			c.Assert(err, qt.IsNil)

			c.Assert(db.DB.Create(u).Error, qt.IsNil)

			cloud := dbmodel.Cloud{
				Name: "test-cloud",
				Type: "test-provider",
				Regions: []dbmodel.CloudRegion{{
					Name: "test-region-1",
				}},
			}
			c.Assert(db.DB.Create(&cloud).Error, qt.IsNil)

			// user u is administrator of the test-cloud
			err = openfga.NewUser(u, client).SetCloudAccess(ctx, cloud.ResourceTag(), ofganames.AdministratorRelation)
			c.Assert(err, qt.IsNil)

			controller := dbmodel.Controller{
				Name:        "test-controller-1",
				UUID:        "00000000-0000-0000-0000-000000000001",
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
				Name:              "test-credential-1",
				CloudName:         cloud.Name,
				OwnerIdentityName: u.Name,
				AuthType:          "empty",
			}
			err = db.SetCloudCredential(ctx, &cred)
			c.Assert(err, qt.IsNil)

			model := dbmodel.Model{
				Name: "test-model",
				UUID: sql.NullString{
					String: "00000000-0000-0000-0000-000000000003",
					Valid:  true,
				},
				OwnerIdentityName: u.Name,
				ControllerID:      controller.ID,
				CloudRegionID:     cloud.Regions[0].ID,
				CloudCredentialID: cred.ID,
			}
			err = db.AddModel(ctx, &model)
			c.Assert(err, qt.IsNil)

			// user u is administrator of the test-model
			err = openfga.NewUser(u, client).SetModelAccess(ctx, model.ResourceTag(), ofganames.AdministratorRelation)
			c.Assert(err, qt.IsNil)

			offerParams := juju.AddApplicationOfferParams{
				ModelTag:               model.ResourceTag(),
				OfferName:              "test-app-offer",
				ApplicationName:        "test-app",
				ApplicationDescription: "a test app offering",
				Endpoints: map[string]string{
					"endpoint1": "url1",
				},
			}

			offer := dbmodel.ApplicationOffer{}

			return *u, offerParams, offer, func(c *qt.C, err error) {
				c.Assert(errors.ErrorCode(err), qt.Equals, errors.CodeNotFound)
				c.Assert(err, qt.ErrorMatches, "application test-app")
			}
		},
	}, {
		about: "user not model admin",
		getApplicationOffer: func(ctx context.Context, s string) (*crossmodel.ApplicationOfferDetails, error) {
			return nil, nil
		},
		offer: func(context.Context, jujuclient.OfferParams) error {
			return nil
		},
		createEnv: func(c *qt.C, db *db.Database, client *openfga.OFGAClient) (dbmodel.Identity, juju.AddApplicationOfferParams, dbmodel.ApplicationOffer, func(*qt.C, error)) {
			ctx := context.Background()

			u, err := dbmodel.NewIdentity("alice@canonical.com")
			c.Assert(err, qt.IsNil)
			c.Assert(db.DB.Create(u).Error, qt.IsNil)

			u1, err := dbmodel.NewIdentity("eve@canonical.com")
			c.Assert(err, qt.IsNil)
			c.Assert(db.DB.Create(u1).Error, qt.IsNil)

			cloud := dbmodel.Cloud{
				Name: "test-cloud",
				Type: "test-provider",
				Regions: []dbmodel.CloudRegion{{
					Name: "test-region-1",
				}},
			}
			c.Assert(db.DB.Create(&cloud).Error, qt.IsNil)

			// user u is administrator of the test-cloud
			err = openfga.NewUser(u, client).SetCloudAccess(ctx, cloud.ResourceTag(), ofganames.AdministratorRelation)
			c.Assert(err, qt.IsNil)

			controller := dbmodel.Controller{
				Name:        "test-controller-1",
				UUID:        "00000000-0000-0000-0000-000000000001",
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
				Name:              "test-credential-1",
				CloudName:         cloud.Name,
				OwnerIdentityName: u.Name,
				AuthType:          "empty",
			}
			err = db.SetCloudCredential(ctx, &cred)
			c.Assert(err, qt.IsNil)

			model := dbmodel.Model{
				Name: "test-model",
				UUID: sql.NullString{
					String: "00000000-0000-0000-0000-000000000003",
					Valid:  true,
				},
				OwnerIdentityName: u.Name,
				ControllerID:      controller.ID,
				CloudRegionID:     cloud.Regions[0].ID,
				CloudCredentialID: cred.ID,
			}
			err = db.AddModel(ctx, &model)
			c.Assert(err, qt.IsNil)

			// user u is administrator of the test-model
			err = openfga.NewUser(u, client).SetModelAccess(ctx, model.ResourceTag(), ofganames.AdministratorRelation)
			c.Assert(err, qt.IsNil)

			offerParams := juju.AddApplicationOfferParams{
				ModelTag:               model.ResourceTag(),
				OfferName:              "test-app-offer",
				ApplicationName:        "test-app",
				ApplicationDescription: "a test app offering",
				Endpoints: map[string]string{
					"endpoint1": "url1",
				},
			}

			offer := dbmodel.ApplicationOffer{}

			return *u1, offerParams, offer, func(c *qt.C, err error) {
				c.Assert(err, qt.ErrorMatches, "unauthorized")
			}
		},
	}, {
		about: "fail to fetch application offer details",
		getApplicationOffer: func(ctx context.Context, s string) (*crossmodel.ApplicationOfferDetails, error) {
			return nil, errors.New("a silly error")
		},
		offer: func(context.Context, jujuclient.OfferParams) error {
			return nil
		},
		createEnv: func(c *qt.C, db *db.Database, client *openfga.OFGAClient) (dbmodel.Identity, juju.AddApplicationOfferParams, dbmodel.ApplicationOffer, func(*qt.C, error)) {
			ctx := context.Background()

			u, err := dbmodel.NewIdentity("alice@canonical.com")
			c.Assert(err, qt.IsNil)
			c.Assert(db.DB.Create(u).Error, qt.IsNil)

			cloud := dbmodel.Cloud{
				Name: "test-cloud",
				Type: "test-provider",
				Regions: []dbmodel.CloudRegion{{
					Name: "test-region-1",
				}},
			}
			c.Assert(db.DB.Create(&cloud).Error, qt.IsNil)

			// user u is administrator of the test-cloud
			err = openfga.NewUser(u, client).SetCloudAccess(ctx, cloud.ResourceTag(), ofganames.AdministratorRelation)
			c.Assert(err, qt.IsNil)

			controller := dbmodel.Controller{
				Name:        "test-controller-1",
				UUID:        "00000000-0000-0000-0000-000000000001",
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
				Name:              "test-credential-1",
				CloudName:         cloud.Name,
				OwnerIdentityName: u.Name,
				AuthType:          "empty",
			}
			err = db.SetCloudCredential(ctx, &cred)
			c.Assert(err, qt.IsNil)

			model := dbmodel.Model{
				Name: "test-model",
				UUID: sql.NullString{
					String: "00000000-0000-0000-0000-000000000003",
					Valid:  true,
				},
				OwnerIdentityName: u.Name,
				ControllerID:      controller.ID,
				CloudRegionID:     cloud.Regions[0].ID,
				CloudCredentialID: cred.ID,
			}
			err = db.AddModel(ctx, &model)
			c.Assert(err, qt.IsNil)

			// user u is administrator of the test-model
			err = openfga.NewUser(u, client).SetModelAccess(ctx, model.ResourceTag(), ofganames.AdministratorRelation)
			c.Assert(err, qt.IsNil)

			offerParams := juju.AddApplicationOfferParams{
				ModelTag:               model.ResourceTag(),
				OfferName:              "test-app-offer",
				ApplicationName:        "test-app",
				ApplicationDescription: "a test app offering",
				Endpoints: map[string]string{
					"endpoint1": "url1",
				},
			}

			offer := dbmodel.ApplicationOffer{}

			return *u, offerParams, offer, func(c *qt.C, err error) {
				c.Assert(err, qt.ErrorMatches, "failed to fetch details of the created application offer: a silly error")
			}
		},
	}, {
		about: "controller returns `application offer already exists`",
		getApplicationOffer: func(ctx context.Context, s string) (*crossmodel.ApplicationOfferDetails, error) {
			return nil, nil
		},
		offer: func(context.Context, jujuclient.OfferParams) error {
			return errors.New("application offer already exists")
		},
		createEnv: func(c *qt.C, db *db.Database, client *openfga.OFGAClient) (dbmodel.Identity, juju.AddApplicationOfferParams, dbmodel.ApplicationOffer, func(*qt.C, error)) {
			ctx := context.Background()

			u, err := dbmodel.NewIdentity("alice@canonical.com")
			c.Assert(err, qt.IsNil)
			c.Assert(db.DB.Create(u).Error, qt.IsNil)

			cloud := dbmodel.Cloud{
				Name: "test-cloud",
				Type: "test-provider",
				Regions: []dbmodel.CloudRegion{{
					Name: "test-region-1",
				}},
			}
			c.Assert(db.DB.Create(&cloud).Error, qt.IsNil)

			// user u is administrator of the test-cloud
			err = openfga.NewUser(u, client).SetCloudAccess(ctx, cloud.ResourceTag(), ofganames.AdministratorRelation)
			c.Assert(err, qt.IsNil)

			controller := dbmodel.Controller{
				Name:        "test-controller-1",
				UUID:        "00000000-0000-0000-0000-000000000001",
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
				Name:              "test-credential-1",
				CloudName:         cloud.Name,
				OwnerIdentityName: u.Name,
				AuthType:          "empty",
			}
			err = db.SetCloudCredential(ctx, &cred)
			c.Assert(err, qt.IsNil)

			model := dbmodel.Model{
				Name: "test-model",
				UUID: sql.NullString{
					String: "00000000-0000-0000-0000-000000000003",
					Valid:  true,
				},
				OwnerIdentityName: u.Name,
				ControllerID:      controller.ID,
				CloudRegionID:     cloud.Regions[0].ID,
				CloudCredentialID: cred.ID,
			}
			err = db.AddModel(ctx, &model)
			c.Assert(err, qt.IsNil)

			// user u is administrator of the test-model
			err = openfga.NewUser(u, client).SetModelAccess(ctx, model.ResourceTag(), ofganames.AdministratorRelation)
			c.Assert(err, qt.IsNil)

			offerParams := juju.AddApplicationOfferParams{
				ModelTag:               model.ResourceTag(),
				OfferName:              "test-app-offer",
				ApplicationName:        "test-app",
				ApplicationDescription: "a test app offering",
				Endpoints: map[string]string{
					"endpoint1": "url1",
				},
			}

			offer := dbmodel.ApplicationOffer{}

			return *u, offerParams, offer, func(c *qt.C, err error) {
				c.Assert(err, qt.ErrorMatches, "application offer already exists")
				c.Assert(errors.ErrorCode(err), qt.Equals, errors.CodeAlreadyExists)
			}
		},
	}}

	for _, test := range tests {
		c.Run(test.about, func(c *qt.C) {
			api := &jimmtest.API{
				GetApplicationOffer_: test.getApplicationOffer,
				Offer_:               test.offer,
			}

			j := newTestJujuManager(c, &parameters{
				Dialer: &jimmtest.Dialer{
					API: api,
				},
			})

			ctx := context.Background()

			u, offerArgs, expectedOffer, errorAssertion := test.createEnv(c, j.Database, j.OpenFGAClient)

			err := j.Offer(context.Background(), openfga.NewUser(&u, j.OpenFGAClient), offerArgs)
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

	createEnv := func(c *qt.C, db *db.Database, client *openfga.OFGAClient) (dbmodel.Identity, juju.AddApplicationOfferParams, dbmodel.ApplicationOffer, func(*qt.C, error)) {
		ctx := context.Background()

		u, err := dbmodel.NewIdentity("alice@canonical.com")
		c.Assert(err, qt.IsNil)
		c.Assert(db.DB.Create(&u).Error, qt.IsNil)

		cloud := dbmodel.Cloud{
			Name: "test-cloud",
			Type: "test-provider",
			Regions: []dbmodel.CloudRegion{{
				Name: "test-region-1",
			}},
		}
		c.Assert(db.DB.Create(&cloud).Error, qt.IsNil)

		// user u is administrator of the test-cloud
		err = openfga.NewUser(u, client).SetCloudAccess(ctx, cloud.ResourceTag(), ofganames.AdministratorRelation)
		c.Assert(err, qt.IsNil)

		controller := dbmodel.Controller{
			Name:        "test-controller",
			UUID:        "00000000-0000-0000-0000-000000000001",
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
			Name:              "test-credential-1",
			CloudName:         cloud.Name,
			OwnerIdentityName: u.Name,
			AuthType:          "empty",
		}
		err = db.SetCloudCredential(ctx, &cred)
		c.Assert(err, qt.IsNil)

		model := dbmodel.Model{
			Name: "test-model",
			UUID: sql.NullString{
				String: "00000000-0000-0000-0000-000000000003",
				Valid:  true,
			},
			OwnerIdentityName: u.Name,
			ControllerID:      controller.ID,
			CloudRegionID:     cloud.Regions[0].ID,
			CloudCredentialID: cred.ID,
		}
		err = db.AddModel(ctx, &model)
		c.Assert(err, qt.IsNil)

		// user u is administrator of the test-model
		err = openfga.NewUser(u, client).SetModelAccess(ctx, model.ResourceTag(), ofganames.AdministratorRelation)
		c.Assert(err, qt.IsNil)

		offerParams := juju.AddApplicationOfferParams{
			ModelTag:               model.ResourceTag(),
			OfferName:              "test-app-offer",
			ApplicationName:        "test-app",
			ApplicationDescription: "a test app offering",
			Endpoints: map[string]string{
				"endpoint1": "url1",
			},
		}

		offer := dbmodel.ApplicationOffer{
			ID:      1,
			Name:    offerParams.OfferName,
			ModelID: model.ID,
			UUID:    "00000000-0000-0000-0000-000000000004",
			URL:     "test-offer-url",
		}

		return *u, offerParams, offer, nil
	}

	api := &jimmtest.API{
		GetApplicationOffer_: func(ctx context.Context, s string) (*crossmodel.ApplicationOfferDetails, error) {
			return &crossmodel.ApplicationOfferDetails{
				OfferUUID:              "00000000-0000-0000-0000-000000000004",
				ApplicationName:        "test-app-offer",
				CharmURL:               "cs:test-app:17",
				OfferName:              "test-app-offer",
				OfferURL:               "test-offer-url",
				ApplicationDescription: "a test app offering",
				Endpoints: []charm.Relation{{
					Name:      "test-endpoint",
					Role:      charm.RoleRequirer,
					Interface: "unknown",
					Limit:     1,
				}},
				Connections: []crossmodel.OfferConnection{{
					SourceModelUUID: "test-model-src",
					RelationId:      1,
					Username:        "unknown",
					Endpoint:        "test-endpoint",
				}},
				Users: []crossmodel.OfferUserDetails{{
					UserName:    "alice",
					DisplayName: "alice, sister of eve",
					Access:      permission.AdminAccess,
				}},
			}, nil
		},
		Offer_: func(ctx context.Context, op jujuclient.OfferParams) error {
			return nil
		},
	}

	j := newTestJujuManager(c, &parameters{
		Dialer: &jimmtest.Dialer{
			API:  api,
			UUID: "00000000-0000-0000-0000-000000000001",
		},
	})

	u, offerArgs, expectedOffer, _ := createEnv(c, j.Database, j.OpenFGAClient)

	err := j.Offer(context.Background(), openfga.NewUser(&u, j.OpenFGAClient), offerArgs)
	c.Assert(err, qt.IsNil)

	offer := dbmodel.ApplicationOffer{
		URL: expectedOffer.URL,
	}
	err = j.Database.GetApplicationOffer(ctx, &offer)
	c.Assert(err, qt.IsNil)
	c.Assert(offer, qt.CmpEquals(cmpopts.EquateEmpty(), cmpopts.IgnoreTypes(time.Time{}, gorm.Model{}), cmpopts.IgnoreTypes(dbmodel.Model{})), expectedOffer)

	// check the controller relation was created
	exists, err := j.OpenFGAClient.CheckRelation(
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
	exists, err = j.OpenFGAClient.CheckRelation(
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

func TestDestroyOffer(t *testing.T) {
	c := qt.New(t)

	ctx := context.Background()

	destroyErrorsChannel := make(chan error, 1)

	tests := []struct {
		about         string
		parameterFunc func(*environment) (dbmodel.Identity, string)
		destroyError  string
		expectedError string
	}{{
		about: "admin allowed to destroy an offer",
		parameterFunc: func(env *environment) (dbmodel.Identity, string) {
			return env.users[0], "test-offer-url"
		},
	}, {
		about: "user with consume access not allowed to destroy an offer",
		parameterFunc: func(env *environment) (dbmodel.Identity, string) {
			return env.users[2], "test-offer-url"
		},
		expectedError: "unauthorized",
	}, {
		about: "user with read access not allowed to destroy an offer",
		parameterFunc: func(env *environment) (dbmodel.Identity, string) {
			return env.users[3], "test-offer-url"
		},
		expectedError: "unauthorized",
	}, {
		about: "user without access not allowed to destroy an offer",
		parameterFunc: func(env *environment) (dbmodel.Identity, string) {
			return env.users[4], "test-offer-url"
		},
		expectedError: "unauthorized",
	}, {
		about: "offer not found",
		parameterFunc: func(env *environment) (dbmodel.Identity, string) {
			return env.users[0], "no-such-offer"
		},
		expectedError: "application offer not found",
	}, {
		about:        "controller returns an error",
		destroyError: "a silly error",
		parameterFunc: func(env *environment) (dbmodel.Identity, string) {
			return env.users[0], "test-offer-url"
		},
		expectedError: "a silly error",
	}}

	for _, test := range tests {
		c.Run(test.about, func(c *qt.C) {
			j := newTestJujuManager(c, &parameters{
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
			})

			environment := initializeEnvironment(c, ctx, j.Database, j.OpenFGAClient, j.ResourceTag())
			authenticatedUser, offerURL := test.parameterFunc(environment)

			if test.destroyError != "" {
				select {
				case destroyErrorsChannel <- errors.E(test.destroyError):
				default:
				}
			}
			err := j.DestroyOffer(ctx, openfga.NewUser(&authenticatedUser, j.OpenFGAClient), offerURL, true)
			if test.expectedError == "" {
				c.Assert(err, qt.IsNil)

				offer := dbmodel.ApplicationOffer{
					URL: offerURL,
				}
				err = j.Database.GetApplicationOffer(ctx, &offer)
				c.Assert(errors.ErrorCode(err), qt.Equals, errors.CodeNotFound)
			} else {
				c.Assert(err, qt.ErrorMatches, test.expectedError)
			}
		})
	}
}

func TestFindApplicationOffers(t *testing.T) {
	c := qt.New(t)

	ctx := context.Background()

	expectedOffer := crossmodel.ApplicationOfferDetails{
		OfferUUID: "00000012-0000-0000-0000-000000000001",
		OfferURL:  "test-offer-url",
		OfferName: "test-offer",
	}

	tests := []struct {
		about         string
		parameterFunc func(*environment) (dbmodel.Identity, string, []crossmodel.ApplicationOfferFilter)
		expectedError string
		expectedOffer *crossmodel.ApplicationOfferDetails
	}{{
		about: "find an offer as an offer consumer",
		parameterFunc: func(env *environment) (dbmodel.Identity, string, []crossmodel.ApplicationOfferFilter) {
			return env.users[2], "consume", []crossmodel.ApplicationOfferFilter{{
				OfferName: "test-offer",
			}}
		},
		expectedOffer: &expectedOffer,
	}, {
		about: "find an offer as model admin",
		parameterFunc: func(env *environment) (dbmodel.Identity, string, []crossmodel.ApplicationOfferFilter) {
			return env.users[0], "admin", []crossmodel.ApplicationOfferFilter{{
				OfferName: "test-offer",
			}}
		},
		expectedOffer: &expectedOffer,
	}, {
		about: "find an offer as offer admin",
		parameterFunc: func(env *environment) (dbmodel.Identity, string, []crossmodel.ApplicationOfferFilter) {
			return env.users[5], "admin", []crossmodel.ApplicationOfferFilter{{
				OfferName: "test-offer",
			}}
		},
		expectedOffer: &expectedOffer,
	}, {
		about: "find an offer as superuser",
		parameterFunc: func(env *environment) (dbmodel.Identity, string, []crossmodel.ApplicationOfferFilter) {
			return env.users[6], "admin", []crossmodel.ApplicationOfferFilter{{
				OfferName: "test-offer",
			}}
		},
		expectedOffer: &expectedOffer,
	}, {
		about: "offer not found",
		parameterFunc: func(env *environment) (dbmodel.Identity, string, []crossmodel.ApplicationOfferFilter) {
			return env.users[0], "admin", []crossmodel.ApplicationOfferFilter{{
				OfferName: "no-such-offer",
			}}
		},
	}, {
		about: "user without access cannot find offers",
		parameterFunc: func(env *environment) (dbmodel.Identity, string, []crossmodel.ApplicationOfferFilter) {
			return env.users[4], "", []crossmodel.ApplicationOfferFilter{{
				OfferName: "test-offer",
			}}
		},
	}}

	for _, test := range tests {
		c.Run(test.about, func(c *qt.C) {

			j := newTestJujuManager(c, &parameters{
				Dialer: &jimmtest.Dialer{
					API: &jimmtest.API{
						FindApplicationOffers_: func(ctx context.Context, aof []crossmodel.ApplicationOfferFilter) ([]*crossmodel.ApplicationOfferDetails, error) {
							if test.expectedOffer != nil {
								return []*crossmodel.ApplicationOfferDetails{test.expectedOffer}, nil
							}
							return nil, nil
						},
					},
				},
			})

			environment := initializeEnvironment(c, ctx, j.Database, j.OpenFGAClient, j.ResourceTag())
			user, accessLevel, filters := test.parameterFunc(environment)

			offers, err := j.FindApplicationOffers(ctx, openfga.NewUser(&user, j.OpenFGAClient), filters...)
			if test.expectedError == "" {
				c.Assert(err, qt.IsNil)
				if test.expectedOffer != nil {
					details := test.expectedOffer
					if accessLevel != string(jujuparams.OfferAdminAccess) {
						details.Users = []crossmodel.OfferUserDetails{{
							UserName: user.Name,
							Access:   permission.Access(accessLevel),
						}}
					} else {
						details.Users = []crossmodel.OfferUserDetails{{
							UserName: "alice@canonical.com",
							Access:   permission.AdminAccess,
						}, {
							UserName: "bob@canonical.com",
							Access:   permission.ConsumeAccess,
						}, {
							UserName: "eve@canonical.com",
							Access:   permission.AdminAccess,
						}, {
							UserName: "fred@canonical.com",
							Access:   permission.ReadAccess,
						}, {
							UserName: "jane@canonical.com",
							Access:   permission.AdminAccess,
						}, {
							// joe is jimm admin
							UserName: "joe@canonical.com",
							Access:   permission.AdminAccess,
						}}
					}

					expectedUsers := make([]crossmodel.OfferUserDetails, 0, len(details.Users))
					for _, user := range details.Users {
						expectedUsers = append(expectedUsers, crossmodel.OfferUserDetails{
							UserName: user.UserName,
							Access:   user.Access,
						})
					}
					for i := range offers {
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
						[]*crossmodel.ApplicationOfferDetails{
							{
								OfferUUID: details.OfferUUID,
								OfferURL:  details.OfferURL,
								OfferName: details.OfferName,
								Users:     expectedUsers,
							},
						},
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
- owner: alice@canonical.com
  name: cred-1
  cloud: test-cloud
controllers:
- name: controller-1
  uuid: 00000001-0000-0000-0000-000000000001
  cloud: test-cloud
  region: test-cloud-region
models:
- name: model-1
  uuid: 00000002-0000-0000-0000-000000000001
  controller: controller-1
  cloud: test-cloud
  region: test-cloud-region
  cloud-credential: cred-1
  owner: bob@canonical.com
  life: alive
  users:
  - user: alice@canonical.com
    access: admin
  - user: bob@canonical.com
    access: admin
  - user: charlie@canonical.com
    access: read
- name: model-2
  uuid: 00000002-0000-0000-0000-000000000002
  controller: controller-1
  cloud: test-cloud
  region: test-cloud-region
  cloud-credential: cred-1
  owner: alice@canonical.com
  life: alive
  users:
  - user: alice@canonical.com
    access: admin
  - user: bob@canonical.com
    access: write
  - user: charlie@canonical.com
    access: read
application-offers:
- name: offer-1
  url: test-offer-url
  uuid: 00000012-0000-0000-0000-000000000001
  model-name: model-1
  model-owner: bob@canonical.com
  application-name: application-1
  application-description: app description 1
- name: offer-2
  url: test-offer-url-2
  uuid: 00000012-0000-0000-0000-000000000002
  model-name: model-1
  model-owner: bob@canonical.com
  application-name: application-2
  application-description: app description 2
- name: offer-3
  url: test-offer-url-3
  uuid: 00000012-0000-0000-0000-000000000003
  model-name: model-2
  model-owner: alice@canonical.com
  application-name: application-3
  application-description: app description 3
`

func TestListApplicationOffers(t *testing.T) {
	c := qt.New(t)

	ctx := context.Background()

	env := jimmtest.ParseEnvironment(c, listApplicationsTestEnv)

	j := newTestJujuManager(c, &parameters{
		Dialer: &jimmtest.Dialer{
			API: &jimmtest.API{
				ListApplicationOffers_: func(ctx context.Context, filters []crossmodel.ApplicationOfferFilter) ([]*crossmodel.ApplicationOfferDetails, error) {
					offers := []*crossmodel.ApplicationOfferDetails{}
					for _, filter := range filters {
						switch filter.ModelName {
						case "model-1":
							offers = append(offers, []*crossmodel.ApplicationOfferDetails{{
								OfferUUID:              "00000012-0000-0000-0000-000000000001",
								OfferName:              "offer-1",
								OfferURL:               "test-offer-url",
								ApplicationDescription: "app description 1",
								Endpoints: []charm.Relation{{
									Name:      "test-endpoint",
									Role:      charm.RoleRequirer,
									Interface: "unknown",
									Limit:     1,
								}},
								ApplicationName: "application-1",
								CharmURL:        "charm-1",
								Connections: []crossmodel.OfferConnection{{
									SourceModelUUID: "00000011-0000-0000-0000-000000000001",
									RelationId:      1,
									Username:        "charlie@canonical.com",
									Endpoint:        "an-endpoint",
								}},
							}, {
								OfferUUID:              "00000012-0000-0000-0000-000000000002",
								OfferName:              "offer-2",
								OfferURL:               "test-offer-url-2",
								ApplicationDescription: "app description 2",
								Endpoints: []charm.Relation{{
									Name:      "test-endpoint",
									Role:      charm.RoleRequirer,
									Interface: "unknown",
									Limit:     1,
								}},
								ApplicationName: "application-2",
								CharmURL:        "charm-2",
								Connections: []crossmodel.OfferConnection{{
									SourceModelUUID: "00000011-0000-0000-0000-000000000002",
									RelationId:      2,
									Username:        "charlie@canonical.com",
									Endpoint:        "an-endpoint",
								}},
							}}...)
						case "model-2":
							offers = append(offers, []*crossmodel.ApplicationOfferDetails{{
								OfferUUID:              "00000012-0000-0000-0000-000000000003",
								OfferName:              "offer-3",
								OfferURL:               "test-offer-url-3",
								ApplicationDescription: "app description 3",
								Endpoints: []charm.Relation{{
									Name:      "test-endpoint",
									Role:      charm.RoleRequirer,
									Interface: "unknown",
									Limit:     1,
								}},
								ApplicationName: "application-3",
								CharmURL:        "charm-3",
								Connections: []crossmodel.OfferConnection{{
									SourceModelUUID: "00000011-0000-0000-0000-000000000003",
									RelationId:      3,
									Username:        "charlie@canonical.com",
									Endpoint:        "an-endpoint",
								}},
							}}...)
						}
					}
					return offers, nil
				},
			},
		},
	})

	env.PopulateDBAndPermissions(c, j.ResourceTag(), j.Database, j.OpenFGAClient)
	tuples := []openfga.Tuple{{
		Object:   ofganames.ConvertTag(names.NewUserTag("alice@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewApplicationOfferTag("00000012-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("eve@canonical.com")),
		Relation: ofganames.ReaderRelation,
		Target:   ofganames.ConvertTag(names.NewApplicationOfferTag("00000012-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("bob@canonical.com")),
		Relation: ofganames.ConsumerRelation,
		Target:   ofganames.ConvertTag(names.NewApplicationOfferTag("00000012-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("alice@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewApplicationOfferTag("00000012-0000-0000-0000-000000000002")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("eve@canonical.com")),
		Relation: ofganames.ReaderRelation,
		Target:   ofganames.ConvertTag(names.NewApplicationOfferTag("00000012-0000-0000-0000-000000000002")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("bob@canonical.com")),
		Relation: ofganames.ConsumerRelation,
		Target:   ofganames.ConvertTag(names.NewApplicationOfferTag("00000012-0000-0000-0000-000000000002")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("alice@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewApplicationOfferTag("00000012-0000-0000-0000-000000000003")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("eve@canonical.com")),
		Relation: ofganames.ReaderRelation,
		Target:   ofganames.ConvertTag(names.NewApplicationOfferTag("00000012-0000-0000-0000-000000000003")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("bob@canonical.com")),
		Relation: ofganames.ConsumerRelation,
		Target:   ofganames.ConvertTag(names.NewApplicationOfferTag("00000012-0000-0000-0000-000000000003")),
	}}
	err := j.OpenFGAClient.AddRelation(context.Background(), tuples...)
	c.Assert(err, qt.IsNil)

	u := env.User("alice@canonical.com").DBObject(c, j.Database)
	_, err = j.ListApplicationOffers(ctx, openfga.NewUser(&u, j.OpenFGAClient))
	c.Assert(err, qt.ErrorMatches, `at least one filter must be specified`)

	filters := []crossmodel.ApplicationOfferFilter{{
		OwnerName: "bob@canonical.com",
		ModelName: "model-1",
	}, {
		ModelName: "model-2",
	}}

	offers, err := j.ListApplicationOffers(ctx, openfga.NewUser(&u, j.OpenFGAClient), filters...)
	c.Assert(err, qt.IsNil)

	for i := range offers {
		sort.Slice(offers[i].Users, func(j, k int) bool {
			return offers[i].Users[j].UserName < offers[i].Users[k].UserName
		})
	}
	c.Check(offers, qt.DeepEquals, []*crossmodel.ApplicationOfferDetails{
		{
			OfferUUID:              "00000012-0000-0000-0000-000000000001",
			OfferURL:               "test-offer-url",
			OfferName:              "offer-1",
			ApplicationDescription: "app description 1",
			Endpoints: []charm.Relation{{
				Name:      "test-endpoint",
				Role:      "requirer",
				Interface: "unknown",
				Limit:     1,
			}},
			Users: []crossmodel.OfferUserDetails{{
				UserName: "alice@canonical.com",
				Access:   "admin",
			}, {
				UserName: "bob@canonical.com",
				Access:   "admin",
			}, {
				UserName: "eve@canonical.com",
				Access:   "read",
			}},
			ApplicationName: "application-1",
			CharmURL:        "charm-1",
			Connections: []crossmodel.OfferConnection{{
				SourceModelUUID: "00000011-0000-0000-0000-000000000001",
				RelationId:      1,
				Username:        "charlie@canonical.com",
				Endpoint:        "an-endpoint",
			},
			},
		}, {
			OfferUUID:              "00000012-0000-0000-0000-000000000002",
			OfferURL:               "test-offer-url-2",
			OfferName:              "offer-2",
			ApplicationDescription: "app description 2",
			Endpoints: []charm.Relation{{
				Name:      "test-endpoint",
				Role:      "requirer",
				Interface: "unknown",
				Limit:     1,
			}},
			Users: []crossmodel.OfferUserDetails{{
				UserName: "alice@canonical.com",
				Access:   "admin",
			}, {
				UserName: "bob@canonical.com",
				Access:   "admin",
			}, {
				UserName: "eve@canonical.com",
				Access:   "read",
			}},
			ApplicationName: "application-2",
			CharmURL:        "charm-2",
			Connections: []crossmodel.OfferConnection{{
				SourceModelUUID: "00000011-0000-0000-0000-000000000002",
				RelationId:      2,
				Username:        "charlie@canonical.com",
				Endpoint:        "an-endpoint",
			},
			},
		}, {
			OfferUUID:              "00000012-0000-0000-0000-000000000003",
			OfferURL:               "test-offer-url-3",
			OfferName:              "offer-3",
			ApplicationDescription: "app description 3",
			Endpoints: []charm.Relation{{
				Name:      "test-endpoint",
				Role:      "requirer",
				Interface: "unknown",
				Limit:     1,
			}},
			Users: []crossmodel.OfferUserDetails{{
				UserName: "alice@canonical.com",
				Access:   "admin",
			}, {
				UserName: "bob@canonical.com",
				Access:   "consume",
			}, {
				UserName: "eve@canonical.com",
				Access:   "read",
			}},
			ApplicationName: "application-3",
			CharmURL:        "charm-3",
			Connections: []crossmodel.OfferConnection{{
				SourceModelUUID: "00000011-0000-0000-0000-000000000003",
				RelationId:      3,
				Username:        "charlie@canonical.com",
				Endpoint:        "an-endpoint",
			}},
		},
	})
}

const offerNotFoundTestEnv = `clouds:
- name: test-cloud
  type: test-provider
  regions:
  - name: test-cloud-region
cloud-credentials:
- owner: alice@canonical.com
  name: cred-1
  cloud: test-cloud
controllers:
- name: controller-1
  uuid: 00000001-0000-0000-0000-000000000001
  cloud: test-cloud
  region: test-cloud-region
- name: controller-2
  uuid: 00000001-0000-0000-0000-000000000002
  cloud: test-cloud
  region: test-cloud-region
models:
- name: model-1
  uuid: 00000002-0000-0000-0000-000000000001
  controller: controller-2
  cloud: test-cloud
  region: test-cloud-region
  cloud-credential: cred-1
  owner: bob@canonical.com
  life: alive
  users:
  - user: bob@canonical.com
    access: admin
application-offers:
- name: offer-1
  url: test-offer-url
  uuid: 00000012-0000-0000-0000-000000000001
  model-name: model-1
  model-owner: bob@canonical.com
  application-name: application-1
  application-description: app description 1
`

func TestFindApplicationOffers_MultipleControllers(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	// Define expected offer from the "good" controller
	// Details copy the offer defined in the test environment
	expectedOffer := crossmodel.ApplicationOfferDetails{
		OfferUUID: "00000012-0000-0000-0000-000000000001",
		OfferURL:  "test-offer-url",
		OfferName: "offer-1",
	}

	controller1Dialed := false
	// Setup dialers for two controllers
	dialers := jimmtest.DialerMap{
		"controller-1": &jimmtest.Dialer{
			API: &jimmtest.API{
				FindApplicationOffers_: func(ctx context.Context, filters []crossmodel.ApplicationOfferFilter) ([]*crossmodel.ApplicationOfferDetails, error) {
					controller1Dialed = true
					return nil, errors.E(errors.CodeNotFound, "offer not found")
				},
			},
		},
		"controller-2": &jimmtest.Dialer{
			API: &jimmtest.API{
				FindApplicationOffers_: func(ctx context.Context, filters []crossmodel.ApplicationOfferFilter) ([]*crossmodel.ApplicationOfferDetails, error) {
					return []*crossmodel.ApplicationOfferDetails{&expectedOffer}, nil
				},
			},
		},
	}

	j := newTestJujuManager(c, &parameters{
		Dialer: dialers,
	})

	// Initialize environment with one controller
	env := jimmtest.ParseEnvironment(c, offerNotFoundTestEnv)
	env.PopulateDBAndPermissions(c, j.ResourceTag(), j.Database, j.OpenFGAClient)

	user, err := dbmodel.NewIdentity("bob@canonical.com")
	c.Assert(err, qt.IsNil)

	filters := []crossmodel.ApplicationOfferFilter{{
		OfferName: "test-offer",
		ModelName: "model-1",
		OwnerName: "bob@canonical.com",
	}}

	offers, err := j.FindApplicationOffers(ctx, openfga.NewUser(user, j.OpenFGAClient), filters...)
	c.Assert(err, qt.IsNil)
	c.Assert(offers, qt.HasLen, 1)
	c.Check(offers[0].OfferURL, qt.Equals, expectedOffer.OfferURL)
	c.Check(offers[0].OfferUUID, qt.Equals, expectedOffer.OfferUUID)
	c.Check(controller1Dialed, qt.IsTrue)
}
