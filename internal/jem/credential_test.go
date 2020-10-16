// Copyright 2015 Canonical Ltd.

package jem_test

import (
	"context"

	"github.com/CanonicalLtd/jimm/internal/conv"
	"github.com/CanonicalLtd/jimm/internal/jem"
	"github.com/CanonicalLtd/jimm/internal/jemtest"
	"github.com/CanonicalLtd/jimm/internal/mgosession"
	"github.com/CanonicalLtd/jimm/internal/mongodoc"
	"github.com/CanonicalLtd/jimm/internal/pubsub"
	"github.com/CanonicalLtd/jimm/params"
	cloudapi "github.com/juju/juju/api/cloud"
	jujuparams "github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cloud"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"
	"gopkg.in/errgo.v1"
)

type credentialSuite struct {
	jemtest.JujuConnSuite
	pool                           *jem.Pool
	sessionPool                    *mgosession.Pool
	jem                            *jem.JEM
	usageSenderAuthorizationClient *testUsageSenderAuthorizationClient
}

var _ = gc.Suite(&credentialSuite{})

func (s *credentialSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.sessionPool = mgosession.NewPool(context.TODO(), s.Session, 5)
	publicCloudMetadata, _, err := cloud.PublicCloudMetadata()
	c.Assert(err, gc.Equals, nil)
	s.usageSenderAuthorizationClient = &testUsageSenderAuthorizationClient{}
	pool, err := jem.NewPool(context.TODO(), jem.Params{
		DB:                             s.Session.DB("jem"),
		ControllerAdmin:                "controller-admin",
		SessionPool:                    s.sessionPool,
		PublicCloudMetadata:            publicCloudMetadata,
		UsageSenderAuthorizationClient: s.usageSenderAuthorizationClient,
		Pubsub: &pubsub.Hub{
			MaxConcurrency: 10,
		},
	})
	c.Assert(err, gc.Equals, nil)
	s.pool = pool
	s.jem = s.pool.JEM(context.TODO())
	s.PatchValue(&utils.OutgoingAccessAllowed, true)
}

func (s *credentialSuite) TearDownTest(c *gc.C) {
	s.jem.Close()
	s.pool.Close()
	s.sessionPool.Close()
	s.JujuConnSuite.TearDownTest(c)
}

func (s *credentialSuite) TestRevokeCredentialsInUse(c *gc.C) {
	ctlId := s.addController(c, params.EntityPath{"bob", "controller"})
	err := s.jem.DB.SetACL(testContext, s.jem.DB.Controllers(), ctlId, params.ACL{
		Read: []string{"everyone"},
	})
	c.Assert(err, gc.Equals, nil)
	credPath := credentialPath("dummy", "bob", "cred1")
	err = s.jem.DB.UpsertCredential(testContext, &mongodoc.Credential{
		Path: mongodoc.CredentialPathFromParams(credPath),
		Type: "empty",
	})
	c.Assert(err, gc.Equals, nil)

	id := jemtest.NewIdentity("bob")
	err = s.jem.CreateModel(testContext, id, jem.CreateModelParams{
		Path:           params.EntityPath{"bob", "oldmodel"},
		ControllerPath: ctlId,
		Credential:     credPath,
		Cloud:          "dummy",
	}, nil)
	c.Assert(err, gc.Equals, nil)
	err = s.jem.RevokeCredential(testContext, credPath, 0)
	c.Assert(err, gc.ErrorMatches, `cannot revoke because credential is in use on at least one model`)

	// Try with just the check.
	err = s.jem.RevokeCredential(testContext, credPath, jem.CredentialCheck)
	c.Assert(err, gc.ErrorMatches, `cannot revoke because credential is in use on at least one model`)

	// Try without the check. It should succeed.
	err = s.jem.RevokeCredential(testContext, credPath, jem.CredentialUpdate)
	c.Assert(err, gc.Equals, nil)

	// Try to create another with the credentials that have
	// been revoked. We should fail to do that.
	err = s.jem.CreateModel(testContext, id, jem.CreateModelParams{
		Path:           params.EntityPath{"bob", "newmodel"},
		ControllerPath: ctlId,
		Credential:     credPath,
		Cloud:          "dummy",
	}, nil)
	c.Assert(err, gc.ErrorMatches, `credential dummy/bob/cred1 has been revoked`)

	// Check that the credential really has been revoked on the
	// controller.
	conn, err := s.jem.OpenAPI(testContext, ctlId)
	c.Assert(err, gc.Equals, nil)
	defer conn.Close()
	r, err := cloudapi.NewClient(conn).Credentials(conv.ToCloudCredentialTag(credPath))
	c.Assert(err, gc.Equals, nil)
	c.Assert(r, jc.DeepEquals, []jujuparams.CloudCredentialResult{{
		Error: &jujuparams.Error{
			Message: `credential "cred1" not found`,
			Code:    "not found",
		},
	}})
}

