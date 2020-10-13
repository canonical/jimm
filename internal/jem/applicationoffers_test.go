// Copyright 2020 Canonical Ltd.

package jem_test

import (
	"context"

	"github.com/juju/charm/v8"
	jujuparams "github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing/factory"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"
	"gopkg.in/errgo.v1"
	"gopkg.in/macaroon-bakery.v2/bakery"
	"gopkg.in/macaroon-bakery.v2/bakery/identchecker"
	"gopkg.in/macaroon-bakery.v2/httpbakery"

	"github.com/CanonicalLtd/jimm/internal/conv"
	"github.com/CanonicalLtd/jimm/internal/jem"
	"github.com/CanonicalLtd/jimm/internal/jemtest"
	"github.com/CanonicalLtd/jimm/internal/mgosession"
	"github.com/CanonicalLtd/jimm/internal/mongodoc"
	"github.com/CanonicalLtd/jimm/internal/pubsub"
	"github.com/CanonicalLtd/jimm/params"
)

type applicationoffersSuite struct {
	jemtest.JujuConnSuite
	pool                           *jem.Pool
	sessionPool                    *mgosession.Pool
	jem                            *jem.JEM
	usageSenderAuthorizationClient *testUsageSenderAuthorizationClient
	model                          *mongodoc.Model
	endpoint                       state.Endpoint
	identity                       identchecker.ACLIdentity
	caCert                         string
	addrs                          []string

	suiteCleanups []func()
}

var _ = gc.Suite(&applicationoffersSuite{})

func (s *applicationoffersSuite) SetUpTest(c *gc.C) {
	ctx := context.Background()

	s.JujuConnSuite.SetUpTest(c)
	s.sessionPool = mgosession.NewPool(ctx, s.Session, 5)
	publicCloudMetadata, _, err := cloud.PublicCloudMetadata()
	c.Assert(err, gc.Equals, nil)
	s.usageSenderAuthorizationClient = &testUsageSenderAuthorizationClient{}
	s.PatchValue(&jem.NewUsageSenderAuthorizationClient, func(_ string, _ *httpbakery.Client) (jem.UsageSenderAuthorizationClient, error) {
		return s.usageSenderAuthorizationClient, nil
	})
	pool, err := jem.NewPool(ctx, jem.Params{
		DB:                  s.Session.DB("jem"),
		ControllerAdmin:     "controller-admin",
		SessionPool:         s.sessionPool,
		PublicCloudMetadata: publicCloudMetadata,
		UsageSenderURL:      "test-usage-sender-url",
		Pubsub: &pubsub.Hub{
			MaxConcurrency: 10,
		},
	})
	c.Assert(err, gc.Equals, nil)
	s.pool = pool
	s.jem = s.pool.JEM(ctx)
	s.PatchValue(&utils.OutgoingAccessAllowed, true)

	info := s.APIInfo(c)
	s.addrs = info.Addrs
	hps, err := mongodoc.ParseAddresses(info.Addrs)
	c.Assert(err, gc.Equals, nil)
	s.caCert, _ = s.ControllerConfig.CACert()
	err = s.jem.DB.AddController(ctx, &mongodoc.Controller{
		Path: params.EntityPath{User: "user1", Name: "controller-1"},
		ACL: params.ACL{
			Read: []string{"everyone"},
		},
		CACert:        s.caCert,
		HostPorts:     [][]mongodoc.HostPort{hps},
		AdminUser:     info.Tag.Id(),
		AdminPassword: info.Password,
		UUID:          s.ControllerConfig.ControllerUUID(),
		Public:        true,
	})
	c.Assert(err, gc.Equals, nil)

	_, err = s.jem.UpdateCredential(ctx, &mongodoc.Credential{
		Path: mongodoc.CredentialPath{
			EntityPath: mongodoc.EntityPath{
				User: "user1",
				Name: "cred1",
			},
			Cloud: "dummy",
		},
		Type: "empty",
	}, 0)
	c.Assert(err, gc.Equals, nil)

	s.identity = jemtest.NewIdentity("user1")
	err = s.jem.CreateModel(ctx, s.identity, jem.CreateModelParams{
		Path:           params.EntityPath{User: "user1", Name: "model-1"},
		ControllerPath: params.EntityPath{User: "user1", Name: "controller-1"},
		Credential: params.CredentialPath{
			Cloud: "dummy",
			User:  "user1",
			Name:  "cred1",
		},
		Cloud:  "dummy",
		Region: "dummy-region",
	}, nil)
	c.Assert(err, gc.Equals, nil)
	s.model = &mongodoc.Model{
		Path: params.EntityPath{User: "user1", Name: "model-1"},
	}
	err = s.jem.DB.GetModel(ctx, s.model)
	c.Assert(err, gc.Equals, nil)

	modelState, err := s.StatePool.Get(s.model.UUID)
	c.Assert(err, gc.Equals, nil)
	defer modelState.Release()

	f := factory.NewFactory(modelState.State, s.StatePool)
	app := f.MakeApplication(c, &factory.ApplicationParams{
		Name: "test-app",
		Charm: f.MakeCharm(c, &factory.CharmParams{
			Name: "wordpress",
		}),
	})
	f.MakeUnit(c, &factory.UnitParams{
		Application: app,
	})
	s.endpoint, err = app.Endpoint("url")
	c.Assert(err, gc.Equals, nil)
}

