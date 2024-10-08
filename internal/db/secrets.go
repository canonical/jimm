// Copyright 2024 Canonical.

package db

import (
	"context"
	"encoding/json"
	"time"

	"github.com/juju/names/v5"
	"github.com/juju/zaputil/zapctx"
	"github.com/lestrrat-go/jwx/v2/jwk"
	"go.uber.org/zap"
	"gorm.io/gorm/clause"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/servermon"
)

const (
	// These keys provide consistency across get/put methods.
	usernameKey = "username"
	passwordKey = "password"

	// These constants are used to create the appropriate identifiers for JWKS related data.
	jwksKind          = "jwks"
	jwksPublicKeyTag  = "jwksPublicKey"
	jwksPrivateKeyTag = "jwksPrivateKey"
	jwksExpiryTag     = "jwksExpiry"
	oauthKind         = "oauth"
	oauthKeyTag       = "oauthKey"
	//nolint:gosec // Thinks credentials hardcoded.
	oauthSessionStoreSecretTag = "oauthSessionStoreSecret"
)

// UpsertSecret stores secret information.
//   - updates the secret's time and data if it already exists
func (d *Database) UpsertSecret(ctx context.Context, secret *dbmodel.Secret) (err error) {
	const op = errors.Op("db.AddSecret")
	if err := d.ready(); err != nil {
		return errors.E(op, err)
	}

	durationObserver := servermon.DurationObserver(servermon.DBQueryDurationHistogram, string(op))
	defer durationObserver()
	defer servermon.ErrorCounter(servermon.DBQueryErrorCount, &err, string(op))

	// On conflict perform an upset to make the operation resemble a Put.
	db := d.DB.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "type"}, {Name: "tag"}},
		DoUpdates: clause.AssignmentColumns([]string{"time", "data"}),
	})
	if err := db.Create(secret).Error; err != nil {
		return errors.E(op, dbError(err))
	}
	return nil
}

// GetSecret gets the secret with the specified type and tag.
func (d *Database) GetSecret(ctx context.Context, secret *dbmodel.Secret) (err error) {
	const op = errors.Op("db.GetSecret")

	if secret.Tag == "" || secret.Type == "" {
		return errors.E(op, "missing secret tag and type", errors.CodeBadRequest)
	}

	if err := d.ready(); err != nil {
		return errors.E(op, err)
	}

	durationObserver := servermon.DurationObserver(servermon.DBQueryDurationHistogram, string(op))
	defer durationObserver()
	defer servermon.ErrorCounter(servermon.DBQueryErrorCount, &err, string(op))

	db := d.DB.WithContext(ctx)

	db = db.Where("tag = ? AND type = ?", secret.Tag, secret.Type)

	if err := db.First(&secret).Error; err != nil {
		err = dbError(err)
		if errors.ErrorCode(err) == errors.CodeNotFound {
			return errors.E(op, err, "secret not found")
		}
		return errors.E(op, dbError(err))
	}
	return nil
}

// Delete secret deletes the secret with the specified type and tag.
func (d *Database) DeleteSecret(ctx context.Context, secret *dbmodel.Secret) (err error) {
	const op = errors.Op("db.DeleteSecret")

	if secret.Tag == "" || secret.Type == "" {
		return errors.E(op, "missing secret tag and type", errors.CodeBadRequest)
	}

	if err := d.ready(); err != nil {
		return errors.E(op, err)
	}

	durationObserver := servermon.DurationObserver(servermon.DBQueryDurationHistogram, string(op))
	defer durationObserver()
	defer servermon.ErrorCounter(servermon.DBQueryErrorCount, &err, string(op))

	db := d.DB.WithContext(ctx)

	if err := db.Unscoped().Where("tag = ? AND type = ?", secret.Tag, secret.Type).Delete(&dbmodel.Secret{}).Error; err != nil {
		return errors.E(op, dbError(err))
	}
	return nil
}

