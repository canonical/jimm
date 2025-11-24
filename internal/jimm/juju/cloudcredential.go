// Copyright 2025 Canonical.

package juju

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"sync"

	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v5"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/jimm/cloudcred"
	"github.com/canonical/jimm/v3/internal/openfga"
)

// GetCloudCredential retrieves the given credential from the database. The
// returned credential will never contain any attributes, see
// GetCloudCredentialAttributes to retrieve those. If credentials
// identified by the given tag cannot be found then an errror with a code
// of CodeNotFound will be returned. If the given user is not a controller
// superuser or the owner of the credentials then an error with a code of
// CodeUnauthorized will be returned.
func (j *JujuManager) GetCloudCredential(ctx context.Context, user *openfga.User, tag names.CloudCredentialTag) (*dbmodel.CloudCredential, error) {

	if !user.JimmAdmin && user.Name != tag.Owner().Id() {
		return nil, errors.E(errors.CodeUnauthorized, "unauthorized")
	}

	var credential dbmodel.CloudCredential
	credential.SetTag(tag)

	err := j.Database.GetCloudCredential(ctx, &credential)
	if err != nil {
		return nil, errors.E(err)
	}

	return &credential, nil
}

// RevokeCloudCredential checks that the credential with the given path
// can be revoked  and revokes the credential.
func (j *JujuManager) RevokeCloudCredential(ctx context.Context, user *dbmodel.Identity, tag names.CloudCredentialTag) error {

	if user.Name != tag.Owner().Id() {
		return errors.E(errors.CodeUnauthorized, "unauthorized")
	}

	var credential dbmodel.CloudCredential
	credential.SetTag(tag)

	err := j.Database.GetCloudCredential(ctx, &credential)
	if err != nil {
		if errors.ErrorCode(err) == errors.CodeNotFound {
			// It is not an error to revoke an non-existent credential
			return nil
		}
		return errors.E(err)
	}

	credential.Valid = sql.NullBool{
		Bool:  false,
		Valid: true,
	}

	models, err := j.Database.GetModelsUsingCredential(ctx, credential.ID)
	if err != nil {
		return errors.E(err)
	}
	// Before we accepted the force flag to remove the credential regardless of the references count.
	// Now we want to ensure that the credential is not used by any models before removing it to maintain
	// referential integrity.
	if len(models) > 0 {
		return errors.E(errors.CodeBadRequest, fmt.Sprintf("cloud credential still used by %d model(s)", len(models)))
	}

	cloud := dbmodel.Cloud{
		Name: credential.CloudName,
	}
	if err = j.Database.GetCloud(ctx, &cloud); err != nil {
		return errors.E(err)
	}

	var controllers []dbmodel.Controller
	seen := make(map[uint]bool)
	for _, region := range cloud.Regions {
		for _, cr := range region.Controllers {
			if seen[cr.ControllerID] {
				continue
			}
			seen[cr.ControllerID] = true
			controllers = append(controllers, cr.Controller)
		}
	}

	err = j.forEachController(ctx, controllers, func(ctl *dbmodel.Controller, api API) error {
		err := api.RevokeCredential(ctx, tag)
		if errors.ErrorCode(err) == errors.CodeNotFound {
			err = nil
		}
		return err
	})

	if err != nil {
		return errors.E(err)
	}

	err = j.Database.DeleteCloudCredential(ctx, &credential)
	if err != nil {
		return errors.E(err, "failed to revoke credential in local database")
	}
	return nil
}

// UpdateCloudCredentialArgs holds arguments for the cloud credential update
type UpdateCloudCredentialArgs struct {
	CredentialTag names.CloudCredentialTag
	Credential    jujuparams.CloudCredential
	SkipCheck     bool
	SkipUpdate    bool
}

