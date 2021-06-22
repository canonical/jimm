package debugapi

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"

	"github.com/CanonicalLtd/jimm/version"
)

// Handler returns an http.Handler to handle requests for /debug endpoints.
func Handler(ctx context.Context, statusChecks map[string]StatusCheck) http.Handler {
	mux := http.NewServeMux()

	// data for /debug/info
	buf, err := json.Marshal(version.VersionInfo)
	if err == nil {
		mux.HandleFunc("/debug/info", func(w http.ResponseWriter, _ *http.Request) {
			w.Write(buf)
		})
	} else {
		// This should be impossible.
		zapctx.Error(ctx, "cannot marshal version", zap.Error(err))
	}

	// /debug/status
	mux.HandleFunc("/debug/status", statusHandler(statusChecks))

	return mux
}

// statusHandler returns a http.HandlerFunc the performs the given status
// checks.
func statusHandler(checks map[string]StatusCheck) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		var mu sync.Mutex
		results := make(map[string]statusResult, len(checks))
		var wg sync.WaitGroup
		wg.Add(len(checks))
		for k, check := range checks {
			k, check := k, check
			go func() {
				defer wg.Done()
				result := statusResult{
					Name: check.Name(),
				}
				start := time.Now()
				v, err := check.Check(req.Context())
				result.Duration = time.Since(start)
				if err == nil {
					result.Passed = true
					result.Value = v
				} else {
					result.Value = err.Error()
				}
				mu.Lock()
				defer mu.Unlock()
				results[k] = result
			}()
		}
		wg.Wait()
		buf, err := json.Marshal(results)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Write(buf)
	}
}

// A statusResult is the type that represents the result of a status check
// in the /debug/status response body.
type statusResult struct {
	Name     string
	Value    interface{}
	Passed   bool
	Duration time.Duration
}

// A StatusCheck is a chack that is performed as part of the /debug/status endpoint
type StatusCheck interface {
	// Name is a human-readable name for the status check.
	Name() string

	// Check runs the actual check.
	Check(ctx context.Context) (interface{}, error)
}

// MakeStatusCheck creates a status check with the given human readable
// name which runs the given function.
func MakeStatusCheck(name string, f func(context.Context) (interface{}, error)) StatusCheck {
	return statusCheck{
		name: name,
		f:    f,
	}
}

// A statusCheck is the implementation of statusCheck returned from
// MakeStatusCheck.
type statusCheck struct {
	name string
	f    func(context.Context) (interface{}, error)
}

// Name implements StatusCheck.Name.
func (c statusCheck) Name() string {
	return c.name
}

// Check implements StatusCheck.Check.
func (c statusCheck) Check(ctx context.Context) (interface{}, error) {
	return c.f(ctx)
}

var startTime = time.Now().UTC()

// ServerStartTime is a StatusCheck that returns the server start time.
var ServerStartTime = MakeStatusCheck("server start time", func(_ context.Context) (interface{}, error) {
	return startTime, nil
})