func (s *applicationoffersSuite) TearDownTest(c *gc.C) {
	s.jem.Close()
	s.pool.Close()
	s.sessionPool.Close()
	s.JujuConnSuite.TearDownTest(c)
}

func (s *applicationoffersSuite) TestGetApplicationOfferConsumeDetails(c *gc.C) {
	ctx := context.Background()

	err := s.jem.Offer(ctx, s.identity, jujuparams.AddApplicationOffer{
		ModelTag:        names.NewModelTag(s.model.UUID).String(),
		OfferName:       "test-offer",
		ApplicationName: "test-app",
		Endpoints: map[string]string{
			s.endpoint.Relation.Name: s.endpoint.Relation.Name,
		},
	})
	c.Assert(err, gc.Equals, nil)

	offerURL := conv.ToOfferURL(s.model.Path, "test-offer")

	d := jujuparams.ConsumeOfferDetails{
		Offer: &jujuparams.ApplicationOfferDetails{
			OfferURL: offerURL,
		},
	}
	err = s.jem.GetApplicationOfferConsumeDetails(ctx, s.identity, "", &d, bakery.Version2)
	c.Assert(err, gc.Equals, nil)

	c.Check(d.Macaroon, gc.Not(gc.IsNil))
	d.Macaroon = nil
	c.Check(d.Offer.OfferUUID, gc.Matches, `[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)
	d.Offer.OfferUUID = ""
	c.Check(d, jc.DeepEquals, jujuparams.ConsumeOfferDetails{
		Offer: &jujuparams.ApplicationOfferDetails{
			SourceModelTag:         names.NewModelTag(s.model.UUID).String(),
			OfferURL:               offerURL,
			OfferName:              "test-offer",
			ApplicationDescription: "A pretty popular blog engine",
			Endpoints: []jujuparams.RemoteEndpoint{{
				Name:      "url",
				Role:      charm.RoleProvider,
				Interface: "http",
			}},
			Users: []jujuparams.OfferUserDetails{{
				UserName: "everyone@external",
				Access:   "read",
			}, {
				UserName: "user1@external",
				Access:   "admin",
			}},
		},
		ControllerInfo: &jujuparams.ExternalControllerInfo{
			ControllerTag: names.NewControllerTag(s.ControllerConfig.ControllerUUID()).String(),
			Alias:         "controller-1",
			Addrs:         s.addrs,
			CACert:        s.caCert,
		},
	})

	err = s.jem.GrantOfferAccess(ctx, s.identity, s.jem.ControllerAdmin(), offerURL, jujuparams.OfferConsumeAccess)
	c.Assert(err, jc.ErrorIsNil)

	err = s.jem.GetApplicationOfferConsumeDetails(ctx, jemtest.NewIdentity(string(s.jem.ControllerAdmin())), params.User("user1"), &d, bakery.Version2)
	c.Assert(err, gc.Equals, nil)

	c.Check(d.Macaroon, gc.Not(gc.IsNil))
	d.Macaroon = nil
	c.Check(d.Offer.OfferUUID, gc.Matches, `[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)
	d.Offer.OfferUUID = ""
	c.Check(d, jc.DeepEquals, jujuparams.ConsumeOfferDetails{
		Offer: &jujuparams.ApplicationOfferDetails{
			SourceModelTag:         names.NewModelTag(s.model.UUID).String(),
			OfferURL:               offerURL,
			OfferName:              "test-offer",
			ApplicationDescription: "A pretty popular blog engine",
			Endpoints: []jujuparams.RemoteEndpoint{{
				Name:      "url",
				Role:      charm.RoleProvider,
				Interface: "http",
			}},
			Users: []jujuparams.OfferUserDetails{{
				UserName:    "controller-admin@external",
				DisplayName: "",
				Access:      "consume",
			}},
		},
		ControllerInfo: &jujuparams.ExternalControllerInfo{
			ControllerTag: names.NewControllerTag(s.ControllerConfig.ControllerUUID()).String(),
			Alias:         "controller-1",
			Addrs:         s.addrs,
			CACert:        s.caCert,
		},
	})
}

func (s *applicationoffersSuite) TestListApplicationOffers(c *gc.C) {
	ctx := context.Background()

	err := s.jem.Offer(ctx, s.identity, jujuparams.AddApplicationOffer{
		ModelTag:        names.NewModelTag(s.model.UUID).String(),
		OfferName:       "test-offer1",
		ApplicationName: "test-app",
		Endpoints: map[string]string{
			s.endpoint.Relation.Name: s.endpoint.Relation.Name,
		},
	})
	c.Assert(err, gc.Equals, nil)

	err = s.jem.Offer(ctx, s.identity, jujuparams.AddApplicationOffer{
		ModelTag:        names.NewModelTag(s.model.UUID).String(),
		OfferName:       "test-offer2",
		ApplicationName: "test-app",
		Endpoints: map[string]string{
			s.endpoint.Relation.Name: s.endpoint.Relation.Name,
		},
	})
	c.Assert(err, gc.Equals, nil)

	offer1 := mongodoc.ApplicationOffer{
		OfferURL: conv.ToOfferURL(s.model.Path, "test-offer1"),
	}
	offer2 := mongodoc.ApplicationOffer{
		OfferURL: conv.ToOfferURL(s.model.Path, "test-offer2"),
	}
	err = s.jem.DB.GetApplicationOffer(ctx, &offer1)
	c.Assert(err, jc.ErrorIsNil)
	err = s.jem.DB.GetApplicationOffer(ctx, &offer2)
	c.Assert(err, jc.ErrorIsNil)

	err = s.jem.DB.SetApplicationOfferAccess(ctx, params.User("user2"), offer1.OfferUUID, mongodoc.ApplicationOfferReadAccess)
	c.Assert(err, jc.ErrorIsNil)

	results, err := s.jem.ListApplicationOffers(ctx, jemtest.NewIdentity("unknown-user"), jujuparams.OfferFilter{
		ModelName: s.model.UUID,
	})
	c.Assert(err, gc.Equals, nil)
	c.Assert(results, gc.HasLen, 0)

	results, err = s.jem.ListApplicationOffers(ctx, jemtest.NewIdentity("user2"), jujuparams.OfferFilter{
		ModelName: s.model.UUID,
	})
	c.Assert(err, gc.Equals, nil)
	c.Assert(results, gc.HasLen, 0)

	results, err = s.jem.ListApplicationOffers(ctx, s.identity, jujuparams.OfferFilter{
		ModelName: string(s.model.Path.Name),
	})
	c.Assert(err, gc.Equals, nil)
	c.Assert(results, gc.DeepEquals, []jujuparams.ApplicationOfferAdminDetails{{
		ApplicationOfferDetails: jujuparams.ApplicationOfferDetails{
			SourceModelTag:         names.NewModelTag(s.model.UUID).String(),
			OfferUUID:              offer1.OfferUUID,
			OfferURL:               offer1.OfferURL,
			OfferName:              offer1.OfferName,
			ApplicationDescription: offer1.ApplicationDescription,
			Endpoints: []jujuparams.RemoteEndpoint{{
				Name:      "url",
				Role:      charm.RoleProvider,
				Interface: "http",
				Limit:     0,
			}},
			Spaces:   []jujuparams.RemoteSpace{},
			Bindings: offer1.Bindings,
			Users: []jujuparams.OfferUserDetails{{
				UserName:    "everyone@external",
				DisplayName: "everyone",
				Access:      "read",
			}, {
				UserName:    "user1@external",
				DisplayName: "user1",
				Access:      "admin",
			}, {
				UserName:    "user2@external",
				DisplayName: "user2",
				Access:      "read",
			}},
		},
		ApplicationName: offer1.ApplicationName,
		CharmURL:        offer1.CharmURL,
		Connections:     []jujuparams.OfferConnection{},
	}, {
		ApplicationOfferDetails: jujuparams.ApplicationOfferDetails{
			SourceModelTag:         names.NewModelTag(s.model.UUID).String(),
			OfferUUID:              offer2.OfferUUID,
			OfferURL:               offer2.OfferURL,
			OfferName:              offer2.OfferName,
			ApplicationDescription: offer2.ApplicationDescription,
			Endpoints: []jujuparams.RemoteEndpoint{{
				Name:      "url",
				Role:      charm.RoleProvider,
				Interface: "http",
				Limit:     0,
			}},
			Spaces:   []jujuparams.RemoteSpace{},
			Bindings: offer2.Bindings,
			Users: []jujuparams.OfferUserDetails{{
				UserName:    "everyone@external",
				DisplayName: "everyone",
				Access:      "read",
			}, {
				UserName:    "user1@external",
				DisplayName: "user1",
				Access:      "admin",
			}},
		},
		ApplicationName: offer2.ApplicationName,
		CharmURL:        offer2.CharmURL,
		Connections:     []jujuparams.OfferConnection{},
	},
	})

}

func (s *applicationoffersSuite) TestModifyOfferAccess(c *gc.C) {
	ctx := context.Background()

	err := s.jem.Offer(ctx, s.identity, jujuparams.AddApplicationOffer{
		ModelTag:        names.NewModelTag(s.model.UUID).String(),
		OfferName:       "test-offer1",
		ApplicationName: "test-app",
		Endpoints: map[string]string{
			s.endpoint.Relation.Name: s.endpoint.Relation.Name,
		},
	})
	c.Assert(err, gc.Equals, nil)

	offer1 := mongodoc.ApplicationOffer{
		OfferURL: conv.ToOfferURL(s.model.Path, "test-offer1"),
	}
	err = s.jem.DB.GetApplicationOffer(ctx, &offer1)
	c.Assert(err, jc.ErrorIsNil)

	err = s.jem.DB.SetApplicationOfferAccess(ctx, "user2", offer1.OfferUUID, mongodoc.ApplicationOfferNoAccess)
	c.Assert(err, jc.ErrorIsNil)
	err = s.jem.DB.SetApplicationOfferAccess(ctx, identchecker.Everyone, offer1.OfferUUID, mongodoc.ApplicationOfferNoAccess)
	c.Assert(err, jc.ErrorIsNil)

	// user2 does not have permission
	err = s.jem.GrantOfferAccess(ctx, jemtest.NewIdentity("user2"), params.User("test-user"), offer1.OfferURL, jujuparams.OfferReadAccess)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)

	err = s.jem.DB.SetApplicationOfferAccess(ctx, "user2", offer1.OfferUUID, mongodoc.ApplicationOfferConsumeAccess)
	c.Assert(err, jc.ErrorIsNil)

	// user2 has consume permission
	err = s.jem.GrantOfferAccess(ctx, jemtest.NewIdentity("user2"), params.User("test-user"), offer1.OfferURL, jujuparams.OfferReadAccess)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrUnauthorized)

	// try granting unknow access level
	err = s.jem.GrantOfferAccess(ctx, jemtest.NewIdentity("user1"), params.User("test-user"), offer1.OfferURL, jujuparams.OfferAccessPermission("unknown"))
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrBadRequest)

	// try granting permission on an offer that does not exist
	err = s.jem.GrantOfferAccess(ctx, jemtest.NewIdentity("user1"), params.User("test-user"), "no such offer", jujuparams.OfferReadAccess)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)

	// user1 is an admin - this should pass
	err = s.jem.GrantOfferAccess(ctx, jemtest.NewIdentity("user1"), params.User("test-user"), offer1.OfferURL, jujuparams.OfferAdminAccess)
	c.Assert(err, jc.ErrorIsNil)

	access, err := s.jem.DB.GetApplicationOfferAccess(ctx, params.User("test-user"), offer1.OfferUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(access, gc.Equals, mongodoc.ApplicationOfferAdminAccess)

	// user1 is an admin - this should pass and access level be set to "read"
	err = s.jem.RevokeOfferAccess(ctx, jemtest.NewIdentity("user1"), params.User("test-user"), offer1.OfferURL, jujuparams.OfferConsumeAccess)
	c.Assert(err, jc.ErrorIsNil)

	access, err = s.jem.DB.GetApplicationOfferAccess(ctx, params.User("test-user"), offer1.OfferUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(access, gc.Equals, mongodoc.ApplicationOfferReadAccess)

	// user2 is has consume access - unauthorized
	err = s.jem.RevokeOfferAccess(ctx, jemtest.NewIdentity("user2"), params.User("test-user"), offer1.OfferURL, jujuparams.OfferConsumeAccess)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrUnauthorized)

	err = s.jem.DB.SetApplicationOfferAccess(ctx, "user2", offer1.OfferUUID, mongodoc.ApplicationOfferNoAccess)
	c.Assert(err, jc.ErrorIsNil)

	// user2 is does not have access - not found
	err = s.jem.RevokeOfferAccess(ctx, jemtest.NewIdentity("user2"), params.User("test-user"), offer1.OfferURL, jujuparams.OfferConsumeAccess)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)

	// try revoking unknown access level
	err = s.jem.RevokeOfferAccess(ctx, jemtest.NewIdentity("user1"), params.User("test-user"), offer1.OfferURL, jujuparams.OfferAccessPermission("unknown"))
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrBadRequest)

	// try revoking for an offer that does not exist
	err = s.jem.RevokeOfferAccess(ctx, jemtest.NewIdentity("user1"), params.User("test-user"), "no such offer", jujuparams.OfferReadAccess)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)
}

