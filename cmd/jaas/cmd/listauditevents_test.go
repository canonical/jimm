// Copyright 2025 Canonical.

package cmd

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
	"github.com/juju/cmd/v3/cmdtesting"
	"github.com/juju/gnuflag"
	"go.uber.org/mock/gomock"

	apiparams "github.com/canonical/jimm/v3/pkg/api/params"
)

func TestListAuditEventsRun_Success(t *testing.T) {
	c := qt.New(t)

	cmdMocks := setupCmdMocks(t)

	expected := apiparams.AuditEvents{Events: []apiparams.AuditEvent{{MessageId: 1}}}

	cmdMocks.client.EXPECT().
		FindAuditEvents(gomock.Any()).
		DoAndReturn(func(req *apiparams.FindAuditEventsRequest) (apiparams.AuditEvents, error) {
			c.Check(req, qt.Not(qt.IsNil))
			return expected, nil
		}).
		Times(1)

	cmdMocks.client.EXPECT().Close()

	command := &listAuditEventsCommand{
		jimmAPIFunc: func() (JIMMAPI, error) {
			return cmdMocks.client, nil
		},
	}

	fs := gnuflag.NewFlagSet("test", gnuflag.ContinueOnError)
	command.SetFlags(fs)

	ctx := newTestContext(t)
	err := command.Run(ctx)
	c.Assert(err, qt.IsNil)

	stdout := ctx.Stdout.(*bytes.Buffer).String()
	c.Assert(stdout, qt.Contains, "events")
}

func TestListAuditEventsRun_APICallFails(t *testing.T) {
	c := qt.New(t)

	cmdMocks := setupCmdMocks(t)

	cmdMocks.client.EXPECT().
		FindAuditEvents(gomock.Any()).
		Return(apiparams.AuditEvents{}, errors.New("nope")).
		Times(1)

	cmdMocks.client.EXPECT().Close()

	command := &listAuditEventsCommand{
		jimmAPIFunc: func() (JIMMAPI, error) {
			return cmdMocks.client, nil
		},
	}

	fs := gnuflag.NewFlagSet("test", gnuflag.ContinueOnError)
	command.SetFlags(fs)

	err := command.Run(newTestContext(t))
	c.Assert(err, qt.ErrorMatches, ".*nope.*")
}

func TestListAuditEventsInit_RejectsArgs(t *testing.T) {
	c := qt.New(t)

	command := listAuditEventsCommand{}
	err := command.Init([]string{"unexpected"})
	c.Assert(err, qt.ErrorMatches, "unknown arguments")
}

func TestListAuditEventsRun_FlagsArePassedToAPICorrectly(t *testing.T) {
	c := qt.New(t)

	cmdMocks := setupCmdMocks(t)

	cmdMocks.client.EXPECT().
		FindAuditEvents(gomock.Any()).
		DoAndReturn(func(req *apiparams.FindAuditEventsRequest) (apiparams.AuditEvents, error) {
			c.Check(req.After, qt.Equals, "2020-01-01T15:00:00Z")
			c.Check(req.Before, qt.Equals, "2020-01-02T15:00:00Z")
			c.Check(req.UserTag, qt.Equals, "user-alice@canonical.com")
			c.Check(req.Method, qt.Equals, "CreateModel")
			c.Check(req.Model, qt.Equals, "controller/model")
			c.Check(req.Offset, qt.Equals, 7)
			c.Check(req.Limit, qt.Equals, 50)
			c.Check(req.SortTime, qt.Equals, true)
			return apiparams.AuditEvents{Events: []apiparams.AuditEvent{}}, nil
		}).
		Times(1)

	cmdMocks.client.EXPECT().Close()

	command := &listAuditEventsCommand{
		jimmAPIFunc: func() (JIMMAPI, error) {
			return cmdMocks.client, nil
		},
	}

	fs := gnuflag.NewFlagSet("test", gnuflag.ContinueOnError)
	command.SetFlags(fs)

	err := cmdtesting.InitCommand(command, []string{
		"--after=2020-01-01T15:00:00Z",
		"--before=2020-01-02T15:00:00Z",
		"--user-tag=user-alice@canonical.com",
		"--method=CreateModel",
		"--model=controller/model",
		"--offset=7",
		"--limit=50",
		"--reverse=true",
	})
	c.Assert(err, qt.IsNil)

	err = command.Run(newTestContext(t))
	c.Assert(err, qt.IsNil)
}

func TestListAuditEventsRun_TabularFormat(t *testing.T) {
	c := qt.New(t)

	cmdMocks := setupCmdMocks(t)

	ts := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	cmdMocks.client.EXPECT().
		FindAuditEvents(gomock.Any()).
		Return(
			apiparams.AuditEvents{Events: []apiparams.AuditEvent{{
				Time:           ts,
				UserTag:        "user-alice@canonical.com",
				Model:          "controller/model",
				ConversationId: "conv-1",
				MessageId:      7,
				FacadeMethod:   "CreateModel",
				IsResponse:     false,
			}}},
			nil,
		).
		Times(1)

	cmdMocks.client.EXPECT().Close()

	command := &listAuditEventsCommand{
		jimmAPIFunc: func() (JIMMAPI, error) {
			return cmdMocks.client, nil
		},
	}

	fs := gnuflag.NewFlagSet("test", gnuflag.ContinueOnError)
	command.SetFlags(fs)

	err := cmdtesting.InitCommand(command, []string{
		"--format=tabular",
	})
	c.Assert(err, qt.IsNil)

	ctx := newTestContext(t)
	err = command.Run(ctx)
	c.Assert(err, qt.IsNil)

	out := strings.TrimRight(ctx.Stdout.(*bytes.Buffer).String(), " ") + "\n"

	expected := "Time                         \tUser                    \tModel           \tConversationId\tMessageId\tMethod     \tIsResponse\tParams\tErrors\n" +
		"2026-01-01 00:00:00 +0000 UTC\tuser-alice@canonical.com\tcontroller/model\tconv-1        \t7        \tCreateModel\tfalse     \tnull  \tnull\n"

	c.Assert(out, qt.Equals, expected)
}
