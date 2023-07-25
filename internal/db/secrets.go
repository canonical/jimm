// Copyright 2023 Canonical Ltd.

package db

import (
	"context"
	"encoding/json"
	"time"

	"github.com/canonical/jimm/internal/dbmodel"
	"github.com/canonical/jimm/internal/errors"
	"github.com/juju/names/v4"
	"github.com/lestrrat-go/jwx/v2/jwk"
)

const (
	// These keys provide consistency across get/put methods.
	usernameKey   = "username"
	passwordKey   = "password"
	jwksExpiryKey = "expiry"

	// These constants are used to create the appropriate identifiers for JWKS related data.
	jwksKind          = "jwks"
	jwksPublicKeyTag  = "jwksPublicKey"
	jwksPrivateKeyTag = "jwksPrivateKey"
	jwksExpiryTag     = "jwksExpiry"
)

// AddSecret stores secret information.
//   - returns an error with code errors.CodeAlreadyExists if
//     secret with the same type+tag already exists.
func (d *Database) AddSecret(ctx context.Context, secret *dbmodel.Secret) error {
	const op = errors.Op("db.AddSecret")
	if err := d.ready(); err != nil {
		return errors.E(op, err)
	}
	db := d.DB.WithContext(ctx)

	if err := db.Create(secret).Error; err != nil {
		return errors.E(op, dbError(err))
	}
	return nil
}

func (d *Database) GetSecret(ctx context.Context, secret *dbmodel.Secret) error {
	const op = errors.Op("db.AddSecret")
	if err := d.ready(); err != nil {
		return errors.E(op, err)
	}
	db := d.DB.WithContext(ctx)
	if secret.Tag == "" || secret.Type == "" {
		return errors.E(op, "missing secret tag and type", errors.CodeBadRequest)
	}

	db = db.Where("tag = ?", secret.Tag).Where("type = ?", secret.Type)

	if err := db.First(&secret).Error; err != nil {
		err = dbError(err)
		if errors.ErrorCode(err) == errors.CodeNotFound {
			return errors.E(op, err, "secret not found")
		}
		return errors.E(op, dbError(err))
	}
	return nil
}

func (d *Database) DeleteSecret(ctx context.Context, secret *dbmodel.Secret) error {
	const op = errors.Op("db.DeleteSecret")
	if err := d.ready(); err != nil {
		return errors.E(op, err)
	}
	db := d.DB.WithContext(ctx)

	if err := db.Unscoped().Where("tag = ?", secret.Tag).Where("type = ?", secret.Type).Delete(&dbmodel.Secret{}).Error; err != nil {
		return errors.E(op, dbError(err))
	}
	return nil
}

func (d *Database) UpdateSecret(ctx context.Context, secret *dbmodel.Secret) error {
	const op = errors.Op("db.UpdateSecret")
	if err := d.ready(); err != nil {
		return errors.E(op, err)
	}
	db := d.DB.WithContext(ctx)
	db = db.Save(secret)
	if db.Error != nil {
		return errors.E(op, dbError(db.Error))
	}
	return nil
}

// newSecret creates a secret object with the time set to the current time
// and the type and tag fields set from the tag object
func newSecret(secretType string, secretTag string, data []byte) dbmodel.Secret {
	return dbmodel.Secret{Time: time.Now(), Type: secretType, Tag: secretTag, Data: data}
}

// Get retrieves the attributes for the given cloud credential from a vault
// service.
func (d *Database) Get(ctx context.Context, tag names.CloudCredentialTag) (map[string]string, error) {
	const op = errors.Op("database.Get")
	secret := newSecret(tag.Kind(), tag.String(), nil)
	err := d.GetSecret(ctx, &secret)
	if err != nil {
		return nil, errors.E(op, err)
	}
	var attr map[string]string
	err = json.Unmarshal(secret.Data, &attr)
	if err != nil {
		return nil, errors.E(op, err)
	}
	return attr, nil
}

// Put stores the attributes associated with a cloud-credential in a vault
// service.
func (d *Database) Put(ctx context.Context, tag names.CloudCredentialTag, attr map[string]string) error {
	const op = errors.Op("database.Put")
	if len(attr) == 0 {
		return d.delete(ctx, tag)
	}
	dataJson, err := json.Marshal(attr)
	if err != nil {
		return errors.E(op, err, "failed to marshal secret data")
	}
	secret := newSecret(tag.Kind(), tag.String(), dataJson)
	return d.AddSecret(ctx, &secret)
}

// delete removes the attributes associated with the cloud-credential in
// the database.
func (d *Database) delete(ctx context.Context, tag names.CloudCredentialTag) error {
	secret := newSecret(tag.Kind(), tag.String(), nil)
	return d.DeleteSecret(ctx, &secret)
}

