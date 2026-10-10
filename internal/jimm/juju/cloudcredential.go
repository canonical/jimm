// Copyright 2025 Canonical.

package juju

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"sync"

	"github.com/juju/juju/api/common/cloudcred"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v5"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
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
		return nil, errors.Codef(errors.CodeUnauthorized, "unauthorized")
	}

	var credential dbmodel.CloudCredential
	credential.SetTag(tag)

	err := j.Database.GetCloudCredential(ctx, &credential)
	if err != nil {
		return nil, err
	}

	return &credential, nil
}

// RevokeCloudCredential checks that the credential with the given path
// can be revoked  and revokes the credential.
func (j *JujuManager) RevokeCloudCredential(ctx context.Context, user *dbmodel.Identity, tag names.CloudCredentialTag) error {

	if user.Name != tag.Owner().Id() {
		return errors.Codef(errors.CodeUnauthorized, "unauthorized")
	}

	var credential dbmodel.CloudCredential
	credential.SetTag(tag)

	err := j.Database.GetCloudCredential(ctx, &credential)
	if err != nil {
		if errors.ErrorCode(err) == errors.CodeNotFound {
			// It is not an error to revoke an non-existent credential
			return nil
		}
		return err
	}

	credential.Valid = sql.NullBool{
		Bool:  false,
		Valid: true,
	}

	models, err := j.Database.GetModelsUsingCredential(ctx, credential.ID)
	if err != nil {
		return err
	}
	// Before we accepted the force flag to remove the credential regardless of the references count.
	// Now we want to ensure that the credential is not used by any models before removing it to maintain
	// referential integrity.
	if len(models) > 0 {
		return errors.Codef(errors.CodeBadRequest, "cloud credential still used by %d model(s)", len(models))
	}

	cloud := dbmodel.Cloud{
		Name: credential.CloudName,
	}
	if err = j.Database.GetCloud(ctx, &cloud); err != nil {
		return err
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
		return err
	}

	err = j.Database.DeleteCloudCredential(ctx, &credential)
	if err != nil {
		return fmt.Errorf("failed to revoke credential in local database: %w", err)
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
			return result, errors.Codef(errors.CodeUnauthorized, "unauthorized")
		}
		// ensure the user we are adding the credential for exists.
		var u2 dbmodel.Identity
		u2.SetTag(args.CredentialTag.Owner())
		if err := j.Database.GetIdentity(ctx, &u2); err != nil {
			return result, err
		}
	}

	var credential dbmodel.CloudCredential
	credential.SetTag(args.CredentialTag)

	err := j.Database.GetCloudCredential(ctx, &credential)
	if err != nil && errors.ErrorCode(err) != errors.CodeNotFound {
		return result, err
	}

	// Confirm the cloud exists.
	var cloud dbmodel.Cloud
	cloud.SetTag(names.NewCloudTag(credential.CloudName))
	if err = j.Database.GetCloud(ctx, &cloud); err != nil {
		return result, err
	}

	models, err := j.Database.GetModelsUsingCredential(ctx, credential.ID)
	if err != nil {
		return result, err
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
			models, err := j.checkControllerCloudCredential(ctx, &credential, args.Credential.Attributes, api)
			resultMu.Lock()
			defer resultMu.Unlock()
			result = append(result, models...)
			return err
		})
		if err != nil {
			return result, err
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
		return result, err
	}

	err = j.forEachController(ctx, controllers, func(ctl *dbmodel.Controller, api API) error {
		models, err := j.forceUpdateControllerCloudCredential(ctx, &credential, args.Credential.Attributes, api)
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
		return result, err
	}
	return result, nil
}

// updateCredential updates the credential stored in JIMM's database.
func (j *JujuManager) updateCredential(ctx context.Context, credential *dbmodel.CloudCredential, attr map[string]string) error {

	if err := j.Database.SetCloudCredential(ctx, credential); err != nil {
		return fmt.Errorf("failed to store credential id: %w", err)
	}
	if err := j.CredentialStore.Put(ctx, credential.ResourceTag(), attr); err != nil {
		return fmt.Errorf("failed to store credentials: %w", err)
	}

	return nil
}