// Get retrieves the attributes for the given cloud credential from the DB.
func (d *Database) Get(ctx context.Context, tag names.CloudCredentialTag) (_ map[string]string, err error) {
	const op = errors.Op("database.Get")

	if err := d.ready(); err != nil {
		return nil, errors.E(op, err)
	}

	durationObserver := servermon.DurationObserver(servermon.DBQueryDurationHistogram, string(op))
	defer durationObserver()
	defer servermon.ErrorCounter(servermon.DBQueryErrorCount, &err, string(op))

	secret := dbmodel.NewSecret(tag.Kind(), tag.String(), nil)
	err = d.GetSecret(ctx, &secret)
	if err != nil {
		zapctx.Error(ctx, "failed to get secret data", zap.Error(err))
		return nil, errors.E(op, err)
	}
	var attr map[string]string
	err = json.Unmarshal(secret.Data, &attr)
	if err != nil {
		zapctx.Error(ctx, "failed to unmarshal secret data", zap.Error(err))
		return nil, errors.E(op, err)
	}
	return attr, nil
}

// Put stores the attributes associated with a cloud-credential in the DB.
func (d *Database) Put(ctx context.Context, tag names.CloudCredentialTag, attr map[string]string) (err error) {
	const op = errors.Op("database.Put")

	if err := d.ready(); err != nil {
		return errors.E(op, err)
	}

	durationObserver := servermon.DurationObserver(servermon.DBQueryDurationHistogram, string(op))
	defer durationObserver()
	defer servermon.ErrorCounter(servermon.DBQueryErrorCount, &err, string(op))

	dataJson, err := json.Marshal(attr)
	if err != nil {
		zapctx.Error(ctx, "failed to marshal secret data", zap.Error(err))
		return errors.E(op, err, "failed to marshal secret data")
	}
	secret := dbmodel.NewSecret(tag.Kind(), tag.String(), dataJson)
	return d.UpsertSecret(ctx, &secret)
}

// GetControllerCredentials retrieves the credentials for the given controller from the DB.
// It is expected for this interface that a non-existent controller credential return empty username/password.
func (d *Database) GetControllerCredentials(ctx context.Context, controllerName string) (_ string, _ string, err error) {
	const op = errors.Op("database.GetControllerCredentials")

	if err := d.ready(); err != nil {
		return "", "", errors.E(op, err)
	}

	durationObserver := servermon.DurationObserver(servermon.DBQueryDurationHistogram, string(op))
	defer durationObserver()
	defer servermon.ErrorCounter(servermon.DBQueryErrorCount, &err, string(op))

	secret := dbmodel.NewSecret(names.ControllerTagKind, controllerName, nil)
	err = d.GetSecret(ctx, &secret)
	if errors.ErrorCode(err) == errors.CodeNotFound {
		return "", "", nil
	}
	if err != nil {
		zapctx.Error(ctx, "failed to get secret data", zap.Error(err))
		return "", "", errors.E(op, err)
	}
	var secretData map[string]string
	err = json.Unmarshal(secret.Data, &secretData)
	if err != nil {
		zapctx.Error(ctx, "failed to unmarshal secret data", zap.Error(err))
		return "", "", errors.E(op, err)
	}
	username, ok := secretData[usernameKey]
	if !ok {
		return "", "", errors.E(op, "missing username")
	}
	password, ok := secretData[passwordKey]
	if !ok {
		return "", "", errors.E(op, "missing password")
	}
	return username, password, nil
}

// PutControllerCredentials stores the controller credentials in the DB.
func (d *Database) PutControllerCredentials(ctx context.Context, controllerName string, username string, password string) (err error) {
	const op = errors.Op("database.PutControllerCredentials")

	if err := d.ready(); err != nil {
		return errors.E(op, err)
	}

	durationObserver := servermon.DurationObserver(servermon.DBQueryDurationHistogram, string(op))
	defer durationObserver()
	defer servermon.ErrorCounter(servermon.DBQueryErrorCount, &err, string(op))

	secretData := make(map[string]string)
	secretData[usernameKey] = username
	secretData[passwordKey] = password
	dataJson, err := json.Marshal(secretData)
	if err != nil {
		zapctx.Error(ctx, "failed to unmarshal secret data", zap.Error(err))
		return errors.E(op, err, "failed to marshal secret data")
	}
	secret := dbmodel.NewSecret(names.ControllerTagKind, controllerName, dataJson)
	return d.UpsertSecret(ctx, &secret)
}

