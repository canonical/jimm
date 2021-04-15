// Copyright 2020 Canonical Ltd.

package jimm

import (
	"context"
	"fmt"
	"strings"
	"time"

	jujuparams "github.com/juju/juju/apiserver/params"
	"github.com/juju/names/v4"
	"github.com/juju/version"
	"go.uber.org/zap"

	"github.com/CanonicalLtd/jimm/internal/dbmodel"
	"github.com/CanonicalLtd/jimm/internal/errors"
	"github.com/CanonicalLtd/jimm/internal/zapctx"
)

// AddController adds the specified controller to JIMM. Only
// controller-admin level users may add new controllers. If the user adding
// the controller is not authorized then an error with a code of
// CodeUnauthorized will be returned. If there already exists a controller
// with the same name as the controller being added then an error with a
// code of CodeAlreadyExists will be returned. If the controller cannot be
// contacted then an error with a code of CodeConnectionFailed will be
// returned.
func (j *JIMM) AddController(ctx context.Context, u *dbmodel.User, ctl *dbmodel.Controller) error {
	const op = errors.Op("jimm.AddController")

	ale := dbmodel.AuditLogEntry{
		Time:    time.Now().UTC().Round(time.Millisecond),
		UserTag: u.Tag().String(),
		Action:  "add",
		Params: dbmodel.StringMap{
			"name": ctl.Name,
		},
	}
	defer j.addAuditLogEntry(&ale)

	fail := func(err error) error {
		ale.Params["err"] = err.Error()
		return err
	}

	if u.ControllerAccess != "superuser" {
		return fail(errors.E(op, errors.CodeUnauthorized, "cannot add controller"))
	}

	api, err := j.dial(ctx, ctl, names.ModelTag{})
	if err != nil {
		return fail(errors.E(op, err))
	}
	defer api.Close()
	ale.Tag = names.NewControllerTag(ctl.UUID).String()

	var ms jujuparams.ModelSummary
	if err := api.ControllerModelSummary(ctx, &ms); err != nil {
		return fail(errors.E(op, err))
	}
	// TODO(mhilton) add the controller model?

	clouds, err := api.Clouds(ctx)
	if err != nil {
		return fail(errors.E(op, err))
	}

	for tag, cld := range clouds {
		ctx := zapctx.WithFields(ctx, zap.Stringer("tag", tag))
		cloud := dbmodel.Cloud{
			Name:             tag.Id(),
			Type:             cld.Type,
			HostCloudRegion:  cld.HostCloudRegion,
			AuthTypes:        dbmodel.Strings(cld.AuthTypes),
			Endpoint:         cld.Endpoint,
			IdentityEndpoint: cld.IdentityEndpoint,
			StorageEndpoint:  cld.StorageEndpoint,
			CACertificates:   dbmodel.Strings(cld.CACertificates),
			Config:           dbmodel.Map(cld.Config),
			Users: []dbmodel.UserCloudAccess{{
				User: dbmodel.User{
					// "everyone@external" represents all authenticated
					// users.
					Username:    "everyone@external",
					DisplayName: "everyone",
				},
				Access: "add-model",
			}},
		}
		// If this cloud is not the one used by the controller model then
		// it is only available to a subset of users.
		if tag.String() != ms.CloudTag {
			var err error
			cloud.Users, err = cloudUsers(ctx, api, tag)
			if err != nil {
				// If there is an error getting the users, log the failure
				// but carry on, this will prevent anyone trying to add a
				// cloud with the same name. The user access can be fixed
				// later.
				zapctx.Error(ctx, "cannot get cloud users", zap.Error(err))
			}
		}

		for _, r := range cld.Regions {
			cr := dbmodel.CloudRegion{
				CloudName:        cloud.Name,
				Name:             r.Name,
				Endpoint:         r.Endpoint,
				IdentityEndpoint: r.IdentityEndpoint,
				StorageEndpoint:  r.StorageEndpoint,
				Config:           dbmodel.Map(cld.RegionConfig[r.Name]),
			}

			cloud.Regions = append(cloud.Regions, cr)
		}

		if err := j.Database.SetCloud(ctx, &cloud); err != nil {
			return fail(errors.E(op, errors.Code(""), fmt.Sprintf("cannot load controller cloud %q", cloud.Name), err))
		}

		for _, cr := range cloud.Regions {
			priority := dbmodel.CloudRegionControllerPrioritySupported
			if tag.String() == ms.CloudTag && cr.Name == ms.CloudRegion {
				priority = dbmodel.CloudRegionControllerPriorityDeployed
			}
			ctl.CloudRegions = append(ctl.CloudRegions, dbmodel.CloudRegionControllerPriority{
				CloudRegionID: cr.ID,
				Priority:      uint(priority),
			})
		}
	}

	if err := j.Database.AddController(ctx, ctl); err != nil {
		if errors.ErrorCode(err) == errors.CodeAlreadyExists {
			return fail(errors.E(op, err, fmt.Sprintf("controller %q already exists", ctl.Name)))
		}
		return fail(errors.E(op, err))
	}
	ale.Success = true
	return nil
}

