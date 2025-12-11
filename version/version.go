// Copyright 2024 Canonical.

package version

import "strings"

const (
	// ControllerVersion is the controller version and client JIMM claims to be.
	ControllerVersion = "3.6.12"
)

// Version describes the current version of the code being run.
type Version struct {
	GitCommit string
	Version   string
}

// VersionInfo is a variable representing the version of the currently
// executing code. Builds of the system where the version information is
// required must arrange to provide the correct values in the files
// commit.txt and version.txt.
var VersionInfo = Version{
	GitCommit: strings.TrimSpace(commit),
	Version:   strings.TrimSpace(version),
}
