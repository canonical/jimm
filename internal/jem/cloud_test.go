// Copyright 2015 Canonical Ltd.

package jem_test

import (
	"context"

	"github.com/juju/juju/cloud"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"
	"gopkg.in/errgo.v1"
	"gopkg.in/macaroon-bakery.v2/httpbakery"
	"gopkg.in/mgo.v2/bson"

	"github.com/CanonicalLtd/jimm/internal/auth"
	"github.com/CanonicalLtd/jimm/internal/jem"
	"github.com/CanonicalLtd/jimm/internal/jemtest"
	"github.com/CanonicalLtd/jimm/internal/kubetest"
	"github.com/CanonicalLtd/jimm/internal/mgosession"
	"github.com/CanonicalLtd/jimm/internal/mongodoc"
	"github.com/CanonicalLtd/jimm/internal/pubsub"
	"github.com/CanonicalLtd/jimm/params"
)

type cloudSuite struct {
	jemtest.JujuConnSuite
	pool                           *jem.Pool
	sessionPool                    *mgosession.Pool
	jem                            *jem.JEM
	usageSenderAuthorizationClient *testUsageSenderAuthorizationClient
}

var _ = gc.Suite(&cloudSuite{})

func (s *cloudSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.sessionPool = mgosession.NewPool(context.TODO(), s.Session, 5)
	publicCloudMetadata, _, err := cloud.PublicCloudMetadata()
	c.Assert(err, gc.Equals, nil)
	s.usageSenderAuthorizationClient = &testUsageSenderAuthorizationClient{}
	s.PatchValue(&jem.NewUsageSenderAuthorizationClient, func(_ string, _ *httpbakery.Client) (jem.UsageSenderAuthorizationClient, error) {
		return s.usageSenderAuthorizationClient, nil
	})
	pool, err := jem.NewPool(context.TODO(), jem.Params{
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
	s.jem = s.pool.JEM(context.TODO())
	s.PatchValue(&utils.OutgoingAccessAllowed, true)
}

func (s *cloudSuite) TearDownTest(c *gc.C) {
	s.jem.Close()
	s.pool.Close()
	s.sessionPool.Close()
	s.JujuConnSuite.TearDownTest(c)
}

func (s *cloudSuite) TestCreateCloud(c *gc.C) {
	ctlPath := params.EntityPath{"bob", "foo"}
	addController(c, ctlPath, s.APIInfo(c), s.jem)
	ctx := auth.ContextWithIdentity(testContext, jemtest.NewIdentity("bob", "bob-group"))
	err := s.jem.CreateCloud(ctx, mongodoc.CloudRegion{
		Cloud:            "test-cloud",
		ProviderType:     "kubernetes",
		AuthTypes:        []string{"certificate"},
		Endpoint:         "https://1.2.3.4:5678",
		IdentityEndpoint: "https://1.2.3.4:5679",
		StorageEndpoint:  "https://1.2.3.4:5680",
		CACertificates:   []string{"This is a CA Certficiate (honest)"},
		ACL: params.ACL{
			Read:  []string{"bob"},
			Write: []string{"bob"},
			Admin: []string{"bob"},
		},
	}, nil, jem.CreateCloudParams{HostCloudRegion: "dummy/dummy-region"})
	c.Assert(err, gc.Equals, nil)

	var docs []mongodoc.CloudRegion
	err = s.jem.DB.CloudRegions().Find(bson.D{{"cloud", "test-cloud"}}).Sort("_id").All(&docs)
	c.Assert(err, gc.Equals, nil)
	c.Assert(docs, jc.DeepEquals, []mongodoc.CloudRegion{{
		Id:                   "test-cloud/",
		Cloud:                "test-cloud",
		ProviderType:         "kubernetes",
		AuthTypes:            []string{"certificate"},
		Endpoint:             "https://1.2.3.4:5678",
		IdentityEndpoint:     "https://1.2.3.4:5679",
		StorageEndpoint:      "https://1.2.3.4:5680",
		CACertificates:       []string{"This is a CA Certficiate (honest)"},
		PrimaryControllers:   []params.EntityPath{{User: "bob", Name: "foo"}},
		SecondaryControllers: []params.EntityPath{},
		ACL: params.ACL{
			Read:  []string{"bob"},
			Write: []string{"bob"},
			Admin: []string{"bob"},
		},
	}})
}

func (s *cloudSuite) TestCreateCloudWithRegions(c *gc.C) {
	ctlPath := params.EntityPath{"bob", "foo"}
	addController(c, ctlPath, s.APIInfo(c), s.jem)
	ctx := auth.ContextWithIdentity(testContext, jemtest.NewIdentity("bob", "bob-group"))
	err := s.jem.CreateCloud(ctx, mongodoc.CloudRegion{
		Cloud:            "test-cloud",
		ProviderType:     "kubernetes",
		AuthTypes:        []string{"certificate"},
		Endpoint:         "https://1.2.3.4:5678",
		IdentityEndpoint: "https://1.2.3.4:5679",
		StorageEndpoint:  "https://1.2.3.4:5680",
		CACertificates:   []string{"This is a CA Certficiate (honest)"},
		ACL: params.ACL{
			Read:  []string{"bob"},
			Write: []string{"bob"},
			Admin: []string{"bob"},
		},
	}, []mongodoc.CloudRegion{{
		Cloud:  "test-cloud",
		Region: "test-region",
		ACL: params.ACL{
			Read:  []string{"bob"},
			Write: []string{"bob"},
			Admin: []string{"bob"},
		},
	}}, jem.CreateCloudParams{
		HostCloudRegion: "dummy/dummy-region",
	})
	c.Assert(err, gc.Equals, nil)

	var docs []mongodoc.CloudRegion
	err = s.jem.DB.CloudRegions().Find(bson.D{{"cloud", "test-cloud"}}).Sort("_id").All(&docs)
	c.Assert(err, gc.Equals, nil)
	c.Assert(docs, jc.DeepEquals, []mongodoc.CloudRegion{{
		Id:                   "test-cloud/",
		Cloud:                "test-cloud",
		ProviderType:         "kubernetes",
		AuthTypes:            []string{"certificate"},
		Endpoint:             "https://1.2.3.4:5678",
		IdentityEndpoint:     "https://1.2.3.4:5679",
		StorageEndpoint:      "https://1.2.3.4:5680",
		CACertificates:       []string{"This is a CA Certficiate (honest)"},
		PrimaryControllers:   []params.EntityPath{{User: "bob", Name: "foo"}},
		SecondaryControllers: []params.EntityPath{},
		ACL: params.ACL{
			Read:  []string{"bob"},
			Write: []string{"bob"},
			Admin: []string{"bob"},
		},
	}, {
		Id:                   "test-cloud/test-region",
		Cloud:                "test-cloud",
		Region:               "test-region",
		PrimaryControllers:   []params.EntityPath{{User: "bob", Name: "foo"}},
		SecondaryControllers: []params.EntityPath{},
		ACL: params.ACL{
			Read:  []string{"bob"},
			Write: []string{"bob"},
			Admin: []string{"bob"},
		},
	}})
}

func (s *cloudSuite) TestCreateCloudNameMatch(c *gc.C) {
	ctlPath := params.EntityPath{"bob", "foo"}
	addController(c, ctlPath, s.APIInfo(c), s.jem)
	ctx := auth.ContextWithIdentity(testContext, jemtest.NewIdentity("bob", "bob-group"))
	err := s.jem.CreateCloud(
		ctx,
		mongodoc.CloudRegion{
			Cloud:            "dummy",
			ProviderType:     "kubernetes",
			AuthTypes:        []string{"certificate"},
			Endpoint:         "https://1.2.3.4:5678",
			IdentityEndpoint: "https://1.2.3.4:5679",
			StorageEndpoint:  "https://1.2.3.4:5680",
			CACertificates:   []string{"This is a CA Certficiate (honest)"},
			ACL: params.ACL{
				Read:  []string{"bob"},
				Write: []string{"bob"},
				Admin: []string{"bob"},
			},
		},
		nil,
		jem.CreateCloudParams{
			HostCloudRegion: "dummy/dummy-region",
		},
	)
	c.Assert(err, gc.ErrorMatches, `cloud "dummy" already exists`)
}

func (s *cloudSuite) TestCreateCloudPublicNameMatch(c *gc.C) {
	ctlPath := params.EntityPath{"bob", "foo"}
	addController(c, ctlPath, s.APIInfo(c), s.jem)
	ctx := auth.ContextWithIdentity(testContext, jemtest.NewIdentity("bob", "bob-group"))
	err := s.jem.CreateCloud(
		ctx,
		mongodoc.CloudRegion{
			Cloud:            "aws-china",
			ProviderType:     "kubernetes",
			AuthTypes:        []string{"certificate"},
			Endpoint:         "https://1.2.3.4:5678",
			IdentityEndpoint: "https://1.2.3.4:5679",
			StorageEndpoint:  "https://1.2.3.4:5680",
			CACertificates:   []string{"This is a CA Certficiate (honest)"},
			ACL: params.ACL{
				Read:  []string{"bob"},
				Write: []string{"bob"},
				Admin: []string{"bob"},
			},
		},
		nil,
		jem.CreateCloudParams{
			HostCloudRegion: "dummy/dummy-region",
		},
	)
	c.Assert(err, gc.ErrorMatches, `cloud "aws-china" already exists`)
}

func (s *cloudSuite) TestCreateCloudNoControllers(c *gc.C) {
	ctx := auth.ContextWithIdentity(testContext, jemtest.NewIdentity("bob", "bob-group"))
	err := s.jem.CreateCloud(
		ctx,
		mongodoc.CloudRegion{
			Cloud:            "test-cloud",
			ProviderType:     "kubernetes",
			AuthTypes:        []string{"certificate"},
			Endpoint:         "https://1.2.3.4:5678",
			IdentityEndpoint: "https://1.2.3.4:5679",
			StorageEndpoint:  "https://1.2.3.4:5680",
			CACertificates:   []string{"This is a CA Certficiate (honest)"},
			ACL: params.ACL{
				Read:  []string{"bob"},
				Write: []string{"bob"},
				Admin: []string{"bob"},
			},
		},
		nil,
		jem.CreateCloudParams{
			HostCloudRegion: "aws/eu-west-99",
		},
	)
	c.Assert(err, gc.ErrorMatches, `cloud "aws" region "eu-west-99" not found`)

	var docs []mongodoc.CloudRegion
	err = s.jem.DB.CloudRegions().Find(bson.D{{"cloud", "test-cloud"}}).Sort("_id").All(&docs)
	c.Assert(err, gc.Equals, nil)
	c.Assert(docs, jc.DeepEquals, []mongodoc.CloudRegion{})
}

func (s *cloudSuite) TestCreateCloudAddCloudError(c *gc.C) {
	ctlPath := params.EntityPath{"bob", "foo"}
	addController(c, ctlPath, s.APIInfo(c), s.jem)
	ctx := auth.ContextWithIdentity(testContext, jemtest.NewIdentity("bob", "bob-group"))
	err := s.jem.CreateCloud(
		ctx,
		mongodoc.CloudRegion{
			Cloud:            "test-cloud",
			ProviderType:     "kubernetes",
			Endpoint:         "https://1.2.3.4:5678",
			IdentityEndpoint: "https://1.2.3.4:5679",
			StorageEndpoint:  "https://1.2.3.4:5680",
			CACertificates:   []string{"This is a CA Certficiate (honest)"},
			ACL: params.ACL{
				Read:  []string{"bob"},
				Write: []string{"bob"},
				Admin: []string{"bob"},
			},
		},
		nil,
		jem.CreateCloudParams{
			HostCloudRegion: "dummy/dummy-region",
		},
	)
	c.Assert(err, gc.ErrorMatches, `invalid cloud: empty auth-types not valid`)

	var docs []mongodoc.CloudRegion
	err = s.jem.DB.CloudRegions().Find(bson.D{{"cloud", "test-cloud"}}).Sort("_id").All(&docs)
	c.Assert(err, gc.Equals, nil)
	c.Assert(docs, jc.DeepEquals, []mongodoc.CloudRegion{})
}

func (s *cloudSuite) TestCreateCloudNoHostCloudRegion(c *gc.C) {
	ctlPath := params.EntityPath{"bob", "foo"}
	addController(c, ctlPath, s.APIInfo(c), s.jem)
	ctx := auth.ContextWithIdentity(testContext, jemtest.NewIdentity("bob", "bob-group"))
	err := s.jem.CreateCloud(
		ctx,
		mongodoc.CloudRegion{
			Cloud:            "test-cloud",
			ProviderType:     "kubernetes",
			Endpoint:         "https://1.2.3.4:5678",
			IdentityEndpoint: "https://1.2.3.4:5679",
			StorageEndpoint:  "https://1.2.3.4:5680",
			CACertificates:   []string{"This is a CA Certficiate (honest)"},
			ACL: params.ACL{
				Read:  []string{"bob"},
				Write: []string{"bob"},
				Admin: []string{"bob"},
			},
		},
		nil,
		jem.CreateCloudParams{},
	)
	c.Assert(err, gc.ErrorMatches, `cloud region required`)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrCloudRegionRequired)

	var docs []mongodoc.CloudRegion
	err = s.jem.DB.CloudRegions().Find(bson.D{{"cloud", "test-cloud"}}).Sort("_id").All(&docs)
	c.Assert(err, gc.Equals, nil)
	c.Assert(docs, jc.DeepEquals, []mongodoc.CloudRegion{})
}

func (s *cloudSuite) TestRemoveCloud(c *gc.C) {
	ctlPath := params.EntityPath{"bob", "foo"}
	addController(c, ctlPath, s.APIInfo(c), s.jem)
	s.createK8sCloud(c, "test-cloud", "bob")

	ctx := auth.ContextWithIdentity(testContext, jemtest.NewIdentity("bob", "bob-group"))
	err := s.jem.RemoveCloud(ctx, "test-cloud")
	c.Assert(err, gc.Equals, nil)

	var docs []mongodoc.CloudRegion
	err = s.jem.DB.CloudRegions().Find(bson.D{{"cloud", "test-cloud"}}).Sort("_id").All(&docs)
	c.Assert(err, gc.Equals, nil)
	c.Assert(docs, jc.DeepEquals, []mongodoc.CloudRegion{})
}

func (s *cloudSuite) TestRemoveCloudUnauthorized(c *gc.C) {
	ctlPath := params.EntityPath{"bob", "foo"}
	addController(c, ctlPath, s.APIInfo(c), s.jem)
	s.createK8sCloud(c, "test-cloud", "alice")
	ctx := auth.ContextWithIdentity(testContext, jemtest.NewIdentity("alice"))
	err := s.jem.GrantCloud(ctx, "test-cloud", "bob", "add-model")
	c.Assert(err, gc.Equals, nil)

	ctx = auth.ContextWithIdentity(testContext, jemtest.NewIdentity("bob", "bob-group"))
	err = s.jem.RemoveCloud(ctx, "test-cloud")
	c.Assert(err, gc.ErrorMatches, `unauthorized`)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrUnauthorized)
}

