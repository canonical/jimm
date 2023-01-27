// Copyright 2021 Canonical Ltd.

package jimm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/canonical/candid/candidclient"
	"github.com/go-chi/chi/v5"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery/dbrootkeystore"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery/identchecker"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/httpbakery"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/httpbakery/agent"
	"github.com/google/uuid"
	vaultapi "github.com/hashicorp/vault/api"
	"github.com/juju/zaputil/zapctx"
	openfga "github.com/openfga/go-sdk"
	"github.com/openfga/go-sdk/credentials"
	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/CanonicalLtd/jimm/internal/auth"
	"github.com/CanonicalLtd/jimm/internal/dashboard"
	"github.com/CanonicalLtd/jimm/internal/db"
	"github.com/CanonicalLtd/jimm/internal/debugapi"
	"github.com/CanonicalLtd/jimm/internal/errors"
	"github.com/CanonicalLtd/jimm/internal/jimm"
	"github.com/CanonicalLtd/jimm/internal/jimmhttp"
	"github.com/CanonicalLtd/jimm/internal/jujuapi"
	"github.com/CanonicalLtd/jimm/internal/jujuclient"
	"github.com/CanonicalLtd/jimm/internal/logger"
	ofgaClient "github.com/CanonicalLtd/jimm/internal/openfga"
	"github.com/CanonicalLtd/jimm/internal/pubsub"
	"github.com/CanonicalLtd/jimm/internal/servermon"
	"github.com/CanonicalLtd/jimm/internal/vault"
	"github.com/CanonicalLtd/jimm/internal/wellknownapi"
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
}

