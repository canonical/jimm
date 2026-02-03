// Copyright 2026 Canonical.

package cmd

import (
	"errors"
	"strings"
	"testing"

	qt "github.com/frankban/quicktest"
	"github.com/juju/cmd/v3/cmdtesting"

	apiparams "github.com/canonical/jimm/v3/pkg/api/params"
)

func runShowModelCommand(c *qt.C, mocks *cmdMocks, args ...string) (string, error) {
	showCmd := showModelCommand{
		client: mocks.client,
	}
	showCmd.SetClientStore(mocks.store)

	ctx := newTestContext(c)
	err := initCommandWithError(&showCmd, args...)
	if err != nil {
		return "", err
	}

	err = showCmd.Run(ctx)
	if err != nil {
		return "", err
	}

	return cmdtesting.Stdout(ctx), nil
}

func TestShowModelOutput(t *testing.T) {
	c := qt.New(t)

	modelControllerInfo := &apiparams.ModelControllerInfo{
		ModelName:      "test-model",
		ModelUUID:      "12345678-1234-1234-1234-123456789abc",
		ControllerName: "test-controller",
		ControllerUUID: "87654321-4321-4321-4321-cba987654321",
	}

	mocks := setupCmdMocks(c)
	mocks.store.EXPECT().CurrentController().Return("test-controller", nil).AnyTimes()
	mocks.store.EXPECT().ControllerByName("test-controller").Return(nil, errors.New("not found")).AnyTimes()
	mocks.client.EXPECT().ModelControllerInfo("12345678-1234-1234-1234-123456789abc").Return(modelControllerInfo, nil).AnyTimes()

	tests := []struct {
		args           []string
		expectedOutput string
	}{{
		args:           []string{"12345678-1234-1234-1234-123456789abc", "--format", "json"},
		expectedOutput: `{"model-name":"test-model","model-uuid":"12345678-1234-1234-1234-123456789abc","controller-name":"test-controller","controller-uuid":"87654321-4321-4321-4321-cba987654321"}`,
	}, {
		args: []string{"12345678-1234-1234-1234-123456789abc", "--format", "yaml"},
		expectedOutput: `model-name: test-model
model-uuid: 12345678-1234-1234-1234-123456789abc
controller-name: test-controller
controller-uuid: 87654321-4321-4321-4321-cba987654321`,
	}, {
		args: []string{"12345678-1234-1234-1234-123456789abc", "--format", "tabular"},
		expectedOutput: `Model name  Model UUID                            Controller name  Controller UUID
test-model  12345678-1234-1234-1234-123456789abc  test-controller  87654321-4321-4321-4321-cba987654321`,
	}}
	for _, test := range tests {
		output, err := runShowModelCommand(c, mocks, test.args...)
		c.Assert(err, qt.IsNil)
		output = strings.TrimRight(output, "\n")
		c.Assert(output, qt.Equals, test.expectedOutput)
	}
}

func TestShowModelError(t *testing.T) {
	c := qt.New(t)

	mocks := setupCmdMocks(c)
	mocks.store.EXPECT().CurrentController().Return("test-controller", nil).AnyTimes()
	mocks.store.EXPECT().ControllerByName("test-controller").Return(nil, errors.New("not found")).AnyTimes()
	mocks.client.EXPECT().ModelControllerInfo("12345678-1234-1234-1234-123456789abc").Return(nil, errors.New("not found")).AnyTimes()

	_, err := runShowModelCommand(c, mocks, "12345678-1234-1234-1234-123456789abc")
	c.Assert(err, qt.ErrorMatches, "not found")
}

func TestShowModelArgsError(t *testing.T) {
	c := qt.New(t)

	mocks := setupCmdMocks(c)
	mocks.store.EXPECT().CurrentController().Return("test-controller", nil).AnyTimes()
	mocks.store.EXPECT().ControllerByName("test-controller").Return(nil, errors.New("not found")).AnyTimes()
	mocks.client.EXPECT().ModelControllerInfo("12345678-1234-1234-1234-123456789abc").Return(nil, errors.New("not found")).AnyTimes()

	_, err := runShowModelCommand(c, mocks)
	c.Assert(err, qt.ErrorMatches, "missing model qualifier")

	_, err = runShowModelCommand(c, mocks, "12345678-1234-1234-1234-123456789abc", "extra-arg")
	c.Assert(err, qt.ErrorMatches, `unknown arguments: \[extra-arg\]`)
}
