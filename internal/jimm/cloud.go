// Copyright 2020 Canonical Ltd.

package jimm

import (
	"context"

	"github.com/juju/names/v4"

	"github.com/CanonicalLtd/jimm/internal/dbmodel"
	"github.com/CanonicalLtd/jimm/internal/errors"
)

// GetCloud retrieves the cloud for the given cloud tag. If the cloud
// cannot be found then an error with the code CodeNotFound is
// returned. If the user does not have permission to view the cloud then an
// error with a code of CodeUnauthorized is returned. If the user only has
// add-model access to the cloud then the returned Users field will only
// contain the authentcated user.
func (j *JIMM) GetCloud(ctx context.Context, u *dbmodel.User, tag names.CloudTag) (dbmodel.Cloud, error) {
	const op = errors.Op("jimm.GetCloud")
	var cl dbmodel.Cloud
	cl.SetTag(tag)

	if err := j.Database.GetCloud(ctx, &cl); err != nil {
		return cl, errors.E(op, err)
	}
	switch cloudUserAccess(u, &cl) {
	case "admin":
		return cl, nil
	case "add-model":
		cl.Users = []dbmodel.UserCloudAccess{{
			Username: u.Username,
			User:     *u,
			Access:   "add-model",
		}}
		return cl, nil
	default:
		return cl, errors.E(op, errors.CodeUnauthorized)
	}
}

// ForEachCloud iterates through all of the clouds a user has access to
// calling the given function for each cloud. If the user has admin level
// access to the cloud then the provided cloud will include all user
// information, otherwise it will just include the authenticated user. If
// the authenticated user is a controller superuser and the all flag is
// true then f will be called with all clouds known to JIMM. If f returns
// an error then iteration will stop immediately and the error will be
// returned unchanged. The given function should not update the database.
func (j *JIMM) ForEachCloud(ctx context.Context, u *dbmodel.User, all bool, f func(*dbmodel.Cloud) error) error {
	const op = errors.Op("jimm.ForEachCloud")

	if all && u.ControllerAccess == "superuser" {
		return j.forEachAllClouds(ctx, f)
	}

	clds, err := j.Database.GetUserClouds(ctx, u)
	if err != nil {
		return errors.E(op, err, "cannot load clouds")
	}
	seen := make(map[string]bool, len(clds))
	for _, uca := range clds {
		cld := uca.Cloud
		if uca.Access != "admin" {
			cld.Users = []dbmodel.UserCloudAccess{{
				Username: u.Username,
				User:     *u,
				Access:   uca.Access,
			}}
		}
		if err := f(&cld); err != nil {
			return err
		}
		seen[cld.Name] = true
	}

	// Also include "public" clouds
	everyone := dbmodel.User{
		Username: "everyone@external",
	}
	clds, err = j.Database.GetUserClouds(ctx, &everyone)
	if err != nil {
		return errors.E(op, err, "cannot load clouds")
	}
	for _, uca := range clds {
		if seen[uca.CloudName] {
			continue
		}
		cld := uca.Cloud
		// For public clouds a user can only ever see themselves.
		cld.Users = []dbmodel.UserCloudAccess{{
			Username: u.Username,
			User:     *u,
			Access:   uca.Access,
		}}
		if err := f(&cld); err != nil {
			return err
		}
	}

	return nil
}

// forEachAllClouds iterates through each cloud known to JIMM calling the
// given function. If f returns an error then iteration stops immediately
// and the error is returned unmodified.
func (j *JIMM) forEachAllClouds(ctx context.Context, f func(*dbmodel.Cloud) error) error {
	const op = errors.Op("jimm.forEachAllCloud")

	clds, err := j.Database.GetClouds(ctx)
	if err != nil {
		return errors.E(op, "cannot load clouds", err)
	}

	for i := range clds {
		if err := f(&clds[i]); err != nil {
			return err
		}
	}
	return nil
}

// cloudUserAccess determines the level of access the given user has on the
// given cloud. The cloud object must have had its users association
// loaded.
func cloudUserAccess(u *dbmodel.User, cl *dbmodel.Cloud) string {
	if u.ControllerAccess == "superuser" {
		// A controller superuser automatically has admin access to a
		// cloud.
		return "admin"
	}
	var userAccess, everyoneAccess string
	for _, cu := range cl.Users {
		if cu.Username == u.Username {
			userAccess = cu.Access
		}
		if cu.Username == "everyone@external" {
			everyoneAccess = cu.Access
		}
	}
	if userAccess == "" {
		userAccess = everyoneAccess
	}
	return userAccess
}
