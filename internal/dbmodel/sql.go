// Copyright 2021 Canoncial Ltd.

package dbmodel

import "embed"

// SQL contains SQL scripts for managing the database.
//go:embed sql/*/*.sql
var SQL embed.FS
