// Copyright 2016 Canonical Ltd.

package monitor

import (
	"time"

	apicontroller "github.com/juju/juju/api/controller"
	"github.com/juju/juju/state/multiwatcher"
	"github.com/juju/version"
	"golang.org/x/net/context"
	"gopkg.in/errgo.v1"

	"github.com/CanonicalLtd/jem/internal/apiconn"
	"github.com/CanonicalLtd/jem/internal/jem"
	"github.com/CanonicalLtd/jem/internal/mongodoc"
	"github.com/CanonicalLtd/jem/params"
)

// jemShim implements the jemInterface interface
// by using a *jem.Database directly.
type jemShim struct {
	*jem.JEM
}

func (j jemShim) Clone() jemInterface {
	return jemShim{j.JEM.Clone()}
}

func (j jemShim) OpenAPI(ctx context.Context, path params.EntityPath) (jujuAPI, error) {
	conn, err := j.JEM.OpenAPI(ctx, path)
	if err != nil {
		return nil, errgo.Mask(err, errgo.Any)
	}
	return apiShim{conn}, nil
}

func (j jemShim) AllControllers(ctx context.Context) ([]*mongodoc.Controller, error) {
	var ctls []*mongodoc.Controller
	err := j.DB.Controllers().Find(nil).All(&ctls)
	if err != nil {
		return nil, errgo.Mask(err)
	}
	return ctls, nil
}

func (j jemShim) SetControllerStats(ctx context.Context, ctlPath params.EntityPath, stats *mongodoc.ControllerStats) error {
	return errgo.Mask(j.DB.SetControllerStats(ctx, ctlPath, stats), errgo.Any)
}

func (j jemShim) SetControllerUnavailableAt(ctx context.Context, ctlPath params.EntityPath, t time.Time) error {
	return errgo.Mask(j.DB.SetControllerUnavailableAt(ctx, ctlPath, t), errgo.Any)
}

func (j jemShim) SetControllerAvailable(ctx context.Context, ctlPath params.EntityPath) error {
	return errgo.Mask(j.DB.SetControllerAvailable(ctx, ctlPath), errgo.Any)
}

func (j jemShim) SetModelLife(ctx context.Context, ctlPath params.EntityPath, uuid string, life string) error {
	return errgo.Mask(j.DB.SetModelLife(ctx, ctlPath, uuid, life), errgo.Any)
}

func (j jemShim) UpdateModelCounts(ctx context.Context, uuid string, counts map[params.EntityCount]int, now time.Time) (err error) {
	return errgo.Mask(j.DB.UpdateModelCounts(ctx, uuid, counts, now), errgo.Any)
}

func (j jemShim) UpdateMachineInfo(ctx context.Context, info *multiwatcher.MachineInfo) error {
	return errgo.Mask(j.DB.UpdateMachineInfo(ctx, info), errgo.Any)
}

func (j jemShim) SetControllerVersion(ctx context.Context, ctlPath params.EntityPath, v version.Number) error {
	return errgo.Mask(j.DB.SetControllerVersion(ctx, ctlPath, v), errgo.Any)
}

func (j jemShim) AcquireMonitorLease(ctx context.Context, ctlPath params.EntityPath, oldExpiry time.Time, oldOwner string, newExpiry time.Time, newOwner string) (time.Time, error) {
	t, err := j.DB.AcquireMonitorLease(ctx, ctlPath, oldExpiry, oldOwner, newExpiry, newOwner)
	if err != nil {
		return time.Time{}, errgo.Mask(err, errgo.Any)
	}
	return t, nil
}

func (j jemShim) Controller(ctx context.Context, ctlPath params.EntityPath) (*mongodoc.Controller, error) {
	ctl, err := j.DB.Controller(ctx, ctlPath)
	return ctl, errgo.Mask(err, errgo.Any)
}

type apiShim struct {
	*apiconn.Conn
}

func (a apiShim) WatchAllModels() (allWatcher, error) {
	w, err := apicontroller.NewClient(a.Conn).WatchAllModels()
	if err != nil {
		return nil, errgo.Mask(err, errgo.Any)
	}
	return w, nil
}
