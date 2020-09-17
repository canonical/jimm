// Copyright 2016 Canonical Ltd.

package jujuapi

import (
	"context"

	jujuparams "github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/permission"
	"github.com/juju/names/v4"
	"gopkg.in/errgo.v1"
	"gopkg.in/macaroon-bakery.v2/bakery/identchecker"

	"github.com/CanonicalLtd/jimm/internal/auth"
	"github.com/CanonicalLtd/jimm/internal/conv"
	"github.com/CanonicalLtd/jimm/internal/jujuapi/rpc"
	"github.com/CanonicalLtd/jimm/params"
)

func init() {
	facadeInit["UserManager"] = func(r *controllerRoot) []int {
		addUserMethod := rpc.Method(r.AddUser)
		disableUserMethod := rpc.Method(r.EnableUser)
		enableUserMethod := rpc.Method(r.DisableUser)
		removeUserMethod := rpc.Method(r.RemoveUser)
		setPasswordMethod := rpc.Method(r.SetPassword)
		userInfoMethod := rpc.Method(r.UserInfo)

		r.AddMethod("UserManager", 1, "AddUser", addUserMethod)
		r.AddMethod("UserManager", 1, "DisableUser", disableUserMethod)
		r.AddMethod("UserManager", 1, "EnableUser", enableUserMethod)
		r.AddMethod("UserManager", 1, "RemoveUser", removeUserMethod)
		r.AddMethod("UserManager", 1, "SetPassword", setPasswordMethod)
		r.AddMethod("UserManager", 1, "UserInfo", userInfoMethod)

		return []int{1}
	}
}

// AddUser implements the UserManager facade's AddUser method.
func (r *controllerRoot) AddUser(args jujuparams.AddUsers) (jujuparams.AddUserResults, error) {
	return jujuparams.AddUserResults{}, params.ErrUnauthorized
}

// RemoveUser implements the UserManager facade's RemoveUser method.
func (r *controllerRoot) RemoveUser(jujuparams.Entities) (jujuparams.ErrorResults, error) {
	return jujuparams.ErrorResults{}, params.ErrUnauthorized
}

// EnableUser implements the UserManager facade's EnableUser method.
func (r *controllerRoot) EnableUser(jujuparams.Entities) (jujuparams.ErrorResults, error) {
	return jujuparams.ErrorResults{}, params.ErrUnauthorized
}

// DisableUser implements the UserManager facade's DisableUser method.
func (r *controllerRoot) DisableUser(jujuparams.Entities) (jujuparams.ErrorResults, error) {
	return jujuparams.ErrorResults{}, params.ErrUnauthorized
}

// UserInfo implements the UserManager facade's UserInfo method.
func (r *controllerRoot) UserInfo(ctx context.Context, req jujuparams.UserInfoRequest) (jujuparams.UserInfoResults, error) {
	ctx = auth.ContextWithIdentity(ctx, r.identity)
	res := jujuparams.UserInfoResults{
		Results: make([]jujuparams.UserInfoResult, len(req.Entities)),
	}
	for i, ent := range req.Entities {
		ui, err := r.userInfo(ctx, ent.Tag)
		if err != nil {
			res.Results[i].Error = mapError(err)
			continue
		}
		res.Results[i].Result = ui
	}
	return res, nil
}

func (r *controllerRoot) userInfo(ctx context.Context, entity string) (*jujuparams.UserInfo, error) {
	userTag, err := names.ParseUserTag(entity)
	if err != nil {
		return nil, errgo.WithCausef(err, params.ErrBadRequest, "invalid user tag")
	}
	user, err := conv.FromUserTag(userTag)
	if err != nil {
		return nil, errgo.Mask(err, errgo.Is(conv.ErrLocalUser))
	}
	if r.identity.Id() != string(user) {
		return nil, params.ErrUnauthorized
	}
	return r.currentUser(r.identity)
}

func (r *controllerRoot) currentUser(id identchecker.ACLIdentity) (*jujuparams.UserInfo, error) {
	userTag := userTag(id.Id())
	return &jujuparams.UserInfo{
		// TODO(mhilton) a number of these fields should
		// be fetched from the identity manager, but that
		// will have to change to support getting them.
		Username:    userTag.Id(),
		DisplayName: userTag.Id(),
		Access:      string(permission.AddModelAccess),
		Disabled:    false,
	}, nil
}

// SetPassword implements the UserManager facade's SetPassword method.
func (r *controllerRoot) SetPassword(jujuparams.EntityPasswords) (jujuparams.ErrorResults, error) {
	return jujuparams.ErrorResults{}, params.ErrUnauthorized
}
