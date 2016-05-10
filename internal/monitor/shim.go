// Copyright 2016 Canonical Ltd.

package monitor

import (
	apicontroller "github.com/juju/juju/api/controller"
	"gopkg.in/errgo.v1"

	"github.com/CanonicalLtd/jem/internal/apiconn"
	"github.com/CanonicalLtd/jem/internal/jem"
	"github.com/CanonicalLtd/jem/internal/mongodoc"
	"github.com/CanonicalLtd/jem/params"
)

// jemShim implements the jemInterface interface
// by using a *jem.JEM directly.
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