func (s *applicationoffersSuite) TestDestroyOffer(c *gc.C) {
	ctx := context.Background()

	err := s.jem.Offer(ctx, s.identity, jujuparams.AddApplicationOffer{
		ModelTag:        names.NewModelTag(s.model.UUID).String(),
		OfferName:       "test-offer1",
		ApplicationName: "test-app",
		Endpoints: map[string]string{
			s.endpoint.Relation.Name: s.endpoint.Relation.Name,
		},
	})
	c.Assert(err, gc.Equals, nil)

	offer1 := mongodoc.ApplicationOffer{
		OfferURL: conv.ToOfferURL(s.model.Path, "test-offer1"),
	}
	err = s.jem.DB.GetApplicationOffer(ctx, &offer1)
	c.Assert(err, gc.Equals, nil)
	err = s.jem.DB.SetApplicationOfferAccess(ctx, identchecker.Everyone, offer1.OfferUUID, mongodoc.ApplicationOfferNoAccess)
	c.Assert(err, gc.Equals, nil)

	// user1 is an admin - this should pass
	err = s.jem.GrantOfferAccess(ctx, jemtest.NewIdentity("user1"), params.User("user2"), offer1.OfferURL, jujuparams.OfferConsumeAccess)
	c.Assert(err, gc.Equals, nil)

	// user2 has consumer access - unauthorized
	err = s.jem.DestroyOffer(ctx, jemtest.NewIdentity("user2"), offer1.OfferURL, true)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrUnauthorized)

	// user3 has no access - not found
	err = s.jem.DestroyOffer(ctx, jemtest.NewIdentity("user3"), offer1.OfferURL, true)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)

	// user1 is admin
	err = s.jem.DestroyOffer(ctx, jemtest.NewIdentity("user1"), offer1.OfferURL, true)
	c.Assert(err, gc.Equals, nil)

	offer2 := offer1
	err = s.jem.DB.GetApplicationOffer(ctx, &offer2)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)

	// offer not found
	err = s.jem.DestroyOffer(ctx, jemtest.NewIdentity("user1"), offer1.OfferURL, true)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)
}

