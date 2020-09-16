// Copyright 2016 Canonical Ltd.

package jujuapi

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/juju/apiserver/common"
	jujuparams "github.com/juju/juju/apiserver/params"
	"github.com/juju/names/v4"
	"go.uber.org/zap"
	"gopkg.in/errgo.v1"
	"gopkg.in/macaroon-bakery.v2/bakery/identchecker"
	"gopkg.in/mgo.v2/bson"

	"github.com/CanonicalLtd/jimm/internal/apiconn"
	"github.com/CanonicalLtd/jimm/internal/auth"
	"github.com/CanonicalLtd/jimm/internal/conv"
	"github.com/CanonicalLtd/jimm/internal/jem"
	"github.com/CanonicalLtd/jimm/internal/jujuapi/rpc"
	"github.com/CanonicalLtd/jimm/internal/mongodoc"
	"github.com/CanonicalLtd/jimm/internal/zapctx"
	"github.com/CanonicalLtd/jimm/params"
)

func init() {
	facadeInit["Cloud"] = func(r *controllerRoot) []int {
		addCloudMethod := rpc.Method(r.AddCloud)
		addCredentialsMethod := rpc.Method(r.AddCredentials)
		checkCredentialsModelsMethod := rpc.Method(r.CheckCredentialsModels)
		cloudMethod := rpc.Method(r.Cloud)
		cloudsMethod := rpc.Method(r.Clouds)
		credentialMethod := rpc.Method(r.Credential)
		credentialContentsMethod := rpc.Method(r.CredentialContents)
		defaultCloudMethod := rpc.Method(r.DefaultCloud)
		modifyCloudAccessMethod := rpc.Method(r.ModifyCloudAccess)
		removeCloudsMethod := rpc.Method(r.RemoveClouds)
		revokeCredentialsMethod := rpc.Method(r.RevokeCredentials)
		revokeCredentialsCheckModelsMethod := rpc.Method(r.RevokeCredentialsCheckModels)
		updateCloudMethod := rpc.Method(r.UpdateCloud)
		updateCredentialsCheckModelsMethod := rpc.Method(r.UpdateCredentialsCheckModels)
		userCredentialsMethod := rpc.Method(r.UserCredentials)

		r.AddMethod("Cloud", 1, "Cloud", cloudMethod)
		r.AddMethod("Cloud", 1, "Clouds", cloudsMethod)
		r.AddMethod("Cloud", 1, "Credential", credentialMethod)
		r.AddMethod("Cloud", 1, "DefaultCloud", defaultCloudMethod)
		r.AddMethod("Cloud", 1, "RevokeCredentials", revokeCredentialsMethod)
		// In JIMM UpdateCredentials behaves in the way AddCredentials is
		// documented to. Presumably in juju UpdateCredentials works
		// slightly differently.
		r.AddMethod("Cloud", 1, "UpdateCredentials", addCredentialsMethod)
		r.AddMethod("Cloud", 1, "UserCredentials", userCredentialsMethod)

		r.AddMethod("Cloud", 2, "AddCloud", addCloudMethod)
		r.AddMethod("Cloud", 2, "AddCredentials", addCredentialsMethod)
		r.AddMethod("Cloud", 2, "Cloud", cloudMethod)
		r.AddMethod("Cloud", 2, "Clouds", cloudsMethod)
		r.AddMethod("Cloud", 2, "Credential", credentialMethod)
		r.AddMethod("Cloud", 2, "CredentialContents", credentialContentsMethod)
		r.AddMethod("Cloud", 2, "DefaultCloud", defaultCloudMethod)
		r.AddMethod("Cloud", 2, "RemoveClouds", removeCloudsMethod)
		r.AddMethod("Cloud", 2, "RevokeCredentials", revokeCredentialsMethod)
		// In JIMM UpdateCredentials behaves in the way AddCredentials is
		// documented to. Presumably in juju UpdateCredentials works
		// slightly differently.
		r.AddMethod("Cloud", 2, "UpdateCredentials", addCredentialsMethod)
		r.AddMethod("Cloud", 2, "UserCredentials", userCredentialsMethod)

		r.AddMethod("Cloud", 3, "AddCloud", addCloudMethod)
		r.AddMethod("Cloud", 3, "AddCredentials", addCredentialsMethod)
		r.AddMethod("Cloud", 3, "CheckCredentialsModels", checkCredentialsModelsMethod)
		r.AddMethod("Cloud", 3, "Cloud", cloudMethod)
		r.AddMethod("Cloud", 3, "Clouds", cloudsMethod)
		r.AddMethod("Cloud", 3, "Credential", credentialMethod)
		r.AddMethod("Cloud", 3, "CredentialContents", credentialContentsMethod)
		r.AddMethod("Cloud", 3, "ModifyCloudAccess", modifyCloudAccessMethod)
		r.AddMethod("Cloud", 3, "DefaultCloud", defaultCloudMethod)
		r.AddMethod("Cloud", 3, "RemoveClouds", removeCloudsMethod)
		r.AddMethod("Cloud", 3, "RevokeCredentialsCheckModels", revokeCredentialsCheckModelsMethod)
		r.AddMethod("Cloud", 3, "UpdateCredentialsCheckModels", updateCredentialsCheckModelsMethod)
		r.AddMethod("Cloud", 3, "UserCredentials", userCredentialsMethod)

		r.AddMethod("Cloud", 4, "AddCloud", addCloudMethod)
		r.AddMethod("Cloud", 4, "AddCredentials", addCredentialsMethod)
		r.AddMethod("Cloud", 4, "CheckCredentialsModels", checkCredentialsModelsMethod)
		r.AddMethod("Cloud", 4, "Cloud", cloudMethod)
		r.AddMethod("Cloud", 4, "Clouds", cloudsMethod)
		r.AddMethod("Cloud", 4, "Credential", credentialMethod)
		r.AddMethod("Cloud", 4, "CredentialContents", credentialContentsMethod)
		r.AddMethod("Cloud", 4, "DefaultCloud", defaultCloudMethod)
		r.AddMethod("Cloud", 4, "ModifyCloudAccess", modifyCloudAccessMethod)
		r.AddMethod("Cloud", 4, "RemoveClouds", removeCloudsMethod)
		r.AddMethod("Cloud", 4, "RevokeCredentialsCheckModels", revokeCredentialsCheckModelsMethod)
		r.AddMethod("Cloud", 4, "UpdateCloud", updateCloudMethod)

		r.AddMethod("Cloud", 4, "UpdateCredentialsCheckModels", updateCredentialsCheckModelsMethod)
		r.AddMethod("Cloud", 4, "UserCredentials", userCredentialsMethod)

		r.AddMethod("Cloud", 5, "AddCloud", addCloudMethod)
		r.AddMethod("Cloud", 5, "AddCredentials", addCredentialsMethod)
		r.AddMethod("Cloud", 5, "CheckCredentialsModels", checkCredentialsModelsMethod)
		r.AddMethod("Cloud", 5, "Cloud", cloudMethod)
		r.AddMethod("Cloud", 5, "Clouds", cloudsMethod)
		r.AddMethod("Cloud", 5, "Credential", credentialMethod)
		r.AddMethod("Cloud", 5, "CredentialContents", credentialContentsMethod)
		// Version 5 removed DefaultCloud
		r.AddMethod("Cloud", 5, "ModifyCloudAccess", modifyCloudAccessMethod)
		r.AddMethod("Cloud", 5, "RemoveClouds", removeCloudsMethod)
		r.AddMethod("Cloud", 5, "RevokeCredentialsCheckModels", revokeCredentialsCheckModelsMethod)
		r.AddMethod("Cloud", 5, "UpdateCloud", updateCloudMethod)
		r.AddMethod("Cloud", 5, "UpdateCredentialsCheckModels", updateCredentialsCheckModelsMethod)
		r.AddMethod("Cloud", 5, "UserCredentials", userCredentialsMethod)

		return []int{1, 2, 3, 4, 5}
	}
}

