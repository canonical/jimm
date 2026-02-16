// Copyright 2026 Canonical.

package cmd

//go:generate go tool mockgen -package mocks -typed -destination ./mocks/client_mock.go github.com/canonical/jimm/v3/cmd/jaas/cmd JIMMAPI,AddModelCloudAPI
//go:generate go tool mockgen -package mocks -typed -destination ./mocks/io_writer_mock.go io Writer
//go:generate go tool mockgen -package mocks -typed -destination ./mocks/migrate_mock.go github.com/canonical/jimm/v3/cmd/jaas/cmd MigrateAPI
//go:generate go tool mockgen -package mocks -typed -destination ./mocks/store_mock.go github.com/juju/juju/jujuclient ClientStore
