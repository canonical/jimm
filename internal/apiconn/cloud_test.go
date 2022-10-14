package apiconn_test

import (
	"context"

	"github.com/juju/juju/api"
	jujuparams "github.com/juju/juju/apiserver/params"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/CanonicalLtd/jimm/internal/apiconn"
	"github.com/CanonicalLtd/jimm/internal/jemtest"
	"github.com/CanonicalLtd/jimm/internal/mongodoc"
	"github.com/CanonicalLtd/jimm/params"
)

type cloudSuite struct {
	jemtest.JujuConnSuite

	cache *apiconn.Cache
	conn  *apiconn.Conn
}

var _ = gc.Suite(&cloudSuite{})

func (s *cloudSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)

	s.cache = apiconn.NewCache(apiconn.CacheParams{})

	var err error
	s.conn, err = s.cache.OpenAPI(context.Background(), s.ControllerConfig.ControllerUUID(), func() (api.Connection, *api.Info, error) {
		apiInfo := s.APIInfo(c)
		return apiOpen(
			&api.Info{
				Addrs:    apiInfo.Addrs,
				CACert:   apiInfo.CACert,
				Tag:      apiInfo.Tag,
				Password: apiInfo.Password,
			},
			api.DialOpts{},
		)
	})
	c.Assert(err, gc.Equals, nil)
}

func (s *cloudSuite) TearDownTest(c *gc.C) {
	if s.conn != nil {
		s.conn.Close()
	}
	if s.cache != nil {
		s.cache.Close()
	}
	s.JujuConnSuite.TearDownTest(c)
}

func (s *cloudSuite) TestSupportsCheckCredentialsModels(c *gc.C) {
	c.Assert(s.conn.SupportsCheckCredentialModels(), gc.Equals, true)
}

func (s *cloudSuite) TestCheckCredentialModels(c *gc.C) {
	cred := &mongodoc.Credential{
		Path: mongodoc.CredentialPath{
			Cloud: "dummy",
			EntityPath: mongodoc.EntityPath{
				User: "admin",
				Name: "pw1",
			},
		},
		Type: "userpass",
		Attributes: map[string]string{
			"username": "alibaba",
			"password": "open sesame",
		},
	}

	models, err := s.conn.CheckCredentialModels(context.Background(), cred)
	c.Assert(err, gc.Equals, nil)
	c.Assert(models, gc.HasLen, 0)
}

func (s *cloudSuite) TestUpdateCredential(c *gc.C) {
	cred := &mongodoc.Credential{
		Path: mongodoc.CredentialPath{
			Cloud: "dummy",
			EntityPath: mongodoc.EntityPath{
				User: "admin",
				Name: "pw1",
			},
		},
		Type: "userpass",
		Attributes: map[string]string{
			"username": "alibaba",
			"password": "open sesame",
		},
	}

	models, err := s.conn.UpdateCredential(context.Background(), cred)
	c.Assert(err, gc.Equals, nil)
	c.Assert(models, gc.HasLen, 0)

	cred.Type = "bad-type"

	models, err = s.conn.UpdateCredential(context.Background(), cred)
	c.Assert(err, gc.ErrorMatches, `api error: updating cloud credentials: validating credential "dummy/admin@external/pw1" for cloud "dummy": supported auth-types \["empty" "userpass"\], "bad-type" not supported`)
	c.Assert(models, gc.HasLen, 0)
}

func (s *cloudSuite) TestRevokeCredential(c *gc.C) {
	cred := &mongodoc.Credential{
		Path: mongodoc.CredentialPath{
			Cloud: "dummy",
			EntityPath: mongodoc.EntityPath{
				User: "admin",
				Name: "pw1",
			},
		},
		Type: "userpass",
		Attributes: map[string]string{
			"username": "alibaba",
			"password": "open sesame",
		},
	}

	models, err := s.conn.UpdateCredential(context.Background(), cred)
	c.Assert(err, gc.Equals, nil)
	c.Assert(models, gc.HasLen, 0)

	err = s.conn.RevokeCredential(context.Background(), cred.Path)
	c.Assert(err, gc.Equals, nil)
}

