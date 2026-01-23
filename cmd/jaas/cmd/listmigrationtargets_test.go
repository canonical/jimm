// Copyright 2025 Canonical.

package cmd

import (
	"bytes"
	"testing"

	qt "github.com/frankban/quicktest"
	"github.com/juju/cmd/v3/cmdtesting"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
	"go.uber.org/mock/gomock"

	"github.com/canonical/jimm/v3/pkg/api/params"
)

func TestListMigrationTargetsValidation(t *testing.T) {
	tests := []struct {
		about      string
		args       []string
		checkFlags func(*qt.C, *listMigrationTargetsCommand)
		errMatch   string
	}{
		{
			about: "valid model uuid",
			args:  []string{"e634de76-0414-49e5-8918-efd3a04ac493"},
			checkFlags: func(c *qt.C, command *listMigrationTargetsCommand) {
				c.Check(command.modelTag, qt.Equals, "model-e634de76-0414-49e5-8918-efd3a04ac493")
			},
		},
		{
			about:    "invalid uuid",
			args:     []string{"foo"},
			errMatch: `invalid model uuid "foo"`,
		}, {
			about:    "too many args",
			args:     []string{"e634de76-0414-49e5-8918-efd3a04ac493", "extra-arg"},
			errMatch: "expected model uuid argument",
		},
	}
	for _, test := range tests {
		t.Run(test.about, func(t *testing.T) {
			c := qt.New(t)
			command := &listMigrationTargetsCommand{}
			command.SetClientStore(jujuclienttesting.MinimalStore())
			err := cmdtesting.InitCommand(command, test.args)
			if test.errMatch == "" {
				c.Check(err, qt.IsNil)
				test.checkFlags(c, command)
			} else {
				c.Check(err, qt.ErrorMatches, test.errMatch)
			}
		})
	}
}

func TestListMigrationTargets(t *testing.T) {
	c := qt.New(t)
	s := setupCmdMocks(t)

	s.client.EXPECT().ListMigrationTargets(gomock.Any()).DoAndReturn(func(lmtr *params.ListMigrationTargetsRequest) ([]params.ControllerInfo, error) {
		c.Check(lmtr.ModelTag, qt.Equals, "model-e14aff09-e951-413b-833d-60b1a27bd604")
		return []params.ControllerInfo{
			{
				Name:          "target-controller-1",
				UUID:          "e14aff09-e951-413b-833d-60b1a27bd604",
				PublicAddress: "controller-address.com",
				CloudTag:      "cloud-mycloud",
			},
		}, nil
	})
	s.client.EXPECT().Close().Return(nil)

	command := &listMigrationTargetsCommand{
		listMigrationTargetsAPIFunc: func() (JIMMAPI, error) {
			return s.client, nil
		},
	}

	initCommand(c, command, "e14aff09-e951-413b-833d-60b1a27bd604")

	ctx := newTestContext(t)
	err := command.Run(ctx)
	c.Assert(err, qt.IsNil)

	res := ctx.Stdout.(*bytes.Buffer).String()
	c.Assert(res, qt.Equals, `- name: target-controller-1
  uuid: e14aff09-e951-413b-833d-60b1a27bd604
  publicaddress: controller-address.com
  apiaddresses: []
  cacertificate: ""
  cloudtag: cloud-mycloud
  cloudregion: ""
  agentversion: ""
  status:
    status: ""
    info: ""
    data: {}
    since: null
`)
}
