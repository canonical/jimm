// Copyright 2020 Canonical Ltd.

package jem_test

import (
	"context"

	vault "github.com/hashicorp/vault/api"
	gc "gopkg.in/check.v1"

	"github.com/CanonicalLtd/jimm/internal/jemtest"
	"github.com/CanonicalLtd/jimm/internal/mongodoc"
)

type jemVaultSuite struct {
	jemtest.BootstrapSuite
}

var _ = gc.Suite(&jemVaultSuite{})

func (s *jemVaultSuite) SetUpTest(c *gc.C) {
	vaultcfg := vault.DefaultConfig()
	vaultcfg.Address = "http://localhost:8200"
	var err error
	s.Params.VaultClient, err = vault.NewClient(vaultcfg)
	c.Assert(err, gc.Equals, nil)

	s.Params.VaultClient.SetToken("test-token")
	err = s.Params.VaultClient.Sys().Mount("/test", &vault.MountInput{
		Type: "kv",
	})
	c.Assert(err, gc.Equals, nil)
	s.Params.VaultPath = "test"
	s.BootstrapSuite.SetUpTest(c)
}

func (s *jemVaultSuite) TearDownTest(c *gc.C) {
	s.BootstrapSuite.TearDownTest(c)
	if s.Params.VaultClient != nil {
		err := s.Params.VaultClient.Sys().Unmount("/test")
		if err != nil {
			c.Logf("cannot unmount vault secret store: %s", err)
		}
		s.Params.VaultClient = nil
	}
}

func (s *jemVaultSuite) TestVaultCredentials(c *gc.C) {
	if vaultNotAvailable {
		c.Skip("vault not available")
	}

	ctx := context.Background()

	cred1 := &mongodoc.Credential{
		Path: mongodoc.CredentialPath{
			Cloud: jemtest.TestCloudName,
			EntityPath: mongodoc.EntityPath{
				User: "bob",
				Name: "test-1",
			},
		},
		Type: "userpass",
		Attributes: map[string]string{
			"username": "test-user",
			"password": "test-pass",
		},
	}
	_, err := s.JEM.UpdateCredential(ctx, jemtest.Bob, cred1, 0)
	c.Assert(err, gc.Equals, nil)

	secret, err := s.Params.VaultClient.Logical().Read("test/creds/" + jemtest.TestCloudName + "/bob/test-1")
	c.Assert(err, gc.Equals, nil)
	c.Assert(secret, gc.Not(gc.IsNil))
	c.Check(secret.Data, gc.DeepEquals, map[string]interface{}{
		"username": "test-user",
		"password": "test-pass",
	})

	cred2 := mongodoc.Credential{
		Path: cred1.Path,
	}
	err = s.JEM.DB.GetCredential(ctx, &cred2)
	c.Assert(err, gc.Equals, nil)
	c.Check(cred2.Attributes, gc.HasLen, 0)

	cred3 := mongodoc.Credential{
		Path: cred1.Path,
	}
	err = s.JEM.GetCredential(ctx, jemtest.NewIdentity("bob"), &cred3)
	c.Assert(err, gc.Equals, nil)
	c.Check(cred3.Attributes, gc.HasLen, 0)

	err = s.JEM.FillCredentialAttributes(ctx, &cred3)
	c.Assert(err, gc.Equals, nil)
	c.Check(cred3.Id, gc.Equals, jemtest.TestCloudName+"/bob/test-1")
	cred3.Id = ""
	cred3.AttributesInVault = false
	c.Check(&cred3, gc.DeepEquals, cred1)
}
