// Copyright 2024 Canonical.

package service_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"

	cofga "github.com/canonical/ofga"
	qt "github.com/frankban/quicktest"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery/checkers"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/httpbakery"
	"github.com/juju/juju/api"
	"github.com/juju/juju/api/client/cloud"
	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/core/macaroon"
	"github.com/juju/names/v5"

	jimmsvc "github.com/canonical/jimm/v3/cmd/jimmsrv/service"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/openfga"
	ofganames "github.com/canonical/jimm/v3/internal/openfga/names"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
	"github.com/canonical/jimm/v3/internal/vault"
)

func TestMain(m *testing.M) {
	code := m.Run()
	os.Exit(code)
}

func TestDefaultService(t *testing.T) {
	c := qt.New(t)

	_, _, cofgaParams, err := jimmtest.SetupTestOFGAClient(c.Name())
	c.Assert(err, qt.IsNil)
	p := jimmtest.NewTestJimmParams(c)
	p.OpenFGAParams = cofgaParamsToJIMMOpenFGAParams(*cofgaParams)
	p.InsecureSecretStorage = true
	svc, err := jimmsvc.NewService(context.Background(), p)
	c.Assert(err, qt.IsNil)
	defer svc.Cleanup()
	rr := httptest.NewRecorder()
	req, err := http.NewRequest("GET", "/debug/info", nil)
	c.Assert(err, qt.IsNil)
	svc.ServeHTTP(rr, req)
	resp := rr.Result()
	defer resp.Body.Close()
	c.Check(resp.StatusCode, qt.Equals, http.StatusOK)
}

func TestServiceDoesNotStartWithoutCredentialStore(t *testing.T) {
	c := qt.New(t)

	_, _, cofgaParams, err := jimmtest.SetupTestOFGAClient(c.Name())
	c.Assert(err, qt.IsNil)
	p := jimmtest.NewTestJimmParams(c)
	p.OpenFGAParams = cofgaParamsToJIMMOpenFGAParams(*cofgaParams)
	_, err = jimmsvc.NewService(context.Background(), p)
	c.Assert(err, qt.ErrorMatches, "jimm cannot start without a credential store")
}

func TestAuthenticator(t *testing.T) {
	c := qt.New(t)

	_, _, cofgaParams, err := jimmtest.SetupTestOFGAClient(c.Name())
	c.Assert(err, qt.IsNil)

	p := jimmtest.NewTestJimmParams(c)
	p.InsecureSecretStorage = true
	p.OpenFGAParams = cofgaParamsToJIMMOpenFGAParams(*cofgaParams)
	svc, err := jimmsvc.NewService(context.Background(), p)
	c.Assert(err, qt.IsNil)
	defer svc.Cleanup()

	srv := httptest.NewTLSServer(svc)
	c.Cleanup(srv.Close)
	info := api.Info{
		Addrs: []string{srv.Listener.Addr().String()},
	}

	conn, err := api.Open(&info, api.DialOpts{
		LoginProvider:      jimmtest.NewUserSessionLogin(c, "alice"),
		InsecureSkipVerify: true,
	})
	c.Assert(err, qt.IsNil)
	c.Cleanup(func() {
		if err := conn.Close(); err != nil {
			c.Logf("closing connection: %s", err)
		}
	})

	c.Check(conn.ControllerTag(), qt.Equals, names.NewControllerTag("6acf4fd8-32d6-49ea-b4eb-dcb9d1590c11"))
	c.Check(conn.AuthTag(), qt.Equals, names.NewUserTag("alice@canonical.com"))
	c.Check(conn.ControllerAccess(), qt.Equals, "")

	conn, err = api.Open(&info, api.DialOpts{
		LoginProvider:      jimmtest.NewUserSessionLogin(c, "bob"),
		InsecureSkipVerify: true,
	})
	c.Assert(err, qt.IsNil)
	c.Cleanup(func() {
		if err := conn.Close(); err != nil {
			c.Logf("closing connection: %s", err)
		}
	})

	c.Check(conn.ControllerTag(), qt.Equals, names.NewControllerTag("6acf4fd8-32d6-49ea-b4eb-dcb9d1590c11"))
	c.Check(conn.AuthTag(), qt.Equals, names.NewUserTag("bob@canonical.com"))
	c.Check(conn.ControllerAccess(), qt.Equals, "")
}

const testVaultEnv = `clouds:
- name: test
  type: test
  regions:
  - name: test-region
`

