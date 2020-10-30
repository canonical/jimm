// Copyright 2015 Canonical Ltd.

package jem

import (
	"context"
	"fmt"
	"path"

	"go.uber.org/zap"
	"gopkg.in/errgo.v1"
	"gopkg.in/macaroon-bakery.v2/bakery/identchecker"

	"github.com/CanonicalLtd/jimm/internal/apiconn"
	"github.com/CanonicalLtd/jimm/internal/auth"
	"github.com/CanonicalLtd/jimm/internal/jem/jimmdb"
	"github.com/CanonicalLtd/jimm/internal/mongodoc"
	"github.com/CanonicalLtd/jimm/internal/zapctx"
	"github.com/CanonicalLtd/jimm/internal/zaputil"
	"github.com/CanonicalLtd/jimm/params"
	jujuparams "github.com/juju/juju/apiserver/params"
)

// GetCredential retrieves the given credential from the database,
// validating that the current user is allowed to read the credential.
func (j *JEM) GetCredential(ctx context.Context, id identchecker.ACLIdentity, cred *mongodoc.Credential) error {
	if err := j.DB.GetCredential(ctx, cred); err != nil {
		if errgo.Cause(err) == params.ErrNotFound {
			// We return an authorization error for all attempts to retrieve credentials
			// from any other user's space.
			if aerr := auth.CheckIsUser(ctx, id, params.User(cred.Path.User)); aerr != nil {
				err = aerr
			}
		}
		return errgo.Mask(err, errgo.Is(params.ErrNotFound), errgo.Is(params.ErrUnauthorized))
	}
	ctx = zapctx.WithFields(ctx, zap.Stringer("credential", cred.Path))
	if err := auth.CheckCanRead(ctx, id, cred); err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrUnauthorized))
	}

	// ensure we always have a provider-type in the credential.
	if cred.ProviderType == "" {
		cr := mongodoc.CloudRegion{Cloud: params.Cloud(cred.Path.Cloud)}
		if err := j.DB.GetCloudRegion(ctx, &cr); err != nil {
			zapctx.Error(ctx, "cannot find provider type for credential", zap.Error(err))
		}
		if err := j.DB.UpdateCredential(ctx, cred, new(jimmdb.Update).Set("providertype", cr.ProviderType), true); err != nil {
			zapctx.Error(ctx, "cannot update credential with provider type", zap.Error(err))
		}
	}

	return nil
}

// FillCredentialAttributes ensures that the credential attributes of the
// given credential are set. User access is not checked in this method, it
// is assumed that if the credential is held the user has access.
func (j *JEM) FillCredentialAttributes(ctx context.Context, cred *mongodoc.Credential) error {
	if !cred.AttributesInVault || len(cred.Attributes) > 0 {
		return nil
	}
	if j.pool.config.VaultClient == nil {
		return errgo.New("vault not configured")
	}

	logical := j.pool.config.VaultClient.Logical()
	secret, err := logical.Read(path.Join(j.pool.config.VaultPath, "creds", cred.Path.String()))
	if err != nil {
		return errgo.Mask(err)
	}
	if secret == nil {
		// secret will be nil if it is not there. Return an error if we
		// Don't expect the attributes to be empty.
		if cred.Type == "empty" {
			return nil
		}
		return errgo.New("credential attributes not found")
	}
	cred.Attributes = make(map[string]string, len(secret.Data))
	for k, v := range secret.Data {
		// Nothing will be stored that isn't a string, so ignore anything
		// that is a different type.
		s, ok := v.(string)
		if !ok {
			continue
		}
		cred.Attributes[k] = s
	}
	return nil
}