func (s *credentialSuite) TestRevokeCredentialsNotInUse(c *gc.C) {
	ctlId := s.addController(c, params.EntityPath{"bob", "controller"})
	err := s.jem.DB.SetACL(testContext, s.jem.DB.Controllers(), ctlId, params.ACL{
		Read: []string{"everyone"},
	})
	c.Assert(err, gc.Equals, nil)
	credPath := credentialPath("dummy", "bob", "cred1")
	mCredPath := mgoCredentialPath("dummy", "bob", "cred1")
	err = s.jem.DB.UpsertCredential(testContext, &mongodoc.Credential{
		Path: mCredPath,
		Type: "empty",
	})
	c.Assert(err, gc.Equals, nil)

	// Sanity check that we can get the credential.
	err = s.jem.DB.GetCredential(testContext, &mongodoc.Credential{Path: mCredPath})
	c.Assert(err, gc.Equals, nil)

	// Try with just the check.
	err = s.jem.RevokeCredential(testContext, credPath, jem.CredentialCheck)
	c.Assert(err, gc.Equals, nil)

	// Check that the credential is still there.
	err = s.jem.DB.GetCredential(testContext, &mongodoc.Credential{Path: mCredPath})
	c.Assert(err, gc.Equals, nil)

	// Try with both the check and the update flag.
	err = s.jem.RevokeCredential(testContext, credPath, 0)
	c.Assert(err, gc.Equals, nil)

	// The credential should be marked as revoked and all
	// the details should be cleater.
	cred := mongodoc.Credential{
		Path: mCredPath,
	}
	err = s.jem.DB.GetCredential(testContext, &cred)
	c.Assert(err, gc.Equals, nil)
	c.Assert(cred, jc.DeepEquals, mongodoc.Credential{
		Id:         "dummy/bob/cred1",
		Path:       mCredPath,
		Revoked:    true,
		Attributes: make(map[string]string),
	})
}

func (s *credentialSuite) TestUpdateCredential(c *gc.C) {
	ctlPath := s.addController(c, params.EntityPath{User: "bob", Name: "controller"})
	credPath := credentialPath("dummy", "bob", "cred")
	mCredPath := mgoCredentialPath("dummy", "bob", "cred")
	cred := &mongodoc.Credential{
		Path: mCredPath,
		Type: "empty",
	}
	err := s.jem.DB.UpsertCredential(testContext, cred)
	c.Assert(err, gc.Equals, nil)
	conn, err := s.jem.OpenAPI(testContext, ctlPath)
	c.Assert(err, gc.Equals, nil)
	defer conn.Close()

	_, err = jem.UpdateControllerCredential(s.jem, testContext, conn, ctlPath, cred)
	c.Assert(err, gc.Equals, nil)
	err = jem.CredentialAddController(s.jem, testContext, cred, ctlPath)
	c.Assert(err, gc.Equals, nil)

	// Sanity check it was deployed
	client := cloudapi.NewClient(conn)
	credTag := names.NewCloudCredentialTag("dummy/bob@external/cred")
	creds, err := client.Credentials(credTag)
	c.Assert(err, gc.Equals, nil)
	c.Assert(creds, jc.DeepEquals, []jujuparams.CloudCredentialResult{{
		Result: &jujuparams.CloudCredential{
			AuthType: "empty",
		},
	}})

	_, err = s.jem.UpdateCredential(testContext, &mongodoc.Credential{
		Path: mCredPath,
		Type: "userpass",
		Attributes: map[string]string{
			"username": "cloud-user",
			"password": "cloud-pass",
		},
	}, jem.CredentialUpdate)
	c.Assert(err, gc.Equals, nil)

	// check it was updated on the controller.
	creds, err = client.Credentials(credTag)
	c.Assert(err, gc.Equals, nil)
	c.Assert(creds, jc.DeepEquals, []jujuparams.CloudCredentialResult{{
		Result: &jujuparams.CloudCredential{
			AuthType: "userpass",
			Attributes: map[string]string{
				"username": "cloud-user",
			},
			Redacted: []string{
				"password",
			},
		},
	}})

	// Revoke the credential
	err = s.jem.RevokeCredential(testContext, credPath, jem.CredentialUpdate)
	c.Assert(err, gc.Equals, nil)

	// check it was removed on the controller.
	creds, err = client.Credentials(credTag)
	c.Assert(err, gc.Equals, nil)
	c.Assert(creds, jc.DeepEquals, []jujuparams.CloudCredentialResult{{
		Error: &jujuparams.Error{
			Code:    "not found",
			Message: `credential "cred" not found`,
		},
	}})
}

var credentialTests = []struct {
	path             params.CredentialPath
	expectErrorCause error
}{{
	path: params.CredentialPath{
		Cloud: "dummy",
		User:  "bob",
		Name:  "credential",
	},
}, {
	path: params.CredentialPath{
		Cloud: "dummy",
		User:  "bob-group",
		Name:  "credential",
	},
}, {
	path: params.CredentialPath{
		Cloud: "dummy",
		User:  "alice",
		Name:  "credential",
	},
	expectErrorCause: params.ErrUnauthorized,
}, {
	path: params.CredentialPath{
		Cloud: "dummy",
		User:  "bob",
		Name:  "credential2",
	},
	expectErrorCause: params.ErrNotFound,
}, {
	path: params.CredentialPath{
		Cloud: "dummy",
		User:  "bob-group",
		Name:  "credential2",
	},
	expectErrorCause: params.ErrNotFound,
}, {
	path: params.CredentialPath{
		Cloud: "dummy",
		User:  "alice",
		Name:  "credential2",
	},
	expectErrorCause: params.ErrUnauthorized,
}}