func TestVault(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	ofgaClient, _, cofgaParams, err := jimmtest.SetupTestOFGAClient(c.Name())
	c.Assert(err, qt.IsNil)

	vaultClient, _, roleID, roleSecretID, _ := jimmtest.VaultClient(c)
	p := jimmtest.NewTestJimmParams(c)
	p.VaultAddress = "http://localhost:8200"
	p.VaultPath = "/jimm-kv/"
	p.VaultRoleID = roleID
	p.VaultRoleSecretID = roleSecretID
	p.OpenFGAParams = cofgaParamsToJIMMOpenFGAParams(*cofgaParams)
	svc, err := jimmsvc.NewService(ctx, p)
	c.Assert(err, qt.IsNil)
	defer svc.Cleanup()

	env := jimmtest.ParseEnvironment(c, testVaultEnv)
	env.PopulateDBAndPermissions(c, names.NewControllerTag(p.ControllerUUID), svc.JIMM().Database, ofgaClient)

	srv := httptest.NewTLSServer(svc)
	c.Cleanup(srv.Close)
	info := api.Info{
		Addrs: []string{srv.Listener.Addr().String()},
	}

	conn, err := api.Open(&info, api.DialOpts{
		LoginProvider:      jimmtest.NewUserSessionLogin(c, "bob"),
		InsecureSkipVerify: true,
	})
	c.Assert(err, qt.IsNil)
	c.Cleanup(func() {
		if err := conn.Close(); err != nil {
			c.Logf("closing connection: %s", err)
		}
	})

	cloudClient := cloud.NewClient(conn)

	tag := names.NewCloudCredentialTag("test/bob@canonical.com/test-1").String()
	_, err = cloudClient.UpdateCloudsCredentials(map[string]jujucloud.Credential{
		tag: jujucloud.NewCredential(jujucloud.UserPassAuthType, map[string]string{
			"username": "test-user",
			"password": "test-secret",
		}),
	}, false)
	c.Assert(err, qt.IsNil)

	store := vault.VaultStore{
		Client:       vaultClient,
		RoleID:       roleID,
		RoleSecretID: roleSecretID,
		KVPath:       p.VaultPath,
	}
	attr, err := store.Get(context.Background(), names.NewCloudCredentialTag("test/bob@canonical.com/test-1"))
	c.Assert(err, qt.IsNil)
	c.Check(attr, qt.DeepEquals, map[string]string{
		"username": "test-user",
		"password": "test-secret",
	})
}

func TestPostgresSecretStore(t *testing.T) {
	c := qt.New(t)

	_, _, cofgaParams, err := jimmtest.SetupTestOFGAClient(c.Name())
	c.Assert(err, qt.IsNil)

	p := jimmtest.NewTestJimmParams(c)
	p.InsecureSecretStorage = true
	p.OpenFGAParams = cofgaParamsToJIMMOpenFGAParams(*cofgaParams)
	svc, err := jimmsvc.NewService(context.Background(), p)
	c.Assert(err, qt.IsNil)
	defer svc.Cleanup()
}

func TestOpenFGA(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	_, _, cofgaParams, err := jimmtest.SetupTestOFGAClient(c.Name())
	c.Assert(err, qt.IsNil)

	p := jimmtest.NewTestJimmParams(c)
	p.InsecureSecretStorage = true
	p.ControllerAdmins = []string{"alice", "eve"}
	p.OpenFGAParams = cofgaParamsToJIMMOpenFGAParams(*cofgaParams)
	svc, err := jimmsvc.NewService(ctx, p)
	c.Assert(err, qt.IsNil)
	defer svc.Cleanup()

	srv := httptest.NewTLSServer(svc)
	c.Cleanup(srv.Close)
	info := api.Info{
		Addrs: []string{srv.Listener.Addr().String()},
	}

	conn, err := api.Open(&info, api.DialOpts{
		LoginProvider:      jimmtest.NewUserSessionLogin(c, "bob"),
		InsecureSkipVerify: true,
	})
	c.Assert(err, qt.IsNil)
	c.Cleanup(func() {
		if err := conn.Close(); err != nil {
			c.Logf("closing connection: %s", err)
		}
	})

	client, err := jimmsvc.NewOpenFGAClient(context.Background(), p.OpenFGAParams)
	c.Assert(err, qt.IsNil)

	// assert controller admins have been created in openfga
	for _, username := range []string{"alice", "eve"} {
		i, err := dbmodel.NewIdentity(username)
		c.Assert(err, qt.IsNil)
		user := openfga.NewUser(
			i,
			client,
		)
		allowed, err := openfga.IsAdministrator(context.Background(), user, names.NewControllerTag(p.ControllerUUID))
		c.Assert(err, qt.IsNil)
		c.Assert(allowed, qt.IsTrue)
	}
}

