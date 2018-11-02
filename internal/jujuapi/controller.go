// Copyright 2016 Canonical Ltd.

package jujuapi

import (
	"context"
	"reflect"
	"sort"

	modelmanagerapi "github.com/juju/juju/api/modelmanager"
	"github.com/juju/juju/apiserver/common"
	jujuparams "github.com/juju/juju/apiserver/params"
	jujucloud "github.com/juju/juju/cloud"
	jujustatus "github.com/juju/juju/core/status"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/permission"
	"github.com/juju/juju/rpc"
	"github.com/juju/juju/rpc/rpcreflect"
	"github.com/juju/version"
	"go.uber.org/zap"
	"gopkg.in/errgo.v1"
	"gopkg.in/juju/names.v2"
	"gopkg.in/macaroon-bakery.v2-unstable/bakery/checkers"
	"gopkg.in/mgo.v2/bson"

	"github.com/CanonicalLtd/jimm/internal/auth"
	"github.com/CanonicalLtd/jimm/internal/ctxutil"
	"github.com/CanonicalLtd/jimm/internal/jem"
	"github.com/CanonicalLtd/jimm/internal/jemserver"
	"github.com/CanonicalLtd/jimm/internal/mongodoc"
	"github.com/CanonicalLtd/jimm/internal/servermon"
	"github.com/CanonicalLtd/jimm/internal/zapctx"
	"github.com/CanonicalLtd/jimm/internal/zaputil"
	"github.com/CanonicalLtd/jimm/params"
)

type facade struct {
	name    string
	version int
}

// facades contains the list of facade versions supported by
// this API.
var facades = map[facade]string{
	{"Admin", 3}:        "Admin",
	{"Bundle", 1}:       "Bundle",
	{"Cloud", 1}:        "Cloud",
	{"Cloud", 2}:        "Cloud",
	{"Controller", 3}:   "Controller",
	{"JIMM", 1}:         "JIMM",
	{"ModelManager", 2}: "ModelManagerV2",
	{"ModelManager", 3}: "ModelManagerV3",
	{"ModelManager", 4}: "ModelManagerAPI",
	{"Pinger", 1}:       "Pinger",
	{"UserManager", 1}:  "UserManager",
}

// controllerRoot is the root for endpoints served on controller connections.
type controllerRoot struct {
	authContext   context.Context
	params        jemserver.Params
	authPool      *auth.Pool
	jem           *jem.JEM
	heartMonitor  heartMonitor
	findMethod    func(rootName string, version int, methodName string) (rpcreflect.MethodCaller, error)
	schemataCache map[params.Cloud]map[jujucloud.AuthType]jujucloud.CredentialSchema
}

func newControllerRoot(jem *jem.JEM, ap *auth.Pool, p jemserver.Params, hm heartMonitor) *controllerRoot {
	r := &controllerRoot{
		authContext:   context.Background(),
		params:        p,
		authPool:      ap,
		jem:           jem,
		heartMonitor:  hm,
		schemataCache: make(map[params.Cloud]map[jujucloud.AuthType]jujucloud.CredentialSchema),
	}
	r.findMethod = rpcreflect.ValueOf(reflect.ValueOf(r)).FindMethod
	return r
}

// Admin returns an implementation of the Admin facade (version 3).
func (r *controllerRoot) Admin(id string) (admin, error) {
	if id != "" {
		// Safeguard id for possible future use.
		return admin{}, common.ErrBadId
	}
	return admin{r}, nil
}

// Bundle returns an implementation of the Bundle facade (version 1).
func (r *controllerRoot) Bundle(id string) (bundleAPI, error) {
	if id != "" {
		// Safeguard id for possible future use.
		return bundleAPI{}, common.ErrBadId
	}
	return bundleAPI{r}, nil
}

// Cloud returns an implementation of the Cloud facade (version 1).
func (r *controllerRoot) Cloud(id string) (cloud, error) {
	if id != "" {
		// Safeguard id for possible future use.
		return cloud{}, common.ErrBadId
	}
	return cloud{r}, nil
}

// Controller returns an implementation of the Controller facade (version 1).
func (r *controllerRoot) Controller(id string) (controller, error) {
	if id != "" {
		// Safeguard id for possible future use.
		return controller{}, common.ErrBadId
	}
	return controller{r}, nil
}

// JIMM returns an implementation of the JIMM-specific
// API facade.
func (r *controllerRoot) JIMM(id string) (jimm, error) {
	if id != "" {
		// Safeguard id for possible future use.
		return jimm{}, common.ErrBadId
	}
	return jimm{r}, nil
}

// Pinger returns an implementation of the Pinger facade (version 1).
func (r *controllerRoot) Pinger(id string) (pinger, error) {
	if id != "" {
		// Safeguard id for possible future use.
		return pinger{}, common.ErrBadId
	}
	return pinger{}, nil
}

// UserManager returns an implementation of the UserManager facade
// (version 1).
func (r *controllerRoot) UserManager(id string) (userManager, error) {
	if id != "" {
		// Safeguard id for possible future use.
		return userManager{}, common.ErrBadId
	}
	return userManager{r}, nil
}

// credentialSchema gets the schema for the credential identified by the
// given cloud and authType.
func (r *controllerRoot) credentialSchema(ctx context.Context, cloud params.Cloud, authType string) (jujucloud.CredentialSchema, error) {
	if cs, ok := r.schemataCache[cloud]; ok {
		return cs[jujucloud.AuthType(authType)], nil
	}
	providerType, err := r.jem.DB.ProviderType(ctx, cloud)
	if err != nil {
		return nil, errgo.Mask(err, errgo.Is(params.ErrNotFound))
	}
	provider, err := environs.Provider(providerType)
	if err != nil {
		return nil, errgo.Mask(err)
	}
	r.schemataCache[cloud] = provider.CredentialSchemas()
	return r.schemataCache[cloud][jujucloud.AuthType(authType)], nil
}

