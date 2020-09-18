// Copyright 2015 Canonical Ltd.

package jem_test

import (
	"context"

	jujuparams "github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cloud"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"
	"gopkg.in/errgo.v1"
	"gopkg.in/macaroon-bakery.v2/httpbakery"
	"gopkg.in/mgo.v2/bson"

	"github.com/CanonicalLtd/jimm/internal/apiconn"
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

func (s *cloudSuite) TestAddCloud(c *gc.C) {
	ctx := context.Background()

	ctlPath := params.EntityPath{"bob", "foo"}
	addController(c, ctlPath, s.APIInfo(c), s.jem)
	id := jemtest.NewIdentity("bob", "bob-group")
	err := s.jem.AddHostedCloud(ctx, id, "test-cloud", jujuparams.Cloud{
		Type:             "kubernetes",
		HostCloudRegion:  "dummy/dummy-region",
		AuthTypes:        []string{"certificate"},
		Endpoint:         "https://1.2.3.4:5678",
		IdentityEndpoint: "https://1.2.3.4:5679",
		StorageEndpoint:  "https://1.2.3.4:5680",
		CACertificates:   []string{"This is a CA Certficiate (honest)"},
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
		Id:                   "test-cloud/default",
		Cloud:                "test-cloud",
		Region:               "default",
		ProviderType:         "kubernetes",
		PrimaryControllers:   []params.EntityPath{{User: "bob", Name: "foo"}},
		SecondaryControllers: []params.EntityPath{},
		ACL: params.ACL{
			Read:  []string{"bob"},
			Write: []string{"bob"},
			Admin: []string{"bob"},
		},
	}})
}

func (s *cloudSuite) TestAddCloudWithRegions(c *gc.C) {
	ctx := context.Background()

	ctlPath := params.EntityPath{"bob", "foo"}
	addController(c, ctlPath, s.APIInfo(c), s.jem)
	id := jemtest.NewIdentity("bob", "bob-group")
	err := s.jem.AddHostedCloud(ctx, id, "test-cloud", jujuparams.Cloud{
		Type:             "kubernetes",
		HostCloudRegion:  "dummy/dummy-region",
		AuthTypes:        []string{"certificate"},
		Endpoint:         "https://1.2.3.4:5678",
		IdentityEndpoint: "https://1.2.3.4:5679",
		StorageEndpoint:  "https://1.2.3.4:5680",
		CACertificates:   []string{"This is a CA Certficiate (honest)"},
		Regions: []jujuparams.CloudRegion{{
			Name: "region1",
		}},
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
		Id:                   "test-cloud/region1",
		Cloud:                "test-cloud",
		Region:               "region1",
		ProviderType:         "kubernetes",
		PrimaryControllers:   []params.EntityPath{{User: "bob", Name: "foo"}},
		SecondaryControllers: []params.EntityPath{},
		ACL: params.ACL{
			Read:  []string{"bob"},
			Write: []string{"bob"},
			Admin: []string{"bob"},
		},
	}})
}

func (s *cloudSuite) TestAddCloudNameMatch(c *gc.C) {
	ctx := context.Background()

	ctlPath := params.EntityPath{"bob", "foo"}
	addController(c, ctlPath, s.APIInfo(c), s.jem)
	id := jemtest.NewIdentity("bob", "bob-group")
	err := s.jem.AddHostedCloud(ctx, id, "dummy", jujuparams.Cloud{
		Type:             "kubernetes",
		HostCloudRegion:  "dummy/dummy-region",
		AuthTypes:        []string{"certificate"},
		Endpoint:         "https://1.2.3.4:5678",
		IdentityEndpoint: "https://1.2.3.4:5679",
		StorageEndpoint:  "https://1.2.3.4:5680",
		CACertificates:   []string{"This is a CA Certficiate (honest)"},
	})
	c.Assert(err, gc.ErrorMatches, `cloud "dummy" already exists`)
}

func (s *cloudSuite) TestAddCloudPublicNameMatch(c *gc.C) {
	ctx := context.Background()

	ctlPath := params.EntityPath{"bob", "foo"}
	addController(c, ctlPath, s.APIInfo(c), s.jem)
	id := jemtest.NewIdentity("bob", "bob-group")
	err := s.jem.AddHostedCloud(ctx, id, "aws-china", jujuparams.Cloud{
		Type:             "kubernetes",
		HostCloudRegion:  "dummy/dummy-region",
		AuthTypes:        []string{"certificate"},
		Endpoint:         "https://1.2.3.4:5678",
		IdentityEndpoint: "https://1.2.3.4:5679",
		StorageEndpoint:  "https://1.2.3.4:5680",
		CACertificates:   []string{"This is a CA Certficiate (honest)"},
	})
	c.Assert(err, gc.ErrorMatches, `cloud "aws-china" already exists`)
}

