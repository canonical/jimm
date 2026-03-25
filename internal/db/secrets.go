// Copyright 2025 Canonical.

package db

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/juju/names/v5"
	"gorm.io/gorm/clause"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/servermon"
)

const (
	// These keys provide consistency across get/put methods.
	usernameKey = "username"
	passwordKey = "password"
)

// UpsertSecret stores secret information.
//   - updates the secret's time and data if it already exists
func (d *Database) UpsertSecret(ctx context.Context, secret *dbmodel.Secret) (err error) {
	const op = "db.AddSecret"
	if err := d.ready(); err != nil {
		return err
	}

	durationObserver := servermon.DurationObserver(servermon.DBQueryDurationHistogram, op)
	defer durationObserver()
	defer servermon.ErrorCounter(servermon.DBQueryErrorCount, &err, op)

	// On conflict perform an upset to make the operation resemble a Put.
	db := d.DB.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "type"}, {Name: "tag"}},
		DoUpdates: clause.AssignmentColumns([]string{"time", "data"}),
	})
	if err := db.Create(secret).Error; err != nil {
		return dbError(err)
	}
	return nil
}

// GetSecret gets the secret with the specified type and tag.
func (d *Database) GetSecret(ctx context.Context, secret *dbmodel.Secret) (err error) {
	const op = "db.GetSecret"

	if secret.Tag == "" || secret.Type == "" {
		return errors.Codef(errors.CodeBadRequest, "missing secret tag and type")
	}

	if err := d.ready(); err != nil {
		return err
	}

	durationObserver := servermon.DurationObserver(servermon.DBQueryDurationHistogram, op)
	defer durationObserver()
	defer servermon.ErrorCounter(servermon.DBQueryErrorCount, &err, op)

	db := d.DB.WithContext(ctx)

	db = db.Where("tag = ? AND type = ?", secret.Tag, secret.Type)

	if err := db.First(&secret).Error; err != nil {
		err = dbError(err)
		if errors.ErrorCode(err) == errors.CodeNotFound {
			return errors.Codef(errors.CodeNotFound, "secret not found")
		}
		return dbError(err)
	}
	return nil
}

// Delete secret deletes the secret with the specified type and tag.
func (d *Database) DeleteSecret(ctx context.Context, secret *dbmodel.Secret) (err error) {
	const op = "db.DeleteSecret"

	if secret.Tag == "" || secret.Type == "" {
		return errors.Codef(errors.CodeBadRequest, "missing secret tag and type")
	}

	if err := d.ready(); err != nil {
		return err
	}

	durationObserver := servermon.DurationObserver(servermon.DBQueryDurationHistogram, op)
	defer durationObserver()
	defer servermon.ErrorCounter(servermon.DBQueryErrorCount, &err, op)

	db := d.DB.WithContext(ctx)

	if err := db.Unscoped().Where("tag = ? AND type = ?", secret.Tag, secret.Type).Delete(&dbmodel.Secret{}).Error; err != nil {
		return dbError(err)
	}
	return nil
}

// Get retrieves the attributes for the given cloud credential from the DB.
func (d *Database) Get(ctx context.Context, tag names.CloudCredentialTag) (_ map[string]string, err error) {
	const op = "database.Get"

	if err := d.ready(); err != nil {
		return nil, err
	}

	durationObserver := servermon.DurationObserver(servermon.DBQueryDurationHistogram, op)
	defer durationObserver()
	defer servermon.ErrorCounter(servermon.DBQueryErrorCount, &err, op)

	secret := dbmodel.NewSecret(tag.Kind(), tag.String(), nil)
	err = d.GetSecret(ctx, &secret)
	if err != nil {
		if errors.ErrorCode(err) == errors.CodeNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get secret data: %w", err)
	}
	var attr map[string]string
	err = json.Unmarshal(secret.Data, &attr)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal secret data: %w", err)
	}
	return attr, nil
}

// Put stores the attributes associated with a cloud-credential in the DB.
func (d *Database) Put(ctx context.Context, tag names.CloudCredentialTag, attr map[string]string) (err error) {
	const op = "database.Put"

	if err := d.ready(); err != nil {
		return err
	}

	durationObserver := servermon.DurationObserver(servermon.DBQueryDurationHistogram, op)
	defer durationObserver()
	defer servermon.ErrorCounter(servermon.DBQueryErrorCount, &err, op)

	dataJson, err := json.Marshal(attr)
	if err != nil {
		return fmt.Errorf("failed to marshal secret data: %w", err)
	}
	secret := dbmodel.NewSecret(tag.Kind(), tag.String(), dataJson)
	return d.UpsertSecret(ctx, &secret)
}

// GetControllerCredentials retrieves the credentials for the given controller from the DB.
// It is expected for this interface that a non-existent controller credential return empty username/password.
func (d *Database) GetControllerCredentials(ctx context.Context, controllerName string) (_ string, _ string, err error) {
	const op = "database.GetControllerCredentials"

	if err := d.ready(); err != nil {
		return "", "", err
	}

	durationObserver := servermon.DurationObserver(servermon.DBQueryDurationHistogram, op)
	defer durationObserver()
	defer servermon.ErrorCounter(servermon.DBQueryErrorCount, &err, op)

	secret := dbmodel.NewSecret(names.ControllerTagKind, controllerName, nil)
	err = d.GetSecret(ctx, &secret)
	if errors.ErrorCode(err) == errors.CodeNotFound {
		return "", "", nil
	}
	if err != nil {
		return "", "", fmt.Errorf("failed to get secret data: %w", err)
	}
	var secretData map[string]string
	err = json.Unmarshal(secret.Data, &secretData)
	if err != nil {
		return "", "", fmt.Errorf("failed to unmarshal secret data: %w", err)
	}
	username, ok := secretData[usernameKey]
	if !ok {
		return "", "", errors.New("missing username")
	}
	password, ok := secretData[passwordKey]
	if !ok {
		return "", "", errors.New("missing password")
	}
	return username, password, nil
}

// PutControllerCredentials stores the controller credentials in the DB.
func (d *Database) PutControllerCredentials(ctx context.Context, controllerName string, username string, password string) (err error) {
	const op = "database.PutControllerCredentials"

	if err := d.ready(); err != nil {
		return err
	}

	durationObserver := servermon.DurationObserver(servermon.DBQueryDurationHistogram, op)
	defer durationObserver()
	defer servermon.ErrorCounter(servermon.DBQueryErrorCount, &err, op)

	secretData := make(map[string]string)
	secretData[usernameKey] = username
	secretData[passwordKey] = password
	dataJson, err := json.Marshal(secretData)
	if err != nil {
		return fmt.Errorf("failed to marshal secret data: %w", err)
	}
	secret := dbmodel.NewSecret(names.ControllerTagKind, controllerName, dataJson)
	return d.UpsertSecret(ctx, &secret)
}
