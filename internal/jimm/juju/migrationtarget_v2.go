// Copyright 2026 Canonical.

package juju

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/juju/juju/core/permission"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v6"
	"gopkg.in/yaml.v3"

	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/openfga"
	ofganames "github.com/canonical/jimm/v3/internal/openfga/names"
)

// The methods in this file implement model migration into JIMM for Juju 4.1+
// source controllers. These send a SerializedModelV2 envelope (MigrationTarget
// facade v8) instead of a model description. The envelope carries the model's
// identity and controller scoped data (users, credential, permissions, keys) as
// typed fields, and the model database as an opaque, versioned payload.
//
// JIMM applies the user mapping to the typed fields only. The payload is passed
// to the target controller untouched. The target ctrl takes the model's identity
// from the typed fields rather than the payload.

// PrechecksV2 is the SerializedModelV2 equivalent of Prechecks.
// It checks the cloud, cloud region and cloud credential exist in JIMM, then
// calls the method of the same name on the target Juju controller.
func (j *JujuManager) PrechecksV2(ctx context.Context, user *openfga.User, envelope jujuparams.SerializedModelV2) error {
	incomingModel := dbmodel.IncomingModelMigration{
		ModelUUID: sql.NullString{
			String: envelope.ModelInfo.UUID,
			Valid:  true,
		},
	}
	err := j.Database.GetIncomingModelMigration(ctx, &incomingModel)
	if err != nil {
		return fmt.Errorf("failed to get model migration %q: %w", envelope.ModelInfo.UUID, err)
	}

	offers, err := offersFromPayload(envelope.Payload)
	if err != nil {
		return err
	}

	err = validateUserMappingV2(envelope, offers, incomingModel.UserMapping)
	if err != nil {
		return fmt.Errorf("failed to validate user mapping: %w", err)
	}

	envelope, err = modifyEnvelope(envelope, incomingModel.UserMapping)
	if err != nil {
		return fmt.Errorf("failed to modify migration info: %w", err)
	}

	info := envelope.ModelInfo
	_, err = j.Database.FindRegionByCloudName(ctx, info.Cloud, info.CloudRegion)
	if err != nil {
		return fmt.Errorf("failed to find region for cloud %q: %w", info.Cloud, err)
	}

	cloudCredential := &dbmodel.CloudCredential{
		CloudName:         info.Cloud,
		OwnerIdentityName: info.CredentialOwner,
		Name:              info.CredentialName,
	}
	err = j.Database.GetCloudCredential(ctx, cloudCredential)
	if err != nil {
		return err
	}

	api, err := j.dialControllerAsSuperuser(ctx, user, &incomingModel.TargetController)
	if err != nil {
		return fmt.Errorf("failed to dial controller: %w", err)
	}
	defer api.Close()

	err = api.PrechecksV2(ctx, envelope)
	if err != nil {
		return fmt.Errorf("failed to run pre-checks for migration: %w", err)
	}
	return nil
}

// ImportV2 is the SerializedModelV2 equivalent of Import.
//   - Checks the incoming model migration record in the database.
//   - Modifies the envelope to replace local user references with their external mapping.
//   - Imports the model and its application offers into JIMM's state.
//   - Adds permissions for the model and application offers.
//   - Calls the import method on the target Juju controller to import the model.
func (j *JujuManager) ImportV2(ctx context.Context, user *openfga.User, envelope jujuparams.SerializedModelV2) error {
	var (
		model             *dbmodel.Model
		dbOffers          []*dbmodel.ApplicationOffer
		incomingMigration *dbmodel.IncomingModelMigration
	)

	// Start a transaction to acquire the incoming model migration record with a
	// lock to prevent it from being modified while we are importing the model.
	// Then import the model and app offers into JIMM's state - the existence
	// of the model implies that the migration record can no longer be modified.
	err := j.Database.Transaction(func(d *db.Database) error {
		incomingMigration = &dbmodel.IncomingModelMigration{
			ModelUUID: sql.NullString{String: envelope.ModelInfo.UUID, Valid: true},
		}

		// Set noWait to false to allow the transaction to wait for the lock.
		noWait := false
		err := d.GetIncomingModelMigrationWithLock(ctx, incomingMigration, noWait)
		if err != nil {
			return fmt.Errorf("failed to get incoming model migration: %w", err)
		}

		envelope, err = modifyEnvelope(envelope, incomingMigration.UserMapping)
		if err != nil {
			return fmt.Errorf("failed to modify migration info: %w", err)
		}

		model, dbOffers, err = importFromEnvelope(ctx, d, incomingMigration.TargetController.ID, envelope)
		if err != nil {
			return fmt.Errorf("failed to import model from envelope: %w", err)
		}
		return nil
	})
	if err != nil {
		return err
	}

	// Pass the controller tag as the controller details
	// are not populated on the model after creation.
	controllerTag := incomingMigration.TargetController.ResourceTag()
	err = j.addModelAndOfferPermissions(ctx, user, model, dbOffers, controllerTag)
	if err != nil {
		return fmt.Errorf("failed to add resource permissions: %w", err)
	}

	api, err := j.dialControllerAsSuperuser(ctx, user, &incomingMigration.TargetController)
	if err != nil {
		return fmt.Errorf("failed to dial controller: %w", err)
	}
	defer api.Close()

	err = api.ImportV2(ctx, envelope)
	if err != nil {
		// TODO: handle migration failure in a cleanup routine.
		return fmt.Errorf("failed to import model: %w", err)
	}
	return nil
}