// GetControllerCredentials retrieves the credentials for the given controller from a vault
// service.
func (d *Database) GetControllerCredentials(ctx context.Context, controllerName string) (string, string, error) {
	const op = errors.Op("database.GetControllerCredentials")
	secret := newSecret(names.ControllerTagKind, controllerName, nil)
	err := d.GetSecret(ctx, &secret)
	if err != nil {
		return "", "", errors.E(op, err)
	}
	var secretData map[string]string
	err = json.Unmarshal(secret.Data, &secretData)
	if err != nil {
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

// PutControllerCredentials stores the controller credentials in a vault
// service.
func (d *Database) PutControllerCredentials(ctx context.Context, controllerName string, username string, password string) error {
	const op = errors.Op("database.PutControllerCredentials")
	secretData := make(map[string]string)
	secretData[usernameKey] = username
	secretData[passwordKey] = password
	dataJson, err := json.Marshal(secretData)
	if err != nil {
		return errors.E(op, err, "failed to marshal secret data")
	}
	secret := newSecret(names.ControllerTagKind, controllerName, dataJson)
	return d.AddSecret(ctx, &secret)
}

// CleanupJWKS removes all secrets associated with the JWKS process.
func (d *Database) CleanupJWKS(ctx context.Context) error {
	const op = errors.Op("database.CleanupJWKS")
	secret := newSecret(jwksKind, jwksPublicKeyTag, nil)
	return d.DeleteSecret(ctx, &secret)
}

// GetJWKS returns the current key set stored within the credential store.
func (d *Database) GetJWKS(ctx context.Context) (jwk.Set, error) {
	const op = errors.Op("database.GetJWKS")
	secret := newSecret(jwksKind, jwksPublicKeyTag, nil)
	err := d.GetSecret(ctx, &secret)
	if err != nil {
		return nil, errors.E(op, err)
	}
	ks, err := jwk.ParseString(string(secret.Data))
	if err != nil {
		return nil, errors.E(op, err)
	}
	return ks, nil
}

// GetJWKSPrivateKey returns the current private key for the active JWKS
func (d *Database) GetJWKSPrivateKey(ctx context.Context) ([]byte, error) {
	const op = errors.Op("database.GetJWKSPrivateKey")
	secret := newSecret(jwksKind, jwksPublicKeyTag, nil)
	err := d.GetSecret(ctx, &secret)
	if err != nil {
		return nil, errors.E(op, err)
	}
	return secret.Data, nil
}

// GetJWKSExpiry returns the expiry of the active JWKS.
func (d *Database) GetJWKSExpiry(ctx context.Context) (time.Time, error) {
	const op = errors.Op("database.GetJWKSExpiry")
	secret := newSecret(jwksKind, jwksExpiryTag, nil)
	err := d.GetSecret(ctx, &secret)
	if err != nil {
		return time.Time{}, errors.E(op, err)
	}
	var secretData map[string]time.Time
	err = json.Unmarshal(secret.Data, &secretData)
	if err != nil {
		return time.Time{}, errors.E(op, err)
	}
	expiry, ok := secretData[jwksExpiryKey]
	if !ok {
		return time.Time{}, errors.E(op, "failed to retrieve expiry")
	}
	return expiry, nil
}

// PutJWKS puts a JWKS into the credential store.
func (d *Database) PutJWKS(ctx context.Context, jwks jwk.Set) error {
	const op = errors.Op("database.PutJWKS")
	jwksJson, err := json.Marshal(jwks)
	if err != nil {
		return errors.E(op, err, "failed to marshal jwks data")
	}
	secret := newSecret(jwksKind, jwksPublicKeyTag, jwksJson)
	return d.AddSecret(ctx, &secret)

}

// PutJWKSPrivateKey persists the private key associated with the current JWKS within the store.
func (d *Database) PutJWKSPrivateKey(ctx context.Context, pem []byte) error {
	const op = errors.Op("database.PutJWKSPrivateKey")
	secret := newSecret(jwksKind, jwksPrivateKeyTag, pem)
	return d.AddSecret(ctx, &secret)
}

// PutJWKSExpiry sets the expiry time for the current JWKS within the store.
func (d *Database) PutJWKSExpiry(ctx context.Context, expiry time.Time) error {
	const op = errors.Op("database.PutJWKSExpiry")
	expiryMap := map[string]time.Time{jwksExpiryKey: expiry}
	expiryJson, err := json.Marshal(expiryMap)
	if err != nil {
		return errors.E(op, err, "failed to marshal jwks data")
	}
	secret := newSecret(jwksKind, jwksExpiryTag, expiryJson)
	return d.AddSecret(ctx, &secret)
}
