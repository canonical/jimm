// Copyright 2016 Canonical Ltd.

package jujuapi

import (
	"context"

	jujuparams "github.com/juju/juju/rpc/params"

	"github.com/CanonicalLtd/jimm/internal/errors"
	"github.com/CanonicalLtd/jimm/internal/jujuapi/rpc"
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
	return jujuparams.AddUserResults{}, errors.E(errors.CodeUnauthorized, "unauthorized")
}

// RemoveUser implements the UserManager facade's RemoveUser method.
func (r *controllerRoot) RemoveUser(jujuparams.Entities) (jujuparams.ErrorResults, error) {
	return jujuparams.ErrorResults{}, errors.E(errors.CodeUnauthorized, "unauthorized")
}

// EnableUser implements the UserManager facade's EnableUser method.
func (r *controllerRoot) EnableUser(jujuparams.Entities) (jujuparams.ErrorResults, error) {
	return jujuparams.ErrorResults{}, errors.E(errors.CodeUnauthorized, "unauthorized")
}

// DisableUser implements the UserManager facade's DisableUser method.
func (r *controllerRoot) DisableUser(jujuparams.Entities) (jujuparams.ErrorResults, error) {
	return jujuparams.ErrorResults{}, errors.E(errors.CodeUnauthorized, "unauthorized")
}

// UserInfo implements the UserManager facade's UserInfo method.
func (r *controllerRoot) UserInfo(ctx context.Context, req jujuparams.UserInfoRequest) (jujuparams.UserInfoResults, error) {
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
	const op = errors.Op("jujuapi.UserInfo")

	user, err := parseUserTag(entity)
	if err != nil {
		return nil, errors.E(op, err, errors.CodeBadRequest)
	}
	if r.user.Username != user.Id() {
		return nil, errors.E(op, errors.CodeUnauthorized)
	}
	ui := r.user.ToJujuUserInfo()
	return &ui, nil
}

// SetPassword implements the UserManager facade's SetPassword method.
func (r *controllerRoot) SetPassword(jujuparams.EntityPasswords) (jujuparams.ErrorResults, error) {
	return jujuparams.ErrorResults{}, errors.E(errors.CodeUnauthorized, "unauthorized")
}
