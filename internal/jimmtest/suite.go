// Copyright 2020 Canonical Ltd.

package jimmtest

import (
	"context"

	"github.com/canonical/candid/candidtest"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery/identchecker"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/httpbakery"
	"github.com/juju/juju/api"
	jujuparams "github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/network"
	corejujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/names/v4"
	gc "gopkg.in/check.v1"

	"github.com/CanonicalLtd/jimm/internal/auth"
	"github.com/CanonicalLtd/jimm/internal/db"
	"github.com/CanonicalLtd/jimm/internal/dbmodel"
	"github.com/CanonicalLtd/jimm/internal/jimm"
	"github.com/CanonicalLtd/jimm/internal/jujuclient"
	"github.com/CanonicalLtd/jimm/internal/pubsub"
)

// ControllerUUID is the UUID of the JIMM controller used in tests.
const ControllerUUID = "c1991ce8-96c2-497d-8e2a-e0cc42ca3aca"

// A cTester adapts a gc.C to the Tester interface.
type cTester struct {
	*gc.C
}

// Name implements Tester.Name.
func (t cTester) Name() string {
	return t.C.TestName()
}

// A JIMMSuite is a suite that initialises a JIMM.
type JIMMSuite struct {
	// JIMM is a JIMM that can be used in tests. JIMM is initialised in
	// SetUpTest. The JIMM configured in this suite does not have an
	// Authenticator configured.
	JIMM *jimm.JIMM

	AdminUser *dbmodel.User
}

func (s *JIMMSuite) SetUpTest(c *gc.C) {
	s.JIMM = &jimm.JIMM{
		Database: db.Database{
			DB: MemoryDB(cTester{c}, nil),
		},
		Dialer: jujuclient.Dialer{},
		Pubsub: new(pubsub.Hub),
		UUID:   ControllerUUID,
	}
	ctx := context.Background()
	err := s.JIMM.Database.Migrate(ctx, false)
	c.Assert(err, gc.Equals, nil)
	s.AdminUser = &dbmodel.User{
		Username:         "alice@external",
		ControllerAccess: "superuser",
		LastLogin:        db.Now(),
	}
	err = s.JIMM.Database.GetUser(ctx, s.AdminUser)
	c.Assert(err, gc.Equals, nil)
}

func (s *JIMMSuite) TearDownTest(c *gc.C) {
	if s.JIMM != nil {
		s.JIMM = nil
	}
}

func (s *JIMMSuite) AddController(c *gc.C, name string, info *api.Info) {
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

func (s *JIMMSuite) UpdateCloudCredential(c *gc.C, tag names.CloudCredentialTag, cred jujuparams.CloudCredential) {
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

func (s *JIMMSuite) AddModel(c *gc.C, owner names.UserTag, name string, cloud names.CloudTag, region string, cred names.CloudCredentialTag) names.ModelTag {
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

// A CandidSuite is a suite that initialises a candid test system to use a
// jimm Authenticator.
type CandidSuite struct {
	// ControllerAdmins is the list of users and groups that are
	// controller adminstrators.
	ControllerAdmins []string

	// The following are created in SetUpTest
	Candid          *candidtest.Server
	CandidPublicKey string
	Authenticator   jimm.Authenticator
}

func (s *CandidSuite) SetUpTest(c *gc.C) {
	s.Candid = candidtest.NewServer()
	s.Candid.AddUser("agent-user", candidtest.GroupListGroup)
	s.Authenticator = auth.JujuAuthenticator{
		Bakery: identchecker.NewBakery(identchecker.BakeryParams{
			Locator:        auth.ThirdPartyLocatorV3{s.Candid},
			Key:            bakery.MustGenerateKey(),
			IdentityClient: auth.IdentityClientV3{s.Candid.CandidClient("agent-user")},
			Location:       "jimmtest",
		}),
		ControllerAdmins: s.ControllerAdmins,
	}
	tpi, err := httpbakery.ThirdPartyInfoForLocation(context.Background(), nil, s.Candid.URL.String())
	c.Assert(err, gc.Equals, nil)
	pk, err := tpi.PublicKey.MarshalText()
	c.Assert(err, gc.Equals, nil)
	s.CandidPublicKey = string(pk)

}

func (s *CandidSuite) TearDownTest(c *gc.C) {
	s.Authenticator = nil
	if s.Candid != nil {
		s.Candid.Close()
		s.Candid = nil
	}
}

// A JujuSuite is a suite that intialises a JIMM and adds the testing juju
// controller.
type JujuSuite struct {
	corejujutesting.JujuConnSuite
	LoggingSuite
	JIMMSuite

	//cancelWatcher func()
}

func (s *JujuSuite) SetUpSuite(c *gc.C) {
	s.JujuConnSuite.SetUpSuite(c)
	s.LoggingSuite.SetUpSuite(c)
}

func (s *JujuSuite) TearDownSuite(c *gc.C) {
	s.LoggingSuite.TearDownSuite(c)
	s.JujuConnSuite.TearDownSuite(c)
}

func (s *JujuSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.LoggingSuite.SetUpTest(c)
	s.JIMMSuite.SetUpTest(c)

	s.AddController(c, "controller-1", s.APIInfo(c))

	//var wctx context.Context
	//wctx, s.cancelWatcher = context.WithCancel(context.Background())
	//w := &jimm.Watcher{
	//	Database: s.JIMM.Database,
	//	Dialer:   s.JIMM.Dialer,
	//}
	//go func() {
	//	err := w.Watch(wctx, time.Second)
	//	c.Logf("watcher stopped: %s", err)
	//}()
}

func (s *JujuSuite) TearDownTest(c *gc.C) {
	//if s.cancelWatcher != nil {
	//	s.cancelWatcher()
	//}
	s.JIMMSuite.TearDownTest(c)
	s.LoggingSuite.TearDownTest(c)
	s.JujuConnSuite.TearDownTest(c)
}

type BootstrapSuite struct {
	JujuSuite

	CloudCredential *dbmodel.CloudCredential
	Model           *dbmodel.Model
}

func (s *BootstrapSuite) SetUpTest(c *gc.C) {
	s.JujuSuite.SetUpTest(c)

	cct := names.NewCloudCredentialTag(TestCloudName + "/bob@external/cred")
	s.UpdateCloudCredential(c, cct, jujuparams.CloudCredential{
		AuthType: "empty",
	})
	ctx := context.Background()
	s.CloudCredential = new(dbmodel.CloudCredential)
	s.CloudCredential.SetTag(cct)
	err := s.JIMM.Database.GetCloudCredential(ctx, s.CloudCredential)
	c.Assert(err, gc.Equals, nil)

	mt := s.AddModel(c, names.NewUserTag("bob@external"), "model-1", names.NewCloudTag(TestCloudName), TestCloudRegionName, cct)
	s.Model = new(dbmodel.Model)
	s.Model.SetTag(mt)
	err = s.JIMM.Database.GetModel(ctx, s.Model)
	c.Assert(err, gc.Equals, nil)
}
