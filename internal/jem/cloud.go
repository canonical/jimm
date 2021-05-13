// Copyright 2015 Canonical Ltd.

package jem

import (
	"context"
	"strings"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery/identchecker"
	jujuparams "github.com/juju/juju/apiserver/params"
	"go.uber.org/zap"
	"gopkg.in/errgo.v1"

	"github.com/CanonicalLtd/jimm/internal/apiconn"
	"github.com/CanonicalLtd/jimm/internal/auth"
	"github.com/CanonicalLtd/jimm/internal/conv"
	"github.com/CanonicalLtd/jimm/internal/jem/jimmdb"
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

// AddHostedCloud creates a new cloud in the database and adds it to a
// controller. If the cloud being added is not a supported CAAS cloud
// (currently only kubernetes) then an error with a cause of
// params.ErrIncompatibleClouds will be returned. If the cloud name already
// exists then an error with a cause of params.ErrAlreadyExists will be
// returned.
func (j *JEM) AddHostedCloud(ctx context.Context, id identchecker.ACLIdentity, name params.Cloud, cloud jujuparams.Cloud) (err error) {
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

	// Check that the cloud can potentially be hosted.
	controllers, err := j.possibleControllers(ctx, id, params.EntityPath{}, &mongodoc.CloudRegion{
		ProviderType: parts[0],
		Region:       parts[1],
	})
	if err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrNotFound), errgo.Is(params.ErrUnauthorized))
	}

	// Create a placeholder cloud to reserve the cloud name.
	acl := params.ACL{
		Read:  []string{id.Id()},
		Write: []string{id.Id()},
		Admin: []string{id.Id()},
	}
	cloudDoc := mongodoc.CloudRegion{
		Cloud:        name,
		ProviderType: cloud.Type,
		ACL:          acl,
	}
	// Attempt to insert the document for the cloud and fail early if such
	// a cloud exists.
	if err := j.DB.InsertCloudRegion(ctx, &cloudDoc); err != nil {
		if errgo.Cause(err) == params.ErrAlreadyExists {
			return errgo.WithCausef(nil, params.ErrAlreadyExists, "cloud %q already exists", name)
		}
		return errgo.Mask(err)
	}

	var ctlPath params.EntityPath
	var conn *apiconn.Conn
	var apiError error
	for _, cp := range controllers {
		ctx := zapctx.WithFields(ctx, zap.Stringer("controller", cp))
		ctl := mongodoc.Controller{
			Path: cp,
		}
		if err := j.DB.GetController(ctx, &ctl); err != nil {
			zapctx.Error(ctx, "cannot get controller", zap.Error(err))
			continue
		}
		if ctl.Deprecated {
			continue
		}
		if !ctl.Public {
			if err := auth.CheckCanRead(ctx, id, &ctl); err != nil {
				zapctx.Error(ctx, "cannot access controller", zap.Error(err))
				continue
			}
		}

		conn, err = j.OpenAPIFromDoc(ctx, &ctl)
		if err != nil {
			zapctx.Error(ctx, "cannot connect to controller", zap.Error(err))
			continue
		}
		defer conn.Close()
		if err := conn.AddCloud(ctx, name, cloud); err != nil {
			zapctx.Error(ctx, "cannot create cloud", zap.Error(err))
			if apiError == nil {
				apiError = errgo.Mask(err, apiconn.IsAPIError)
			}
			continue
		}
		ctlPath = cp
		break
	}

	if ctlPath.IsZero() {
		if dberr := j.DB.RemoveCloudRegion(ctx, &cloudDoc); dberr != nil {
			zapctx.Warn(ctx, "cannot remove cloud that failed to deploy", zaputil.Error(dberr), zap.String("cloud", string(name)))
		}
		if apiError != nil {
			return apiError
		}
		return errgo.New("cannot create cloud")
	}
	zapctx.WithFields(ctx, zap.Stringer("controller", ctlPath))

	// Ensure the creating user is an admin on the cloud.
	if err := conn.GrantCloudAccess(ctx, name, params.User(id.Id()), "admin"); err != nil {
		zapctx.Error(ctx, "cannot grant cloud access after creating cloud", zap.Error(err), zap.String("user", id.Id()))
		return errgo.Mask(err)
	}

	// Get the new cloud's definition from the controller.
	if err := conn.Cloud(ctx, name, &cloud); err != nil {
		zapctx.Error(ctx, "cannot get cloud definition after creating cloud", zap.Error(err))
		return errgo.Mask(err)
	}

	if err := j.updateControllerCloud(ctx, ctlPath, name, cloud, nil, acl); err != nil {
		zapctx.Error(ctx, "cannot store cloud definition after creating cloud", zap.Error(err))
		return errgo.Mask(err)
	}

	j.DB.AppendAudit(ctx, id, &params.AuditCloudCreated{
		ID:    string(name) + "/",
		Cloud: string(name),
	})
	return nil
}