// RevokeCredential checks that the credential with the given path
// can be revoked (if flags&CredentialCheck!=0) and revokes
// the credential (if flags&CredentialUpdate!=0).
// If flags==0, it acts as if both CredentialCheck and CredentialUpdate
// were set.
func (j *JEM) RevokeCredential(ctx context.Context, id identchecker.ACLIdentity, cred *mongodoc.Credential, flags CredentialUpdateFlags) error {
	if flags == 0 {
		flags = ^0
	}
	if err := j.GetCredential(ctx, id, cred); err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrNotFound), errgo.Is(params.ErrUnauthorized))
	}
	controllers := cred.Controllers
	if flags&CredentialCheck != 0 {
		n, err := j.DB.CountModels(ctx, jimmdb.Eq("credential", cred.Path))
		if err != nil {
			return errgo.Mask(err)
		}
		if n > 0 {
			// TODO more informative error message.
			return errgo.Newf("cannot revoke because credential is in use on at least one model")
		}
	}
	if flags&CredentialUpdate == 0 {
		return nil
	}
	if err := j.DB.UpsertCredential(ctx, &mongodoc.Credential{
		Path:       cred.Path,
		Attributes: map[string]string{},
		Revoked:    true,
	}); err != nil {
		return errgo.Notef(err, "cannot update local database")
	}
	ch := make(chan struct{}, len(controllers))
	n := len(controllers)
	for _, ctlPath := range controllers {
		ctlPath, j := ctlPath, j.Clone()
		go func() {
			defer func() {
				ch <- struct{}{}
			}()
			defer j.Close()
			conn, err := j.OpenAPI(ctx, ctlPath)
			if err != nil {
				zapctx.Warn(ctx,
					"cannot connect to controller to revoke credential",
					zap.String("controller", ctlPath.String()),
					zaputil.Error(err),
				)
				return
			}
			defer conn.Close()

			err = j.revokeControllerCredential(ctx, conn, ctlPath, cred.Path)
			if err != nil {
				zapctx.Warn(ctx,
					"cannot revoke credential",
					zap.String("controller", ctlPath.String()),
					zaputil.Error(err),
				)
			}
		}()
	}
	for n > 0 {
		select {
		case <-ch:
			n--
		case <-ctx.Done():
			return errgo.Notef(ctx.Err(), "timed out revoking credentials")
		}
	}
	return nil
}

type CredentialUpdateFlags int

const (
	CredentialUpdate CredentialUpdateFlags = 1 << iota
	CredentialCheck
)

// UpdateCredential checks that the credential can be updated (if the
// CredentialUpdate flag is set) and updates its in the local database
// and all controllers to which it is deployed (if the CredentialCheck
// flag is specified).
//
// If flags is zero, it will both check and update.
func (j *JEM) UpdateCredential(ctx context.Context, id identchecker.ACLIdentity, cred *mongodoc.Credential, flags CredentialUpdateFlags) ([]jujuparams.UpdateCredentialModelResult, error) {
	if cred.Revoked {
		return nil, errgo.Newf("cannot use UpdateCredential to revoke a credential")
	}
	if flags == 0 {
		flags = ^0
	}
	var controllers []params.EntityPath
	c := mongodoc.Credential{
		Path: cred.Path,
	}
	if err := j.GetCredential(ctx, id, &c); err == nil {
		controllers = c.Controllers
	} else if errgo.Cause(err) != params.ErrNotFound {
		return nil, errgo.Mask(err)
	}
	if flags&CredentialCheck != 0 {
		// There is a credential already recorded, so check with all its controllers
		// that it's valid before we update it locally and update it on the controllers.
		models, err := j.checkCredential(ctx, cred, controllers)
		if err != nil || flags&CredentialUpdate == 0 {
			return models, errgo.Mask(err, apiconn.IsAPIError)
		}
	}

	// Try to ensure that we set the provider type.
	cred.ProviderType = c.ProviderType
	if cred.ProviderType == "" {
		cr := mongodoc.CloudRegion{Cloud: params.Cloud(cred.Path.Cloud)}
		if err := j.DB.GetCloudRegion(ctx, &cr); err != nil {
			zapctx.Warn(ctx, "cannot determine provider type", zap.Error(err), zap.String("cloud", cred.Path.Cloud))
		}
		cred.ProviderType = cr.ProviderType
	}

	// Note that because CredentialUpdate is checked for inside the
	// CredentialCheck case above, we know that we need to
	// update the credential in this case.
	models, err := j.updateCredential(ctx, cred, controllers)
	return models, errgo.Mask(err, apiconn.IsAPIError)
}

