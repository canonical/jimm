// Copyright 2015 Canonical Ltd.

package jem_test

import (
	"context"

	jujuparams "github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cloud"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/errgo.v1"
	"gopkg.in/mgo.v2/bson"

	"github.com/CanonicalLtd/jimm/internal/apiconn"
	"github.com/CanonicalLtd/jimm/internal/jem"
	"github.com/CanonicalLtd/jimm/internal/jemtest"
	"github.com/CanonicalLtd/jimm/internal/kubetest"
	"github.com/CanonicalLtd/jimm/internal/mongodoc"
	"github.com/CanonicalLtd/jimm/params"
)

type cloudSuite struct {
	jemtest.BootstrapSuite

	KubernetesCloud jujuparams.Cloud
}

var _ = gc.Suite(&cloudSuite{})

func (s *cloudSuite) SetUpTest(c *gc.C) {
	s.BootstrapSuite.SetUpTest(c)
	s.KubernetesCloud = jujuparams.Cloud{
		Type:             "kubernetes",
		HostCloudRegion:  "dummy/dummy-region",
		AuthTypes:        []string{"certificate"},
		Endpoint:         "https://1.2.3.4:5678",
		IdentityEndpoint: "https://1.2.3.4:5679",
		StorageEndpoint:  "https://1.2.3.4:5680",
		CACertificates:   []string{"This is a CA Certficiate (honest)"},
	}
	err := s.JEM.AddHostedCloud(context.TODO(), jemtest.Alice, "k8s-cloud", s.KubernetesCloud)
	c.Assert(err, gc.Equals, nil)
}