func (s *cloudSuite) TestRemoveCloudNotFound(c *gc.C) {
	ctlPath := params.EntityPath{"bob", "foo"}
	addController(c, ctlPath, s.APIInfo(c), s.jem)
	ctx := auth.ContextWithIdentity(testContext, jemtest.NewIdentity("bob", "bob-group"))

	err := s.jem.RemoveCloud(ctx, "test-cloud")
	c.Assert(err, gc.ErrorMatches, `cloud "test-cloud" region "" not found`)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)
}

func (s *cloudSuite) TestGrantCloud(c *gc.C) {
	ctlPath := params.EntityPath{"bob", "foo"}
	addController(c, ctlPath, s.APIInfo(c), s.jem)
	s.createK8sCloud(c, "test-cloud", "bob")

	ctx := auth.ContextWithIdentity(testContext, jemtest.NewIdentity("bob", "bob-group"))
	err := s.jem.GrantCloud(ctx, "test-cloud", "alice", "admin")
	c.Assert(err, gc.Equals, nil)

	var docs []mongodoc.CloudRegion
	err = s.jem.DB.CloudRegions().Find(bson.D{{"cloud", "test-cloud"}}).Sort("_id").All(&docs)
	c.Assert(err, gc.Equals, nil)
	c.Assert(docs, jc.DeepEquals, []mongodoc.CloudRegion{{
		Id:                   "test-cloud/",
		Cloud:                "test-cloud",
		ProviderType:         "kubernetes",
		AuthTypes:            []string{"certificate"},
		Endpoint:             "https://1.2.3.4:5678",
		IdentityEndpoint:     "https://1.2.3.4:5679",
		StorageEndpoint:      "https://1.2.3.4:5680",
		CACertificates:       []string{"This is a CA Certficiate (honest)"},
		PrimaryControllers:   []params.EntityPath{{User: "bob", Name: "foo"}},
		SecondaryControllers: []params.EntityPath{},
		ACL: params.ACL{
			Read:  []string{"bob", "alice"},
			Write: []string{"bob", "alice"},
			Admin: []string{"bob", "alice"},
		},
	}})
}

