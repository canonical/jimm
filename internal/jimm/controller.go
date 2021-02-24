// Copyright 2020 Canonical Ltd.

package jimm

import (
	"context"
	"fmt"
	"strings"

	jujuparams "github.com/juju/juju/apiserver/params"
	"github.com/juju/names/v4"
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
	if u.ControllerAccess != "superuser" {
		return errors.E(op, errors.CodeUnauthorized, "cannot add controller")
	}

	api, err := j.dial(ctx, ctl, names.ModelTag{})
	if err != nil {
		return errors.E(op, err)
	}
	defer api.Close()

	var ms jujuparams.ModelSummary
	if err := api.ControllerModelSummary(ctx, &ms); err != nil {
		return errors.E(op, err)
	}
	// TODO(mhilton) add the controller model?

	clouds, err := api.Clouds(ctx)
	if err != nil {
		return errors.E(op, err)
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
			return errors.E(op, errors.Code(""), fmt.Sprintf("cannot load controller cloud %q", cloud.Name), err)
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
			return errors.E(op, err, fmt.Sprintf("controller %q already exists", ctl.Name))
		}
		return errors.E(op, err)
	}
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
