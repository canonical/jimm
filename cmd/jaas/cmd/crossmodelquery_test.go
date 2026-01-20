// Copyright 2025 Canonical.

package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"

	qt "github.com/frankban/quicktest"

	apiparams "github.com/canonical/jimm/v3/pkg/api/params"
)

func TestCrossModelQueryRun(t *testing.T) {
	c := qt.New(t)
	s := setupCmdMocks(t)

	expectedReq := &apiparams.CrossModelQueryRequest{
		Type:  "jq",
		Query: ".applications",
	}
	expectedResp := &apiparams.CrossModelQueryResponse{
		Results: map[string][]any{
			"model-uuid": {
				map[string]any{"applications": map[string]any{"app": map[string]any{"status": "active"}}},
			},
		},
		Errors: map[string][]string{},
	}

	s.client.EXPECT().CrossModelQuery(expectedReq).Return(expectedResp, nil)
	s.client.EXPECT().Close().Return(nil)

	command := &crossModelQueryCommand{
		crossModelQueryAPIFunc: func() (JIMMAPI, error) {
			return s.client, nil
		},
	}

	initCommand(c, command, ".applications")

	ctx := newTestContext(t)
	err := command.Run(ctx)
	c.Assert(err, qt.IsNil)

	buf := ctx.Stdout.(*bytes.Buffer)
	var got apiparams.CrossModelQueryResponse
	jsonErr := json.Unmarshal(buf.Bytes(), &got)
	c.Assert(jsonErr, qt.IsNil)
	c.Assert(got.Results, qt.DeepEquals, expectedResp.Results)
	c.Assert(got.Errors, qt.DeepEquals, expectedResp.Errors)
}

func TestCrossModelQueryRunClientError(t *testing.T) {
	c := qt.New(t)

	command := &crossModelQueryCommand{
		crossModelQueryAPIFunc: func() (JIMMAPI, error) {
			return nil, errors.New("boom")
		},
	}

	initCommand(c, command, ".applications")

	ctx := newTestContext(t)
	err := command.Run(ctx)
	c.Assert(err, qt.ErrorMatches, "could not create JIMM client: boom")
}