func (j *JEM) updateCredential(ctx context.Context, cred *mongodoc.Credential, controllers []params.EntityPath) ([]jujuparams.UpdateCredentialModelResult, error) {
	// The credential has now been checked (or we're going
	// to force the update), so update it in the local database.
	// and mark in the local database that an update is required for
	// all controllers
	if j.pool.config.VaultClient != nil {
		// There is a vault, so store the actual credential in there.
		cred1 := *cred
		cred1.Attributes = nil
		cred1.AttributesInVault = true
		if err := j.DB.UpsertCredential(ctx, &cred1); err != nil {
			return nil, errgo.Notef(err, "cannot update local database")
		}
		data := make(map[string]interface{}, len(cred.Attributes))
		for k, v := range cred.Attributes {
			data[k] = v
		}
		if len(data) > 0 {
			// Don't attempt to write no data to the vault.
			logical := j.pool.config.VaultClient.Logical()
			_, err := logical.Write(path.Join(j.pool.config.VaultPath, "creds", cred.Path.String()), data)
			if err != nil {
				return nil, errgo.Mask(err)
			}
		}
	} else {
		cred.AttributesInVault = false
		if err := j.DB.UpsertCredential(ctx, cred); err != nil {
			return nil, errgo.Notef(err, "cannot update local database")
		}
	}
	if err := j.setCredentialUpdates(ctx, cred.Controllers, cred.Path); err != nil {
		return nil, errgo.Notef(err, "cannot mark controllers to be updated")
	}

	// Attempt to update all controllers to which the credential is
	// deployed. If these fail they will be updated by the monitor.
	// Make the channel buffered so we don't leak go-routines
	ch := make(chan updateCredentialResult, len(controllers))
	for _, ctlPath := range controllers {
		ctlPath, j := ctlPath, j.Clone()
		go func() {
			defer j.Close()
			conn, err := j.OpenAPI(ctx, ctlPath)
			if err != nil {
				ch <- updateCredentialResult{
					ctlPath: ctlPath,
					err:     errgo.Mask(err),
				}
				return
			}
			defer conn.Close()

			models, err := j.updateControllerCredential(ctx, conn, ctlPath, cred)
			ch <- updateCredentialResult{
				ctlPath: ctlPath,
				models:  models,
				err:     errgo.Mask(err, apiconn.IsAPIError),
			}
		}()
	}
	models, err := mergeUpdateCredentialResults(ctx, ch, len(controllers))
	return models, errgo.Mask(err, apiconn.IsAPIError)
}

func (j *JEM) checkCredential(ctx context.Context, newCred *mongodoc.Credential, controllers []params.EntityPath) ([]jujuparams.UpdateCredentialModelResult, error) {
	if len(controllers) == 0 {
		// No controllers, so there's nowhere to check that the credential
		// is valid.
		return nil, nil
	}
	ch := make(chan updateCredentialResult, len(controllers))
	for _, ctlPath := range controllers {
		ctlPath, j := ctlPath, j.Clone()
		go func() {
			defer j.Close()
			models, err := j.checkCredentialOnController(ctx, ctlPath, newCred)
			ch <- updateCredentialResult{ctlPath, models, errgo.Mask(err, apiconn.IsAPIError)}
		}()
	}
	models, err := mergeUpdateCredentialResults(ctx, ch, len(controllers))
	return models, errgo.Mask(err, apiconn.IsAPIError)
}

type updateCredentialResult struct {
	ctlPath params.EntityPath
	models  []jujuparams.UpdateCredentialModelResult
	err     error
}

func mergeUpdateCredentialResults(ctx context.Context, ch <-chan updateCredentialResult, n int) ([]jujuparams.UpdateCredentialModelResult, error) {
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
					zap.String("controller", r.ctlPath.String()),
					zaputil.Error(r.err),
				)
				if firstError == nil {
					firstError = errgo.NoteMask(r.err, fmt.Sprintf("controller %s", r.ctlPath), apiconn.IsAPIError)
				}
			}

		case <-ctx.Done():
			return nil, errgo.Notef(ctx.Err(), "timed out checking credentials")
		}
	}
	return models, errgo.Mask(firstError, apiconn.IsAPIError)
}

