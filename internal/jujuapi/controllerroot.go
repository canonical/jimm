// Copyright 2024 Canonical.

package jujuapi

import (
	"context"
	"fmt"
	"sync"

	"github.com/juju/names/v5"
	"github.com/rogpeppe/fastuuid"
	"golang.org/x/oauth2"

	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/jimm"
	"github.com/canonical/jimm/v3/internal/jujuapi/rpc"
	"github.com/canonical/jimm/v3/internal/openfga"
	jimmnames "github.com/canonical/jimm/v3/pkg/names"
)

// controllerRoot is the root for endpoints served on controller connections.
type controllerRoot struct {
	rpc.Root

	params   Params
	jimm     JIMM
	watchers *watcherRegistry
	pingF    func()

	// mu protects the fields below it
	mu                    sync.Mutex
	user                  *openfga.User
	controllerUUIDMasking bool
	generator             *fastuuid.Generator

	// deviceOAuthResponse holds a device code flow response for this request,
	// such that JIMM can retrieve the access and ID tokens via polling the Authentication
	// Service's issuer via the /token endpoint.
	//
	// NOTE: As this is on the controller root struct, and a new controller root
	// is created per WS, it is EXPECTED that the subsequent call to GetDeviceSessionToken
	// happens on the SAME websocket.
	deviceOAuthResponse *oauth2.DeviceAuthResponse

	// identityId is the id of the identity attempting to login via a session cookie.
	identityId string
}

func newControllerRoot(j JIMM, p Params, identityId string) *controllerRoot {
	watcherRegistry := &watcherRegistry{
		watchers: make(map[string]*modelSummaryWatcher),
	}
	r := &controllerRoot{
		params:                p,
		jimm:                  j,
		watchers:              watcherRegistry,
		pingF:                 func() {},
		controllerUUIDMasking: true,
		identityId:            identityId,
	}

	r.AddMethod("Admin", 1, "Login", rpc.Method(unsupportedLogin))
	r.AddMethod("Admin", 2, "Login", rpc.Method(unsupportedLogin))
	r.AddMethod("Admin", 3, "Login", rpc.Method(unsupportedLogin))
	r.AddMethod("Admin", 4, "Login", rpc.Method(unsupportedLogin))
	r.AddMethod("Admin", 4, "LoginDevice", rpc.Method(r.LoginDevice))
	r.AddMethod("Admin", 4, "GetDeviceSessionToken", rpc.Method(r.GetDeviceSessionToken))
	r.AddMethod("Admin", 4, "LoginWithSessionToken", rpc.Method(r.LoginWithSessionToken))
	r.AddMethod("Admin", 4, "LoginWithSessionCookie", rpc.Method(r.LoginWithSessionCookie))
	r.AddMethod("Admin", 4, "LoginWithClientCredentials", rpc.Method(r.LoginWithClientCredentials))
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
	if !r.user.JimmAdmin {
		return nil, errors.E(errors.CodeUnauthorized, "unauthorized")
	}
	user, err := r.jimm.UserLogin(ctx, ut.Id())
	if err != nil {
		return nil, err
	}
	return user, nil
}

// parseUserTag parses a names.UserTag and validates it is for an
// identity-provider user.
func parseUserTag(tag string) (names.UserTag, error) {
	ut, err := names.ParseUserTag(tag)
	if err != nil {
		return names.UserTag{}, errors.E(errors.CodeBadRequest, err)
	}
	if ut.IsLocal() {
		return names.UserTag{}, errors.E(errors.CodeBadRequest, fmt.Sprintf("unsupported local user; if this is a service account add @%s domain", jimmnames.ServiceAccountDomain))
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

func (r *controllerRoot) newAuditLogger() jimm.DbAuditLogger {
	return jimm.NewDbAuditLogger(r.jimm, r.getUser)
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
