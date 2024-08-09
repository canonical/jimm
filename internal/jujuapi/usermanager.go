// Copyright 2024 Canonical.

package jujuapi

import (
	"context"

	jujuparams "github.com/juju/juju/rpc/params"

	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/jujuapi/rpc"
)

func init() {
	facadeInit["UserManager"] = func(r *controllerRoot) []int {
		addUserMethod := rpc.Method(r.AddUser)
		disableUserMethod := rpc.Method(r.EnableUser)
		enableUserMethod := rpc.Method(r.DisableUser)
		removeUserMethod := rpc.Method(r.RemoveUser)
		setPasswordMethod := rpc.Method(r.SetPassword)
		userInfoMethod := rpc.Method(r.UserInfo)
		resetPasswordMethod := rpc.Method(r.ResetPassword)
		modelUserInfoMethod := rpc.Method(r.ModelUserInfo)

		r.AddMethod("UserManager", 3, "AddUser", addUserMethod)
		r.AddMethod("UserManager", 3, "DisableUser", disableUserMethod)
		r.AddMethod("UserManager", 3, "EnableUser", enableUserMethod)
		r.AddMethod("UserManager", 3, "RemoveUser", removeUserMethod)
		r.AddMethod("UserManager", 3, "SetPassword", setPasswordMethod)
		r.AddMethod("UserManager", 3, "UserInfo", userInfoMethod)
		r.AddMethod("UserManager", 3, "ResetPassword", resetPasswordMethod)
		r.AddMethod("UserManager", 3, "ModelUserInfo", modelUserInfoMethod)

		return []int{3}
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

// ModelUserInfo returns information on all users in the model.
func (r *controllerRoot) ModelUserInfo(args jujuparams.Entities) (jujuparams.ModelUserInfoResults, error) {
	return jujuparams.ModelUserInfoResults{}, errors.E(errors.CodeNotImplemented, "not implements")
}

// UserInfo implements the UserManager facade's UserInfo method.
func (r *controllerRoot) UserInfo(ctx context.Context, req jujuparams.UserInfoRequest) (jujuparams.UserInfoResults, error) {
	res := jujuparams.UserInfoResults{
		Results: make([]jujuparams.UserInfoResult, len(req.Entities)),
	}
	for i, ent := range req.Entities {
		ui, err := r.userInfo(ent.Tag)
		if err != nil {
			res.Results[i].Error = mapError(err)
			continue
		}
		res.Results[i].Result = ui
	}
	return res, nil
}

func (r *controllerRoot) userInfo(entity string) (*jujuparams.UserInfo, error) {
	const op = errors.Op("jujuapi.UserInfo")

	user, err := parseUserTag(entity)
	if err != nil {
		return nil, errors.E(op, err, errors.CodeBadRequest)
	}
	if r.user.Name != user.Id() {
		return nil, errors.E(op, errors.CodeUnauthorized)
	}
	ui := r.user.ToJujuUserInfo()
	return &ui, nil
}

// SetPassword implements the UserManager facade's SetPassword method.
func (r *controllerRoot) SetPassword(jujuparams.EntityPasswords) (jujuparams.ErrorResults, error) {
	return jujuparams.ErrorResults{}, errors.E(errors.CodeUnauthorized, "unauthorized")
}

// ResetPassword implements the UserManager facade's ResetPassword method.
func (r *controllerRoot) ResetPassword(jujuparams.Entities) (jujuparams.ErrorResults, error) {
	// JIMM does not support resetting user's password.
	return jujuparams.ErrorResults{}, errors.E(errors.CodeUnauthorized, "unauthorized")
}
