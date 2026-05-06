// Copyright 2025 Canonical.

// service defines the methods necessary to start a JIMM server
// alongside all the config options that can be supplied to configure JIMM.
package service

import (
	"context"
	"fmt"
	"maps"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/antonlindstrom/pgstore"
	service "github.com/canonical/go-service"
	cofga "github.com/canonical/ofga"
	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	vaultapi "github.com/hashicorp/vault/api"
	"github.com/juju/names/v5"
	"github.com/juju/zaputil/zapctx"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/cors"
	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/canonical/jimm/v3/internal/auth"
	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/discharger"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/jimm"
	"github.com/canonical/jimm/v3/internal/jimm/config"
	jimmcreds "github.com/canonical/jimm/v3/internal/jimm/credentials"
	"github.com/canonical/jimm/v3/internal/jimm/juju"
	"github.com/canonical/jimm/v3/internal/jimm/login"
	"github.com/canonical/jimm/v3/internal/jimmhttp"
	"github.com/canonical/jimm/v3/internal/jimmhttp/rebac_admin"
	"github.com/canonical/jimm/v3/internal/jimmjwx"
	"github.com/canonical/jimm/v3/internal/jujuapi"
	"github.com/canonical/jimm/v3/internal/jujuclient"
	"github.com/canonical/jimm/v3/internal/logger"
	"github.com/canonical/jimm/v3/internal/middleware"
	"github.com/canonical/jimm/v3/internal/openfga"
	ofganames "github.com/canonical/jimm/v3/internal/openfga/names"
	"github.com/canonical/jimm/v3/internal/pubsub"
	"github.com/canonical/jimm/v3/internal/river"
	"github.com/canonical/jimm/v3/internal/vault"
)

const (
	localDischargePath                        = "/macaroons"
	INTERVAL_BETWEEN_MODEL_MIGRATIONS_CLEANUP = 10 * time.Minute
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

	// SecureSessionCookies determines if HTTPS must be enabled in order for JIMM
	// to set cookies when creating browser based sessions.
	SecureSessionCookies bool

	// JWTSessionKey holds the secret key used for signing/verifying JWT tokens.
	// See internal/auth/oauth2.go AuthenticationService.SessionSecretkey for more details.
	JWTSessionKey string

	// AuthStyle configures how the client credentials should be sent to the token endpoint.
	AuthStyle string
}

// A Params structure contains the parameters required to initialise a new
// Service.
type Params struct {
	// ControllerUUID contains the UUID of the JIMM controller, if this
	// is not set a random UUID will be generated.
	ControllerUUID string

	// IsLeader indicates that this is the JIMM leader unit.
	IsLeader bool

	// DSN is the data source name that the JIMM service will use to
	// connect to its database. If this is empty an in-memory database
	// will be used.
	DSN string

	// ControllerAdmins contains a list of users (or groups)
	// that will be given the access-level "superuser" when they
	// authenticate to the controller.
	ControllerAdmins []string

	// VaultRoleID is the AppRole role ID.
	VaultRoleID string

	// VaultRoleSecretID is the AppRole secret ID.
	VaultRoleSecretID string

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

	// JWKSPath is the path to the JSON Web Key Set document served by JIMM.
	JWKSPath string

	// JWKSPrivateKeyPath is the path to the PEM encoded RSA private key used to sign controller JWTs.
	JWKSPrivateKeyPath string

	// InsecureSecretStorage instructs JIMM to store secrets in its database
	// instead of dedicated secure storage. SHOULD NOT BE USED IN PRODUCTION.
	InsecureSecretStorage bool

	// OAuthAuthenticatorParams holds parameters needed to configure an OAuthAuthenticator
	// implementation.
	OAuthAuthenticatorParams OAuthAuthenticatorParams

	// DashboardFinalRedirectURL is the URL to FINALLY redirect to after completing
	// the /callback in an authorisation code OAuth2.0 flow to finish the flow.
	DashboardFinalRedirectURL string

	// CookieSessionKey is a randomly generated secret passed via config used for signing
	// cookie data. The recommended length is 32/64 characters from the Gorilla securecookie lib.
	// https://github.com/gorilla/securecookie/blob/main/securecookie.go#L124
	CookieSessionKey []byte

	// CorsAllowedOrigins represents all addresses that are valid for cross-origin
	// requests. A wildcard '*' is accepted to allow all cross-origin requests.
	CorsAllowedOrigins []string

	// LogSQL determines whether ORM queries are printed when debug logs are enabled.
	// This may leak secrets in logs when sensitive values are stored in the DB like OAuth tokens.
	LogSQL bool

	// LogLevel is the default logger is set.
	// Setting this to "debug" enables the requests logger as well.
	LogLevel string

	// HostKeyFingerprints is the fingerprint of the SSH public host key.
	HostKeyFingerprints map[string]string

	// ControllerConfig is the configuration to expose when the ControllerConfig
	// facade is called.
	ControllerConfig config.ControllerConfig

	// CrossModelQueryTimeout is the timeout for cross model queries.
	CrossModelQueryTimeout time.Duration

	// BootstrapLoginTokenRefreshURL is the URL when bootstrapping a controller via JIMM.
	// It should look something like:
	// <scheme><ip/dns>[<port>]/.well-known/jwks.json"
	BootstrapLoginTokenRefreshURL string
}

