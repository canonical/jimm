// Copyright 2021 Canonical Ltd.

package jimm_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/canonical/candid/candidtest"
	qt "github.com/frankban/quicktest"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/httpbakery"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/httpbakery/agent"
	vaultapi "github.com/hashicorp/vault/api"
	"github.com/juju/juju/api"
	"github.com/juju/juju/api/cloud"
	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/names/v4"

	"github.com/CanonicalLtd/jimm"
	"github.com/CanonicalLtd/jimm/internal/jimmtest"
)

func TestMain(m *testing.M) {
	code := m.Run()
	jimmtest.VaultStop()
	os.Exit(code)
}

func TestDefaultService(t *testing.T) {
	c := qt.New(t)

	svc, err := jimm.NewService(context.Background(), jimm.Params{})
	c.Assert(err, qt.IsNil)
	rr := httptest.NewRecorder()
	req, err := http.NewRequest("GET", "/debug/info", nil)
	c.Assert(err, qt.IsNil)
	svc.ServeHTTP(rr, req)
	resp := rr.Result()
	c.Check(resp.StatusCode, qt.Equals, http.StatusOK)
}

func TestAuthenticator(t *testing.T) {
	c := qt.New(t)

	p := jimm.Params{
		ControllerUUID:   "6acf4fd8-32d6-49ea-b4eb-dcb9d1590c11",
		ControllerAdmins: []string{"admin"},
	}
	candid := startCandid(c, &p)
	svc, err := jimm.NewService(context.Background(), p)
	c.Assert(err, qt.IsNil)

	srv := httptest.NewTLSServer(svc)
	c.Cleanup(srv.Close)
	info := api.Info{
		Addrs: []string{srv.Listener.Addr().String()},
	}

	conn, err := api.Open(&info, api.DialOpts{
		BakeryClient:       userClient(candid, "alice", "admin"),
		InsecureSkipVerify: true,
	})
	c.Assert(err, qt.IsNil)
	c.Cleanup(func() {
		if err := conn.Close(); err != nil {
			c.Logf("closing connection: %s", err)
		}
	})

	c.Check(conn.ControllerTag(), qt.Equals, names.NewControllerTag("6acf4fd8-32d6-49ea-b4eb-dcb9d1590c11"))
	c.Check(conn.AuthTag(), qt.Equals, names.NewUserTag("alice@external"))
	c.Check(conn.ControllerAccess(), qt.Equals, "superuser")

	conn, err = api.Open(&info, api.DialOpts{
		BakeryClient:       userClient(candid, "bob"),
		InsecureSkipVerify: true,
	})
	c.Assert(err, qt.IsNil)
	c.Cleanup(func() {
		if err := conn.Close(); err != nil {
			c.Logf("closing connection: %s", err)
		}
	})

	c.Check(conn.ControllerTag(), qt.Equals, names.NewControllerTag("6acf4fd8-32d6-49ea-b4eb-dcb9d1590c11"))
	c.Check(conn.AuthTag(), qt.Equals, names.NewUserTag("bob@external"))
	c.Check(conn.ControllerAccess(), qt.Equals, "add-model")
}

func TestVault(t *testing.T) {
	c := qt.New(t)

	p := jimm.Params{
		ControllerUUID: "6acf4fd8-32d6-49ea-b4eb-dcb9d1590c11",
	}
	candid := startCandid(c, &p)
	vaultClient := startVault(c, &p)
	svc, err := jimm.NewService(context.Background(), p)
	c.Assert(err, qt.IsNil)

	ctx, cancel := context.WithCancel(context.Background())
	c.Cleanup(cancel)
	go func() {
		c.Check(svc.WatchVaultToken(ctx), qt.Equals, context.Canceled)
	}()

	srv := httptest.NewTLSServer(svc)
	c.Cleanup(srv.Close)
	info := api.Info{
		Addrs: []string{srv.Listener.Addr().String()},
	}

	conn, err := api.Open(&info, api.DialOpts{
		BakeryClient:       userClient(candid, "bob"),
		InsecureSkipVerify: true,
	})
	c.Assert(err, qt.IsNil)
	c.Cleanup(func() {
		if err := conn.Close(); err != nil {
			c.Logf("closing connection: %s", err)
		}
	})

	cloudClient := cloud.NewClient(conn)
	tag := names.NewCloudCredentialTag("dummy/bob@external/test-1").String()
	_, err = cloudClient.UpdateCloudsCredentials(map[string]jujucloud.Credential{
		tag: jujucloud.NewCredential(jujucloud.UserPassAuthType, map[string]string{
			"username": "test-user",
			"password": "test-secret",
		}),
	}, false)
	c.Assert(err, qt.IsNil)

	s, err := vaultClient.Logical().Read(c.Name() + "/creds/dummy/bob@external/test-1")
	c.Assert(err, qt.IsNil)
	c.Assert(s, qt.Not(qt.IsNil))
	c.Check(s.Data, qt.DeepEquals, map[string]interface{}{
		"username": "test-user",
		"password": "test-secret",
	})
}

