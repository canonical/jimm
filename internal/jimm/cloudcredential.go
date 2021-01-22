// Copyright 2020 Canonical Ltd.

package jimm

import (
	"context"
	"database/sql"
	"fmt"
	"path"

	jujuparams "github.com/juju/juju/apiserver/params"
	"github.com/juju/names/v4"
	"go.uber.org/zap"

	"github.com/CanonicalLtd/jimm/internal/dbmodel"
	"github.com/CanonicalLtd/jimm/internal/errors"
	"github.com/CanonicalLtd/jimm/internal/zapctx"
	"github.com/CanonicalLtd/jimm/internal/zaputil"
)

// GetCloudCredential retrieves the given credential from the database.
func (j *JIMM) GetCloudCredential(ctx context.Context, user *dbmodel.User, tag names.CloudCredentialTag) (*dbmodel.CloudCredential, error) {
	const op = errors.Op("jimm.GetCloudCredential")

	if user.Username != tag.Owner().Id() {
		return nil, errors.E(op, errors.CodeUnauthorized)
	}

	var credential dbmodel.CloudCredential
	credential.SetTag(tag)

	err := j.Database.GetCloudCredential(ctx, &credential)
	if err != nil {
		return nil, errors.E(op, err)
	}
	return &credential, nil
}

// RevokeCloudCredential checks that the credential with the given path
// can be revoked  and revokes the credential.
func (j *JIMM) RevokeCloudCredential(ctx context.Context, user *dbmodel.User, tag names.CloudCredentialTag) error {
	const op = errors.Op("jimm.RevokeCloudCredential")

	if user.Username != tag.Owner().Id() {
		return errors.E(op, errors.CodeUnauthorized)
	}

	var credential dbmodel.CloudCredential
	credential.SetTag(tag)

	err := j.Database.GetCloudCredential(ctx, &credential)
	if err != nil {
		return errors.E(op, err)
	}

	credential.Valid = sql.NullBool{
		Bool:  false,
		Valid: true,
	}

	models, err := j.Database.GetModelsUsingCredential(ctx, credential.ID)
	if err != nil {
		return errors.E(op, err)
	}
	// if the cloud credential is still used by any model we return an error
	if len(models) > 0 {
		return errors.E(op, errors.CodeBadRequest, fmt.Sprintf("cloud credential still used by %d model(s)", len(models)))
	}

	cloud := dbmodel.Cloud{
		Name: credential.CloudName,
	}
	if err = j.Database.GetCloud(ctx, &cloud); err != nil {
		return errors.E(op, err)
	}

	controllers := make(map[uint]dbmodel.Controller)
	for _, region := range cloud.Regions {
		for _, cr := range region.Controllers {
			controllers[cr.ControllerID] = cr.Controller
		}
	}

	ch := make(chan error, len(controllers))
	for _, controller := range controllers {
		controller := controller
		go func() {
			err := j.revokeControllerCredential(ctx, &controller, tag)
			select {
			case ch <- err:
			case <-ctx.Done():
				zapctx.Error(ctx, "timed out: failed to forward credential check results")
			}
		}()
	}

	for i := 0; i < len(controllers); i++ {
		select {
		case err := <-ch:
			if err != nil {
				return errors.E(err, "failed to revoke credential")
			}
		case <-ctx.Done():
			return errors.E("timed out: revoking credentials")
		}
	}

	err = j.Database.SetCloudCredential(ctx, &credential)
	if err != nil {
		return errors.E(op, err, "failed to revoke credential in local database")
	}
	return nil
}

func (j *JIMM) revokeControllerCredential(ctx context.Context, controller *dbmodel.Controller, credentialTag names.CloudCredentialTag) error {
	api, err := j.dial(ctx, controller, names.ModelTag{})
	if err != nil {
		return errors.E(err)
	}
	defer api.Close()

	err = api.RevokeCredential(ctx, credentialTag)
	if err != nil {
		if errors.ErrorCode(err) == errors.CodeNotFound {
			return nil
		}
		return errors.E(err, "cannot revoke credential")
	}
	return nil
}

// UpdateCloudCredentialArgs holds arguments for the cloud credential update
type UpdateCloudCredentialArgs struct {
	User          *dbmodel.User
	CredentialTag names.CloudCredentialTag
	Credential    jujuparams.CloudCredential
	SkipCheck     bool
	SkipUpdate    bool
}

// UpdateCloudCredential checks that the credential can be updated
// and updates it in the local database and all controllers
// to which it is deployed.
func (j *JIMM) UpdateCloudCredential(ctx context.Context, args UpdateCloudCredentialArgs) ([]jujuparams.UpdateCredentialModelResult, error) {
	const op = errors.Op("jimm.UpdateCloudCredential")

	if args.User.Username != args.CredentialTag.Owner().Id() {
		return nil, errors.E(op, errors.CodeUnauthorized)
	}

	var credential dbmodel.CloudCredential
	credential.SetTag(args.CredentialTag)

	err := j.Database.GetCloudCredential(ctx, &credential)
	if err != nil {
		return nil, errors.E(op, err)
	}

	models, err := j.Database.GetModelsUsingCredential(ctx, credential.ID)
	if err != nil {
		return nil, errors.E(op, err)
	}
	controllers := make(map[uint]dbmodel.Controller)
	for _, model := range models {
		controllers[model.ControllerID] = model.Controller
	}

	if !args.SkipCheck {
		modelResults, err := j.checkCredential(ctx, args, controllers)
		if err != nil {
			return modelResults, errors.E(op, err)
		}
	}

	if !args.SkipUpdate {
		credential.Attributes = args.Credential.Attributes
		credential.AuthType = args.Credential.AuthType
		credential.Label = args.CredentialTag.String()

		modelResults, err := j.updateCredential(ctx, args, &credential, controllers)
		if err != nil {
			return modelResults, errors.E(op, err)
		}
		return modelResults, nil
	}
	return nil, nil
}