// checkControllerCloudCredential checks that a given credential is safe to
// update across all of the models it is currently used. This is useful for
// doing a cross-controller check credential update safety.
func (j *JujuManager) checkControllerCloudCredential(
	ctx context.Context,
	cred *dbmodel.CloudCredential,
	attrs map[string]string,
	api API,
) ([]jujuparams.UpdateCredentialModelResult, error) {
	out, err := api.CheckCredentialModels(ctx, jujuparams.TaggedCredential{
		Tag: cred.Tag().String(),
		Credential: jujuparams.CloudCredential{
			AuthType:   cred.AuthType,
			Attributes: attrs,
		},
	})
	if err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, nil
	}
	if out[0].Error != nil {
		return out[0].Models, out[0].Error
	}
	return out[0].Models, nil
}

// forceUpdateControllerCloudCredential updates a cloud credential for a
// given controller. It presumes the caller has run checkControllerCloudCredential
// prior.
func (j *JujuManager) forceUpdateControllerCloudCredential(
	ctx context.Context,
	cred *dbmodel.CloudCredential,
	attrs map[string]string,
	api API,
) ([]jujuparams.UpdateCredentialModelResult, error) {
	out, err := api.UpdateCloudsCredentialForce(ctx, jujuparams.TaggedCredential{
		Tag: cred.Tag().String(),
		Credential: jujuparams.CloudCredential{
			AuthType:   cred.AuthType,
			Attributes: attrs,
		},
	})
	if err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, nil
	}
	if out[0].Error != nil {
		return out[0].Models, out[0].Error
	}
	return out[0].Models, nil
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

	errStop := errors.New("stop")
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
			return nil, nil, errors.Codef(errors.CodeUnauthorized, "unauthorized")
		}
	} else {
		if !user.JimmAdmin && user.Name != cred.OwnerIdentityName {
			return nil, nil, errors.Codef(errors.CodeUnauthorized, "unauthorized")
		}
	}

	attrs, err = j.getCloudCredentialAttributes(ctx, cred)
	if err != nil {
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
		return nil, err
	}
	return attr, nil
}