// doModels calls the given function for each model that the
// authenticated user has access to. If f returns an error, the iteration
// will be stopped and the returned error will have the same cause.
func (r *controllerRoot) doModels(ctx context.Context, f func(context.Context, *mongodoc.Model) error) error {
	it := r.jem.DB.NewCanReadIter(ctx, r.jem.DB.Models().Find(nil).Sort("_id").Iter())
	defer it.Close()

	for {
		var model mongodoc.Model
		if !it.Next(&model) {
			break
		}
		if err := f(ctx, &model); err != nil {
			return errgo.Mask(err, errgo.Any)
		}
	}
	return errgo.Mask(it.Err())
}

// FindMethod implements rpcreflect.MethodFinder.
func (r *controllerRoot) FindMethod(rootName string, version int, methodName string) (rpcreflect.MethodCaller, error) {
	// update the heart monitor for every request received.
	r.heartMonitor.Heartbeat()

	if rootName == "Admin" && version < 3 {
		return nil, &rpc.RequestError{
			Code:    jujuparams.CodeNotSupported,
			Message: "JIMM does not support login from old clients",
		}
	}

	rn := facades[facade{rootName, version}]
	if rn == "" {
		return nil, &rpcreflect.CallNotImplementedError{
			RootMethod: rootName,
			Version:    version,
		}
	}
	return r.findMethod(rn, 0, methodName)
}

// Kill implements rpcreflect.Root.Kill.
func (r *controllerRoot) Kill() {}

// admin implements the Admin facade.
type admin struct {
	root *controllerRoot
}

// Login implements the Login method on the Admin facade.
func (a admin) Login(ctx context.Context, req jujuparams.LoginRequest) (jujuparams.LoginResult, error) {
	// JIMM only supports macaroon login, ignore all the other fields.
	authenticator := a.root.authPool.Authenticator(ctx)
	defer authenticator.Close()
	authContext, m, err := authenticator.Authenticate(a.root.authContext, req.Macaroons, checkers.TimeBefore)
	if err != nil {
		servermon.LoginFailCount.Inc()
		if m != nil {
			return jujuparams.LoginResult{
				DischargeRequired:       m,
				DischargeRequiredReason: err.Error(),
			}, nil
		}
		return jujuparams.LoginResult{}, errgo.Mask(err)
	}
	a.root.authContext = authContext
	ctx = ctxutil.Join(ctx, a.root.authContext)
	servermon.LoginSuccessCount.Inc()
	username := auth.Username(ctx)
	srvVersion, err := a.root.jem.EarliestControllerVersion(ctx)
	if err != nil {
		return jujuparams.LoginResult{}, errgo.Mask(err)
	}
	return jujuparams.LoginResult{
		UserInfo: &jujuparams.AuthUserInfo{
			// TODO(mhilton) get a better display name from the identity manager.
			DisplayName: username,
			Identity:    userTag(username).String(),
		},
		ControllerTag: names.NewControllerTag(a.root.params.ControllerUUID).String(),
		Facades:       facadeVersions(),
		ServerVersion: srvVersion.String(),
	}, nil
}

// facadeVersions creates a list of facadeVersions as specified in
// facades.
func facadeVersions() []jujuparams.FacadeVersions {
	names := make([]string, 0, len(facades))
	versions := make(map[string][]int, len(facades))
	for k := range facades {
		vs, ok := versions[k.name]
		if !ok {
			names = append(names, k.name)
		}
		versions[k.name] = append(vs, k.version)
	}
	sort.Strings(names)
	fvs := make([]jujuparams.FacadeVersions, len(names))
	for i, name := range names {
		vs := versions[name]
		sort.Ints(vs)
		fvs[i] = jujuparams.FacadeVersions{
			Name:     name,
			Versions: vs,
		}
	}
	return fvs
}

// cloud implements the Cloud facade.
type cloud struct {
	root *controllerRoot
}

// Cloud implements the Cloud method of the Cloud facade.
func (c cloud) Cloud(ctx context.Context, ents jujuparams.Entities) (jujuparams.CloudResults, error) {
	ctx = ctxutil.Join(ctx, c.root.authContext)
	cloudResults := make([]jujuparams.CloudResult, len(ents.Entities))
	clouds, err := c.clouds(ctx)
	if err != nil {
		return jujuparams.CloudResults{}, mapError(err)
	}
	for i, ent := range ents.Entities {
		cloud, err := c.cloud(ctx, ent.Tag, clouds)
		if err != nil {
			cloudResults[i].Error = mapError(err)
			continue
		}
		cloudResults[i].Cloud = cloud
	}
	return jujuparams.CloudResults{
		Results: cloudResults,
	}, nil
}

// cloud finds and returns the cloud identified by cloudTag in clouds.
func (c cloud) cloud(ctx context.Context, cloudTag string, clouds map[string]jujuparams.Cloud) (*jujuparams.Cloud, error) {
	if cloud, ok := clouds[cloudTag]; ok {
		return &cloud, nil
	}
	ct, err := names.ParseCloudTag(cloudTag)
	if err != nil {
		return nil, errgo.WithCausef(err, params.ErrBadRequest, "")
	}
	return nil, errgo.WithCausef(nil, params.ErrNotFound, "cloud %q not available", ct.Id())
}

