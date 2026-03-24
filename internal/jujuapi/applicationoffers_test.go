// Copyright 2026 Canonical.

package jujuapi_test

import (
	"context"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/juju/charm/v12"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/relation"
	jujustatus "github.com/juju/juju/core/status"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v5"

	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/jimm/juju"
	"github.com/canonical/jimm/v3/internal/jujuapi"
	"github.com/canonical/jimm/v3/internal/openfga"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest/mocks"
)

func TestOffer(t *testing.T) {
	c := qt.New(t)

	modelTag := names.NewModelTag("00000001-0000-0000-0000-000000000001").String()
	ownerTag := names.NewUserTag("owner@canonical.com").String()
	called := 0

	jujuManager := mocks.JujuManager{
		Offer_: func(ctx context.Context, user *openfga.User, offer juju.AddApplicationOfferParams) error {
			called++
			c.Check(offer.ModelTag.String(), qt.Equals, modelTag)
			c.Check(offer.OwnerTag.String(), qt.Equals, ownerTag)
			c.Check(offer.OfferName, qt.Equals, "test-offer")
			c.Check(offer.ApplicationName, qt.Equals, "test-app")
			c.Check(offer.ApplicationDescription, qt.Equals, "test description")
			c.Check(offer.Endpoints, qt.DeepEquals, map[string]string{"source": "source"})
			return nil
		},
	}

	root := newTestControllerRoot(&jimmtest.JIMM{
		JujuManager_: func() jujuapi.JujuManager {
			return &jujuManager
		},
	}, "alice@canonical.com", false)

	result, err := root.Offer(context.Background(), jujuparams.AddApplicationOffers{
		Offers: []jujuparams.AddApplicationOffer{{
			ModelTag:               modelTag,
			OwnerTag:               ownerTag,
			OfferName:              "test-offer",
			ApplicationName:        "test-app",
			ApplicationDescription: "test description",
			Endpoints:              map[string]string{"source": "source"},
		}, {
			ModelTag: "invalid-model-tag",
		}},
	})

	c.Assert(err, qt.IsNil)
	c.Assert(result.Results, qt.HasLen, 2)
	c.Assert(result.Results[0].Error, qt.Equals, (*jujuparams.Error)(nil))
	c.Assert(result.Results[1].Error, qt.Not(qt.IsNil))
	c.Assert(called, qt.Equals, 1)
}

func TestGetConsumeDetails(t *testing.T) {
	c := qt.New(t)

	called := 0
	jujuManager := mocks.JujuManager{
		GetApplicationOfferConsumeDetails_: func(ctx context.Context, user *openfga.User, details *jujuparams.ConsumeOfferDetails, v bakery.Version) error {
			called++
			c.Check(user.Name, qt.Equals, "alice@canonical.com")
			c.Check(v, qt.Equals, bakery.Version3)
			c.Check(details.Offer, qt.Not(qt.IsNil))
			c.Check(details.Offer.OfferURL, qt.Equals, "alice@canonical.com/test-model.test-offer")
			return nil
		},
	}

	root := newTestControllerRoot(&jimmtest.JIMM{
		JujuManager_: func() jujuapi.JujuManager {
			return &jujuManager
		},
	}, "alice@canonical.com", false)

	result, err := root.GetConsumeDetails(context.Background(), jujuparams.ConsumeOfferDetailsArg{
		OfferURLs: jujuparams.OfferURLs{
			OfferURLs:     []string{"test-model.test-offer", "not a valid offer url"},
			BakeryVersion: bakery.Version3,
		},
	})

	c.Assert(err, qt.IsNil)
	c.Assert(result.Results, qt.HasLen, 2)
	c.Assert(result.Results[0].Error, qt.Equals, (*jujuparams.Error)(nil))
	c.Assert(result.Results[0].Offer.OfferURL, qt.Equals, "alice@canonical.com/test-model.test-offer")
	c.Assert(result.Results[1].Error, qt.Not(qt.IsNil))
	c.Assert(called, qt.Equals, 1)
}

