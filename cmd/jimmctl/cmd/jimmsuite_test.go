// Copyright 2021 Canonical Ltd.

package cmd_test

import (
	"bytes"
	"context"
	"encoding/pem"
	"fmt"
	"net/http/httptest"
	"net/url"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/httpbakery"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/httpbakery/agent"
	"github.com/juju/juju/api"
	"github.com/juju/juju/core/network"
	jjclient "github.com/juju/juju/jujuclient"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v4"
	gc "gopkg.in/check.v1"

	service "github.com/CanonicalLtd/jimm"
	"github.com/CanonicalLtd/jimm/internal/db"
	"github.com/CanonicalLtd/jimm/internal/dbmodel"
	"github.com/CanonicalLtd/jimm/internal/jimm"
	"github.com/CanonicalLtd/jimm/internal/jimmtest"
	"github.com/CanonicalLtd/jimm/internal/jujuclient"
	ofga "github.com/CanonicalLtd/jimm/internal/openfga"
)

type gcTester struct {
	*gc.C
}

func (t *gcTester) Name() string {
	return t.C.TestName()
}

type jimmSuite struct {
	jimmtest.CandidSuite
	jimmtest.JujuSuite

	Params      service.Params
	HTTP        *httptest.Server
	Service     *service.Service
	AdminUser   *dbmodel.User
	ClientStore *jjclient.MemStore
	JIMM        *jimm.JIMM
}

func (s *jimmSuite) SetUpTest(c *gc.C) {
	ctx := context.Background()

	s.ControllerAdmins = []string{"controller-admin"}
	s.CandidSuite.SetUpTest(c)
	s.JujuConnSuite.SetUpTest(c)

	s.JIMM = &jimm.JIMM{
		Database: db.Database{
			DB: jimmtest.MemoryDB(&gcTester{c}, nil),
		},
		Dialer: &jujuclient.Dialer{},
	}
	err := s.JIMM.Database.Migrate(context.Background(), true)
	c.Assert(err, gc.Equals, nil)

	s.Params = service.Params{
		ControllerUUID:   "914487b5-60e7-42bb-bd63-1adc3fd3a388",
		CandidURL:        s.Candid.URL.String(),
		CandidPublicKey:  s.CandidPublicKey,
		ControllerAdmins: []string{"admin"},
		DSN:              fmt.Sprintf("file:%s?mode=memory&cache=shared", c.TestName()),
		OpenFGAParams: service.OpenFGAParams{
			Scheme:    ofga.OpenFGATestConfig.ApiScheme,
			Host:      ofga.OpenFGATestConfig.ApiHost,
			Store:     ofga.OpenFGATestConfig.StoreId,
			Token:     ofga.OpenFGATestConfig.Credentials.Config.ApiToken,
			AuthModel: ofga.OpenFGATestAuthModel,
		},
	}
	srv, err := service.NewService(ctx, s.Params)
	c.Assert(err, gc.Equals, nil)
	s.Service = srv

	s.HTTP = httptest.NewTLSServer(srv)

	ofgaAPI, ofgaClient := ofga.SetupTestOFGAClient()
	s.OFGAApi = ofgaAPI
	s.JIMM.OpenFGAClient = ofgaClient

	s.AdminUser = &dbmodel.User{
		Username:         "alice@external",
		ControllerAccess: "superuser",
		LastLogin:        db.Now(),
	}
	err = s.JIMM.Database.GetUser(ctx, s.AdminUser)
	c.Assert(err, gc.Equals, nil)

	s.Candid.AddUser("alice")

	u, err := url.Parse(s.HTTP.URL)
	c.Assert(err, gc.IsNil)

	w := new(bytes.Buffer)
	err = pem.Encode(w, &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: s.HTTP.TLS.Certificates[0].Certificate[0],
	})
	c.Assert(err, gc.Equals, nil)

	s.ClientStore = jjclient.NewMemStore()
	s.ClientStore.CurrentControllerName = "JIMM"
	s.ClientStore.Controllers["JIMM"] = jjclient.ControllerDetails{
		ControllerUUID: "914487b5-60e7-42bb-bd63-1adc3fd3a388",
		APIEndpoints:   []string{u.Host},
		PublicDNSName:  s.HTTP.URL,
		CACert:         w.String(),
	}
}

func (s *jimmSuite) TearDownTest(c *gc.C) {
	if s.HTTP != nil {
		s.HTTP.Close()
	}
	s.CandidSuite.TearDownTest(c)
	s.JujuConnSuite.TearDownTest(c)
}

func (s *jimmSuite) userBakeryClient(username string) *httpbakery.Client {
	s.Candid.AddUser(username)
	key := s.Candid.UserPublicKey(username)
	bClient := httpbakery.NewClient()
	bClient.Key = &bakery.KeyPair{
		Public:  bakery.PublicKey{Key: bakery.Key(key.Public.Key)},
		Private: bakery.PrivateKey{Key: bakery.Key(key.Private.Key)},
	}
	agent.SetUpAuth(bClient, &agent.AuthInfo{
		Key: bClient.Key,
		Agents: []agent.Agent{{
			URL:      s.Candid.URL.String(),
			Username: username,
		}},
	})
	return bClient
}

func (s *jimmSuite) AddController(c *gc.C, name string, info *api.Info) {
	ctl := &dbmodel.Controller{
		Name:          name,
		AdminUser:     info.Tag.Id(),
		AdminPassword: info.Password,
		CACertificate: info.CACert,
		Addresses:     nil,
	}
	ctl.Addresses = make(dbmodel.HostPorts, 0, len(info.Addrs))
	for _, addr := range info.Addrs {
		hp, err := network.ParseMachineHostPort(addr)
		c.Assert(err, gc.Equals, nil)
		ctl.Addresses = append(ctl.Addresses, []jujuparams.HostPort{{
			Address: jujuparams.FromMachineAddress(hp.MachineAddress),
			Port:    hp.Port(),
		}})
	}
	err := s.JIMM.AddController(context.Background(), s.AdminUser, ctl)
	c.Assert(err, gc.Equals, nil)
}

func (s *jimmSuite) UpdateCloudCredential(c *gc.C, tag names.CloudCredentialTag, cred jujuparams.CloudCredential) {
	ctx := context.Background()
	u := dbmodel.User{
		Username: tag.Owner().Id(),
	}
	err := s.JIMM.Database.GetUser(ctx, &u)
	c.Assert(err, gc.Equals, nil)
	_, err = s.JIMM.UpdateCloudCredential(ctx, &u, jimm.UpdateCloudCredentialArgs{
		CredentialTag: tag,
		Credential:    cred,
		SkipCheck:     true,
	})
	c.Assert(err, gc.Equals, nil)
}

func (s *jimmSuite) AddModel(c *gc.C, owner names.UserTag, name string, cloud names.CloudTag, region string, cred names.CloudCredentialTag) names.ModelTag {
	ctx := context.Background()
	u := dbmodel.User{
		Username: owner.Id(),
	}
	err := s.JIMM.Database.GetUser(ctx, &u)
	c.Assert(err, gc.Equals, nil)
	mi, err := s.JIMM.AddModel(ctx, &u, &jimm.ModelCreateArgs{
		Name:            name,
		Owner:           owner,
		Cloud:           cloud,
		CloudRegion:     region,
		CloudCredential: cred,
	})
	c.Assert(err, gc.Equals, nil)
	return names.NewModelTag(mi.UUID)
}