func (s *cloudSuite) TestGrantCloudUnauthorized(c *gc.C) {
	ctlPath := params.EntityPath{"bob", "foo"}
	addController(c, ctlPath, s.APIInfo(c), s.jem)
	s.createK8sCloud(c, "test-cloud", "bob")

	err := s.jem.GrantCloud(auth.ContextWithIdentity(testContext, jemtest.NewIdentity("alice")), "test-cloud", "alice", "admin")
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrUnauthorized)
	c.Assert(err, gc.ErrorMatches, `unauthorized`)
}

func (s *cloudSuite) TestGrantCloudNotFound(c *gc.C) {
	err := s.jem.GrantCloud(auth.ContextWithIdentity(testContext, jemtest.NewIdentity("alice")), "test-cloud", "alice", "admin")
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)
	c.Assert(err, gc.ErrorMatches, `cloud "test-cloud" region "" not found`)
}

func (s *cloudSuite) TestGrantCloudInvalidAccess(c *gc.C) {
	ctlPath := params.EntityPath{"bob", "foo"}
	addController(c, ctlPath, s.APIInfo(c), s.jem)
	s.createK8sCloud(c, "test-cloud", "bob")

	ctx := auth.ContextWithIdentity(testContext, jemtest.NewIdentity("bob", "bob-group"))
	err := s.jem.GrantCloud(ctx, "test-cloud", "alice", "not-valid")
	c.Assert(err, gc.ErrorMatches, `"not-valid" cloud access not valid`)
}