// RemoveCloud removes the given cloud, so long as no models are using it.
func (j *JEM) RemoveCloud(ctx context.Context, id identchecker.ACLIdentity, cloud params.Cloud) (err error) {
	cr := mongodoc.CloudRegion{
		Cloud: cloud,
	}
	if err := j.DB.GetCloudRegion(ctx, &cr); err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrNotFound))
	}
	if err := auth.CheckACL(ctx, id, cr.ACL.Admin); err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrUnauthorized))
	}
	// This check is technically redundant as we can't know whether
	// the cloud is in use by any models at the moment we remove it from a controller
	// (remember that only one of the primary controllers might be using it).
	// However we like the error message and it's usually going to be OK,
	// so we'll do the advance check anyway.
	if n, err := j.DB.CountModels(ctx, jimmdb.Eq("cloud", cloud)); n > 0 || err != nil {
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
	if _, err := j.DB.RemoveCloudRegions(ctx, jimmdb.Eq("cloud", cloud)); err != nil {
		return errgo.Mask(err)
	}

	j.DB.AppendAudit(ctx, id, &params.AuditCloudRemoved{
		ID:     cr.Id,
		Cloud:  string(cr.Cloud),
		Region: cr.Region,
	})
	return nil
}

// GrantCloud grants access to the given cloud at the given access level to the given user.
func (j *JEM) GrantCloud(ctx context.Context, id identchecker.ACLIdentity, cloud params.Cloud, user params.User, access string) error {
	cr := mongodoc.CloudRegion{
		Cloud: cloud,
	}
	if err := j.DB.GetCloudRegion(ctx, &cr); err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrNotFound))
	}
	if err := auth.CheckACL(ctx, id, cr.ACL.Admin); err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrUnauthorized))
	}
	q := jimmdb.Eq("cloud", cloud)
	u := new(jimmdb.Update)
	switch access {
	case "admin":
		u.AddToSet("acl.admin", user)
		u.AddToSet("acl.write", user)
		fallthrough
	case "add-model":
		u.AddToSet("acl.read", user)
	default:
		return errgo.WithCausef(nil, params.ErrBadRequest, "%q cloud access not valid", access)
	}
	if _, err := j.DB.UpdateCloudRegions(ctx, q, u); err != nil {
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
	cr := mongodoc.CloudRegion{
		Cloud: cloud,
	}
	if err := j.DB.GetCloudRegion(ctx, &cr); err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrNotFound))
	}
	if err := auth.CheckACL(ctx, id, cr.ACL.Admin); err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrUnauthorized))
	}
	q := jimmdb.Eq("cloud", cloud)
	u := new(jimmdb.Update)
	switch access {
	case "add-model":
		u.Pull("acl.read", user)
		fallthrough
	case "admin":
		u.Pull("acl.admin", user)
		u.Pull("acl.write", user)
	default:
		return errgo.WithCausef(nil, params.ErrBadRequest, "%q cloud access not valid", access)
	}
	if _, err := j.DB.UpdateCloudRegions(ctx, q, u); err != nil {
		return errgo.Mask(err)
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

	return nil
}

// A Cloud contains the known information about a cloud. It is the superset
// of the known information that is returned by the various cloud
// endpoints.
type Cloud struct {
	Name             params.Cloud
	Type             string
	AuthTypes        []string
	Endpoint         string
	IdentityEndpoint string
	StorageEndpoint  string
	Regions          []jujuparams.CloudRegion
	CACertificates   []string
	Users            []jujuparams.CloudUserInfo
}

