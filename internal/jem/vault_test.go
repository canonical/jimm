// Copyright 2020 Canonical Ltd.

package jem_test

import (
	"context"

	vault "github.com/hashicorp/vault/api"
	"github.com/juju/juju/cloud"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"
	"gopkg.in/macaroon-bakery.v2/httpbakery"

	"github.com/CanonicalLtd/jimm/internal/jem"
	"github.com/CanonicalLtd/jimm/internal/jemtest"
	"github.com/CanonicalLtd/jimm/internal/mgosession"
	"github.com/CanonicalLtd/jimm/internal/mongodoc"
	"github.com/CanonicalLtd/jimm/internal/pubsub"
)

type jemVaultSuite struct {
	jemtest.JujuConnSuite
	pool                           *jem.Pool
	sessionPool                    *mgosession.Pool
	jem                            *jem.JEM
	usageSenderAuthorizationClient *testUsageSenderAuthorizationClient
	vaultClient                    *vault.Client

	suiteCleanups []func()
}

var _ = gc.Suite(&jemVaultSuite{})

func (s *jemVaultSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.sessionPool = mgosession.NewPool(context.TODO(), s.Session, 5)
	publicCloudMetadata, _, err := cloud.PublicCloudMetadata()
	c.Assert(err, gc.Equals, nil)
	s.usageSenderAuthorizationClient = &testUsageSenderAuthorizationClient{}
	s.PatchValue(&jem.NewUsageSenderAuthorizationClient, func(_ string, _ *httpbakery.Client) (jem.UsageSenderAuthorizationClient, error) {
		return s.usageSenderAuthorizationClient, nil
	})

	vaultcfg := vault.DefaultConfig()
	vaultcfg.Address = "http://localhost:8200"

	s.vaultClient, err = vault.NewClient(vaultcfg)
	c.Assert(err, gc.Equals, nil)
	s.vaultClient.SetToken("test-token")
	err = s.vaultClient.Sys().Mount("/test", &vault.MountInput{
		Type: "kv",
	})
	c.Assert(err, gc.Equals, nil)

	pool, err := jem.NewPool(context.TODO(), jem.Params{
		DB:                  s.Session.DB("jem"),
		ControllerAdmin:     "controller-admin",
		SessionPool:         s.sessionPool,
		PublicCloudMetadata: publicCloudMetadata,
		UsageSenderURL:      "test-usage-sender-url",
		Pubsub: &pubsub.Hub{
			MaxConcurrency: 10,
		},
		VaultClient: s.vaultClient,
		VaultPath:   "test",
	})
	c.Assert(err, gc.Equals, nil)
	s.pool = pool
	s.jem = s.pool.JEM(context.TODO())
	s.PatchValue(&utils.OutgoingAccessAllowed, true)
}

func (s *jemVaultSuite) TearDownTest(c *gc.C) {
	err := s.vaultClient.Sys().Unmount("/test")
	if err != nil {
		c.Logf("cannot unmount vault secret store: %s", err)
	}
	s.jem.Close()
	s.pool.Close()
	s.sessionPool.Close()
	s.JujuConnSuite.TearDownTest(c)
}

func (s *jemVaultSuite) TestVaultCredentials(c *gc.C) {
	if vaultNotAvailable {
		c.Skip("vault not available")
	}

	ctx := context.Background()

	cred1 := &mongodoc.Credential{
		Path: mongodoc.CredentialPath{
			Cloud: "dummy",
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
	_, err := s.jem.UpdateCredential(ctx, cred1, 0)
	c.Assert(err, gc.Equals, nil)

	secret, err := s.vaultClient.Logical().Read("test/creds/dummy/bob/test-1")
	c.Assert(err, gc.Equals, nil)
	c.Check(secret.Data, gc.DeepEquals, map[string]interface{}{
		"username": "test-user",
		"password": "test-pass",
	})

	var cred2 mongodoc.Credential
	err = s.jem.DB.Credentials().FindId("dummy/bob/test-1").One(&cred2)
	c.Assert(err, gc.Equals, nil)
	c.Check(cred2.Attributes, gc.HasLen, 0)

	cred3, err := s.jem.GetCredential(ctx, jemtest.NewIdentity("bob"), cred1.Path.ToParams())
	c.Assert(err, gc.Equals, nil)
	c.Check(cred3.Attributes, gc.HasLen, 0)

	err = s.jem.GetCredentialAttributes(ctx, cred3)
	c.Assert(err, gc.Equals, nil)
	c.Check(cred3.Id, gc.Equals, "dummy/bob/test-1")
	cred3.Id = ""
	cred3.AttributesInVault = false
	c.Check(cred3, gc.DeepEquals, cred1)
}
