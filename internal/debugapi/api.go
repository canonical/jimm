package debugapi

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/render"

	"github.com/CanonicalLtd/jimm/version"
)

// DebugHandler
type DebugHandler struct {
	Router       *chi.Mux
	StatusChecks map[string]StatusCheck
}

// Routes
func (dh *DebugHandler) Routes() chi.Router {
	dh.SetupMiddleware()
	dh.Router.Get("/info", dh.Info)
	dh.Router.Get("/status", dh.Status)
	return dh.Router
}

// SetupMiddleware
func (dh *DebugHandler) SetupMiddleware() {
	dh.Router.Use(
		render.SetContentType(
			render.ContentTypeJSON,
		),
	)
}

// Info handles /info
func (dh *DebugHandler) Info(w http.ResponseWriter, r *http.Request) {
	render.JSON(w, r, version.VersionInfo)
}

// Status handles /status
func (dh *DebugHandler) Status(w http.ResponseWriter, r *http.Request) {
	checks := dh.StatusChecks
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
			v, err := check.Check(r.Context())
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
	render.JSON(w, r, results)
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