func TestPublicKey(t *testing.T) {
	c := qt.New(t)

	_, _, cofgaParams, err := jimmtest.SetupTestOFGAClient(c.Name())
	c.Assert(err, qt.IsNil)

	p := jimmtest.NewTestJimmParams(c)
	p.OpenFGAParams = cofgaParamsToJIMMOpenFGAParams(*cofgaParams)
	p.InsecureSecretStorage = true
	svc, err := jimmsvc.NewService(context.Background(), p)
	c.Assert(err, qt.IsNil)
	defer svc.Cleanup()

	srv := httptest.NewTLSServer(svc)
	c.Cleanup(srv.Close)

	response, err := srv.Client().Get(srv.URL + "/macaroons/publickey")
	c.Assert(err, qt.IsNil)
	defer response.Body.Close()
	data, err := io.ReadAll(response.Body)
	c.Assert(err, qt.IsNil)
	c.Assert(string(data), qt.Equals, `{"PublicKey":"izcYsQy3TePp6bLjqOo3IRPFvkQd2IKtyODGqC6SdFk="}`)
}

func TestRebacAdminApi(t *testing.T) {
	c := qt.New(t)

	_, _, cofgaParams, err := jimmtest.SetupTestOFGAClient(c.Name())
	c.Assert(err, qt.IsNil)

	p := jimmtest.NewTestJimmParams(c)
	p.InsecureSecretStorage = true
	p.OpenFGAParams = cofgaParamsToJIMMOpenFGAParams(*cofgaParams)

	svc, err := jimmsvc.NewService(context.Background(), p)
	c.Assert(err, qt.IsNil)
	defer svc.Cleanup()

	srv := httptest.NewTLSServer(svc)
	c.Cleanup(srv.Close)

	response, err := srv.Client().Get(srv.URL + "/rebac/v1/swagger.json")
	c.Assert(err, qt.IsNil)
	defer response.Body.Close()

	// The `/swagger.json` endpoint doesn't require authentication, so the returned
	// status code should be 200.
	c.Assert(response.StatusCode, qt.Equals, 200)
}