func startCandid(c *qt.C, p *jimm.Params) *candidtest.Server {
	candid := candidtest.NewServer()
	c.Cleanup(candid.Close)
	p.CandidURL = candid.URL.String()

	tpi, err := httpbakery.ThirdPartyInfoForLocation(context.Background(), nil, candid.URL.String())
	c.Assert(err, qt.IsNil)
	pk, err := tpi.PublicKey.MarshalText()
	c.Assert(err, qt.IsNil)
	p.CandidPublicKey = string(pk)

	candid.AddUser("jimm-agent", candidtest.GroupListGroup)
	buf, err := json.Marshal(agent.AuthInfo{
		Key: key(candid, "jimm-agent"),
		Agents: []agent.Agent{{
			URL:      candid.URL.String(),
			Username: "jimm-agent",
		}},
	})
	c.Assert(err, qt.IsNil)
	p.BakeryAgentFile = filepath.Join(c.Mkdir(), "agent.json")
	err = os.WriteFile(p.BakeryAgentFile, buf, 0400)
	c.Assert(err, qt.IsNil)
	return candid
}

func userClient(candid *candidtest.Server, user string, groups ...string) *httpbakery.Client {
	candid.AddUser(user, groups...)
	client := httpbakery.NewClient()
	agent.SetUpAuth(client, &agent.AuthInfo{
		Key: key(candid, user),
		Agents: []agent.Agent{{
			URL:      candid.URL.String(),
			Username: user,
		}},
	})
	return client
}

func key(candid *candidtest.Server, user string) *bakery.KeyPair {
	key := candid.UserPublicKey(user)
	return &bakery.KeyPair{
		Public:  bakery.PublicKey{Key: bakery.Key(key.Public.Key)},
		Private: bakery.PrivateKey{Key: bakery.Key(key.Private.Key)},
	}
}

var policyTemplate = `
path "%s/*" {
  capabilities = ["create", "read", "update", "delete", "list"]
}
`[1:]

func startVault(c *qt.C, p *jimm.Params) *vaultapi.Client {
	client, path, ok := jimmtest.VaultClient(c)
	if !ok {
		c.Skip("vault not available")
	}
	p.VaultAddress = client.Address()
	p.VaultPath = path

	auths, err := client.Sys().ListAuth()
	c.Assert(err, qt.IsNil)
	if _, ok := auths["approle"]; !ok {
		err := client.Sys().EnableAuthWithOptions("approle", &vaultapi.EnableAuthOptions{Type: "approle"})
		c.Assert(err, qt.IsNil)
	}

	err = client.Sys().PutPolicy(c.Name(), fmt.Sprintf(policyTemplate, path))
	c.Assert(err, qt.IsNil)

	authSecret := vaultapi.Secret{
		Data: make(map[string]interface{}, 2),
	}
	_, err = client.Logical().Write("/auth/approle/role/"+c.Name(), map[string]interface{}{
		"token_policies": []string{c.Name()},
	})
	c.Assert(err, qt.IsNil)
	s, err := client.Logical().Read("/auth/approle/role/" + c.Name() + "/role-id")
	c.Assert(err, qt.IsNil)
	authSecret.Data["role_id"] = s.Data["role_id"]
	s, err = client.Logical().Write("/auth/approle/role/"+c.Name()+"/secret-id", nil)
	c.Assert(err, qt.IsNil)
	authSecret.Data["secret_id"] = s.Data["secret_id"]

	buf, err := json.Marshal(authSecret)
	c.Assert(err, qt.IsNil)
	p.VaultSecretFile = filepath.Join(c.Mkdir(), "approle.json")
	err = os.WriteFile(p.VaultSecretFile, buf, 0400)
	c.Assert(err, qt.IsNil)
	p.VaultAuthPath = "/auth/approle/login"
	return client
}
