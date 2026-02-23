// Copyright 2025 Canonical.

package jimmtest

import (
	"context"
	"crypto/x509"
	_ "embed"
	"encoding/pem"
	"net/http/httptest"
	"strings"
	"time"

	"github.com/antonlindstrom/pgstore"
	cofga "github.com/canonical/ofga"
	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/juju/juju/api"
	"github.com/juju/juju/core/network"
	corejujutesting "github.com/juju/juju/juju/testing"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v5"
	"github.com/lestrrat-go/jwx/v2/jwa"
	"github.com/lestrrat-go/jwx/v2/jwk"
	gc "gopkg.in/check.v1"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	jimmsvc "github.com/canonical/jimm/v3/cmd/jimmsrv/service"
	"github.com/canonical/jimm/v3/internal/auth"
	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/jimm"
	"github.com/canonical/jimm/v3/internal/jimm/juju"
	"github.com/canonical/jimm/v3/internal/jimmhttp"
	"github.com/canonical/jimm/v3/internal/jimmjwx"
	"github.com/canonical/jimm/v3/internal/jujuclient"
	"github.com/canonical/jimm/v3/internal/logger"
	"github.com/canonical/jimm/v3/internal/openfga"
	ofganames "github.com/canonical/jimm/v3/internal/openfga/names"
	"github.com/canonical/jimm/v3/internal/river"
	"github.com/canonical/jimm/v3/internal/testutils/testdb"
)

//go:embed testdata/jwks_private_key.pem
var testJWKSPrivateKey []byte

// ControllerUUID is the UUID of the JIMM controller used in tests.
const ControllerUUID = "c1991ce8-96c2-497d-8e2a-e0cc42ca3aca"

// A GocheckTester adapts a gc.C to the Tester interface.
type GocheckTester struct {
	*gc.C
	AddCleanup func(func())
}

// Name implements Tester.Name.
func (t GocheckTester) Name() string {
	return t.TestName()
}

func (t GocheckTester) Cleanup(f func()) {
	if t.AddCleanup != nil {
		t.AddCleanup(f)
	} else {
		t.Logf("warning: gocheck does not support Cleanup functions; make sure you're using suite's tear-down method")
	}
}

// jimmModifiers controls how JIMM is initialised.
type jimmModifiers struct {
	useRealAuthN     bool
	useHardcodedJWKS bool
}

// A JIMMSuite is a suite that initialises a JIMM.
type JIMMSuite struct {
	// JIMM is a JIMM that can be used in tests. JIMM is initialised in
	// SetUpTest. The JIMM configured in this suite does not have an
	// Authenticator configured.
	JIMM *jimm.JIMM

	AdminUser   *openfga.User
	OFGAClient  *openfga.OFGAClient
	COFGAClient *cofga.Client
	COFGAParams *cofga.OpenFGAParams

	Server         *httptest.Server
	cancel         context.CancelFunc
	deviceFlowChan chan string
	cleanup        []func()
	service        *jimmsvc.Service

	modifiers jimmModifiers
}

