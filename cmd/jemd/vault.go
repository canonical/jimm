// Copyright 2020 Canonical Ltd.

package main

import (
	"context"
	"encoding/json"
	"os"

	vault "github.com/hashicorp/vault/api"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
	"gopkg.in/errgo.v1"

	"github.com/CanonicalLtd/jimm/config"
	"github.com/CanonicalLtd/jimm/internal/servermon"
	"github.com/CanonicalLtd/jimm/internal/zapctx"
)

// startVaultClient creates a client to the configured vault server,
// starting the background services needed to maintain the connection.
func startVaultClient(ctx context.Context, eg *errgroup.Group, conf config.VaultConfig) (*vault.Client, error) {
	servermon.VaultConfigured.Inc()
	vConfig := vault.DefaultConfig()
	vConfig.Address = conf.Address

	vClient, err := vault.NewClient(vConfig)
	if err != nil {
		return nil, errgo.Notef(err, "cannot create vault client")
	}

	s, err := loadVaultSecret(ctx, conf.AuthSecretPath, vClient, conf.WrappedSecret)
	if err != nil {
		return nil, errgo.Notef(err, "cannot load vault secret")
	}

	tok, err := s.TokenID()
	if err != nil {
		return nil, errgo.Notef(err, "invalid vault secret")
	}
	vClient.SetToken(tok)

	w, err := vClient.NewLifetimeWatcher(&vault.LifetimeWatcherInput{
		Secret: s,
	})
	if err != nil {
		return nil, errgo.Notef(err, "cannot create lifetime watcher")
	}
	eg.Go(func() error {
		for {
			select {
			case r := <-w.RenewCh():
				servermon.VaultSecretRefreshes.Inc()
				zapctx.Debug(ctx, "renewed auth secret", zap.Time("renewed-at", r.RenewedAt))
				if err := writeSecret(conf.AuthSecretPath, r.Secret); err != nil {
					zapctx.Error(ctx, "cannot write secret", zap.Error(err))
				}
			case err := <-w.DoneCh():
				return errgo.Mask(err)
			}
		}
	})
	w.Start()

	eg.Go(func() error {
		<-ctx.Done()
		w.Stop()
		return ctx.Err()
	})

	return vClient, nil
}

// loadVaultSecret loads the vault secret from the given path, if the
// secret cannot be found, or is expired
func loadVaultSecret(ctx context.Context, path string, client *vault.Client, wrappedToken string) (*vault.Secret, error) {
	f, err := os.Open(path)
	switch {
	case err == nil:
		s, err := vault.ParseSecret(f)
		return s, errgo.Mask(err)
	case os.IsNotExist(err):
		// Attempt to unwrap the configured token (below).
	default:
		return nil, errgo.Mask(err)
	}

	s, err := client.Logical().Unwrap(wrappedToken)
	if err != nil {
		return nil, errgo.Mask(err)
	}
	zapctx.Debug(ctx, "unwrapped secret")
	if err := writeSecret(path, s); err != nil {
		zapctx.Error(ctx, "cannot write secret", zap.Error(err))
	}
	return s, nil
}

func writeSecret(path string, s *vault.Secret) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		servermon.VaultSecretWriteErrors.Inc()
		return errgo.Mask(err)
	}
	defer f.Close()
	if err := json.NewEncoder(f).Encode(s); err != nil {
		servermon.VaultSecretWriteErrors.Inc()
		return errgo.Mask(err)
	}
	return nil
}