func (s *cloudSuite) TestRevokeCloud(c *gc.C) {
	ctlPath := params.EntityPath{"bob", "foo"}
	addController(c, ctlPath, s.APIInfo(c), s.jem)
	s.createK8sCloud(c, "test-cloud", "bob")

	ctx := auth.ContextWithIdentity(testContext, jemtest.NewIdentity("bob", "bob-group"))
	err := s.jem.GrantCloud(ctx, "test-cloud", "alice", "admin")
	c.Assert(err, gc.Equals, nil)

	var docs []mongodoc.CloudRegion
	err = s.jem.DB.CloudRegions().Find(bson.D{{"cloud", "test-cloud"}}).Sort("_id").All(&docs)
	c.Assert(err, gc.Equals, nil)
	c.Assert(docs, jc.DeepEquals, []mongodoc.CloudRegion{{
		Id:                   "test-cloud/",
		Cloud:                "test-cloud",
		ProviderType:         "kubernetes",
		AuthTypes:            []string{"certificate"},
		Endpoint:             "https://1.2.3.4:5678",
		IdentityEndpoint:     "https://1.2.3.4:5679",
		StorageEndpoint:      "https://1.2.3.4:5680",
		CACertificates:       []string{"This is a CA Certficiate (honest)"},
		PrimaryControllers:   []params.EntityPath{{User: "bob", Name: "foo"}},
		SecondaryControllers: []params.EntityPath{},
		ACL: params.ACL{
			Read:  []string{"bob", "alice"},
			Write: []string{"bob", "alice"},
			Admin: []string{"bob", "alice"},
		},
	}})

	err = s.jem.RevokeCloud(ctx, "test-cloud", "alice", "add-model")
	c.Assert(err, gc.Equals, nil)

	err = s.jem.DB.CloudRegions().Find(bson.D{{"cloud", "test-cloud"}}).Sort("_id").All(&docs)
	c.Assert(err, gc.Equals, nil)
	c.Assert(docs, jc.DeepEquals, []mongodoc.CloudRegion{{
		Id:                   "test-cloud/",
		Cloud:                "test-cloud",
		ProviderType:         "kubernetes",
		AuthTypes:            []string{"certificate"},
		Endpoint:             "https://1.2.3.4:5678",
		IdentityEndpoint:     "https://1.2.3.4:5679",
		StorageEndpoint:      "https://1.2.3.4:5680",
		CACertificates:       []string{"This is a CA Certficiate (honest)"},
		PrimaryControllers:   []params.EntityPath{{User: "bob", Name: "foo"}},
		SecondaryControllers: []params.EntityPath{},
		ACL: params.ACL{
			Read:  []string{"bob"},
			Write: []string{"bob"},
			Admin: []string{"bob"},
		},
	}})
}

