// Copyright 2017 Canonical Ltd.

package jujuapi

import (
	"context"
	"reflect"

	"github.com/juju/juju/apiserver/common"
	jujuparams "github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/rpc"
	"github.com/juju/rpcreflect"
	"gopkg.in/errgo.v1"

	"github.com/CanonicalLtd/jimm/internal/jem"
	"github.com/CanonicalLtd/jimm/internal/mongodoc"
	"github.com/CanonicalLtd/jimm/params"
)

// modelRoot is the root for endpoints served on model connections.
type modelRoot struct {
	authContext  context.Context
	jem          *jem.JEM
	uuid         string
	model        *mongodoc.Model
	controller   *mongodoc.Controller
	heartMonitor heartMonitor
	findMethod   func(rootName string, version int, methodName string) (rpcreflect.MethodCaller, error)
}

func newModelRoot(jem *jem.JEM, hm heartMonitor, uuid string) *modelRoot {
	r := &modelRoot{
		jem:          jem,
		uuid:         uuid,
		heartMonitor: hm,
	}
	r.findMethod = rpcreflect.ValueOf(reflect.ValueOf(r)).FindMethod
	return r
}

// Admin returns an implementation of the Admin facade (version 3).
func (r *modelRoot) Admin(id string) (modelAdmin, error) {
	if id != "" {
		// Safeguard id for possible future use.
		return modelAdmin{}, common.ErrBadId
	}
	return modelAdmin{r}, nil
}

// Pinger returns an implementation of the Pinger facade (version 1).
func (r *modelRoot) Pinger(id string) (pinger, error) {
	if id != "" {
		// Safeguard id for possible future use.
		return pinger{}, common.ErrBadId
	}
	return pinger{}, nil
}

// FindMethod implements rpcreflect.MethodFinder.
func (r *modelRoot) FindMethod(rootName string, version int, methodName string) (rpcreflect.MethodCaller, error) {
	// update the heart monitor for every request received.
	r.heartMonitor.Heartbeat()

	if rootName == "Admin" && version < 3 {
		return nil, &rpc.RequestError{
			Code:    jujuparams.CodeNotSupported,
			Message: "JIMM does not support login from old clients",
		}
	}

	return r.findMethod(rootName, 0, methodName)
}

// Kill implements rpcreflect.Root.Kill.
func (r *modelRoot) Kill() {}

// modelInfo retrieves the data about the model.
func (r *modelRoot) modelInfo(ctx context.Context) (*mongodoc.Model, *mongodoc.Controller, error) {
	if r.model == nil {
		var err error
		r.model, err = r.jem.DB.ModelFromUUID(ctx, r.uuid)
		if errgo.Cause(err) == params.ErrNotFound {
			return nil, nil, errgo.WithCausef(err, params.ErrModelNotFound, "%s", "")
		}
		if err != nil {
			return nil, nil, errgo.Mask(err)
		}
		r.controller, err = r.jem.DB.Controller(ctx, r.model.Controller)
		if err != nil {
			return nil, nil, errgo.Mask(err)
		}
	}
	return r.model, r.controller, nil
}
