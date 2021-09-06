// Copyright 2021 Canonical Ltd.

// +build version
//go:build version

package version

import (
	_ "embed"
)

//go:embed version.txt
var version string

//go:embed commit.txt
var commit string
