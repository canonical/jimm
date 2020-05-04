package apiconn_test

import (
	"context"

	"github.com/juju/juju/api"
	gc "gopkg.in/check.v1"

	"github.com/CanonicalLtd/jimm/internal/apiconn"
	"github.com/CanonicalLtd/jimm/internal/jemtest"
	"github.com/CanonicalLtd/jimm/internal/mongodoc"
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