func TestListAndFindApplicationOffers(t *testing.T) {
	c := qt.New(t)

	since := time.Now().UTC().Truncate(time.Second)
	var listFilters []crossmodel.ApplicationOfferFilter
	var findFilters []crossmodel.ApplicationOfferFilter

	expectedOffer := &crossmodel.ApplicationOfferDetails{
		OfferName:              "test-offer",
		ApplicationName:        "test-app",
		ApplicationDescription: "test description",
		OfferURL:               "owner@canonical.com/test-model.test-offer",
		CharmURL:               "ch:test-app",
		Endpoints: []charm.Relation{{
			Name:      "source",
			Role:      charm.RoleProvider,
			Interface: "dummy-token",
			Limit:     1,
		}},
		Users: []crossmodel.OfferUserDetails{{
			UserName:    "alice@canonical.com",
			DisplayName: "Alice",
			Access:      permission.AdminAccess,
		}},
		Connections: []crossmodel.OfferConnection{{
			SourceModelUUID: "00000002-0000-0000-0000-000000000002",
			RelationId:      11,
			Username:        "consumer@canonical.com",
			Endpoint:        "source",
			Status:          relation.Joined,
			Message:         "connected",
			Since:           &since,
			IngressSubnets:  []string{"10.0.0.0/24"},
		}},
	}

	jujuManager := mocks.JujuManager{
		ListApplicationOffers_: func(ctx context.Context, user *openfga.User, filters ...crossmodel.ApplicationOfferFilter) ([]*crossmodel.ApplicationOfferDetails, error) {
			listFilters = filters
			return []*crossmodel.ApplicationOfferDetails{expectedOffer}, nil
		},
		FindApplicationOffers_: func(ctx context.Context, user *openfga.User, filters ...crossmodel.ApplicationOfferFilter) ([]*crossmodel.ApplicationOfferDetails, error) {
			findFilters = filters
			return []*crossmodel.ApplicationOfferDetails{expectedOffer}, nil
		},
	}

	root := newTestControllerRoot(&jimmtest.JIMM{
		JujuManager_: func() jujuapi.JujuManager {
			return &jujuManager
		},
	}, "alice@canonical.com", false)

	args := jujuparams.OfferFilters{
		Filters: []jujuparams.OfferFilter{{
			OwnerName:              "owner@canonical.com",
			ModelName:              "test-model",
			OfferName:              "test-offer",
			ApplicationName:        "test-app",
			ApplicationDescription: "test description",
			Endpoints: []jujuparams.EndpointFilterAttributes{{
				Name:      "source",
				Interface: "dummy-token",
				Role:      charm.RoleProvider,
			}},
			ConnectedUserTags:   []string{names.NewUserTag("consumer@canonical.com").String(), "raw-user"},
			AllowedConsumerTags: []string{names.NewUserTag("reader@canonical.com").String(), "raw-consumer"},
		}},
	}

	listResult, err := root.ListApplicationOffers(context.Background(), args)
	c.Assert(err, qt.IsNil)
	c.Assert(listResult.Results, qt.HasLen, 1)
	c.Assert(listFilters, qt.HasLen, 1)
	c.Assert(listFilters[0].ConnectedUsers, qt.DeepEquals, []string{"consumer@canonical.com", "raw-user"})
	c.Assert(listFilters[0].AllowedConsumers, qt.DeepEquals, []string{"reader@canonical.com", "raw-consumer"})
	c.Assert(listResult.Results[0].OfferName, qt.Equals, "test-offer")
	c.Assert(listResult.Results[0].ApplicationName, qt.Equals, "test-app")
	c.Assert(listResult.Results[0].Users, qt.HasLen, 1)
	c.Assert(listResult.Results[0].Users[0].DisplayName, qt.Equals, "Alice")
	c.Assert(listResult.Results[0].Users[0].Access, qt.Equals, "admin")
	c.Assert(listResult.Results[0].Connections, qt.HasLen, 1)
	c.Assert(listResult.Results[0].Connections[0].SourceModelTag, qt.Equals, names.NewModelTag("00000002-0000-0000-0000-000000000002").String())
	c.Assert(listResult.Results[0].Connections[0].RelationId, qt.Equals, 11)
	c.Assert(listResult.Results[0].Connections[0].Status.Status, qt.Equals, jujustatus.Status(relation.Joined))
	c.Assert(listResult.Results[0].Connections[0].Status.Info, qt.Equals, "connected")
	c.Assert(listResult.Results[0].Connections[0].Status.Since, qt.DeepEquals, &since)

	findResult, err := root.FindApplicationOffers(context.Background(), args)
	c.Assert(err, qt.IsNil)
	c.Assert(findResult.Results, qt.HasLen, 1)
	c.Assert(findFilters, qt.HasLen, 1)
	c.Assert(findFilters[0].OfferName, qt.Equals, "test-offer")
}