func (s *applicationoffersSuite) TestFindApplicationOffers(c *gc.C) {
	ctx := context.Background()

	err := s.jem.Offer(ctx, s.identity, jujuparams.AddApplicationOffer{
		ModelTag:        names.NewModelTag(s.model.UUID).String(),
		OfferName:       "test-offer1",
		ApplicationName: "test-app",
		Endpoints: map[string]string{
			s.endpoint.Relation.Name: s.endpoint.Relation.Name,
		},
	})
	c.Assert(err, gc.Equals, nil)

	err = s.jem.Offer(ctx, s.identity, jujuparams.AddApplicationOffer{
		ModelTag:        names.NewModelTag(s.model.UUID).String(),
		OfferName:       "test-offer2",
		ApplicationName: "test-app",
		Endpoints: map[string]string{
			s.endpoint.Relation.Name: s.endpoint.Relation.Name,
		},
	})
	c.Assert(err, gc.Equals, nil)

	offer1 := mongodoc.ApplicationOffer{
		OfferURL: conv.ToOfferURL(s.model.Path, "test-offer1"),
	}
	offer2 := mongodoc.ApplicationOffer{
		OfferURL: conv.ToOfferURL(s.model.Path, "test-offer2"),
	}
	err = s.jem.DB.GetApplicationOffer(ctx, &offer1)
	c.Assert(err, jc.ErrorIsNil)
	err = s.jem.DB.GetApplicationOffer(ctx, &offer2)
	c.Assert(err, jc.ErrorIsNil)

	err = s.jem.DB.SetApplicationOfferAccess(ctx, params.User("user2"), offer1.OfferUUID, mongodoc.ApplicationOfferReadAccess)
	c.Assert(err, jc.ErrorIsNil)

	err = s.jem.DB.SetApplicationOfferAccess(ctx, params.User("everyone"), offer2.OfferUUID, mongodoc.ApplicationOfferNoAccess)
	c.Assert(err, jc.ErrorIsNil)

	results, err := s.jem.FindApplicationOffers(ctx, jemtest.NewIdentity("unknown-user"), jujuparams.OfferFilter{
		ModelName: s.model.UUID,
	})
	c.Assert(err, gc.Equals, nil)
	c.Assert(results, gc.HasLen, 0)

	results, err = s.jem.FindApplicationOffers(ctx, jemtest.NewIdentity("user2"), jujuparams.OfferFilter{
		ModelName: string(s.model.Path.Name),
	})
	c.Assert(err, gc.Equals, nil)
	c.Assert(results, jc.DeepEquals, []jujuparams.ApplicationOfferAdminDetails{{
		ApplicationOfferDetails: jujuparams.ApplicationOfferDetails{
			SourceModelTag:         names.NewModelTag(s.model.UUID).String(),
			OfferUUID:              offer1.OfferUUID,
			OfferURL:               offer1.OfferURL,
			OfferName:              offer1.OfferName,
			ApplicationDescription: offer1.ApplicationDescription,
			Endpoints: []jujuparams.RemoteEndpoint{{
				Name:      "url",
				Role:      charm.RoleProvider,
				Interface: "http",
				Limit:     0,
			}},
			Spaces:   []jujuparams.RemoteSpace{},
			Bindings: offer1.Bindings,
			Users: []jujuparams.OfferUserDetails{{
				UserName:    "everyone@external",
				DisplayName: "everyone",
				Access:      "read",
			}, {
				UserName:    "user2@external",
				DisplayName: "user2",
				Access:      "read",
			}},
		},
		ApplicationName: offer1.ApplicationName,
		CharmURL:        offer1.CharmURL,
		Connections:     []jujuparams.OfferConnection{},
	}})

	results, err = s.jem.FindApplicationOffers(ctx, s.identity, jujuparams.OfferFilter{
		ModelName: string(s.model.Path.Name),
	})
	c.Assert(err, gc.Equals, nil)
	c.Assert(results, jc.DeepEquals, []jujuparams.ApplicationOfferAdminDetails{{
		ApplicationOfferDetails: jujuparams.ApplicationOfferDetails{
			SourceModelTag:         names.NewModelTag(s.model.UUID).String(),
			OfferUUID:              offer1.OfferUUID,
			OfferURL:               offer1.OfferURL,
			OfferName:              offer1.OfferName,
			ApplicationDescription: offer1.ApplicationDescription,
			Endpoints: []jujuparams.RemoteEndpoint{{
				Name:      "url",
				Role:      charm.RoleProvider,
				Interface: "http",
				Limit:     0,
			}},
			Spaces:   []jujuparams.RemoteSpace{},
			Bindings: offer1.Bindings,
			Users: []jujuparams.OfferUserDetails{{
				UserName:    "everyone@external",
				DisplayName: "everyone",
				Access:      "read",
			}, {
				UserName:    "user1@external",
				DisplayName: "user1",
				Access:      "admin",
			}, {
				UserName:    "user2@external",
				DisplayName: "user2",
				Access:      "read",
			}},
		},
		ApplicationName: offer1.ApplicationName,
		CharmURL:        offer1.CharmURL,
		Connections:     []jujuparams.OfferConnection{},
	}, {
		ApplicationOfferDetails: jujuparams.ApplicationOfferDetails{
			SourceModelTag:         names.NewModelTag(s.model.UUID).String(),
			OfferUUID:              offer2.OfferUUID,
			OfferURL:               offer2.OfferURL,
			OfferName:              offer2.OfferName,
			ApplicationDescription: offer2.ApplicationDescription,
			Endpoints: []jujuparams.RemoteEndpoint{{
				Name:      "url",
				Role:      charm.RoleProvider,
				Interface: "http",
				Limit:     0,
			}},
			Spaces:   []jujuparams.RemoteSpace{},
			Bindings: offer2.Bindings,
			Users: []jujuparams.OfferUserDetails{{
				UserName:    "everyone@external",
				DisplayName: "everyone",
			}, {
				UserName:    "user1@external",
				DisplayName: "user1",
				Access:      "admin",
			}},
		},
		ApplicationName: offer2.ApplicationName,
		CharmURL:        offer2.CharmURL,
		Connections:     []jujuparams.OfferConnection{},
	}})
}

