// Copyright 2015 Canonical Ltd.

package jemcmd

import (
	"os"

	"github.com/juju/cmd"
)

// jujuLoggingConfigEnvKey matches osenv.JujuLoggingConfigEnvKey
// in the Juju project.
const jujuLoggingConfigEnvKey = "JUJU_LOGGING_CONFIG"

var cmdDoc = `
The juju jem command provides ... TODO.
`

// New returns a command that can execute juju-jem
// commands.
func New() cmd.Command {
	supercmd := cmd.NewSuperCommand(cmd.SuperCommandParams{
		Name:        "jem",
		UsagePrefix: "juju",
		Doc:         cmdDoc,
		Purpose:     "TODO",
		Log: &cmd.Log{
			DefaultConfig: os.Getenv(jujuLoggingConfigEnvKey),
		},
	})
	supercmd.Register(&addServerCommand{})

	return supercmd
}
