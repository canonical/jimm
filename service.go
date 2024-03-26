// Copyright 2021 Canonical Ltd.

package jimm

import (
	"context"
	"crypto/rand"
	"database/sql"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/antonlindstrom/pgstore"
	cofga "github.com/canonical/ofga"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	vaultapi "github.com/hashicorp/vault/api"
	"github.com/juju/names/v5"
	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/canonical/jimm/internal/auth"
	"github.com/canonical/jimm/internal/dashboard"
	"github.com/canonical/jimm/internal/dbmodel"
	"github.com/canonical/jimm/internal/debugapi"
	"github.com/canonical/jimm/internal/discharger"
	"github.com/canonical/jimm/internal/errors"
	"github.com/canonical/jimm/internal/jimm"
	jimmcreds "github.com/canonical/jimm/internal/jimm/credentials"
	"github.com/canonical/jimm/internal/jimmhttp"
	"github.com/canonical/jimm/internal/jimmjwx"
	"github.com/canonical/jimm/internal/jujuapi"
	"github.com/canonical/jimm/internal/jujuclient"
	"github.com/canonical/jimm/internal/logger"
	"github.com/canonical/jimm/internal/openfga"
	ofganames "github.com/canonical/jimm/internal/openfga/names"
	"github.com/canonical/jimm/internal/pubsub"
	"github.com/canonical/jimm/internal/servermon"
	"github.com/canonical/jimm/internal/vault"
	"github.com/canonical/jimm/internal/wellknownapi"
)

const (
	localDischargePath = "/macaroons"
)

// OpenFGAParams holds parameters needed to connect to the OpenFGA server.
type OpenFGAParams struct {
	Scheme    string
	Host      string
	Store     string
	AuthModel string
	Token     string
	Port      string
}

// OAuthAuthenticatorParams holds parameters needed to configure an OAuthAuthenticator
// implementation.
type OAuthAuthenticatorParams struct {
	// IssuerURL is the URL of the OAuth2.0 server.
	// I.e., http://localhost:8082/realms/jimm in the case of keycloak.
	IssuerURL string

	// ClientID holds the OAuth2.0. The client IS expected to be confidential.
	ClientID string

	// ClientSecret holds the OAuth2.0 "client-secret" to authenticate when performing
	// /auth and /token requests.
	ClientSecret string

	// Scopes holds the scopes that you wish to retrieve.
	Scopes []string

	// SessionTokenExpiry holds the expiry duration for issued JWTs
	// for user (CLI) to JIMM authentication.
	SessionTokenExpiry time.Duration

	// SessionCookieMaxAge holds the max age for session cookies in seconds.
	SessionCookieMaxAge int
}

