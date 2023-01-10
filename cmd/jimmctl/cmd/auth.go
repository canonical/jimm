// Copyright 2023 Canonical Ltd.

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

	return cmd
}
