// Copyright 2025 Canonical.

package cmd

import (
	"context"

	"github.com/juju/cmd/v3"
	"github.com/juju/cmd/v3/cmdtesting"
	"github.com/juju/gnuflag"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/canonical/jimm/v3/cmd/jaas/cmd/mocks"
	"github.com/canonical/jimm/v3/pkg/api/params"
)

type listMigrationTargetsCmdSuite struct {
	client *mocks.MockJIMMAPI
	writer *mocks.MockWriter
}

var _ = gc.Suite(&listMigrationTargetsCmdSuite{})

func (s *listMigrationTargetsCmdSuite) SetupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.client = mocks.NewMockJIMMAPI(ctrl)
	s.writer = mocks.NewMockWriter(ctrl)

	return ctrl
}

func (s *listMigrationTargetsCmdSuite) TestListMigrationTargetsValidation(c *gc.C) {
	tests := []struct {
		about      string
		args       []string
		checkFlags func(*gc.C, *listMigrationTargetsCommand)
		errMatch   string
	}{
		{
			about: "valid model uuid",
			args:  []string{"e634de76-0414-49e5-8918-efd3a04ac493"},
			checkFlags: func(c *gc.C, command *listMigrationTargetsCommand) {
				c.Check(command.modelTag, gc.Equals, "model-e634de76-0414-49e5-8918-efd3a04ac493")
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
	for i, test := range tests {
		c.Logf("Test %d: %s", i, test.about)
		command := &listMigrationTargetsCommand{}
		command.SetClientStore(jujuclienttesting.MinimalStore())
		err := cmdtesting.InitCommand(command, test.args)
		if test.errMatch == "" {
			c.Check(err, gc.IsNil)
			test.checkFlags(c, command)
		} else {
			c.Check(err, gc.ErrorMatches, test.errMatch)
		}
	}
}

func (s *listMigrationTargetsCmdSuite) TestListMigrationTargets(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	s.client.EXPECT().ListMigrationTargets(gomock.Any()).Return(
		[]params.ControllerInfo{
			{
				Name:          "target-controller-1",
				UUID:          "fake-uuid",
				PublicAddress: "controller-address.com",
				CloudTag:      "cloud-mycloud",
			},
		},
		nil,
	)
	s.client.EXPECT().Close().Return(nil)

	s.writer.EXPECT().Write(gomock.Any()).DoAndReturn(func(b []byte) (int, error) {
		c.Check(string(b), gc.Equals, `- name: target-controller-1
  uuid: fake-uuid
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
		return len(b), nil
	})

	command := &listMigrationTargetsCommand{
		listMigrationTargetsAPIFunc: func() (JIMMAPI, error) {
			return s.client, nil
		},
	}
	f := gnuflag.NewFlagSet("test", gnuflag.ExitOnError)
	f.SetOutput(s.writer)
	command.SetFlags(f)

	ctx := &cmd.Context{
		Context: context.Background(),
		Stdout:  s.writer,
	}

	err := command.Run(ctx)
	c.Assert(err, gc.IsNil)
}
