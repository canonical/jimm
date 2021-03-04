// Copyright 2020 Canonical Ltd.

package jimm

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"path"
	"sort"
	"sync"

	vault "github.com/hashicorp/vault/api"
	jujuparams "github.com/juju/juju/apiserver/params"
	"github.com/juju/names/v4"

	"github.com/CanonicalLtd/jimm/internal/cloudcred"
	"github.com/CanonicalLtd/jimm/internal/dbmodel"
	"github.com/CanonicalLtd/jimm/internal/errors"
)

// GetCloudCredential retrieves the given credential from the database. The
// returned credential will never contain any attrbiutes, see
// GetCloudCredentialAttributes to retrieve those. If credentials
// identified by the given tag cannot be found then an errror with a code
// of CodeNotFound will be returned. If the given user is not a controller
// superuser or the owner of the credentials then an error with a code of
// CodeUnauthorized will be returned.
func (j *JIMM) GetCloudCredential(ctx context.Context, user *dbmodel.User, tag names.CloudCredentialTag) (*dbmodel.CloudCredential, error) {
	const op = errors.Op("jimm.GetCloudCredential")

	if user.ControllerAccess != "superuser" && user.Username != tag.Owner().Id() {
		return nil, errors.E(op, errors.CodeUnauthorized)
	}

	var credential dbmodel.CloudCredential
	credential.SetTag(tag)

	err := j.Database.GetCloudCredential(ctx, &credential)
	if err != nil {
		return nil, errors.E(op, err)
	}
	credential.Attributes = nil

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
		return errors.E(op, err)
	}

	err = j.Database.SetCloudCredential(ctx, &credential)
	if err != nil {
		return errors.E(op, err, "failed to revoke credential in local database")
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
func (j *JIMM) UpdateCloudCredential(ctx context.Context, u *dbmodel.User, args UpdateCloudCredentialArgs) ([]jujuparams.UpdateCredentialModelResult, error) {
	const op = errors.Op("jimm.UpdateCloudCredential")

	if u.ControllerAccess != "superuser" && u.Username != args.CredentialTag.Owner().Id() {
		return nil, errors.E(op, errors.CodeUnauthorized)
	}

	var credential dbmodel.CloudCredential
	credential.SetTag(args.CredentialTag)

	err := j.Database.GetCloudCredential(ctx, &credential)
	if err != nil && errors.ErrorCode(err) != errors.CodeNotFound {
		return nil, errors.E(op, err)
	}

	models, err := j.Database.GetModelsUsingCredential(ctx, credential.ID)
	if err != nil {
		return nil, errors.E(op, err)
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
	credential.Attributes = args.Credential.Attributes

	var resultMu sync.Mutex
	var result []jujuparams.UpdateCredentialModelResult
	if !args.SkipCheck {
		err := j.forEachController(ctx, controllers, func(ctl *dbmodel.Controller, api API) error {
			models, err := j.updateControllerCloudCredential(ctx, &credential, api.CheckCredentialModels)
			if err != nil {
				return err
			}
			resultMu.Lock()
			defer resultMu.Unlock()
			result = append(result, models...)
			return nil
		})
		if err != nil {
			return result, errors.E(op, err)
		}
	}
	var modelsErr bool
	for _, r := range result {
		if len(r.Errors) > 0 {
			modelsErr = true
		}
	}
	if args.SkipUpdate || modelsErr {
		return result, nil
	}

	if err := j.updateCredential(ctx, &credential); err != nil {
		return result, errors.E(op, err)
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
		return result, errors.E(op, err)
	}
	return result, nil
}

// updateCredential updates the credential stored in JIMM's database.
func (j *JIMM) updateCredential(ctx context.Context, credential *dbmodel.CloudCredential) error {
	const op = errors.Op("jimm.updateCredential")

	if j.VaultClient == nil {
		credential.AttributesInVault = false
		if err := j.Database.SetCloudCredential(ctx, credential); err != nil {
			return errors.E(op, err)
		}
		return nil
	}

	credential1 := *credential
	credential1.Attributes = nil
	credential1.AttributesInVault = true
	if err := j.Database.SetCloudCredential(ctx, &credential1); err != nil {
		return errors.E(op, err)
	}

	data := make(map[string]interface{}, len(credential.Attributes))
	for k, v := range credential.Attributes {
		data[k] = v
	}
	logical := j.VaultClient.Logical()
	pth := path.Join(j.vaultCredPath(credential))

	var err error
	if len(data) == 0 {
		_, err = logical.Delete(pth)
		if rerr, ok := err.(*vault.ResponseError); ok && rerr.StatusCode == http.StatusNotFound {
			// Ignore the error if attempting to delete something that isn't there.
			err = nil
		}
	} else {
		_, err = logical.Write(pth, data)
	}
	if err != nil {
		return errors.E(op, err)
	}
	return nil
}

func (j *JIMM) updateControllerCloudCredential(
	ctx context.Context,
	cred *dbmodel.CloudCredential,
	f func(context.Context, jujuparams.TaggedCredential) ([]jujuparams.UpdateCredentialModelResult, error),
) ([]jujuparams.UpdateCredentialModelResult, error) {
	const op = errors.Op("jimm.updateControllerCloudCredential")

	attr := cred.Attributes
	if attr == nil {
		var err error
		attr, err = j.getCloudCredentialAttributes(ctx, cred)
		if err != nil {
			return nil, errors.E(op, err)
		}
	}

	models, err := f(ctx, jujuparams.TaggedCredential{
		Tag: cred.Tag().String(),
		Credential: jujuparams.CloudCredential{
			AuthType:   cred.AuthType,
			Attributes: attr,
		},
	})
	if err != nil {
		return models, errors.E(op, err)
	}
	return models, nil
}

// ForEachUserCloudCredential iterates through every credential owned by
// the given user and for the given cloud (if specified). The given
// function is called for each credential found. The credential used when
// calling the function will not contain any attributes,
// GetCloudCredentialAttributes should be used to retrive the credential
// attributes if needed. The given function should not update the database.
func (j *JIMM) ForEachUserCloudCredential(ctx context.Context, u *dbmodel.User, ct names.CloudTag, f func(cred *dbmodel.CloudCredential) error) error {
	const op = errors.Op("jimm.ForEachUserCloudCredential")

	var cloud string
	if ct != (names.CloudTag{}) {
		cloud = ct.Id()
	}

	errStop := errors.E("stop")
	var iterErr error
	err := j.Database.ForEachCloudCredential(ctx, u.Username, cloud, func(cred *dbmodel.CloudCredential) error {
		cred.Attributes = nil
		iterErr = f(cred)
		if iterErr != nil {
			return errStop
		}
		return nil
	})
	if err == errStop {
		err = iterErr
	} else if err != nil {
		err = errors.E(op, err)
	}
	return err
}

// GetCloudCredentialAttributes retrieves the attributes for a cloud
// credential. If hidden is true then returned credentials will include
// hidden attributes, otherwise a list of redacted attributes will be
// returned. Only the credential owner can retrieve hidden attributes any
// other user, including controller superusers, will recieve an error with
// the code CodeUnauthorized.
func (j *JIMM) GetCloudCredentialAttributes(ctx context.Context, u *dbmodel.User, cred *dbmodel.CloudCredential, hidden bool) (attrs map[string]string, redacted []string, err error) {
	const op = errors.Op("jimm.GetCloudCredentialAttributes")

	if hidden {
		// Controller superusers cannot read hidden credential attributes.
		if u.Username != cred.OwnerID {
			return nil, nil, errors.E(op, errors.CodeUnauthorized)
		}
	} else {
		if u.ControllerAccess != "superuser" && u.Username != cred.OwnerID {
			return nil, nil, errors.E(op, errors.CodeUnauthorized)
		}
	}

	attrs, err = j.getCloudCredentialAttributes(ctx, cred)
	if err != nil {
		err = errors.E(op, err)
		return
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
func (j *JIMM) getCloudCredentialAttributes(ctx context.Context, cred *dbmodel.CloudCredential) (map[string]string, error) {
	const op = errors.Op("jimm.getCloudCredentialAttributes")

	if !cred.AttributesInVault {
		if err := j.Database.GetCloudCredential(ctx, cred); err != nil {
			return nil, errors.E(op, err)
		}
	}
	if !cred.AttributesInVault {
		return map[string]string(cred.Attributes), nil
	}

	// Attributes have to be loaded from vault.
	if j.VaultClient == nil {
		return nil, errors.E(op, errors.CodeServerConfiguration, "vault not configured")
	}

	logical := j.VaultClient.Logical()
	secret, err := logical.Read(j.vaultCredPath(cred))
	if err != nil {
		return nil, errors.E(op, err)
	}
	if secret == nil {
		// secret will be nil if it is not there. Return an error if we
		// Don't expect the attributes to be empty.
		if cred.AuthType == "empty" {
			return nil, nil
		}
		return nil, errors.E(op, "credential attributes not found")
	}
	attributes := make(map[string]string, len(secret.Data))
	for k, v := range secret.Data {
		// Nothing will be stored that isn't a string, so ignore anything
		// that is a different type.
		s, ok := v.(string)
		if !ok {
			continue
		}
		attributes[k] = s
	}

	return attributes, nil
}

func (j *JIMM) vaultCredPath(cred *dbmodel.CloudCredential) string {
	return path.Join(j.VaultPath, "creds", cred.Path())
}
