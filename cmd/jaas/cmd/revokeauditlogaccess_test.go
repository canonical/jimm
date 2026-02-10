// Copyright 2025 Canonical.

package cmd

import (
	"fmt"
	"testing"

	qt "github.com/frankban/quicktest"
	"github.com/juju/cmd/v3/cmdtesting"
	"github.com/juju/names/v5"

	apiparams "github.com/canonical/jimm/v3/pkg/api/params"
)

func runRevokeAuditLogAccessCommand(c *qt.C, mocks *cmdMocks, args ...string) (string, error) {
	revokeAuditLogAccessCmd := revokeAuditLogAccessCommand{
		client: mocks.client,
	}
	revokeAuditLogAccessCmd.SetClientStore(mocks.store)
	ctx := newTestContext(c)
	err := initCommandWithError(&revokeAuditLogAccessCmd, args...)
	if err != nil {
		return "", err
	}

	err = revokeAuditLogAccessCmd.Run(ctx)
	if err != nil {
		return "", err
	}

	return cmdtesting.Stdout(ctx), nil
}

func TestRevokeAuditLogAccess(t *testing.T) {
	c := qt.New(t)

	mocks := setupCmdMocks(c)
	mocks.client.EXPECT().RevokeAuditLogAccess(&apiparams.AuditLogAccessRequest{
		UserTag: names.NewUserTag("alice@canonical.com").String(),
	}).Return(nil)

	_, err := runRevokeAuditLogAccessCommand(c, mocks, "alice@canonical.com")
	c.Assert(err, qt.IsNil)
}

func TestRevokeAuditLogAccessError(t *testing.T) {
	c := qt.New(t)

	mocks := setupCmdMocks(c)
	mocks.client.EXPECT().RevokeAuditLogAccess(&apiparams.AuditLogAccessRequest{
		UserTag: names.NewUserTag("alice@canonical.com").String(),
	}).Return(fmt.Errorf("unauthorized"))

	_, err := runRevokeAuditLogAccessCommand(c, mocks, "alice@canonical.com")
	c.Assert(err, qt.ErrorMatches, "unauthorized")
}
