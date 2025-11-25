// Copyright 2025 Canonical.

package db

import (
	"context"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/servermon"
)

// AddApplicationOffer stores the application offer information.
func (d *Database) AddApplicationOffer(ctx context.Context, offer *dbmodel.ApplicationOffer) (err error) {
	const op = "db.AddApplicationOffer"

	if err := d.ready(); err != nil {
		return errors.E(err)
	}

	durationObserver := servermon.DurationObserver(servermon.DBQueryDurationHistogram, op)
	defer durationObserver()
	defer servermon.ErrorCounter(servermon.DBQueryErrorCount, &err, op)

	db := d.DB.WithContext(ctx)

	result := db.Create(offer)
	if result.Error != nil {
		return errors.E(dbError(result.Error))
	}
	return nil
}

// GetApplicationOffer returns application offer information based on the
// offer UUID or URL.
func (d *Database) GetApplicationOffer(ctx context.Context, offer *dbmodel.ApplicationOffer) (err error) {
	const op = "db.GetApplicationOffer"

	if err := d.ready(); err != nil {
		return errors.E(err)
	}

	durationObserver := servermon.DurationObserver(servermon.DBQueryDurationHistogram, op)
	defer durationObserver()
	defer servermon.ErrorCounter(servermon.DBQueryErrorCount, &err, op)
	db := d.DB.WithContext(ctx)

	switch {
	case offer.UUID != "":
		db = db.Where("uuid = ?", offer.UUID)
	case offer.URL != "":
		db = db.Where("url = ?", offer.URL)
	default:
		return errors.E("missing offer UUID or URL")
	}

	db = db.Preload("Model").Preload("Model.Controller")
	if err := db.First(&offer).Error; err != nil {
		err := dbError(err)
		if errors.ErrorCode(err) == errors.CodeNotFound {
			return errors.E(err, "application offer not found")
		}
		return errors.E(err)
	}
	return nil
}

// DeleteApplicationOffer deletes the application offer.
func (d *Database) DeleteApplicationOffer(ctx context.Context, offer *dbmodel.ApplicationOffer) (err error) {
	const op = "db.DeleteApplicationOffer"

	if err := d.ready(); err != nil {
		return errors.E(err)
	}

	durationObserver := servermon.DurationObserver(servermon.DBQueryDurationHistogram, op)
	defer durationObserver()
	defer servermon.ErrorCounter(servermon.DBQueryErrorCount, &err, op)

	db := d.DB.WithContext(ctx)

	result := db.Delete(offer)
	if result.Error != nil {
		return errors.E(dbError(result.Error))
	}
	return nil
}

// FindApplicationOffersByModel returns all application offers in a model specified by model name and owner.
func (d *Database) FindApplicationOffersByModel(ctx context.Context, modelName, modelOwner string) (_ []dbmodel.ApplicationOffer, err error) {
	const op = "db.FindApplicationOfferByModel"

	if modelName == "" || modelOwner == "" {
		return nil, errors.E(errors.CodeBadRequest, "model name or owner not specified")
	}
	if err := d.ready(); err != nil {
		return nil, errors.E(err)
	}

	durationObserver := servermon.DurationObserver(servermon.DBQueryDurationHistogram, op)
	defer durationObserver()
	defer servermon.ErrorCounter(servermon.DBQueryErrorCount, &err, op)

	db := d.DB.WithContext(ctx)
	db = db.Table("application_offers AS offers")

	db = db.Joins("JOIN models ON models.id = offers.model_id").
		Where("models.name = ?", modelName).
		Where("models.owner_identity_name = ?", modelOwner)

	var offers []dbmodel.ApplicationOffer
	result := db.Preload("Model").Find(&offers)
	if result.Error != nil {
		return nil, errors.E(dbError(result.Error))
	}

	for i, offer := range offers {
		offer := offer
		err := d.GetApplicationOffer(ctx, &offer)
		if err != nil {
			return nil, errors.E(dbError(err))
		}
		offers[i] = offer
	}

	return offers, nil
}