// A Service is the implementation of a JIMM server.
type Service struct {
	jimm       *jimm.JIMM
	jwkService *jimmjwx.JWKSService

	isLeader bool

	mux      *chi.Mux
	cleanups []func() error
}

// ServiceDependencies contains initialized services and parameters used to construct a fully functional Service.
// It allows callers to override specific dependencies (e.g. auth) before creating the service.
type ServiceDependencies struct {
	// Params
	AuditLogRetentionDays         int
	BootstrapLoginTokenRefreshURL string
	ControllerConfig              config.ControllerConfig
	ControllerUUID                string
	CorsAllowedOrigins            []string
	CrossModelQueryTimeout        time.Duration
	DischargerPrivateKey          string
	DischargerPublicKey           string
	HostKeyFingerprints           map[string]string
	IsLeader                      bool
	MacaroonExpiryDuration        time.Duration
	PublicDNSName                 string
	PublicDNSHost                 string
	// Clients/Services
	Client                  juju.Dialer
	CredentialStore         jimmcreds.CredentialStore
	Database                *db.Database
	RiverClient             *river.Client
	MigrationTokenGenerator juju.MigrationTokenGenerator
	JWKSService             *jimmjwx.JWKSService
	JWTService              *jimmjwx.JWTService
	OAuthAuthenticator      login.OAuthAuthenticator
	OAuthHandler            *jimmhttp.OAuthHandler
	OpenFGAClient           *openfga.OFGAClient
	// Cleanup
	cleanupFuncs []func() error
}

// Validate checks that all required dependencies are present and returns an error if any are missing.
func (s *ServiceDependencies) Validate() error {
	if s.Database == nil {
		return errors.New("missing database")
	}
	if s.RiverClient == nil {
		return errors.New("missing river client")
	}
	if s.OpenFGAClient == nil {
		return errors.New("missing openfga client")
	}
	if s.CredentialStore == nil {
		return errors.New("missing credential store")
	}
	if s.JWTService == nil {
		return errors.New("missing jwt service")
	}
	if s.JWKSService == nil {
		return errors.New("missing jwks service")
	}
	if s.OAuthAuthenticator == nil {
		return errors.New("missing oauth authenticator")
	}
	if s.MigrationTokenGenerator == nil {
		return errors.New("missing migration token generator")
	}
	return nil
}

func (s *Service) JIMM() *jimm.JIMM {
	return s.jimm
}

// ServeHTTP implements http.Handler.
func (s *Service) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	s.mux.ServeHTTP(w, req)
}

// WatchModelSummaries connects to all controllers and starts a
// ModelSummaryWatcher for all models. WatchModelSummaries finishes when
// the given context is canceled, or there is a fatal error watching model
// summaries.
func (s *Service) WatchModelSummaries(ctx context.Context) error {
	w := juju.Watcher{
		Database: s.jimm.Database,
		Dialer:   s.jimm.Dialer,
		Pubsub:   s.jimm.Pubsub,
	}
	return w.WatchAllModelSummaries(ctx, 10*time.Minute)
}