func (s *cloudSuite) TestAddCloudNoControllers(c *gc.C) {
	ctx := context.Background()

	ctlPath := params.EntityPath{"bob", "foo"}
	addController(c, ctlPath, s.APIInfo(c), s.jem)
	id := jemtest.NewIdentity("bob", "bob-group")
	err := s.jem.AddHostedCloud(ctx, id, "test-cloud", jujuparams.Cloud{
		Type:             "kubernetes",
		HostCloudRegion:  "aws/eu-west-99",
		AuthTypes:        []string{"certificate"},
		Endpoint:         "https://1.2.3.4:5678",
		IdentityEndpoint: "https://1.2.3.4:5679",
		StorageEndpoint:  "https://1.2.3.4:5680",
		CACertificates:   []string{"This is a CA Certficiate (honest)"},
	})
	c.Assert(err, gc.ErrorMatches, `cloudregion not found`)

	var docs []mongodoc.CloudRegion
	err = s.jem.DB.CloudRegions().Find(bson.D{{"cloud", "test-cloud"}}).Sort("_id").All(&docs)
	c.Assert(err, gc.Equals, nil)
	c.Assert(docs, jc.DeepEquals, []mongodoc.CloudRegion{})
}

func (s *cloudSuite) TestAddCloudControllerError(c *gc.C) {
	ctx := context.Background()

	ctlPath := params.EntityPath{"bob", "foo"}
	addController(c, ctlPath, s.APIInfo(c), s.jem)
	id := jemtest.NewIdentity("bob", "bob-group")
	err := s.jem.AddHostedCloud(ctx, id, "test-cloud", jujuparams.Cloud{
		Type:             "kubernetes",
		HostCloudRegion:  "dummy/dummy-region",
		Endpoint:         "https://1.2.3.4:5678",
		IdentityEndpoint: "https://1.2.3.4:5679",
		StorageEndpoint:  "https://1.2.3.4:5680",
		CACertificates:   []string{"This is a CA Certficiate (honest)"},
	})
	c.Assert(apiconn.IsAPIError(errgo.Cause(err)), gc.Equals, true)
	c.Assert(err, gc.ErrorMatches, `api error: invalid cloud: empty auth-types not valid`)

	var docs []mongodoc.CloudRegion
	err = s.jem.DB.CloudRegions().Find(bson.D{{"cloud", "test-cloud"}}).Sort("_id").All(&docs)
	c.Assert(err, gc.Equals, nil)
	c.Assert(docs, jc.DeepEquals, []mongodoc.CloudRegion{})
}

func (s *cloudSuite) TestAddCloudNoHostCloudRegion(c *gc.C) {
	ctx := context.Background()

	ctlPath := params.EntityPath{"bob", "foo"}
	addController(c, ctlPath, s.APIInfo(c), s.jem)
	id := jemtest.NewIdentity("bob", "bob-group")
	err := s.jem.AddHostedCloud(ctx, id, "test-cloud", jujuparams.Cloud{
		Type:             "kubernetes",
		AuthTypes:        []string{"certificate"},
		Endpoint:         "https://1.2.3.4:5678",
		IdentityEndpoint: "https://1.2.3.4:5679",
		StorageEndpoint:  "https://1.2.3.4:5680",
		CACertificates:   []string{"This is a CA Certficiate (honest)"},
	})
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

	err := s.jem.RemoveCloud(testContext, jemtest.NewIdentity("bob", "bob-group"), "test-cloud")
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
	err := s.jem.GrantCloud(testContext, jemtest.NewIdentity("alice"), "test-cloud", "bob", "add-model")
	c.Assert(err, gc.Equals, nil)

	err = s.jem.RemoveCloud(testContext, jemtest.NewIdentity("bob", "bob-group"), "test-cloud")
	c.Assert(err, gc.ErrorMatches, `unauthorized`)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrUnauthorized)
}

func (s *cloudSuite) TestRemoveCloudNotFound(c *gc.C) {
	ctlPath := params.EntityPath{"bob", "foo"}
	addController(c, ctlPath, s.APIInfo(c), s.jem)

	err := s.jem.RemoveCloud(testContext, jemtest.NewIdentity("bob", "bob-group"), "test-cloud")
	c.Assert(err, gc.ErrorMatches, `cloudregion not found`)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)
}

func (s *cloudSuite) TestGrantCloud(c *gc.C) {
	ctlPath := params.EntityPath{"bob", "foo"}
	addController(c, ctlPath, s.APIInfo(c), s.jem)
	s.createK8sCloud(c, "test-cloud", "bob")

	err := s.jem.GrantCloud(testContext, jemtest.NewIdentity("bob", "bob-group"), "test-cloud", "alice", "admin")
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
	}, {
		Id:                   "test-cloud/default",
		Cloud:                "test-cloud",
		Region:               "default",
		ProviderType:         "kubernetes",
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

	err := s.jem.GrantCloud(testContext, jemtest.NewIdentity("alice"), "test-cloud", "alice", "admin")
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrUnauthorized)
	c.Assert(err, gc.ErrorMatches, `unauthorized`)
}

