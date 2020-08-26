// Copyright 2020 Canonical Ltd.

package main

import (
	"context"
	"encoding/json"
	"os"

	vault "github.com/hashicorp/vault/api"
	"go.uber.org/zap"
	"gopkg.in/errgo.v1"

	"github.com/CanonicalLtd/jimm/config"
	"github.com/CanonicalLtd/jimm/internal/zapctx"
)

// newVaultClient returns the vault client and kv store path, if configured.
func newVaultClient(ctx context.Context, conf *config.Config) (*vault.Client, string) {
	if conf.Vault.Address == "" {
		zapctx.Info(ctx, "vault not configured")
		return nil, ""
	}

	vConfig := vault.DefaultConfig()
	vConfig.Address = conf.Vault.Address

	vClient, err := vault.NewClient(vConfig)
	if err != nil {
		zapctx.Error(ctx, "cannot create vault client", zap.Error(err))
		return nil, ""
	}

	s, err := loadVaultSecret(ctx, conf.Vault.AuthSecretPath, vClient, conf.Vault.WrappedSecret)
	if err != nil {
		zapctx.Error(ctx, "cannot load vault secret", zap.Error(err))
		return nil, ""
	}

	tok, err := s.TokenID()
	if err != nil {
		zapctx.Error(ctx, "invalid vault secret", zap.Error(err))
		return nil, ""
	}
	vClient.SetToken(tok)

	w, err := vClient.NewLifetimeWatcher(&vault.LifetimeWatcherInput{
		Secret: s,
	})
	if err != nil {
		zapctx.Error(ctx, "cannot create lifetime watcher", zap.Error(err))
		return vClient, conf.Vault.KVPrefix
	}
	go func() {
		for {
			r := <-w.RenewCh()
			zapctx.Debug(ctx, "renewed auth secret", zap.Time("renewed-at", r.RenewedAt))
			if err := writeSecret(conf.Vault.AuthSecretPath, r.Secret); err != nil {
				zapctx.Error(ctx, "cannot write secret", zap.Error(err))
			}
		}
	}()
	w.Start()

	return vClient, conf.Vault.KVPrefix
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
		return errgo.Mask(err)
	}
	defer f.Close()
	if err := json.NewEncoder(f).Encode(s); err != nil {
		return errgo.Mask(err)
	}
	return nil
}
