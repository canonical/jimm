// Copyright 2021 Canonical Ltd.

package jimm

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/canonical/candid/candidclient"
	cofga "github.com/canonical/ofga"
	"github.com/go-chi/chi/v5"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery/dbrootkeystore"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery/identchecker"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/httpbakery"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/httpbakery/agent"
	"github.com/google/uuid"
	vaultapi "github.com/hashicorp/vault/api"
	"github.com/juju/names/v4"
	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/canonical/jimm/internal/auth"
	"github.com/canonical/jimm/internal/dashboard"
	"github.com/canonical/jimm/internal/db"
	"github.com/canonical/jimm/internal/dbmodel"
	"github.com/canonical/jimm/internal/debugapi"
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

	// CandidURL contains the URL of the candid server that the JIMM
	// service will use for authentication. If this is empty then no
	// authentication will be possible.
	CandidURL string

	// CandidPublicKey contains the base64 encoded public key of the
	// candid server specified in CandidURL. In most cases there is no
	// need to set this parameter, The public key will be retrieved
	// from the candid server itself.
	CandidPublicKey string

	// BakeryAgentFile contains the path of a file containing agent
	// authentication information for JIMM. If this is empty then
	// authentication will only use information contained in the
	// discharged macaroons.
	BakeryAgentFile string

	// ControllerAdmins contains a list of candid users (or groups)
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

	// JWTExpiryDuration holds the expiry duration for issued JWTs.
	JWTExpiryDuration time.Duration

	// InsecureSecretStorage instructs JIMM to store secrets in its database
	// instead of dedicated secure storage. SHOULD NOT BE USED IN PRODUCTION.
	InsecureSecretStorage bool

	// InsecureJwksLookup instructs JIMM to lookup its JWKS value via
	// http instead of https. Useful when running JIMM in a docker compose.
	InsecureJwksLookup bool
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

// RegisterJwksCache registers the JWKS Cache with JIMM's JWT service.
func (s *Service) RegisterJwksCache(ctx context.Context) {
	if s.jimm.JWTService == nil {
		zapctx.Warn(ctx, "skipping JWKS cache registration - service not available")
		return
	}
	tlsConfig := &tls.Config{
		InsecureSkipVerify: true,
	}
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: tlsConfig,
		},
		Timeout: 15 * time.Second,
	}
	s.jimm.JWTService.RegisterJWKSCache(ctx, client)
}

// NewService creates a new Service using the given params.
func NewService(ctx context.Context, p Params) (*Service, error) {
	const op = errors.Op("NewService")

	s := new(Service)
	s.mux = chi.NewRouter()

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

	kp, dischargeMux, err := s.setupDischarger(p, openFGAclient)
	if err != nil {
		return nil, errors.E(op, err, "failed to set up discharger")
	}
	s.mux.Handle(localDischargePath+"/*", dischargeMux)

	s.jimm.Authenticator, err = newAuthenticator(ctx, &s.jimm.Database, openFGAclient, kp, p)
	if err != nil {
		return nil, errors.E(op, err)
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
		Secure: !p.InsecureJwksLookup,
		Store:  s.jimm.CredentialStore,
		Expiry: p.JWTExpiryDuration,
	})
	s.jimm.Dialer = &jujuclient.Dialer{
		JWTService: s.jimm.JWTService,
	}

	if !p.DisableConnectionCache {
		s.jimm.Dialer = jimm.CacheDialer(s.jimm.Dialer)
	}

	s.jimm.River, err = jimm.NewRiver(ctx, nil, p.DSN, s.jimm.OpenFGAClient, s.jimm.Database)
	if err != nil {
		return nil, errors.E(op, err)
	}

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

	params := jujuapi.Params{
		ControllerUUID:   p.ControllerUUID,
		IdentityLocation: p.CandidURL,
		PublicDNSName:    p.PublicDNSName,
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
func (s *Service) setupDischarger(p Params, openFGAclient *openfga.OFGAClient) (*bakery.KeyPair, *http.ServeMux, error) {
	macaroonDischarger, err := newMacaroonDischarger(p, &s.jimm.Database, openFGAclient)
	if err != nil {
		return nil, nil, errors.E(err)
	}

	discharger := httpbakery.NewDischarger(
		httpbakery.DischargerParams{
			Key:     &macaroonDischarger.kp,
			Checker: httpbakery.ThirdPartyCaveatCheckerFunc(macaroonDischarger.checkThirdPartyCaveat),
		},
	)
	dischargeMux := http.NewServeMux()
	discharger.AddMuxHandlers(dischargeMux, localDischargePath)

	return &macaroonDischarger.kp, dischargeMux, nil
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

func newAuthenticator(ctx context.Context, db *db.Database, client *openfga.OFGAClient, key *bakery.KeyPair, p Params) (jimm.Authenticator, error) {
	if p.CandidURL == "" {
		// No authenticator configured
		return nil, nil
	}
	zapctx.Info(ctx, "configuring authenticator",
		zap.String("CandidURL", p.CandidURL),
		zap.String("CandidPublicKey", p.CandidPublicKey),
		zap.String("BakeryAgentFile", p.BakeryAgentFile),
	)
	tps := bakery.NewThirdPartyStore()
	if p.CandidPublicKey != "" {
		var pk bakery.PublicKey
		if err := pk.Key.UnmarshalText([]byte(p.CandidPublicKey)); err != nil {
			return nil, err
		}
		tps.AddInfo(p.CandidURL, bakery.ThirdPartyInfo{
			PublicKey: pk,
			Version:   bakery.Version2,
		})
	}

	bClient := httpbakery.NewClient()
	var agentUsername string
	if p.BakeryAgentFile != "" {
		data, err := os.ReadFile(p.BakeryAgentFile)
		if err != nil {
			return nil, err
		}
		var info agent.AuthInfo
		if err := json.Unmarshal(data, &info); err != nil {
			return nil, err
		}
		if err := agent.SetUpAuth(bClient, &info); err != nil {
			return nil, err
		}
		for _, a := range info.Agents {
			if a.URL == p.CandidURL {
				agentUsername = a.Username
			}
		}
	}
	candidClient, err := candidclient.New(candidclient.NewParams{
		BaseURL:       p.CandidURL,
		Client:        bClient,
		AgentUsername: agentUsername,
		CacheTime:     10 * time.Minute,
	})
	if err != nil {
		return nil, err
	}

	if p.MacaroonExpiryDuration == 0 {
		p.MacaroonExpiryDuration = 24 * time.Hour
	}

	return auth.JujuAuthenticator{
		Bakery: identchecker.NewBakery(identchecker.BakeryParams{
			RootKeyStore: dbrootkeystore.NewRootKeys(100, nil).NewStore(
				db,
				dbrootkeystore.Policy{
					ExpiryDuration: p.MacaroonExpiryDuration,
				},
			),
			Locator:        httpbakery.NewThirdPartyLocator(nil, tps),
			Key:            key,
			IdentityClient: candidClient,
			Location:       "jimm",
			Logger:         logger.BakeryLogger{},
		}),
		ControllerAdmins: p.ControllerAdmins,
		Client:           client,
	}, nil
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
		user := openfga.NewUser(&dbmodel.User{Username: userTag.Id()}, client)
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