// cloudUsers determines the users that can access a cloud.
func cloudUsers(ctx context.Context, api API, tag names.CloudTag) ([]dbmodel.UserCloudAccess, error) {
	const op = errors.Op("jimm.cloudUsers")
	var ci jujuparams.CloudInfo
	if err := api.CloudInfo(ctx, tag, &ci); err != nil {
		return nil, errors.E(op, err)
	}
	var users []dbmodel.UserCloudAccess
	for _, u := range ci.Users {
		if strings.Index(u.UserName, "@") < 0 {
			// If the username doesn't contain an "@" the user is local
			// to the controller and we don't want to propagate it.
			continue
		}
		users = append(users, dbmodel.UserCloudAccess{
			User: dbmodel.User{
				Username:    u.UserName,
				DisplayName: u.DisplayName,
			},
			Access: u.Access,
		})
	}
	return users, nil
}

// EarliestControllerVersion returns the earliest agent version
// that any of the available public controllers is known to be running.
// If there are no available controllers or none of their versions are
// known, it returns the zero version.
func (j *JIMM) EarliestControllerVersion(ctx context.Context) (version.Number, error) {
	const op = errors.Op("jimm.EarliestControllerVersion")
	var v *version.Number

	err := j.Database.ForEachController(ctx, func(controller *dbmodel.Controller) error {
		if controller.AgentVersion == "" {
			return nil
		}
		versionNumber, err := version.Parse(controller.AgentVersion)
		if err != nil {
			zapctx.Error(
				ctx,
				"failed to parse agent version",
				zap.String("version", controller.AgentVersion),
				zap.String("controller", controller.Name),
			)
			return nil
		}
		if v == nil || versionNumber.Compare(*v) < 0 {
			v = &versionNumber
		}
		return nil
	})
	if err != nil {
		return version.Number{}, errors.E(op, err)
	}
	if v == nil {
		return version.Number{}, nil
	}
	return *v, nil
}

// controllerAccessLevel holds the controller access level for a user.
type controllerAccessLevel string

const (
	// noAccess allows a user no permissions at all.
	noAccess controllerAccessLevel = ""

	// loginAccess allows a user to log-ing into the subject.
	loginAccess controllerAccessLevel = "login"

	// addModelAccess allows user to add new models in subjects supporting it.
	addModelAccess controllerAccessLevel = "add-model"

	// superuserAccess allows user unrestricted permissions in the subject.
	superuserAccess controllerAccessLevel = "superuser"
)

// validate returns error if the current is not a valid access level.
func (a controllerAccessLevel) validate() error {
	switch a {
	case noAccess, loginAccess, addModelAccess, superuserAccess:
		return nil
	}
	return errors.E(fmt.Sprintf("invalid access level %q", a))
}

func (a controllerAccessLevel) value() int {
	switch a {
	case noAccess:
		return 0
	case loginAccess:
		return 1
	case addModelAccess:
		return 2
	case superuserAccess:
		return 3
	default:
		return -1
	}
}

// GrantControllerAccess changes the controller access granted to users.
func (j *JIMM) GrantControllerAccess(ctx context.Context, u *dbmodel.User, accessUserTag names.UserTag, accessLevel string) error {
	const op = errors.Op("jimm.GrantControllerAccess")

	ale := dbmodel.AuditLogEntry{
		Time:    time.Now().UTC().Round(time.Millisecond),
		UserTag: u.Tag().String(),
		Action:  "grant",
		Params: dbmodel.StringMap{
			"access":     accessLevel,
			"controller": names.NewControllerTag(j.UUID).String(),
			"user":       accessUserTag.String(),
		},
	}
	defer j.addAuditLogEntry(&ale)

	fail := func(err error) error {
		ale.Params["err"] = err.Error()
		return err
	}

	if u.ControllerAccess != "superuser" {
		return fail(errors.E(op, errors.CodeUnauthorized, "cannot grant controller access"))
	}

	if err := controllerAccessLevel(accessLevel).validate(); err != nil {
		return fail(errors.E(op, err, errors.CodeBadRequest))
	}

	user := dbmodel.User{
		Username: accessUserTag.Id(),
	}
	err := j.Database.GetUser(ctx, &user)
	if err != nil {
		return fail(errors.E(op, err))
	}

	// if user's access level is already greater than what we are trying to set
	// there is nothing to do
	if controllerAccessLevel(user.ControllerAccess).value() >= controllerAccessLevel(accessLevel).value() {
		return nil
	}
	user.ControllerAccess = string(accessLevel)
	err = j.Database.UpdateUser(ctx, &user)
	if err != nil {
		return errors.E(op, err)
	}
	return nil
}

