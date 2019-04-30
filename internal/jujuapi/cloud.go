// Copyright 2016 Canonical Ltd.

package jujuapi

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/juju/apiserver/common"
	jujuparams "github.com/juju/juju/apiserver/params"
	"go.uber.org/zap"
	"gopkg.in/errgo.v1"
	"gopkg.in/juju/names.v2"
	"gopkg.in/mgo.v2/bson"

	"github.com/CanonicalLtd/jimm/internal/auth"
	"github.com/CanonicalLtd/jimm/internal/ctxutil"
	"github.com/CanonicalLtd/jimm/internal/jem"
	"github.com/CanonicalLtd/jimm/internal/mongodoc"
	"github.com/CanonicalLtd/jimm/internal/zapctx"
	"github.com/CanonicalLtd/jimm/params"
)

func (r *controllerRoot) CloudV1(id string) (cloudV1, error) {
	if id != "" {
		// Safeguard id for possible future use.
		return cloudV1{}, common.ErrBadId
	}
	return cloudV1{cloudV2{cloudV3{cloudV4{cloudV5{r}}}}}, nil
}

func (r *controllerRoot) CloudV2(id string) (cloudV2, error) {
	if id != "" {
		// Safeguard id for possible future use.
		return cloudV2{}, common.ErrBadId
	}
	return cloudV2{cloudV3{cloudV4{cloudV5{r}}}}, nil
}

func (r *controllerRoot) CloudV3(id string) (cloudV3, error) {
	if id != "" {
		// Safeguard id for possible future use.
		return cloudV3{}, common.ErrBadId
	}
	return cloudV3{cloudV4{cloudV5{r}}}, nil
}

func (r *controllerRoot) CloudV4(id string) (cloudV4, error) {
	if id != "" {
		// Safeguard id for possible future use.
		return cloudV4{}, common.ErrBadId
	}
	return cloudV4{cloudV5{r}}, nil
}

func (r *controllerRoot) CloudV5(id string) (cloudV5, error) {
	if id != "" {
		// Safeguard id for possible future use.
		return cloudV5{}, common.ErrBadId
	}
	return cloudV5{r}, nil
}

type cloudV1 struct {
	c cloudV2
}

func (c cloudV1) Cloud(ctx context.Context, args jujuparams.Entities) (jujuparams.CloudResults, error) {
	return c.c.Cloud(ctx, args)
}

func (c cloudV1) Clouds(ctx context.Context) (jujuparams.CloudsResult, error) {
	return c.c.Clouds(ctx)
}

func (c cloudV1) Credential(ctx context.Context, args jujuparams.Entities) (jujuparams.CloudCredentialResults, error) {
	return c.c.Credential(ctx, args)
}

func (c cloudV1) DefaultCloud(ctx context.Context) (jujuparams.StringResult, error) {
	return c.c.DefaultCloud(ctx)
}

func (c cloudV1) RevokeCredentials(ctx context.Context, args jujuparams.Entities) (jujuparams.ErrorResults, error) {
	return c.c.RevokeCredentials(ctx, args)
}

func (c cloudV1) UpdateCredentials(ctx context.Context, args jujuparams.TaggedCredentials) (jujuparams.ErrorResults, error) {
	return c.c.UpdateCredentials(ctx, args)
}

func (c cloudV1) UserCredentials(ctx context.Context, args jujuparams.UserClouds) (jujuparams.StringsResults, error) {
	return c.c.UserCredentials(ctx, args)
}

type cloudV2 struct {
	c cloudV3
}

func (c cloudV2) Cloud(ctx context.Context, args jujuparams.Entities) (jujuparams.CloudResults, error) {
	return c.c.Cloud(ctx, args)
}

func (c cloudV2) Clouds(ctx context.Context) (jujuparams.CloudsResult, error) {
	return c.c.Clouds(ctx)
}

