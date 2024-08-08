// Copyright 2024 Canonical.

package cmd

import (
	jujucmd "github.com/juju/cmd/v3"
)

var authDoc = `
auth enables users to manage authorisation model used by JIMM.
`

func NewAuthCommand() *jujucmd.SuperCommand {
	cmd := jujucmd.NewSuperCommand(jujucmd.SuperCommandParams{
		Name:    "auth",
		Doc:     authDoc,
		Purpose: "Authorisation model management.",
	})
	cmd.Register(NewGroupCommand())
	cmd.Register(NewRelationCommand())

	return cmd
}
