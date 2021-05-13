// Copyright 2016 Canonical Ltd.

package jujuapi

import (
	"context"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery/identchecker"
	"github.com/juju/errors"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	jujuparams "github.com/juju/juju/apiserver/params"
	"github.com/juju/names/v4"
	"gopkg.in/errgo.v1"

	"github.com/CanonicalLtd/jimm/internal/apiconn"
	"github.com/CanonicalLtd/jimm/internal/cloudcred"
	"github.com/CanonicalLtd/jimm/internal/conv"
	"github.com/CanonicalLtd/jimm/internal/jem"
	"github.com/CanonicalLtd/jimm/internal/jujuapi/rpc"
	"github.com/CanonicalLtd/jimm/internal/mongodoc"
	"github.com/CanonicalLtd/jimm/params"
)

func init() {
	facadeInit["Cloud"] = func(r *controllerRoot) []int {
		addCloudMethod := rpc.Method(r.AddCloud)
		addCredentialsMethod := rpc.Method(r.AddCredentials)
		checkCredentialsModelsMethod := rpc.Method(r.CheckCredentialsModels)
		cloudMethod := rpc.Method(r.Cloud)
		cloudInfoMethod := rpc.Method(r.CloudInfo)
		cloudsMethod := rpc.Method(r.Clouds)
		credentialMethod := rpc.Method(r.Credential)
		credentialContentsMethod := rpc.Method(r.CredentialContents)
		defaultCloudMethod := rpc.Method(r.DefaultCloud)
		listCloudInfoMethod := rpc.Method(r.ListCloudInfo)
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
		r.AddMethod("Cloud", 5, "CloudInfo", cloudInfoMethod)
		r.AddMethod("Cloud", 5, "Clouds", cloudsMethod)
		r.AddMethod("Cloud", 5, "Credential", credentialMethod)
		r.AddMethod("Cloud", 5, "CredentialContents", credentialContentsMethod)
		// Version 5 removed DefaultCloud
		r.AddMethod("Cloud", 5, "ListCloudInfo", listCloudInfoMethod)
		r.AddMethod("Cloud", 5, "ModifyCloudAccess", modifyCloudAccessMethod)
		r.AddMethod("Cloud", 5, "RemoveClouds", removeCloudsMethod)
		r.AddMethod("Cloud", 5, "RevokeCredentialsCheckModels", revokeCredentialsCheckModelsMethod)
		r.AddMethod("Cloud", 5, "UpdateCloud", updateCloudMethod)
		r.AddMethod("Cloud", 5, "UpdateCredentialsCheckModels", updateCredentialsCheckModelsMethod)
		r.AddMethod("Cloud", 5, "UserCredentials", userCredentialsMethod)

		r.AddMethod("Cloud", 6, "AddCloud", addCloudMethod)
		r.AddMethod("Cloud", 6, "AddCredentials", addCredentialsMethod)
		r.AddMethod("Cloud", 6, "CheckCredentialsModels", checkCredentialsModelsMethod)
		r.AddMethod("Cloud", 6, "Cloud", cloudMethod)
		r.AddMethod("Cloud", 6, "CloudInfo", cloudInfoMethod)
		r.AddMethod("Cloud", 6, "Clouds", cloudsMethod)
		r.AddMethod("Cloud", 6, "Credential", credentialMethod)
		r.AddMethod("Cloud", 6, "CredentialContents", credentialContentsMethod)
		r.AddMethod("Cloud", 6, "ListCloudInfo", listCloudInfoMethod)
		r.AddMethod("Cloud", 6, "ModifyCloudAccess", modifyCloudAccessMethod)
		r.AddMethod("Cloud", 6, "RemoveClouds", removeCloudsMethod)
		r.AddMethod("Cloud", 6, "RevokeCredentialsCheckModels", revokeCredentialsCheckModelsMethod)
		r.AddMethod("Cloud", 6, "UpdateCloud", updateCloudMethod)
		r.AddMethod("Cloud", 6, "UpdateCredentialsCheckModels", updateCredentialsCheckModelsMethod)
		r.AddMethod("Cloud", 6, "UserCredentials", userCredentialsMethod)

		r.AddMethod("Cloud", 7, "AddCloud", addCloudMethod)
		r.AddMethod("Cloud", 7, "AddCredentials", addCredentialsMethod)
		r.AddMethod("Cloud", 7, "CheckCredentialsModels", checkCredentialsModelsMethod)
		r.AddMethod("Cloud", 7, "Cloud", cloudMethod)
		r.AddMethod("Cloud", 7, "CloudInfo", cloudInfoMethod)
		r.AddMethod("Cloud", 7, "Clouds", cloudsMethod)
		r.AddMethod("Cloud", 7, "Credential", credentialMethod)
		r.AddMethod("Cloud", 7, "CredentialContents", credentialContentsMethod)
		r.AddMethod("Cloud", 7, "ListCloudInfo", listCloudInfoMethod)
		r.AddMethod("Cloud", 7, "ModifyCloudAccess", modifyCloudAccessMethod)
		r.AddMethod("Cloud", 7, "RemoveClouds", removeCloudsMethod)
		r.AddMethod("Cloud", 7, "RevokeCredentialsCheckModels", revokeCredentialsCheckModelsMethod)
		r.AddMethod("Cloud", 7, "UpdateCloud", updateCloudMethod)
		r.AddMethod("Cloud", 7, "UpdateCredentialsCheckModels", updateCredentialsCheckModelsMethod)
		r.AddMethod("Cloud", 7, "UserCredentials", userCredentialsMethod)

		return []int{1, 2, 3, 4, 5, 6, 7}
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
	var result jujuparams.StringResult
	err := r.jem.ForEachCloud(ctx, r.identity, func(cld *jem.Cloud) error {
		if result.Result == "" {
			result.Result = conv.ToCloudTag(cld.Name).String()
			return nil
		}
		result.Result = ""
		return errgo.WithCausef(nil, params.ErrNotFound, "no default cloud")
	})
	if err == nil && result.Result == "" {
		// If there are no clouds then there can be no default cloud.
		err = errgo.WithCausef(nil, params.ErrNotFound, "no default cloud")
	}
	return result, err
}

// Cloud implements the Cloud method of the Cloud facade.
func (r *controllerRoot) Cloud(ctx context.Context, ents jujuparams.Entities) (jujuparams.CloudResults, error) {
	cloudResults := make([]jujuparams.CloudResult, len(ents.Entities))
	for i, ent := range ents.Entities {
		tag, err := names.ParseCloudTag(ent.Tag)
		if err != nil {
			cloudResults[i].Error = mapError(errgo.WithCausef(err, params.ErrBadRequest, ""))
			continue
		}
		cloud := jem.Cloud{Name: params.Cloud(tag.Id())}
		if err := r.jem.GetCloud(ctx, r.identity, &cloud); err != nil {
			cloudResults[i].Error = mapError(err)
			continue
		}
		cloudResults[i].Cloud = &jujuparams.Cloud{
			Type:             cloud.Type,
			AuthTypes:        cloud.AuthTypes,
			Endpoint:         cloud.Endpoint,
			IdentityEndpoint: cloud.IdentityEndpoint,
			StorageEndpoint:  cloud.StorageEndpoint,
			Regions:          cloud.Regions,
			CACertificates:   cloud.CACertificates,
		}
	}
	return jujuparams.CloudResults{
		Results: cloudResults,
	}, nil
}

// Clouds implements the Clouds method on the Cloud facade.
func (r *controllerRoot) Clouds(ctx context.Context) (jujuparams.CloudsResult, error) {
	res := jujuparams.CloudsResult{
		Clouds: make(map[string]jujuparams.Cloud),
	}
	err := r.jem.ForEachCloud(ctx, r.identity, func(cld *jem.Cloud) error {
		res.Clouds[conv.ToCloudTag(cld.Name).String()] = jujuparams.Cloud{
			Type:             cld.Type,
			AuthTypes:        cld.AuthTypes,
			Endpoint:         cld.Endpoint,
			IdentityEndpoint: cld.IdentityEndpoint,
			StorageEndpoint:  cld.StorageEndpoint,
			Regions:          cld.Regions,
			CACertificates:   cld.CACertificates,
		}
		return nil
	})
	return res, errgo.Mask(err)
}

// UserCredentials implements the UserCredentials method of the Cloud facade.
func (r *controllerRoot) UserCredentials(ctx context.Context, userclouds jujuparams.UserClouds) (jujuparams.StringsResults, error) {
	results := make([]jujuparams.StringsResult, len(userclouds.UserClouds))
	for i, ent := range userclouds.UserClouds {
		owner, err := conv.ParseUserTag(ent.UserTag)
		if err != nil {
			results[i].Error = mapError(err)
			continue
		}
		cld, err := names.ParseCloudTag(ent.CloudTag)
		if err != nil {
			results[i].Error = mapError(errgo.WithCausef(err, params.ErrBadRequest, ""))
			continue
		}
		err = r.jem.ForEachCredential(ctx, r.identity, owner, params.Cloud(cld.Id()), func(c *mongodoc.Credential) error {
			results[i].Result = append(results[i].Result, conv.ToCloudCredentialTag(c.Path).String())
			return nil
		})
		results[i].Error = mapError(err)
	}

	return jujuparams.StringsResults{
		Results: results,
	}, nil
}

func (r *controllerRoot) RevokeCredentialsCheckModels(ctx context.Context, args jujuparams.RevokeCredentialArgs) (jujuparams.ErrorResults, error) {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
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
	path, err := conv.FromCloudCredentialTag(credtag)
	if err != nil {
		if errgo.Cause(err) == conv.ErrLocalUser {
			// such a credential will not have been uploaded, so it exits.
			return nil
		}
		return errgo.Mask(err)
	}
	if err := r.jem.RevokeCredential(ctx, r.identity, &mongodoc.Credential{
		Path: path,
	}, flags); err != nil {
		return errgo.Mask(err)
	}
	return nil
}

// Credential implements the Credential method of the Cloud facade.
func (r *controllerRoot) Credential(ctx context.Context, args jujuparams.Entities) (jujuparams.CloudCredentialResults, error) {
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
	path, err := conv.FromCloudCredentialTag(cct)
	if err != nil {
		return nil, errgo.Mask(err, errgo.Is(conv.ErrLocalUser))
	}

	cred := mongodoc.Credential{
		Path: path,
	}
	if err := r.jem.GetCredential(ctx, r.identity, &cred); err != nil {
		return nil, errgo.Mask(err, errgo.Is(params.ErrNotFound), errgo.Is(params.ErrUnauthorized))
	}
	if cred.Revoked {
		return nil, errgo.WithCausef(nil, params.ErrNotFound, "credential %q not found", cct.Id())
	}
	if err := r.jem.FillCredentialAttributes(ctx, &cred); err != nil {
		return nil, errgo.Mask(err)
	}
	cc := jujuparams.CloudCredential{
		AuthType:   cred.Type,
		Attributes: make(map[string]string),
	}
	for k, v := range cred.Attributes {
		if cloudcred.IsVisibleAttribute(cred.ProviderType, cred.Type, k) {
			cc.Attributes[k] = v
		} else {
			cc.Redacted = append(cc.Redacted, k)
		}
	}
	return &cc, nil
}

// AddCloud implements the AddCloud method of the Cloud (v2) facade.
func (r *controllerRoot) AddCloud(ctx context.Context, args jujuparams.AddCloudArgs) error {
	return r.jem.AddHostedCloud(ctx, r.identity, params.Cloud(args.Name), args.Cloud)
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
				resultErrors = append(resultErrors, jujuparams.ErrorResult{apiservererrors.ServerError(combined)})
			}
		}
		if len(resultErrors) == 1 {
			results.Results[i].Error = resultErrors[0].Error
			continue
		}
		if len(resultErrors) > 1 {
			credentialError := jujuparams.ErrorResults{resultErrors}
			results.Results[i].Error = apiservererrors.ServerError(credentialError.Combine())
		}
	}
	return results, nil
}

