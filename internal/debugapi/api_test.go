// Copyright 2024 Canonical.
package debugapi_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
	"github.com/go-chi/chi/v5"

	"github.com/canonical/jimm/v3/internal/debugapi"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/version"
)

func setupHandlerAndRecorder(c *qt.C, startTime debugapi.StatusCheck, path string) *httptest.ResponseRecorder {
	r := (&debugapi.DebugHandler{
		Router: chi.NewRouter(),
		StatusChecks: map[string]debugapi.StatusCheck{
			"start_time": startTime,
		},
	}).Routes()

	rr := httptest.NewRecorder()
	req, err := http.NewRequest("GET", path, nil)
	c.Assert(err, qt.IsNil)
	r.ServeHTTP(rr, req)
	return rr
}

func TestDebugInfo(t *testing.T) {
	c := qt.New(t)

	rr := setupHandlerAndRecorder(c, debugapi.ServerStartTime, "/info")

	resp := rr.Result()
	defer resp.Body.Close()
	c.Check(resp.StatusCode, qt.Equals, http.StatusOK)
	buf, err := io.ReadAll(resp.Body)
	c.Assert(err, qt.IsNil)
	c.Check(buf, qt.JSONEquals, version.VersionInfo)
}

func TestDebugStatus(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()
	startTime, err := debugapi.ServerStartTime.Check(ctx)
	c.Assert(err, qt.IsNil)

	rr := setupHandlerAndRecorder(c, debugapi.ServerStartTime, "/status")

	resp := rr.Result()
	defer resp.Body.Close()
	c.Check(resp.StatusCode, qt.Equals, http.StatusOK)
	buf, err := io.ReadAll(resp.Body)
	c.Assert(err, qt.IsNil)

	fmt.Println(string(buf))
	var v map[string]map[string]interface{}
	err = json.Unmarshal(buf, &v)
	c.Assert(err, qt.IsNil)

	c.Check(v["start_time"]["Name"], qt.Equals, debugapi.ServerStartTime.Name())
	c.Check(v["start_time"]["Value"], qt.Equals, startTime.(time.Time).Format(time.RFC3339Nano))
	c.Check(v["start_time"]["Passed"], qt.Equals, true)
}

func TestDebugStatusStatusError(t *testing.T) {
	c := qt.New(t)

	rr := setupHandlerAndRecorder(c, debugapi.MakeStatusCheck("Test", func(context.Context) (interface{}, error) {
		return nil, errors.E("test error")
	}), "/status")

	resp := rr.Result()
	defer resp.Body.Close()
	c.Check(resp.StatusCode, qt.Equals, http.StatusOK)
	buf, err := io.ReadAll(resp.Body)
	c.Assert(err, qt.IsNil)

	var v map[string]map[string]interface{}
	err = json.Unmarshal(buf, &v)
	c.Assert(err, qt.IsNil)
	c.Check(v["start_time"]["Name"], qt.Equals, "Test")
	c.Check(v["start_time"]["Value"], qt.Equals, "test error")
	c.Check(v["start_time"]["Passed"], qt.Equals, false)
}