// A Params structure contains the parameters required to initialise a new
// Service.
type Params struct {
	// ControllerUUID contains the UUID of the JIMM controller, if this
	// is not set a random UUID will be generated.
	ControllerUUID string

	// DSN is the data source name that the JIMM service will use to
	// connect to its database. If this is empty an in-memory database
	// will be used.
	DSN string

	// ControllerAdmins contains a list of users (or groups)
	// that will be given the access-level "superuser" when they
	// authenticate to the controller.
	ControllerAdmins []string

	// DisableConnectionCache disables caching connections to
	// controllers. By default controller connections are cached, if
	// this is set then a new connection will be created for each API
	// call. This is mostly useful for testing.
	DisableConnectionCache bool

	// VaultSecretFile is the path of the file containing the secret to
	// use with the vault server. If this is empty then no attempt will
	// be made to use a vault server and JIMM will store everything in
	// it's local database.
	VaultSecretFile string

	// VaultAddress is the URL of a vault server that will be used to
	// store secrets for JIMM. If this is empty then the default
	// address of the vault server is used.
	VaultAddress string

	// VaultAuthPath is the path on the vault server that JIMM will use
	// to attempt to authenticate using the credentials in the
	// VaultSecretFile. If this is empty then authentication is not
	// attempted and the VaultSecretFile must contain token that can be
	// used directly.
	VaultAuthPath string

	// VaultPath is the path on the vault server which hosts the kv
	// secrets engine JIMM will use to store secrets.
	VaultPath string

	// DashboardLocation contains the location where the JAAS dashboard
	// can be found. If this location parses as an absolute URL then
	// requests to /dashboard will redirect to that URL. If this is a
	// filesystem path then the dashboard files will be served from
	// that path.
	DashboardLocation string

	// PublicDNSName is the name to advertise as the public address of
	// the juju controller.
	PublicDNSName string

	// Parameters used to initialize connection to an OpenFGA server.
	OpenFGAParams OpenFGAParams

	// PrivateKey holds the private part of the bakery keypair.
	PrivateKey string

	// PublicKey holds the public part of the bakery keypair.
	PublicKey string

	// auditLogRetentionPeriodInDays is the number of days detailing how long
	// to keep an audit log for before purging it from the database.
	AuditLogRetentionPeriodInDays string

	// MacaroonExpiryDuration holds the expiry duration of authentication macaroons.
	MacaroonExpiryDuration time.Duration

	// JWTExpiryDuration holds the expiry duration for issued JWTs
	// for controller to JIMM communication ONLY.
	JWTExpiryDuration time.Duration

	// InsecureSecretStorage instructs JIMM to store secrets in its database
	// instead of dedicated secure storage. SHOULD NOT BE USED IN PRODUCTION.
	InsecureSecretStorage bool

	// OAuthAuthenticatorParams holds parameters needed to configure an OAuthAuthenticator
	// implementation.
	OAuthAuthenticatorParams OAuthAuthenticatorParams

	// DashboardFinalRedirectURL is the URL to FINALLY redirect to after completing
	// the /callback in an authorisation code OAuth2.0 flow to finish the flow.
	DashboardFinalRedirectURL string

	// SecureSessionCookies determines if HTTPS must be enabled in order for JIMM
	// to set cookies when creating browser based sessions.
	SecureSessionCookies bool
}

// A Service is the implementation of a JIMM server.
type Service struct {
	jimm jimm.JIMM

	mux *chi.Mux
}

func (s *Service) JIMM() *jimm.JIMM {
	return &s.jimm
}

// ServeHTTP implements http.Handler.
func (s *Service) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	s.mux.ServeHTTP(w, req)
}

// WatchControllers connects to all controllers and starts an AllWatcher
// monitoring all changes to models. WatchControllers finishes when the
// given context is canceled, or there is a fatal error watching models.
func (s *Service) WatchControllers(ctx context.Context) error {
	w := jimm.Watcher{
		Database: s.jimm.Database,
		Dialer:   s.jimm.Dialer,
	}
	return w.Watch(ctx, 10*time.Minute)
}

// WatchModelSummaries connects to all controllers and starts a
// ModelSummaryWatcher for all models. WatchModelSummaries finishes when
// the given context is canceled, or there is a fatal error watching model
// summaries.
func (s *Service) WatchModelSummaries(ctx context.Context) error {
	w := jimm.Watcher{
		Database: s.jimm.Database,
		Dialer:   s.jimm.Dialer,
		Pubsub:   s.jimm.Pubsub,
	}
	return w.WatchAllModelSummaries(ctx, 10*time.Minute)
}

// StartJWKSRotator see internal/jimmjwx/jwks.go for details.
func (s *Service) StartJWKSRotator(ctx context.Context, checkRotateRequired <-chan time.Time, initialRotateRequiredTime time.Time) error {
	if s.jimm.JWKService == nil {
		zapctx.Warn(ctx, "not starting JWKS rotation")
		return nil
	}
	return s.jimm.JWKService.StartJWKSRotator(ctx, checkRotateRequired, initialRotateRequiredTime)
}