// Clouds implements the Clouds method on the Cloud facade.
func (c cloud) Clouds(ctx context.Context) (jujuparams.CloudsResult, error) {
	ctx = ctxutil.Join(ctx, c.root.authContext)
	var res jujuparams.CloudsResult
	var err error
	res.Clouds, err = c.clouds(ctx)
	return res, errgo.Mask(err)
}

func (c cloud) clouds(ctx context.Context) (map[string]jujuparams.Cloud, error) {
	iter := c.root.jem.DB.GetCloudRegionsIter(ctx)
	results := map[string]jujuparams.Cloud{}
	var v mongodoc.CloudRegion
	for iter.Next(&v) {
		key := names.NewCloudTag(string(v.Cloud)).String()
		cr, _ := results[key]
		if v.Region == "" {
			// v is a cloud
			cr.Type = v.ProviderType
			cr.AuthTypes = v.AuthTypes
			cr.Endpoint = v.Endpoint
			cr.IdentityEndpoint = v.IdentityEndpoint
			cr.StorageEndpoint = v.StorageEndpoint
		} else {
			// v is a region
			cr.Regions = append(cr.Regions, jujuparams.CloudRegion{
				Name:             v.Region,
				Endpoint:         v.Endpoint,
				IdentityEndpoint: v.IdentityEndpoint,
				StorageEndpoint:  v.StorageEndpoint,
			})
		}
		results[key] = cr
	}
	if err := iter.Err(); err != nil {
		return nil, errgo.Notef(err, "cannot query")
	}
	return results, nil
}

// DefaultCloud implements the DefaultCloud method of the Cloud facade.
// It returns a default cloud if there is only one cloud available.
func (c cloud) DefaultCloud(ctx context.Context) (jujuparams.StringResult, error) {
	ctx = ctxutil.Join(ctx, c.root.authContext)
	var result jujuparams.StringResult
	clouds, err := c.clouds(ctx)
	if err != nil {
		return result, errgo.Mask(err)
	}
	zapctx.Info(ctx, "clouds", zap.Any("clouds", clouds))
	if len(clouds) != 1 {
		return result, errgo.WithCausef(nil, params.ErrNotFound, "no default cloud")
	}
	for tag := range clouds {
		result = jujuparams.StringResult{
			Result: tag,
		}
	}
	return result, nil
}

// UserCredentials implements the UserCredentials method of the Cloud facade.
func (c cloud) UserCredentials(ctx context.Context, userclouds jujuparams.UserClouds) (jujuparams.StringsResults, error) {
	ctx = ctxutil.Join(ctx, c.root.authContext)
	results := make([]jujuparams.StringsResult, len(userclouds.UserClouds))
	for i, ent := range userclouds.UserClouds {
		creds, err := c.userCredentials(ctx, ent.UserTag, ent.CloudTag)
		if err != nil {
			results[i].Error = mapError(err)
			continue
		}
		results[i].Result = creds
	}

	return jujuparams.StringsResults{
		Results: results,
	}, nil
}

// userCredentials retrieves the credentials stored for given owner and cloud.
func (c cloud) userCredentials(ctx context.Context, ownerTag, cloudTag string) ([]string, error) {
	ot, err := names.ParseUserTag(ownerTag)
	if err != nil {
		return nil, errgo.WithCausef(err, params.ErrBadRequest, "")
	}
	owner, err := user(ot)
	if err != nil {
		return nil, errgo.Mask(err, errgo.Is(params.ErrBadRequest))
	}
	cld, err := names.ParseCloudTag(cloudTag)
	if err != nil {
		return nil, errgo.WithCausef(err, params.ErrBadRequest, "")
	}
	var cloudCreds []string
	it := c.root.jem.DB.NewCanReadIter(ctx, c.root.jem.DB.Credentials().Find(
		bson.D{{
			"path.entitypath.user", owner,
		}, {
			"path.cloud", cld.Id(),
		}, {
			"revoked", false,
		}},
	).Iter())
	var cred mongodoc.Credential
	for it.Next(&cred) {
		cloudCreds = append(cloudCreds, jem.CloudCredentialTag(cred.Path).String())
	}

	return cloudCreds, errgo.Mask(it.Err())
}

// UpdateCredentials implements the UpdateCredentials method of the Cloud
// facade.
func (c cloud) UpdateCredentials(ctx context.Context, args jujuparams.TaggedCredentials) (jujuparams.ErrorResults, error) {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	ctx = ctxutil.Join(ctx, c.root.authContext)
	results := make([]jujuparams.ErrorResult, len(args.Credentials))
	for i, ucc := range args.Credentials {
		if err := c.updateCredential(ctx, ucc); err != nil {
			results[i].Error = mapError(err)
		}
	}
	return jujuparams.ErrorResults{
		Results: results,
	}, nil
}

// updateCredential adds a single credential to the database.
func (c cloud) updateCredential(ctx context.Context, arg jujuparams.TaggedCredential) error {
	tag, err := names.ParseCloudCredentialTag(arg.Tag)
	if err != nil {
		return errgo.WithCausef(err, params.ErrBadRequest, "")
	}
	ownerTag := tag.Owner()
	owner, err := user(ownerTag)
	if err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrBadRequest))
	}
	if err := auth.CheckIsUser(ctx, owner); err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrUnauthorized))
	}
	var name params.Name
	if err := name.UnmarshalText([]byte(tag.Name())); err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrBadRequest))
	}
	credential := mongodoc.Credential{
		Path: params.CredentialPath{
			Cloud: params.Cloud(tag.Cloud().Id()),
			EntityPath: params.EntityPath{
				User: owner,
				Name: params.Name(tag.Name()),
			},
		},
		Type:       arg.Credential.AuthType,
		Attributes: arg.Credential.Attributes,
	}
	if err := c.root.jem.UpdateCredential(ctx, &credential); err != nil {
		return errgo.Mask(err)
	}
	return nil
}