func (s *cloudSuite) TestAddCloud(c *gc.C) {
	ctx := context.Background()

	err := s.JEM.AddHostedCloud(ctx, jemtest.Bob, "test-cloud", jujuparams.Cloud{
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
	err = s.JEM.DB.CloudRegions().Find(bson.D{{"cloud", "test-cloud"}}).Sort("_id").All(&docs)
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
		PrimaryControllers:   []params.EntityPath{s.Controller.Path},
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
		PrimaryControllers:   []params.EntityPath{s.Controller.Path},
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
	err := s.JEM.AddHostedCloud(ctx, jemtest.Bob, "test-cloud", jujuparams.Cloud{
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
	err = s.JEM.DB.CloudRegions().Find(bson.D{{"cloud", "test-cloud"}}).Sort("_id").All(&docs)
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
		PrimaryControllers:   []params.EntityPath{s.Controller.Path},
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
		PrimaryControllers:   []params.EntityPath{s.Controller.Path},
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

	err := s.JEM.AddHostedCloud(ctx, jemtest.Bob, "dummy", jujuparams.Cloud{
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

	err := s.JEM.AddHostedCloud(ctx, jemtest.Bob, "aws-china", jujuparams.Cloud{
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

	err := s.JEM.AddHostedCloud(ctx, jemtest.Bob, "test-cloud", jujuparams.Cloud{
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
	err = s.JEM.DB.CloudRegions().Find(bson.D{{"cloud", "test-cloud"}}).Sort("_id").All(&docs)
	c.Assert(err, gc.Equals, nil)
	c.Assert(docs, jc.DeepEquals, []mongodoc.CloudRegion{})
}

func (s *cloudSuite) TestAddCloudControllerError(c *gc.C) {
	ctx := context.Background()

	err := s.JEM.AddHostedCloud(ctx, jemtest.Bob, "test-cloud", jujuparams.Cloud{
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
	err = s.JEM.DB.CloudRegions().Find(bson.D{{"cloud", "test-cloud"}}).Sort("_id").All(&docs)
	c.Assert(err, gc.Equals, nil)
	c.Assert(docs, jc.DeepEquals, []mongodoc.CloudRegion{})
}

func (s *cloudSuite) TestAddCloudNoHostCloudRegion(c *gc.C) {
	ctx := context.Background()

	err := s.JEM.AddHostedCloud(ctx, jemtest.Bob, "test-cloud", jujuparams.Cloud{
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
	err = s.JEM.DB.CloudRegions().Find(bson.D{{"cloud", "test-cloud"}}).Sort("_id").All(&docs)
	c.Assert(err, gc.Equals, nil)
	c.Assert(docs, jc.DeepEquals, []mongodoc.CloudRegion{})
}

func (s *cloudSuite) TestRemoveCloud(c *gc.C) {
	err := s.JEM.RemoveCloud(testContext, jemtest.Alice, "k8s-cloud")
	c.Assert(err, gc.Equals, nil)

	var docs []mongodoc.CloudRegion
	err = s.JEM.DB.CloudRegions().Find(bson.D{{"cloud", "k8s-cloud"}}).Sort("_id").All(&docs)
	c.Assert(err, gc.Equals, nil)
	c.Assert(docs, jc.DeepEquals, []mongodoc.CloudRegion{})
}

func (s *cloudSuite) TestRemoveCloudUnauthorized(c *gc.C) {
	err := s.JEM.GrantCloud(testContext, jemtest.Alice, "k8s-cloud", "bob", "add-model")
	c.Assert(err, gc.Equals, nil)

	err = s.JEM.RemoveCloud(testContext, jemtest.Bob, "k8s-cloud")
	c.Assert(err, gc.ErrorMatches, `unauthorized`)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrUnauthorized)
}

func (s *cloudSuite) TestRemoveCloudNotFound(c *gc.C) {
	err := s.JEM.RemoveCloud(testContext, jemtest.Bob, "test-cloud")
	c.Assert(err, gc.ErrorMatches, `cloudregion not found`)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)
}

func (s *cloudSuite) TestGrantCloud(c *gc.C) {
	err := s.JEM.GrantCloud(testContext, jemtest.Alice, "k8s-cloud", "bob", "admin")
	c.Assert(err, gc.Equals, nil)

	var docs []mongodoc.CloudRegion
	err = s.JEM.DB.CloudRegions().Find(bson.D{{"cloud", "k8s-cloud"}}).Sort("_id").All(&docs)
	c.Assert(err, gc.Equals, nil)
	c.Assert(docs, jc.DeepEquals, []mongodoc.CloudRegion{{
		Id:                   "k8s-cloud/",
		Cloud:                "k8s-cloud",
		ProviderType:         "kubernetes",
		AuthTypes:            []string{"certificate"},
		Endpoint:             "https://1.2.3.4:5678",
		IdentityEndpoint:     "https://1.2.3.4:5679",
		StorageEndpoint:      "https://1.2.3.4:5680",
		CACertificates:       []string{"This is a CA Certficiate (honest)"},
		PrimaryControllers:   []params.EntityPath{s.Controller.Path},
		SecondaryControllers: []params.EntityPath{},
		ACL: params.ACL{
			Read:  []string{"alice", "bob"},
			Write: []string{"alice", "bob"},
			Admin: []string{"alice", "bob"},
		},
	}, {
		Id:                   "k8s-cloud/default",
		Cloud:                "k8s-cloud",
		Region:               "default",
		ProviderType:         "kubernetes",
		PrimaryControllers:   []params.EntityPath{s.Controller.Path},
		SecondaryControllers: []params.EntityPath{},
		ACL: params.ACL{
			Read:  []string{"alice", "bob"},
			Write: []string{"alice", "bob"},
			Admin: []string{"alice", "bob"},
		},
	}})
}

func (s *cloudSuite) TestGrantCloudUnauthorized(c *gc.C) {
	err := s.JEM.GrantCloud(testContext, jemtest.Charlie, "k8s-cloud", "alice", "admin")
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrUnauthorized)
	c.Assert(err, gc.ErrorMatches, `unauthorized`)
}

func (s *cloudSuite) TestGrantCloudNotFound(c *gc.C) {
	err := s.JEM.GrantCloud(testContext, jemtest.Charlie, "test-cloud", "alice", "admin")
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)
	c.Assert(err, gc.ErrorMatches, `cloudregion not found`)
}

func (s *cloudSuite) TestGrantCloudInvalidAccess(c *gc.C) {
	err := s.JEM.GrantCloud(testContext, jemtest.Alice, "k8s-cloud", "bob", "not-valid")
	c.Assert(err, gc.ErrorMatches, `"not-valid" cloud access not valid`)
}

func (s *cloudSuite) TestRevokeCloud(c *gc.C) {
	err := s.JEM.GrantCloud(testContext, jemtest.Alice, "k8s-cloud", "bob", "admin")
	c.Assert(err, gc.Equals, nil)

	var docs []mongodoc.CloudRegion
	err = s.JEM.DB.CloudRegions().Find(bson.D{{"cloud", "k8s-cloud"}}).Sort("_id").All(&docs)
	c.Assert(err, gc.Equals, nil)
	c.Assert(docs, jc.DeepEquals, []mongodoc.CloudRegion{{
		Id:                   "k8s-cloud/",
		Cloud:                "k8s-cloud",
		ProviderType:         "kubernetes",
		AuthTypes:            []string{"certificate"},
		Endpoint:             "https://1.2.3.4:5678",
		IdentityEndpoint:     "https://1.2.3.4:5679",
		StorageEndpoint:      "https://1.2.3.4:5680",
		CACertificates:       []string{"This is a CA Certficiate (honest)"},
		PrimaryControllers:   []params.EntityPath{s.Controller.Path},
		SecondaryControllers: []params.EntityPath{},
		ACL: params.ACL{
			Read:  []string{"alice", "bob"},
			Write: []string{"alice", "bob"},
			Admin: []string{"alice", "bob"},
		},
	}, {
		Id:                   "k8s-cloud/default",
		Cloud:                "k8s-cloud",
		Region:               "default",
		ProviderType:         "kubernetes",
		PrimaryControllers:   []params.EntityPath{s.Controller.Path},
		SecondaryControllers: []params.EntityPath{},
		ACL: params.ACL{
			Read:  []string{"alice", "bob"},
			Write: []string{"alice", "bob"},
			Admin: []string{"alice", "bob"},
		},
	}})

	err = s.JEM.RevokeCloud(testContext, jemtest.Alice, "k8s-cloud", "bob", "add-model")
	c.Assert(err, gc.Equals, nil)

	err = s.JEM.DB.CloudRegions().Find(bson.D{{"cloud", "k8s-cloud"}}).Sort("_id").All(&docs)
	c.Assert(err, gc.Equals, nil)
	c.Assert(docs, jc.DeepEquals, []mongodoc.CloudRegion{{
		Id:                   "k8s-cloud/",
		Cloud:                "k8s-cloud",
		ProviderType:         "kubernetes",
		AuthTypes:            []string{"certificate"},
		Endpoint:             "https://1.2.3.4:5678",
		IdentityEndpoint:     "https://1.2.3.4:5679",
		StorageEndpoint:      "https://1.2.3.4:5680",
		CACertificates:       []string{"This is a CA Certficiate (honest)"},
		PrimaryControllers:   []params.EntityPath{s.Controller.Path},
		SecondaryControllers: []params.EntityPath{},
		ACL: params.ACL{
			Read:  []string{"alice"},
			Write: []string{"alice"},
			Admin: []string{"alice"},
		},
	}, {
		Id:                   "k8s-cloud/default",
		Cloud:                "k8s-cloud",
		Region:               "default",
		ProviderType:         "kubernetes",
		PrimaryControllers:   []params.EntityPath{s.Controller.Path},
		SecondaryControllers: []params.EntityPath{},
		ACL: params.ACL{
			Read:  []string{"alice"},
			Write: []string{"alice"},
			Admin: []string{"alice"},
		},
	}})
}

func (s *cloudSuite) TestRevokeCloudUnauthorized(c *gc.C) {
	err := s.JEM.GrantCloud(testContext, jemtest.Alice, "k8s-cloud", "bob", "admin")
	c.Assert(err, gc.Equals, nil)

	err = s.JEM.RevokeCloud(testContext, jemtest.Charlie, "k8s-cloud", "bob", "admin")
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrUnauthorized)
	c.Assert(err, gc.ErrorMatches, `unauthorized`)
}

func (s *cloudSuite) TestRevokeCloudNotFound(c *gc.C) {
	err := s.JEM.RevokeCloud(testContext, jemtest.Alice, "test-cloud", "bob", "admin")
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)
	c.Assert(err, gc.ErrorMatches, `cloudregion not found`)
}

func (s *cloudSuite) TestRevokeCloudInvalidAccess(c *gc.C) {
	err := s.JEM.RevokeCloud(testContext, jemtest.Alice, "k8s-cloud", "bob", "not-valid")
	c.Assert(err, gc.ErrorMatches, `"not-valid" cloud access not valid`)
}

func (s *cloudSuite) TestRemoveCloudWithModel(c *gc.C) {
	ksrv := kubetest.NewFakeKubernetes(c)
	defer ksrv.Close()

	ctlPath := params.EntityPath{"bob", "foo"}
	addController(c, ctlPath, s.APIInfo(c), s.JEM)

	id := jemtest.NewIdentity("bob", "bob-group")
	err := s.JEM.AddHostedCloud(
		testContext,
		id,
		params.Cloud("test-cloud"),
		jujuparams.Cloud{
			Type:            "kubernetes",
			HostCloudRegion: "dummy/dummy-region",
			AuthTypes:       []string{string(cloud.UserPassAuthType)},
			Endpoint:        ksrv.URL,
		},
	)
	c.Assert(err, gc.Equals, nil)

	credpath := mongodoc.CredentialPath{
		Cloud: "test-cloud",
		EntityPath: mongodoc.EntityPath{
			User: "bob",
			Name: "kubernetes",
		},
	}
	_, err = s.JEM.UpdateCredential(testContext, id, &mongodoc.Credential{
		Path: credpath,
		Type: string(cloud.UserPassAuthType),
		Attributes: map[string]string{
			"username": kubetest.Username,
			"password": kubetest.Password,
		},
	}, jem.CredentialUpdate)
	c.Assert(err, gc.Equals, nil)

	err = s.JEM.CreateModel(testContext, id, jem.CreateModelParams{
		Path:       params.EntityPath{"bob", "test-model"},
		Cloud:      "test-cloud",
		Credential: credpath,
	}, nil)
	c.Assert(err, gc.Equals, nil)

	err = s.JEM.RemoveCloud(testContext, id, "test-cloud")
	c.Assert(err, gc.ErrorMatches, `cloud is used by 1 model`)
}

func (s *cloudSuite) TestGetCloud(c *gc.C) {
	cloud := jem.Cloud{
		Name: "dummy",
	}

	err := s.JEM.GetCloud(testContext, jemtest.Bob, &cloud)
	c.Assert(err, gc.Equals, nil)
	c.Check(cloud, jc.DeepEquals, jem.Cloud{
		Name: "dummy",
		Cloud: jujuparams.Cloud{
			Type:             "dummy",
			AuthTypes:        []string{"empty", "userpass"},
			Endpoint:         "dummy-endpoint",
			IdentityEndpoint: "dummy-identity-endpoint",
			StorageEndpoint:  "dummy-storage-endpoint",
			Regions: []jujuparams.CloudRegion{{
				Name:             "dummy-region",
				Endpoint:         "dummy-endpoint",
				IdentityEndpoint: "dummy-identity-endpoint",
				StorageEndpoint:  "dummy-storage-endpoint",
			}},
		},
	})

	cloud = jem.Cloud{
		Name: "not-dummy",
	}
	err = s.JEM.GetCloud(testContext, jemtest.Bob, &cloud)
	c.Assert(err, gc.ErrorMatches, `cloud not found`)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)

	cr := mongodoc.CloudRegion{
		Cloud: "dummy-2",
		ACL: params.ACL{
			Read: []string{"alice"},
		},
	}
	err = s.JEM.DB.InsertCloudRegion(testContext, &cr)
	c.Assert(err, gc.Equals, nil)

	cloud = jem.Cloud{
		Name: "dummy-2",
	}
	err = s.JEM.GetCloud(testContext, jemtest.Bob, &cloud)
	c.Assert(err, gc.ErrorMatches, `unauthorized`)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrUnauthorized)
}

func (s *cloudSuite) TestForEachCloud(c *gc.C) {
	cr := mongodoc.CloudRegion{
		Cloud: "dummy-2",
		ACL: params.ACL{
			Read: []string{"alice"},
		},
	}
	err := s.JEM.DB.InsertCloudRegion(testContext, &cr)
	c.Assert(err, gc.Equals, nil)

	cloud := jem.Cloud{
		Name: "dummy",
	}
	err = s.JEM.GetCloud(testContext, jemtest.Bob, &cloud)
	c.Assert(err, gc.Equals, nil)

	expect := []jem.Cloud{cloud}
	err = s.JEM.ForEachCloud(testContext, jemtest.Bob, func(cld *jem.Cloud) error {
		if len(expect) < 1 {
			return errgo.Newf("unexpected cloud %q", cld.Name)
		}
		c.Check(*cld, jc.DeepEquals, expect[0])
		expect = expect[1:]
		return nil
	})
	c.Assert(err, gc.Equals, nil)

	testError := errgo.New("test")
	err = s.JEM.ForEachCloud(testContext, jemtest.Bob, func(cld *jem.Cloud) error {
		return testError
	})
	c.Check(err, gc.Equals, testError)
}
