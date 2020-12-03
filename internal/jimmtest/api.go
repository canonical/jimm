// Copyright 2020 Canonical Ltd.

package jimmtest

import (
	"context"
	"sync/atomic"

	jujuparams "github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/version"

	"github.com/CanonicalLtd/jimm/internal/dbmodel"
	"github.com/CanonicalLtd/jimm/internal/errors"
	"github.com/CanonicalLtd/jimm/internal/jimm"
)

// DefaultControllerUUID is the controller UUID returned by Dialer if
// the is no configured controller UUID.
const DefaultControllerUUID = "982b16d9-a945-4762-b684-fd4fd885aa10"

// A Dialer is a jimm.Dialer that either returns an error if Err is
// non-zero, or returns the value of API. The number of open API
// connections is tracked.
type Dialer struct {
	// API contains the API implementation to return, if Err is nil.
	API jimm.API

	// Err contains the error to return when a Dial is attempted.
	Err error

	// UUID is the UUID of the connected controller, if this is not set
	// then DefaultControllerUUID will be used.
	UUID string

	// AgentVersion contains the juju-agent version to the report to the
	// controller connection. If this is empty the version of the linked
	// juju is used.
	AgentVersion string

	// HostPorts contains the HostPorts to set on the controller.
	HostPorts dbmodel.HostPorts

	open int64
}

// Dialer implements jimm.Dialer.
func (d *Dialer) Dial(_ context.Context, ctl *dbmodel.Controller, _ string) (jimm.API, error) {
	if d.Err != nil {
		return nil, d.Err
	}
	atomic.AddInt64(&d.open, 1)
	ctl.UUID = d.UUID
	if ctl.UUID == "" {
		ctl.UUID = DefaultControllerUUID
	}
	ctl.AgentVersion = d.AgentVersion
	if ctl.AgentVersion == "" {
		ctl.AgentVersion = version.Current.String()
	}
	ctl.HostPorts = d.HostPorts
	return apiWrapper{
		API:  d.API,
		open: &d.open,
	}, nil
}

// IsClosed returns true if all opened connections have been closed.
func (d *Dialer) IsClosed() bool {
	return atomic.LoadInt64(&d.open) == 0
}

// apiWrapper is the API implementation used by Dialer to track usage.
type apiWrapper struct {
	jimm.API
	open *int64
}

// Close closes the API and decrements the open count.
func (w apiWrapper) Close() {
	atomic.AddInt64(w.open, -1)
	w.API.Close()
}

// API is a default implementation of the jimm.API interface. Every method
// has a corresponding function field. Whenever the method is called it
// will delegate to the requested function or if the function is nil return
// a NotImplemented error.
type API struct {
	Close_                  func()
	CloudInfo_              func(context.Context, string, *jujuparams.CloudInfo) error
	Clouds_                 func(context.Context) (map[string]jujuparams.Cloud, error)
	ControllerModelSummary_ func(context.Context, *jujuparams.ModelSummary) error
}

func (a *API) Close() {
	if a.Close_ == nil {
		return
	}
	a.Close_()
}

func (a *API) CloudInfo(ctx context.Context, tag string, ci *jujuparams.CloudInfo) error {
	if a.CloudInfo_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return a.CloudInfo_(ctx, tag, ci)
}

func (a *API) Clouds(ctx context.Context) (map[string]jujuparams.Cloud, error) {
	if a.Clouds_ == nil {
		return nil, errors.E(errors.CodeNotImplemented)
	}
	return a.Clouds_(ctx)
}

func (a *API) ControllerModelSummary(ctx context.Context, ms *jujuparams.ModelSummary) error {
	if a.ControllerModelSummary_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return a.ControllerModelSummary_(ctx, ms)
}

var _ jimm.API = &API{}