func (s *JIMMSuite) SetUpTest(c *gc.C) {
	var err error
	s.cleanup = nil

	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel

	// Setup OpenFGA.
	s.OFGAClient, s.COFGAClient, s.COFGAParams, err = SetupTestOFGAClient(c.TestName())
	c.Assert(err, gc.IsNil)

	gct := &GocheckTester{
		C: c,
		AddCleanup: func(f func()) {
			s.cleanup = append(s.cleanup, f)
		},
	}

	dsn := testdb.CreateEmptyDatabase(gct)

	params := jimmsvc.Params{
		ControllerUUID:                ControllerUUID,
		PrivateKey:                    "ly/dzsI9Nt/4JxUILQeAX79qZ4mygDiuYGqc2ZEiDEc=",
		PublicKey:                     "izcYsQy3TePp6bLjqOo3IRPFvkQd2IKtyODGqC6SdFk=",
		MacaroonExpiryDuration:        time.Hour,
		JWTExpiryDuration:             time.Minute,
		PublicDNSName:                 "127.0.0.1",
		CrossModelQueryTimeout:        time.Second * 5,
		BootstrapLoginTokenRefreshURL: "https://jimm.localhost/.well-known/jwks.json",
		DashboardFinalRedirectURL:     "localhost", // Can be any URL.
	}

	gormDB, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.NewGormTestLogger(gct)})
	c.Assert(err, gc.IsNil)
	database := &db.Database{DB: gormDB}

	riverClient, err := river.NewRiverClient(database)
	c.Assert(err, gc.IsNil)

	credentialStore := NewInMemoryCredentialStore()

	jwtExpiry := params.JWTExpiryDuration
	if jwtExpiry == 0 {
		jwtExpiry = 24 * time.Hour
	}

	jwtService := jimmjwx.NewJWTService(jimmjwx.JWTServiceParams{
		Host:   params.PublicDNSName,
		Store:  credentialStore,
		Expiry: jwtExpiry,
	})

	dialer := &jujuclient.Dialer{
		ControllerCredentialsStore: credentialStore,
		JWTService:                 jwtService,
	}

	deps := &jimmsvc.ServiceDependencies{
		ControllerUUID:                params.ControllerUUID,
		PublicDNSName:                 params.PublicDNSName,
		PublicDNSHost:                 params.PublicDNSName,
		CrossModelQueryTimeout:        params.CrossModelQueryTimeout,
		BootstrapLoginTokenRefreshURL: params.BootstrapLoginTokenRefreshURL,
		MacaroonExpiryDuration:        params.MacaroonExpiryDuration,
		DischargerPrivateKey:          params.PrivateKey,
		DischargerPublicKey:           params.PublicKey,
		Database:                      database,
		Client:                        jimm.NewDialerAdapter(dialer),
		RiverClient:                   riverClient,
		OpenFGAClient:                 s.OFGAClient,
		CredentialStore:               credentialStore,
		JWTService:                    jwtService,
		JWKSService:                   jimmjwx.NewJWKSService(credentialStore),
	}

	if s.modifiers.useRealAuthN {
		authSvc := s.realAuthenticationService(c, database)
		deps.OAuthAuthenticator = authSvc
		deps.MigrationTokenGenerator = authSvc

		oauthHandler, err := jimmhttp.NewOAuthHandler(jimmhttp.OAuthHandlerParams{
			Authenticator:             authSvc,
			DashboardFinalRedirectURL: params.DashboardFinalRedirectURL,
		})
		c.Assert(err, gc.IsNil)
		deps.OAuthHandler = oauthHandler
		s.deviceFlowChan = nil
	} else {
		s.deviceFlowChan = make(chan string, 1)
		a := newMockOAuthAuthenticator(c, s.deviceFlowChan)
		deps.OAuthAuthenticator = &a
		deps.MigrationTokenGenerator = mockMigrationTokenGenerator{}
		deps.OAuthHandler = nil // This field can be nil to disable browser auth.
	}

	s.service, err = jimmsvc.NewServiceFromDependencies(ctx, deps)
	c.Assert(err, gc.IsNil)
	s.JIMM = s.service.JIMM()
	s.OFGAClient = s.JIMM.OpenFGAClient

	err = river.MigrateRiver(ctx, s.JIMM.Database)
	c.Assert(err, gc.IsNil)

	if s.modifiers.useHardcodedJWKS {
		set, privateKey, err := jwkSetFromPrivateKeyFile()
		c.Assert(err, gc.IsNil)
		err = s.JIMM.CredentialStore.PutJWKS(ctx, set)
		c.Assert(err, gc.IsNil)
		err = s.JIMM.CredentialStore.PutJWKSPrivateKey(ctx, privateKey)
		c.Assert(err, gc.IsNil)
		err = s.JIMM.CredentialStore.PutJWKSExpiry(ctx, time.Now().UTC().AddDate(10, 0, 0))
		c.Assert(err, gc.IsNil)
	} else {
		err = s.service.StartJWKSRotator(ctx, time.NewTicker(time.Hour).C, time.Now().UTC().AddDate(0, 3, 0))
		c.Assert(err, gc.IsNil)
	}

	alice, err := dbmodel.NewIdentity("alice@canonical.com")
	c.Assert(err, gc.IsNil)
	err = s.JIMM.Database.GetIdentity(ctx, alice)
	c.Assert(err, gc.Equals, nil)

	s.AdminUser = openfga.NewUser(alice, s.OFGAClient)
	s.AdminUser.JimmAdmin = true
	s.Server = httptest.NewServer(s.service)

	err = s.AdminUser.SetControllerAccess(ctx, s.JIMM.ResourceTag(), ofganames.AdministratorRelation)
	c.Assert(err, gc.Equals, nil)
}