// RevokeCredentials revokes a set of cloud credentials.
func (c cloud) RevokeCredentials(ctx context.Context, args jujuparams.Entities) (jujuparams.ErrorResults, error) {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	ctx = ctxutil.Join(ctx, c.root.authContext)
	results := make([]jujuparams.ErrorResult, len(args.Entities))
	for i, ent := range args.Entities {
		if err := c.revokeCredential(ctx, ent.Tag); err != nil {
			results[i].Error = mapError(err)
		}
	}
	return jujuparams.ErrorResults{
		Results: results,
	}, nil
}

// RevokeCredentials revokes a set of cloud credentials.
func (c cloud) revokeCredential(ctx context.Context, tag string) error {
	credtag, err := names.ParseCloudCredentialTag(tag)
	if err != nil {
		return errgo.WithCausef(err, params.ErrBadRequest, "cannot parse %q", tag)
	}
	if credtag.Owner().Domain() == "local" {
		// such a credential will not have been uploaded, so it exists
		return nil
	}
	if err := auth.CheckIsUser(ctx, params.User(credtag.Owner().Name())); err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrUnauthorized))
	}
	credential := mongodoc.Credential{
		Path: params.CredentialPath{
			Cloud: params.Cloud(credtag.Cloud().Id()),
			EntityPath: params.EntityPath{
				User: params.User(credtag.Owner().Name()),
				Name: params.Name(credtag.Name()),
			},
		},
		Revoked: true,
	}
	if err := c.root.jem.UpdateCredential(ctx, &credential); err != nil {
		return errgo.Mask(err)
	}
	return nil
}

// Credential implements the Credential method of the Cloud facade.
func (c cloud) Credential(ctx context.Context, args jujuparams.Entities) (jujuparams.CloudCredentialResults, error) {
	ctx = ctxutil.Join(ctx, c.root.authContext)
	results := make([]jujuparams.CloudCredentialResult, len(args.Entities))
	for i, e := range args.Entities {
		cred, err := c.credential(ctx, e.Tag)
		if err != nil {
			results[i].Error = mapError(err)
			continue
		}
		results[i].Result = cred
	}
	return jujuparams.CloudCredentialResults{
		Results: results,
	}, nil
}

// credential retrieves the given credential.
func (c cloud) credential(ctx context.Context, cloudCredentialTag string) (*jujuparams.CloudCredential, error) {
	cct, err := names.ParseCloudCredentialTag(cloudCredentialTag)
	if err != nil {
		return nil, errgo.WithCausef(err, params.ErrBadRequest, "")

	}
	ownerTag := cct.Owner()
	owner, err := user(ownerTag)
	if err != nil {
		return nil, errgo.Mask(err, errgo.Is(params.ErrBadRequest))
	}

	credPath := params.CredentialPath{
		Cloud: params.Cloud(cct.Cloud().Id()),
		EntityPath: params.EntityPath{
			User: owner,
			Name: params.Name(cct.Name()),
		},
	}

	cred, err := c.root.jem.Credential(ctx, credPath)
	if err != nil {
		return nil, errgo.Mask(err, errgo.Is(params.ErrNotFound), errgo.Is(params.ErrUnauthorized))
	}
	if cred.Revoked {
		return nil, errgo.WithCausef(nil, params.ErrNotFound, "credential %q not found", cct.Id())
	}
	schema, err := c.root.credentialSchema(ctx, cred.Path.Cloud, cred.Type)
	if err != nil {
		return nil, errgo.Mask(err, errgo.Is(params.ErrNotFound))
	}
	cc := jujuparams.CloudCredential{
		AuthType:   cred.Type,
		Attributes: make(map[string]string),
	}
	for k, v := range cred.Attributes {
		if ca, ok := schema.Attribute(k); ok && !ca.Hidden {
			cc.Attributes[k] = v
		} else {
			cc.Redacted = append(cc.Redacted, k)
		}
	}
	return &cc, nil
}

// AddCloud implements the AddCloud method of the Cloud (v2) facade.
func (c cloud) AddCloud(ctx context.Context, args jujuparams.AddCloudArgs) error {
	ctx = ctxutil.Join(ctx, c.root.authContext)
	username := auth.Username(ctx)
	cloud := mongodoc.CloudRegion{
		Cloud:            params.Cloud(args.Name),
		ProviderType:     args.Cloud.Type,
		AuthTypes:        args.Cloud.AuthTypes,
		Endpoint:         args.Cloud.Endpoint,
		IdentityEndpoint: args.Cloud.IdentityEndpoint,
		StorageEndpoint:  args.Cloud.StorageEndpoint,
		CACertificates:   args.Cloud.CACertificates,
		ACL: params.ACL{
			Read:  []string{username},
			Write: []string{username},
			Admin: []string{username},
		},
	}
	regions := make([]mongodoc.CloudRegion, len(args.Cloud.Regions))
	for i, region := range args.Cloud.Regions {
		regions[i] = mongodoc.CloudRegion{
			Cloud:            params.Cloud(args.Name),
			Region:           region.Name,
			Endpoint:         region.Endpoint,
			IdentityEndpoint: region.IdentityEndpoint,
			StorageEndpoint:  region.StorageEndpoint,
			ACL: params.ACL{
				Read:  []string{username},
				Write: []string{username},
				Admin: []string{username},
			},
		}
	}
	return c.root.jem.CreateCloud(ctx, cloud, regions)
}