// CredentialContents implements the CredentialContents method of the Cloud (v5) facade.
func (r *controllerRoot) CredentialContents(ctx context.Context, args jujuparams.CloudCredentialArgs) (jujuparams.CredentialContentResults, error) {
	// calculate the model access for all known models.
	modelAccess := make(map[mongodoc.CredentialPath][]jujuparams.ModelAccess)
	err := r.jem.ForEachModel(ctx, r.identity, jujuparams.ModelReadAccess, func(m *mongodoc.Model) error {
		access, err := r.modelAccess(ctx, m)
		if err != nil {
			return errgo.Mask(err)
		}
		modelAccess[m.Credential] = append(modelAccess[m.Credential], jujuparams.ModelAccess{
			Model:  string(m.Path.Name),
			Access: string(access),
		})
		return nil
	})
	if err != nil {
		return jujuparams.CredentialContentResults{}, errgo.Mask(err)
	}

	credentialContents := func(c *mongodoc.Credential) *jujuparams.ControllerCredentialInfo {
		attr := make(map[string]string)
		for k, v := range c.Attributes {
			if args.IncludeSecrets || cloudcred.IsVisibleAttribute(c.ProviderType, c.Type, k) {
				attr[k] = v
			}
		}
		return &jujuparams.ControllerCredentialInfo{
			Content: jujuparams.CredentialContent{
				Name:       string(c.Path.Name),
				Cloud:      string(c.Path.Cloud),
				AuthType:   c.Type,
				Attributes: attr,
			},
			Models: modelAccess[c.Path],
		}
	}

	results := make([]jujuparams.CredentialContentResult, len(args.Credentials))
	for i, arg := range args.Credentials {
		cred := mongodoc.Credential{
			Path: mongodoc.CredentialPath{
				Cloud: arg.CloudName,
				EntityPath: mongodoc.EntityPath{
					User: r.identity.Id(),
					Name: arg.CredentialName,
				},
			},
		}
		if err := r.jem.GetCredential(ctx, r.identity, &cred); err != nil {
			results[i].Error = mapError(err)
			continue
		}
		if err := r.jem.FillCredentialAttributes(ctx, &cred); err != nil {
			results[i].Error = mapError(err)
			continue
		}
		results[i].Result = credentialContents(&cred)
	}
	if len(results) > 0 {
		return jujuparams.CredentialContentResults{Results: results}, nil
	}

	err = r.jem.ForEachCredential(ctx, r.identity, params.User(r.identity.Id()), "", func(c *mongodoc.Credential) error {
		var result jujuparams.CredentialContentResult
		err := r.jem.FillCredentialAttributes(ctx, c)
		if err == nil {
			result.Result = credentialContents(c)
		}
		result.Error = mapError(err)
		results = append(results, result)
		return nil
	})

	return jujuparams.CredentialContentResults{Results: results}, errgo.Mask(err)
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
	user, err := conv.ParseUserTag(change.UserTag)
	if err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrBadRequest), errgo.Is(conv.ErrLocalUser))
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
	if err := modifyf(ctx, r.identity, params.Cloud(cloudTag.Id()), user, change.Access); err != nil {
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
	path, err := conv.FromCloudCredentialTag(tag)
	if err != nil {
		return nil, errgo.Mask(err, errgo.Is(conv.ErrLocalUser))
	}

	var name params.Name
	if err := name.UnmarshalText([]byte(tag.Name())); err != nil {
		return nil, errgo.Mask(err, errgo.Is(params.ErrBadRequest))
	}
	credential := mongodoc.Credential{
		Path:       path,
		Type:       cred.Credential.AuthType,
		Attributes: cred.Credential.Attributes,
	}
	modelResults, err := r.jem.UpdateCredential(ctx, r.identity, &credential, flags)
	return modelResults, errgo.Mask(err, apiconn.IsAPIError)
}