func (c cloudV2) Credential(ctx context.Context, args jujuparams.Entities) (jujuparams.CloudCredentialResults, error) {
	return c.c.Credential(ctx, args)
}

func (c cloudV2) DefaultCloud(ctx context.Context) (jujuparams.StringResult, error) {
	return c.c.DefaultCloud(ctx)
}

func (c cloudV2) RevokeCredentials(ctx context.Context, args jujuparams.Entities) (jujuparams.ErrorResults, error) {
	creds := make([]jujuparams.RevokeCredentialArg, len(args.Entities))
	for i, e := range args.Entities {
		creds[i].Tag = e.Tag
		creds[i].Force = true
	}
	return c.c.RevokeCredentialsCheckModels(ctx, jujuparams.RevokeCredentialArgs{
		Credentials: creds,
	})
}

func (c cloudV2) UpdateCredentials(ctx context.Context, args jujuparams.TaggedCredentials) (jujuparams.ErrorResults, error) {
	// In JIMM UpdateCredentials behaves in the way AddCredentials is
	// documented to. Presumably in juju UpdateCredentials works
	// slightly differently.
	return c.c.AddCredentials(ctx, args)
}

func (c cloudV2) UserCredentials(ctx context.Context, args jujuparams.UserClouds) (jujuparams.StringsResults, error) {
	return c.c.UserCredentials(ctx, args)
}

func (c cloudV2) AddCloud(ctx context.Context, args jujuparams.AddCloudArgs) error {
	// New in V2.
	return c.c.AddCloud(ctx, args)
}

func (c cloudV2) AddCredentials(ctx context.Context, args jujuparams.TaggedCredentials) (jujuparams.ErrorResults, error) {
	// New in V2.
	return c.c.AddCredentials(ctx, args)
}

func (c cloudV2) CredentialContents(ctx context.Context, args jujuparams.CloudCredentialArgs) (jujuparams.CredentialContentResults, error) {
	// New in V2.
	return c.c.CredentialContents(ctx, args)
}

func (c cloudV2) RemoveClouds(ctx context.Context, args jujuparams.Entities) (jujuparams.ErrorResults, error) {
	// New in V2.
	return c.c.RemoveClouds(ctx, args)
}

type cloudV3 struct {
	c cloudV4
}

func (c cloudV3) Cloud(ctx context.Context, ents jujuparams.Entities) (jujuparams.CloudResults, error) {
	return c.c.Cloud(ctx, ents)
}

func (c cloudV3) Clouds(ctx context.Context) (jujuparams.CloudsResult, error) {
	return c.c.Clouds(ctx)
}

func (c cloudV3) DefaultCloud(ctx context.Context) (jujuparams.StringResult, error) {
	return c.c.DefaultCloud(ctx)
}

func (c cloudV3) UserCredentials(ctx context.Context, userclouds jujuparams.UserClouds) (jujuparams.StringsResults, error) {
	return c.c.UserCredentials(ctx, userclouds)
}

func (c cloudV3) RevokeCredentialsCheckModels(ctx context.Context, args jujuparams.RevokeCredentialArgs) (jujuparams.ErrorResults, error) {
	return c.c.RevokeCredentialsCheckModels(ctx, args)
}

func (c cloudV3) Credential(ctx context.Context, args jujuparams.Entities) (jujuparams.CloudCredentialResults, error) {
	return c.c.Credential(ctx, args)
}

func (c cloudV3) AddCloud(ctx context.Context, args jujuparams.AddCloudArgs) error {
	return c.c.AddCloud(ctx, args)
}

func (c cloudV3) AddCredentials(ctx context.Context, args jujuparams.TaggedCredentials) (jujuparams.ErrorResults, error) {
	return c.c.AddCredentials(ctx, args)
}

func (c cloudV3) CredentialContents(ctx context.Context, args jujuparams.CloudCredentialArgs) (jujuparams.CredentialContentResults, error) {
	return c.c.CredentialContents(ctx, args)
}

