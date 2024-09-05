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
const SELECT_APPLICATION_OFFERS = `
'application_offer' AS type, 
application_offers.uuid AS id, 
application_offers.name AS name, 
models.uuid AS parent_id,
models.name AS parent_name,
'model' AS parent_type
`

const CloudsQueryKey = "clouds"
const SELECT_CLOUDS = `
'cloud' AS type, 
clouds.name AS id, 
clouds.name AS name, 
'' AS parent_id,
'' AS parent_name,
'' AS parent_type
`

const ControllersQueryKey = "controllers"
const SELECT_CONTROLLERS = `
'controller' AS type, 
controllers.uuid AS id, 
controllers.name AS name, 
'' AS parent_id,
'' AS parent_name,
'' AS parent_type
`

const ModelsQueryKey = "models"
const SELECT_MODELS = `
'model' AS type, 
models.uuid AS id, 
models.name AS name, 
controllers.uuid AS parent_id,
controllers.name AS parent_name,
'controller' AS parent_type
`

const ServiceAccountQueryKey = "identities"
const SELECT_IDENTITIES = `
'service_account' AS type, 
identities.name AS id, 
identities.name AS name, 
'' AS parent_id,
'' AS parent_name,
'' AS parent_type
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
func (d *Database) ListResources(ctx context.Context, limit, offset int, nameFilter, typeFilter string) (_ []Resource, err error) {
	const op = errors.Op("db.ListResources")
	if err := d.ready(); err != nil {
		return nil, errors.E(op, err)
	}

	durationObserver := servermon.DurationObserver(servermon.DBQueryDurationHistogram, string(op))
	defer durationObserver()
	defer servermon.ErrorCounter(servermon.DBQueryErrorCount, &err, string(op))

	db := d.DB.WithContext(ctx)
	query, err := buildQuery(db, offset, limit, nameFilter, typeFilter)
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

// buildQuery is an utility function to build the database query according to two optional parameters.
// nameFilter: used to match resources name.
// typeFilter used to match resources type. If this is not empty the resources are fetched from a single table.
func buildQuery(db *gorm.DB, offset, limit int, nameFilter, typeFilter string) (*gorm.DB, error) {
	query := `
	? UNION ? UNION ? UNION ? UNION ?
	ORDER BY type, id
	OFFSET ?
	LIMIT  ?;
	`
	applicationOffersQuery := db.Select(SELECT_APPLICATION_OFFERS).
		Model(&dbmodel.ApplicationOffer{}).
		Joins("JOIN models ON application_offers.model_id = models.id")

	cloudsQuery := db.Select(SELECT_CLOUDS).
		Model(&dbmodel.Cloud{})

	controllersQuery := db.Select(SELECT_CONTROLLERS).
		Model(&dbmodel.Controller{})

	modelsQuery := db.Select(SELECT_MODELS).
		Model(&dbmodel.Model{}).
		Joins("JOIN controllers ON models.controller_id = controllers.id")

	serviceAccountsQuery := db.Select(SELECT_IDENTITIES).
		Model(&dbmodel.Identity{}).
		Where("name LIKE '%@serviceaccount'")

	queries := map[string]*gorm.DB{
		ApplicationOffersQueryKey: applicationOffersQuery,
		CloudsQueryKey:            cloudsQuery,
		ControllersQueryKey:       controllersQuery,
		ModelsQueryKey:            modelsQuery,
		ServiceAccountQueryKey:    serviceAccountsQuery,
	}
	// we add the where clause only if the nameFilter is filled
	if nameFilter != "" {
		nameFilter = "%" + nameFilter + "%"
		for k := range queries {
			queries[k] = queries[k].Where(k+".name LIKE ?", nameFilter)
		}
	}
	// if the typeFilter is set we only return the query for that specif entityType, otherwise the union.
	if typeFilter == "" {
		return db.
			Raw(query,
				applicationOffersQuery,
				cloudsQuery,
				controllersQuery,
				modelsQuery,
				serviceAccountsQuery,
				offset,
				limit,
			), nil
	} else {
		query, ok := queries[typeFilter]
		if !ok {
			// this shouldn't happen because we have validated the entityFilter at API layer
			return nil, errors.E("this entityType does not exist")
		}
		return query.Order("name").Offset(offset).Limit(limit), nil
	}

}