func (s *credentialSuite) TestCredential(c *gc.C) {
	creds := []mongodoc.Credential{{
		Path: mongodoc.CredentialPath{
			Cloud: "dummy",
			EntityPath: mongodoc.EntityPath{
				User: "alice",
				Name: "credential",
			},
		},
	}, {
		Path: mongodoc.CredentialPath{
			Cloud: "dummy",
			EntityPath: mongodoc.EntityPath{
				User: "bob",
				Name: "credential",
			},
		},
	}, {
		Path: mongodoc.CredentialPath{
			Cloud: "dummy",
			EntityPath: mongodoc.EntityPath{
				User: "bob-group",
				Name: "credential",
			},
		},
	}}
	for _, cred := range creds {
		cred.Id = cred.Path.String()
		err := s.jem.DB.UpsertCredential(testContext, &cred)
		c.Assert(err, gc.Equals, nil)
	}
	for i, test := range credentialTests {
		c.Logf("test %d. %s", i, test.path)
		ctl := mongodoc.Credential{
			Path: mongodoc.CredentialPathFromParams(test.path),
		}
		err := s.jem.GetCredential(context.Background(), jemtest.NewIdentity("bob", "bob-group"), &ctl)
		if test.expectErrorCause != nil {
			c.Assert(errgo.Cause(err), gc.Equals, test.expectErrorCause)
			continue
		}
		c.Assert(err, gc.Equals, nil)
		c.Assert(ctl.Path.ToParams(), jc.DeepEquals, test.path)
	}
}

func credentialPath(cloud, user, name string) params.CredentialPath {
	return params.CredentialPath{
		Cloud: params.Cloud(cloud),
		User:  params.User(user),
		Name:  params.CredentialName(name),
	}
}

func mgoCredentialPath(cloud, user, name string) mongodoc.CredentialPath {
	return mongodoc.CredentialPath{
		Cloud: cloud,
		EntityPath: mongodoc.EntityPath{
			User: user,
			Name: name,
		},
	}
}

func (s *credentialSuite) TestCredentialAddController(c *gc.C) {
	path := credentialPath("test-cloud", "test-user", "test-credential")
	mpath := mongodoc.CredentialPathFromParams(path)
	expectId := path.String()
	err := s.jem.DB.UpsertCredential(testContext, &mongodoc.Credential{
		Path: mpath,
		Type: "empty",
	})
	c.Assert(err, gc.Equals, nil)

	ctlPath := params.EntityPath{"bob", "x"}
	ctl := &mongodoc.Controller{
		Path: ctlPath,
	}
	err = s.jem.DB.InsertController(testContext, ctl)
	c.Assert(err, gc.Equals, nil)

	cred := mongodoc.Credential{
		Path: mpath,
	}
	err = jem.CredentialAddController(s.jem, testContext, &cred, ctlPath)
	c.Assert(err, gc.Equals, nil)

	err = s.jem.DB.GetCredential(testContext, &cred)
	c.Assert(err, gc.Equals, nil)
	c.Assert(cred, jc.DeepEquals, mongodoc.Credential{
		Id:   expectId,
		Path: mpath,
		Type: "empty",
		Controllers: []params.EntityPath{
			ctlPath,
		},
	})

	// Add a second time
	err = jem.CredentialAddController(s.jem, testContext, &cred, ctlPath)
	c.Assert(err, gc.Equals, nil)

	err = s.jem.DB.GetCredential(testContext, &cred)
	c.Assert(err, gc.Equals, nil)
	c.Assert(cred, jc.DeepEquals, mongodoc.Credential{
		Id:   expectId,
		Path: mpath,
		Type: "empty",
		Controllers: []params.EntityPath{
			ctlPath,
		},
	})
	cred2 := mongodoc.Credential{
		Path: mongodoc.CredentialPath{
			Cloud: "test-cloud",
			EntityPath: mongodoc.EntityPath{
				User: "test-user",
				Name: "no-such-cred",
			},
		},
	}
	// Add to a non-existant credential
	err = jem.CredentialAddController(s.jem, testContext, &cred2, ctlPath)
	c.Assert(err, gc.ErrorMatches, `credential not found`)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)
}

func (s *credentialSuite) addController(c *gc.C, path params.EntityPath) params.EntityPath {
	return addController(c, path, s.APIInfo(c), s.jem)
}

func (s *credentialSuite) bootstrapModel(c *gc.C, path params.EntityPath) *mongodoc.Model {
	return bootstrapModel(c, path, s.APIInfo(c), s.jem)
}