// RevokeCredentials implements the RevokeCredentials method used in
// version 1 & 2 of the Cloud facade.
func (r *controllerRoot) RevokeCredentials(ctx context.Context, args jujuparams.Entities) (jujuparams.ErrorResults, error) {
	creds := make([]jujuparams.RevokeCredentialArg, len(args.Entities))
	for i, e := range args.Entities {
		creds[i].Tag = e.Tag
		creds[i].Force = true
	}
	return r.RevokeCredentialsCheckModels(ctx, jujuparams.RevokeCredentialArgs{
		Credentials: creds,
	})
}

// DefaultCloud implements the DefaultCloud method of the Cloud facade.
// It returns a default cloud if there is only one cloud available.
func (r *controllerRoot) DefaultCloud(ctx context.Context) (jujuparams.StringResult, error) {
	ctx = auth.ContextWithIdentity(ctx, r.identity)
	var result jujuparams.StringResult
	clouds, err := r.clouds(ctx)
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

// Cloud implements the Cloud method of the Cloud facade.
func (r *controllerRoot) Cloud(ctx context.Context, ents jujuparams.Entities) (jujuparams.CloudResults, error) {
	ctx = auth.ContextWithIdentity(ctx, r.identity)
	cloudResults := make([]jujuparams.CloudResult, len(ents.Entities))
	clouds, err := r.clouds(ctx)
	if err != nil {
		return jujuparams.CloudResults{}, mapError(err)
	}
	for i, ent := range ents.Entities {
		cloud, err := r.cloud(ent.Tag, clouds)
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
func (r *controllerRoot) cloud(cloudTag string, clouds map[string]jujuparams.Cloud) (*jujuparams.Cloud, error) {
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
func (r *controllerRoot) Clouds(ctx context.Context) (jujuparams.CloudsResult, error) {
	ctx = auth.ContextWithIdentity(ctx, r.identity)
	var res jujuparams.CloudsResult
	var err error
	res.Clouds, err = r.clouds(ctx)
	return res, errgo.Mask(err)
}

func (r *controllerRoot) clouds(ctx context.Context) (map[string]jujuparams.Cloud, error) {
	iter := r.jem.DB.GetCloudRegionsIter(ctx)
	results := map[string]jujuparams.Cloud{}
	var v mongodoc.CloudRegion
	for iter.Next(ctx, &v) {
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
	if err := iter.Err(ctx); err != nil {
		return nil, errgo.Notef(err, "cannot query")
	}
	return results, nil
}

// UserCredentials implements the UserCredentials method of the Cloud facade.
func (r *controllerRoot) UserCredentials(ctx context.Context, userclouds jujuparams.UserClouds) (jujuparams.StringsResults, error) {
	ctx = auth.ContextWithIdentity(ctx, r.identity)
	results := make([]jujuparams.StringsResult, len(userclouds.UserClouds))
	for i, ent := range userclouds.UserClouds {
		creds, err := r.userCredentials(ctx, ent.UserTag, ent.CloudTag)
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
func (r *controllerRoot) userCredentials(ctx context.Context, ownerTag, cloudTag string) ([]string, error) {
	ot, err := names.ParseUserTag(ownerTag)
	if err != nil {
		return nil, errgo.WithCausef(err, params.ErrBadRequest, "")
	}
	owner, err := conv.FromUserTag(ot)
	if err != nil {
		return nil, errgo.Mask(err, errgo.Is(conv.ErrLocalUser))
	}
	cld, err := names.ParseCloudTag(cloudTag)
	if err != nil {
		return nil, errgo.WithCausef(err, params.ErrBadRequest, "")
	}
	var cloudCreds []string
	it := r.jem.DB.NewCanReadIter(ctx, r.jem.DB.Credentials().Find(
		bson.D{{
			"path.entitypath.user", owner,
		}, {
			"path.cloud", cld.Id(),
		}, {
			"revoked", false,
		}},
	).Iter())
	var cred mongodoc.Credential
	for it.Next(ctx, &cred) {
		cloudCreds = append(cloudCreds, conv.ToCloudCredentialTag(cred.Path.ToParams()).String())
	}

	return cloudCreds, errgo.Mask(it.Err(ctx))
}

func (r *controllerRoot) RevokeCredentialsCheckModels(ctx context.Context, args jujuparams.RevokeCredentialArgs) (jujuparams.ErrorResults, error) {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	ctx = auth.ContextWithIdentity(ctx, r.identity)
	results := make([]jujuparams.ErrorResult, len(args.Credentials))
	for i, ent := range args.Credentials {
		if err := r.revokeCredential(ctx, ent.Tag, ent.Force); err != nil {
			results[i].Error = mapError(err)
		}
	}
	return jujuparams.ErrorResults{
		Results: results,
	}, nil
}

// RevokeCredentials revokes a set of cloud credentials.
func (r *controllerRoot) revokeCredential(ctx context.Context, tag string, force bool) error {
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
	if err := auth.CheckIsUser(ctx, r.identity, params.User(credtag.Owner().Name())); err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrUnauthorized))
	}
	if err := r.jem.RevokeCredential(ctx, params.CredentialPath{
		Cloud: params.Cloud(credtag.Cloud().Id()),
		User:  params.User(credtag.Owner().Name()),
		Name:  params.CredentialName(credtag.Name()),
	}, flags); err != nil {
		return errgo.Mask(err)
	}
	return nil
}

// Credential implements the Credential method of the Cloud facade.
func (r *controllerRoot) Credential(ctx context.Context, args jujuparams.Entities) (jujuparams.CloudCredentialResults, error) {
	ctx = auth.ContextWithIdentity(ctx, r.identity)
	results := make([]jujuparams.CloudCredentialResult, len(args.Entities))
	for i, e := range args.Entities {
		cred, err := r.credential(ctx, e.Tag)
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
func (r *controllerRoot) credential(ctx context.Context, cloudCredentialTag string) (*jujuparams.CloudCredential, error) {
	cct, err := names.ParseCloudCredentialTag(cloudCredentialTag)
	if err != nil {
		return nil, errgo.WithCausef(err, params.ErrBadRequest, "")

	}
	ownerTag := cct.Owner()
	owner, err := conv.FromUserTag(ownerTag)
	if err != nil {
		return nil, errgo.Mask(err, errgo.Is(conv.ErrLocalUser))
	}

	credPath := params.CredentialPath{
		Cloud: params.Cloud(cct.Cloud().Id()),
		User:  owner,
		Name:  params.CredentialName(cct.Name()),
	}

	cred, err := r.jem.GetCredential(ctx, r.identity, credPath)
	if err != nil {
		return nil, errgo.Mask(err, errgo.Is(params.ErrNotFound), errgo.Is(params.ErrUnauthorized))
	}
	if cred.Revoked {
		return nil, errgo.WithCausef(nil, params.ErrNotFound, "credential %q not found", cct.Id())
	}
	if err := r.jem.FillCredentialAttributes(ctx, cred); err != nil {
		return nil, errgo.Mask(err)
	}
	schema, err := r.credentialSchema(ctx, cred.Path.ToParams().Cloud, cred.Type)
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
func (r *controllerRoot) AddCloud(ctx context.Context, args jujuparams.AddCloudArgs) error {
	return r.jem.AddCloud(ctx, r.identity, params.Cloud(args.Name), args.Cloud)
}

// AddCredentials implements the AddCredentials method of the Cloud (v2) facade.
func (r *controllerRoot) AddCredentials(ctx context.Context, args jujuparams.TaggedCredentials) (jujuparams.ErrorResults, error) {
	updateResults, err := r.UpdateCredentialsCheckModels(ctx, jujuparams.UpdateCredentialArgs{
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

// CredentialContents implements the CredentialContents method of the Cloud (v5) facade.
func (r *controllerRoot) CredentialContents(ctx context.Context, args jujuparams.CloudCredentialArgs) (jujuparams.CredentialContentResults, error) {
	ctx = auth.ContextWithIdentity(ctx, r.identity)
	if len(args.Credentials) == 0 {
		creds, err := r.findUserCredentials(ctx)
		if err != nil {
			return jujuparams.CredentialContentResults{}, errgo.Mask(err)
		}
		args.Credentials = creds
	}
	results := make([]jujuparams.CredentialContentResult, len(args.Credentials))
	for i, arg := range args.Credentials {
		credInfo, err := r.credentialInfo(ctx, arg.CloudName, arg.CredentialName, args.IncludeSecrets)
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

// findUserCredentials finds all credentials owned by the authenticated user.
func (r *controllerRoot) findUserCredentials(ctx context.Context) ([]jujuparams.CloudCredentialArg, error) {
	ctx = auth.ContextWithIdentity(ctx, r.identity)

	username := r.identity.Id()
	query := bson.D{{"path.entitypath.user", username}, {"revoked", false}}
	iter := r.jem.DB.NewCanReadIter(ctx, r.jem.DB.Credentials().Find(query).Iter())
	defer iter.Close(ctx)

	var creds []jujuparams.CloudCredentialArg
	var cred mongodoc.Credential
	for iter.Next(ctx, &cred) {
		creds = append(creds, jujuparams.CloudCredentialArg{CloudName: cred.Path.Cloud, CredentialName: cred.Path.Name})
	}
	return creds, errgo.Mask(iter.Err(ctx))
}

// credentialInfo returns Juju API information on the given credential
// within the given cloud. If includeSecrets is true, secret information
// will be included too.
func (r *controllerRoot) credentialInfo(ctx context.Context, cloudName, credentialName string, includeSecrets bool) (*jujuparams.ControllerCredentialInfo, error) {
	credPath := params.CredentialPath{
		Cloud: params.Cloud(cloudName),
		User:  params.User(r.identity.Id()),
		Name:  params.CredentialName(credentialName),
	}
	cred, err := r.jem.GetCredential(ctx, r.identity, credPath)
	if err != nil {
		return nil, errgo.Mask(err, errgo.Is(params.ErrNotFound), errgo.Is(params.ErrUnauthorized))
	}
	if cred.Revoked {
		return nil, errgo.WithCausef(nil, params.ErrNotFound, "")
	}
	if err := r.jem.FillCredentialAttributes(ctx, cred); err != nil {
		return nil, errgo.Mask(err)
	}
	schema, err := r.credentialSchema(ctx, cred.Path.ToParams().Cloud, cred.Type)
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
	r.doModels(ctx, func(ctx context.Context, model *mongodoc.Model) error {
		if model.Credential != mongodoc.CredentialPathFromParams(credPath) {
			return nil
		}
		access := jujuparams.ModelReadAccess
		switch {
		case params.User(r.identity.Id()) == model.Path.User:
			access = jujuparams.ModelAdminAccess
		case auth.CheckACL(ctx, r.identity, model.ACL.Admin) == nil:
			access = jujuparams.ModelAdminAccess
		case auth.CheckACL(ctx, r.identity, model.ACL.Write) == nil:
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
func (r *controllerRoot) RemoveClouds(ctx context.Context, args jujuparams.Entities) (jujuparams.ErrorResults, error) {
	result := jujuparams.ErrorResults{
		Results: make([]jujuparams.ErrorResult, len(args.Entities)),
	}
	for i, entity := range args.Entities {
		tag, err := names.ParseCloudTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = mapError(err)
			continue
		}
		err = r.jem.RemoveCloud(ctx, r.identity, params.Cloud(tag.Id()))
		if err != nil {
			result.Results[i].Error = mapError(err)
		}
	}
	return result, nil
}

func (r *controllerRoot) CheckCredentialsModels(ctx context.Context, args jujuparams.TaggedCredentials) (jujuparams.UpdateCredentialResults, error) {
	return r.updateCredentials(ctx, args.Credentials, jem.CredentialCheck)
}

// ModifyCloudAccess changes the cloud access granted to users.
func (r *controllerRoot) ModifyCloudAccess(ctx context.Context, args jujuparams.ModifyCloudAccessRequest) (jujuparams.ErrorResults, error) {
	results := make([]jujuparams.ErrorResult, len(args.Changes))
	for i, change := range args.Changes {
		results[i].Error = mapError(r.modifyCloudAccess(ctx, change))
	}
	return jujuparams.ErrorResults{
		Results: results,
	}, nil
}

func (r *controllerRoot) modifyCloudAccess(ctx context.Context, change jujuparams.ModifyCloudAccess) error {
	userTag, err := names.ParseUserTag(change.UserTag)
	if err != nil {
		return errgo.WithCausef(err, params.ErrBadRequest, "")
	}
	cloudTag, err := names.ParseCloudTag(change.CloudTag)
	if err != nil {
		return errgo.WithCausef(err, params.ErrBadRequest, "")
	}
	var modifyf func(context.Context, identchecker.ACLIdentity, params.Cloud, params.User, string) error
	switch change.Action {
	case jujuparams.GrantCloudAccess:
		modifyf = r.jem.GrantCloud
	case jujuparams.RevokeCloudAccess:
		modifyf = r.jem.RevokeCloud
	default:
		return errgo.WithCausef(nil, params.ErrBadRequest, "unsupported modify cloud action %q", change.Action)
	}
	if err := modifyf(ctx, r.identity, params.Cloud(cloudTag.Id()), params.User(userTag.Id()), change.Access); err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrNotFound), errgo.Is(params.ErrUnauthorized))
	}
	return nil
}

// UpdateCredentialsCheckModels updates a set of cloud credentials' content.
// If there are any models that are using a credential and these models
// are not going to be visible with updated credential content,
// there will be detailed validation errors per model.
func (r *controllerRoot) UpdateCredentialsCheckModels(ctx context.Context, args jujuparams.UpdateCredentialArgs) (jujuparams.UpdateCredentialResults, error) {
	flags := jem.CredentialUpdate | jem.CredentialCheck
	if args.Force {
		flags &^= jem.CredentialCheck
	}
	return r.updateCredentials(ctx, args.Credentials, flags)
}

func (r *controllerRoot) updateCredentials(ctx context.Context, args []jujuparams.TaggedCredential, flags jem.CredentialUpdateFlags) (jujuparams.UpdateCredentialResults, error) {
	ctx = auth.ContextWithIdentity(ctx, r.identity)
	results := jujuparams.UpdateCredentialResults{
		Results: make([]jujuparams.UpdateCredentialResult, len(args)),
	}
	for i, arg := range args {
		var err error
		models, err := r.updateCredential(ctx, arg, flags)
		results.Results[i] = jujuparams.UpdateCredentialResult{
			CredentialTag: arg.Tag,
			Error:         mapError(err),
			Models:        models,
		}
		results.Results[i].CredentialTag = arg.Tag
	}
	return results, nil
}

func (r *controllerRoot) updateCredential(ctx context.Context, cred jujuparams.TaggedCredential, flags jem.CredentialUpdateFlags) ([]jujuparams.UpdateCredentialModelResult, error) {
	tag, err := names.ParseCloudCredentialTag(cred.Tag)
	if err != nil {
		return nil, errgo.WithCausef(err, params.ErrBadRequest, "")
	}
	ownerTag := tag.Owner()
	owner, err := conv.FromUserTag(ownerTag)
	if err != nil {
		return nil, errgo.Mask(err, errgo.Is(conv.ErrLocalUser))
	}
	if err := auth.CheckIsUser(ctx, r.identity, owner); err != nil {
		return nil, errgo.Mask(err, errgo.Is(params.ErrUnauthorized))
	}
	var name params.Name
	if err := name.UnmarshalText([]byte(tag.Name())); err != nil {
		return nil, errgo.Mask(err, errgo.Is(params.ErrBadRequest))
	}
	credential := mongodoc.Credential{
		Path: mongodoc.CredentialPath{
			Cloud: tag.Cloud().Id(),
			EntityPath: mongodoc.EntityPath{
				User: string(owner),
				Name: tag.Name(),
			},
		},
		Type:       cred.Credential.AuthType,
		Attributes: cred.Credential.Attributes,
	}
	modelResults, err := r.jem.UpdateCredential(ctx, &credential, flags)
	return modelResults, errgo.Mask(err, apiconn.IsAPIError)
}

// UpdateCloud updates the specified clouds.
func (r *controllerRoot) UpdateCloud(ctx context.Context, args jujuparams.UpdateCloudArgs) (jujuparams.ErrorResults, error) {
	ctx = auth.ContextWithIdentity(ctx, r.identity)
	results := jujuparams.ErrorResults{
		Results: make([]jujuparams.ErrorResult, len(args.Clouds)),
	}
	for i, arg := range args.Clouds {
		err := r.updateCloud(ctx, arg)
		if err != nil {
			results.Results[i].Error = mapError(err)
		}
	}
	return results, nil
}

func (r *controllerRoot) updateCloud(ctx context.Context, args jujuparams.AddCloudArgs) error {
	// TODO(mhilton) work out how to support updating clouds, for now
	// tell everyone they're not allowed.
	return errgo.WithCausef(nil, params.ErrForbidden, "forbidden")
}