func TestThirdPartyCaveatDischarge(t *testing.T) {
	c := qt.New(t)

	offer := dbmodel.ApplicationOffer{
		UUID: "7e4e7ffb-5116-4544-a400-f584d08c410e",
		Name: "test-application-offer",
	}
	user, err := dbmodel.NewIdentity("alice@canonical.com")
	c.Assert(err, qt.IsNil)

	ctx := context.Background()

	tests := []struct {
		about          string
		setup          func(c *qt.C, ofgaClient *openfga.OFGAClient, user *dbmodel.Identity)
		caveats        []string
		expectDeclared map[string]string
		expectedError  string
	}{{
		about:         "unknown caveats",
		caveats:       []string{"unknown-caveat"},
		expectedError: ".*third party refused discharge: cannot discharge: caveat not recognized",
	}, {
		about: "user is an offer reader",
		setup: func(c *qt.C, ofgaClient *openfga.OFGAClient, user *dbmodel.Identity) {
			u := openfga.NewUser(user, ofgaClient)
			err := u.SetApplicationOfferAccess(ctx, offer.ResourceTag(), ofganames.ReaderRelation)
			c.Assert(err, qt.IsNil)
		},
		caveats:       []string{fmt.Sprintf("is-consumer %s %s", user.ResourceTag(), offer.UUID)},
		expectedError: ".*cannot discharge: permission denied",
	}, {
		about:         "user is not an offer consumer",
		caveats:       []string{fmt.Sprintf("is-consumer %s %s", user.ResourceTag(), offer.UUID)},
		expectedError: ".*cannot discharge: permission denied",
	}, {
		about: "user is an offer consumer",
		setup: func(c *qt.C, ofgaClient *openfga.OFGAClient, user *dbmodel.Identity) {
			u := openfga.NewUser(user, ofgaClient)
			err := u.SetApplicationOfferAccess(ctx, offer.ResourceTag(), ofganames.ConsumerRelation)
			c.Assert(err, qt.IsNil)
		},
		caveats:        []string{fmt.Sprintf("is-consumer %s %s", user.ResourceTag(), offer.UUID)},
		expectDeclared: map[string]string{"offer-uuid": offer.UUID},
	}, {
		about: "user is an offer administrator",
		setup: func(c *qt.C, ofgaClient *openfga.OFGAClient, user *dbmodel.Identity) {
			u := openfga.NewUser(user, ofgaClient)
			err := u.SetApplicationOfferAccess(ctx, offer.ResourceTag(), ofganames.AdministratorRelation)
			c.Assert(err, qt.IsNil)
		},
		caveats:        []string{fmt.Sprintf("is-consumer %s %s", user.ResourceTag(), offer.UUID)},
		expectDeclared: map[string]string{"offer-uuid": offer.UUID},
	}}
	for _, test := range tests {
		c.Run(test.about, func(c *qt.C) {
			ofgaClient, _, cofgaParams, err := jimmtest.SetupTestOFGAClient(c.Name())
			c.Assert(err, qt.IsNil)

			p := jimmtest.NewTestJimmParams(c)
			p.OpenFGAParams = cofgaParamsToJIMMOpenFGAParams(*cofgaParams)
			p.InsecureSecretStorage = true
			svc, err := jimmsvc.NewService(context.Background(), p)
			c.Assert(err, qt.IsNil)
			defer svc.Cleanup()

			srv := httptest.NewTLSServer(svc)
			c.Cleanup(srv.Close)

			var pk bakery.PublicKey
			err = pk.UnmarshalText([]byte(p.PublicKey))
			c.Assert(err, qt.IsNil)

			locator := bakery.NewThirdPartyStore()
			locator.AddInfo(srv.URL+"/macaroons", bakery.ThirdPartyInfo{
				PublicKey: pk,
				Version:   bakery.LatestVersion,
			})

			if test.setup != nil {
				test.setup(c, ofgaClient, user)
			}

			m, err := bakery.NewMacaroon(
				[]byte("root key"),
				[]byte("id"),
				"location",
				bakery.LatestVersion,
				macaroon.MacaroonNamespace,
			)
			c.Assert(err, qt.IsNil)

			kp := bakery.MustGenerateKey()

			for _, caveat := range test.caveats {
				err = m.AddCaveat(context.TODO(), checkers.Caveat{
					Location:  srv.URL + "/macaroons",
					Condition: caveat,
				}, kp, locator)
				c.Assert(err, qt.IsNil)
			}

			bakeryClient := httpbakery.NewClient()
			// Give the bakery client the transport config from the test server client
			// so that the bakery client has the necessary certs for the test server.
			bakeryClient.Client.Transport = srv.Client().Transport
			ms, err := bakeryClient.DischargeAll(context.TODO(), m)
			if test.expectedError != "" {
				c.Assert(err, qt.ErrorMatches, test.expectedError)
			} else {
				c.Assert(err, qt.IsNil)
				c.Assert(ms, qt.HasLen, 2)

				declaredCaveats := checkers.InferDeclared(macaroon.MacaroonNamespace, ms)
				c.Logf("declared caveats %v", declaredCaveats)
				c.Assert(declaredCaveats, qt.DeepEquals, test.expectDeclared)
			}
		})
	}
}

func TestDisableOAuthEndpointsWhenDashboardRedirectURLNotSet(t *testing.T) {
	c := qt.New(t)

	_, _, cofgaParams, err := jimmtest.SetupTestOFGAClient(c.Name())
	c.Assert(err, qt.IsNil)

	p := jimmtest.NewTestJimmParams(c)
	p.DashboardFinalRedirectURL = ""
	p.InsecureSecretStorage = true
	p.OpenFGAParams = cofgaParamsToJIMMOpenFGAParams(*cofgaParams)
	svc, err := jimmsvc.NewService(context.Background(), p)
	c.Assert(err, qt.IsNil)
	defer svc.Cleanup()

	srv := httptest.NewTLSServer(svc)
	c.Cleanup(srv.Close)

	response, err := srv.Client().Get(srv.URL + "/auth/whoami")
	c.Assert(err, qt.IsNil)
	defer response.Body.Close()
	c.Assert(response.StatusCode, qt.Equals, http.StatusNotFound)
}

