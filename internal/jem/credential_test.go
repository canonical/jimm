// Copyright 2015 Canonical Ltd.

package jem_test

import (
	cloudapi "github.com/juju/juju/api/cloud"
	jujuparams "github.com/juju/juju/apiserver/params"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/errgo.v1"

	"github.com/canonical/jimm/internal/conv"
	"github.com/canonical/jimm/internal/jem"
	"github.com/canonical/jimm/internal/jemtest"
	"github.com/canonical/jimm/internal/mongodoc"
	"github.com/canonical/jimm/params"
)

type credentialSuite struct {
	jemtest.BootstrapSuite
}

var _ = gc.Suite(&credentialSuite{})

func (s *credentialSuite) TestRevokeCredentialsInUse(c *gc.C) {
	err := s.JEM.RevokeCredential(testContext, jemtest.Bob, &s.Credential, 0)
	c.Assert(err, gc.ErrorMatches, `cannot revoke because credential is in use on at least one model`)

	// Try with just the check.
	err = s.JEM.RevokeCredential(testContext, jemtest.Bob, &s.Credential, jem.CredentialCheck)
	c.Assert(err, gc.ErrorMatches, `cannot revoke because credential is in use on at least one model`)

	// Try without the check. It should succeed.
	err = s.JEM.RevokeCredential(testContext, jemtest.Bob, &s.Credential, jem.CredentialUpdate)
	c.Assert(err, gc.Equals, nil)

	// Try to create another model with the credentials that have
	// been revoked. We should fail to do that.
	err = s.JEM.CreateModel(testContext, jemtest.Bob, jem.CreateModelParams{
		Path:           params.EntityPath{"bob", "newmodel"},
		ControllerPath: s.Controller.Path,
		Credential:     s.Credential.Path,
		Cloud:          jemtest.TestCloudName,
	}, nil)
	c.Assert(err, gc.ErrorMatches, `credential `+jemtest.TestCloudName+`/bob/cred has been revoked`)

	// Check that the credential really has been revoked on the
	// controller.
	conn, err := s.JEM.OpenAPI(testContext, s.Controller.Path)
	c.Assert(err, gc.Equals, nil)
	defer conn.Close()
	r, err := cloudapi.NewClient(conn).Credentials(conv.ToCloudCredentialTag(s.Credential.Path))
	c.Assert(err, gc.Equals, nil)
	c.Assert(r, jc.DeepEquals, []jujuparams.CloudCredentialResult{{
		Error: &jujuparams.Error{
			Message: `credential "cred" not found`,
			Code:    "not found",
		},
	}})
}

func (s *credentialSuite) TestRevokeCredentialsNotInUse(c *gc.C) {
	cred := jemtest.EmptyCredential("bob", "cred1")
	err := s.JEM.DB.UpsertCredential(testContext, &cred)
	c.Assert(err, gc.Equals, nil)

	// Check that we can get the credential.
	err = s.JEM.DB.GetCredential(testContext, &cred)
	c.Assert(err, gc.Equals, nil)

	// Try with just the check.
	err = s.JEM.RevokeCredential(testContext, jemtest.Bob, &cred, jem.CredentialCheck)
	c.Assert(err, gc.Equals, nil)

	// Check that the credential is still there.
	err = s.JEM.DB.GetCredential(testContext, &cred)
	c.Assert(err, gc.Equals, nil)

	// Try with both the check and the update flag.
	err = s.JEM.RevokeCredential(testContext, jemtest.Bob, &cred, 0)
	c.Assert(err, gc.Equals, nil)

	// The credential should be marked as revoked and all
	// the details should be cleared.
	err = s.JEM.DB.GetCredential(testContext, &cred)
	c.Assert(err, gc.Equals, nil)
	c.Assert(cred.Revoked, gc.Equals, true)
	c.Assert(cred.Attributes, gc.HasLen, 0)
	c.Assert(cred.AttributesInVault, gc.Equals, false)
}

