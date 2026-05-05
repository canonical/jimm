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
	qt "github.com/frankban/quicktest"
	"github.com/juju/juju/api"
	"github.com/juju/juju/core/network"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v5"
	"github.com/lestrrat-go/jwx/v2/jwa"
	"github.com/lestrrat-go/jwx/v2/jwk"

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
	"github.com/canonical/jimm/v3/internal/openfga"
	ofganames "github.com/canonical/jimm/v3/internal/openfga/names"
	"github.com/canonical/jimm/v3/internal/river"
	"github.com/canonical/jimm/v3/internal/testutils/testdb"
)

//go:embed testdata/jwks_private_key.pem
var testJWKSPrivateKey []byte

// ControllerUUID is the UUID of the JIMM controller used in tests.
const ControllerUUID = "c1991ce8-96c2-497d-8e2a-e0cc42ca3aca"

// A JIMMEnv is an environment that initialises a JIMM for tests.
type JIMMEnv struct {
	// JIMM is the service that can be used in tests.
	// The JIMM configured does not have an Authenticator configured by default.
	JIMM *jimm.JIMM

	AdminUser   *openfga.User
	OFGAClient  *openfga.OFGAClient
	COFGAClient *cofga.Client

	Server         *httptest.Server
	deviceFlowChan chan string
	service        *jimmsvc.Service
}

func SetupJimmEnv(c *qt.C, opts ...SetupOption) JIMMEnv {
	// TODO: We can't modify the context on the *testing.T that is embedded in the *qt.C,
	// so callers won't have the test logger in their context.
	ctx := SetupTestLogger(c)
	o := applySetupOptions(opts)
	s := JIMMEnv{}

	// Setup OpenFGA.
	var err error
	s.OFGAClient, s.COFGAClient, _, err = SetupTestOFGAClient(c.Name())
	c.Assert(err, qt.IsNil)

	database := &db.Database{DB: testdb.PostgresDB(c, time.Now)}
	c.Cleanup(func() {
		database.Close()
	})

	// #nosec G101 fixed test signing keys
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

	riverClient, err := river.NewRiverClient(database)
	c.Assert(err, qt.IsNil)

	credentialStore := NewInMemoryCredentialStore()
	jwksService, err := NewStaticJWKSService(c)
	c.Assert(err, qt.IsNil)

	jwtExpiry := params.JWTExpiryDuration
	if jwtExpiry == 0 {
		jwtExpiry = 24 * time.Hour
	}

	jwtService := jimmjwx.NewJWTService(jimmjwx.JWTServiceParams{
		Host:   params.PublicDNSName,
		Expiry: jwtExpiry,
		JWKS:   jwksService,
	})

	dialer := jujuclient.NewDialer(jwtService, ControllerUUID)

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
		JWKSService:                   jwksService,
	}

	if o.useRealAuthN {
		authSvc := s.realAuthenticationService(c, database)
		deps.OAuthAuthenticator = authSvc
		deps.MigrationTokenGenerator = authSvc

		oauthHandler, err := jimmhttp.NewOAuthHandler(jimmhttp.OAuthHandlerParams{
			Authenticator:             authSvc,
			DashboardFinalRedirectURL: params.DashboardFinalRedirectURL,
		})
		c.Assert(err, qt.IsNil)
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
	c.Assert(err, qt.IsNil)
	c.Cleanup(s.service.Cleanup)

	s.JIMM = s.service.JIMM()
	s.OFGAClient = s.JIMM.OpenFGAClient

	err = river.MigrateRiver(ctx, s.JIMM.Database)
	c.Assert(err, qt.IsNil)

	alice, err := dbmodel.NewIdentity("alice@canonical.com")
	c.Assert(err, qt.IsNil)
	err = s.JIMM.Database.GetIdentity(ctx, alice)
	c.Assert(err, qt.Equals, nil)

	s.AdminUser = openfga.NewUser(alice, s.OFGAClient)
	s.AdminUser.JimmAdmin = true
	s.Server = httptest.NewServer(s.service)
	c.Cleanup(s.Server.Close)

	err = s.AdminUser.SetControllerAccess(ctx, s.JIMM.ResourceTag(), ofganames.AdministratorRelation)
	c.Assert(err, qt.Equals, nil)
	return s
}