// MonitorResources periodically updates metrics.
func (s *Service) MonitorResources(ctx context.Context) {
	s.jimm.JujuManager.UpdateMetrics(ctx)
	ticker := time.NewTicker(5 * time.Minute)
	for {
		select {
		case <-ticker.C:
			s.jimm.JujuManager.UpdateMetrics(ctx)
		case <-ctx.Done():
			zapctx.Info(ctx, "exiting resource monitor polling")
			return
		}
	}
}

// OpenFGACleanup starts a goroutine that cleans up any orphaned tuples from OpenFGA.
func (s *Service) OpenFGACleanup(ctx context.Context, trigger <-chan time.Time) error {
	for {
		select {
		case <-trigger:
			err := s.jimm.PermissionManager.OpenFGACleanup(ctx)
			if err != nil {
				zapctx.Error(ctx, "openfga cleanup", zap.Error(err))
				continue
			}
		case <-ctx.Done():
			zapctx.Info(ctx, "exiting OpenFGA cleanup polling")
			return nil
		}
	}
}

// CleanupNotFoundModels triggers every `trigger` time and calls the jimm methods to cleanup dying models.
func (s *Service) CleanupNotFoundModels(ctx context.Context, trigger <-chan time.Time) error {
	for {
		select {
		case <-trigger:
			err := s.jimm.JujuManager.PollModels(ctx)
			if err != nil {
				zapctx.Error(ctx, "dying models cleanup", zap.Error(err))
				continue
			}
		case <-ctx.Done():
			zapctx.Info(ctx, "exiting dying model cleanup polling")
			return nil
		}
	}
}

// CleanupPartialModelMigrations triggers every `trigger` time and calls the jimm methods to cleanup partial model migrations.
func (s *Service) CleanupPartialModelMigrations(ctx context.Context, trigger <-chan time.Time) error {
	for {
		select {
		case <-trigger:
			err := s.jimm.JujuManager.CleanupPartialModelMigrations(ctx)
			if err != nil {
				zapctx.Error(ctx, "partial model migrations cleanup", zap.Error(err))
				continue
			}
		case <-ctx.Done():
			zapctx.Info(ctx, "exiting partial model migration cleanup polling")
			return nil
		}
	}
}

// Cleanup cleans up resources that need to be released on shutdown.
func (s *Service) Cleanup() {
	// Iterating over clean up function in reverse-order to avoid early clean ups.
	for i := len(s.cleanups) - 1; i >= 0; i-- {
		f := s.cleanups[i]
		if err := f(); err != nil {
			zapctx.Error(context.Background(), "cleanup failed", zap.Error(err))
		}
	}
}

// AddCleanup adds a clean up function to be run at service shutdown.
func (s *Service) AddCleanup(f func() error) {
	s.cleanups = append(s.cleanups, f)
}