func TestModifyOfferAccess(t *testing.T) {
	c := qt.New(t)

	grantCalled := false
	revokeCalled := false
	permissionManager := mocks.PermissionManager{
		GrantOfferAccess_: func(ctx context.Context, user *openfga.User, offerURL string, ut names.UserTag, access jujuparams.OfferAccessPermission) error {
			grantCalled = true
			c.Check(offerURL, qt.Equals, "owner@canonical.com/test-model.test-offer")
			c.Check(ut.Id(), qt.Equals, "consumer@canonical.com")
			c.Check(access, qt.Equals, jujuparams.OfferReadAccess)
			return nil
		},
		RevokeOfferAccess_: func(ctx context.Context, user *openfga.User, offerURL string, ut names.UserTag, access jujuparams.OfferAccessPermission) error {
			revokeCalled = true
			c.Check(offerURL, qt.Equals, "owner@canonical.com/test-model.test-offer")
			c.Check(ut.Id(), qt.Equals, "consumer@canonical.com")
			c.Check(access, qt.Equals, jujuparams.OfferReadAccess)
			return nil
		},
	}

	root := newTestControllerRoot(&jimmtest.JIMM{
		PermissionManager_: func() jujuapi.PermissionManager {
			return &permissionManager
		},
	}, "alice@canonical.com", false)

	result, err := root.ModifyOfferAccess(context.Background(), jujuparams.ModifyOfferAccessRequest{
		Changes: []jujuparams.ModifyOfferAccess{{
			Action:   jujuparams.GrantOfferAccess,
			OfferURL: "owner@canonical.com/test-model.test-offer",
			UserTag:  names.NewUserTag("consumer@canonical.com").String(),
			Access:   jujuparams.OfferReadAccess,
		}, {
			Action:   jujuparams.RevokeOfferAccess,
			OfferURL: "owner@canonical.com/test-model.test-offer",
			UserTag:  names.NewUserTag("consumer@canonical.com").String(),
			Access:   jujuparams.OfferReadAccess,
		}, {
			Action:   "unknown",
			OfferURL: "owner@canonical.com/test-model.test-offer",
			UserTag:  names.NewUserTag("consumer@canonical.com").String(),
			Access:   jujuparams.OfferReadAccess,
		}, {
			Action:   jujuparams.GrantOfferAccess,
			OfferURL: "owner@canonical.com/test-model.test-offer",
			UserTag:  "user-local",
			Access:   jujuparams.OfferReadAccess,
		}},
	})

	c.Assert(err, qt.IsNil)
	c.Assert(result.Results, qt.HasLen, 4)
	c.Assert(result.Results[0].Error, qt.Equals, (*jujuparams.Error)(nil))
	c.Assert(result.Results[1].Error, qt.Equals, (*jujuparams.Error)(nil))
	c.Assert(result.Results[2].Error, qt.Not(qt.IsNil))
	c.Assert(result.Results[3].Error, qt.Not(qt.IsNil))
	c.Assert(grantCalled, qt.IsTrue)
	c.Assert(revokeCalled, qt.IsTrue)
}

func TestDestroyOffers(t *testing.T) {
	c := qt.New(t)

	called := 0
	jujuManager := mocks.JujuManager{
		DestroyOffer_: func(ctx context.Context, user *openfga.User, offerURL string, force bool) error {
			called++
			if offerURL == "owner@canonical.com/test-model.fail" {
				return errors.New("cannot destroy offer")
			}
			c.Check(force, qt.IsTrue)
			return nil
		},
	}

	root := newTestControllerRoot(&jimmtest.JIMM{
		JujuManager_: func() jujuapi.JujuManager {
			return &jujuManager
		},
	}, "alice@canonical.com", false)

	result, err := root.DestroyOffers(context.Background(), jujuparams.DestroyApplicationOffers{
		OfferURLs: []string{"owner@canonical.com/test-model.ok", "owner@canonical.com/test-model.fail"},
		Force:     true,
	})

	c.Assert(err, qt.IsNil)
	c.Assert(result.Results, qt.HasLen, 2)
	c.Assert(result.Results[0].Error, qt.Equals, (*jujuparams.Error)(nil))
	c.Assert(result.Results[1].Error, qt.Not(qt.IsNil))
	c.Assert(called, qt.Equals, 2)
}

func TestApplicationOffers(t *testing.T) {
	c := qt.New(t)

	jujuManager := mocks.JujuManager{
		GetApplicationOffer_: func(ctx context.Context, user *openfga.User, offerURL string) (*crossmodel.ApplicationOfferDetails, error) {
			if offerURL == "owner@canonical.com/test-model.missing" {
				return nil, errors.E(errors.CodeNotFound, "application offer not found")
			}
			return &crossmodel.ApplicationOfferDetails{
				OfferName:              "test-offer",
				ApplicationName:        "test-app",
				ApplicationDescription: "test description",
				OfferURL:               offerURL,
				Endpoints: []charm.Relation{{
					Name:      "source",
					Role:      charm.RoleProvider,
					Interface: "dummy-token",
				}},
				Users: []crossmodel.OfferUserDetails{{
					UserName: "alice@canonical.com",
					Access:   permission.AdminAccess,
				}},
			}, nil
		},
	}

	root := newTestControllerRoot(&jimmtest.JIMM{
		JujuManager_: func() jujuapi.JujuManager {
			return &jujuManager
		},
	}, "alice@canonical.com", false)

	result, err := root.ApplicationOffers(context.Background(), jujuparams.OfferURLs{
		OfferURLs: []string{"owner@canonical.com/test-model.test-offer", "owner@canonical.com/test-model.missing"},
	})

	c.Assert(err, qt.IsNil)
	c.Assert(result.Results, qt.HasLen, 2)
	c.Assert(result.Results[0].Error, qt.Equals, (*jujuparams.Error)(nil))
	c.Assert(result.Results[0].Result.OfferURL, qt.Equals, "owner@canonical.com/test-model.test-offer")
	c.Assert(result.Results[0].Result.Users, qt.HasLen, 1)
	c.Assert(result.Results[0].Result.Users[0].Access, qt.Equals, "admin")
	c.Assert(result.Results[1].Result, qt.IsNil)
	c.Assert(result.Results[1].Error, qt.Not(qt.IsNil))
}