// CleanupJWKS removes all secrets associated with the JWKS process.
func (d *Database) CleanupJWKS(ctx context.Context) (err error) {
	const op = errors.Op("database.CleanupJWKS")

	if err := d.ready(); err != nil {
		return errors.E(op, err)
	}

	durationObserver := servermon.DurationObserver(servermon.DBQueryDurationHistogram, string(op))
	defer durationObserver()
	defer servermon.ErrorCounter(servermon.DBQueryErrorCount, &err, string(op))

	secret := dbmodel.NewSecret(jwksKind, jwksPublicKeyTag, nil)
	err = d.DeleteSecret(ctx, &secret)
	if err != nil {
		zapctx.Error(ctx, "failed to cleanup jwks public key", zap.Error(err))
		return errors.E(op, err, "failed to cleanup jwks public key")
	}
	secret = dbmodel.NewSecret(jwksKind, jwksPrivateKeyTag, nil)
	err = d.DeleteSecret(ctx, &secret)
	if err != nil {
		zapctx.Error(ctx, "failed to cleanup jwks private key", zap.Error(err))
		return errors.E(op, err, "failed to cleanup jwks private key")
	}
	secret = dbmodel.NewSecret(jwksKind, jwksExpiryTag, nil)
	err = d.DeleteSecret(ctx, &secret)
	if err != nil {
		zapctx.Error(ctx, "failed to cleanup jwks expiry info", zap.Error(err))
		return errors.E(op, err, "failed to cleanup jwks expiry info")
	}
	return nil
}

// GetJWKS returns the current key set stored within the DB.
func (d *Database) GetJWKS(ctx context.Context) (_ jwk.Set, err error) {
	const op = errors.Op("database.GetJWKS")

	if err := d.ready(); err != nil {
		return nil, errors.E(op, err)
	}

	durationObserver := servermon.DurationObserver(servermon.DBQueryDurationHistogram, string(op))
	defer durationObserver()
	defer servermon.ErrorCounter(servermon.DBQueryErrorCount, &err, string(op))

	secret := dbmodel.NewSecret(jwksKind, jwksPublicKeyTag, nil)
	err = d.GetSecret(ctx, &secret)
	if err != nil {
		zapctx.Error(ctx, "failed to get jwks data", zap.Error(err))
		return nil, errors.E(op, err)
	}
	ks, err := jwk.ParseString(string(secret.Data))
	if err != nil {
		zapctx.Error(ctx, "failed to parse jwk data", zap.Error(err))
		return nil, errors.E(op, err)
	}
	return ks, nil
}

// GetJWKSPrivateKey returns the current private key for the active JWKS
func (d *Database) GetJWKSPrivateKey(ctx context.Context) (_ []byte, err error) {
	const op = errors.Op("database.GetJWKSPrivateKey")

	if err := d.ready(); err != nil {
		return nil, errors.E(op, err)
	}

	durationObserver := servermon.DurationObserver(servermon.DBQueryDurationHistogram, string(op))
	defer durationObserver()
	defer servermon.ErrorCounter(servermon.DBQueryErrorCount, &err, string(op))

	secret := dbmodel.NewSecret(jwksKind, jwksPrivateKeyTag, nil)
	err = d.GetSecret(ctx, &secret)
	if err != nil {
		zapctx.Error(ctx, "failed to get jwks jwks private key", zap.Error(err))
		return nil, errors.E(op, err)
	}
	var pem []byte
	err = json.Unmarshal(secret.Data, &pem)
	if err != nil {
		zapctx.Error(ctx, "failed to unmarshal pem data data", zap.Error(err))
		return nil, errors.E(op, err)
	}
	return pem, nil
}

// GetJWKSExpiry returns the expiry of the active JWKS.
func (d *Database) GetJWKSExpiry(ctx context.Context) (_ time.Time, err error) {
	const op = errors.Op("database.GetJWKSExpiry")

	if err := d.ready(); err != nil {
		return time.Time{}, errors.E(op, err)
	}

	durationObserver := servermon.DurationObserver(servermon.DBQueryDurationHistogram, string(op))
	defer durationObserver()
	defer servermon.ErrorCounter(servermon.DBQueryErrorCount, &err, string(op))

	secret := dbmodel.NewSecret(jwksKind, jwksExpiryTag, nil)
	err = d.GetSecret(ctx, &secret)
	if err != nil {
		zapctx.Error(ctx, "failed to get jwks expiry", zap.Error(err))
		return time.Time{}, errors.E(op, err)
	}
	var expiryTime time.Time
	err = json.Unmarshal(secret.Data, &expiryTime)
	if err != nil {
		zapctx.Error(ctx, "failed to unmarshal jwks expiry data", zap.Error(err))
		return time.Time{}, errors.E(op, err)
	}
	return expiryTime, nil
}

