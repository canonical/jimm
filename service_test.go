// Copyright 2021 Canonical Ltd.

package jimm_test

import (
	"context"
	"encoding/json"
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
	"github.com/juju/juju/api"
	"github.com/juju/juju/api/client/cloud"
	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/names/v4"

	"github.com/CanonicalLtd/jimm"
	"github.com/CanonicalLtd/jimm/internal/dbmodel"
	"github.com/CanonicalLtd/jimm/internal/jimmtest"
	"github.com/CanonicalLtd/jimm/internal/openfga"
	"github.com/CanonicalLtd/jimm/internal/vault"
)

func TestMain(m *testing.M) {
	code := m.Run()
	os.Exit(code)
}

func TestDefaultService(t *testing.T) {
	c := qt.New(t)

	_, ofgaClient, cfg, err := jimmtest.SetupTestOFGAClient(c.Name())
	c.Assert(err, qt.IsNil)

	svc, err := jimm.NewService(context.Background(), jimm.Params{
		OpenFGAParams: jimm.OpenFGAParams{
			Scheme:    cfg.ApiScheme,
			Host:      cfg.ApiHost,
			Store:     cfg.StoreId,
			Token:     cfg.Credentials.Config.ApiToken,
			AuthModel: ofgaClient.AuthModelId,
		},
	})
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

	_, ofgaClient, cfg, err := jimmtest.SetupTestOFGAClient(c.Name())
	c.Assert(err, qt.IsNil)

	p := jimm.Params{
		ControllerUUID:   "6acf4fd8-32d6-49ea-b4eb-dcb9d1590c11",
		ControllerAdmins: []string{"admin"},
		OpenFGAParams: jimm.OpenFGAParams{
			Scheme:    cfg.ApiScheme,
			Host:      cfg.ApiHost,
			Store:     cfg.StoreId,
			Token:     cfg.Credentials.Config.ApiToken,
			AuthModel: ofgaClient.AuthModelId,
		},
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
	c.Check(conn.ControllerAccess(), qt.Equals, "login")
}

func TestVault(t *testing.T) {
	c := qt.New(t)

	_, ofgaClient, cfg, err := jimmtest.SetupTestOFGAClient(c.Name())
	c.Assert(err, qt.IsNil)

	p := jimm.Params{
		ControllerUUID:  "6acf4fd8-32d6-49ea-b4eb-dcb9d1590c11",
		VaultAddress:    "http://localhost:8200",
		VaultAuthPath:   "/auth/approle/login",
		VaultPath:       "/jimm-kv/",
		VaultSecretFile: "./local/vault/approle.json",
		OpenFGAParams: jimm.OpenFGAParams{
			Scheme:    cfg.ApiScheme,
			Host:      cfg.ApiHost,
			Store:     cfg.StoreId,
			Token:     cfg.Credentials.Config.ApiToken,
			AuthModel: ofgaClient.AuthModelId,
		},
	}
	candid := startCandid(c, &p)
	vaultClient, _, creds, _ := jimmtest.VaultClient(c, ".")

	svc, err := jimm.NewService(context.Background(), p)
	c.Assert(err, qt.IsNil)

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
	tag := names.NewCloudCredentialTag(jimmtest.TestCloudName + "/bob@external/test-1").String()
	_, err = cloudClient.UpdateCloudsCredentials(map[string]jujucloud.Credential{
		tag: jujucloud.NewCredential(jujucloud.UserPassAuthType, map[string]string{
			"username": "test-user",
			"password": "test-secret",
		}),
	}, false)
	c.Assert(err, qt.IsNil)

	store := vault.VaultStore{
		Client:     vaultClient,
		AuthSecret: creds,
		AuthPath:   p.VaultAuthPath,
		KVPath:     p.VaultPath,
	}
	attr, err := store.Get(context.Background(), names.NewCloudCredentialTag(jimmtest.TestCloudName+"/bob@external/test-1"))
	c.Assert(err, qt.IsNil)
	c.Check(attr, qt.DeepEquals, map[string]string{
		"username": "test-user",
		"password": "test-secret",
	})
}

func TestOpenFGA(t *testing.T) {
	c := qt.New(t)

	_, ofgaClient, cfg, err := jimmtest.SetupTestOFGAClient(c.Name())
	c.Assert(err, qt.IsNil)

	p := jimm.Params{
		ControllerUUID: "6acf4fd8-32d6-49ea-b4eb-dcb9d1590c11",
		OpenFGAParams: jimm.OpenFGAParams{
			Scheme:    cfg.ApiScheme,
			Host:      cfg.ApiHost,
			Store:     cfg.StoreId,
			Token:     cfg.Credentials.Config.ApiToken,
			AuthModel: ofgaClient.AuthModelId,
		},
		ControllerAdmins: []string{"alice", "eve"},
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
		BakeryClient:       userClient(candid, "bob"),
		InsecureSkipVerify: true,
	})
	c.Assert(err, qt.IsNil)
	c.Cleanup(func() {
		if err := conn.Close(); err != nil {
			c.Logf("closing connection: %s", err)
		}
	})

	client, err := jimm.NewOpenFGAClient(context.Background(), p)
	c.Assert(err, qt.IsNil)

	// assert controller admins have been created in openfga
	for _, username := range []string{"alice", "eve"} {
		user := openfga.NewUser(
			&dbmodel.User{Username: username},
			client,
		)
		allowed, err := openfga.IsAdministrator(context.Background(), user, names.NewControllerTag(p.ControllerUUID))
		c.Assert(err, qt.IsNil)
		c.Assert(allowed, qt.IsTrue)
	}
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
	p.BakeryAgentFile = filepath.Join(c.TempDir(), "agent.json")
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