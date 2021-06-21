// Copyright 2020 Canonical Ltd.

package jimm

import (
	"context"
	"fmt"
	"strings"
	"time"

	jujuparams "github.com/juju/juju/apiserver/params"
	"github.com/juju/names/v4"

	"github.com/CanonicalLtd/jimm/internal/auth"
	"github.com/CanonicalLtd/jimm/internal/db"
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
		return cl, errors.E(op, errors.CodeUnauthorized, "unauthorized")
	}
}

// ForEachUserCloud iterates through all of the clouds a user has access to
// calling the given function for each cloud. If the user has admin level
// access to the cloud then the provided cloud will include all user
// information, otherwise it will just include the authenticated user. If
// the authenticated user is a controller superuser and the all flag is
// true then f will be called with all clouds known to JIMM. If f returns
// an error then iteration will stop immediately and the error will be
// returned unchanged. The given function should not update the database.
func (j *JIMM) ForEachUserCloud(ctx context.Context, u *dbmodel.User, f func(*dbmodel.Cloud) error) error {
	const op = errors.Op("jimm.ForEachUserCloud")

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
		Username: auth.Everyone,
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

// ForEachCloud iterates through each cloud known to JIMM calling the given
// function. If f returns an error then iteration stops immediately and the
// error is returned unmodified. If the given user is not a controller
// superuser then an error with the code CodeUnauthorized is returned. The
// given function should not update the database.
func (j *JIMM) ForEachCloud(ctx context.Context, u *dbmodel.User, f func(*dbmodel.Cloud) error) error {
	const op = errors.Op("jimm.ForEachCloud")

	if u.ControllerAccess != "superuser" {
		return errors.E(op, errors.CodeUnauthorized, "unauthorized")
	}

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
		if cu.Username == auth.Everyone {
			everyoneAccess = cu.Access
		}
	}
	if userAccess == "" {
		userAccess = everyoneAccess
	}
	return userAccess
}

// DefaultReservedCloudNames contains a list of cloud names that are used
// with public (or similar) clouds that cannot be used for the name of a
// hosted cloud.
var DefaultReservedCloudNames = []string{
	"aks",
	"aws",
	"aws-china",
	"aws-gov",
	"azure",
	"azure-china",
	"cloudsigma",
	"ecs",
	"eks",
	"google",
	"joyent",
	"localhost",
	"oracle",
	"oracle-classic",
	"oracle-compute",
	"rackspace",
}