// AddCredentials implements the AddCredentials method of the Cloud (v2) facade.
func (c cloud) AddCredentials(ctx context.Context, args jujuparams.TaggedCredentials) (jujuparams.ErrorResults, error) {
	// In JIMM UpdateCredentials behaves in the way AddCredentials is
	// documented to. Presumably in juju UpdateCredentials works
	// slightly differently.
	return c.UpdateCredentials(ctx, args)
}

// CredentialContents implements the CredentialContents method of the Cloud (v2) facade.
func (c cloud) CredentialContents(ctx context.Context, args jujuparams.CloudCredentialArgs) (jujuparams.CredentialContentResults, error) {
	ctx = ctxutil.Join(ctx, c.root.authContext)
	results := make([]jujuparams.CredentialContentResult, len(args.Credentials))
	for i, arg := range args.Credentials {
		credInfo, err := c.credentialInfo(ctx, arg.CloudName, arg.CredentialName, args.IncludeSecrets)
		if err != nil {
			results[i].Error = mapError(err)
			continue
		}
		results[i].Result = credInfo
	}
	return jujuparams.CredentialContentResults{
		Results: results,
	}, nil
}

// credentialInfo returns Juju API information on the given credential
// within the given cloud. If includeSecrets is true, secret information
// will be included too.
func (c cloud) credentialInfo(ctx context.Context, cloudName, credentialName string, includeSecrets bool) (*jujuparams.ControllerCredentialInfo, error) {
	credPath := params.CredentialPath{
		Cloud: params.Cloud(cloudName),
		EntityPath: params.EntityPath{
			User: params.User(auth.Username(ctx)),
			Name: params.Name(credentialName),
		},
	}
	cred, err := c.root.jem.Credential(ctx, credPath)
	if err != nil {
		return nil, errgo.Mask(err, errgo.Is(params.ErrNotFound), errgo.Is(params.ErrUnauthorized))
	}
	if cred.Revoked {
		return nil, errgo.WithCausef(nil, params.ErrNotFound, "")
	}
	schema, err := c.root.credentialSchema(ctx, cred.Path.Cloud, cred.Type)
	if err != nil {
		return nil, errgo.Mask(err, errgo.Is(params.ErrNotFound))
	}
	cci := jujuparams.ControllerCredentialInfo{
		Content: jujuparams.CredentialContent{
			Name:       credentialName,
			Cloud:      cloudName,
			AuthType:   cred.Type,
			Attributes: make(map[string]string),
		},
	}
	for k, v := range cred.Attributes {
		if ca, ok := schema.Attribute(k); ok && (!ca.Hidden || includeSecrets) {
			cci.Content.Attributes[k] = v
		}
	}
	c.root.doModels(ctx, func(ctx context.Context, model *mongodoc.Model) error {
		if model.Credential != credPath {
			return nil
		}
		access := jujuparams.ModelReadAccess
		switch {
		case params.User(auth.Username(ctx)) == model.Path.User:
			access = jujuparams.ModelAdminAccess
		case auth.CheckACL(ctx, model.ACL.Admin) == nil:
			access = jujuparams.ModelAdminAccess
		case auth.CheckACL(ctx, model.ACL.Write) == nil:
			access = jujuparams.ModelWriteAccess
		}
		cci.Models = append(cci.Models, jujuparams.ModelAccess{
			Model:  string(model.Path.Name),
			Access: string(access),
		})
		return nil
	})
	return &cci, nil
}

// RemoveClouds removes the specified clouds from the controller.
// If a cloud is in use (has models deployed to it), the removal will fail.
func (c cloud) RemoveClouds(ctx context.Context, args jujuparams.Entities) (jujuparams.ErrorResults, error) {
	ctx = ctxutil.Join(ctx, c.root.authContext)
	result := jujuparams.ErrorResults{
		Results: make([]jujuparams.ErrorResult, len(args.Entities)),
	}
	for i, entity := range args.Entities {
		tag, err := names.ParseCloudTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = mapError(err)
			continue
		}
		err = c.root.jem.RemoveCloud(ctx, params.Cloud(tag.Id()))
		if err != nil {
			result.Results[i].Error = mapError(err)
		}
	}
	return result, nil
}

// controller implements the Controller facade.
type controller struct {
	root *controllerRoot
}

func (c controller) AllModels(ctx context.Context) (jujuparams.UserModelList, error) {
	ctx = ctxutil.Join(ctx, c.root.authContext)
	return c.root.allModels(ctx)
}

func (c controller) ModelStatus(ctx context.Context, args jujuparams.Entities) (jujuparams.ModelStatusResults, error) {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	ctx = ctxutil.Join(ctx, c.root.authContext)
	results := make([]jujuparams.ModelStatus, len(args.Entities))
	// TODO (fabricematrat) get status for all of the models connected
	// to a single controller in one go.
	for i, arg := range args.Entities {
		mi, err := c.modelStatus(ctx, arg)
		if err != nil {
			return jujuparams.ModelStatusResults{}, errgo.Mask(err, errgo.Is(params.ErrNotFound))
		}
		results[i] = *mi
	}

	return jujuparams.ModelStatusResults{
		Results: results,
	}, nil
}

// modelStatus retrieves the model status for the specified entity.
func (c controller) modelStatus(ctx context.Context, arg jujuparams.Entity) (*jujuparams.ModelStatus, error) {
	mi, err := c.root.modelInfo(ctx, arg, false)
	if err != nil {
		return &jujuparams.ModelStatus{}, errgo.Mask(err, errgo.Is(params.ErrNotFound))
	}
	return &jujuparams.ModelStatus{
		ModelTag:           names.NewModelTag(mi.UUID).String(),
		Life:               mi.Life,
		HostedMachineCount: len(mi.Machines),
		ApplicationCount:   0,
		OwnerTag:           mi.OwnerTag,
		Machines:           mi.Machines,
	}, nil
}

