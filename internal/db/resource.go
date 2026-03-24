// Copyright 2025 Canonical.

package db

import (
	"context"
	"database/sql"

	"gorm.io/gorm"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/servermon"
)

const ApplicationOffersQueryKey = "application_offers"
const selectApplicationOffers = `
'application_offer' AS type, 
application_offers.uuid AS id, 
application_offers.name AS name, 
models.uuid AS parent_id,
models.name AS parent_name,
'model' AS parent_type
`

const CloudsQueryKey = "clouds"
const selectClouds = `
'cloud' AS type, 
clouds.name AS id, 
clouds.name AS name, 
'' AS parent_id,
'' AS parent_name,
'' AS parent_type
`

const ControllersQueryKey = "controllers"
const selectControllers = `
'controller' AS type, 
controllers.uuid AS id, 
controllers.name AS name, 
'' AS parent_id,
'' AS parent_name,
'' AS parent_type
`

const ModelsQueryKey = "models"
const selectModels = ` 
'model' AS type, 
models.uuid AS id, 
models.name AS name, 
controllers.uuid AS parent_id,
controllers.name AS parent_name,
'controller' AS parent_type
`

const unionQuery = `
? UNION ? UNION ? UNION ?
ORDER BY type, id
OFFSET ?
LIMIT  ?;
`

type Resource struct {
	Type       string
	ID         sql.NullString
	Name       string
	ParentId   sql.NullString
	ParentName string
	ParentType string
}

// ListResources returns a list of models, clouds, controllers, and application offers, with its respective parents.
// It has been implemented with a raw query because this is a specific implementation for the ReBAC Admin UI.
func (d *Database) ListResources(ctx context.Context, limit, offset int, namePrefixFilter, typeFilter string) (_ []Resource, err error) {
	const op = "db.ListResources"
	if err := d.ready(); err != nil {
		return nil, err
	}

	durationObserver := servermon.DurationObserver(servermon.DBQueryDurationHistogram, op)
	defer durationObserver()
	defer servermon.ErrorCounter(servermon.DBQueryErrorCount, &err, op)

	db := d.DB.WithContext(ctx)
	query, err := buildQuery(db, offset, limit, namePrefixFilter, typeFilter)
	if err != nil {
		return nil, err
	}

	var resources []Resource
	if err := query.Find(&resources).Error; err != nil {
		return nil, dbError(err)
	}
	return resources, nil
}

// buildQuery is a utility function to build the database query according to two optional parameters.
// namePrefixFilter: used to match resources name prefix.
// typeFilter: used to match resources type. If this is not empty the resources are fetched from a single table.
func buildQuery(db *gorm.DB, offset, limit int, namePrefixFilter, typeFilter string) (*gorm.DB, error) {
	applicationOffersQuery := db.Select(selectApplicationOffers).
		Model(&dbmodel.ApplicationOffer{}).
		Where("(CASE WHEN ? = '' THEN TRUE ELSE application_offers.name LIKE ? END)", namePrefixFilter, namePrefixFilter+"%").
		Joins("JOIN models ON application_offers.model_id = models.id")

	cloudsQuery := db.Select(selectClouds).
		Model(&dbmodel.Cloud{}).
		Where("(CASE WHEN ? = '' THEN TRUE ELSE clouds.name LIKE ? END)", namePrefixFilter, namePrefixFilter+"%")

	controllersQuery := db.Select(selectControllers).
		Model(&dbmodel.Controller{}).
		Where("(CASE WHEN ? = '' THEN TRUE ELSE controllers.name LIKE ? END)", namePrefixFilter, namePrefixFilter+"%")

	modelsQuery := db.Select(selectModels).
		Model(&dbmodel.Model{}).
		Where("(CASE WHEN ? = '' THEN TRUE ELSE models.name LIKE ? END)", namePrefixFilter, namePrefixFilter+"%").
		Joins("JOIN controllers ON models.controller_id = controllers.id")

	// if the typeFilter is set we only return the query for that specif entityType, otherwise the union.
	if typeFilter == "" {
		return db.
			Raw(unionQuery,
				applicationOffersQuery,
				cloudsQuery,
				controllersQuery,
				modelsQuery,
				offset,
				limit,
			), nil
	}
	var query *gorm.DB
	switch typeFilter {
	case ControllersQueryKey:
		query = controllersQuery
	case CloudsQueryKey:
		query = cloudsQuery
	case ApplicationOffersQueryKey:
		query = applicationOffersQuery
	case ModelsQueryKey:
		query = modelsQuery
	default:
		// this shouldn't happen because we have validated the entityFilter at API layer
		return nil, errors.New("this entityType does not exist")
	}
	return query.Order("id").Offset(offset).Limit(limit), nil
}