// jwkSetFromPrivateKeyFile reads a PEM-encoded RSA private key from a file
// and returns a JWK set containing the public key along with the private key PEM bytes.
// This is useful for testing scenarios where you want to use a pre-existing key.
func jwkSetFromPrivateKeyFile() (jwk.Set, []byte, error) {
	block, _ := pem.Decode(testJWKSPrivateKey)
	if block == nil {
		return nil, nil, errors.New("failed to decode PEM block from private key file")
	}

	privateKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return nil, nil, err
	}

	jwks, err := jwk.FromRaw(privateKey.PublicKey)
	if err != nil {
		return nil, nil, err
	}

	if err := jwks.Set(jwk.KeyIDKey, "test-kid"); err != nil {
		return nil, nil, err
	}

	if err := jwks.Set(jwk.KeyUsageKey, "sig"); err != nil {
		return nil, nil, err
	}

	if err := jwks.Set(jwk.AlgorithmKey, jwa.RS256); err != nil {
		return nil, nil, err
	}

	ks := jwk.NewSet()
	if err := ks.AddKey(jwks); err != nil {
		return nil, nil, err
	}

	return ks, testJWKSPrivateKey, nil
}

func (s *JIMMEnv) realAuthenticationService(c *qt.C, db *db.Database) *auth.AuthenticationService {
	sqldb, err := db.DB.DB()
	c.Assert(err, qt.IsNil)

	sessionStore, err := pgstore.NewPGStoreFromPool(sqldb, []byte("secretsecretdigletts"))
	c.Assert(err, qt.IsNil)
	c.Cleanup(func() {
		sessionStore.Close()
	})

	// #nosec G101 fixed test secret
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
	c.Assert(err, qt.IsNil)
	return authSvc
}

func (s *JIMMEnv) AddAdminUser(c *qt.C, email string) {
	identity, err := dbmodel.NewIdentity(email)
	c.Assert(err, qt.IsNil)

	err = s.JIMM.Database.GetIdentity(context.Background(), identity)
	c.Assert(err, qt.IsNil)
	// Set the display name of the identity.
	displayName, _, _ := strings.Cut(email, "@")
	identity.DisplayName = displayName
	err = s.JIMM.Database.UpdateIdentity(context.Background(), identity)
	c.Assert(err, qt.IsNil)
	// Give the identity admin permission.
	ofgaUser := openfga.NewUser(identity, s.OFGAClient)
	err = ofgaUser.SetControllerAccess(context.Background(), s.JIMM.ResourceTag(), ofganames.AdministratorRelation)
	c.Assert(err, qt.IsNil)
}

func (s *JIMMEnv) AddUser(c *qt.C, email string) {
	identity, err := dbmodel.NewIdentity(email)
	c.Assert(err, qt.IsNil)

	err = s.JIMM.Database.GetIdentity(context.Background(), identity)
	c.Assert(err, qt.IsNil)
}

func (s *JIMMEnv) NewUser(u *dbmodel.Identity) *openfga.User {
	return openfga.NewUser(u, s.OFGAClient)
}

func (s *JIMMEnv) AddController(c *qt.C, name string, info *api.Info) *dbmodel.Controller {
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
		c.Assert(err, qt.Equals, nil)
		ctl.Addresses = append(ctl.Addresses, []jujuparams.HostPort{{
			Address: jujuparams.FromMachineAddress(hp.MachineAddress),
			Port:    hp.Port(),
		}})
	}
	ctl.TLSHostname = "juju-apiserver"
	err := s.JIMM.JujuManager.AddController(context.Background(), s.AdminUser, ctl, ctlCreds)
	c.Assert(err, qt.Equals, nil)
	return ctl
}