// A Service is the implementation of a JIMM server.
type Service struct {
	jimm jimm.JIMM

	mux *chi.Mux
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

// PollModels regularly polls each known model to get required data not
// included in the multiwatcher deltas. PollModels finishes when the given
// context is canceled, or there is a fatal error polling models.
func (s *Service) PollModels(ctx context.Context) error {
	w := jimm.Watcher{
		Database: s.jimm.Database,
		Dialer:   s.jimm.Dialer,
	}
	return w.PollModels(ctx, 10*time.Minute)
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
		p.DSN = "file::memory:?mode=memory&cache=shared"
	}
	var err error
	s.jimm.Database.DB, err = openDB(ctx, p.DSN)
	if err != nil {
		return nil, errors.E(op, err)
	}
	if err := s.jimm.Database.Migrate(ctx, false); err != nil {
		return nil, errors.E(op, err)
	}

	s.jimm.Authenticator, err = newAuthenticator(ctx, &s.jimm.Database, p)
	if err != nil {
		return nil, errors.E(op, err)
	}

	vs, err := newVaultStore(ctx, p)
	if err != nil {
		return nil, errors.E(op, err)
	}
	if vs != nil {
		s.jimm.CredentialStore = vs
	}

	openFGAclient, err := newOpenFGAClient(ctx, p)
	if err != nil {
		return nil, errors.E(op, err)
	}
	if openFGAclient != nil {
		s.jimm.OpenFGAClient = openFGAclient
	}

	s.jimm.Dialer = &jujuclient.Dialer{
		ControllerCredentialsStore: vs,
	}
	if !p.DisableConnectionCache {
		s.jimm.Dialer = jimm.CacheDialer(s.jimm.Dialer)
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

	// TODO: Have ticker stopped on graceful shutdown using closure func
	if _, err := vs.StartJWKSRotator(ctx, time.NewTicker(time.Hour), time.Now().AddDate(0, 3, 1)); err != nil {
		zapctx.Error(ctx, "failed to start jwks rotator", zap.Error(err))
		os.Exit(3)
	}
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
	s.mux.Handle("/model/", jujuapi.ModelHandler(ctx, &s.jimm, params))
	// If the request is not for a known path assume it is part of the dashboard.
	// If dashboard location env var is not defined, do not handle a dashboard.
	if p.DashboardLocation != "" {
		s.mux.Handle("/", dashboard.Handler(ctx, p.DashboardLocation))
	}

	return s, nil
}

func openDB(ctx context.Context, dsn string) (*gorm.DB, error) {
	zapctx.Info(ctx, "connecting database")

	var dialect gorm.Dialector
	switch {
	case strings.HasPrefix(dsn, "pgx:"):
		dialect = postgres.Open(strings.TrimPrefix(dsn, "pgx:"))
	case strings.HasPrefix(dsn, "postgres:") || strings.HasPrefix(dsn, "postgresql:"):
		dialect = postgres.Open(dsn)
	case strings.HasPrefix(dsn, "file:"):
		dialect = sqlite.Open(dsn)
	default:
		return nil, errors.E(errors.CodeServerConfiguration, "unsupported DSN")
	}
	return gorm.Open(dialect, &gorm.Config{
		Logger: logger.GormLogger{},
	})
}

func newAuthenticator(ctx context.Context, db *db.Database, p Params) (jimm.Authenticator, error) {
	if p.CandidURL == "" {
		// No authenticator configured
		return nil, nil
	}
	zapctx.Info(ctx, "configuring authenticator",
		zap.String("CandidURL", p.CandidURL),
		zap.String("CandidPublicKey", p.CandidPublicKey),
		zap.String("BakeryAgentFile", p.BakeryAgentFile),
	)
	key, err := bakery.GenerateKey()
	if err != nil {
		return nil, err
	}
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
	return auth.JujuAuthenticator{
		Bakery: identchecker.NewBakery(identchecker.BakeryParams{
			RootKeyStore: dbrootkeystore.NewRootKeys(100, nil).NewStore(db, dbrootkeystore.Policy{
				ExpiryDuration: 24 * time.Hour,
			}),
			Locator:        httpbakery.NewThirdPartyLocator(nil, tps),
			Key:            key,
			IdentityClient: candidClient,
			Location:       "jimm",
			Logger:         logger.BakeryLogger{},
		}),
		ControllerAdmins: p.ControllerAdmins,
	}, nil
}

func newVaultStore(ctx context.Context, p Params) (jimm.CredentialStore, error) {
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
	if err != nil {
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

func newOpenFGAClient(ctx context.Context, p Params) (*ofgaClient.OFGAClient, error) {
	if p.OpenFGAParams.Host == "" {
		return nil, nil
	}
	zapctx.Info(ctx, "configuring OpenFGA client",
		zap.String("OpenFGA host", p.OpenFGAParams.Host),
		zap.String("OpenFGA scheme", p.OpenFGAParams.Scheme),
		zap.String("OpenFGA store", p.OpenFGAParams.Store),
	)

	config := openfga.Configuration{
		ApiScheme: p.OpenFGAParams.Scheme,
		ApiHost:   fmt.Sprintf("%s:%s", p.OpenFGAParams.Host, p.OpenFGAParams.Port), // required, define without the scheme (e.g. api.fga.example instead of https://api.fga.example)
		StoreId:   p.OpenFGAParams.Store,
	}
	if p.OpenFGAParams.Token != "" {
		config.Credentials = &credentials.Credentials{
			Method: credentials.CredentialsMethodApiToken,
			Config: &credentials.Config{
				ApiToken: p.OpenFGAParams.Token,
			},
		}
	}
	configuration, err := openfga.NewConfiguration(config)
	if err != nil {
		return nil, err
	}
	client := openfga.NewAPIClient(configuration)
	api := client.OpenFgaApi

	_, response, err := api.ListStores(ctx).Execute()
	if err != nil {
		return nil, err
	}
	body, _ := io.ReadAll(response.Body)
	if response.StatusCode != http.StatusOK {
		return nil, errors.E("failed to contact the OpenFga server: received %v: %s", response.StatusCode, string(body))
	}

	storeResp, _, err := api.GetStore(ctx).Execute()
	if err != nil {
		zapctx.Error(ctx, "could not retrieve store.", zap.Error(err))
		return nil, errors.E("could not retrieve store")
	} else {
		zapctx.Info(ctx, "store appears to exist", zap.String("store-name", *storeResp.Name))
	}
	return ofgaClient.NewOpenFGAClient(client.OpenFgaApi, p.OpenFGAParams.AuthModel), nil
}