// ControllerConfig returns the controller's configuration.
func (c controller) ControllerConfig() (jujuparams.ControllerConfigResult, error) {
	result := jujuparams.ControllerConfigResult{
		Config: map[string]interface{}{
			"charmstore-url": c.root.params.CharmstoreLocation,
			"metering-url":   c.root.params.MeteringLocation,
		},
	}
	return result, nil
}

// allModels returns all the models the logged in user has access to.
func (r *controllerRoot) allModels(ctx context.Context) (jujuparams.UserModelList, error) {
	var models []jujuparams.UserModel
	err := r.doModels(ctx, func(ctx context.Context, model *mongodoc.Model) error {
		models = append(models, jujuparams.UserModel{
			Model:          userModelForModelDoc(model),
			LastConnection: nil, // TODO (mhilton) work out how to record and set this.
		})
		return nil
	})
	if err != nil {
		return jujuparams.UserModelList{}, errgo.Mask(err)
	}
	return jujuparams.UserModelList{
		UserModels: models,
	}, nil
}

func userModelForModelDoc(m *mongodoc.Model) jujuparams.Model {
	return jujuparams.Model{
		Name:     string(m.Path.Name),
		UUID:     m.UUID,
		Type:     m.Type,
		OwnerTag: jem.UserTag(m.Path.User).String(),
	}
}

// modelInfo retrieves the model information for the specified entity.
func (r *controllerRoot) modelInfo(ctx context.Context, arg jujuparams.Entity, localOnly bool) (*jujuparams.ModelInfo, error) {
	model, err := getModel(ctx, r.jem, arg.Tag, auth.CheckCanRead)
	if err != nil {
		return nil, errgo.Mask(err,
			errgo.Is(params.ErrBadRequest),
			errgo.Is(params.ErrUnauthorized),
			errgo.Is(params.ErrNotFound),
		)
	}
	ctx = zapctx.WithFields(ctx, zap.String("model-uuid", model.UUID))
	info, err := r.modelDocToModelInfo(ctx, model)
	if err != nil {
		return nil, errgo.Mask(err)
	}
	if localOnly {
		return info, nil
	}
	// Query the model itself for user information.
	infoFromController, err := fetchModelInfo(ctx, r.jem, model)
	if err != nil {
		code := jujuparams.ErrCode(err)
		if model.Life() == string(jujuparams.Dying) && code == jujuparams.CodeUnauthorized {
			zapctx.Info(ctx, "could not get ModelInfo for dying model, marking dead", zap.Error(err))
			// The model was dying and now cannot be accessed, assume it is now dead.
			if err := r.jem.DB.DeleteModelWithUUID(ctx, model.Controller, model.UUID); err != nil {
				// If this update fails then don't worry as the watcher
				// will detect the state change and update as appropriate.
				zapctx.Warn(ctx, "error deleting model", zap.Error(err))
			}
			// return the error with the an appropriate cause.
			return nil, errgo.WithCausef(err, params.ErrUnauthorized, "%s", "")
		}

		// We have most of the information we want already so return that.
		zapctx.Error(ctx, "failed to get ModelInfo from controller", zap.String("controller", model.Controller.String()), zaputil.Error(err))
		return info, nil
	}
	info.Users = filterUsers(ctx, infoFromController.Users, isModelAdmin(ctx, infoFromController))
	return info, nil
}

func (r *controllerRoot) modelDocToModelInfo(ctx context.Context, model *mongodoc.Model) (*jujuparams.ModelInfo, error) {
	machines, err := r.jem.DB.MachinesForModel(ctx, model.UUID)
	if err != nil {
		return nil, errgo.Mask(err)
	}
	providerType := model.ProviderType
	if providerType == "" {
		providerType, err = r.jem.DB.ProviderType(ctx, model.Cloud)
		if err != nil {
			return nil, errgo.Notef(err, "cannot get cloud %q", model.Cloud)
		}
	}

	userLevels := make(map[string]jujuparams.UserAccessPermission)
	for _, user := range model.ACL.Read {
		userLevels[user] = jujuparams.ModelReadAccess
	}
	for _, user := range model.ACL.Write {
		userLevels[user] = jujuparams.ModelWriteAccess
	}
	for _, user := range model.ACL.Admin {
		userLevels[user] = jujuparams.ModelAdminAccess
	}
	userLevels[string(model.Path.User)] = jujuparams.ModelAdminAccess

	var users []jujuparams.ModelUserInfo
	if auth.CheckIsAdmin(ctx, model) == nil {
		usernames := make([]string, 0, len(userLevels))
		for user := range userLevels {
			usernames = append(usernames, user)
		}
		sort.Strings(usernames)
		for _, user := range usernames {
			ut := userTag(user)
			users = append(users, jujuparams.ModelUserInfo{
				UserName:    ut.Id(),
				DisplayName: ut.Name(),
				Access:      userLevels[user],
			})
		}
	} else {
		ut := userTag(auth.Username(ctx))
		users = append(users, jujuparams.ModelUserInfo{
			UserName:    ut.Id(),
			DisplayName: ut.Name(),
			Access:      userLevels[auth.Username(ctx)],
		})
	}
	return &jujuparams.ModelInfo{
		Name:               string(model.Path.Name),
		UUID:               model.UUID,
		ControllerUUID:     r.params.ControllerUUID,
		ProviderType:       providerType,
		DefaultSeries:      model.DefaultSeries,
		CloudTag:           jem.CloudTag(model.Cloud).String(),
		CloudRegion:        model.CloudRegion,
		CloudCredentialTag: jem.CloudCredentialTag(model.Credential).String(),
		OwnerTag:           jem.UserTag(model.Path.User).String(),
		Life:               jujuparams.Life(model.Life()),
		Status:             modelStatus(model.Info),
		Users:              users,
		Machines:           jemMachinesToModelMachineInfo(machines),
		AgentVersion:       modelVersion(ctx, model.Info),
		Type:               model.Type,
	}, nil
}

