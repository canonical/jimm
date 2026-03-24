// Copyright 2025 Canonical.

package main

import (
	"context"
	"fmt"
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

	jimmsvc "github.com/canonical/jimm/v3/cmd/jimmsrv/service"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/jimm/config"
	"github.com/canonical/jimm/v3/internal/logger"
	"github.com/canonical/jimm/v3/internal/river"
	"github.com/canonical/jimm/v3/internal/ssh"
	"github.com/canonical/jimm/v3/version"
)

func main() {
	ctx, s := service.NewService(context.Background(), os.Interrupt, syscall.SIGTERM)
	s.Go(func() error {
		return start(ctx, s)
	})
	err := s.Wait()

	zapctx.Error(context.Background(), "jimm shutdown complete", zap.Error(err))
	if _, ok := err.(*service.SignalError); !ok {
		os.Exit(1)
	}
}

// start initialises the jimmsrv service.
//
//nolint:gocognit // Start function to be ignored.
func start(ctx context.Context, s *service.Service) error {
	logLevel := os.Getenv("JIMM_LOG_LEVEL")
	logDevMode, _ := strconv.ParseBool(os.Getenv("JIMM_LOG_DEV_MODE"))
	logger.SetupLogger(ctx, logLevel, logDevMode)
	logger.LogJimmStartup(ctx)
	zapctx.Info(ctx, "jimm info",
		zap.String("version", version.VersionInfo.Version),
		zap.String("commit", version.VersionInfo.GitCommit),
	)
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
		return errors.New("oauth issuer url has no scheme")
	}

	clientID := os.Getenv("JIMM_OAUTH_CLIENT_ID")
	if clientID == "" {
		return errors.New("no oauth client id")
	}

	clientSecret := os.Getenv("JIMM_OAUTH_CLIENT_SECRET")
	if clientSecret == "" {
		return errors.New("no oauth client secret")
	}

	scopes := os.Getenv("JIMM_OAUTH_SCOPES")
	scopesParsed := strings.Split(scopes, " ")
	for i, scope := range scopesParsed {
		scopesParsed[i] = strings.TrimSpace(scope)
	}
	zapctx.Info(ctx, "oauth scopes", zap.Any("scopes", scopesParsed))
	if len(scopesParsed) == 0 {
		return errors.New("no oauth client scopes present")
	}

	insecureSecretStorage := false
	if key, ok := os.LookupEnv("INSECURE_SECRET_STORAGE"); ok && key != "" {
		insecureSecretStorage, err = strconv.ParseBool(key)
		if err != nil {
			return fmt.Errorf("failed to parse INSECURE_SECRET_STORAGE env var: %v", err)
		}
	}
	if insecureSecretStorage {
		zapctx.Warn(ctx, "insecure secret storage is enabled, this is not recommended for production use")
	}

	secureSessionCookies := false
	if _, ok := os.LookupEnv("JIMM_SECURE_SESSION_COOKIES"); ok {
		secureSessionCookies = true
	}

	sessionCookieMaxAge := os.Getenv("JIMM_SESSION_COOKIE_MAX_AGE")
	sessionCookieMaxAgeInt, err := strconv.Atoi(sessionCookieMaxAge)
	if err != nil {
		return errors.New("unable to parse jimm session cookie max age")
	}
	if sessionCookieMaxAgeInt < 0 {
		return errors.New("jimm session cookie max age cannot be less than 0")
	}

	sessionSecretKey := os.Getenv("JIMM_SESSION_SECRET_KEY")
	if len(sessionSecretKey) < 64 {
		return errors.New("jimm session store secret must be at least 64 characters")
	}

	hostKeyRaw := os.Getenv("JIMM_SSH_HOST_KEY")
	if hostKeyRaw == "" {
		return errors.New("empty hostkey from env variable")
	}
	maxConcurrentConnections, _ := strconv.Atoi(os.Getenv("JIMM_SSH_MAX_CONCURRENT_CONNECTIONS"))
	publicHostKey, err := ssh.GetPublicKeyFromPrivateKey([]byte(hostKeyRaw))
	if err != nil {
		return errors.New("cannot parse hostkey from env variable")
	}
	fingerprints, err := ssh.GetFingerprintsFromPrivateKey([]byte(hostKeyRaw))
	if err != nil {
		return errors.New("cannot parse hostkey from env variable")
	}

	corsAllowedOrigins := strings.Split(os.Getenv("CORS_ALLOWED_ORIGINS"), " ")

	logSQL, _ := strconv.ParseBool(os.Getenv("JIMM_LOG_SQL"))

	sshPort := os.Getenv("JIMM_SSH_PORT")
	if sshPort == "" {
		return errors.New("empty ssh port from env variable")
	}
	sshPortInt, err := strconv.Atoi(sshPort)
	if err != nil {
		return errors.E("failed to parse ssh port", zap.Error(err))
	}
	jimmUUID := os.Getenv("JIMM_UUID")
	publicDnsName := os.Getenv("JIMM_DNS_NAME")
	crossModelQueryTimeout := 5 * time.Second
	crossModelQueryTimeoutRaw := os.Getenv("JIMM_CROSS_MODEL_QUERY_TIMEOUT")
	if crossModelQueryTimeoutRaw != "" {
		expiry, err := time.ParseDuration(crossModelQueryTimeoutRaw)
		if err != nil {
			return errors.New("cannot parse cross model query timeout into duration")
		} else {
			crossModelQueryTimeout = expiry
		}
	}

	internalAddr := os.Getenv("JIMM_INTERNAL_LISTEN_ADDR")
	if internalAddr == "" {
		internalAddr = ":9090"
	}
	internalSrv := jimmsvc.NewInternalService(internalAddr, corsAllowedOrigins)

	jimmsvc, err := jimmsvc.NewService(ctx, jimmsvc.Params{
		ControllerUUID:      jimmUUID,
		DSN:                 os.Getenv("JIMM_DSN"),
		HostKeyFingerprints: fingerprints,
		ControllerAdmins:    strings.Fields(os.Getenv("JIMM_ADMINS")),
		VaultRoleID:         os.Getenv("VAULT_ROLE_ID"),
		VaultRoleSecretID:   os.Getenv("VAULT_ROLE_SECRET_ID"),
		VaultAddress:        os.Getenv("VAULT_ADDR"),
		VaultPath:           os.Getenv("VAULT_PATH"),
		PublicDNSName:       publicDnsName,
		OpenFGAParams: jimmsvc.OpenFGAParams{
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
		OAuthAuthenticatorParams: jimmsvc.OAuthAuthenticatorParams{
			IssuerURL:            issuerURL,
			ClientID:             clientID,
			ClientSecret:         clientSecret,
			Scopes:               scopesParsed,
			SessionTokenExpiry:   sessionTokenExpiryDuration,
			SessionCookieMaxAge:  sessionCookieMaxAgeInt,
			JWTSessionKey:        sessionSecretKey,
			SecureSessionCookies: secureSessionCookies,
			AuthStyle:            os.Getenv("JIMM_OAUTH_AUTH_STYLE"),
		},
		DashboardFinalRedirectURL: os.Getenv("JIMM_DASHBOARD_FINAL_REDIRECT_URL"),
		CookieSessionKey:          []byte(sessionSecretKey),
		CorsAllowedOrigins:        corsAllowedOrigins,
		LogSQL:                    logSQL,
		LogLevel:                  logLevel,
		IsLeader:                  os.Getenv("JIMM_IS_LEADER") != "",
		ControllerConfig: config.ControllerConfig{
			ControllerUUID:   jimmUUID,
			PublicDNSName:    publicDnsName,
			SSHPort:          sshPortInt,
			SSHPublicHostKey: publicHostKey,
		},
		CrossModelQueryTimeout:        crossModelQueryTimeout,
		BootstrapLoginTokenRefreshURL: os.Getenv("JIMM_BOOTSTRAP_LOGIN_TOKEN_REFRESH_URL"),
	})
	if err != nil {
		return err
	}

	jimmsvc.StartServices(ctx, s)

	httpsrv := &http.Server{
		Addr:              addr,
		Handler:           jimmsvc,
		ReadHeaderTimeout: time.Second * 30,
	}

	sshServer, err := ssh.NewJumpServer(ctx, ssh.Config{
		Port:                     sshPort,
		HostKey:                  []byte(hostKeyRaw),
		MaxConcurrentConnections: maxConcurrentConnections,
	}, jimmsvc.JIMM().SSHManager)

	s.OnShutdown(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		logger.LogJimmShutdown(ctx)

		zapctx.Warn(ctx, "HTTP server shutdown triggered")
		err = httpsrv.Shutdown(ctx)
		if err != nil {
			zapctx.Error(ctx, "failed to shutdown HTTP server gracefully", zap.Error(err))
		}

		zapctx.Warn(ctx, "Internal HTTP server shutdown triggered")
		err = internalSrv.Shutdown(ctx)
		if err != nil {
			zapctx.Error(ctx, "failed to shutdown internal HTTP server gracefully", zap.Error(err))
		}

		zapctx.Warn(ctx, "SSH server shutdown triggered")
		err = sshServer.Shutdown(ctx)
		if err != nil {
			zapctx.Error(ctx, "failed to shutdown SSH server gracefully", zap.Error(err))
		}

		jimmsvc.Cleanup()
	})
	err = river.MigrateRiver(ctx, jimmsvc.JIMM().Database)
	if err != nil {
		return err
	}
	err = river.StartWorkers(
		ctx,
		jimmsvc.JIMM().Database,
		jimmsvc.JIMM().OpenFGAClient,
		jimmsvc.JIMM().UpgradeManager,
		jimmsvc.JIMM().BootstrapManager,
	)
	if err != nil {
		return err
	}
	zapctx.Info(ctx, "Registered all River workers")
	s.Go(httpsrv.ListenAndServe)
	zapctx.Info(ctx, "Started JIMM HTTP server")
	s.Go(internalSrv.ListenAndServe)
	zapctx.Info(ctx, "Started JIMM internal HTTP server")
	s.Go(sshServer.ListenAndServe)
	zapctx.Info(ctx, "Started JIMM SSH server")
	return nil
}