// NewServiceDependencies creates the default dependency set used by NewService.
func NewServiceDependencies(ctx context.Context, p Params) (*ServiceDependencies, error) {
	controllerUUID := p.ControllerUUID
	if controllerUUID == "" {
		controllerUUID = uuid.NewString()
	}

	auditLogRetentionDays := 0
	if p.AuditLogRetentionPeriodInDays != "" {
		retentionPeriod, err := strconv.Atoi(p.AuditLogRetentionPeriodInDays)
		if err != nil {
			return nil, errors.New("failed to parse audit log retention period")
		}
		if retentionPeriod < 0 {
			return nil, errors.New("retention period cannot be less than 0")
		}
		auditLogRetentionDays = retentionPeriod
	}

	if _, err := url.Parse(p.DashboardFinalRedirectURL); err != nil {
		return nil, fmt.Errorf("failed to parse final redirect url for the dashboard: %w", err)
	}

	publicDNS, err := parseURLWithOptionalScheme(p.PublicDNSName)
	if err != nil {
		return nil, fmt.Errorf("failed to parse public DNS name: %v", err)
	}

	jwksService, err := jimmjwx.NewJWKSService(ctx, jimmjwx.JWKSServiceParams{
		JWKSPath:       p.JWKSPath,
		PrivateKeyPath: p.JWKSPrivateKeyPath,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to configure jwks: %w", err)
	}

	if p.DSN == "" {
		return nil, errors.New("missing DSN")
	}

	database, err := openDB(ctx, p.DSN, p.LogSQL)
	if err != nil {
		return nil, err
	}
	db := &db.Database{DB: database}

	riverClient, err := river.NewRiverClient(db)
	if err != nil {
		return nil, fmt.Errorf("failed to create river client: %w", err)
	}

	openFGAclient, err := newOpenFGAClient(ctx, p.OpenFGAParams)
	if err != nil {
		return nil, err
	}

	if err := ensureControllerAdministrators(ctx, openFGAclient, controllerUUID, p.ControllerAdmins); err != nil {
		return nil, fmt.Errorf("failed to ensure controller admins: %w", err)
	}

	credentialStore, err := setupCredentialStore(ctx, p, db)
	if err != nil {
		return nil, err
	}

	jwtExpiry := p.JWTExpiryDuration
	if jwtExpiry == 0 {
		jwtExpiry = 24 * time.Hour
	}

	jwtService := jimmjwx.NewJWTService(jimmjwx.JWTServiceParams{
		Host:   p.PublicDNSName,
		Expiry: jwtExpiry,
		JWKS:   jwksService,
	})

	dialer := jujuclient.NewDialer(jwtService, controllerUUID)

	deps := &ServiceDependencies{
		ControllerUUID:                controllerUUID,
		PublicDNSName:                 p.PublicDNSName,
		PublicDNSHost:                 publicDNS.Host,
		CorsAllowedOrigins:            append([]string(nil), p.CorsAllowedOrigins...),
		HostKeyFingerprints:           maps.Clone(p.HostKeyFingerprints),
		ControllerConfig:              p.ControllerConfig,
		CrossModelQueryTimeout:        p.CrossModelQueryTimeout,
		BootstrapLoginTokenRefreshURL: p.BootstrapLoginTokenRefreshURL,
		AuditLogRetentionDays:         auditLogRetentionDays,
		IsLeader:                      p.IsLeader,
		MacaroonExpiryDuration:        p.MacaroonExpiryDuration,
		DischargerPrivateKey:          p.PrivateKey,
		DischargerPublicKey:           p.PublicKey,
		Database:                      db,
		Client:                        jimm.NewDialerAdapter(dialer),
		RiverClient:                   riverClient,
		OpenFGAClient:                 openFGAclient,
		CredentialStore:               credentialStore,
		JWTService:                    jwtService,
		JWKSService:                   jwksService,
	}

	sessionStore, cleanupFuncs, err := setupSessionStore(p.CookieSessionKey, db)
	if err != nil {
		return nil, err
	}
	deps.cleanupFuncs = append(deps.cleanupFuncs, cleanupFuncs...)

	redirectUrl := p.PublicDNSName + jimmhttp.AuthResourceBasePath + jimmhttp.CallbackEndpoint
	if !strings.HasPrefix(redirectUrl, "https://") && !strings.HasPrefix(redirectUrl, "http://") {
		redirectUrl = "https://" + redirectUrl
	}

	authSvc, err := auth.NewAuthenticationService(
		ctx,
		auth.AuthenticationServiceParams{
			IssuerURL:           p.OAuthAuthenticatorParams.IssuerURL,
			ClientID:            p.OAuthAuthenticatorParams.ClientID,
			ClientSecret:        p.OAuthAuthenticatorParams.ClientSecret,
			Scopes:              p.OAuthAuthenticatorParams.Scopes,
			SessionTokenExpiry:  p.OAuthAuthenticatorParams.SessionTokenExpiry,
			SessionCookieMaxAge: p.OAuthAuthenticatorParams.SessionCookieMaxAge,
			JWTSessionKey:       p.OAuthAuthenticatorParams.JWTSessionKey,
			SecureCookies:       p.OAuthAuthenticatorParams.SecureSessionCookies,
			AuthStyle:           auth.AuthStyle(p.OAuthAuthenticatorParams.AuthStyle),
			Store:               db,
			SessionStore:        sessionStore,
			RedirectURL:         redirectUrl,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to setup authentication service: %w", err)
	}
	deps.OAuthAuthenticator = authSvc
	deps.MigrationTokenGenerator = authSvc
	deps.OAuthHandler = nil
	if p.DashboardFinalRedirectURL != "" {
		var err error
		deps.OAuthHandler, err = jimmhttp.NewOAuthHandler(jimmhttp.OAuthHandlerParams{
			Authenticator:             authSvc,
			DashboardFinalRedirectURL: p.DashboardFinalRedirectURL,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to setup authentication handler: %w", err)
		}
	} else {
		zapctx.Warn(ctx, "Dashboard final redirect URL not set, OAuth HTTP handler will not be configured")
	}

	return deps, nil
}

// NewServiceFromDependencies creates a Service from a provided dependency set.
// Callers can use this to inject mock dependencies while keeping handler setup identical.
func NewServiceFromDependencies(ctx context.Context, deps *ServiceDependencies) (*Service, error) {
	if deps == nil {
		return nil, errors.New("missing service dependencies")
	}
	if err := deps.Validate(); err != nil {
		return nil, fmt.Errorf("invalid service dependencies: %w", err)
	}

	s := &Service{
		jwkService: deps.JWKSService,
		cleanups:   append([]func() error(nil), deps.cleanupFuncs...),
	}

	jimmParameters := jimm.Parameters{
		UUID:                          deps.ControllerUUID,
		Database:                      deps.Database,
		Dialer:                        deps.Client,
		CredentialStore:               deps.CredentialStore,
		Pubsub:                        &pubsub.Hub{MaxConcurrency: 50},
		OpenFGAClient:                 deps.OpenFGAClient,
		RiverClient:                   deps.RiverClient,
		OAuthAuthenticator:            deps.OAuthAuthenticator,
		MigrationTokenGenerator:       deps.MigrationTokenGenerator,
		JWTService:                    deps.JWTService,
		ControllerConfig:              deps.ControllerConfig,
		CrossModelQueryTimeout:        deps.CrossModelQueryTimeout,
		BootstrapLoginTokenRefreshURL: deps.BootstrapLoginTokenRefreshURL,
	}
	jimmParameters.AuditLogRetentionDays = deps.AuditLogRetentionDays

	var err error
	s.jimm, err = jimm.New(jimmParameters)
	if err != nil {
		return nil, err
	}

	s.mux = chi.NewRouter()
	s.mux.Use(chimiddleware.RequestLogger(&logger.HTTPLogFormatter{}))
	s.mux.Use(middleware.MeasureHTTPResponseTime)

	// Setup CORS middleware
	corsOpts := cors.New(cors.Options{
		AllowedOrigins:   deps.CorsAllowedOrigins,
		AllowedMethods:   []string{"GET"},
		AllowCredentials: true,
	})
	s.mux.Use(corsOpts.Handler)

	// Setup all HTTP handlers.
	mountHandler := func(path string, h jimmhttp.JIMMHttpHandler) {
		s.mux.Mount(path, h.Routes())
	}

	rebacBackend, err := rebac_admin.SetupBackend(ctx, jujuapi.NewJIMMAdapter(s.jimm))
	if err != nil {
		return nil, err
	}

	s.mux.Mount("/rebac", middleware.AuthenticateRebac("/rebac", rebacBackend.Handler(""), s.jimm.LoginManager))

	mountHandler("/.well-known", jimmhttp.NewWellKnownHandler(s.jwkService))

	if deps.OAuthHandler == nil {
		zapctx.Warn(ctx, "OAuth HTTP handler not enabled, browser flow login disabled")
	} else {
		mountHandler(jimmhttp.AuthResourceBasePath, deps.OAuthHandler)
	}

	macaroonDischarger, err := discharger.NewMacaroonDischarger(discharger.MacaroonDischargerConfig{
		PublicKey:              deps.DischargerPublicKey,
		PrivateKey:             deps.DischargerPrivateKey,
		MacaroonExpiryDuration: deps.MacaroonExpiryDuration,
		ControllerUUID:         deps.ControllerUUID,
	}, deps.Database, s.jimm.OfferAuthorizer)
	if err != nil {
		return nil, fmt.Errorf("failed to set up discharger: %v", err)
	}
	s.mux.Handle(localDischargePath+"/*", discharger.GetDischargerMux(macaroonDischarger, localDischargePath))

	params := jujuapi.Params{
		ControllerUUID: deps.ControllerUUID,
		PublicDNSName:  deps.PublicDNSHost,
	}

	// Websockets require extra care when cookies are used for authentication
	// to avoid CSRF attacks. https://portswigger.net/web-security/websockets/cross-site-websocket-hijacking
	websocketCors := middleware.NewWebsocketCors(deps.CorsAllowedOrigins)
	// Juju API handlers
	s.mux.Handle("/api", websocketCors.Handler(jujuapi.APIHandler(ctx, s.jimm, params)))
	s.mux.Handle("/model/*", websocketCors.Handler(http.StripPrefix("/model", jujuapi.ModelHandler(ctx, s.jimm, params))))
	// Uploading local charms (s3 compatible endpoint and legacy HTTP endpoint, respectively)
	proxyHandler := jimmhttp.NewHTTPProxyHandler(s.jimm.LoginManager, s.jimm.JujuManager, s.jimm.JujuAuthFactory)
	mountHandler("/model-{uuid}/charms/{charmref}", proxyHandler)
	mountHandler("/model/{uuid}/{type:charms|applications}", proxyHandler)
	// HTTP Migration endpoints
	mountHandler("/migrate", jimmhttp.NewMigrationHTTPProxyHandler(s.jimm.LoginManager, s.jimm.JujuManager, s.jimm.JujuAuthFactory))
	// Log transfer endpoint
	s.mux.Handle("/migrate/logtransfer", jujuapi.LogTransferHandler(ctx, s.jimm, params))

	// serve the ssh public key fingerprint
	s.mux.Get("/ssh/public-key-fingerprints", jimmhttp.WriteFingerprints(deps.HostKeyFingerprints))

	s.isLeader = deps.IsLeader

	return s, nil
}

// NewInternalService builds the internal-only HTTP server.
func NewInternalService(addr string, corsAllowedOrigins []string) *http.Server {
	mux := chi.NewRouter()
	corsOpts := cors.New(cors.Options{
		AllowedOrigins:   corsAllowedOrigins,
		AllowedMethods:   []string{"GET"},
		AllowCredentials: true,
	})
	mux.Use(corsOpts.Handler)
	mux.Mount("/metrics", promhttp.Handler())
	mux.Mount("/debug", jimmhttp.NewDebugHandler(
		map[string]jimmhttp.StatusCheck{
			"start_time": jimmhttp.ServerStartTime,
		},
	).Routes())
	return &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: time.Second * 5,
	}
}

// NewService creates a new Service using the given params.
func NewService(ctx context.Context, p Params) (*Service, error) {
	deps, err := NewServiceDependencies(ctx, p)
	if err != nil {
		return nil, err
	}
	return NewServiceFromDependencies(ctx, deps)
}

// parseURLWithOptionalScheme parses an input string
// that may exclude a scheme, i.e. missing "http://".
// If no scheme is provided, "https" will be used.
func parseURLWithOptionalScheme(addr string) (*url.URL, error) {
	uriScheme := "https"
	// Add the schema if parsing fails or the host is empty.
	// This avoids parsing ambiguity in url.Parse.
	url, err := url.Parse(addr)
	if err != nil || url.Host == "" {
		url, err = url.Parse(uriScheme + "://" + addr)
		if err != nil {
			return nil, err
		}
	}
	return url, nil
}

func (s *Service) StartServices(ctx context.Context, svc *service.Service) {
	// on the leader unit we start additional routines
	if s.isLeader {
		// audit log cleanup routine
		svc.Go(func() error {
			s.jimm.AuditLogManager.StartCleanup(ctx)
			return nil
		})

		// OpenFGA cleanup - cleans up all orphaned tuples
		svc.Go(func() error {
			return s.OpenFGACleanup(ctx, time.NewTicker(6*time.Hour).C)
		})

		// CleanupNotFoundModels cleanup - cleans up all models not found on the respective controller.
		svc.Go(func() error {
			return s.CleanupNotFoundModels(ctx, time.NewTicker(time.Minute).C)
		})

		// CleanupPartialModelMigration cleanup - cleans up all partial model migrations.
		svc.Go(func() error {
			return s.CleanupPartialModelMigrations(ctx, time.NewTicker(INTERVAL_BETWEEN_MODEL_MIGRATIONS_CLEANUP).C)
		})
	}

	// all units periodically update their controller/model metrics
	svc.Go(func() error {
		s.MonitorResources(ctx)
		return nil
	})

	// all units watch for model summaries
	svc.Go(func() error { return s.WatchModelSummaries(ctx) })
}

func setupSessionStore(sessionSecret []byte, db *db.Database) (*pgstore.PGStore, []func() error, error) {
	sqlDb, err := db.DB.DB()
	if err != nil {
		return nil, nil, err
	}

	store, err := pgstore.NewPGStoreFromPool(sqlDb, sessionSecret)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create session store: %w", err)
	}

	// Cleanup expired session every 30 minutes
	cleanupQuit, cleanupDone := store.Cleanup(time.Minute * 30)
	cleanups := []func() error{
		func() error {
			store.StopCleanup(cleanupQuit, cleanupDone)
			return nil
		},
		func() error {
			store.Close()
			return nil
		},
	}
	return store, cleanups, nil
}

func openDB(ctx context.Context, dsn string, logSQL bool) (*gorm.DB, error) {
	zapctx.Info(ctx, "connecting database")

	var dialect gorm.Dialector
	switch {
	case strings.HasPrefix(dsn, "pgx:"):
		dialect = postgres.Open(strings.TrimPrefix(dsn, "pgx:"))
	case strings.HasPrefix(dsn, "postgres:") || strings.HasPrefix(dsn, "postgresql:"):
		dialect = postgres.Open(dsn)
	default:
		return nil, errors.Codef(errors.CodeServerConfiguration, "unsupported DSN")
	}
	return gorm.Open(dialect, &gorm.Config{
		Logger: &logger.GormLogger{LogSQL: logSQL},
		NowFunc: func() time.Time {
			// This is to set the timestamp precision at the service level.
			return time.Now().Truncate(time.Microsecond)
		},
	})
}

func setupCredentialStore(ctx context.Context, p Params, db *db.Database) (jimmcreds.CredentialStore, error) {

	// Only enable Postgres storage for secrets if explicitly enabled.
	if p.InsecureSecretStorage {
		zapctx.Warn(ctx, "using plaintext postgres for secret storage")
		return db, nil
	}

	vs, err := newVaultStore(ctx, p)
	if err != nil {
		return nil, fmt.Errorf("vault store error: %v", err)
	}
	if vs != nil {
		return vs, nil
	}

	return nil, errors.New("jimm cannot start without a credential store")
}

func newVaultStore(ctx context.Context, p Params) (jimmcreds.CredentialStore, error) {
	if p.VaultRoleID == "" || p.VaultRoleSecretID == "" {
		return nil, nil
	}
	zapctx.Info(ctx, "configuring vault client",
		zap.String("VaultAddress", p.VaultAddress),
		zap.String("VaultPath", p.VaultPath),
		zap.String("VaultRoleID", p.VaultRoleID),
	)

	cfg := vaultapi.DefaultConfig()
	if p.VaultAddress != "" {
		cfg.Address = p.VaultAddress
	}

	client, err := vaultapi.NewClient(cfg)
	if err != nil {
		return nil, err
	}

	return &vault.VaultStore{
		Client:       client,
		RoleID:       p.VaultRoleID,
		RoleSecretID: p.VaultRoleSecretID,
		KVPath:       strings.ReplaceAll(p.VaultPath, "/", ""),
	}, nil
}

func newOpenFGAClient(ctx context.Context, p OpenFGAParams) (*openfga.OFGAClient, error) {

	cofgaClient, err := cofga.NewClient(ctx, cofga.OpenFGAParams{
		Scheme:      p.Scheme,
		Host:        p.Host,
		Token:       p.Token,
		Port:        p.Port,
		StoreID:     p.Store,
		AuthModelID: p.AuthModel,
	})
	if err != nil {
		return nil, err
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
		i, err := dbmodel.NewIdentity(userTag.Id())
		if err != nil {
			return err
		}
		user := openfga.NewUser(i, client)
		isAdmin, err := openfga.IsAdministrator(ctx, user, controller)
		if err != nil {
			return err
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
	err := client.AddRelation(ctx, tuples...)
	if err != nil {
		return err
	}
	logger.LogGrantJimmAdmins(ctx, admins)
	return nil
}
