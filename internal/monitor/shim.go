// Copyright 2016 Canonical Ltd.

package monitor

import (
	"context"
	"time"

	cloudapi "github.com/juju/juju/api/cloud"
	apicontroller "github.com/juju/juju/api/controller"
	jujuparams "github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/state/multiwatcher"
	"github.com/juju/version"
	"gopkg.in/errgo.v1"
	"gopkg.in/juju/names.v2"

	"github.com/CanonicalLtd/jimm/internal/apiconn"
	"github.com/CanonicalLtd/jimm/internal/jem"
	"github.com/CanonicalLtd/jimm/internal/mongodoc"
	"github.com/CanonicalLtd/jimm/params"
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

func (j jemShim) ModelUUIDsForController(ctx context.Context, ctlPath params.EntityPath) ([]string, error) {
	uuids, err := j.DB.ModelUUIDsForController(ctx, ctlPath)
	if err != nil {
		return nil, errgo.Mask(err)
	}
	return uuids, nil
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

func (j jemShim) SetModelInfo(ctx context.Context, ctlPath params.EntityPath, uuid string, info *mongodoc.ModelInfo) error {
	return errgo.Mask(j.DB.SetModelInfo(ctx, ctlPath, uuid, info), errgo.Any)
}

func (j jemShim) DeleteModelWithUUID(ctx context.Context, ctlPath params.EntityPath, uuid string) error {
	return errgo.Mask(j.DB.DeleteModelWithUUID(ctx, ctlPath, uuid), errgo.Any)
}

func (j jemShim) UpdateModelCounts(ctx context.Context, ctlPath params.EntityPath, uuid string, counts map[params.EntityCount]int, now time.Time) (err error) {
	return errgo.Mask(j.DB.UpdateModelCounts(ctx, ctlPath, uuid, counts, now), errgo.Any)
}

func (j jemShim) RemoveControllerMachines(ctx context.Context, ctlPath params.EntityPath) error {
	return errgo.Mask(j.DB.RemoveControllerMachines(ctx, ctlPath), errgo.Any)
}

func (j jemShim) RemoveControllerApplications(ctx context.Context, ctlPath params.EntityPath) error {
	return errgo.Mask(j.DB.RemoveControllerApplications(ctx, ctlPath), errgo.Any)
}

func (j jemShim) UpdateMachineInfo(ctx context.Context, ctlPath params.EntityPath, info *multiwatcher.MachineInfo) error {
	return errgo.Mask(j.JEM.UpdateMachineInfo(ctx, ctlPath, info), errgo.Any)
}

func (j jemShim) UpdateApplicationInfo(ctx context.Context, ctlPath params.EntityPath, info *multiwatcher.ApplicationInfo) error {
	return errgo.Mask(j.JEM.UpdateApplicationInfo(ctx, ctlPath, info), errgo.Any)
}

func (j jemShim) SetControllerVersion(ctx context.Context, ctlPath params.EntityPath, v version.Number) error {
	return errgo.Mask(j.DB.SetControllerVersion(ctx, ctlPath, v), errgo.Any)
}

func (j jemShim) SetControllerRegions(ctx context.Context, ctlPath params.EntityPath, regions []mongodoc.Region) error {
	return errgo.Mask(j.DB.SetControllerRegions(ctx, ctlPath, regions), errgo.Any)
}

func (j jemShim) UpdateCloudRegions(ctx context.Context, cloudRegions []mongodoc.CloudRegion) error {
	return errgo.Mask(j.DB.UpdateCloudRegions(ctx, cloudRegions), errgo.Any)
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

func (a apiShim) ModelExists(uuid string) (bool, error) {
	if !names.IsValidModel(uuid) {
		return false, nil
	}
	results, err := apicontroller.NewClient(a.Conn).ModelStatus(names.NewModelTag(uuid))
	if err != nil {
		if jujuparams.IsCodeNotFound(err) {
			return false, nil
		}
		return false, errgo.Mask(err)
	}

	if len(results) != 1 {
		return false, errgo.Notef(err, "unexpected result count, %d, from ModelStatus call", len(results))
	}
	// A later version of the API adds an Error parameter, but we
	// can't use that yet, so rely on UUID as a proxy for "is found".
	return results[0].UUID != "", nil
}

func (a apiShim) WatchAllModels() (allWatcher, error) {
	w, err := apicontroller.NewClient(a.Conn).WatchAllModels()
	if err != nil {
		return nil, errgo.Mask(err, errgo.Any)
	}
	return w, nil
}

func (a apiShim) Clouds() (map[names.CloudTag]cloud.Cloud, error) {
	clouds, err := cloudapi.NewClient(a.Conn).Clouds()
	if err != nil {
		return nil, errgo.Mask(err, errgo.Any)
	}
	return clouds, nil
}
