// Copyright 2016 Canonical Ltd.

package jujuapi

import (
	"context"
	"sync"

	"github.com/juju/names/v4"
	"github.com/rogpeppe/fastuuid"

	"github.com/CanonicalLtd/jimm/internal/dbmodel"
	"github.com/CanonicalLtd/jimm/internal/errors"
	"github.com/CanonicalLtd/jimm/internal/jimm"
	"github.com/CanonicalLtd/jimm/internal/jujuapi/rpc"
	"github.com/CanonicalLtd/jimm/internal/openfga"
)

// controllerRoot is the root for endpoints served on controller connections.
type controllerRoot struct {
	rpc.Root

	params   Params
	jimm     *jimm.JIMM
	watchers *watcherRegistry
	pingF    func()

	// mu protects the fields below it
	mu                    sync.Mutex
	user                  *openfga.User
	controllerUUIDMasking bool
	generator             *fastuuid.Generator
}

func newControllerRoot(j *jimm.JIMM, p Params) *controllerRoot {
	watcherRegistry := &watcherRegistry{
		watchers: make(map[string]*modelSummaryWatcher),
	}
	r := &controllerRoot{
		params:                p,
		jimm:                  j,
		watchers:              watcherRegistry,
		pingF:                 func() {},
		controllerUUIDMasking: true,
	}

	r.AddMethod("Admin", 1, "Login", rpc.Method(unsupportedLogin))
	r.AddMethod("Admin", 2, "Login", rpc.Method(unsupportedLogin))
	r.AddMethod("Admin", 3, "Login", rpc.Method(r.Login))
	r.AddMethod("Pinger", 1, "Ping", rpc.Method(r.Ping))
	return r
}

// masquarade allows a controller superuser to perform an action on behalf
// of another user. masquarade checks that the authenticated user is a
// controller user and that the requested is a valid JAAS user. If these
// conditions are met then masquarade returns a replacement user to use in
// JIMM requests.
func (r *controllerRoot) masquerade(ctx context.Context, userTag string) (*openfga.User, error) {
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
	return openfga.NewUser(&user, r.jimm.OpenFGAClient), nil
}

// parseUserTag parses a names.UserTag and validates it is for an
// identity-provider user.
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

// setPingF configures the function to call when an ping is received.
func (r *controllerRoot) setPingF(f func()) {
	r.pingF = f
}

// cleanup releases all resources used by the controllerRoot.
func (r *controllerRoot) cleanup() {
	r.watchers.stop()
}

func (r *controllerRoot) setupUUIDGenerator() error {
	if r.generator != nil {
		return nil
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	var err error
	r.generator, err = fastuuid.NewGenerator()
	if err != nil {
		return errors.E(err)
	}
	return nil
}

func (r *controllerRoot) newAuditLogger() dbAuditLogger {
	return newDbAuditLogger(r.jimm, r.getUser)
}

// getUser implements jujuapi.root interface to return the currently logged in user.
func (r *controllerRoot) getUser() names.UserTag {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.user != nil {
		return r.user.ResourceTag()
	}
	return names.UserTag{}
}
