// Copyright 2016 Canonical Ltd.

package jujuapi

import (
	"context"
	"fmt"

	jujuerrors "github.com/juju/errors"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v4"

	"github.com/CanonicalLtd/jimm/internal/dbmodel"
	"github.com/CanonicalLtd/jimm/internal/errors"
	"github.com/CanonicalLtd/jimm/internal/jimm"
	"github.com/CanonicalLtd/jimm/internal/jujuapi/rpc"
	"github.com/CanonicalLtd/jimm/internal/openfga"
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
	return jujuparams.StringResult{}, errors.E(errors.Op("jujuapi.DefaultCloud"), errors.CodeNotFound, "no default cloud")
}

// Cloud implements the Cloud method of the Cloud facade.
func (r *controllerRoot) Cloud(ctx context.Context, ents jujuparams.Entities) (jujuparams.CloudResults, error) {
	const op = errors.Op("jujuapi.Cloud")

	cloudResults := make([]jujuparams.CloudResult, len(ents.Entities))
	for i, ent := range ents.Entities {
		tag, err := names.ParseCloudTag(ent.Tag)
		if err != nil {
			cloudResults[i].Error = mapError(errors.E(op, errors.CodeBadRequest, err))
			continue
		}
		cloud, err := r.jimm.GetCloud(ctx, r.user, tag)
		if err != nil {
			cloudResults[i].Error = mapError(errors.E(op, err))
			continue
		}
		cloudResults[i].Cloud = new(jujuparams.Cloud)
		*cloudResults[i].Cloud = cloud.ToJujuCloud()
	}
	return jujuparams.CloudResults{
		Results: cloudResults,
	}, nil
}

// Clouds implements the Clouds method on the Cloud facade.
func (r *controllerRoot) Clouds(ctx context.Context) (jujuparams.CloudsResult, error) {
	const op = errors.Op("jujuapi.Clouds")

	res := jujuparams.CloudsResult{
		Clouds: make(map[string]jujuparams.Cloud),
	}
	err := r.jimm.ForEachUserCloud(ctx, r.user, func(cld *dbmodel.Cloud) error {
		res.Clouds[cld.Tag().String()] = cld.ToJujuCloud()
		return nil
	})
	if err != nil {
		return res, errors.E(op, err)
	}
	return res, nil
}