func (s *credentialSuite) TestUpdateCredential(c *gc.C) {
	// Check the credential was deployed
	conn, err := s.JEM.OpenAPI(testContext, s.Controller.Path)
	c.Assert(err, gc.Equals, nil)
	client := cloudapi.NewClient(conn)
	creds, err := client.Credentials(conv.ToCloudCredentialTag(s.Credential.Path))
	c.Assert(err, gc.Equals, nil)
	c.Assert(creds, jc.DeepEquals, []jujuparams.CloudCredentialResult{{
		Result: &jujuparams.CloudCredential{
			AuthType: "empty",
		},
	}})

	_, err = s.JEM.UpdateCredential(testContext, jemtest.Bob, &mongodoc.Credential{
		Path: s.Credential.Path,
		Type: "userpass",
		Attributes: map[string]string{
			"username": "cloud-user",
			"password": "cloud-pass",
		},
		ProviderType: jemtest.TestProviderType,
	}, jem.CredentialUpdate)
	c.Assert(err, gc.Equals, nil)

	// check it was updated on the controller.
	creds, err = client.Credentials(conv.ToCloudCredentialTag(s.Credential.Path))
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
	err = s.JEM.RevokeCredential(testContext, jemtest.Bob, &s.Credential, jem.CredentialUpdate)
	c.Assert(err, gc.Equals, nil)

	// check it was removed on the controller.
	creds, err = client.Credentials(conv.ToCloudCredentialTag(s.Credential.Path))
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
		Cloud: jemtest.TestCloudName,
		User:  "bob",
		Name:  "credential",
	},
}, {
	path: params.CredentialPath{
		Cloud: jemtest.TestCloudName,
		User:  "bob-group",
		Name:  "credential",
	},
}, {
	path: params.CredentialPath{
		Cloud: jemtest.TestCloudName,
		User:  "alice",
		Name:  "credential",
	},
	expectErrorCause: params.ErrUnauthorized,
}, {
	path: params.CredentialPath{
		Cloud: jemtest.TestCloudName,
		User:  "bob",
		Name:  "credential2",
	},
	expectErrorCause: params.ErrNotFound,
}, {
	path: params.CredentialPath{
		Cloud: jemtest.TestCloudName,
		User:  "bob-group",
		Name:  "credential2",
	},
	expectErrorCause: params.ErrNotFound,
}, {
	path: params.CredentialPath{
		Cloud: jemtest.TestCloudName,
		User:  "alice",
		Name:  "credential2",
	},
	expectErrorCause: params.ErrUnauthorized,
}}

func (s *credentialSuite) TestGetCredential(c *gc.C) {
	creds := []mongodoc.Credential{
		jemtest.EmptyCredential("alice", "credential"),
		jemtest.EmptyCredential("bob", "credential"),
		jemtest.EmptyCredential("bob-group", "credential"),
	}
	for _, cred := range creds {
		err := s.JEM.DB.UpsertCredential(testContext, &cred)
		c.Assert(err, gc.Equals, nil)
	}
	for i, test := range credentialTests {
		c.Logf("test %d. %s", i, test.path)
		ctl := mongodoc.Credential{
			Path: mongodoc.CredentialPathFromParams(test.path),
		}
		err := s.JEM.GetCredential(testContext, jemtest.Bob, &ctl)
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

func (s *credentialSuite) TestCredentialAddController(c *gc.C) {
	path := credentialPath("test-cloud", "test-user", "test-credential")
	mpath := mongodoc.CredentialPathFromParams(path)
	expectId := path.String()
	err := s.JEM.DB.UpsertCredential(testContext, &mongodoc.Credential{
		Path: mpath,
		Type: "empty",
	})
	c.Assert(err, gc.Equals, nil)

	ctlPath := params.EntityPath{"bob", "x"}
	ctl := &mongodoc.Controller{
		Path: ctlPath,
	}
	err = s.JEM.DB.InsertController(testContext, ctl)
	c.Assert(err, gc.Equals, nil)

	cred := mongodoc.Credential{
		Path: mpath,
	}
	err = jem.CredentialAddController(s.JEM, testContext, &cred, ctlPath)
	c.Assert(err, gc.Equals, nil)

	err = s.JEM.DB.GetCredential(testContext, &cred)
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
	err = jem.CredentialAddController(s.JEM, testContext, &cred, ctlPath)
	c.Assert(err, gc.Equals, nil)

	err = s.JEM.DB.GetCredential(testContext, &cred)
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
	err = jem.CredentialAddController(s.JEM, testContext, &cred2, ctlPath)
	c.Assert(err, gc.ErrorMatches, `credential not found`)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)
}