func (s *cloudSuite) TestRevokeCloudUnauthorized(c *gc.C) {
	ctlPath := params.EntityPath{"bob", "foo"}
	addController(c, ctlPath, s.APIInfo(c), s.jem)
	s.createK8sCloud(c, "test-cloud", "bob")

	ctx := auth.ContextWithIdentity(testContext, jemtest.NewIdentity("bob", "bob-group"))
	err := s.jem.GrantCloud(ctx, "test-cloud", "alice", "admin")
	c.Assert(err, gc.Equals, nil)

	err = s.jem.RevokeCloud(auth.ContextWithIdentity(testContext, jemtest.NewIdentity("charlie")), "test-cloud", "alice", "admin")
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrUnauthorized)
	c.Assert(err, gc.ErrorMatches, `unauthorized`)
}

func (s *cloudSuite) TestRevokeCloudNotFound(c *gc.C) {
	err := s.jem.RevokeCloud(auth.ContextWithIdentity(testContext, jemtest.NewIdentity("alice")), "test-cloud", "alice", "admin")
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)
	c.Assert(err, gc.ErrorMatches, `cloud "test-cloud" region "" not found`)
}

func (s *cloudSuite) TestRevokeCloudInvalidAccess(c *gc.C) {
	ctlPath := params.EntityPath{"bob", "foo"}
	addController(c, ctlPath, s.APIInfo(c), s.jem)
	s.createK8sCloud(c, "test-cloud", "bob")

	ctx := auth.ContextWithIdentity(testContext, jemtest.NewIdentity("bob", "bob-group"))
	err := s.jem.RevokeCloud(ctx, "test-cloud", "alice", "not-valid")
	c.Assert(err, gc.ErrorMatches, `"not-valid" cloud access not valid`)
}