// NewService creates a new Service using the given params.
func NewService(ctx context.Context, p Params) (*Service, error) {
	const op = errors.Op("NewService")

	s := new(Service)
	s.mux = chi.NewRouter()

	// Setup all dependency services

	if p.ControllerUUID == "" {
		controllerUUID, err := uuid.NewRandom()
		if err != nil {
			return nil, errors.E(op, err)
		}
		p.ControllerUUID = controllerUUID.String()
	}
	s.jimm.UUID = p.ControllerUUID
	s.jimm.Pubsub = &pubsub.Hub{MaxConcurrency: 50}

	if p.DSN == "" {
		return nil, errors.E(op, "missing DSN")
	}

	var err error
	s.jimm.Database.DB, err = openDB(ctx, p.DSN)
	if err != nil {
		return nil, errors.E(op, err)
	}
	if err := s.jimm.Database.Migrate(ctx, false); err != nil {
		return nil, errors.E(op, err)
	}
	sqlDb, err := s.jimm.Database.DB.DB()
	if err != nil {
		return nil, errors.E(op, err)
	}

	// Setup browser session store
	sessionStore, err := setupSessionStore(sqlDb, "secret-key-todo")
	if err != nil {
		return nil, errors.E(op, err)
	}

	// Cleanup expired session every 30 minutes
	defer sessionStore.StopCleanup(sessionStore.Cleanup(time.Minute * 30))

	if p.AuditLogRetentionPeriodInDays != "" {
		period, err := strconv.Atoi(p.AuditLogRetentionPeriodInDays)
		if err != nil {
			return nil, errors.E(op, "failed to parse audit log retention period")
		}
		if period < 0 {
			return nil, errors.E(op, "retention period cannot be less than 0")
		}
		if period != 0 {
			jimm.NewAuditLogCleanupService(s.jimm.Database, period).Start(ctx)
		}
	}

	openFGAclient, err := newOpenFGAClient(ctx, p.OpenFGAParams)
	if err != nil {
		return nil, errors.E(op, err)
	}
	s.jimm.OpenFGAClient = openFGAclient
	if err := ensureControllerAdministrators(ctx, openFGAclient, p.ControllerUUID, p.ControllerAdmins); err != nil {
		return nil, errors.E(op, err, "failed to ensure controller admins")
	}

	authResourceBasePath := "/auth"
	redirectUrl, _ := url.Parse(p.PublicDNSName + authResourceBasePath + jimmhttp.CallbackEndpoint)
	redirectUrl.Scheme = "https"

	authSvc, err := auth.NewAuthenticationService(
		ctx,
		auth.AuthenticationServiceParams{
			IssuerURL:           p.OAuthAuthenticatorParams.IssuerURL,
			ClientID:            p.OAuthAuthenticatorParams.ClientID,
			ClientSecret:        p.OAuthAuthenticatorParams.ClientSecret,
			Scopes:              p.OAuthAuthenticatorParams.Scopes,
			SessionTokenExpiry:  p.OAuthAuthenticatorParams.SessionTokenExpiry,
			SessionCookieMaxAge: p.OAuthAuthenticatorParams.SessionCookieMaxAge,
			Store:               &s.jimm.Database,
			SessionStore:        sessionStore,
			RedirectURL:         redirectUrl.String(),
		},
	)
	s.jimm.OAuthAuthenticator = authSvc
	if err != nil {
		zapctx.Error(ctx, "failed to setup authentication service", zap.Error(err))
		return nil, errors.E(op, err, "failed to setup authentication service")
	}

	if err := s.setupCredentialStore(ctx, p); err != nil {
		return nil, errors.E(op, err)
	}

	if p.JWTExpiryDuration == 0 {
		p.JWTExpiryDuration = 24 * time.Hour
	}

	s.jimm.JWKService = jimmjwx.NewJWKSService(s.jimm.CredentialStore)
	s.jimm.JWTService = jimmjwx.NewJWTService(jimmjwx.JWTServiceParams{
		Host:   p.PublicDNSName,
		Store:  s.jimm.CredentialStore,
		Expiry: p.JWTExpiryDuration,
	})
	s.jimm.Dialer = &jujuclient.Dialer{
		JWTService: s.jimm.JWTService,
	}

	if !p.DisableConnectionCache {
		s.jimm.Dialer = jimm.CacheDialer(s.jimm.Dialer)
	}

	if _, err := url.Parse(p.DashboardFinalRedirectURL); err != nil {
		return nil, errors.E(op, err, "failed to parse final redirect url for the dashboard")
	}

	// Setup all HTTP handlers.
	mountHandler := func(path string, h jimmhttp.JIMMHttpHandler) {
		s.mux.Mount(path, h.Routes())
	}

	mountHandler(
		"/debug",
		debugapi.NewDebugHandler(
			map[string]debugapi.StatusCheck{
				"start_time": debugapi.ServerStartTime,
			},
		),
	)
	mountHandler(
		"/.well-known",
		wellknownapi.NewWellKnownHandler(s.jimm.CredentialStore),
	)
	oauthHandler, err := jimmhttp.NewOAuthHandler(jimmhttp.OAuthHandlerParams{
		Authenticator:             authSvc,
		DashboardFinalRedirectURL: p.DashboardFinalRedirectURL,
		SecureCookies:             p.SecureSessionCookies,
	})
	if err != nil {
		return nil, errors.E(op, err, "failed to setup authentication handler")
	}
	mountHandler(
		authResourceBasePath,
		oauthHandler,
	)
	macaroonDischarger, err := s.setupDischarger(p)
	if err != nil {
		return nil, errors.E(op, err, "failed to set up discharger")
	}
	s.mux.Handle(localDischargePath+"/*", discharger.GetDischargerMux(macaroonDischarger, localDischargePath))

	params := jujuapi.Params{
		ControllerUUID: p.ControllerUUID,
		PublicDNSName:  p.PublicDNSName,
	}

	s.mux.Handle("/api", jujuapi.APIHandler(ctx, &s.jimm, params))
	s.mux.Handle("/model/*", jujuapi.ModelHandler(ctx, &s.jimm, params))
	// If the request is not for a known path assume it is part of the dashboard.
	// If dashboard location env var is not defined, do not handle a dashboard.
	if p.DashboardLocation != "" {
		s.mux.Handle("/", dashboard.Handler(ctx, p.DashboardLocation, p.PublicDNSName))
	}

	return s, nil
}

