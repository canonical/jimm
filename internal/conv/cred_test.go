// Copyright 2020 Canonical Ltd.

package conv_test

import (
	jujuparams "github.com/juju/juju/apiserver/params"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/errgo.v1"

	"github.com/CanonicalLtd/jimm/internal/conv"
	"github.com/CanonicalLtd/jimm/internal/mongodoc"
)

type credSuite struct{}

var _ = gc.Suite(&credSuite{})

func (s *credSuite) TestToCloudCredentialTag(c *gc.C) {
	cp1 := mongodoc.CredentialPath{
		Cloud: "dummy",
		EntityPath: mongodoc.EntityPath{
			User: "alice",
			Name: "cred",
		},
	}
	cp2 := mongodoc.CredentialPath{
		Cloud: "dummy",
		EntityPath: mongodoc.EntityPath{
			User: "alice@domain",
			Name: "cred",
		},
	}
	var cp3 mongodoc.CredentialPath

	c.Assert(conv.ToCloudCredentialTag(cp1).String(), gc.Equals, "cloudcred-dummy_alice@external_cred")
	c.Assert(conv.ToCloudCredentialTag(cp2).String(), gc.Equals, "cloudcred-dummy_alice@domain_cred")
	c.Assert(conv.ToCloudCredentialTag(cp3).String(), gc.Equals, "")
}

var fromCloudCredentialTagTests = []struct {
	tag              string
	expect           mongodoc.CredentialPath
	expectError      string
	expectErrorCause error
}{{
	tag: "cloudcred-dummy_alice@external_cred",
	expect: mongodoc.CredentialPath{
		Cloud: "dummy",
		EntityPath: mongodoc.EntityPath{
			User: "alice",
			Name: "cred",
		},
	},
}, {
	tag: "cloudcred-dummy_alice@domain_cred",
	expect: mongodoc.CredentialPath{
		Cloud: "dummy",
		EntityPath: mongodoc.EntityPath{
			User: "alice@domain",
			Name: "cred",
		},
	},
}, {
	tag:              "cloudcred-dummy_alice_cred",
	expectError:      "unsupported local user",
	expectErrorCause: conv.ErrLocalUser,
}, {
	tag:    "",
	expect: mongodoc.CredentialPath{},
}}

func (s *credSuite) TestFromCloudCredentialTag(c *gc.C) {
	for i, test := range fromCloudCredentialTagTests {
		c.Logf("test %d. %s", i, test.tag)
		var tag names.CloudCredentialTag
		if test.tag != "" {
			var err error
			tag, err = names.ParseCloudCredentialTag(test.tag)
			c.Assert(err, gc.Equals, nil)
		}
		path, err := conv.FromCloudCredentialTag(tag)
		if test.expectError != "" {
			c.Assert(err, gc.ErrorMatches, test.expectError)
			if test.expectErrorCause != nil {
				c.Assert(errgo.Cause(err), gc.Equals, test.expectErrorCause)
			}
			continue
		}
		c.Assert(err, gc.Equals, nil)
		c.Check(path, jc.DeepEquals, test.expect)
	}
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
		Tag: conv.ToCloudCredentialTag(mongodoc.CredentialPath{
			Cloud: "dummy",
			EntityPath: mongodoc.EntityPath{
				User: "test-user",
				Name: "test-cred",
			},
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
