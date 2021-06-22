package debugapi_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"

	"github.com/CanonicalLtd/jimm/internal/debugapi"
	"github.com/CanonicalLtd/jimm/internal/errors"
	"github.com/CanonicalLtd/jimm/version"
)

func TestDebugInfo(t *testing.T) {
	c := qt.New(t)

	hnd := debugapi.Handler(context.Background(), nil)
	rr := httptest.NewRecorder()
	req, err := http.NewRequest("GET", "/debug/info", nil)
	c.Assert(err, qt.IsNil)
	hnd.ServeHTTP(rr, req)
	resp := rr.Result()
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

	hnd := debugapi.Handler(ctx, map[string]debugapi.StatusCheck{
		"start_time": debugapi.ServerStartTime,
	})
	rr := httptest.NewRecorder()
	req, err := http.NewRequest("GET", "/debug/status", nil)
	c.Assert(err, qt.IsNil)
	hnd.ServeHTTP(rr, req)
	resp := rr.Result()
	c.Check(resp.StatusCode, qt.Equals, http.StatusOK)
	buf, err := io.ReadAll(resp.Body)
	c.Assert(err, qt.IsNil)

	var v map[string]map[string]interface{}
	err = json.Unmarshal(buf, &v)
	c.Check(v["start_time"]["Name"], qt.Equals, debugapi.ServerStartTime.Name())
	c.Check(v["start_time"]["Value"], qt.Equals, startTime.(time.Time).Format(time.RFC3339Nano))
	c.Check(v["start_time"]["Passed"], qt.Equals, true)
}

func TestDebugStatusStatusError(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	hnd := debugapi.Handler(ctx, map[string]debugapi.StatusCheck{
		"test": debugapi.MakeStatusCheck("Test", func(context.Context) (interface{}, error) {
			return nil, errors.E("test error")
		}),
	})
	rr := httptest.NewRecorder()
	req, err := http.NewRequest("GET", "/debug/status", nil)
	c.Assert(err, qt.IsNil)
	hnd.ServeHTTP(rr, req)
	resp := rr.Result()
	c.Check(resp.StatusCode, qt.Equals, http.StatusOK)
	buf, err := io.ReadAll(resp.Body)
	c.Assert(err, qt.IsNil)

	var v map[string]map[string]interface{}
	err = json.Unmarshal(buf, &v)
	c.Check(v["test"]["Name"], qt.Equals, "Test")
	c.Check(v["test"]["Value"], qt.Equals, "test error")
	c.Check(v["test"]["Passed"], qt.Equals, false)
}
