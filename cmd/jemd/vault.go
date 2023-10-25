// Copyright 2020 Canonical Ltd.

package main

import (
	"context"
	"path"

	vault "github.com/hashicorp/vault/api"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
	"gopkg.in/errgo.v1"

	"github.com/canonical/jimm/config"
	"github.com/canonical/jimm/internal/servermon"
	"github.com/canonical/jimm/internal/zapctx"
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

	if conf.ApprolePath == "" {
		conf.ApprolePath = "auth/approle"
	}

	s, err := vClient.Logical().Write(path.Join(conf.ApprolePath, "login"), map[string]interface{}{
		"role_id":   conf.ApproleRoleID,
		"secret_id": conf.ApproleSecretID,
	})
	if err != nil {
		return nil, errgo.Notef(err, "cannot authenticate to vault")
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
			case err := <-w.DoneCh():
				return errgo.Mask(err)
			}
		}
	})
	eg.Go(func() error {
		w.Start()
		return nil
	})
	eg.Go(func() error {
		defer w.Stop()
		<-ctx.Done()
		return ctx.Err()
	})

	return vClient, nil
}
