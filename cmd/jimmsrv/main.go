// Copyright 2021 Canonical Ltd.

package main

import (
	"context"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"

	service "github.com/canonical/go-service"
	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"

	"github.com/canonical/jimm"
	"github.com/canonical/jimm/internal/errors"
	"github.com/canonical/jimm/version"
)

func main() {
	ctx, s := service.NewService(context.Background(), os.Interrupt, syscall.SIGTERM)
	s.Go(func() error {
		return start(ctx, s)
	})
	err := s.Wait()

	zapctx.Error(context.Background(), "shutdown", zap.Error(err))
	if _, ok := err.(*service.SignalError); !ok {
		os.Exit(1)
	}
}

// start initialises the jimmsrv service.
func start(ctx context.Context, s *service.Service) error {
	zapctx.Info(ctx, "jimm info",
		zap.String("version", version.VersionInfo.Version),
		zap.String("commit", version.VersionInfo.GitCommit),
	)
	if logLevel := os.Getenv("JIMM_LOG_LEVEL"); logLevel != "" {
		if err := zapctx.LogLevel.UnmarshalText([]byte(logLevel)); err != nil {
			zapctx.Error(ctx, "cannot set log level", zap.Error(err))
		}
	}
	// TODO(mhilton) access logs?
	addr := os.Getenv("JIMM_LISTEN_ADDR")
	if addr == "" {
		addr = ":http-alt"
	}
	macaroonExpiryDuration := 24 * time.Hour
	durationString := os.Getenv("JIMM_MACAROON_EXPIRY_DURATION")
	if durationString != "" {
		expiry, err := time.ParseDuration(durationString)
		if err != nil {
			zapctx.Error(ctx, "failed to parse macaroon expiry duration", zap.Error(err))
		}
		macaroonExpiryDuration = expiry
	}
	jwtExpiryDuration := 24 * time.Hour
	durationString = os.Getenv("JIMM_JWT_EXPIRY")
	if durationString != "" {
		expiry, err := time.ParseDuration(durationString)
		if err != nil {
			zapctx.Error(ctx, "failed to parse jwt expiry duration", zap.Error(err))
		} else {
			jwtExpiryDuration = expiry
		}
	}

	sessionTokenExpiryDuration := time.Duration(0)
	durationString = os.Getenv("JIMM_ACCESS_TOKEN_EXPIRY_DURATION")
	if durationString != "" {
		expiry, err := time.ParseDuration(durationString)
		if err != nil {
			zapctx.Error(ctx, "failed to parse access token expiry duration", zap.Error(err))
			return err
		}
		sessionTokenExpiryDuration = expiry
	}

	issuerURL := os.Getenv("JIMM_OAUTH_ISSUER_URL")
	parsedIssuerURL, err := url.Parse(issuerURL)
	if err != nil {
		zapctx.Error(ctx, "failed to parse oauth issuer url", zap.Error(err))
		return err
	}

	if parsedIssuerURL.Scheme == "" {
		zapctx.Error(ctx, "oauth issuer url has no scheme")
		return errors.E("oauth issuer url has no scheme")
	}

	clientID := os.Getenv("JIMM_OAUTH_CLIENT_ID")
	if clientID == "" {
		zapctx.Error(ctx, "no oauth client id")
		return errors.E("no oauth client id")
	}

	clientSecret := os.Getenv("JIMM_OAUTH_CLIENT_SECRET")
	if clientSecret == "" {
		zapctx.Error(ctx, "no oauth client secret")
		return errors.E("no oauth client secret")
	}

	scopes := os.Getenv("JIMM_OAUTH_SCOPES")
	scopesParsed := strings.Split(scopes, " ")
	for i, scope := range scopesParsed {
		scopesParsed[i] = strings.TrimSpace(scope)
	}
	zapctx.Info(ctx, "oauth scopes", zap.Any("scopes", scopesParsed))
	if len(scopesParsed) == 0 {
		zapctx.Error(ctx, "no oauth client scopes present")
		return errors.E("no oauth client scopes present")
	}

	insecureSecretStorage := false
	if _, ok := os.LookupEnv("INSECURE_SECRET_STORAGE"); ok {
		insecureSecretStorage = true
	}

	secureSessionCookies := false
	if _, ok := os.LookupEnv("JIMM_SECURE_SESSION_COOKIES"); ok {
		secureSessionCookies = true
	}

	sessionCookieMaxAge := os.Getenv("JIMM_SESSION_COOKIE_MAX_AGE")
	sessionCookieMaxAgeInt, err := strconv.Atoi(sessionCookieMaxAge)
	if err != nil {
		return errors.E("unable to parse jimm session cookie max age")
	}
	if sessionCookieMaxAgeInt < 0 {
		return errors.E("jimm session cookie max age cannot be less than 0")
	}

	sessionStoreSecret := os.Getenv("JIMM_SESSION_SECRET_KEY")
	if sessionStoreSecret == "" || len(sessionStoreSecret) < 64 {
		return errors.E("jimm session store secret must be at least 64 characters")
	}

	jimmsvc, err := jimm.NewService(ctx, jimm.Params{
		ControllerUUID:    os.Getenv("JIMM_UUID"),
		DSN:               os.Getenv("JIMM_DSN"),
		ControllerAdmins:  strings.Fields(os.Getenv("JIMM_ADMINS")),
		VaultRoleID:       os.Getenv("VAULT_ROLE_ID"),
		VaultRoleSecretID: os.Getenv("VAULT_ROLE_SECRET_ID"),
		VaultAddress:      os.Getenv("VAULT_ADDR"),
		VaultPath:         os.Getenv("VAULT_PATH"),
		DashboardLocation: os.Getenv("JIMM_DASHBOARD_LOCATION"),
		PublicDNSName:     os.Getenv("JIMM_DNS_NAME"),
		OpenFGAParams: jimm.OpenFGAParams{
			Scheme:    os.Getenv("OPENFGA_SCHEME"),
			Host:      os.Getenv("OPENFGA_HOST"),
			Store:     os.Getenv("OPENFGA_STORE"),
			AuthModel: os.Getenv("OPENFGA_AUTH_MODEL"),
			Token:     os.Getenv("OPENFGA_TOKEN"),
			Port:      os.Getenv("OPENFGA_PORT"),
		},
		PrivateKey:                    os.Getenv("BAKERY_PRIVATE_KEY"),
		PublicKey:                     os.Getenv("BAKERY_PUBLIC_KEY"),
		AuditLogRetentionPeriodInDays: os.Getenv("JIMM_AUDIT_LOG_RETENTION_PERIOD_IN_DAYS"),
		MacaroonExpiryDuration:        macaroonExpiryDuration,
		JWTExpiryDuration:             jwtExpiryDuration,
		InsecureSecretStorage:         insecureSecretStorage,
		OAuthAuthenticatorParams: jimm.OAuthAuthenticatorParams{
			IssuerURL:           issuerURL,
			ClientID:            clientID,
			ClientSecret:        clientSecret,
			Scopes:              scopesParsed,
			SessionTokenExpiry:  sessionTokenExpiryDuration,
			SessionCookieMaxAge: sessionCookieMaxAgeInt,
			SessionSecretKey:    sessionStoreSecret,
		},
		DashboardFinalRedirectURL: os.Getenv("JIMM_DASHBOARD_FINAL_REDIRECT_URL"),
		SecureSessionCookies:      secureSessionCookies,
		SessionStoreSecret:        []byte(sessionStoreSecret),
	})
	if err != nil {
		return err
	}

	isLeader := os.Getenv("JIMM_IS_LEADER") != ""
	// Remove extra check besides isLeader once the charm is updated.
	if isLeader || os.Getenv("JIMM_WATCH_CONTROLLERS") != "" {
		s.Go(func() error { return jimmsvc.WatchControllers(ctx) }) // Deletes dead/dying models, updates model config.
	}

	// Remove extra check besides isLeader once the charm is updated.
	if isLeader || os.Getenv("JIMM_ENABLE_JWKS_ROTATOR") != "" {
		zapctx.Info(ctx, "attempting to start JWKS rotator and generate OAuth secret key")
		s.Go(func() error {
			if err := jimmsvc.StartJWKSRotator(ctx, time.NewTicker(time.Hour).C, time.Now().UTC().AddDate(0, 3, 0)); err != nil {
				zapctx.Error(ctx, "failed to start JWKS rotator", zap.Error(err))
				return err
			}
			return nil
		})
	}

	httpsrv := &http.Server{
		Addr:    addr,
		Handler: jimmsvc,
	}
	s.OnShutdown(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		zapctx.Warn(ctx, "server shutdown triggered")
		httpsrv.Shutdown(ctx)
		jimmsvc.Cleanup()
	})
	s.Go(httpsrv.ListenAndServe)
	zapctx.Info(ctx, "Successfully started JIMM server")
	return nil
}