func (j *JEM) checkCredentialOnController(ctx context.Context, ctlPath params.EntityPath, cred *mongodoc.Credential) ([]jujuparams.UpdateCredentialModelResult, error) {
	conn, err := j.OpenAPI(ctx, ctlPath)
	if err != nil {
		return nil, errgo.Mask(err)
	}
	defer conn.Close()

	if !conn.SupportsCheckCredentialModels() {
		// Version 3 of the Cloud facade isn't supported, so there is nothing to do.
		return nil, nil
	}
	models, err := conn.CheckCredentialModels(ctx, cred)
	return models, errgo.Mask(err, apiconn.IsAPIError)
}

// updateControllerCredential updates the given credential (which must
// not be revoked) on the given controller.
// If rp is non-nil, it will be updated with information
// on the models updated.
func (j *JEM) updateControllerCredential(
	ctx context.Context,
	conn *apiconn.Conn,
	ctlPath params.EntityPath,
	cred *mongodoc.Credential,
) ([]jujuparams.UpdateCredentialModelResult, error) {
	if cred.Revoked {
		return nil, errgo.New("updateControllerCredential called with revoked credential (shouldn't happen)")
	}
	if err := j.FillCredentialAttributes(ctx, cred); err != nil {
		return nil, errgo.Mask(err)
	}
	models, err := conn.UpdateCredential(ctx, cred)
	if err == nil {
		if dberr := j.clearCredentialUpdate(ctx, ctlPath, cred.Path); dberr != nil {
			err = errgo.Notef(dberr, "cannot update controller %q after successfully updating credential", ctlPath)
		}
	}
	if err != nil {
		err = errgo.NoteMask(err, "cannot update credentials", apiconn.IsAPIError)
	}
	return models, err
}

func (j *JEM) revokeControllerCredential(
	ctx context.Context,
	conn *apiconn.Conn,
	ctlPath params.EntityPath,
	credPath mongodoc.CredentialPath,
) error {
	if err := conn.RevokeCredential(ctx, credPath); err != nil {
		return errgo.Mask(err, apiconn.IsAPIError)
	}
	if err := j.clearCredentialUpdate(ctx, ctlPath, credPath); err != nil {
		return errgo.Notef(err, "cannot update controller %q after successfully updating credential", ctlPath)
	}
	return nil
}

// credentialAddController stores the fact that the credential with the
// given user, cloud and name is present on the given controller.
func (j *JEM) credentialAddController(ctx context.Context, c *mongodoc.Credential, controller params.EntityPath) error {
	err := j.DB.UpdateCredential(ctx, c, new(jimmdb.Update).AddToSet("controllers", controller), true)
	return errgo.Mask(err, errgo.Is(params.ErrNotFound))
}

// credentialsRemoveController stores the fact that the given controller
// was removed and credentials are no longer present there.
func (j *JEM) credentialsRemoveController(ctx context.Context, controller params.EntityPath) error {
	_, err := j.DB.UpdateCredentials(ctx, nil, new(jimmdb.Update).Pull("controllers", controller))
	if err != nil {
		return errgo.Notef(err, "cannot remove controller from credentials")
	}
	return nil
}

var errStop = errgo.New("stop")

// ForEachCredential iterates through each controller owned by the given
// user that as visible to the given identity and calls f for each one.
// If a non-zero cloud is specified then only credentials for the cloud
// will be included. If f returns an error the iteration stops immediately
// and the error is returned with the cause unmasked.
func (j *JEM) ForEachCredential(ctx context.Context, id identchecker.ACLIdentity, user params.User, cloud params.Cloud, f func(c *mongodoc.Credential) error) error {
	var ferr error
	qs := make([]jimmdb.Query, 2, 3)
	qs[0] = jimmdb.Eq("path.entitypath.user", user)
	qs[1] = jimmdb.Eq("revoked", false)
	if cloud != "" {
		qs = append(qs, jimmdb.Eq("path.cloud", cloud))
	}
	err := j.DB.ForEachCredential(ctx, jimmdb.And(qs...), nil, func(c *mongodoc.Credential) error {
		if err := auth.CheckCanRead(ctx, id, c); err != nil {
			if errgo.Cause(err) == params.ErrUnauthorized {
				err = nil
			}
			return errgo.Mask(err)
		}
		if err := f(c); err != nil {
			ferr = err
			return errStop
		}
		return nil
	})
	if errgo.Cause(err) == errStop {
		return errgo.Mask(ferr, errgo.Any)
	}
	return errgo.Mask(err)
}