func (s *cloudSuite) createK8sCloud(c *gc.C, name, owner string) {
	err := s.jem.CreateCloud(
		auth.ContextWithIdentity(testContext, jemtest.NewIdentity(owner)),
		mongodoc.CloudRegion{
			Cloud:            params.Cloud(name),
			ProviderType:     "kubernetes",
			AuthTypes:        []string{"certificate"},
			Endpoint:         "https://1.2.3.4:5678",
			IdentityEndpoint: "https://1.2.3.4:5679",
			StorageEndpoint:  "https://1.2.3.4:5680",
			CACertificates:   []string{"This is a CA Certficiate (honest)"},
			ACL: params.ACL{
				Read:  []string{owner},
				Write: []string{owner},
				Admin: []string{owner},
			},
		},
		nil,
		jem.CreateCloudParams{
			HostCloudRegion: "dummy/dummy-region",
		},
	)
	c.Assert(err, gc.Equals, nil)
}

func (s *cloudSuite) TestRemoveCloudWithModel(c *gc.C) {
	ksrv := kubetest.NewFakeKubernetes(c)
	defer ksrv.Close()

	ctlPath := params.EntityPath{"bob", "foo"}
	addController(c, ctlPath, s.APIInfo(c), s.jem)
	ctx := auth.ContextWithIdentity(testContext, jemtest.NewIdentity("bob", "bob-group"))

	err := s.jem.CreateCloud(
		ctx,
		mongodoc.CloudRegion{
			Cloud:        "test-cloud",
			ProviderType: "kubernetes",
			AuthTypes:    []string{string(cloud.UserPassAuthType)},
			Endpoint:     ksrv.URL,
			ACL: params.ACL{
				Read:  []string{"bob"},
				Write: []string{"bob"},
				Admin: []string{"bob"},
			},
		},
		nil,
		jem.CreateCloudParams{
			HostCloudRegion: "dummy/dummy-region",
		},
	)
	c.Assert(err, gc.Equals, nil)

	credpath := params.CredentialPath{
		Cloud: "test-cloud",
		User:  "bob",
		Name:  "kubernetes",
	}
	_, err = s.jem.UpdateCredential(ctx, &mongodoc.Credential{
		Path: mongodoc.CredentialPathFromParams(credpath),
		Type: string(cloud.UserPassAuthType),
		Attributes: map[string]string{
			"username": kubetest.Username,
			"password": kubetest.Password,
		},
	}, jem.CredentialUpdate)
	c.Assert(err, gc.Equals, nil)

	_, err = s.jem.CreateModel(ctx, jem.CreateModelParams{
		Path:       params.EntityPath{"bob", "test-model"},
		Cloud:      "test-cloud",
		Credential: credpath,
	})
	c.Assert(err, gc.Equals, nil)

	err = s.jem.RemoveCloud(ctx, "test-cloud")
	c.Assert(err, gc.ErrorMatches, `cloud is used by 1 model`)
}