// UserCredentials implements the UserCredentials method of the Cloud facade.
func (r *controllerRoot) UserCredentials(ctx context.Context, userclouds jujuparams.UserClouds) (jujuparams.StringsResults, error) {
	const op = errors.Op("jujuapi.UserCredentials")

	results := make([]jujuparams.StringsResult, len(userclouds.UserClouds))
	for i, ent := range userclouds.UserClouds {
		user, err := r.masquerade(ctx, ent.UserTag)
		if err != nil {
			results[i].Error = mapError(errors.E(op, err))
			continue
		}
		cld, err := names.ParseCloudTag(ent.CloudTag)
		if err != nil {
			results[i].Error = mapError(errors.E(op, err, errors.CodeBadRequest))
			continue
		}
		err = r.jimm.ForEachUserCloudCredential(ctx, user.User, cld, func(c *dbmodel.CloudCredential) error {
			results[i].Result = append(results[i].Result, c.Tag().String())
			return nil
		})
		if err != nil {
			results[i].Error = mapError(errors.E(op, err))
		}
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
	const op = errors.Op("jujuapi.RevokeCredentialsCheckModels")

	ct, err := names.ParseCloudCredentialTag(tag)
	if err != nil {
		return errors.E(op, err, errors.CodeBadRequest)
	}
	if err := r.jimm.RevokeCloudCredential(ctx, r.user.User, ct, force); err != nil {
		return errors.E(op, err)
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
	const op = errors.Op("jujuapi.Credential")

	cct, err := names.ParseCloudCredentialTag(cloudCredentialTag)
	if err != nil {
		return nil, errors.E(op, err, errors.CodeBadRequest)
	}

	cred, err := r.jimm.GetCloudCredential(ctx, r.user.User, cct)
	if err != nil {
		return nil, errors.E(op, err)
	}
	cc := jujuparams.CloudCredential{
		AuthType: cred.AuthType,
	}
	cc.Attributes, cc.Redacted, err = r.jimm.GetCloudCredentialAttributes(ctx, r.user.User, cred, false)
	if err != nil {
		return nil, errors.E(op, err)
	}
	return &cc, nil
}

// AddCloud implements the AddCloud method of the Cloud (v2) facade.
func (r *controllerRoot) AddCloud(ctx context.Context, args jujuparams.AddCloudArgs) error {
	const op = errors.Op("jujuapi.AddCloud")
	if err := r.jimm.AddHostedCloud(ctx, r.user, names.NewCloudTag(args.Name), args.Cloud); err != nil {
		return errors.E(op, err)
	}
	return nil
}

// AddCredentials implements the AddCredentials method of the Cloud (v2) facade.
func (r *controllerRoot) AddCredentials(ctx context.Context, args jujuparams.TaggedCredentials) (jujuparams.ErrorResults, error) {
	const op = errors.Op("jujuapi.AddCredentials")

	updateResults, err := r.UpdateCredentialsCheckModels(ctx, jujuparams.UpdateCredentialArgs{
		Credentials: args.Credentials,
	})
	if err != nil {
		return jujuparams.ErrorResults{}, errors.E(op, err)
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
				modelErrors := jujuparams.ErrorResults{m.Errors}
				combined := jujuerrors.Annotatef(modelErrors.Combine(), "model %q (uuid %v)", m.ModelName, m.ModelUUID)
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

func userModelAccess(ctx context.Context, user *openfga.User, model names.ModelTag) (string, error) {
	isAdmin, err := openfga.IsAdministrator(ctx, user, model)
	if err != nil {
		return "", errors.E(err)
	}
	if isAdmin {
		return "admin", nil
	}
	hasWriteAccess, err := user.IsModelWriter(ctx, model)
	if err != nil {
		return "", errors.E(err)
	}
	if hasWriteAccess {
		return "write", nil
	}
	hasReadAccess, err := user.IsModelReader(ctx, model)
	if err != nil {
		return "", errors.E(err)
	}
	if hasReadAccess {
		return "read", nil
	}

	return "", nil
}

// CredentialContents implements the CredentialContents method of the Cloud (v5) facade.
func (r *controllerRoot) CredentialContents(ctx context.Context, args jujuparams.CloudCredentialArgs) (jujuparams.CredentialContentResults, error) {
	const op = errors.Op("jujuapi.CredentialContents")

	credentialContents := func(c *dbmodel.CloudCredential) (*jujuparams.ControllerCredentialInfo, error) {
		content := jujuparams.CredentialContent{
			Name:     c.Name,
			Cloud:    c.CloudName,
			AuthType: c.AuthType,
		}
		if c.Valid.Valid {
			content.Valid = &c.Valid.Bool
		}
		var err error
		content.Attributes, _, err = r.jimm.GetCloudCredentialAttributes(ctx, r.user.User, c, args.IncludeSecrets)
		if err != nil {
			return nil, errors.E(err)
		}
		mas := make([]jujuparams.ModelAccess, len(c.Models))
		for i, m := range c.Models {
			userModelAccess, err := userModelAccess(ctx, r.user, m.ResourceTag())
			if err != nil {
				return nil, errors.E(err)
			}
			mas[i] = jujuparams.ModelAccess{
				Model:  m.Name,
				Access: userModelAccess,
			}
		}
		return &jujuparams.ControllerCredentialInfo{
			Content: content,
			Models:  mas,
		}, nil
	}

	results := make([]jujuparams.CredentialContentResult, len(args.Credentials))
	for i, arg := range args.Credentials {
		cct := names.NewCloudCredentialTag(fmt.Sprintf("%s/%s/%s", arg.CloudName, r.user.Username, arg.CredentialName))
		cred, err := r.jimm.GetCloudCredential(ctx, r.user.User, cct)
		if err != nil {
			results[i].Error = mapError(errors.E(op, err))
			continue
		}
		results[i].Result, err = credentialContents(cred)
		if err != nil {
			results[i].Error = mapError(errors.E(op, err))
		}
	}
	if len(results) > 0 {
		return jujuparams.CredentialContentResults{Results: results}, nil
	}

	err := r.jimm.ForEachUserCloudCredential(ctx, r.user.User, names.CloudTag{}, func(c *dbmodel.CloudCredential) error {
		var result jujuparams.CredentialContentResult
		var err error
		result.Result, err = credentialContents(c)
		if err != nil {
			result.Error = mapError(errors.E(op, err))
		}
		results = append(results, result)
		return nil
	})
	if err != nil {
		return jujuparams.CredentialContentResults{}, errors.E(op, err)
	}
	return jujuparams.CredentialContentResults{Results: results}, nil
}

// RemoveClouds removes the specified clouds from the controller.
// If a cloud is in use (has models deployed to it), the removal will fail.
func (r *controllerRoot) RemoveClouds(ctx context.Context, args jujuparams.Entities) (jujuparams.ErrorResults, error) {
	const op = errors.Op("jujuapi.RemoveClouds")

	result := jujuparams.ErrorResults{
		Results: make([]jujuparams.ErrorResult, len(args.Entities)),
	}
	for i, entity := range args.Entities {
		tag, err := names.ParseCloudTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = mapError(errors.E(op, err))
			continue
		}
		err = r.jimm.RemoveCloud(ctx, r.user, tag)
		if err != nil {
			result.Results[i].Error = mapError(errors.E(op, err))
		}
	}
	return result, nil
}

func (r *controllerRoot) CheckCredentialsModels(ctx context.Context, args jujuparams.TaggedCredentials) (jujuparams.UpdateCredentialResults, error) {
	return r.updateCredentials(ctx, args.Credentials, false, true)
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
	const op = errors.Op("jujuapi.ModiftCloudAccess")
	// TODO (alesstimec) granting and revoking access tbd in a followup
	return errors.E(errors.CodeNotImplemented)

	/*
		user, err := parseUserTag(change.UserTag)
		if err != nil {
			return errors.E(op, err)
		}
		cloudTag, err := names.ParseCloudTag(change.CloudTag)
		if err != nil {
			return errors.E(op, errors.CodeBadRequest, err)
		}
		var modifyf func(context.Context, *dbmodel.User, names.CloudTag, names.UserTag, string) error
		switch change.Action {
		case jujuparams.GrantCloudAccess:
			modifyf = r.jimm.GrantCloudAccess
		case jujuparams.RevokeCloudAccess:
			modifyf = r.jimm.RevokeCloudAccess
		default:
			return errors.E(op, errors.CodeBadRequest, fmt.Sprintf("unsupported modify cloud action %q", change.Action))
		}
		if err := modifyf(ctx, r.user, cloudTag, user, change.Access); err != nil {
			return errors.E(op, err)
		}
		return nil
	*/
}

// UpdateCredentialsCheckModels updates a set of cloud credentials' content.
// If there are any models that are using a credential and these models
// are not going to be visible with updated credential content,
// there will be detailed validation errors per model.
func (r *controllerRoot) UpdateCredentialsCheckModels(ctx context.Context, args jujuparams.UpdateCredentialArgs) (jujuparams.UpdateCredentialResults, error) {
	return r.updateCredentials(ctx, args.Credentials, args.Force, false)
}

func (r *controllerRoot) updateCredentials(ctx context.Context, args []jujuparams.TaggedCredential, skipCheck, skipUpdate bool) (jujuparams.UpdateCredentialResults, error) {
	results := jujuparams.UpdateCredentialResults{
		Results: make([]jujuparams.UpdateCredentialResult, len(args)),
	}
	for i, arg := range args {
		var err error
		models, err := r.updateCredential(ctx, arg, skipCheck, skipUpdate)
		results.Results[i] = jujuparams.UpdateCredentialResult{
			CredentialTag: arg.Tag,
			Error:         mapError(err),
			Models:        models,
		}
		results.Results[i].CredentialTag = arg.Tag
	}
	return results, nil
}

func (r *controllerRoot) updateCredential(ctx context.Context, cred jujuparams.TaggedCredential, skipCheck, skipUpdate bool) ([]jujuparams.UpdateCredentialModelResult, error) {
	tag, err := names.ParseCloudCredentialTag(cred.Tag)
	if err != nil {
		return nil, errors.E(err, errors.CodeBadRequest)
	}
	return r.jimm.UpdateCloudCredential(ctx, r.user.User, jimm.UpdateCloudCredentialArgs{
		CredentialTag: tag,
		Credential:    cred.Credential,
		SkipCheck:     skipCheck,
		SkipUpdate:    skipUpdate,
	})
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
	return errors.E(errors.CodeForbidden, "permission denied")
}

// CloudInfo implements the cloud facades CloudInfo method.
func (r *controllerRoot) CloudInfo(ctx context.Context, args jujuparams.Entities) (jujuparams.CloudInfoResults, error) {
	const op = errors.Op("jujuapi.CloudInfo")

	results := make([]jujuparams.CloudInfoResult, len(args.Entities))
	for i, ent := range args.Entities {
		tag, err := names.ParseCloudTag(ent.Tag)
		if err != nil {
			results[i].Error = mapError(errors.E(op, err, errors.CodeBadRequest))
			continue
		}
		cloud, err := r.jimm.GetCloud(ctx, r.user, tag)
		if err != nil {
			results[i].Error = mapError(errors.E(op, err))
			continue
		}

		results[i].Result = new(jujuparams.CloudInfo)
		*results[i].Result = cloud.ToJujuCloudInfo()
	}
	return jujuparams.CloudInfoResults{
		Results: results,
	}, nil
}

// ListCloudInfo implements the ListCloudInfo method on the cloud facade.
func (r *controllerRoot) ListCloudInfo(ctx context.Context, args jujuparams.ListCloudsRequest) (jujuparams.ListCloudInfoResults, error) {
	const op = errors.Op("jujuapi.ListCloudInfo")

	listF := r.jimm.ForEachUserCloud
	if args.All {
		listF = r.jimm.ForEachCloud
	}

	var results []jujuparams.ListCloudInfoResult
	err := listF(ctx, r.user, func(c *dbmodel.Cloud) error {
		results = append(results, jujuparams.ListCloudInfoResult{
			Result: &jujuparams.ListCloudInfo{
				CloudDetails: c.ToJujuCloudDetails(),
				Access:       jimm.ToCloudAccessString(r.user.GetCloudAccess(ctx, c.ResourceTag())),
			},
		})
		return nil
	})
	if err != nil {
		return jujuparams.ListCloudInfoResults{}, errors.E(op, err)
	}

	return jujuparams.ListCloudInfoResults{
		Results: results,
	}, nil
}