// AddHostedCloud adds the cloud defined by the given tag and cloud to the
// JAAS system. The cloud will be created on a controller running on the
// requested host cloud-region and the cloud created there. If the given
// user does not have add-model access to JAAS then an error with a code of
// CodeUnauthorized will be returned (please note this differs from juju
// which requires admin controller access to create clouds). If the
// requested cloud cannot be created on this JAAS system an error with a
// code of CodeIncompatibleClouds will be returned. If there is an error
// returned by the controller when creating the cloud then that error code
// will be preserved.
func (j *JIMM) AddHostedCloud(ctx context.Context, u *dbmodel.User, tag names.CloudTag, cloud jujuparams.Cloud) error {
	const op = errors.Op("jimm.AddHostedCloud")

	ale := dbmodel.AuditLogEntry{
		Time:    time.Now().UTC().Round(time.Millisecond),
		Tag:     tag.String(),
		UserTag: u.Tag().String(),
		Action:  "add",
		Params: dbmodel.StringMap{
			"type":              cloud.Type,
			"host-cloud-region": cloud.HostCloudRegion,
		},
	}
	defer j.addAuditLogEntry(&ale)
	fail := func(err error) error {
		ale.Params["err"] = err.Error()
		return err
	}

	if u.ControllerAccess != "add-model" && u.ControllerAccess != "superuser" {
		return fail(errors.E(op, errors.CodeUnauthorized, "unauthorized"))
	}

	// Ensure the new cloud could not mask the name of a known public cloud.
	reservedNames := j.ReservedCloudNames
	if len(reservedNames) == 0 {
		reservedNames = DefaultReservedCloudNames
	}
	for _, n := range reservedNames {
		if tag.Id() == n {
			return fail(errors.E(op, errors.CodeAlreadyExists, fmt.Sprintf("cloud %q already exists", tag.Id())))
		}
	}

	// Validate that the requested cloud is valid.
	if cloud.Type != "kubernetes" {
		return fail(errors.E(op, errors.CodeIncompatibleClouds, fmt.Sprintf("unsupported cloud type %q", cloud.Type)))
	}
	if cloud.HostCloudRegion == "" {
		return fail(errors.E(op, errors.CodeCloudRegionRequired, "cloud host region not specified"))
	}
	parts := strings.SplitN(cloud.HostCloudRegion, "/", 2)
	if len(parts) != 2 || parts[0] == "" {
		return fail(errors.E(op, errors.CodeIncompatibleClouds, fmt.Sprintf("unsupported cloud host region %q", cloud.HostCloudRegion)))
	}
	region, err := j.Database.FindRegion(ctx, parts[0], parts[1])
	if errors.ErrorCode(err) == errors.CodeNotFound {
		return fail(errors.E(op, err, errors.CodeIncompatibleClouds, fmt.Sprintf("unsupported cloud host region %q", cloud.HostCloudRegion)))
	} else if err != nil {
		return fail(errors.E(op, err))
	}

	switch cloudUserAccess(u, &region.Cloud) {
	case "admin", "add-model":
	default:
		return fail(errors.E(op, errors.CodeIncompatibleClouds, fmt.Sprintf("unsupported cloud host region %q", cloud.HostCloudRegion)))
	}

	if region.Cloud.HostCloudRegion != "" {
		// Do not support creating a new cloud on an already hosted
		// cloud.
		return fail(errors.E(op, errors.CodeIncompatibleClouds, fmt.Sprintf("unsupported cloud host region %q", cloud.HostCloudRegion)))
	}

	// Create the cloud locally, to reserve the name.
	var dbCloud dbmodel.Cloud
	dbCloud.FromJujuCloud(cloud)
	dbCloud.Name = tag.Id()
	dbCloud.Users = []dbmodel.UserCloudAccess{{
		User:   *u,
		Access: "admin",
	}}
	if err := j.Database.AddCloud(ctx, &dbCloud); err != nil {
		return fail(errors.E(op, err))
	}

	// Create the cloud on a host.
	shuffleRegionControllers(region.Controllers)
	ccloud, err := j.addControllerCloud(ctx, &region.Controllers[0].Controller, u.Tag().(names.UserTag), tag, cloud)
	if err != nil {
		// TODO(mhilton) remove the added cloud if adding it to the controller failed.
		return fail(errors.E(op, err))
	}
	// Update the cloud in the database.
	dbCloud.FromJujuCloud(*ccloud)
	for i := range dbCloud.Regions {
		dbCloud.Regions[i].Controllers = []dbmodel.CloudRegionControllerPriority{{
			ControllerID: region.Controllers[0].ID,
			Priority:     dbmodel.CloudRegionControllerPrioritySupported,
		}}
	}

	if err := j.Database.UpdateCloud(ctx, &dbCloud); err != nil {
		// At this point the cloud has been created on the
		// controller and we know something about it. Trying to
		// undo that will probably make things worse.
		return fail(errors.E(op, err))
	}
	ale.Success = true
	return nil
}

// addControllerCloud creates the hosted cloud defined by the given tag and
// jujuparams cloud definition. Admin access to the cloud will be granted
// to the user identified by the given user tag. On success
// addControllerCloud returns the definition of the cloud retrieved from
// the controller.
func (j *JIMM) addControllerCloud(ctx context.Context, ctl *dbmodel.Controller, ut names.UserTag, tag names.CloudTag, cloud jujuparams.Cloud) (*jujuparams.Cloud, error) {
	const op = errors.Op("jimm.addControllerCloud")

	api, err := j.dial(ctx, ctl, names.ModelTag{})
	if err != nil {
		return nil, errors.E(op, err)
	}
	defer api.Close()
	if err := api.AddCloud(ctx, tag, cloud); err != nil {
		return nil, errors.E(op, err)
	}
	if err := api.GrantCloudAccess(ctx, tag, ut, "admin"); err != nil {
		return nil, errors.E(op, err)
	}
	var result jujuparams.Cloud
	if err := api.Cloud(ctx, tag, &result); err != nil {
		return nil, errors.E(op, err)
	}
	return &result, nil
}