func TestEnableOAuthEndpointsWhenDashboardRedirectURLSet(t *testing.T) {
	c := qt.New(t)

	_, _, cofgaParams, err := jimmtest.SetupTestOFGAClient(c.Name())
	c.Assert(err, qt.IsNil)

	p := jimmtest.NewTestJimmParams(c)
	p.DashboardFinalRedirectURL = "some-redirect-url"
	p.InsecureSecretStorage = true
	p.OpenFGAParams = cofgaParamsToJIMMOpenFGAParams(*cofgaParams)

	svc, err := jimmsvc.NewService(context.Background(), p)
	c.Assert(err, qt.IsNil)
	defer svc.Cleanup()

	srv := httptest.NewTLSServer(svc)
	c.Cleanup(srv.Close)

	response, err := srv.Client().Get(srv.URL + "/auth/whoami")
	c.Assert(err, qt.IsNil)
	defer response.Body.Close()
	c.Assert(response.StatusCode, qt.Not(qt.Equals), http.StatusNotFound)
}

// cofgaParamsToJIMMOpenFGAParams To avoid circular references, the test setup function (jimmtest.SetupTestOFGAClient)
// does not provide us with an instance of `jimmSvc.OpenFGAParams`, so it just returns a `cofga.OpenFGAParams` instance.
// This method reshapes the later into the former.
func cofgaParamsToJIMMOpenFGAParams(cofgaParams cofga.OpenFGAParams) jimmsvc.OpenFGAParams {
	return jimmsvc.OpenFGAParams{
		Scheme:    cofgaParams.Scheme,
		Host:      cofgaParams.Host,
		Port:      cofgaParams.Port,
		Store:     cofgaParams.StoreID,
		Token:     cofgaParams.Token,
		AuthModel: cofgaParams.AuthModelID,
	}
}

func TestCleanup(t *testing.T) {
	c := qt.New(t)

	outputs := make(chan string, 2)
	svc := jimmsvc.Service{}
	svc.AddCleanup(func() error { outputs <- "first"; return nil })
	svc.AddCleanup(func() error { outputs <- "second"; return nil })
	svc.Cleanup()
	c.Assert([]string{<-outputs, <-outputs}, qt.DeepEquals, []string{"second", "first"})
}

func TestCleanupDoesNotPanic_SessionStoreRelatedCleanups(t *testing.T) {
	c := qt.New(t)

	_, _, cofgaParams, err := jimmtest.SetupTestOFGAClient(c.Name())
	c.Assert(err, qt.IsNil)
	p := jimmtest.NewTestJimmParams(c)
	p.OpenFGAParams = cofgaParamsToJIMMOpenFGAParams(*cofgaParams)
	p.InsecureSecretStorage = true

	svc, err := jimmsvc.NewService(context.Background(), p)
	c.Assert(err, qt.IsNil)

	// Make sure `cleanups` is not empty.
	c.Assert(len(svc.GetCleanups()) > 0, qt.IsTrue)

	svc.Cleanup()
}

func TestCORS(t *testing.T) {
	c := qt.New(t)

	_, _, cofgaParams, err := jimmtest.SetupTestOFGAClient(c.Name())
	c.Assert(err, qt.IsNil)
	p := jimmtest.NewTestJimmParams(c)
	p.OpenFGAParams = cofgaParamsToJIMMOpenFGAParams(*cofgaParams)
	allowedOrigin := "http://my-referrer.com"
	p.CorsAllowedOrigins = []string{allowedOrigin}
	p.InsecureSecretStorage = true

	svc, err := jimmsvc.NewService(context.Background(), p)
	c.Assert(err, qt.IsNil)
	defer svc.Cleanup()

	srv := httptest.NewServer(svc)
	c.Cleanup(srv.Close)

	url, err := url.Parse(srv.URL + "/debug/info")
	c.Assert(err, qt.IsNil)
	// Invalid origin won't receive CORS headers.
	req := http.Request{
		Method: "GET",
		URL:    url,
		Header: http.Header{"Origin": []string{"123"}},
	}
	response, err := srv.Client().Do(&req)
	c.Assert(err, qt.IsNil)
	defer response.Body.Close()
	c.Assert(response.StatusCode, qt.Equals, http.StatusOK)
	c.Assert(response.Header.Get("Access-Control-Allow-Credentials"), qt.Equals, "")
	c.Assert(response.Header.Get("Access-Control-Allow-Origin"), qt.Equals, "")

	// Valid origin should receive CORS headers.
	req.Header = http.Header{"Origin": []string{allowedOrigin}}
	response, err = srv.Client().Do(&req)
	c.Assert(err, qt.IsNil)
	defer response.Body.Close()
	c.Assert(response.StatusCode, qt.Equals, http.StatusOK)
	c.Assert(response.Header.Get("Access-Control-Allow-Credentials"), qt.Equals, "true")
	c.Assert(response.Header.Get("Access-Control-Allow-Origin"), qt.Equals, allowedOrigin)
}
