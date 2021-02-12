// Copyright 2020 Canonical Ltd.

package db

import (
	"context"

	"gorm.io/gorm"

	"github.com/CanonicalLtd/jimm/internal/dbmodel"
	"github.com/CanonicalLtd/jimm/internal/errors"
)

// AddApplicationOffer stores the application offer information.
func (d *Database) AddApplicationOffer(ctx context.Context, offer *dbmodel.ApplicationOffer) error {
	const op = errors.Op("db.AddApplicationOffer")
	if err := d.ready(); err != nil {
		return errors.E(op, err)
	}
	db := d.DB.WithContext(ctx)

	result := db.Create(offer)
	if result.Error != nil {
		return errors.E(op, dbError(result.Error))
	}
	return nil
}

// UpdateApplicationOffer updates the application offer information.
func (d *Database) UpdateApplicationOffer(ctx context.Context, offer *dbmodel.ApplicationOffer) error {
	const op = errors.Op("db.UpdateApplicationOffer")
	if err := d.ready(); err != nil {
		return errors.E(op, err)
	}
	db := d.DB.WithContext(ctx)

	result := db.Session(&gorm.Session{FullSaveAssociations: true}).Save(offer)
	if result.Error != nil {
		return errors.E(op, dbError(result.Error))
	}

	if err := d.GetApplicationOffer(ctx, offer); err != nil {
		return err
	}

	return nil
}

// GetApplicationOffer returns application offer information based on the
// offer UUID.
func (d *Database) GetApplicationOffer(ctx context.Context, offer *dbmodel.ApplicationOffer) error {
	const op = errors.Op("db.GetApplicationOffer")
	if err := d.ready(); err != nil {
		return errors.E(op, err)
	}
	db := d.DB.WithContext(ctx)

	if offer.UUID != "" {
		db = db.Where("uuid = ?", offer.UUID)
	} else if offer.URL != "" {
		db = db.Where("url = ?", offer.URL)
	} else {
		return errors.E(op, "missing offer UUID or URL")
	}
	result := db.Preload("Application").Preload("Application.Model").Preload("Users").Preload("Users.User").Preload("Endpoints").Preload("Spaces").Preload("Connections").First(&offer)
	if result.Error != nil {
		return errors.E(op, dbError(result.Error))
	}
	return nil
}

// DeleteApplicationOffer deletes the application offer.
func (d *Database) DeleteApplicationOffer(ctx context.Context, offer *dbmodel.ApplicationOffer) error {
	const op = errors.Op("db.DeleteApplicationOffer")
	if err := d.ready(); err != nil {
		return errors.E(op, err)
	}
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
		return db.Joins("JOIN applications AS a1 ON a1.id = offers.application_id").Joins("JOIN models ON models.id = a1.model_id").Where("models.name = ?", modelName)
	}
}

// ApplicationOfferFilterByApplication filters application offers by application name.
func ApplicationOfferFilterByApplication(applicationName string) ApplicationOfferFilter {
	return func(db *gorm.DB) *gorm.DB {
		return db.Joins("JOIN applications AS a2 ON a2.id = offers.application_id").Where("a2.name = ?", applicationName)
	}
}

// ApplicationOfferFilterByUser filters application offer accessible by the user.
func ApplicationOfferFilterByUser(userID uint) ApplicationOfferFilter {
	// TODO (ashipika) allow offers to which everyone@external has access.
	return func(db *gorm.DB) *gorm.DB {
		return db.Joins("INNER JOIN user_application_offer_accesses AS users ON users.application_offer_id = offers.id AND users.user_id = ? AND users.access IN ?", userID, []string{"read", "consume", "admin"})
	}
}

// ApplicationOfferFilterByConsumer filters application offer accessible by the user.
func ApplicationOfferFilterByConsumer(userID uint) ApplicationOfferFilter {
	return func(db *gorm.DB) *gorm.DB {
		return db.Joins("INNER JOIN user_application_offer_accesses AS consumers ON consumers.application_offer_id = offers.id AND consumers.user_id = ? AND consumers.access IN ?", userID, []string{"consume", "admin"})
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
func (d *Database) FindApplicationOffers(ctx context.Context, filters ...ApplicationOfferFilter) ([]dbmodel.ApplicationOffer, error) {
	const op = errors.Op("db.FindApplicationOffer")
	if len(filters) == 0 {
		return nil, errors.E(op, errors.CodeBadRequest, "no filters specified")
	}
	if err := d.ready(); err != nil {
		return nil, errors.E(op, err)
	}
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
