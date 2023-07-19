// Copyright 2021 Canonical Ltd.

package main

import (
	"context"
	"net/http"
	"os"
	"strings"
	"syscall"
	"time"

	service "github.com/canonical/go-service"
	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"

	"github.com/canonical/jimm"
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
	jimmsvc, err := jimm.NewService(ctx, jimm.Params{
		ControllerUUID:    os.Getenv("JIMM_UUID"),
		DSN:               os.Getenv("JIMM_DSN"),
		CandidURL:         os.Getenv("CANDID_URL"),
		CandidPublicKey:   os.Getenv("CANDID_PUBLIC_KEY"),
		BakeryAgentFile:   os.Getenv("BAKERY_AGENT_FILE"),
		ControllerAdmins:  strings.Fields(os.Getenv("JIMM_ADMINS")),
		VaultSecretFile:   os.Getenv("VAULT_SECRET_FILE"),
		VaultAddress:      os.Getenv("VAULT_ADDR"),
		VaultAuthPath:     os.Getenv("VAULT_AUTH_PATH"),
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
		PrivateKey:              os.Getenv("BAKERY_PRIVATE_KEY"),
		PublicKey:               os.Getenv("BAKERY_PUBLIC_KEY"),
		AuditLogRetentionPeriod: os.Getenv("JIMM_AUDIT_LOG_RETENTION_PERIOD"),
	})
	if err != nil {
		return err
	}
	if os.Getenv("JIMM_WATCH_CONTROLLERS") != "" {
		s.Go(func() error { return jimmsvc.WatchControllers(ctx) }) // Deletes dead/dying models, updates model config.
		s.Go(func() error { return jimmsvc.PollModels(ctx) })       // Poll for access control changes on the controller.
	}
	s.Go(func() error { return jimmsvc.WatchModelSummaries(ctx) })

	if os.Getenv("JIMM_ENABLE_JWKS_ROTATOR") != "" {
		zapctx.Info(ctx, "attempting to start JWKS rotator")
		s.Go(func() error {
			err := jimmsvc.StartJWKSRotator(ctx, time.NewTicker(time.Hour).C, time.Now().UTC().AddDate(0, 3, 0))
			zapctx.Error(ctx, "failed to start JWKS rotator", zap.Error(err))
			return err
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
	})
	s.Go(httpsrv.ListenAndServe)
	jimmsvc.RegisterJwksCache(ctx)
	zapctx.Info(ctx, "Successfully started JIMM server")
	return nil
}