func (s *cloudSuite) TestClouds(c *gc.C) {
	clouds, err := s.conn.Clouds(context.Background())
	c.Assert(err, gc.Equals, nil)
	c.Assert(clouds, jc.DeepEquals, map[params.Cloud]jujuparams.Cloud{
		"dummy": jujuparams.Cloud{
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
}

func (s *cloudSuite) TestCloud(c *gc.C) {
	ctx := context.Background()

	clouds, err := s.conn.Clouds(ctx)
	c.Assert(err, gc.Equals, nil)

	var cloud jujuparams.Cloud
	err = s.conn.Cloud(ctx, "dummy", &cloud)
	c.Assert(err, gc.Equals, nil)

	c.Check(cloud, jc.DeepEquals, clouds["dummy"])
}

func (s *cloudSuite) TestAddCloud(c *gc.C) {
	cloud := jujuparams.Cloud{
		Type:      "kubernetes",
		AuthTypes: []string{"empty"},
	}

	ctx := context.Background()

	err := s.conn.AddCloud(ctx, "dummy", cloud)
	c.Assert(jujuparams.ErrCode(err), gc.Equals, jujuparams.CodeAlreadyExists)

	err = s.conn.AddCloud(ctx, "test-cloud", cloud)
	c.Assert(err, gc.Equals, nil)

	clouds, err := s.conn.Clouds(ctx)
	c.Assert(err, gc.Equals, nil)

	c.Check(clouds["test-cloud"], jc.DeepEquals, jujuparams.Cloud{
		Type:      "kubernetes",
		AuthTypes: []string{"empty"},
		Regions: []jujuparams.CloudRegion{{
			Name: "default",
		}},
	})
}

func (s *cloudSuite) TestRemoveCloud(c *gc.C) {
	cloud := jujuparams.Cloud{
		Type:      "kubernetes",
		AuthTypes: []string{"empty"},
	}

	ctx := context.Background()

	err := s.conn.RemoveCloud(ctx, "test-cloud")
	c.Check(jujuparams.ErrCode(err), gc.Equals, jujuparams.CodeNotFound)

	err = s.conn.AddCloud(ctx, "test-cloud", cloud)
	c.Assert(err, gc.Equals, nil)

	clouds, err := s.conn.Clouds(ctx)
	c.Assert(err, gc.Equals, nil)

	c.Assert(clouds["test-cloud"], jc.DeepEquals, jujuparams.Cloud{
		Type:      "kubernetes",
		AuthTypes: []string{"empty"},
		Regions: []jujuparams.CloudRegion{{
			Name: "default",
		}},
	})

	err = s.conn.RemoveCloud(ctx, "test-cloud")
	c.Assert(err, gc.Equals, nil)

	clouds, err = s.conn.Clouds(ctx)
	c.Assert(err, gc.Equals, nil)

	_, ok := clouds["test-cloud"]
	c.Assert(ok, gc.Equals, false)
}

func (s *cloudSuite) TestGrantCloudAccess(c *gc.C) {
	err := s.conn.GrantCloudAccess(context.Background(), "no-such-cloud", "user", "add-model")
	c.Check(jujuparams.ErrCode(err), gc.Equals, jujuparams.CodeNotFound)
	err = s.conn.GrantCloudAccess(context.Background(), "dummy", "user", "add-model")
	c.Check(err, gc.Equals, nil)
}

func (s *cloudSuite) TestRevokeCloudAccess(c *gc.C) {
	err := s.conn.RevokeCloudAccess(context.Background(), "no-such-cloud", "user", "add-model")
	c.Check(jujuparams.ErrCode(err), gc.Equals, jujuparams.CodeNotFound)
	err = s.conn.GrantCloudAccess(context.Background(), "dummy", "user", "admin")
	c.Assert(err, gc.Equals, nil)
	err = s.conn.RevokeCloudAccess(context.Background(), "dummy", "user", "admin")
	c.Check(err, gc.Equals, nil)
	err = s.conn.RevokeCloudAccess(context.Background(), "dummy", "user", "add-model")
	c.Check(err, gc.Equals, nil)
	err = s.conn.RevokeCloudAccess(context.Background(), "dummy", "user", "add-model")
	c.Check(jujuparams.ErrCode(err), gc.Equals, jujuparams.CodeNotFound)
}