// GetCloud fills in the given cloud structure. If no cloud with the given
// name is found then an error with a cause of params.ErrNotFound will be
// returned. If the given identity doesn't have access to the cloud then an
// error with a cause of params.ErrPermissionDenied is returned.
func (j *JEM) GetCloud(ctx context.Context, id identchecker.ACLIdentity, cloud *Cloud) error {
	q := jimmdb.Eq("cloud", cloud.Name)
	var found bool
	err := j.DB.ForEachCloudRegion(ctx, q, []string{"region"}, func(cr *mongodoc.CloudRegion) error {
		found = true
		if err := auth.CheckCanRead(ctx, id, cr); err != nil {
			return err
		}
		if cr.Region != "" {
			cloud.Regions = append(cloud.Regions, jujuparams.CloudRegion{
				Name:             cr.Region,
				Endpoint:         cr.Endpoint,
				IdentityEndpoint: cr.IdentityEndpoint,
				StorageEndpoint:  cr.StorageEndpoint,
			})
			return nil
		}
		cloud.Type = cr.ProviderType
		cloud.AuthTypes = cr.AuthTypes
		cloud.Endpoint = cr.Endpoint
		cloud.IdentityEndpoint = cr.IdentityEndpoint
		cloud.StorageEndpoint = cr.StorageEndpoint
		cloud.CACertificates = cr.CACertificates
		cloud.Users = cloudUsers(ctx, id, cr)
		return nil
	})
	if !found {
		return errgo.WithCausef(nil, params.ErrNotFound, "cloud not found")
	}
	return errgo.Mask(err, errgo.Is(params.ErrUnauthorized))
}

// ForEachCloud iterates through each cloud readable by the given identity
// and calls the given function for each one. If the given function returns
// an error iteration will stop immediately and the error will be returned
// unmasked.
func (j *JEM) ForEachCloud(ctx context.Context, id identchecker.ACLIdentity, f func(*Cloud) error) error {
	var cloud *Cloud
	var ferr error
	err := j.DB.ForEachCloudRegion(ctx, nil, []string{"cloud", "region"}, func(cr *mongodoc.CloudRegion) error {
		if err := auth.CheckCanRead(ctx, id, cr); err != nil {
			if errgo.Cause(err) == params.ErrUnauthorized {
				err = nil
			}
			return err
		}

		if cr.Region != "" {
			// This is a new region in the current cloud.
			cloud.Regions = append(cloud.Regions, jujuparams.CloudRegion{
				Name:             cr.Region,
				Endpoint:         cr.Endpoint,
				IdentityEndpoint: cr.IdentityEndpoint,
				StorageEndpoint:  cr.StorageEndpoint,
			})
			return nil
		}
		// This is a new cloud.

		// Call f with the previous cloud, if any.
		if cloud != nil {
			if err := f(cloud); err != nil {
				ferr = err
				return errStop
			}
		}

		cloud = &Cloud{
			Name:             cr.Cloud,
			Type:             cr.ProviderType,
			AuthTypes:        cr.AuthTypes,
			Endpoint:         cr.Endpoint,
			IdentityEndpoint: cr.IdentityEndpoint,
			StorageEndpoint:  cr.StorageEndpoint,
			CACertificates:   cr.CACertificates,
			Users:            cloudUsers(ctx, id, cr),
		}
		return nil
	})
	if errgo.Cause(err) == errStop {
		return ferr
	}
	if err != nil {
		return errgo.Mask(err)
	}
	// f will not have been called for the final cloud, call it now.
	if cloud != nil {
		return f(cloud)
	}
	return nil
}

func cloudUsers(ctx context.Context, id identchecker.ACLIdentity, cr *mongodoc.CloudRegion) []jujuparams.CloudUserInfo {
	if err := auth.CheckACL(ctx, id, cr.ACL.Admin); err != nil {
		ut := conv.ToUserTag(params.User(id.Id()))
		return []jujuparams.CloudUserInfo{{
			UserName:    ut.Id(),
			DisplayName: ut.Name(),
			Access:      "add-model",
		}}
	}
	var users []jujuparams.CloudUserInfo
	// Admin users can see all other users.
	seen := make(map[string]bool)
	for _, u := range cr.ACL.Admin {
		if seen[u] {
			continue
		}
		seen[u] = true
		ut := conv.ToUserTag(params.User(u))
		users = append(users, jujuparams.CloudUserInfo{
			UserName:    ut.Id(),
			DisplayName: ut.Name(),
			Access:      "admin",
		})
	}
	for _, u := range cr.ACL.Read {
		if seen[u] {
			continue
		}
		seen[u] = true
		ut := conv.ToUserTag(params.User(u))
		users = append(users, jujuparams.CloudUserInfo{
			UserName:    ut.Id(),
			DisplayName: ut.Name(),
			Access:      "add-model",
		})
	}
	return users
}
