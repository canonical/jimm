// Copyright 2024 Canonical.
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

const ServiceAccountQueryKey = "identities"
const selectIdentities = `
'service_account' AS type, 
identities.name AS id, 
identities.name AS name, 
'' AS parent_id,
'' AS parent_name,
'' AS parent_type
`

const unionQuery = `
? UNION ? UNION ? UNION ? UNION ?
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

// ListResources returns a list of models, clouds, controllers, service accounts, and application offers, with its respective parents.
// It has been implemented with a raw query because this is a specific implementation for the ReBAC Admin UI.
func (d *Database) ListResources(ctx context.Context, limit, offset int, namePrefixFilter, typeFilter string) (_ []Resource, err error) {
	const op = errors.Op("db.ListResources")
	if err := d.ready(); err != nil {
		return nil, errors.E(op, err)
	}

	durationObserver := servermon.DurationObserver(servermon.DBQueryDurationHistogram, string(op))
	defer durationObserver()
	defer servermon.ErrorCounter(servermon.DBQueryErrorCount, &err, string(op))

	db := d.DB.WithContext(ctx)
	query, err := buildQuery(db, offset, limit, namePrefixFilter, typeFilter)
	if err != nil {
		return nil, err
	}
	rows, err := query.Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	resources := make([]Resource, 0)
	for rows.Next() {
		var res Resource
		err := db.ScanRows(rows, &res)
		if err != nil {
			return nil, err
		}
		resources = append(resources, res)
	}
	return resources, nil
}

// buildQuery is a utility function to build the database query according to two optional parameters.
// namePrefixFilter: used to match resources name prefix.
// typeFilter: used to match resources type. If this is not empty the resources are fetched from a single table.
func buildQuery(db *gorm.DB, offset, limit int, namePrefixFilter, typeFilter string) (*gorm.DB, error) {
	// if namePrefixEmpty set to true we have a 'TRUE IS TRUE' in the SQL WHERE statement, which disable the filtering.
	namePrefixEmpty := namePrefixFilter == ""
	namePrefixFilter += "%"

	applicationOffersQuery := db.Select(selectApplicationOffers).
		Model(&dbmodel.ApplicationOffer{}).
		Where("? IS TRUE OR application_offers.name LIKE ?", namePrefixEmpty, namePrefixFilter).
		Joins("JOIN models ON application_offers.model_id = models.id")

	cloudsQuery := db.Select(selectClouds).
		Model(&dbmodel.Cloud{}).
		Where("? IS TRUE OR clouds.name LIKE ?", namePrefixEmpty, namePrefixFilter)

	controllersQuery := db.Select(selectControllers).
		Model(&dbmodel.Controller{}).
		Where("? IS TRUE OR controllers.name LIKE ?", namePrefixEmpty, namePrefixFilter)

	modelsQuery := db.Select(selectModels).
		Model(&dbmodel.Model{}).
		Where("? IS TRUE OR models.name LIKE ?", namePrefixEmpty, namePrefixFilter).
		Joins("JOIN controllers ON models.controller_id = controllers.id")

	serviceAccountsQuery := db.Select(selectIdentities).
		Model(&dbmodel.Identity{}).
		Where("name LIKE '%@serviceaccount' AND (? IS TRUE OR identities.name LIKE ?)", namePrefixEmpty, namePrefixFilter)

	// if the typeFilter is set we only return the query for that specif entityType, otherwise the union.
	if typeFilter == "" {
		return db.
			Raw(unionQuery,
				applicationOffersQuery,
				cloudsQuery,
				controllersQuery,
				modelsQuery,
				serviceAccountsQuery,
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
	case ServiceAccountQueryKey:
		query = serviceAccountsQuery
	default:
		// this shouldn't happen because we have validated the entityFilter at API layer
		return nil, errors.E("this entityType does not exist")
	}
	return query.Order("id").Offset(offset).Limit(limit), nil
}
