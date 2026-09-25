// Copyright 2026 Canonical.

package jujuapi_test

import (
	"context"
	"testing"

	qt "github.com/frankban/quicktest"
	jujuparams "github.com/juju/juju/rpc/params"

	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/jujuapi"
	"github.com/canonical/jimm/v3/internal/openfga"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest/mocks"
)

func TestModelConfigSchema_Success(t *testing.T) {
	c := qt.New(t)

	ctx := c.Context()
	schema := map[string]jujuparams.ModelConfigSchemaField{
		"name": {
			Description: "The name of the model.",
			Type:        "string",
			Mandatory:   true,
		},
	}

	called := false
	jimm := &jimmtest.JIMM{
		JujuManager_: func() jujuapi.JujuManager {
			return &mocks.JujuManager{
				ModelConfigSchema_: func(_ context.Context, u *openfga.User, providerType string) (map[string]jujuparams.ModelConfigSchemaField, error) {
					called = true
					c.Assert(u.Name, qt.Equals, "alice@canonical.com")
					c.Assert(providerType, qt.Equals, "ec2")
					return schema, nil
				},
			}
		},
	}
	root := newTestControllerRoot(jimm, "alice@canonical.com", false)

	result, err := root.ModelConfigSchema(ctx, jujuparams.ModelConfigSchemaArgs{ProviderType: "ec2"})
	c.Assert(err, qt.IsNil)
	c.Assert(called, qt.IsTrue)
	c.Assert(result.Error == nil, qt.IsTrue)
	c.Assert(result.Schema, qt.DeepEquals, schema)
}

func TestModelConfigSchema_Error(t *testing.T) {
	c := qt.New(t)

	ctx := c.Context()
	jimm := &jimmtest.JIMM{
		JujuManager_: func() jujuapi.JujuManager {
			return &mocks.JujuManager{
				ModelConfigSchema_: func(_ context.Context, _ *openfga.User, _ string) (map[string]jujuparams.ModelConfigSchemaField, error) {
					return nil, errors.Codef(errors.CodeNotFound, "no such provider")
				},
			}
		},
	}
	root := newTestControllerRoot(jimm, "alice@canonical.com", false)

	result, err := root.ModelConfigSchema(ctx, jujuparams.ModelConfigSchemaArgs{ProviderType: "no-dice"})
	// Per-request errors are returned in the result, not as a top-level error.
	c.Assert(err, qt.IsNil)
	c.Assert(result.Schema, qt.IsNil)
	c.Assert(result.Error != nil, qt.IsTrue)
	c.Assert(result.Error.Code, qt.Equals, string(errors.CodeNotFound))
}
