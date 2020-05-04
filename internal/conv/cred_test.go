// Copyright 2020 Canonical Ltd.

package conv_test

import (
	gc "gopkg.in/check.v1"

	jujuparams "github.com/juju/juju/apiserver/params"

	"github.com/CanonicalLtd/jimm/internal/conv"
	"github.com/CanonicalLtd/jimm/internal/mongodoc"
	"github.com/CanonicalLtd/jimm/params"
)

type credSuite struct{}

var _ = gc.Suite(&credSuite{})

func (s *credSuite) TestToCloudCredentialTag(c *gc.C) {
	cp1 := params.CredentialPath{
		Cloud: "dummy",
		User:  "alice",
		Name:  "cred",
	}
	cp2 := params.CredentialPath{
		Cloud: "dummy",
		User:  "alice@domain",
		Name:  "cred",
	}
	var cp3 params.CredentialPath

	c.Assert(conv.ToCloudCredentialTag(cp1).String(), gc.Equals, "cloudcred-dummy_alice@external_cred")
	c.Assert(conv.ToCloudCredentialTag(cp2).String(), gc.Equals, "cloudcred-dummy_alice@domain_cred")
	c.Assert(conv.ToCloudCredentialTag(cp3).String(), gc.Equals, "")
}

func (s *credSuite) TestToTaggedCredential(c *gc.C) {
	tc := conv.ToTaggedCredential(&mongodoc.Credential{
		Path: mongodoc.CredentialPath{
			Cloud: "dummy",
			EntityPath: mongodoc.EntityPath{
				User: "test-user",
				Name: "test-cred",
			},
		},
		Type: "userpass",
		Attributes: map[string]string{
			"username": "alibaba",
			"password": "open sesame",
		},
	})
	c.Assert(tc, gc.DeepEquals, jujuparams.TaggedCredential{
		Tag: conv.ToCloudCredentialTag(params.CredentialPath{
			Cloud: "dummy",
			User:  "test-user",
			Name:  "test-cred",
		}).String(),
		Credential: jujuparams.CloudCredential{
			AuthType: "userpass",
			Attributes: map[string]string{
				"username": "alibaba",
				"password": "open sesame",
			},
		},
	})
}