// RevokeControllerAccess revokes the controller access for a users.
func (j *JIMM) RevokeControllerAccess(ctx context.Context, u *dbmodel.User, accessUserTag names.UserTag, accessLevel string) error {
	const op = errors.Op("jimm.RevokeControllerAccess")

	ale := dbmodel.AuditLogEntry{
		Time:    time.Now().UTC().Round(time.Millisecond),
		UserTag: u.Tag().String(),
		Action:  "revoke",
		Params: dbmodel.StringMap{
			"access":     accessLevel,
			"controller": names.NewControllerTag(j.UUID).String(),
			"user":       accessUserTag.String(),
		},
	}
	defer j.addAuditLogEntry(&ale)

	fail := func(err error) error {
		ale.Params["err"] = err.Error()
		return err
	}

	if u.ControllerAccess != "superuser" {
		return fail(errors.E(op, errors.CodeUnauthorized, "cannot revoke controller access"))
	}

	if err := controllerAccessLevel(accessLevel).validate(); err != nil {
		return fail(errors.E(op, err, errors.CodeBadRequest))
	}
	userAccess := noAccess
	switch controllerAccessLevel(accessLevel) {
	case loginAccess:
	case addModelAccess:
		userAccess = loginAccess
	case superuserAccess:
		userAccess = addModelAccess
	}

	user := dbmodel.User{
		Username: accessUserTag.Id(),
	}
	err := j.Database.GetUser(ctx, &user)
	if err != nil {
		return fail(errors.E(op, err))
	}
	user.ControllerAccess = string(userAccess)
	err = j.Database.UpdateUser(ctx, &user)
	if err != nil {
		return errors.E(op, err)
	}
	return nil
}

// ImportModel imports model with the specified uuid from the controller.
func (j *JIMM) ImportModel(ctx context.Context, u *dbmodel.User, controllerName string, modelTag names.ModelTag) error {
	const op = errors.Op("jimm.ImportModel")

	ale := dbmodel.AuditLogEntry{
		Time:    time.Now().UTC().Round(time.Millisecond),
		UserTag: u.Tag().String(),
		Action:  "import",
		Tag:     modelTag.String(),
		Params: dbmodel.StringMap{
			"controller": controllerName,
			"model":      modelTag.String(),
		},
	}
	defer j.addAuditLogEntry(&ale)

	fail := func(err error) error {
		ale.Params["err"] = err.Error()
		return err
	}

	if u.ControllerAccess != "superuser" {
		return fail(errors.E(op, errors.CodeUnauthorized, "cannot import model"))
	}

	controller := dbmodel.Controller{
		Name: controllerName,
	}
	err := j.Database.GetController(ctx, &controller)
	if err != nil {
		return errors.E(op, err)
	}

	api, err := j.dial(ctx, &controller, names.ModelTag{})
	if err != nil {
		return errors.E(op, err)
	}
	defer api.Close()

	modelInfo := jujuparams.ModelInfo{
		UUID: modelTag.Id(),
	}
	err = api.ModelInfo(ctx, &modelInfo)
	if err != nil {
		return errors.E(op, err)
	}

	model := dbmodel.Model{}
	// fill in data from model info
	err = model.FromJujuModelInfo(&modelInfo)
	if err != nil {
		return errors.E(op, err)
	}
	model.ControllerID = controller.ID
	model.Controller = controller

	// fetch the model owner user
	ownerTag, err := names.ParseUserTag(modelInfo.OwnerTag)
	if err != nil {
		return errors.E(op, err)
	}
	owner := dbmodel.User{}
	owner.SetTag(ownerTag)
	err = j.Database.GetUser(ctx, &owner)
	if err != nil {
		return errors.E(op, err)
	}
	model.OwnerUsername = owner.Username
	model.Owner = owner

	// fetch cloud credential used by the model
	credentialTag, err := names.ParseCloudCredentialTag(modelInfo.CloudCredentialTag)
	if err != nil {
		return errors.E(op, err)
	}
	cloudCredential := dbmodel.CloudCredential{}
	cloudCredential.SetTag(credentialTag)
	err = j.Database.GetCloudCredential(ctx, &cloudCredential)
	if err != nil {
		return errors.E(op, err)
	}
	model.CloudCredentialID = cloudCredential.ID
	model.CloudCredential = cloudCredential

	// fetch the cloud used by the model
	cloud := dbmodel.Cloud{
		Name: cloudCredential.CloudName,
	}
	err = j.Database.GetCloud(ctx, &cloud)
	if err != nil {
		return errors.E(op, err)
	}

	regionFound := false
	for _, cr := range cloud.Regions {
		if cr.Name == modelInfo.CloudRegion {
			regionFound = true
			model.CloudRegion = cr
			model.CloudRegionID = cr.ID
			break
		}
	}
	if !regionFound {
		return errors.E(op, "cloud region not found")
	}

	for i, userAccess := range model.Users {
		u := userAccess.User
		err = j.Database.GetUser(ctx, &u)
		if err != nil {
			return errors.E(op, err)
		}
		model.Users[i].Username = u.Username
		model.Users[i].User = u
	}

	err = j.Database.AddModel(ctx, &model)
	if err != nil {
		if errors.ErrorCode(err) == errors.CodeAlreadyExists {
			return errors.E(op, err, "model already exists")
		}
		return errors.E(op, err)
	}

	return nil
}