// setupDischarger set JIMM up as a discharger of 3rd party caveats addressed to it. This is intended
// to enable Juju controllers to check for permissions using a macaroon-based workflow (atm only
// for cross model relations).
func (s *Service) setupDischarger(p Params) (*discharger.MacaroonDischarger, error) {
	cfg := discharger.MacaroonDischargerConfig{
		PublicKey:              p.PublicKey,
		PrivateKey:             p.PrivateKey,
		MacaroonExpiryDuration: p.MacaroonExpiryDuration,
		ControllerUUID:         p.ControllerUUID,
	}
	MacaroonDischarger, err := discharger.NewMacaroonDischarger(cfg, &s.jimm.Database, s.jimm.OpenFGAClient)
	if err != nil {
		return nil, errors.E(err)
	}
	return MacaroonDischarger, nil
}

func setupSessionStore(db *sql.DB, secretKey string) (*pgstore.PGStore, error) {
	store, err := pgstore.NewPGStoreFromPool(db, []byte(secretKey))
	return store, err
}

func openDB(ctx context.Context, dsn string) (*gorm.DB, error) {
	zapctx.Info(ctx, "connecting database")

	var dialect gorm.Dialector
	switch {
	case strings.HasPrefix(dsn, "pgx:"):
		dialect = postgres.Open(strings.TrimPrefix(dsn, "pgx:"))
	case strings.HasPrefix(dsn, "postgres:") || strings.HasPrefix(dsn, "postgresql:"):
		dialect = postgres.Open(dsn)
	default:
		return nil, errors.E(errors.CodeServerConfiguration, "unsupported DSN")
	}
	return gorm.Open(dialect, &gorm.Config{
		Logger: logger.GormLogger{},
		NowFunc: func() time.Time {
			// This is to set the timestamp precision at the service level.
			return time.Now().Truncate(time.Microsecond)
		},
	})
}

func (s *Service) setupCredentialStore(ctx context.Context, p Params) error {
	const op = errors.Op("newSecretStore")
	vs, err := newVaultStore(ctx, p)
	if err != nil {
		zapctx.Error(ctx, "Vault Store error", zap.Error(err))
		return errors.E(op, err)
	}
	if vs != nil {
		s.jimm.CredentialStore = vs
		return nil
	}

	// Only enable Postgres storage for secrets if explicitly enabled.
	if p.InsecureSecretStorage {
		zapctx.Warn(ctx, "using plaintext postgres for secret storage")
		s.jimm.CredentialStore = &s.jimm.Database
		return nil
	}
	// Currently jimm will start without a credential store but
	// functionality will be limited.
	return nil
}

