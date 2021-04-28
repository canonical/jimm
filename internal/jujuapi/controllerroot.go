// Copyright 2016 Canonical Ltd.

package jujuapi

import (
	"context"
	"sync"

	"github.com/juju/names/v4"
	"github.com/juju/rpcreflect"
	"github.com/rogpeppe/fastuuid"

	"github.com/CanonicalLtd/jimm/internal/dbmodel"
	"github.com/CanonicalLtd/jimm/internal/errors"
	"github.com/CanonicalLtd/jimm/internal/jemserver"
	"github.com/CanonicalLtd/jimm/internal/jimm"
	"github.com/CanonicalLtd/jimm/internal/jujuapi/rpc"
)

// controllerRoot is the root for endpoints served on controller connections.
type controllerRoot struct {
	rpc.Root

	params       jemserver.Params
	jimm         *jimm.JIMM
	heartMonitor heartMonitor
	watchers     *watcherRegistry

	// mu protects the fields below it
	mu                    sync.Mutex
	user                  *dbmodel.User
	controllerUUIDMasking bool
	generator             *fastuuid.Generator
}

func newControllerRoot(j *jimm.JIMM, p jemserver.Params, hm heartMonitor) *controllerRoot {
	r := &controllerRoot{
		params:       p,
		jimm:         j,
		heartMonitor: hm,
		watchers: &watcherRegistry{
			watchers: make(map[string]*modelSummaryWatcher),
		},
		controllerUUIDMasking: true,
	}

	r.AddMethod("Admin", 1, "Login", rpc.Method(unsupportedLogin))
	r.AddMethod("Admin", 2, "Login", rpc.Method(unsupportedLogin))
	r.AddMethod("Admin", 3, "Login", rpc.Method(r.Login))
	r.AddMethod("Pinger", 1, "Ping", rpc.Method(ping))
	return r
}

// masquarade allows a controller superuser to perform an action on behalf
// of another user. masquarade checks that the authenticated user is a
// controller user and that the requested is a valid JAAS user. If these
// conditions are met then masquarade returns a replacement user to use in
// JIMM requests.
func (r *controllerRoot) masquerade(ctx context.Context, userTag string) (*dbmodel.User, error) {
	ut, err := parseUserTag(userTag)
	if err != nil {
		return nil, errors.E(errors.CodeBadRequest, err)
	}
	if r.user.Tag() == ut {
		// allow anyone to masquarade as themselves.
		return r.user, nil
	}
	if r.user.ControllerAccess != "superuser" {
		return nil, errors.E(errors.CodeUnauthorized, "permission denied")
	}
	user := dbmodel.User{
		Username: ut.Id(),
	}
	if err := r.jimm.Database.GetUser(ctx, &user); err != nil {
		return nil, err
	}
	return &user, nil
}

func parseUserTag(tag string) (names.UserTag, error) {
	ut, err := names.ParseUserTag(tag)
	if err != nil {
		return names.UserTag{}, errors.E(errors.CodeBadRequest, err)
	}
	if ut.IsLocal() {
		return names.UserTag{}, errors.E(errors.CodeBadRequest, "unsupported local user")
	}
	return ut, nil
}

// FindMethod implements rpcreflect.MethodFinder.
func (r *controllerRoot) FindMethod(rootName string, version int, methodName string) (rpcreflect.MethodCaller, error) {
	// update the heart monitor for every request received.
	r.heartMonitor.Heartbeat()
	return r.Root.FindMethod(rootName, version, methodName)
}
