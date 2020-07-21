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
	"github.com/CanonicalLtd/jimm/params"
)

// userManager implements the UserManager facade.
type userManager struct {
	root *controllerRoot
}

// AddUser implements the UserManager facade's AddUser method.
func (u userManager) AddUser(args jujuparams.AddUsers) (jujuparams.AddUserResults, error) {
	return jujuparams.AddUserResults{}, params.ErrUnauthorized
}

// RemoveUser implements the UserManager facade's RemoveUser method.
func (u userManager) RemoveUser(jujuparams.Entities) (jujuparams.ErrorResults, error) {
	return jujuparams.ErrorResults{}, params.ErrUnauthorized
}

// EnableUser implements the UserManager facade's EnableUser method.
func (u userManager) EnableUser(jujuparams.Entities) (jujuparams.ErrorResults, error) {
	return jujuparams.ErrorResults{}, params.ErrUnauthorized
}

// DisableUser implements the UserManager facade's DisableUser method.
func (u userManager) DisableUser(jujuparams.Entities) (jujuparams.ErrorResults, error) {
	return jujuparams.ErrorResults{}, params.ErrUnauthorized
}

// UserInfo implements the UserManager facade's UserInfo method.
func (u userManager) UserInfo(ctx context.Context, req jujuparams.UserInfoRequest) (jujuparams.UserInfoResults, error) {
	ctx = auth.ContextWithIdentity(ctx, u.root.identity)
	res := jujuparams.UserInfoResults{
		Results: make([]jujuparams.UserInfoResult, len(req.Entities)),
	}
	for i, ent := range req.Entities {
		ui, err := u.userInfo(ctx, ent.Tag)
		if err != nil {
			res.Results[i].Error = mapError(err)
			continue
		}
		res.Results[i].Result = ui
	}
	return res, nil
}

func (u userManager) userInfo(ctx context.Context, entity string) (*jujuparams.UserInfo, error) {
	userTag, err := names.ParseUserTag(entity)
	if err != nil {
		return nil, errgo.WithCausef(err, params.ErrBadRequest, "invalid user tag")
	}
	user, err := user(userTag)
	if err != nil {
		return nil, errgo.Mask(err, errgo.Is(params.ErrBadRequest))
	}
	if u.root.identity.Id() != string(user) {
		return nil, params.ErrUnauthorized
	}
	return u.currentUser(u.root.identity)
}

func (u userManager) currentUser(id identchecker.ACLIdentity) (*jujuparams.UserInfo, error) {
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
func (u userManager) SetPassword(jujuparams.EntityPasswords) (jujuparams.ErrorResults, error) {
	return jujuparams.ErrorResults{}, params.ErrUnauthorized
}