// jwkSetFromPrivateKeyFile reads a PEM-encoded RSA private key from a file
// and returns a JWK set containing the public key along with the private key PEM bytes.
// This is useful for testing scenarios where you want to use a pre-existing key.
func jwkSetFromPrivateKeyFile() (jwk.Set, []byte, error) {
	block, _ := pem.Decode(testJWKSPrivateKey)
	if block == nil {
		return nil, nil, errors.E("failed to decode PEM block from private key file")
	}

	privateKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return nil, nil, errors.E(err)
	}

	jwks, err := jwk.FromRaw(privateKey.PublicKey)
	if err != nil {
		return nil, nil, errors.E(err)
	}

	if err := jwks.Set(jwk.KeyIDKey, "test-kid"); err != nil {
		return nil, nil, errors.E(err)
	}

	if err := jwks.Set(jwk.KeyUsageKey, "sig"); err != nil {
		return nil, nil, errors.E(err)
	}

	if err := jwks.Set(jwk.AlgorithmKey, jwa.RS256); err != nil {
		return nil, nil, errors.E(err)
	}

	ks := jwk.NewSet()
	if err := ks.AddKey(jwks); err != nil {
		return nil, nil, errors.E(err)
	}

	return ks, testJWKSPrivateKey, nil
}

func (s *JIMMSuite) UseHardcodedJWKS(c *gc.C) {
	s.modifiers.useHardcodedJWKS = true
}

func (s *JIMMSuite) TearDownTest(c *gc.C) {
	if s.cancel != nil {
		s.cancel()
	}
	if s.Server != nil {
		s.Server.Close()
	}
	if s.service != nil {
		s.service.Cleanup()
	}
	if s.JIMM != nil && s.JIMM.Database != nil {
		if err := s.JIMM.Database.Close(); err != nil {
			c.Logf("failed to close database connections at tear down: %s", err)
		}
	}

	for i := len(s.cleanup) - 1; i >= 0; i-- {
		s.cleanup[i]()
	}
	s.cleanup = nil
}

func (s *JIMMSuite) UseRealAuthentication(c *gc.C) {
	s.modifiers.useRealAuthN = true
}

func (s *JIMMSuite) realAuthenticationService(c *gc.C, db *db.Database) *auth.AuthenticationService {
	sqldb, err := db.DB.DB()
	c.Assert(err, gc.IsNil)

	sessionStore, err := pgstore.NewPGStoreFromPool(sqldb, []byte("secretsecretdigletts"))
	c.Assert(err, gc.IsNil)
	s.cleanup = append(s.cleanup, func() {
		sessionStore.Close()
	})

	authSvc, err := auth.NewAuthenticationService(context.Background(), auth.AuthenticationServiceParams{
		IssuerURL:           "http://localhost:8082/realms/jimm",
		ClientID:            "jimm-device",
		ClientSecret:        "SwjDofnbDzJDm9iyfUhEp67FfUFMY8L4",
		Scopes:              []string{oidc.ScopeOpenID, "profile", "email"},
		SessionTokenExpiry:  time.Hour,
		Store:               db,
		SessionStore:        sessionStore,
		SessionCookieMaxAge: 60,
		JWTSessionKey:       "test-secret",
		SecureCookies:       false,
	})
	c.Assert(err, gc.IsNil)
	return authSvc
}