func (s *JIMMEnv) UpdateCloudCredential(c *qt.C, tag names.CloudCredentialTag, cred jujuparams.CloudCredential) {
	ctx := context.Background()
	u, err := dbmodel.NewIdentity(tag.Owner().Id())
	c.Assert(err, qt.IsNil)

	user := openfga.NewUser(u, s.JIMM.OpenFGAClient)
	err = s.JIMM.Database.GetIdentity(ctx, u)
	c.Assert(err, qt.Equals, nil)
	_, err = s.JIMM.JujuManager.UpdateCloudCredential(ctx, user, juju.UpdateCloudCredentialArgs{
		CredentialTag: tag,
		Credential:    cred,
		SkipCheck:     true,
	})
	c.Assert(err, qt.Equals, nil)
}

type AddModelArgs struct {
	Owner                names.UserTag
	Name                 string
	Cloud                names.CloudTag
	Region               string
	Cred                 names.CloudCredentialTag
	TargetControllerName string
}

// AddModelWithCleanup adds a model to JIMM with the given arguments and returns the model tag.
// The model will be destroyed and removed from the database when the test finishes.
func (s *JIMMEnv) AddModelWithCleanup(c *qt.C, args AddModelArgs) names.ModelTag {
	ctx := context.Background()

	u, err := dbmodel.NewIdentity(args.Owner.Id())
	c.Assert(err, qt.IsNil)

	err = s.JIMM.Database.GetIdentity(ctx, u)
	c.Assert(err, qt.Equals, nil)
	modelCreateArgs := &juju.ModelCreateArgs{
		Name:            args.Name,
		Owner:           args.Owner,
		Cloud:           args.Cloud,
		CloudRegion:     args.Region,
		CloudCredential: args.Cred,
		ControllerName:  args.TargetControllerName,
	}

	mi, err := s.JIMM.JujuManager.AddModel(ctx, s.NewUser(u), modelCreateArgs)
	c.Assert(err, qt.Equals, nil, qt.Commentf("failed to add model %q for owner %q on cloud %q region %q with cred %q: %v", args.Name, args.Owner.String(), args.Cloud.String(), args.Region, args.Cred.String(), err))
	mt := names.NewModelTag(mi.UUID)
	c.Cleanup(func() {
		s.DestroyModelAndDeleteFromDatabase(c, mt)
	})
	return mt
}

func (s *JIMMEnv) DestroyModelAndDeleteFromDatabase(c *qt.C, modelTag names.ModelTag) {
	ctx := context.Background()

	// Call Juju DestroyModel API and set the model to dying state.
	err := s.JIMM.JujuManager.DestroyModel(ctx, s.AdminUser, modelTag, nil, nil, nil, nil)
	if errors.ErrorCode(err) == errors.CodeNotFound {
		return
	}
	c.Assert(err, qt.Equals, nil)

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
			c.Assert(err, qt.IsNil)
		}
	}
}

// RemoveCloud removes a cloud from JIMM and the backing controller.
func (s *JIMMEnv) RemoveCloud(c *qt.C, cloudName string) {
	err := s.JIMM.JujuManager.RemoveCloud(context.Background(), s.AdminUser, names.NewCloudTag(cloudName))
	c.Assert(err, qt.Equals, nil)
}

func (s *JIMMEnv) AddGroup(c *qt.C, groupName string) dbmodel.GroupEntry {
	ctx := context.Background()
	group, err := s.JIMM.GroupManager.AddGroup(ctx, s.AdminUser, groupName)
	c.Assert(err, qt.Equals, nil)
	return *group
}

func (s *JIMMEnv) AddRole(c *qt.C, roleName string) dbmodel.RoleEntry {
	ctx := context.Background()
	role, err := s.JIMM.RoleManager.AddRole(ctx, s.AdminUser, roleName)
	c.Assert(err, qt.Equals, nil)
	return *role
}

// EnableDeviceFlow allows a test to use the device flow.
// Call this non-blocking function before login to ensure the device flow won't block.
//
// This is necessary as the mock authenticator simulates polling an external OIDC server.
func (s *JIMMEnv) EnableDeviceFlow(username string) {
	s.deviceFlowChan <- username
}

type mockMigrationTokenGenerator struct{}

func (m mockMigrationTokenGenerator) NewMigrationToken(ctx context.Context, username string) (string, error) {
	// Simulate a token generation by returning a simple string.
	// In a real implementation, this would be a JWT or similar token.
	return "test-migration-token", nil
}