// UpdateCloud updates the specified clouds.
func (r *controllerRoot) UpdateCloud(ctx context.Context, args jujuparams.UpdateCloudArgs) (jujuparams.ErrorResults, error) {
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

// CloudInfo implements the cloud facades CloudInfo method.
func (r *controllerRoot) CloudInfo(ctx context.Context, args jujuparams.Entities) (jujuparams.CloudInfoResults, error) {
	results := make([]jujuparams.CloudInfoResult, len(args.Entities))
	for i, ent := range args.Entities {
		tag, err := names.ParseCloudTag(ent.Tag)
		if err != nil {
			results[i].Error = mapError(errgo.WithCausef(err, params.ErrBadRequest, ""))
			continue
		}
		c := jem.Cloud{Name: params.Cloud(tag.Id())}
		if err := r.jem.GetCloud(ctx, r.identity, &c); err != nil {
			results[i].Error = mapError(err)
			continue
		}
		results[i].Result = &jujuparams.CloudInfo{
			CloudDetails: jujuparams.CloudDetails{
				Type:             c.Type,
				AuthTypes:        c.AuthTypes,
				Endpoint:         c.Endpoint,
				IdentityEndpoint: c.IdentityEndpoint,
				StorageEndpoint:  c.StorageEndpoint,
				Regions:          c.Regions,
			},
			Users: c.Users,
		}
	}
	return jujuparams.CloudInfoResults{
		Results: results,
	}, nil
}

// ListCloudInfo implements the ListCloudInfo method on the cloud facade.
// ListClouds ignores the request parameters and returns the clouds visible
// to the current user.
func (r *controllerRoot) ListCloudInfo(ctx context.Context, _ jujuparams.ListCloudsRequest) (jujuparams.ListCloudInfoResults, error) {
	// TODO(mhilton) support the arguments.
	var results []jujuparams.ListCloudInfoResult
	err := r.jem.ForEachCloud(ctx, r.identity, func(c *jem.Cloud) error {
		var access string
		for _, u := range c.Users {
			if u.UserName == conv.ToUserTag(params.User(r.identity.Id())).Id() {
				access = u.Access
			}
		}
		results = append(results, jujuparams.ListCloudInfoResult{
			Result: &jujuparams.ListCloudInfo{
				CloudDetails: jujuparams.CloudDetails{
					Type:             c.Type,
					AuthTypes:        c.AuthTypes,
					Endpoint:         c.Endpoint,
					IdentityEndpoint: c.IdentityEndpoint,
					StorageEndpoint:  c.StorageEndpoint,
					Regions:          c.Regions,
				},
				Access: access,
			},
		})
		return nil
	})
	if err != nil {
		return jujuparams.ListCloudInfoResults{}, errgo.Mask(err)
	}

	return jujuparams.ListCloudInfoResults{
		Results: results,
	}, nil
}