func (s *JIMMSuite) AddAdminUser(c *gc.C, email string) {
	identity, err := dbmodel.NewIdentity(email)
	c.Assert(err, gc.IsNil)

	err = s.JIMM.Database.GetIdentity(context.Background(), identity)
	c.Assert(err, gc.IsNil)
	// Set the display name of the identity.
	displayName, _, _ := strings.Cut(email, "@")
	identity.DisplayName = displayName
	err = s.JIMM.Database.UpdateIdentity(context.Background(), identity)
	c.Assert(err, gc.IsNil)
	// Give the identity admin permission.
	ofgaUser := openfga.NewUser(identity, s.OFGAClient)
	err = ofgaUser.SetControllerAccess(context.Background(), s.JIMM.ResourceTag(), ofganames.AdministratorRelation)
	c.Assert(err, gc.IsNil)
}

func (s *JIMMSuite) AddUser(c *gc.C, email string) {
	identity, err := dbmodel.NewIdentity(email)
	c.Assert(err, gc.IsNil)

	err = s.JIMM.Database.GetIdentity(context.Background(), identity)
	c.Assert(err, gc.IsNil)
}

func (s *JIMMSuite) NewUser(u *dbmodel.Identity) *openfga.User {
	return openfga.NewUser(u, s.OFGAClient)
}

func (s *JIMMSuite) AddController(c *gc.C, name string, info *api.Info) *dbmodel.Controller {
	ctl := &dbmodel.Controller{
		UUID:          info.ControllerUUID,
		Name:          name,
		CACertificate: info.CACert,
		Addresses:     nil,
	}
	ctlCreds := juju.ControllerCreds{
		AdminIdentityName: info.Tag.Id(),
		AdminPassword:     info.Password,
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
	ctl.TLSHostname = "juju-apiserver"
	err := s.JIMM.JujuManager.AddController(context.Background(), s.AdminUser, ctl, ctlCreds)
	c.Assert(err, gc.Equals, nil)
	return ctl
}

func (s *JIMMSuite) UpdateCloudCredential(c *gc.C, tag names.CloudCredentialTag, cred jujuparams.CloudCredential) {
	ctx := context.Background()
	u, err := dbmodel.NewIdentity(tag.Owner().Id())
	c.Assert(err, gc.IsNil)

	user := openfga.NewUser(u, s.JIMM.OpenFGAClient)
	err = s.JIMM.Database.GetIdentity(ctx, u)
	c.Assert(err, gc.Equals, nil)
	_, err = s.JIMM.JujuManager.UpdateCloudCredential(ctx, user, juju.UpdateCloudCredentialArgs{
		CredentialTag: tag,
		Credential:    cred,
		SkipCheck:     true,
	})
	c.Assert(err, gc.Equals, nil)
}

type addModelArgs struct {
	owner                names.UserTag
	name                 string
	cloud                names.CloudTag
	region               string
	cred                 names.CloudCredentialTag
	targetControllerName string
}

func (s *JIMMSuite) AddModel(c *gc.C, args addModelArgs) names.ModelTag {
	ctx := context.Background()

	u, err := dbmodel.NewIdentity(args.owner.Id())
	c.Assert(err, gc.IsNil)

	err = s.JIMM.Database.GetIdentity(ctx, u)
	c.Assert(err, gc.Equals, nil)
	modelCreateArgs := &juju.ModelCreateArgs{
		Name:            args.name,
		Owner:           args.owner,
		Cloud:           args.cloud,
		CloudRegion:     args.region,
		CloudCredential: args.cred,
	}
	if args.targetControllerName != "" {
		modelCreateArgs.ControllerName = args.targetControllerName
	}

	mi, err := s.JIMM.JujuManager.AddModel(ctx, s.NewUser(u), modelCreateArgs)
	c.Assert(err, gc.Equals, nil, gc.Commentf("failed to add model %q for owner %q on cloud %q region %q with cred %q: %v", args.name, args.owner.String(), args.cloud.String(), args.region, args.cred.String(), err))
	return names.NewModelTag(mi.UUID)
}

func (s *JIMMSuite) DestroyModelAndDeleteFromDatabase(c *gc.C, modelTag names.ModelTag) {
	ctx := context.Background()

	// Call Juju DestroyModel API and set the model to dying state.
	err := s.JIMM.JujuManager.DestroyModel(ctx, s.AdminUser, modelTag, nil, nil, nil, nil)
	c.Assert(err, gc.Equals, nil)

	// Poll until the model is destroyed with a timeout
	timeout := time.After(5 * 60 * time.Second)
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			c.Fatalf("timeout waiting for model to be destroyed")
		case <-ticker.C:
			_, err := s.JIMM.JujuManager.ModelInfo(ctx, s.AdminUser, modelTag)
			if errors.ErrorCode(err) == errors.CodeNotFound || errors.ErrorCode(err) == errors.CodeUnauthorized {
				return
			}
			c.Assert(err, gc.IsNil)
		}
	}
}