// importFromEnvelope is the SerializedModelV2 equivalent of importFromDescription.
// It imports resources into JIMM's state from a model envelope that has already had
// the user mapping applied. It creates a new model record in the database with the
// given target controller ID and sets the migration mode to importing.
// Application offers are created for any offers in the envelope's payload.
// It also ensures that the cloud credential and region are present in the database.
func importFromEnvelope(ctx context.Context, tx *db.Database, targetControllerID uint, envelope jujuparams.SerializedModelV2) (*dbmodel.Model, []*dbmodel.ApplicationOffer, error) {
	offers, err := offersFromPayload(envelope.Payload)
	if err != nil {
		return nil, nil, err
	}

	info := envelope.ModelInfo
	return addImportingModel(ctx, tx, importingModel{
		UUID:               info.UUID,
		Name:               info.Name,
		Owner:              info.Qualifier,
		TargetControllerID: targetControllerID,
		Cloud:              info.Cloud,
		CloudRegion:        info.CloudRegion,
		CredentialOwner:    info.CredentialOwner,
		CredentialName:     info.CredentialName,
		Offers:             offers,
	})
}

// offersFromPayload extracts the application offers from an envelope's
// model database payload. Only the "offer" table is decoded so that JIMM
// does not depend on Juju's versioned model database schema.
func offersFromPayload(payload []byte) ([]importingOffer, error) {
	var p struct {
		Offer []struct {
			UUID string `yaml:"uuid"`
			Name string `yaml:"name"`
		} `yaml:"offer"`
	}
	if err := yaml.Unmarshal(payload, &p); err != nil {
		return nil, fmt.Errorf("failed to read offers from model payload: %w", err)
	}
	offers := make([]importingOffer, 0, len(p.Offer))
	for _, o := range p.Offer {
		offers = append(offers, importingOffer{UUID: o.UUID, Name: o.Name})
	}
	return offers, nil
}

// validateUserMappingV2 checks that the provided user mapping contains all the users
// that either have access to the model or have access to any application offers in the model.
func validateUserMappingV2(envelope jujuparams.SerializedModelV2, offers []importingOffer, userMapping dbmodel.StringMap) error {
	offerNames := make(map[string]string, len(offers))
	for _, o := range offers {
		offerNames[o.UUID] = o.Name
	}

	var missingUserMessages []string
	for _, p := range envelope.Permissions {
		if p.SubjectName == ofganames.EveryoneUser {
			continue
		}
		if _, ok := userMapping[p.SubjectName]; ok {
			continue
		}
		switch permission.ObjectType(p.ObjectType) {
		case permission.Model:
			missingUserMessages = append(missingUserMessages, fmt.Sprintf("expected user %q who has %s access to the model", p.SubjectName, p.Access))
		case permission.Offer:
			missingUserMessages = append(missingUserMessages, fmt.Sprintf("expected user %q who has %s access to offer %q", p.SubjectName, p.Access, offerNames[p.GrantOn]))
		}
	}
	if len(missingUserMessages) > 0 {
		return fmt.Errorf("user mapping is missing the following users:\n%s", strings.Join(missingUserMessages, "\n"))
	}
	return nil
}