func jemMachinesToModelMachineInfo(machines []mongodoc.Machine) []jujuparams.ModelMachineInfo {
	infos := make([]jujuparams.ModelMachineInfo, 0, len(machines))
	for _, m := range machines {
		if m.Info.Life != "dead" {
			infos = append(infos, jemMachineToModelMachineInfo(m))
		}
	}
	return infos
}

func jemMachineToModelMachineInfo(m mongodoc.Machine) jujuparams.ModelMachineInfo {
	var hardware *jujuparams.MachineHardware
	if m.Info.HardwareCharacteristics != nil {
		hardware = &jujuparams.MachineHardware{
			Arch:             m.Info.HardwareCharacteristics.Arch,
			Mem:              m.Info.HardwareCharacteristics.Mem,
			RootDisk:         m.Info.HardwareCharacteristics.RootDisk,
			Cores:            m.Info.HardwareCharacteristics.CpuCores,
			CpuPower:         m.Info.HardwareCharacteristics.CpuPower,
			Tags:             m.Info.HardwareCharacteristics.Tags,
			AvailabilityZone: m.Info.HardwareCharacteristics.AvailabilityZone,
		}
	}
	return jujuparams.ModelMachineInfo{
		Id:         m.Info.Id,
		InstanceId: m.Info.InstanceId,
		Status:     string(m.Info.AgentStatus.Current),
		HasVote:    m.Info.HasVote,
		WantsVote:  m.Info.WantsVote,
		Hardware:   hardware,
	}
}

// isModelAdmin determines if the current user is an admin on the given model.
func isModelAdmin(ctx context.Context, info *jujuparams.ModelInfo) bool {
	var admin bool
	iterUsers(ctx, info.Users, func(u params.User, ui jujuparams.ModelUserInfo) {
		admin = admin || ui.Access == jujuparams.ModelAdminAccess && auth.CheckIsUser(ctx, u) == nil
	})
	return admin
}

// filterUsers returns a slice holding all of the given users that the
// current user should be able to see. Admin users can see everyone;
// other users can only see users and groups they're a member of. Users
// local to the controller are always removed.
func filterUsers(ctx context.Context, users []jujuparams.ModelUserInfo, admin bool) []jujuparams.ModelUserInfo {
	filtered := make([]jujuparams.ModelUserInfo, 0, len(users))
	iterUsers(ctx, users, func(u params.User, ui jujuparams.ModelUserInfo) {
		if admin || auth.CheckIsUser(ctx, u) == nil {
			filtered = append(filtered, ui)
		}
	})
	return filtered
}

// iterUsers iterates through all the non-local users in users and calls
// f with each in turn.
func iterUsers(ctx context.Context, users []jujuparams.ModelUserInfo, f func(params.User, jujuparams.ModelUserInfo)) {
	for _, u := range users {
		if !names.IsValidUser(u.UserName) {
			zapctx.Info(ctx, "controller sent invalid username, skipping", zap.String("username", u.UserName))
			continue
		}
		tag := names.NewUserTag(u.UserName)
		user, err := user(tag)
		if err != nil {
			// This error will occur if the user is local to
			// the controller, it can be safely ignored.
			continue
		}
		f(user, u)
	}
}

// jimm implements a facade containing JIMM-specific API calls.
type jimm struct {
	root *controllerRoot
}

// UserModelStats returns statistics about all the models that were created
// by the currently authenticated user.
func (j jimm) UserModelStats(ctx context.Context) (params.UserModelStatsResponse, error) {
	ctx = ctxutil.Join(ctx, j.root.authContext)
	models := make(map[string]params.ModelStats)

	user := auth.Username(ctx)
	it := j.root.jem.DB.NewCanReadIter(ctx,
		j.root.jem.DB.Models().
			Find(bson.D{{"creator", user}}).
			Select(bson.D{{"uuid", 1}, {"path", 1}, {"creator", 1}, {"counts", 1}}).
			Iter())
	var model mongodoc.Model
	for it.Next(&model) {
		models[model.UUID] = params.ModelStats{
			Model:  userModelForModelDoc(&model),
			Counts: model.Counts,
		}
	}
	if err := it.Err(); err != nil {
		return params.UserModelStatsResponse{}, errgo.Mask(err)
	}
	return params.UserModelStatsResponse{
		Models: models,
	}, nil
}