func (c cloudV3) RemoveClouds(ctx context.Context, args jujuparams.Entities) (jujuparams.ErrorResults, error) {
	return c.c.RemoveClouds(ctx, args)
}

func (c cloudV3) CheckCredentialsModels(ctx context.Context, args jujuparams.TaggedCredentials) (jujuparams.UpdateCredentialResults, error) {
	return c.c.CheckCredentialsModels(ctx, args)
}

func (c cloudV3) ModifyCloudAccess(ctx context.Context, args jujuparams.ModifyCloudAccessRequest) (jujuparams.ErrorResults, error) {
	return c.c.ModifyCloudAccess(ctx, args)
}

func (c cloudV3) UpdateCredentialsCheckModels(ctx context.Context, args jujuparams.UpdateCredentialArgs) (jujuparams.UpdateCredentialResults, error) {
	return c.c.UpdateCredentialsCheckModels(ctx, args)
}

type cloudV4 struct {
	c cloudV5
}

func (c cloudV4) Cloud(ctx context.Context, ents jujuparams.Entities) (jujuparams.CloudResults, error) {
	return c.c.Cloud(ctx, ents)
}

func (c cloudV4) Clouds(ctx context.Context) (jujuparams.CloudsResult, error) {
	return c.c.Clouds(ctx)
}