// PutJWKS puts a JWKS into the DB.
func (d *Database) PutJWKS(ctx context.Context, jwks jwk.Set) (err error) {
	const op = errors.Op("database.PutJWKS")

	if err := d.ready(); err != nil {
		return errors.E(op, err)
	}

	durationObserver := servermon.DurationObserver(servermon.DBQueryDurationHistogram, string(op))
	defer durationObserver()
	defer servermon.ErrorCounter(servermon.DBQueryErrorCount, &err, string(op))

	jwksJson, err := json.Marshal(jwks)
	if err != nil {
		zapctx.Error(ctx, "failed to marshal jwks", zap.Error(err))
		return errors.E(op, err, "failed to marshal jwks data")
	}
	secret := dbmodel.NewSecret(jwksKind, jwksPublicKeyTag, jwksJson)
	return d.UpsertSecret(ctx, &secret)

}

// PutJWKSPrivateKey persists the private key associated with the current JWKS within the DB.
func (d *Database) PutJWKSPrivateKey(ctx context.Context, pem []byte) (err error) {
	const op = errors.Op("database.PutJWKSPrivateKey")

	if err := d.ready(); err != nil {
		return errors.E(op, err)
	}

	durationObserver := servermon.DurationObserver(servermon.DBQueryDurationHistogram, string(op))
	defer durationObserver()
	defer servermon.ErrorCounter(servermon.DBQueryErrorCount, &err, string(op))

	privateKeyJson, err := json.Marshal(pem)
	if err != nil {
		zapctx.Error(ctx, "failed to marshal pem data", zap.Error(err))
		return errors.E(op, err, "failed to marshal jwks private key")
	}
	secret := dbmodel.NewSecret(jwksKind, jwksPrivateKeyTag, privateKeyJson)
	return d.UpsertSecret(ctx, &secret)
}

// PutJWKSExpiry sets the expiry time for the current JWKS within the DB.
func (d *Database) PutJWKSExpiry(ctx context.Context, expiry time.Time) (err error) {
	const op = errors.Op("database.PutJWKSExpiry")

	if err := d.ready(); err != nil {
		return errors.E(op, err)
	}

	durationObserver := servermon.DurationObserver(servermon.DBQueryDurationHistogram, string(op))
	defer durationObserver()
	defer servermon.ErrorCounter(servermon.DBQueryErrorCount, &err, string(op))

	expiryJson, err := json.Marshal(expiry)
	if err != nil {
		zapctx.Error(ctx, "failed to marshal jwks expiry data", zap.Error(err))
		return errors.E(op, err, "failed to marshal jwks data")
	}
	secret := dbmodel.NewSecret(jwksKind, jwksExpiryTag, expiryJson)
	return d.UpsertSecret(ctx, &secret)
}

// CleanupOAuthSecrets removes all secrets associated with OAuth.
func (d *Database) CleanupOAuthSecrets(ctx context.Context) (err error) {
	const op = errors.Op("database.CleanupOAuthSecrets")

	if err := d.ready(); err != nil {
		return errors.E(op, err)
	}

	durationObserver := servermon.DurationObserver(servermon.DBQueryDurationHistogram, string(op))
	defer durationObserver()
	defer servermon.ErrorCounter(servermon.DBQueryErrorCount, &err, string(op))

	secret := dbmodel.NewSecret(oauthKind, oauthKeyTag, nil)
	if err := d.DeleteSecret(ctx, &secret); err != nil {
		zapctx.Error(ctx, "failed to cleanup OAuth key", zap.Error(err))
		return errors.E(op, err, "failed to cleanup OAuth key")
	}
	secret = dbmodel.NewSecret(oauthKind, oauthSessionStoreSecretTag, nil)
	if err := d.DeleteSecret(ctx, &secret); err != nil {
		zapctx.Error(ctx, "failed to cleanup OAuth session store secret", zap.Error(err))
		return errors.E(op, err, "failed to cleanup OAuth session store secret")
	}
	return nil
}