// doCloudAdmin is a simple wrapper that provides the common parts of cloud
// administration commands. doCloudAdmin finds the cloud with the given tag
// and validates that the given user has admin access to the cloud.
// doCloudAdmin then connects to the controller hosting the cloud and calls
// the given function with the cloud and API connection to perform the
// operation specific commands. If the cloud cannot be found then an error
// with the code CodeNotFound is returned. If the given user does not have
// admin access to the cloud then an error with the code CodeUnauthorized
// is returned. If there is an error connecting to the controller hosting
// the cloud then the returned error will have the same code as the error
// returned from the dial operation. If the given function returns an error
// that error will be returned with the code unmasked.
func (j *JIMM) doCloudAdmin(ctx context.Context, u *dbmodel.User, ct names.CloudTag, f func(*dbmodel.Cloud, API) error) error {
	const op = errors.Op("jimm.doCloudAdmin")

	var c dbmodel.Cloud
	c.SetTag(ct)

	if err := j.Database.GetCloud(ctx, &c); err != nil {
		return errors.E(op, err)
	}
	if cloudUserAccess(u, &c) != "admin" {
		// If the user doesn't have admin access on the cloud return
		// an unauthorized error.
		return errors.E(op, errors.CodeUnauthorized, "unauthorized")
	}

	if len(c.Regions) != 1 || len(c.Regions[0].Controllers) != 1 {
		return errors.E(op, "cloud administration not available for %s", ct.Id())
	}

	api, err := j.dial(ctx, &c.Regions[0].Controllers[0].Controller, names.ModelTag{})
	if err != nil {
		return errors.E(op, err)
	}
	defer api.Close()
	if err := f(&c, api); err != nil {
		return errors.E(op, err)
	}
	return nil
}

// GrantCloudAccess grants the given access level on the given cloud to the
// given user. If the cloud is not found then an error with the code
// CodeNotFound is returned. If the authenticated user does not have admin
// access to the cloud then an error with the code CodeUnauthorized is
// returned. If the ModifyCloudAccess API call retuns an error the error
// code is not masked.
func (j *JIMM) GrantCloudAccess(ctx context.Context, u *dbmodel.User, ct names.CloudTag, ut names.UserTag, access string) error {
	const op = errors.Op("jimm.GrantCloudAccess")

	ale := dbmodel.AuditLogEntry{
		Time:    time.Now().UTC().Round(time.Millisecond),
		Tag:     ct.String(),
		UserTag: u.Tag().String(),
		Action:  "grant",
		Params: dbmodel.StringMap{
			"user":   ut.String(),
			"access": access,
		},
	}
	defer j.addAuditLogEntry(&ale)

	err := j.doCloudAdmin(ctx, u, ct, func(c *dbmodel.Cloud, api API) error {
		targetUser := dbmodel.User{
			Username: ut.Id(),
		}
		if err := j.Database.GetUser(ctx, &targetUser); err != nil {
			return err
		}
		if err := api.GrantCloudAccess(ctx, ct, ut, access); err != nil {
			return err
		}
		var uca dbmodel.UserCloudAccess
		for _, a := range c.Users {
			if a.Username == targetUser.Username {
				uca = a
				break
			}
		}
		uca.User = targetUser
		uca.Cloud = *c
		uca.Access = access

		if err := j.Database.UpdateUserCloudAccess(ctx, &uca); err != nil {
			return errors.E(op, err, "cannot update database after updating controller")
		}
		return nil
	})
	if err != nil {
		ale.Params["err"] = err.Error()
		return errors.E(op, err)
	}
	ale.Success = true
	return nil
}

// RevokeCloudAccess revokes the given access level on the given cloud from
// the given user. If the cloud is not found then an error with the code
// CodeNotFound is returned. If the authenticated user does not have admin
// access to the cloud then an error with the code CodeUnauthorized is
// returned. If the ModifyCloudAccess API call retuns an error the error
// code is not masked.
func (j *JIMM) RevokeCloudAccess(ctx context.Context, u *dbmodel.User, ct names.CloudTag, ut names.UserTag, access string) error {
	const op = errors.Op("jimm.RevokeCloudAccess")

	ale := dbmodel.AuditLogEntry{
		Time:    time.Now().UTC().Round(time.Millisecond),
		Tag:     ct.String(),
		UserTag: u.Tag().String(),
		Action:  "revoke",
		Params: dbmodel.StringMap{
			"user":   ut.String(),
			"access": access,
		},
	}
	defer j.addAuditLogEntry(&ale)

	err := j.doCloudAdmin(ctx, u, ct, func(c *dbmodel.Cloud, api API) error {
		targetUser := dbmodel.User{
			Username: ut.Id(),
		}
		if err := j.Database.GetUser(ctx, &targetUser); err != nil {
			return err
		}
		if err := api.RevokeCloudAccess(ctx, ct, ut, access); err != nil {
			return err
		}
		var uca dbmodel.UserCloudAccess
		for _, a := range c.Users {
			if a.Username == targetUser.Username {
				uca = a
				break
			}
		}
		uca.User = targetUser
		uca.Cloud = *c
		switch access {
		case "admin":
			uca.Access = "add-model"
		default:
			uca.Access = ""
		}

		if err := j.Database.UpdateUserCloudAccess(ctx, &uca); err != nil {
			return errors.E(op, err, "cannot update database after updating controller")
		}
		return nil
	})
	if err != nil {
		ale.Params["err"] = err.Error()
		return errors.E(op, err)
	}
	ale.Success = true
	return nil
}