// UpdateCloudCredential checks that the credential can be updated
// and updates it in the local database and all controllers
// to which it is deployed.
func (j *JujuManager) UpdateCloudCredential(ctx context.Context, user *openfga.User, args UpdateCloudCredentialArgs) ([]jujuparams.UpdateCredentialModelResult, error) {

	var resultMu sync.Mutex
	var result []jujuparams.UpdateCredentialModelResult
	if user.Tag() != args.CredentialTag.Owner() {
		if !user.JimmAdmin {
			return result, errors.E(errors.CodeUnauthorized, "unauthorized")
		}
		// ensure the user we are adding the credential for exists.
		var u2 dbmodel.Identity
		u2.SetTag(args.CredentialTag.Owner())
		if err := j.Database.GetIdentity(ctx, &u2); err != nil {
			return result, errors.E(err)
		}
	}

	var credential dbmodel.CloudCredential
	credential.SetTag(args.CredentialTag)

	err := j.Database.GetCloudCredential(ctx, &credential)
	if err != nil && errors.ErrorCode(err) != errors.CodeNotFound {
		return result, errors.E(err)
	}

	// Confirm the cloud exists.
	var cloud dbmodel.Cloud
	cloud.SetTag(names.NewCloudTag(credential.CloudName))
	if err = j.Database.GetCloud(ctx, &cloud); err != nil {
		return result, errors.E(err)
	}

	models, err := j.Database.GetModelsUsingCredential(ctx, credential.ID)
	if err != nil {
		return result, errors.E(err)
	}
	var controllers []dbmodel.Controller
	seen := make(map[uint]bool)
	for _, model := range models {
		if seen[model.ControllerID] {
			continue
		}
		seen[model.ControllerID] = true
		controllers = append(controllers, model.Controller)
	}

	credential.AuthType = args.Credential.AuthType

	if !args.SkipCheck {
		err := j.forEachController(ctx, controllers, func(ctl *dbmodel.Controller, api API) error {
			models, err := j.updateControllerCloudCredential(ctx, &credential, api.CheckCredentialModels)
			resultMu.Lock()
			defer resultMu.Unlock()
			result = append(result, models...)
			return err
		})
		if err != nil {
			return result, errors.E(err)
		}
	}
	var modelsErr bool
	for _, r := range result {
		if len(r.Errors) > 0 {
			modelsErr = true
		}
	}
	if modelsErr {
		return result, nil
	}
	if args.SkipUpdate {
		return result, nil
	}

	if err := j.updateCredential(ctx, &credential, args.Credential.Attributes); err != nil {
		return result, errors.E(err)
	}

	err = j.forEachController(ctx, controllers, func(ctl *dbmodel.Controller, api API) error {
		models, err := j.updateControllerCloudCredential(ctx, &credential, api.UpdateCredential)
		if err != nil {
			return err
		}
		if args.SkipCheck {
			resultMu.Lock()
			defer resultMu.Unlock()
			result = append(result, models...)
		}
		return nil
	})
	if err != nil {
		return result, errors.E(err)
	}
	return result, nil
}

// updateCredential updates the credential stored in JIMM's database.
func (j *JujuManager) updateCredential(ctx context.Context, credential *dbmodel.CloudCredential, attr map[string]string) error {

	if err := j.Database.SetCloudCredential(ctx, credential); err != nil {
		return errors.E(fmt.Errorf("failed to store credential id: %w", err))
	}
	if err := j.CredentialStore.Put(ctx, credential.ResourceTag(), attr); err != nil {
		return errors.E(fmt.Errorf("failed to store credentials: %w", err))
	}

	return nil
}

func (j *JujuManager) updateControllerCloudCredential(
	ctx context.Context,
	cred *dbmodel.CloudCredential,
	f func(context.Context, jujuparams.TaggedCredential) ([]jujuparams.UpdateCredentialModelResult, error),
) ([]jujuparams.UpdateCredentialModelResult, error) {

	attr, err := j.getCloudCredentialAttributes(ctx, cred)
	if err != nil {
		return nil, errors.E(err)
	}

	models, err := f(ctx, jujuparams.TaggedCredential{
		Tag: cred.Tag().String(),
		Credential: jujuparams.CloudCredential{
			AuthType:   cred.AuthType,
			Attributes: attr,
		},
	})
	if err != nil {
		return models, errors.E(err)
	}
	return models, nil
}

