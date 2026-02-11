// Copyright 2026 Canonical.

package dbmodel

import "embed"

// SQL contains SQL scripts for managing the database.
//
//go:embed sql/*/*.sql
var SQL embed.FS