// DefaultCloud implements the DefaultCloud method of the Cloud facade.
// It returns a default cloud if there is only one cloud available.
func (c cloudV4) DefaultCloud(ctx context.Context) (jujuparams.StringResult, error) {
	ctx = ctxutil.Join(ctx, c.c.root.authContext)
	var result jujuparams.StringResult
	clouds, err := c.c.clouds(ctx)
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

func (c cloudV4) UserCredentials(ctx context.Context, userclouds jujuparams.UserClouds) (jujuparams.StringsResults, error) {
	return c.c.UserCredentials(ctx, userclouds)
}

func (c cloudV4) RevokeCredentialsCheckModels(ctx context.Context, args jujuparams.RevokeCredentialArgs) (jujuparams.ErrorResults, error) {
	return c.c.RevokeCredentialsCheckModels(ctx, args)
}

func (c cloudV4) Credential(ctx context.Context, args jujuparams.Entities) (jujuparams.CloudCredentialResults, error) {
	return c.c.Credential(ctx, args)
}

func (c cloudV4) AddCloud(ctx context.Context, args jujuparams.AddCloudArgs) error {
	return c.c.AddCloud(ctx, args)
}

func (c cloudV4) AddCredentials(ctx context.Context, args jujuparams.TaggedCredentials) (jujuparams.ErrorResults, error) {
	return c.c.AddCredentials(ctx, args)
}

func (c cloudV4) CredentialContents(ctx context.Context, args jujuparams.CloudCredentialArgs) (jujuparams.CredentialContentResults, error) {
	return c.c.CredentialContents(ctx, args)
}

func (c cloudV4) RemoveClouds(ctx context.Context, args jujuparams.Entities) (jujuparams.ErrorResults, error) {
	return c.c.RemoveClouds(ctx, args)
}

func (c cloudV4) CheckCredentialsModels(ctx context.Context, args jujuparams.TaggedCredentials) (jujuparams.UpdateCredentialResults, error) {
	return c.c.CheckCredentialsModels(ctx, args)
}

func (c cloudV4) ModifyCloudAccess(ctx context.Context, args jujuparams.ModifyCloudAccessRequest) (jujuparams.ErrorResults, error) {
	return c.c.ModifyCloudAccess(ctx, args)
}

func (c cloudV4) UpdateCredentialsCheckModels(ctx context.Context, args jujuparams.UpdateCredentialArgs) (jujuparams.UpdateCredentialResults, error) {
	return c.c.UpdateCredentialsCheckModels(ctx, args)
}

func (c cloudV4) UpdateCloud(ctx context.Context, args jujuparams.UpdateCloudArgs) (jujuparams.ErrorResults, error) {
	return c.c.UpdateCloud(ctx, args)
}

// cloudV5 implements the Cloud facade.
type cloudV5 struct {
	root *controllerRoot
}

// Cloud implements the Cloud method of the Cloud facade.
func (c cloudV5) Cloud(ctx context.Context, ents jujuparams.Entities) (jujuparams.CloudResults, error) {
	ctx = ctxutil.Join(ctx, c.root.authContext)
	cloudResults := make([]jujuparams.CloudResult, len(ents.Entities))
	clouds, err := c.clouds(ctx)
	if err != nil {
		return jujuparams.CloudResults{}, mapError(err)
	}
	for i, ent := range ents.Entities {
		cloud, err := c.cloud(ent.Tag, clouds)
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
func (c cloudV5) cloud(cloudTag string, clouds map[string]jujuparams.Cloud) (*jujuparams.Cloud, error) {
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
func (c cloudV5) Clouds(ctx context.Context) (jujuparams.CloudsResult, error) {
	ctx = ctxutil.Join(ctx, c.root.authContext)
	var res jujuparams.CloudsResult
	var err error
	res.Clouds, err = c.clouds(ctx)
	return res, errgo.Mask(err)
}

func (c cloudV5) clouds(ctx context.Context) (map[string]jujuparams.Cloud, error) {
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

// UserCredentials implements the UserCredentials method of the Cloud facade.
func (c cloudV5) UserCredentials(ctx context.Context, userclouds jujuparams.UserClouds) (jujuparams.StringsResults, error) {
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
func (c cloudV5) userCredentials(ctx context.Context, ownerTag, cloudTag string) ([]string, error) {
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

func (c cloudV5) RevokeCredentialsCheckModels(ctx context.Context, args jujuparams.RevokeCredentialArgs) (jujuparams.ErrorResults, error) {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	ctx = ctxutil.Join(ctx, c.root.authContext)
	results := make([]jujuparams.ErrorResult, len(args.Credentials))
	for i, ent := range args.Credentials {
		if err := c.revokeCredential(ctx, ent.Tag, ent.Force); err != nil {
			results[i].Error = mapError(err)
		}
	}
	return jujuparams.ErrorResults{
		Results: results,
	}, nil
}

// RevokeCredentials revokes a set of cloud credentials.
func (c cloudV5) revokeCredential(ctx context.Context, tag string, force bool) error {
	var flags jem.CredentialUpdateFlags
	if force {
		flags |= jem.CredentialUpdate
	}
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
	if err := c.root.jem.RevokeCredential(ctx, params.CredentialPath{
		Cloud: params.Cloud(credtag.Cloud().Id()),
		EntityPath: params.EntityPath{
			User: params.User(credtag.Owner().Name()),
			Name: params.Name(credtag.Name()),
		},
	}, flags); err != nil {
		return errgo.Mask(err)
	}
	return nil
}

// Credential implements the Credential method of the Cloud facade.
func (c cloudV5) Credential(ctx context.Context, args jujuparams.Entities) (jujuparams.CloudCredentialResults, error) {
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
func (c cloudV5) credential(ctx context.Context, cloudCredentialTag string) (*jujuparams.CloudCredential, error) {
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
func (c cloudV5) AddCloud(ctx context.Context, args jujuparams.AddCloudArgs) error {
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
	return c.root.jem.CreateCloud(
		ctx,
		cloud,
		regions,
		jem.CreateCloudParams{
			HostCloudRegion: args.Cloud.HostCloudRegion,
			Config:          args.Cloud.Config,
			RegionConfig:    args.Cloud.RegionConfig,
		},
	)
}

// AddCredentials implements the AddCredentials method of the Cloud (v2) facade.
func (c cloudV5) AddCredentials(ctx context.Context, args jujuparams.TaggedCredentials) (jujuparams.ErrorResults, error) {
	updateResults, err := c.UpdateCredentialsCheckModels(ctx, jujuparams.UpdateCredentialArgs{
		Credentials: args.Credentials,
	})
	if err != nil {
		return jujuparams.ErrorResults{}, err
	}
	results := jujuparams.ErrorResults{
		Results: make([]jujuparams.ErrorResult, len(args.Credentials)),
	}

	// If there are any models that are using a credential and these models
	// are not going to be visible with updated credential content,
	// there will be detailed validation errors per model.
	// However, old return parameter structure could not hold this much detail and,
	// thus, per-model-per-credential errors are squashed into per-credential errors.
	for i, result := range updateResults.Results {
		var resultErrors []jujuparams.ErrorResult
		if result.Error != nil {
			resultErrors = append(resultErrors, jujuparams.ErrorResult{result.Error})
		}
		for _, m := range result.Models {
			if len(m.Errors) > 0 {
				modelErors := jujuparams.ErrorResults{m.Errors}
				combined := errors.Annotatef(modelErors.Combine(), "model %q (uuid %v)", m.ModelName, m.ModelUUID)
				resultErrors = append(resultErrors, jujuparams.ErrorResult{common.ServerError(combined)})
			}
		}
		if len(resultErrors) == 1 {
			results.Results[i].Error = resultErrors[0].Error
			continue
		}
		if len(resultErrors) > 1 {
			credentialError := jujuparams.ErrorResults{resultErrors}
			results.Results[i].Error = common.ServerError(credentialError.Combine())
		}
	}
	return results, nil
}

// CredentialContents implements the CredentialContents method of the Cloud (v2) facade.
func (c cloudV5) CredentialContents(ctx context.Context, args jujuparams.CloudCredentialArgs) (jujuparams.CredentialContentResults, error) {
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
func (c cloudV5) credentialInfo(ctx context.Context, cloudName, credentialName string, includeSecrets bool) (*jujuparams.ControllerCredentialInfo, error) {
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
func (c cloudV5) RemoveClouds(ctx context.Context, args jujuparams.Entities) (jujuparams.ErrorResults, error) {
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

func (c cloudV5) CheckCredentialsModels(ctx context.Context, args jujuparams.TaggedCredentials) (jujuparams.UpdateCredentialResults, error) {
	return c.updateCredentials(ctx, args.Credentials, jem.CredentialCheck)
}

// ModifyCloudAccess changes the cloud access granted to users.
func (c cloudV5) ModifyCloudAccess(ctx context.Context, args jujuparams.ModifyCloudAccessRequest) (jujuparams.ErrorResults, error) {
	ctx = ctxutil.Join(ctx, c.root.authContext)
	results := make([]jujuparams.ErrorResult, len(args.Changes))
	for i, change := range args.Changes {
		results[i].Error = mapError(c.modifyCloudAccess(ctx, change))
	}
	return jujuparams.ErrorResults{
		Results: results,
	}, nil
}

func (c cloudV5) modifyCloudAccess(ctx context.Context, change jujuparams.ModifyCloudAccess) error {
	userTag, err := names.ParseUserTag(change.UserTag)
	if err != nil {
		return errgo.WithCausef(err, params.ErrBadRequest, "")
	}
	cloudTag, err := names.ParseCloudTag(change.CloudTag)
	if err != nil {
		return errgo.WithCausef(err, params.ErrBadRequest, "")
	}
	var modifyf func(context.Context, params.Cloud, params.User, string) error
	switch change.Action {
	case jujuparams.GrantCloudAccess:
		modifyf = c.root.jem.GrantCloud
	case jujuparams.RevokeCloudAccess:
		modifyf = c.root.jem.RevokeCloud
	default:
		return errgo.WithCausef(nil, params.ErrBadRequest, "unsupported modify cloud action %q", change.Action)
	}
	if err := modifyf(ctx, params.Cloud(cloudTag.Id()), params.User(userTag.Id()), change.Access); err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrNotFound), errgo.Is(params.ErrUnauthorized))
	}
	return nil
}

// UpdateCredentialsCheckModels updates a set of cloud credentials' content.
// If there are any models that are using a credential and these models
// are not going to be visible with updated credential content,
// there will be detailed validation errors per model.
func (c cloudV5) UpdateCredentialsCheckModels(ctx context.Context, args jujuparams.UpdateCredentialArgs) (jujuparams.UpdateCredentialResults, error) {
	flags := jem.CredentialUpdate | jem.CredentialCheck
	if args.Force {
		flags &^= jem.CredentialCheck
	}
	return c.updateCredentials(ctx, args.Credentials, flags)
}

func (c cloudV5) updateCredentials(ctx context.Context, args []jujuparams.TaggedCredential, flags jem.CredentialUpdateFlags) (jujuparams.UpdateCredentialResults, error) {
	ctx = ctxutil.Join(ctx, c.root.authContext)
	results := jujuparams.UpdateCredentialResults{
		Results: make([]jujuparams.UpdateCredentialResult, len(args)),
	}
	for i, arg := range args {
		results.Results[i].CredentialTag = arg.Tag
		var err error
		mr, err := c.updateCredential(ctx, arg, flags)
		if err != nil {
			results.Results[i].Error = mapError(err)
		} else {
			results.Results[i] = *mr
		}
	}
	return results, nil
}

func (c cloudV5) updateCredential(ctx context.Context, cred jujuparams.TaggedCredential, flags jem.CredentialUpdateFlags) (*jujuparams.UpdateCredentialResult, error) {
	tag, err := names.ParseCloudCredentialTag(cred.Tag)
	if err != nil {
		return nil, errgo.WithCausef(err, params.ErrBadRequest, "")
	}
	ownerTag := tag.Owner()
	owner, err := user(ownerTag)
	if err != nil {
		return nil, errgo.Mask(err, errgo.Is(params.ErrBadRequest))
	}
	if err := auth.CheckIsUser(ctx, owner); err != nil {
		return nil, errgo.Mask(err, errgo.Is(params.ErrUnauthorized))
	}
	var name params.Name
	if err := name.UnmarshalText([]byte(tag.Name())); err != nil {
		return nil, errgo.Mask(err, errgo.Is(params.ErrBadRequest))
	}
	credential := mongodoc.Credential{
		Path: params.CredentialPath{
			Cloud: params.Cloud(tag.Cloud().Id()),
			EntityPath: params.EntityPath{
				User: owner,
				Name: params.Name(tag.Name()),
			},
		},
		Type:       cred.Credential.AuthType,
		Attributes: cred.Credential.Attributes,
	}
	r, err := c.root.jem.UpdateCredential(ctx, &credential, flags)
	if err != nil {
		return nil, errgo.Mask(err)
	}
	return r, nil
}

// UpdateCloud updates the specified clouds.
func (c cloudV5) UpdateCloud(ctx context.Context, args jujuparams.UpdateCloudArgs) (jujuparams.ErrorResults, error) {
	ctx = ctxutil.Join(ctx, c.root.authContext)
	results := jujuparams.ErrorResults{
		Results: make([]jujuparams.ErrorResult, len(args.Clouds)),
	}
	for i, arg := range args.Clouds {
		err := c.updateCloud(ctx, arg)
		if err != nil {
			results.Results[i].Error = mapError(err)
		}
	}
	return results, nil
}

func (c cloudV5) updateCloud(ctx context.Context, args jujuparams.AddCloudArgs) error {
	// TODO(mhilton) work out how to support updating clouds, for now
	// tell everyone they're not allowed.
	return errgo.WithCausef(nil, params.ErrForbidden, "forbidden")
}