func (s *cloudSuite) TestGrantCloudNotFound(c *gc.C) {
	err := s.jem.GrantCloud(testContext, jemtest.NewIdentity("alice"), "test-cloud", "alice", "admin")
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)
	c.Assert(err, gc.ErrorMatches, `cloudregion not found`)
}

func (s *cloudSuite) TestGrantCloudInvalidAccess(c *gc.C) {
	ctlPath := params.EntityPath{"bob", "foo"}
	addController(c, ctlPath, s.APIInfo(c), s.jem)
	s.createK8sCloud(c, "test-cloud", "bob")

	err := s.jem.GrantCloud(testContext, jemtest.NewIdentity("bob", "bob-group"), "test-cloud", "alice", "not-valid")
	c.Assert(err, gc.ErrorMatches, `"not-valid" cloud access not valid`)
}

func (s *cloudSuite) TestRevokeCloud(c *gc.C) {
	ctlPath := params.EntityPath{"bob", "foo"}
	addController(c, ctlPath, s.APIInfo(c), s.jem)
	s.createK8sCloud(c, "test-cloud", "bob")

	bobID := jemtest.NewIdentity("bob", "bob-group")

	err := s.jem.GrantCloud(testContext, bobID, "test-cloud", "alice", "admin")
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
	}, {
		Id:                   "test-cloud/default",
		Cloud:                "test-cloud",
		Region:               "default",
		ProviderType:         "kubernetes",
		PrimaryControllers:   []params.EntityPath{{User: "bob", Name: "foo"}},
		SecondaryControllers: []params.EntityPath{},
		ACL: params.ACL{
			Read:  []string{"bob", "alice"},
			Write: []string{"bob", "alice"},
			Admin: []string{"bob", "alice"},
		},
	}})

	err = s.jem.RevokeCloud(testContext, bobID, "test-cloud", "alice", "add-model")
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
	}, {
		Id:                   "test-cloud/default",
		Cloud:                "test-cloud",
		Region:               "default",
		ProviderType:         "kubernetes",
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

	err := s.jem.GrantCloud(testContext, jemtest.NewIdentity("bob", "bob-group"), "test-cloud", "alice", "admin")
	c.Assert(err, gc.Equals, nil)

	err = s.jem.RevokeCloud(testContext, jemtest.NewIdentity("charlie"), "test-cloud", "alice", "admin")
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrUnauthorized)
	c.Assert(err, gc.ErrorMatches, `unauthorized`)
}

func (s *cloudSuite) TestRevokeCloudNotFound(c *gc.C) {
	err := s.jem.RevokeCloud(testContext, jemtest.NewIdentity("alice"), "test-cloud", "alice", "admin")
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)
	c.Assert(err, gc.ErrorMatches, `cloudregion not found`)
}

func (s *cloudSuite) TestRevokeCloudInvalidAccess(c *gc.C) {
	ctlPath := params.EntityPath{"bob", "foo"}
	addController(c, ctlPath, s.APIInfo(c), s.jem)
	s.createK8sCloud(c, "test-cloud", "bob")

	err := s.jem.RevokeCloud(testContext, jemtest.NewIdentity("bob", "bob-group"), "test-cloud", "alice", "not-valid")
	c.Assert(err, gc.ErrorMatches, `api error: "not-valid" cloud access not valid`)
}

func (s *cloudSuite) createK8sCloud(c *gc.C, name, owner string) {
	err := s.jem.AddHostedCloud(
		context.Background(),
		jemtest.NewIdentity(owner),
		params.Cloud(name),
		jujuparams.Cloud{
			Type:             "kubernetes",
			HostCloudRegion:  "dummy/dummy-region",
			AuthTypes:        []string{"certificate"},
			Endpoint:         "https://1.2.3.4:5678",
			IdentityEndpoint: "https://1.2.3.4:5679",
			StorageEndpoint:  "https://1.2.3.4:5680",
			CACertificates:   []string{"This is a CA Certficiate (honest)"},
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
	err := s.jem.AddHostedCloud(
		ctx,
		jemtest.NewIdentity("bob", "bob-group"),
		params.Cloud("test-cloud"),
		jujuparams.Cloud{
			Type:            "kubernetes",
			HostCloudRegion: "dummy/dummy-region",
			AuthTypes:       []string{string(cloud.UserPassAuthType)},
			Endpoint:        ksrv.URL,
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

	err = s.jem.RemoveCloud(testContext, jemtest.NewIdentity("bob", "bob-group"), "test-cloud")
	c.Assert(err, gc.ErrorMatches, `cloud is used by 1 model`)
}