func (s *applicationoffersSuite) TestGetApplicationOffer(c *gc.C) {
	ctx := context.Background()

	err := s.jem.Offer(ctx, s.identity, jujuparams.AddApplicationOffer{
		ModelTag:        names.NewModelTag(s.model.UUID).String(),
		OfferName:       "test-offer1",
		ApplicationName: "test-app",
		Endpoints: map[string]string{
			s.endpoint.Relation.Name: s.endpoint.Relation.Name,
		},
	})
	c.Assert(err, gc.Equals, nil)

	err = s.jem.Offer(ctx, s.identity, jujuparams.AddApplicationOffer{
		ModelTag:        names.NewModelTag(s.model.UUID).String(),
		OfferName:       "test-offer2",
		ApplicationName: "test-app",
		Endpoints: map[string]string{
			s.endpoint.Relation.Name: s.endpoint.Relation.Name,
		},
	})
	c.Assert(err, gc.Equals, nil)

	offer1 := mongodoc.ApplicationOffer{
		OfferURL: conv.ToOfferURL(s.model.Path, "test-offer1"),
	}
	offer2 := mongodoc.ApplicationOffer{
		OfferURL: conv.ToOfferURL(s.model.Path, "test-offer2"),
	}
	err = s.jem.DB.GetApplicationOffer(ctx, &offer1)
	c.Assert(err, jc.ErrorIsNil)
	err = s.jem.DB.GetApplicationOffer(ctx, &offer2)
	c.Assert(err, jc.ErrorIsNil)

	err = s.jem.DB.SetApplicationOfferAccess(ctx, params.User("everyone"), offer2.OfferUUID, mongodoc.ApplicationOfferNoAccess)
	c.Assert(err, jc.ErrorIsNil)

	// "unknown-user" does not have acces to offer2
	_, err = s.jem.GetApplicationOffer(ctx, jemtest.NewIdentity("unknown-user"), offer2.OfferURL)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)

	// "user2" has read access to offer1
	offerDetails, err := s.jem.GetApplicationOffer(ctx, jemtest.NewIdentity("unknown-user"), offer1.OfferURL)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(offerDetails, jc.DeepEquals, &jujuparams.ApplicationOfferAdminDetails{
		ApplicationOfferDetails: jujuparams.ApplicationOfferDetails{
			SourceModelTag:         names.NewModelTag(s.model.UUID).String(),
			OfferUUID:              offer1.OfferUUID,
			OfferURL:               offer1.OfferURL,
			OfferName:              offer1.OfferName,
			ApplicationDescription: offer1.ApplicationDescription,
			Endpoints: []jujuparams.RemoteEndpoint{{
				Name:      "url",
				Role:      charm.RoleProvider,
				Interface: "http",
				Limit:     0,
			}},
			Spaces:   []jujuparams.RemoteSpace{},
			Bindings: offer1.Bindings,
			Users: []jujuparams.OfferUserDetails{{
				UserName:    "everyone@external",
				DisplayName: "everyone",
				Access:      "read",
			}},
		},
		ApplicationName: offer1.ApplicationName,
		CharmURL:        offer1.CharmURL,
		Connections:     []jujuparams.OfferConnection{},
	})

	// "user1" is admin and will see addition details of offer1
	offerDetails, err = s.jem.GetApplicationOffer(ctx, jemtest.NewIdentity("user1"), offer1.OfferURL)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(offerDetails, jc.DeepEquals, &jujuparams.ApplicationOfferAdminDetails{
		ApplicationOfferDetails: jujuparams.ApplicationOfferDetails{
			SourceModelTag:         names.NewModelTag(s.model.UUID).String(),
			OfferUUID:              offer1.OfferUUID,
			OfferURL:               offer1.OfferURL,
			OfferName:              offer1.OfferName,
			ApplicationDescription: offer1.ApplicationDescription,
			Endpoints: []jujuparams.RemoteEndpoint{{
				Name:      "url",
				Role:      charm.RoleProvider,
				Interface: "http",
				Limit:     0,
			}},
			Spaces:   []jujuparams.RemoteSpace{},
			Bindings: offer1.Bindings,
			Users: []jujuparams.OfferUserDetails{{
				UserName:    "everyone@external",
				DisplayName: "everyone",
				Access:      "read",
			}, {
				UserName:    "user1@external",
				DisplayName: "user1",
				Access:      "admin",
			}},
		},
		ApplicationName: offer1.ApplicationName,
		CharmURL:        offer1.CharmURL,
		Connections:     []jujuparams.OfferConnection{},
	})

	// "user1" is admin but still cannot get application offers that do not exist
	_, err = s.jem.GetApplicationOffer(ctx, jemtest.NewIdentity("user1"), "no-such-offer")
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)
}

