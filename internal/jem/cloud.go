// Copyright 2015 Canonical Ltd.

package jem

import (
	"context"
	"strings"

	cloudapi "github.com/juju/juju/api/cloud"
	jujucloud "github.com/juju/juju/cloud"
	"go.uber.org/zap"
	"gopkg.in/errgo.v1"
	"gopkg.in/mgo.v2/bson"

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

// CreateCloud creates a new cloud in the database and adds it to a
// controller. If the cloud name already exists then an error with a
// cause of params.ErrAlreadyExists will be returned.
func (j *JEM) CreateCloud(ctx context.Context, cloud mongodoc.CloudRegion, regions []mongodoc.CloudRegion, ccp CreateCloudParams) error {
	if _, ok := j.pool.config.PublicCloudMetadata[string(cloud.Cloud)]; ok {
		// The cloud uses the name of a public cloud, we assume
		// these already exist (even if they don't yet).
		return errgo.WithCausef(nil, params.ErrAlreadyExists, "cloud %q already exists", cloud.Cloud)
	}

	// Attempt to insert the document for the cloud and fail early if
	// such a cloud exists.
	if err := j.DB.InsertCloudRegion(ctx, &cloud); err != nil {
		if errgo.Cause(err) == params.ErrAlreadyExists {
			return errgo.WithCausef(nil, params.ErrAlreadyExists, "cloud %q already exists", cloud.Cloud)
		}
		return errgo.Mask(err)
	}
	var regionConfig map[string]jujucloud.Attrs
	for r, attr := range ccp.RegionConfig {
		if regionConfig == nil {
			regionConfig = make(map[string]jujucloud.Attrs)
		}
		regionConfig[r] = attr
	}
	jcloud := jujucloud.Cloud{
		Name:             string(cloud.Cloud),
		Type:             cloud.ProviderType,
		Endpoint:         cloud.Endpoint,
		IdentityEndpoint: cloud.IdentityEndpoint,
		StorageEndpoint:  cloud.StorageEndpoint,
		CACertificates:   cloud.CACertificates,
		HostCloudRegion:  ccp.HostCloudRegion,
		Config:           ccp.Config,
		RegionConfig:     regionConfig,
	}
	for _, authType := range cloud.AuthTypes {
		jcloud.AuthTypes = append(jcloud.AuthTypes, jujucloud.AuthType(authType))
	}
	for _, reg := range regions {
		jcloud.Regions = append(jcloud.Regions, jujucloud.Region{
			Name:             reg.Region,
			Endpoint:         reg.Endpoint,
			IdentityEndpoint: reg.IdentityEndpoint,
			StorageEndpoint:  reg.StorageEndpoint,
		})
	}
	ctlPath, err := j.createCloud(ctx, jcloud)
	if err != nil {
		if err := j.DB.RemoveCloudRegion(ctx, cloud.Cloud, ""); err != nil {
			zapctx.Warn(ctx, "cannot remove cloud that failed to deploy", zaputil.Error(err))
		}
		return errgo.Mask(err, errgo.Is(params.ErrCloudRegionRequired), errgo.Is(params.ErrNotFound), errgo.Is(params.ErrUnauthorized))
	}
	cloud.PrimaryControllers = []params.EntityPath{ctlPath}
	for i := range regions {
		regions[i].PrimaryControllers = []params.EntityPath{ctlPath}
	}
	if err := j.DB.UpdateCloudRegions(ctx, append(regions, cloud)); err != nil {
		return errgo.Mask(err)
	}
	j.DB.AppendAudit(ctx, &params.AuditCloudCreated{
		ID:     cloud.Id,
		Cloud:  string(cloud.Cloud),
		Region: cloud.Region,
	})
	return nil
}

func (j *JEM) createCloud(ctx context.Context, cloud jujucloud.Cloud) (params.EntityPath, error) {
	parts := strings.SplitN(cloud.HostCloudRegion, "/", 2)
	if len(parts) != 2 || parts[0] == "" {
		return params.EntityPath{}, errgo.WithCausef(nil, params.ErrCloudRegionRequired, "")
	}

	ctlPaths, err := j.possibleControllers(ctx, params.EntityPath{}, params.Cloud(parts[0]), parts[1])
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
		// TODO(mhilton) support force?
		err = cloudapi.NewClient(conn).AddCloud(cloud, false)
		if err == nil {
			return ctlPath, nil
		}
		zapctx.Error(ctx, "cannot create cloud", zap.Stringer("controller", ctlPath), zaputil.Error(err))
		errors[i] = err
		continue
	}
	if len(errors) > 0 {
		// TODO(mhilton) perhaps filter errors to find the "best" one.
		return params.EntityPath{}, errgo.Mask(errors[0])
	}
	return params.EntityPath{}, errgo.New("cannot create cloud")
}

// RemoveCloud removes the given cloud, so long as no models are using it.
func (j *JEM) RemoveCloud(ctx context.Context, cloud params.Cloud) (err error) {
	cr, err := j.DB.CloudRegion(ctx, cloud, "")
	if err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrNotFound), errgo.Is(params.ErrUnauthorized))
	}
	if err := auth.CheckACL(ctx, auth.IdentityFromContext(ctx), cr.ACL.Admin); err != nil {
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
		if err := cloudapi.NewClient(conn).RemoveCloud(string(cloud)); err != nil {
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
func (j *JEM) GrantCloud(ctx context.Context, cloud params.Cloud, user params.User, access string) error {
	cr, err := j.DB.CloudRegion(ctx, cloud, "")
	if err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrNotFound))
	}
	if err := auth.CheckACL(ctx, auth.IdentityFromContext(ctx), cr.ACL.Admin); err != nil {
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
		if err := cloudapi.NewClient(conn).GrantCloud(conv.ToUserTag(user).String(), access, string(cloud)); err != nil {
			// TODO(mhilton) If this happens then the
			// controller will be in an inconsistent state
			// with JIMM. Try and resolve this.
			return errgo.Mask(err)
		}
	}
	return nil
}

// RevokeCloud revokes access to the given cloud at the given access level from the given user.
func (j *JEM) RevokeCloud(ctx context.Context, cloud params.Cloud, user params.User, access string) error {
	cr, err := j.DB.CloudRegion(ctx, cloud, "")
	if err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrNotFound))
	}
	if err := auth.CheckACL(ctx, auth.IdentityFromContext(ctx), cr.ACL.Admin); err != nil {
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
		if err := cloudapi.NewClient(conn).RevokeCloud(conv.ToUserTag(user).String(), access, string(cloud)); err != nil {
			return errgo.Mask(err)
		}
	}
	if err := j.DB.RevokeCloud(ctx, cloud, user, access); err != nil {
		return errgo.Mask(err)
	}
	return nil
}
