// Copyright 2016 Canonical Ltd.

package monitor

import (
	"context"

	apicontroller "github.com/juju/juju/api/controller"
	jujuparams "github.com/juju/juju/apiserver/params"
	"github.com/juju/names/v4"
	"gopkg.in/errgo.v1"

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
	conn, err := j.JEM.ConnectMonitor(ctx, path)
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

func (j jemShim) DeleteModelWithUUID(ctx context.Context, ctlPath params.EntityPath, uuid string) error {
	return errgo.Mask(j.DB.RemoveModel(ctx, &mongodoc.Model{Controller: ctlPath, UUID: uuid}), errgo.Any)
}

func (j jemShim) Controller(ctx context.Context, ctlPath params.EntityPath) (*mongodoc.Controller, error) {
	ctl := &mongodoc.Controller{Path: ctlPath}
	err := j.DB.GetController(ctx, ctl)
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
		return false, errgo.Mask(err)
	}

	if len(results) != 1 {
		return false, errgo.Notef(err, "unexpected result count, %d, from ModelStatus call", len(results))
	}
	if results[0].Error != nil {
		if jujuparams.IsCodeNotFound(err) {
			return false, nil
		}
		return false, errgo.Mask(err)
	}
	return true, nil
}

func (a apiShim) WatchAllModels() (allWatcher, error) {
	w, err := apicontroller.NewClient(a.Conn).WatchAllModels()
	if err != nil {
		return nil, errgo.Mask(err, errgo.Any)
	}
	return w, nil
}
