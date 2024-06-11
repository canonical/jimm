// Copyright 2020 Canonical Ltd.

package db

import (
	"context"

	"gorm.io/gorm"

	"github.com/canonical/jimm/internal/dbmodel"
	"github.com/canonical/jimm/internal/errors"
	"github.com/canonical/jimm/internal/servermon"
)

// AddApplicationOffer stores the application offer information.
func (d *Database) AddApplicationOffer(ctx context.Context, offer *dbmodel.ApplicationOffer) (err error) {
	const op = errors.Op("db.AddApplicationOffer")

	if err := d.ready(); err != nil {
		return errors.E(op, err)
	}

	durationObserver := servermon.DurationObserver(servermon.DBQueryDurationHistogram, string(op))
	defer durationObserver()
	defer servermon.ErrorCounter(servermon.DBQueryErrorCount, &err, string(op))

	db := d.DB.WithContext(ctx)

	result := db.Create(offer)
	if result.Error != nil {
		return errors.E(op, dbError(result.Error))
	}
	return nil
}

// UpdateApplicationOffer updates the application offer information.
func (d *Database) UpdateApplicationOffer(ctx context.Context, offer *dbmodel.ApplicationOffer) (err error) {
	const op = errors.Op("db.UpdateApplicationOffer")

	if err := d.ready(); err != nil {
		return errors.E(op, err)
	}

	durationObserver := servermon.DurationObserver(servermon.DBQueryDurationHistogram, string(op))
	defer durationObserver()
	defer servermon.ErrorCounter(servermon.DBQueryErrorCount, &err, string(op))

	db := d.DB.WithContext(ctx)
	err = db.Transaction(func(tx *gorm.DB) error {
		tx.Omit("Connections", "Endpoints", "Spaces").Save(offer)
		tx.Model(offer).Association("Connections").Replace(offer.Connections)
		tx.Model(offer).Association("Endpoints").Replace(offer.Endpoints)
		tx.Model(offer).Association("Spaces").Replace(offer.Spaces)
		return tx.Error
	})
	if err != nil {
		return errors.E(op, dbError(err))
	}

	if err := d.GetApplicationOffer(ctx, offer); err != nil {
		return err
	}

	return nil
}

// GetApplicationOffer returns application offer information based on the
// offer UUID or URL.
func (d *Database) GetApplicationOffer(ctx context.Context, offer *dbmodel.ApplicationOffer) (err error) {
	const op = errors.Op("db.GetApplicationOffer")

	if err := d.ready(); err != nil {
		return errors.E(op, err)
	}

	durationObserver := servermon.DurationObserver(servermon.DBQueryDurationHistogram, string(op))
	defer durationObserver()
	defer servermon.ErrorCounter(servermon.DBQueryErrorCount, &err, string(op))
	db := d.DB.WithContext(ctx)

	if offer.UUID != "" {
		db = db.Where("uuid = ?", offer.UUID)
	} else if offer.URL != "" {
		db = db.Where("url = ?", offer.URL)
	} else {
		return errors.E(op, "missing offer UUID or URL")
	}
	db = db.Preload("Connections")
	db = db.Preload("Endpoints")
	db = db.Preload("Model").Preload("Model.Controller")
	db = db.Preload("Spaces")
	if err := db.First(&offer).Error; err != nil {
		err := dbError(err)
		if errors.ErrorCode(err) == errors.CodeNotFound {
			return errors.E(op, err, "application offer not found")
		}
		return errors.E(op, err)
	}
	return nil
}

// DeleteApplicationOffer deletes the application offer.
func (d *Database) DeleteApplicationOffer(ctx context.Context, offer *dbmodel.ApplicationOffer) (err error) {
	const op = errors.Op("db.DeleteApplicationOffer")

	if err := d.ready(); err != nil {
		return errors.E(op, err)
	}

	durationObserver := servermon.DurationObserver(servermon.DBQueryDurationHistogram, string(op))
	defer durationObserver()
	defer servermon.ErrorCounter(servermon.DBQueryErrorCount, &err, string(op))

	db := d.DB.WithContext(ctx)

	result := db.Delete(offer)
	if result.Error != nil {
		return errors.E(op, dbError(result.Error))
	}
	return nil
}

