// Copyright 2015 Canonical Ltd.

package version

import (
	"bytes"
	"embed"
)

//go:embed v
var fs embed.FS

func init() {
	b, err := fs.ReadFile("v/git-commit")
	if err == nil {
		VersionInfo.GitCommit = string(bytes.TrimSpace(b))
	}
	b, err = fs.ReadFile("v/version")
	if err == nil {
		VersionInfo.Version = string(bytes.TrimSpace(b))
	}
}

// Version describes the current version of the code being run.
type Version struct {
	GitCommit string
	Version   string
}

// VersionInfo is a variable representing the version of the currently
// executing code. Builds of the system where the version information
// is required must arrange to provide the correct values for this
// variable. One possible way to do this is to create an init() function
// that updates this variable.
var VersionInfo = unknownVersion

var unknownVersion = Version{
	GitCommit: "unknown git commit",
	Version:   "unknown version",
}