func (s *applicationoffersSuite) TestUpdateApplicationOffer(c *gc.C) {
	ctx := context.Background()

	err := s.jem.Offer(ctx, s.identity, jujuparams.AddApplicationOffer{
		ModelTag:               names.NewModelTag(s.model.UUID).String(),
		OfferName:              "test-offer1",
		ApplicationName:        "test-app",
		ApplicationDescription: "test application description",
		Endpoints: map[string]string{
			s.endpoint.Relation.Name: s.endpoint.Relation.Name,
		},
	})
	c.Assert(err, gc.Equals, nil)

	offer1 := mongodoc.ApplicationOffer{
		OfferURL: conv.ToOfferURL(s.model.Path, "test-offer1"),
	}
	err = s.jem.DB.GetApplicationOffer(ctx, &offer1)
	c.Assert(err, jc.ErrorIsNil)

	modelState, err := s.StatePool.Get(s.model.UUID)
	c.Assert(err, gc.Equals, nil)
	defer modelState.Release()

	appOfferState := state.NewApplicationOffers(modelState.State)
	_, err = appOfferState.UpdateOffer(crossmodel.AddApplicationOfferArgs{
		OfferName:              offer1.OfferName,
		Owner:                  offer1.OwnerName,
		ApplicationName:        offer1.ApplicationName,
		ApplicationDescription: "changed test application description",
	})
	c.Assert(err, jc.ErrorIsNil)

	err = s.jem.UpdateApplicationOffer(ctx, offer1.OfferUUID, false)
	c.Assert(err, jc.ErrorIsNil)

	offer2 := mongodoc.ApplicationOffer{
		OfferUUID: offer1.OfferUUID,
	}
	err = s.jem.DB.GetApplicationOffer(ctx, &offer2)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(offer2.ApplicationDescription, gc.Equals, "changed test application description")

	err = s.jem.UpdateApplicationOffer(ctx, offer1.OfferUUID, true)
	c.Assert(err, jc.ErrorIsNil)

	offer3 := mongodoc.ApplicationOffer{
		OfferUUID: offer1.OfferUUID,
	}
	err = s.jem.DB.GetApplicationOffer(ctx, &offer3)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)
}
