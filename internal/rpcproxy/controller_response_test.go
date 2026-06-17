// Copyright 2025 Canonical.

package rpcproxy

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"maps"
	"testing"

	qt "github.com/frankban/quicktest"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v5"

	"github.com/canonical/jimm/v3/internal/openfga"
)

func TestCheckPermissionsRequiredTopLevelError(t *testing.T) {
	c := qt.New(t)

	msg := &message{
		ErrorCode: accessRequiredErrorCode,
		ErrorInfo: map[string]any{
			"model-test": "write",
		},
	}

	permissions, err := checkPermissionsRequired(context.Background(), msg)
	c.Assert(err, qt.IsNil)
	c.Assert(permissions, qt.DeepEquals, msg.ErrorInfo)
}

func TestCheckPermissionsRequiredBulkErrors(t *testing.T) {
	c := qt.New(t)

	response, err := json.Marshal(jujuparams.ErrorResults{
		Results: []jujuparams.ErrorResult{{
			Error: &jujuparams.Error{
				Code: accessRequiredErrorCode,
				Info: map[string]any{
					"model-test": "write",
				},
			},
		}, {
			Error: &jujuparams.Error{
				Code: accessRequiredErrorCode,
				Info: map[string]any{
					"controller-test": "superuser",
				},
			},
		}},
	})
	c.Assert(err, qt.IsNil)

	permissions, err := checkPermissionsRequired(context.Background(), &message{Response: response})
	c.Assert(err, qt.IsNil)
	c.Assert(permissions, qt.DeepEquals, map[string]any{
		"model-test":      "write",
		"controller-test": "superuser",
	})
}

func TestControllerResponseHandlerRedoLoginReplaysOriginalRequest(t *testing.T) {
	c := qt.New(t)

	loginParams, err := json.Marshal(jujuparams.LoginRequest{
		AuthTag: names.NewUserTag("alice@example.com").String(),
		Token:   "old-token",
	})
	c.Assert(err, qt.IsNil)

	loginMsg := &message{
		RequestID: 1,
		Type:      "Admin",
		Version:   3,
		Request:   "Login",
		Params:    loginParams,
	}
	originalMsg := &message{
		RequestID: 2,
		Type:      "Client",
		Version:   7,
		Request:   "DoThing",
		Params:    json.RawMessage(`{"key":"value"}`),
	}
	writes := &captureWebsocketConnection{}
	tokenGen := &recordingTokenGenerator{}
	tracker := newInflightTracker()
	tracker.rememberLogin(loginMsg)
	tracker.track(originalMsg)
	proxy := &controllerProxy{modelProxy: modelProxy{
		src:      &writeLockConn{conn: writes},
		inflight: tracker,
		tokenGen: tokenGen,
	}}
	handler := newControllerResponseHandler(proxy)

	shouldForward := handler.processControllerErrors(context.Background(), &message{
		RequestID: 2,
		ErrorCode: accessRequiredErrorCode,
		ErrorInfo: map[string]any{
			"model-test": "write",
		},
	})

	c.Assert(shouldForward, qt.IsFalse)
	c.Assert(tokenGen.permissionMaps, qt.HasLen, 1)
	c.Assert(tokenGen.permissionMaps[0], qt.DeepEquals, map[string]any{"model-test": "write"})
	c.Assert(writes.writes, qt.HasLen, 2)

	var wroteLogin message
	err = json.Unmarshal(writes.writes[0], &wroteLogin)
	c.Assert(err, qt.IsNil)
	c.Assert(wroteLogin.RequestID, qt.Equals, uint64(1))
	c.Assert(wroteLogin.Type, qt.Equals, "Admin")
	c.Assert(wroteLogin.Request, qt.Equals, "Login")

	var retriedLogin jujuparams.LoginRequest
	err = json.Unmarshal(wroteLogin.Params, &retriedLogin)
	c.Assert(err, qt.IsNil)
	c.Assert(retriedLogin.Token, qt.Equals, base64.StdEncoding.EncodeToString([]byte("test token")))

	var wroteOriginal message
	err = json.Unmarshal(writes.writes[1], &wroteOriginal)
	c.Assert(err, qt.IsNil)
	c.Assert(wroteOriginal.RequestID, qt.Equals, uint64(2))
	c.Assert(wroteOriginal.Type, qt.Equals, "Client")
	c.Assert(wroteOriginal.Request, qt.Equals, "DoThing")
	var retriedParams map[string]any
	err = json.Unmarshal(wroteOriginal.Params, &retriedParams)
	c.Assert(err, qt.IsNil)
	c.Assert(retriedParams, qt.DeepEquals, map[string]any{"key": "value"})
}

func TestModifyControllerResponseStripsServers(t *testing.T) {
	c := qt.New(t)

	msg := &message{Response: json.RawMessage(`{"results":[],"servers":[{"value":"controller-1"}]}`)}
	err := modifyControllerResponse(msg)
	c.Assert(err, qt.IsNil)
	var response map[string]any
	err = json.Unmarshal(msg.Response, &response)
	c.Assert(err, qt.IsNil)
	c.Assert(response, qt.DeepEquals, map[string]any{"results": []any{}})
}

type captureWebsocketConnection struct {
	writes [][]byte
}

func (c *captureWebsocketConnection) ReadJSON(any) error {
	return fmt.Errorf("unexpected ReadJSON call")
}

func (c *captureWebsocketConnection) WriteJSON(v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	c.writes = append(c.writes, data)
	return nil
}

func (c *captureWebsocketConnection) Close() error {
	return nil
}

type recordingTokenGenerator struct {
	permissionMaps []map[string]any
}

func (r *recordingTokenGenerator) MakeLoginToken(context.Context, *openfga.User) ([]byte, error) {
	return []byte("test token"), nil
}

func (r *recordingTokenGenerator) MakeToken(_ context.Context, permissionMap map[string]any) ([]byte, error) {
	copyMap := make(map[string]any, len(permissionMap))
	maps.Copy(copyMap, permissionMap)
	r.permissionMaps = append(r.permissionMaps, copyMap)
	return []byte("test token"), nil
}

func (r *recordingTokenGenerator) SetTags(names.ModelTag, names.ControllerTag) {
}

func (r *recordingTokenGenerator) GetUser() names.UserTag {
	return names.NewUserTag("test-user")
}