// RemoveCloud removes a cloud from JIMM and the backing controller.
func (s *JIMMSuite) RemoveCloud(c *gc.C, cloudName string) {
	err := s.JIMM.JujuManager.RemoveCloud(context.Background(), s.AdminUser, names.NewCloudTag(cloudName))
	c.Assert(err, gc.Equals, nil)
}

func (s *JIMMSuite) AddGroup(c *gc.C, groupName string) dbmodel.GroupEntry {
	ctx := context.Background()
	group, err := s.JIMM.GroupManager.AddGroup(ctx, s.AdminUser, groupName)
	c.Assert(err, gc.Equals, nil)
	return *group
}

func (s *JIMMSuite) AddRole(c *gc.C, roleName string) dbmodel.RoleEntry {
	ctx := context.Background()
	role, err := s.JIMM.RoleManager.AddRole(ctx, s.AdminUser, roleName)
	c.Assert(err, gc.Equals, nil)
	return *role
}

// EnableDeviceFlow allows a test to use the device flow.
// Call this non-blocking function before login to ensure the device flow won't block.
//
// This is necessary as the mock authenticator simulates polling an external OIDC server.
func (s *JIMMSuite) EnableDeviceFlow(username string) {
	s.deviceFlowChan <- username
}

// A JujuSuite is a suite that intialises a JIMM and adds the testing juju
// controller.
type JujuSuite struct {
	JIMMSuite
	corejujutesting.JujuConnSuite
	LoggingSuite
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
	s.JIMMSuite.SetUpTest(c)
	s.ControllerConfigAttrs = map[string]interface{}{
		"login-token-refresh-url": s.Server.URL + "/.well-known/jwks.json",
	}
	s.JujuConnSuite.SetUpTest(c)
	s.LoggingSuite.SetUpTest(c)

	s.AddController(c, "controller-1", s.APIInfo(c))
}

func (s *JujuSuite) TearDownTest(c *gc.C) {
	s.LoggingSuite.TearDownTest(c)
	s.JujuConnSuite.TearDownTest(c)
	s.JIMMSuite.TearDownTest(c)
}

type mockMigrationTokenGenerator struct{}

func (m mockMigrationTokenGenerator) NewMigrationToken(ctx context.Context, username string) (string, error) {
	// Simulate a token generation by returning a simple string.
	// In a real implementation, this would be a JWT or similar token.
	return "test-migration-token", nil
}