// ApplicationOfferFilter can be used to find application offers that match certain criteria.
type ApplicationOfferFilter func(*gorm.DB) *gorm.DB

// ApplicationOfferFilterByName filters application offers by the offer name.
func ApplicationOfferFilterByName(name string) ApplicationOfferFilter {
	return func(db *gorm.DB) *gorm.DB {
		return db.Where("offers.name LIKE ?", "%"+name+"%")
	}
}

// ApplicationOfferFilterByDescription filters application offers by application description.
func ApplicationOfferFilterByDescription(substring string) ApplicationOfferFilter {
	return func(db *gorm.DB) *gorm.DB {
		return db.Where("offers.application_description LIKE ?", "%"+substring+"%")
	}
}

// ApplicationOfferFilterByModel filters application offers by model name.
func ApplicationOfferFilterByModel(modelName string) ApplicationOfferFilter {
	return func(db *gorm.DB) *gorm.DB {
		return db.Joins("JOIN models ON models.id = offers.model_id").Where("models.name = ?", modelName)
	}
}

// ApplicationOfferFilterByApplication filters application offers by application name.
func ApplicationOfferFilterByApplication(applicationName string) ApplicationOfferFilter {
	return func(db *gorm.DB) *gorm.DB {
		return db.Where("offers.application_name = ?", applicationName)
	}
}

// ApplicationOfferFilterByUUID filters application offers by UUID.
func ApplicationOfferFilterByUUID(uuids []string) ApplicationOfferFilter {
	return func(db *gorm.DB) *gorm.DB {
		db = db.Where("offers.uuid IN ?", uuids)
		return db
	}
}

// ApplicationOfferFilterByEndpoint filters application offer accessible by the user.
func ApplicationOfferFilterByEndpoint(endpoint dbmodel.ApplicationOfferRemoteEndpoint) ApplicationOfferFilter {
	return func(db *gorm.DB) *gorm.DB {
		db = db.Joins("JOIN application_offer_remote_endpoints AS endpoints ON endpoints.application_offer_id = offers.id")
		if endpoint.Interface != "" {
			db = db.Where("endpoints.interface = ?", endpoint.Interface)
		}
		if endpoint.Name != "" {
			db = db.Where("endpoints.name = ?", endpoint.Name)
		}
		if endpoint.Role != "" {
			db = db.Where("endpoints.role = ?", endpoint.Role)
		}
		return db
	}
}

// FindApplicationOffers returns application offers matching criteria specified by the filters.
func (d *Database) FindApplicationOffers(ctx context.Context, filters ...ApplicationOfferFilter) (_ []dbmodel.ApplicationOffer, err error) {
	const op = errors.Op("db.FindApplicationOffer")

	if len(filters) == 0 {
		return nil, errors.E(op, errors.CodeBadRequest, "no filters specified")
	}
	if err := d.ready(); err != nil {
		return nil, errors.E(op, err)
	}

	durationObserver := servermon.DurationObserver(servermon.DBQueryDurationHistogram, string(op))
	defer durationObserver()
	defer servermon.ErrorCounter(servermon.DBQueryErrorCount, &err, string(op))

	db := d.DB.WithContext(ctx)
	db = db.Table("application_offers AS offers")

	for _, filter := range filters {
		db = filter(db)
	}

	var offers []dbmodel.ApplicationOffer
	result := db.Find(&offers)
	if result.Error != nil {
		return nil, errors.E(op, dbError(result.Error))
	}

	for i, offer := range offers {
		offer := offer
		err := d.GetApplicationOffer(ctx, &offer)
		if err != nil {
			return nil, errors.E(op, dbError(err))
		}
		offers[i] = offer
	}

	return offers, nil
}