// ForEachUserCloudCredential iterates through every credential owned by
// the given user and for the given cloud (if specified). The given
// function is called for each credential found. The credential used when
// calling the function will not contain any attributes,
// GetCloudCredentialAttributes should be used to retrive the credential
// attributes if needed. The given function should not update the database.
func (j *JujuManager) ForEachUserCloudCredential(ctx context.Context, u *dbmodel.Identity, ct names.CloudTag, f func(cred *dbmodel.CloudCredential) error) error {

	var cloud string
	if ct != (names.CloudTag{}) {
		cloud = ct.Id()
	}

	errStop := errors.E("stop")
	var iterErr error
	err := j.Database.ForEachCloudCredential(ctx, u.Name, cloud, func(cred *dbmodel.CloudCredential) error {
		iterErr = f(cred)
		if iterErr != nil {
			return errStop
		}
		return nil
	})
	if err == errStop {
		err = iterErr
	} else if err != nil {
		err = errors.E(err)
	}
	return err
}

// GetCloudCredentialAttributes retrieves the attributes for a cloud
// credential. If hidden is true then returned credentials will include
// hidden attributes, otherwise a list of redacted attributes will be
// returned. Only the credential owner can retrieve hidden attributes any
// other user, including controller superusers, will receive an error with
// the code CodeUnauthorized.
func (j *JujuManager) GetCloudCredentialAttributes(ctx context.Context, user *openfga.User, cred *dbmodel.CloudCredential, hidden bool) (attrs map[string]string, redacted []string, err error) {

	if hidden {
		// Controller superusers cannot read hidden credential attributes.
		if user.Name != cred.OwnerIdentityName {
			return nil, nil, errors.E(errors.CodeUnauthorized, "unauthorized")
		}
	} else {
		if !user.JimmAdmin && user.Name != cred.OwnerIdentityName {
			return nil, nil, errors.E(errors.CodeUnauthorized, "unauthorized")
		}
	}

	attrs, err = j.getCloudCredentialAttributes(ctx, cred)
	if err != nil {
		err = errors.E(err)
		return
	}
	if len(attrs) == 0 {
		return map[string]string{}, nil, nil
	}

	if hidden {
		return
	}

	for k := range attrs {
		if !cloudcred.IsVisibleAttribute(cred.Cloud.Type, cred.AuthType, k) {
			delete(attrs, k)
			redacted = append(redacted, k)
		}
	}
	sort.Strings(redacted)

	return
}

// getCloudCredentialAttributes retrieves the attributes for a cloud credential.
func (j *JujuManager) getCloudCredentialAttributes(ctx context.Context, cred *dbmodel.CloudCredential) (map[string]string, error) {

	attr, err := j.CredentialStore.Get(ctx, cred.ResourceTag())
	if err != nil {
		return nil, errors.E(err)
	}
	return attr, nil
}

// CopyCredential copies a cloud credential from one user to another.
func (j *JujuManager) CopyCredential(ctx context.Context, originalUser *openfga.User, newUser *openfga.User, cred names.CloudCredentialTag) (names.CloudCredentialTag, []jujuparams.UpdateCredentialModelResult, error) {
	credential, err := j.GetCloudCredential(ctx, originalUser, cred)
	if err != nil {
		return names.CloudCredentialTag{}, nil, errors.E(err)
	}

	attr, err := j.getCloudCredentialAttributes(ctx, credential)
	if err != nil {
		return names.CloudCredentialTag{}, nil, errors.E(err)
	}

	newCredID := fmt.Sprintf("%s/%s/%s", cred.Cloud().Id(), newUser.Name, cred.Name())
	if !names.IsValidCloudCredential(newCredID) {
		return names.CloudCredentialTag{}, nil, errors.E(fmt.Sprintf("new credential ID %s is not a valid cloud credential tag", newCredID))
	}

	newCredential := jujuparams.CloudCredential{
		AuthType:   credential.AuthType,
		Attributes: attr,
	}
	newTag := names.NewCloudCredentialTag(newCredID)

	modelRes, err := j.UpdateCloudCredential(ctx, newUser, UpdateCloudCredentialArgs{
		CredentialTag: newTag,
		Credential:    newCredential,
		SkipCheck:     false,
		SkipUpdate:    false,
	})

	return newTag, modelRes, err
}