// RemoveCloud removes the given cloud from JAAS If the cloud is not found
// then an error with the code CodeNotFound is returned. If the
// authenticated user does not have admin access to the cloud then an error
// with the code CodeUnauthorized is returned. If the REmoveClouds API call
// retuns an error the error code is not masked.
func (j *JIMM) RemoveCloud(ctx context.Context, u *dbmodel.User, ct names.CloudTag) error {
	const op = errors.Op("jimm.RemoveCloud")

	ale := dbmodel.AuditLogEntry{
		Time:    time.Now().UTC().Round(time.Millisecond),
		Tag:     ct.String(),
		UserTag: u.Tag().String(),
		Action:  "remove",
		Params:  dbmodel.StringMap{},
	}
	defer j.addAuditLogEntry(&ale)

	err := j.doCloudAdmin(ctx, u, ct, func(c *dbmodel.Cloud, api API) error {
		// Note: JIMM doesn't attempt to determine if the cloud is
		// used by any models before attempting to remove it. JIMM
		// relies on the controller failing the RemoveClouds API
		// request if the cloud is in use.
		if err := api.RemoveCloud(ctx, ct); err != nil {
			return err
		}

		if err := j.Database.DeleteCloud(ctx, c); err != nil {
			return errors.E(op, err, "cannot update database after updating controller")
		}
		return nil
	})
	if err != nil {
		ale.Params["err"] = err.Error()
		return errors.E(op, err)
	}
	ale.Success = true
	return nil
}

// UpdateCloud updates the cloud with the given name on all controllers
// that host the cloud. If the given user is not a controller superuser or
// an admin on the cloud an error is returned with a code of
// CodeUnauthorized. If the cloud with the given name cannot be found then
// an error with the code CodeNotFound is returned.
func (j *JIMM) UpdateCloud(ctx context.Context, u *dbmodel.User, ct names.CloudTag, cloud jujuparams.Cloud) error {
	const op = errors.Op("jimm.UpdateCloud")

	ale := dbmodel.AuditLogEntry{
		Time:    time.Now().UTC().Round(time.Millisecond),
		Tag:     ct.String(),
		UserTag: u.Tag().String(),
		Action:  "update",
		Params:  dbmodel.StringMap{},
	}
	fail := func(err error) error {
		ale.Params["err"] = err.Error()
		return err
	}

	var c dbmodel.Cloud
	c.SetTag(ct)

	if err := j.Database.GetCloud(ctx, &c); err != nil {
		return fail(errors.E(op, err))
	}
	if cloudUserAccess(u, &c) != "admin" {
		// If the user doesn't have admin access on the cloud return
		// an unauthorized error.
		return fail(errors.E(op, errors.CodeUnauthorized, "unauthorized"))
	}

	var controllers []dbmodel.Controller
	seen := make(map[uint]bool)
	for _, r := range c.Regions {
		for _, ctl := range r.Controllers {
			if seen[ctl.ControllerID] {
				continue
			}
			seen[ctl.ControllerID] = true
			controllers = append(controllers, ctl.Controller)
		}
	}

	err := j.forEachController(ctx, controllers, func(ctl *dbmodel.Controller, api API) error {
		return api.UpdateCloud(ctx, ct, cloud)
	})
	if err != nil {
		return fail(errors.E(op, err))
	}

	// Update the local database with the updated cloud definition. We
	// do this in a transaction so that the local view cannot finish in
	// an inconsistent state.
	err = j.Database.Transaction(func(db *db.Database) error {

		var c dbmodel.Cloud
		c.SetTag(ct)
		if err := db.GetCloud(ctx, &c); err != nil {
			return err
		}
		c.FromJujuCloud(cloud)
		for i := range c.Regions {
			if len(c.Regions[i].Controllers) == 0 {
				for _, ctl := range controllers {
					c.Regions[i].Controllers = append(c.Regions[i].Controllers, dbmodel.CloudRegionControllerPriority{
						Controller: ctl,
						Priority:   dbmodel.CloudRegionControllerPrioritySupported,
					})
				}
			}
		}
		return db.UpdateCloud(ctx, &c)
	})

	if err != nil {
		return fail(errors.E(op, err))
	}
	ale.Success = true
	return nil
}
