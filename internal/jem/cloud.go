// Copyright 2015 Canonical Ltd.

package jem

import (
	"context"
	"strings"

	jujuparams "github.com/juju/juju/apiserver/params"
	"go.uber.org/zap"
	"gopkg.in/errgo.v1"
	"gopkg.in/macaroon-bakery.v2/bakery/identchecker"
	"gopkg.in/mgo.v2/bson"

	"github.com/CanonicalLtd/jimm/internal/apiconn"
	"github.com/CanonicalLtd/jimm/internal/auth"
	"github.com/CanonicalLtd/jimm/internal/conv"
	"github.com/CanonicalLtd/jimm/internal/mongodoc"
	"github.com/CanonicalLtd/jimm/internal/zapctx"
	"github.com/CanonicalLtd/jimm/internal/zaputil"
	"github.com/CanonicalLtd/jimm/params"
)

// A CreateCloudParams is used to pass additional cloud information to
// CreateCloud.
type CreateCloudParams struct {
	HostCloudRegion string
	Config          map[string]interface{}
	RegionConfig    map[string]map[string]interface{}
}

// AddCloud creates a new cloud in the database and adds it to a
// controller. If the cloud being added is not a supported CAAS cloud
// (currently only kubernetes) then an error with a cause of
// params.ErrIncompatibleClouds will be returned. If the cloud name already
// exists then an error with a cause of params.ErrAlreadyExists will be
// returned.
func (j *JEM) AddCloud(ctx context.Context, id identchecker.ACLIdentity, name params.Cloud, cloud jujuparams.Cloud) (err error) {
	if cloud.Type != "kubernetes" {
		return errgo.WithCausef(nil, params.ErrIncompatibleClouds, "clouds of type %q cannot be added to JAAS", cloud.Type)
	}

	parts := strings.SplitN(cloud.HostCloudRegion, "/", 2)
	if len(parts) != 2 || parts[0] == "" {
		return errgo.WithCausef(nil, params.ErrCloudRegionRequired, "")
	}

	if _, ok := j.pool.config.PublicCloudMetadata[string(name)]; ok {
		// The cloud uses the name of a public cloud, we assume
		// these already exist (even if they don't yet).
		return errgo.WithCausef(nil, params.ErrAlreadyExists, "cloud %q already exists", name)
	}

	// Create a placeholder cloud to reserve the cloud name.
	cloudDoc := mongodoc.CloudRegion{
		Cloud:        name,
		ProviderType: cloud.Type,
		ACL: params.ACL{
			Read:  []string{id.Id()},
			Write: []string{id.Id()},
			Admin: []string{id.Id()},
		},
	}

	// Attempt to insert the document for the cloud and fail early if
	// such a cloud exists.
	if err := j.DB.InsertCloudRegion(ctx, &cloudDoc); err != nil {
		if errgo.Cause(err) == params.ErrAlreadyExists {
			return errgo.WithCausef(nil, params.ErrAlreadyExists, "cloud %q already exists", name)
		}
		return errgo.Mask(err)
	}

	ctlPath, err := j.addCloud(ctx, id, name, cloud, parts[0], parts[1])
	if err != nil {
		if dberr := j.DB.RemoveCloudRegion(ctx, name, ""); dberr != nil {
			zapctx.Warn(ctx, "cannot remove cloud that failed to deploy", zaputil.Error(dberr), zap.String("cloud", string(name)))
		}
		return errgo.Mask(err,
			errgo.Is(params.ErrCloudRegionRequired),
			errgo.Is(params.ErrNotFound),
			errgo.Is(params.ErrUnauthorized),
			apiconn.IsAPIError,
		)
	}

	// Get the new cloud's definition from the controller.
	conn, err := j.OpenAPI(ctx, ctlPath)
	if err != nil {
		return errgo.Mask(err)
	}
	if err := conn.Cloud(ctx, name, &cloud); err != nil {
		return errgo.Mask(err)
	}

	// Ensure the creating user is an admin on the cloud.
	if err := conn.GrantCloudAccess(ctx, name, params.User(id.Id()), "admin"); err != nil {
		return errgo.Mask(err)
	}

	regions := conv.FromCloud(name, cloud)
	for i := range regions {
		regions[i].PrimaryControllers = []params.EntityPath{ctlPath}
		regions[i].ACL = params.ACL{
			Read:  []string{id.Id()},
			Write: []string{id.Id()},
			Admin: []string{id.Id()},
		}
	}
	if err := j.DB.UpdateCloudRegions(ctx, regions); err != nil {
		return errgo.Mask(err)
	}
	j.DB.AppendAudit(ctx, &params.AuditCloudCreated{
		ID:    regions[0].Id,
		Cloud: string(name),
	})
	return nil
}

