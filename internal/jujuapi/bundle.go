// Copyright 2016 Canonical Ltd.

package jujuapi

import (
	"github.com/juju/juju/apiserver/facades/client/bundle"
	"github.com/juju/juju/core/permission"
	"github.com/juju/names/v4"
	"gopkg.in/macaroon-bakery.v2/bakery/identchecker"

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

func (a authorizer) GetAuthTag() names.Tag {
	n := a.id.Id()
	if names.IsValidUserName(n) {
		return names.NewLocalUserTag(n)
	}
	return names.NewUserTag(n)
}

func (authorizer) AuthController() bool {
	return false
}

func (authorizer) AuthMachineAgent() bool {
	return false
}

func (authorizer) AuthApplicationAgent() bool {
	return false
}

func (authorizer) AuthUnitAgent() bool {
	return false
}

func (a authorizer) AuthOwner(tag names.Tag) bool {
	t := a.GetAuthTag()
	return tag.Kind() == t.Kind() && tag.Id() == t.Id()
}

func (authorizer) AuthClient() bool {
	return true
}

func (authorizer) HasPermission(operation permission.Access, target names.Tag) (bool, error) {
	return false, nil
}

func (authorizer) UserHasPermission(user names.UserTag, operation permission.Access, target names.Tag) (bool, error) {
	return false, nil
}

func (authorizer) ConnectedModel() string {
	return ""
}

func (authorizer) AuthModelAgent() bool {
	return false
}
