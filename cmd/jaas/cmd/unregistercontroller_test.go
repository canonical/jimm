// Copyright 2025 Canonical.

package cmd

import (
	"testing"

	qt "github.com/frankban/quicktest"
	"github.com/juju/cmd/v3/cmdtesting"
	"github.com/juju/juju/rpc/params"
	"gopkg.in/yaml.v3"

	apiparams "github.com/canonical/jimm/v3/pkg/api/params"
)

func TestUnregisterControllerSuperuser(t *testing.T) {
	c := qt.New(t)
	cmdMocks := setupCmdMocks(c)

	fakeCtrl := apiparams.ControllerInfo{
		Name:          "mycontroller",
		UUID:          "some-uuid",
		PublicAddress: "public-addr",
		APIAddresses:  []string{"ep-1", "ep-2"},
		CACertificate: "ca-cert",
		CloudTag:      "cloud-openstack",
		CloudRegion:   "private-region",
		AgentVersion:  "v1.0.0",
		Status: params.EntityStatus{
			Data: map[string]any{},
		},
	}

	cmdMocks.client.EXPECT().RemoveController(&apiparams.RemoveControllerRequest{
		Name:  "mycontroller",
		Force: true,
	}).Return(fakeCtrl, nil)
	cmdMocks.client.EXPECT().Close().Return(nil)

	command := &unregisterControllerCommand{
		jimmAPIFunc: func() (JIMMAPI, error) {
			return cmdMocks.client, nil
		},
	}
	command.SetClientStore(cmdMocks.store)

	initCommand(c, command, "mycontroller", "--force")

	ctx := newTestContext(c)

	err := command.Run(ctx)
	c.Assert(err, qt.IsNil)

	// Unmarshal to verify output.
	var info apiparams.ControllerInfo
	err = yaml.Unmarshal([]byte(cmdtesting.Stdout(ctx)), &info)
	c.Assert(err, qt.IsNil)

	c.Assert(info, qt.DeepEquals, fakeCtrl)
}