func (j *JEM) addCloud(ctx context.Context, id identchecker.ACLIdentity, name params.Cloud, cloud jujuparams.Cloud, hostCloud string, hostRegion string) (params.EntityPath, error) {
	ctlPaths, err := j.possibleControllers(ctx, id, params.EntityPath{}, params.Cloud(hostCloud), hostRegion)
	if err != nil {
		return params.EntityPath{}, errgo.Mask(err, errgo.Is(params.ErrNotFound), errgo.Is(params.ErrUnauthorized))
	}
	errors := make([]error, len(ctlPaths))
	for i, ctlPath := range ctlPaths {
		conn, err := j.OpenAPI(ctx, ctlPath)
		if err != nil {
			errors[i] = err
			zapctx.Error(ctx, "cannot connect to controller", zap.Stringer("controller", ctlPath), zaputil.Error(err))
			continue
		}
		defer conn.Close()

		err = conn.AddCloud(ctx, name, cloud)
		if err == nil {
			return ctlPath, nil
		}
		zapctx.Error(ctx, "cannot create cloud", zap.Stringer("controller", ctlPath), zaputil.Error(err))
		errors[i] = err
		continue
	}
	if len(errors) > 0 {
		// TODO(mhilton) perhaps filter errors to find the "best" one.
		return params.EntityPath{}, errgo.Mask(errors[0], apiconn.IsAPIError)
	}
	return params.EntityPath{}, errgo.New("cannot create cloud")
}

// RemoveCloud removes the given cloud, so long as no models are using it.
func (j *JEM) RemoveCloud(ctx context.Context, id identchecker.ACLIdentity, cloud params.Cloud) (err error) {
	cr, err := j.DB.CloudRegion(auth.ContextWithIdentity(ctx, id), cloud, "")
	if err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrNotFound), errgo.Is(params.ErrUnauthorized))
	}
	if err := auth.CheckACL(ctx, id, cr.ACL.Admin); err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrUnauthorized))
	}
	// This check is technically redundant as we can't know whether
	// the cloud is in use by any models at the moment we remove it from a controller
	// (remember that only one of the primary controllers might be using it).
	// However we like the error message and it's usually going to be OK,
	// so we'll do the advance check anyway.
	if n, err := j.DB.Models().Find(bson.D{{"cloud", cloud}}).Count(); n > 0 || err != nil {
		if err != nil {
			return errgo.Mask(err)
		}
		return errgo.Newf("cloud is used by %d model%s", n, plural(n))
	}
	// TODO delete the cloud from the controllers in parallel
	// (although currently there is only ever one anyway).
	for _, ctl := range cr.PrimaryControllers {
		conn, err := j.OpenAPI(ctx, ctl)
		if err != nil {
			return errgo.Mask(err)
		}
		defer conn.Close()
		if err := conn.RemoveCloud(ctx, cloud); err != nil {
			return errgo.Notef(err, "cannot remove cloud from controller %s", ctl)
		}
	}
	if err := j.DB.RemoveCloud(ctx, cloud); err != nil {
		return errgo.Mask(err)
	}
	j.DB.AppendAudit(ctx, &params.AuditCloudRemoved{
		ID:     cr.Id,
		Cloud:  string(cr.Cloud),
		Region: cr.Region,
	})
	return nil
}

// GrantCloud grants access to the given cloud at the given access level to the given user.
func (j *JEM) GrantCloud(ctx context.Context, id identchecker.ACLIdentity, cloud params.Cloud, user params.User, access string) error {
	cr, err := j.DB.CloudRegion(ctx, cloud, "")
	if err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrNotFound))
	}
	if err := auth.CheckACL(ctx, id, cr.ACL.Admin); err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrUnauthorized))
	}
	if err := j.DB.GrantCloud(ctx, cloud, user, access); err != nil {
		return errgo.Mask(err)
	}
	// TODO grant the cloud access on the controllers in parallel
	// (although currently there is only ever one anyway).
	for _, ctl := range cr.PrimaryControllers {
		conn, err := j.OpenAPI(ctx, ctl)
		if err != nil {
			return errgo.Mask(err)
		}
		defer conn.Close()
		if err := conn.GrantCloudAccess(ctx, cloud, user, access); err != nil {
			// TODO(mhilton) If this happens then the
			// controller will be in an inconsistent state
			// with JIMM. Try and resolve this.
			return errgo.Mask(err, apiconn.IsAPIError)
		}
	}
	return nil
}

// RevokeCloud revokes access to the given cloud at the given access level from the given user.
func (j *JEM) RevokeCloud(ctx context.Context, id identchecker.ACLIdentity, cloud params.Cloud, user params.User, access string) error {
	cr, err := j.DB.CloudRegion(ctx, cloud, "")
	if err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrNotFound))
	}
	if err := auth.CheckACL(ctx, id, cr.ACL.Admin); err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrUnauthorized))
	}
	// TODO revoke the cloud access on the controllers in parallel
	// (although currently there is only ever one anyway).
	for _, ctl := range cr.PrimaryControllers {
		conn, err := j.OpenAPI(ctx, ctl)
		if err != nil {
			return errgo.Mask(err)
		}
		defer conn.Close()
		if err := conn.RevokeCloudAccess(ctx, cloud, user, access); err != nil {
			return errgo.Mask(err, apiconn.IsAPIError)
		}
	}
	if err := j.DB.RevokeCloud(ctx, cloud, user, access); err != nil {
		return errgo.Mask(err)
	}
	return nil
}
