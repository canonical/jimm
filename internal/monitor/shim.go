// Copyright 2016 Canonical Ltd.

package monitor

import (
	"time"

	apicontroller "github.com/juju/juju/api/controller"
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

func (j jemShim) OpenAPI(path params.EntityPath) (jujuAPI, error) {
	conn, err := j.JEM.OpenAPI(path)
	if err != nil {
		return nil, errgo.Mask(err, errgo.Any)
	}
	return apiShim{conn}, nil
}

func (j jemShim) AllControllers() ([]*mongodoc.Controller, error) {
	var ctls []*mongodoc.Controller
	err := j.DB.Controllers().Find(nil).All(&ctls)
	if err != nil {
		return nil, errgo.Mask(err)
	}
	return ctls, nil
}

func (j jemShim) SetControllerStats(ctlPath params.EntityPath, stats *mongodoc.ControllerStats) error {
	return errgo.Mask(j.DB.SetControllerStats(ctlPath, stats), errgo.Any)
}

func (j jemShim) SetControllerUnavailableAt(ctlPath params.EntityPath, t time.Time) error {
	return errgo.Mask(j.DB.SetControllerUnavailableAt(ctlPath, t), errgo.Any)
}

func (j jemShim) SetControllerAvailable(ctlPath params.EntityPath) error {
	return errgo.Mask(j.DB.SetControllerAvailable(ctlPath), errgo.Any)
}

func (j jemShim) SetModelLife(ctlPath params.EntityPath, uuid string, life string) error {
	return errgo.Mask(j.DB.SetModelLife(ctlPath, uuid, life), errgo.Any)
}

func (j jemShim) SetModelUnitCount(ctlPath params.EntityPath, uuid string, n int) (err error) {
	return errgo.Mask(j.DB.SetModelUnitCount(ctlPath, uuid, n), errgo.Any)
}

func (j jemShim) AcquireMonitorLease(ctlPath params.EntityPath, oldExpiry time.Time, oldOwner string, newExpiry time.Time, newOwner string) (time.Time, error) {
	t, err := j.DB.AcquireMonitorLease(ctlPath, oldExpiry, oldOwner, newExpiry, newOwner)
	if err != nil {
		return time.Time{}, errgo.Mask(err, errgo.Any)
	}
	return t, nil
}

func (j jemShim) Controller(ctlPath params.EntityPath) (*mongodoc.Controller, error) {
	ctl, err := j.DB.Controller(ctlPath)
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