// modifyEnvelope is the SerializedModelV2 equivalent of modifyModelDescription.
// It replaces local user references with their external mapping:
//   - the model qualifier (the model owner in Juju 4),
//   - the cloud credential owner,
//   - users with access to application offers,
//   - the owners of authorized SSH keys.
//
// Model permissions are dropped, as access to the model is managed by JIMM.
//
// A Juju 4.1 target only accepts permissions and keys for users it knows, and it
// learns users from the envelope's user list. So the user list is replaced with
// an entry for every user referenced after mapping.
//
// The returned envelope shares no modified slices with the given envelope.
func modifyEnvelope(envelope jujuparams.SerializedModelV2, userMapping dbmodel.StringMap) (jujuparams.SerializedModelV2, error) {
	owner, ok := mapUser(envelope.ModelInfo.Qualifier, userMapping)
	if !ok {
		return jujuparams.SerializedModelV2{}, fmt.Errorf("no external user mapping found for local user %q", envelope.ModelInfo.Qualifier)
	}
	envelope.ModelInfo.Qualifier = owner

	if envelope.ModelCredential == nil {
		return jujuparams.SerializedModelV2{}, fmt.Errorf("model must have a cloud credential")
	}
	credentialOwner, ok := mapUser(envelope.ModelCredential.Owner, userMapping)
	if !ok {
		return jujuparams.SerializedModelV2{}, fmt.Errorf("no external user mapping found for cloud credential local user %q", envelope.ModelCredential.Owner)
	}
	credential := *envelope.ModelCredential
	credential.Owner = credentialOwner
	envelope.ModelCredential = &credential
	envelope.ModelInfo.CredentialOwner = credentialOwner

	users := newModelUserList(envelope.Users)
	users.add(owner)
	users.add(credentialOwner)

	var permissions []jujuparams.ModelPermission
	for _, p := range envelope.Permissions {
		switch permission.ObjectType(p.ObjectType) {
		case permission.Model:
			// Access to the model is managed by JIMM.
			continue
		case permission.Offer:
			if p.SubjectName != ofganames.EveryoneUser {
				subject, ok := mapUser(p.SubjectName, userMapping)
				if !ok {
					// The local user was intentionally not mapped.
					continue
				}
				p.SubjectName = subject
				users.add(subject)
			}
		}
		permissions = append(permissions, p)
	}
	envelope.Permissions = permissions

	var keys []jujuparams.ModelAuthorizedKey
	for _, k := range envelope.AuthorizedKeys {
		keyOwner, ok := mapUser(k.Username, userMapping)
		if !ok {
			// Keep keys belonging to unmapped users usable by
			// attributing them to the model owner.
			keyOwner = owner
		}
		k.Username = keyOwner
		users.add(keyOwner)
		keys = append(keys, k)
	}
	envelope.AuthorizedKeys = keys

	envelope.Users = users.list
	return envelope, nil
}

// mapUser returns the external user that a local user is mapped to.
// Non-local users are returned unchanged. It returns false if a local user
// has no valid mapping, including when it was intentionally left unmapped.
func mapUser(name string, userMapping dbmodel.StringMap) (string, bool) {
	if !names.IsValidUser(name) || !names.NewUserTag(name).IsLocal() {
		return name, true
	}
	mapped := userMapping[name]
	if mapped == "" || !names.IsValidUser(mapped) {
		return "", false
	}
	return mapped, true
}

// modelUserList builds the user list of an envelope, reusing the source's
// entry for a user when one exists.
type modelUserList struct {
	original map[string]jujuparams.ModelUser
	seen     map[string]bool
	list     []jujuparams.ModelUser
}

func newModelUserList(original []jujuparams.ModelUser) *modelUserList {
	l := &modelUserList{
		original: make(map[string]jujuparams.ModelUser, len(original)),
		seen:     make(map[string]bool),
	}
	for _, u := range original {
		l.original[u.Name] = u
	}
	return l
}

func (l *modelUserList) add(name string) {
	if l.seen[name] {
		return
	}
	l.seen[name] = true
	if u, ok := l.original[name]; ok {
		l.list = append(l.list, u)
		return
	}
	l.list = append(l.list, jujuparams.ModelUser{
		Name:      name,
		CreatedAt: time.Now().UTC(),
		External:  names.IsValidUser(name) && !names.NewUserTag(name).IsLocal(),
	})
}