// CopyCredential copies a cloud credential from one user to another.
func (j *JujuManager) CopyCredential(ctx context.Context, originalUser *openfga.User, newUser *openfga.User, cred names.CloudCredentialTag) (names.CloudCredentialTag, []jujuparams.UpdateCredentialModelResult, error) {
	credential, err := j.GetCloudCredential(ctx, originalUser, cred)
	if err != nil {
		return names.CloudCredentialTag{}, nil, err
	}

	attr, err := j.getCloudCredentialAttributes(ctx, credential)
	if err != nil {
		return names.CloudCredentialTag{}, nil, err
	}

	newCredID := fmt.Sprintf("%s/%s/%s", cred.Cloud().Id(), newUser.Name, cred.Name())
	if !names.IsValidCloudCredential(newCredID) {
		return names.CloudCredentialTag{}, nil, fmt.Errorf("new credential ID %s is not a valid cloud credential tag", newCredID)
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

// RecoverModelCredential recovers a lost cloud credential (e.g. after a Vault
// outage) by fetching the credential's secret contents from a controller that
// hosts a model using that credential, and storing them back into JIMM's
// credential store. This is a disaster-recovery operation and requires JIMM
// admin access.
//
// The credential metadata (name, cloud, owner, auth-type) must still exist in
// JIMM's database (Postgres), the controller hosting the model must still be
// reachable, and the controller must still hold the credential secrets.
//
// When dryRun is true, all read operations are performed (verifying the secrets
// can be fetched) but the secrets are NOT written back into JIMM's credential
// store.
func (j *JujuManager) RecoverModelCredential(ctx context.Context, user *openfga.User, tag names.CloudCredentialTag, dryRun bool) error {
	if err := j.checkJimmAdmin(user); err != nil {
		return err
	}

	var credential dbmodel.CloudCredential
	credential.SetTag(tag)
	if err := j.Database.GetCloudCredential(ctx, &credential); err != nil {
		return err
	}

	return j.recoverCredential(ctx, &credential, dryRun)
}

// RecoverModelCredentialResult holds the outcome of recovering a single cloud
// credential.
type RecoverModelCredentialResult struct {
	// Tag is the cloud credential that was processed.
	Tag names.CloudCredentialTag
	// Recovered is true if the secrets were fetched from a controller and
	// stored back into JIMM's credential store.
	Recovered bool
	// Err holds the reason recovery failed, if any.
	Err error
}

// RecoverAllModelCredentials attempts to recover every cloud credential known
// to JIMM. It requires JIMM admin access. Recovery of each credential is
// attempted independently; the returned slice contains one result per
// credential describing whether it was recovered and, if not, why.
//
// When dryRun is true, all read operations are performed (verifying the secrets
// can be fetched) but the secrets are NOT written back into JIMM's credential
// store.
func (j *JujuManager) RecoverAllModelCredentials(ctx context.Context, user *openfga.User, dryRun bool) ([]RecoverModelCredentialResult, error) {
	if err := j.checkJimmAdmin(user); err != nil {
		return nil, err
	}

	creds, err := j.Database.GetAllCloudCredentials(ctx)
	if err != nil {
		return nil, err
	}

	results := make([]RecoverModelCredentialResult, 0, len(creds))
	for i := range creds {
		cred := creds[i]
		res := RecoverModelCredentialResult{Tag: cred.ResourceTag()}
		if rErr := j.recoverCredential(ctx, &cred, dryRun); rErr != nil {
			res.Err = rErr
		} else {
			res.Recovered = true
		}
		results = append(results, res)
	}
	return results, nil
}

// recoverCredential fetches the given credential's secrets from a controller
// hosting a model that uses it and stores them back into JIMM's credential
// store. It performs no authorisation checks; callers must do so.
//
// When dryRun is true the read phase is performed but the secrets are NOT
// written to the credential store.
//
//nolint:gocognit // the recovery flow is inherently branchy but reads clearly top-to-bottom.
func (j *JujuManager) recoverCredential(ctx context.Context, credential *dbmodel.CloudCredential, dryRun bool) error {
	tag := credential.ResourceTag()

	// Find the models (and therefore controllers) that use this credential so
	// we know where to fetch the secrets from.
	models, err := j.Database.GetModelsUsingCredential(ctx, credential.ID)
	if err != nil {
		return err
	}
	if len(models) == 0 {
		return errors.Codef(errors.CodeNotFound, "no models use credential attributes %q, cannot recover secrets", tag.String())
	}

	// The credential secrets on the controller are owned by the credential's
	// owner, so we must dial (and therefore log in) as that user - otherwise
	// CredentialContents scopes the lookup to the wrong user and finds nothing.
	ownerIdentity, err := dbmodel.NewIdentity(credential.OwnerIdentityName)
	if err != nil {
		return err
	}
	ownerUser := openfga.NewUser(ownerIdentity, j.OpenFGAClient)

	// Try each controller until we successfully retrieve the secret contents.
	var attrs map[string]string
	var lastErr error
	seen := make(map[uint]bool)
	for i := range models {
		ctl := models[i].Controller
		if seen[ctl.ID] {
			continue
		}
		seen[ctl.ID] = true

		api, dialErr := j.dialControllerAsUser(ctx, ownerUser, &ctl)
		if dialErr != nil {
			lastErr = dialErr
			continue
		}

		results, credErr := api.CredentialContents(tag.Cloud().Id(), tag.Name(), true)
		api.Close()
		if credErr != nil {
			lastErr = credErr
			continue
		}
		for _, res := range results {
			if res.Error != nil {
				lastErr = res.Error
				continue
			}
			if res.Result == nil {
				continue
			}
			if res.Result.Content.Name != tag.Name() {
				continue
			}
			if len(res.Result.Content.Attributes) == 0 {
				continue
			}
			attrs = res.Result.Content.Attributes
			break
		}
		if len(attrs) > 0 {
			break
		}
	}

	if len(attrs) == 0 {
		if lastErr != nil {
			return errors.Codef(errors.CodeNotFound, "could not recover credential secrets from any controller: %v", lastErr)
		}
		return errors.Codef(errors.CodeNotFound, "credential secrets not found on any controller")
	}

	if dryRun {
		// Dry-run: secrets were successfully fetched but we do not write
		// them back into the credential store.
		return nil
	}

	// Store the recovered attributes back into JIMM's credential store (Vault).
	if err := j.CredentialStore.Put(ctx, credential.ResourceTag(), attrs); err != nil {
		return fmt.Errorf("failed to store recovered credentials: %w", err)
	}

	return nil
}