// getModel attempts to get the specified model from jem. If the model
// tag is not valid then the error cause will be params.ErrBadRequest. If
// the model cannot be found then the error cause will be
// params.ErrNotFound. If authf is non-nil then it will be called with
// the found model. authf is used to authenticate access to the model, if
// access is denied authf should return an error with the cause
// params.ErrUnauthorized. The cause of any error returned by authf will
// not be masked.
func getModel(ctx context.Context, jem *jem.JEM, modelTag string, authf func(context.Context, auth.ACLEntity) error) (*mongodoc.Model, error) {
	tag, err := names.ParseModelTag(modelTag)
	if err != nil {
		return nil, errgo.WithCausef(err, params.ErrBadRequest, "invalid model tag")
	}
	model, err := jem.DB.ModelFromUUID(ctx, tag.Id())
	if err != nil {
		return nil, errgo.Mask(err, errgo.Is(params.ErrNotFound))
	}
	if authf == nil {
		return model, nil
	}
	if err := authf(ctx, model); err != nil {
		return nil, errgo.Mask(err, errgo.Any)
	}
	if model.Cloud != "" {
		return model, nil
	}
	// The model does not currently store its cloud information so go
	// and fetch it from the model itself. This happens if the model
	// was created with a JIMM version older than 0.9.5.
	info, err := fetchModelInfo(ctx, jem, model)
	if err != nil {
		return nil, errgo.Mask(err)
	}
	cloudTag, err := names.ParseCloudTag(info.CloudTag)
	if err != nil {
		return nil, errgo.Notef(err, "bad data from controller")
	}
	credentialTag, err := names.ParseCloudCredentialTag(info.CloudCredentialTag)
	if err != nil {
		return nil, errgo.Notef(err, "bad data from controller")
	}
	model.Cloud = params.Cloud(cloudTag.Id())
	model.CloudRegion = info.CloudRegion
	owner, err := user(credentialTag.Owner())
	if err != nil {
		return nil, errgo.Mask(err, errgo.Is(params.ErrBadRequest))
	}
	model.Credential = params.CredentialPath{
		Cloud: params.Cloud(credentialTag.Cloud().Id()),
		EntityPath: params.EntityPath{
			User: owner,
			Name: params.Name(credentialTag.Name()),
		},
	}
	model.DefaultSeries = info.DefaultSeries

	if err := jem.DB.UpdateLegacyModel(ctx, model); err != nil {
		zapctx.Warn(ctx, "cannot update %s with cloud details", zap.String("model", model.Path.String()), zaputil.Error(err))
	}
	return model, nil
}

func fetchModelInfo(ctx context.Context, jem *jem.JEM, model *mongodoc.Model) (*jujuparams.ModelInfo, error) {
	conn, err := jem.OpenAPI(ctx, model.Controller)
	if err != nil {
		return nil, errgo.Mask(err, errgo.Is(context.DeadlineExceeded))
	}
	defer conn.Close()
	client := modelmanagerapi.NewClient(conn)
	var infos []jujuparams.ModelInfoResult
	err = runWithContext(ctx, func() error {
		var err error
		infos, err = client.ModelInfo([]names.ModelTag{names.NewModelTag(model.UUID)})
		return err
	})
	if err != nil {
		return nil, errgo.Mask(err, errgo.Is(context.DeadlineExceeded))
	}
	if len(infos) != 1 {
		return nil, errgo.Newf("unexpected number of ModelInfo results")
	}
	if infos[0].Error != nil {
		return nil, infos[0].Error
	}
	return infos[0].Result, nil
}

// pinger implements the Pinger facade.
type pinger struct{}

// Ping implements the Pinger facade's Ping method. It doesn't do
// anything.
func (p pinger) Ping() {}

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
	ctx = ctxutil.Join(ctx, u.root.authContext)
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
	if auth.Username(ctx) != string(user) {
		return nil, params.ErrUnauthorized
	}
	return u.currentUser(ctx)
}

func (u userManager) currentUser(ctx context.Context) (*jujuparams.UserInfo, error) {
	userTag := userTag(auth.Username(ctx))
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

// userTag creates a UserTag from the given username. The returned
// UserTag will always have a domain set. If username has no domain then
// @external will be used.
func userTag(username string) names.UserTag {
	tag := names.NewUserTag(username)
	if tag.Domain() == "" {
		tag = tag.WithDomain("external")
	}
	return tag
}

// user creates a params.User from the given UserTag. If the UserTag is
// for a local user then an error will be returned. If the UserTag has
// the domain "external" then the returned User will only contain the
// name part.
func user(tag names.UserTag) (params.User, error) {
	if tag.IsLocal() {
		return "", errgo.WithCausef(nil, params.ErrBadRequest, "unsupported local user")
	}
	var username string
	if tag.Domain() == "external" {
		username = tag.Name()
	} else {
		username = tag.Id()
	}
	return params.User(username), nil
}

// runWithContext runs the given function and completes either when the
// function completes, or when the given context is canceled. If the
// function returns because the context was cancelled then the returned
// error will have the value of ctx.Err().
func runWithContext(ctx context.Context, f func() error) error {
	c := make(chan error)
	go func() {
		err := f()
		select {
		case c <- err:
		case <-ctx.Done():
			if err != nil {
				zapctx.Info(ctx, "error in canceled task", zaputil.Error(err))
			}
		}
	}()
	select {
	case err := <-c:
		return errgo.Mask(err, errgo.Any)
	case <-ctx.Done():
		return errgo.Mask(ctx.Err(), errgo.Any)
	}
}

func modelStatus(info *mongodoc.ModelInfo) jujuparams.EntityStatus {
	var status jujuparams.EntityStatus
	if info == nil {
		return status
	}
	status.Status = jujustatus.Status(info.Status.Status)
	status.Info = info.Status.Message
	status.Data = info.Status.Data
	if !info.Status.Since.IsZero() {
		status.Since = &info.Status.Since
	}
	return status
}

func modelVersion(ctx context.Context, info *mongodoc.ModelInfo) *version.Number {
	if info == nil {
		return nil
	}
	versionString, _ := info.Config[config.AgentVersionKey].(string)
	if versionString == "" {
		return nil
	}
	v, err := version.Parse(versionString)
	if err != nil {
		zapctx.Warn(ctx, "cannot parse agent-version", zap.String("agent-version", versionString), zap.Error(err))
		return nil
	}
	return &v
}
