// Copyright 2016 Canonical Ltd.

package jujuapi

import (
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery/identchecker"
	"github.com/juju/juju/apiserver/facades/client/bundle"
	"github.com/juju/juju/core/permission"
	"github.com/juju/names/v4"

	"github.com/CanonicalLtd/jimm/internal/jujuapi/rpc"
)

func init() {
	facadeInit["Bundle"] = func(r *controllerRoot) []int {
		api, err := bundle.NewBundleAPIv1(nil, authorizer{r.identity}, names.NewModelTag(""))
		if err != nil {
			return nil
		}
		r.AddMethod("Bundle", 1, "GetChanges", rpc.Method(api.GetChanges))

		return []int{1}
	}
}

// authorizer implements facade.Authorizer
type authorizer struct {
	id identchecker.Identity
}

// GetAuthTag implements facade.Authorizer.
func (a authorizer) GetAuthTag() names.Tag {
	n := a.id.Id()
	if names.IsValidUserName(n) {
		return names.NewLocalUserTag(n)
	}
	return names.NewUserTag(n)
}

// AuthController implements facade.Authorizer.
func (authorizer) AuthController() bool {
	return false
}

// AuthMachineAgent implements facade.Authorizer.
func (authorizer) AuthMachineAgent() bool {
	return false
}

// AuthApplicationAgent implements facade.Authorizer.
func (authorizer) AuthApplicationAgent() bool {
	return false
}

// AuthUnitAgent implements facade.Authorizer.
func (authorizer) AuthUnitAgent() bool {
	return false
}

// AuthOwner implements facade.Authorizer.
func (a authorizer) AuthOwner(tag names.Tag) bool {
	t := a.GetAuthTag()
	return tag.Kind() == t.Kind() && tag.Id() == t.Id()
}

// AuthClient implements facade.Authorizer.
func (authorizer) AuthClient() bool {
	return true
}

// HasPermission implements facade.Authorizer.
func (authorizer) HasPermission(operation permission.Access, target names.Tag) (bool, error) {
	return false, nil
}

// UserHasPermission implements facade.Authorizer.
func (authorizer) UserHasPermission(user names.UserTag, operation permission.Access, target names.Tag) (bool, error) {
	return false, nil
}

// ConnectedModel implements facade.Authorizer.
func (authorizer) ConnectedModel() string {
	return ""
}

// AuthModelAgent implements facade.Authorizer.
func (authorizer) AuthModelAgent() bool {
	return false
}