func newVaultStore(ctx context.Context, p Params) (jimmcreds.CredentialStore, error) {
	if p.VaultSecretFile == "" {
		return nil, nil
	}
	zapctx.Info(ctx, "configuring vault client",
		zap.String("VaultAddress", p.VaultAddress),
		zap.String("VaultPath", p.VaultPath),
		zap.String("VaultSecretFile", p.VaultSecretFile),
		zap.String("VaultAuthPath", p.VaultAuthPath),
	)
	servermon.VaultConfigured.Inc()

	f, err := os.Open(p.VaultSecretFile)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	s, err := vaultapi.ParseSecret(f)
	if err != nil || s == nil {
		zapctx.Error(ctx, "failed to parse vault secret from file")
		return nil, err
	}

	cfg := vaultapi.DefaultConfig()
	if p.VaultAddress != "" {
		cfg.Address = p.VaultAddress
	}

	client, err := vaultapi.NewClient(cfg)
	if err != nil {
		return nil, err
	}

	return &vault.VaultStore{
		Client:     client,
		AuthSecret: s.Data,
		AuthPath:   p.VaultAuthPath,
		KVPath:     p.VaultPath,
	}, nil
}

// CheckOrGenerateOAuthKey checks if the OAuth secret key already exists on the
// credential store, and if not, generates a random 4096-bit secret key and
func (s *Service) CheckOrGenerateOAuthKey(ctx context.Context) error {
	const op = errors.Op("CheckOrGenerateOAuthKey")
	store := s.jimm.CredentialStore
	if store == nil {
		zapctx.Info(ctx, "skipped generating initial OAuth secret key due to nil credential store")
		return nil
	}

	if secret, err := store.GetOAuthSecret(ctx); err == nil && secret != nil && len(secret) > 0 {
		zapctx.Info(ctx, "detected existing OAuth secret key")
		return nil
	}

	secret := make([]byte, 4096)
	if _, err := rand.Read(secret); err != nil {
		zapctx.Error(ctx, "failed to generate OAuth secret key", zap.Error(err))
		return errors.E(op, err, "failed to generate OAuth secret key")
	}

	if err := store.PutOAuthSecret(ctx, secret); err != nil {
		zapctx.Error(ctx, "failed to store generated OAuth secret key", zap.Error(err))
		return errors.E(op, err, "failed to store generated OAuth secret key")
	}
	return nil
}

func newOpenFGAClient(ctx context.Context, p OpenFGAParams) (*openfga.OFGAClient, error) {
	const op = errors.Op("newOpenFGAClient")
	cofgaClient, err := cofga.NewClient(ctx, cofga.OpenFGAParams{
		Scheme:      p.Scheme,
		Host:        p.Host,
		Token:       p.Token,
		Port:        p.Port,
		StoreID:     p.Store,
		AuthModelID: p.AuthModel,
	})
	if err != nil {
		return nil, errors.E(op, err)
	}
	return openfga.NewOpenFGAClient(cofgaClient), nil
}

// ensureControllerAdministrators ensures that listed users have admin access to the JIMM controller.
// This method checks if these users already have administrator access to the JIMM controller,
// otherwise it will add a direct administrator relation between each user and the JIMM
// controller.
func ensureControllerAdministrators(ctx context.Context, client *openfga.OFGAClient, controllerUUID string, admins []string) error {
	controller := names.NewControllerTag(controllerUUID)
	tuples := []openfga.Tuple{}
	for _, username := range admins {
		userTag := names.NewUserTag(username)
		user := openfga.NewUser(&dbmodel.Identity{Name: userTag.Id()}, client)
		isAdmin, err := openfga.IsAdministrator(ctx, user, controller)
		if err != nil {
			return errors.E(err)
		}
		if !isAdmin {
			tuples = append(tuples, openfga.Tuple{
				Object:   ofganames.ConvertTag(userTag),
				Relation: ofganames.AdministratorRelation,
				Target:   ofganames.ConvertTag(controller),
			})
		}
	}
	if len(tuples) == 0 {
		return nil
	}
	return client.AddRelation(ctx, tuples...)
}