func (j *JIMM) updateCredential(ctx context.Context, arg UpdateCloudCredentialArgs, credential *dbmodel.CloudCredential, controllers map[uint]dbmodel.Controller) ([]jujuparams.UpdateCredentialModelResult, error) {
	if j.VaultClient != nil {
		credential1 := *credential
		credential1.Attributes = nil
		credential1.AttributesInVault = true
		if err := j.Database.SetCloudCredential(ctx, &credential1); err != nil {
			return nil, errors.E(err, "cannot update local database")
		}

		data := make(map[string]interface{}, len(credential.Attributes))
		for k, v := range credential.Attributes {
			data[k] = v
		}
		if len(data) > 0 {
			// Don't attempt to write no data to the vault.
			logical := j.VaultClient.Logical()
			_, err := logical.Write(path.Join(j.VaultPath, "creds", credential.Cloud.Name, credential.Owner.Username, credential.Name), data)
			if err != nil {
				return nil, errors.E(err)
			}
		}
	} else {
		credential.AttributesInVault = false
		if err := j.Database.SetCloudCredential(ctx, credential); err != nil {
			return nil, errors.E(err, "cannot update local database")
		}
	}
	ch := make(chan credentialResult, len(controllers))
	for _, controller := range controllers {
		controller := controller
		go func() {
			models, err := j.updateControllerCredential(ctx, &controller, arg)
			select {
			case ch <- credentialResult{
				controller: controller,
				models:     models,
				err:        err,
			}:
			case <-ctx.Done():
				zapctx.Error(ctx, "timed out: failed to forward credential check results")
			}
		}()
	}
	models, err := mergeCredentialResults(ctx, ch, len(controllers))
	if err != nil {
		return nil, errors.E(err)
	}
	return models, nil
}

func (j *JIMM) updateControllerCredential(
	ctx context.Context,
	controller *dbmodel.Controller,
	arg UpdateCloudCredentialArgs,
) ([]jujuparams.UpdateCredentialModelResult, error) {
	api, err := j.dial(ctx, controller, names.ModelTag{})
	if err != nil {
		return nil, errors.E(err)
	}
	defer api.Close()

	models, err := api.UpdateCredential(ctx, jujuparams.TaggedCredential{
		Tag:        arg.CredentialTag.String(),
		Credential: arg.Credential,
	})
	if err != nil {
		return models, errors.E(err, "cannot update credentials")
	}
	return models, err
}

func (j *JIMM) checkCredential(ctx context.Context, arg UpdateCloudCredentialArgs, controllers map[uint]dbmodel.Controller) ([]jujuparams.UpdateCredentialModelResult, error) {
	if len(controllers) == 0 {
		return nil, nil
	}
	ch := make(chan credentialResult, len(controllers))
	for _, controller := range controllers {
		controller := controller
		go func() {
			models, err := j.checkCredentialOnController(ctx, &controller, jujuparams.TaggedCredential{
				Tag:        arg.CredentialTag.String(),
				Credential: arg.Credential,
			})
			select {
			case ch <- credentialResult{
				controller: controller,
				models:     models,
				err:        err,
			}:
			case <-ctx.Done():
				zapctx.Error(ctx, "timed out: failed to forward credential check results")
			}
		}()
	}

	models, err := mergeCredentialResults(ctx, ch, len(controllers))
	if err != nil {
		return nil, errors.E(err)
	}
	return models, nil
}

func (j *JIMM) checkCredentialOnController(ctx context.Context, controller *dbmodel.Controller, credential jujuparams.TaggedCredential) ([]jujuparams.UpdateCredentialModelResult, error) {
	api, err := j.dial(ctx, controller, names.ModelTag{})
	if err != nil {
		return nil, errors.E(err)
	}
	defer api.Close()

	if !api.SupportsCheckCredentialModels() {
		return nil, errors.E(fmt.Sprintf("CheckCredentialModels method not supported on controller %v", controller.Name))
	}

	models, err := api.CheckCredentialModels(ctx, credential)
	if err != nil {
		return nil, errors.E(err, "credential check failed")
	}
	return models, nil
}

type credentialResult struct {
	controller dbmodel.Controller
	models     []jujuparams.UpdateCredentialModelResult
	err        error
}

func mergeCredentialResults(ctx context.Context, ch <-chan credentialResult, n int) ([]jujuparams.UpdateCredentialModelResult, error) {
	var models []jujuparams.UpdateCredentialModelResult
	var firstError error
	for n > 0 {
		select {
		case r := <-ch:
			n--
			models = append(models, r.models...)
			if r.err != nil {
				zapctx.Warn(ctx,
					"cannot update credential",
					zap.String("controller", r.controller.Name),
					zaputil.Error(r.err),
				)
				if firstError == nil {
					firstError = errors.E(r.err, fmt.Sprintf("controller %s: %v", r.controller.Name, r.err))
				}
			}

		case <-ctx.Done():
			return nil, errors.E(ctx.Err(), "timed out: waiting for credential checks")
		}
	}
	if firstError != nil {
		return models, errors.E(firstError)
	}
	return models, nil
}
