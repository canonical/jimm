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

const SELECT_APPLICATION_OFFERS = `
'application_offer' AS type, 
application_offers.uuid AS id, 
application_offers.name AS name, 
models.uuid AS parent_id,
models.name AS parent_name,
'model' AS parent_type
`

const SELECT_CLOUDS = `
'cloud' AS type, 
clouds.name AS id, 
clouds.name AS name, 
'' AS parent_id,
'' AS parent_name,
'' AS parent_type
`

const SELECT_CONTROLLERS = `
'controller' AS type, 
controllers.uuid AS id, 
controllers.name AS name, 
'' AS parent_id,
'' AS parent_name,
'' AS parent_type
`

const SELECT_MODELS = `
'model' AS type, 
models.uuid AS id, 
models.name AS name, 
controllers.uuid AS parent_id,
controllers.name AS parent_name,
'controller' AS parent_type
`

const SELECT_IDENTITIES = `
'service_account' AS type, 
identities.name AS id, 
identities.name AS name, 
'' AS parent_id,
'' AS parent_name,
'' AS parent_type
`

// RESOURCES_RAW_SQL contains the raw query fetching entities from multiple tables, with their respective entity parents.
const RESOURCES_RAW_SQL = `
(
	SELECT 'application_offer' AS type, 
			application_offers.uuid AS id, 
			application_offers.name AS name, 
			models.uuid AS parent_id,
			models.name AS parent_name,
			'model' AS parent_type
	FROM application_offers
	JOIN models ON application_offers.model_id = models.id
)
UNION
(
	SELECT 'cloud' AS type, 
			clouds.name AS id, 
			clouds.name AS name, 
			'' AS parent_id,
			'' AS parent_name,
			'' AS parent_type
	FROM clouds
)
UNION
(
	SELECT 'controller' AS type, 
		controllers.uuid AS id, 
		controllers.name AS name, 
		'' AS parent_id,
		'' AS parent_name,
		'' AS parent_type
		FROM controllers
)
UNION
(
	SELECT 'model' AS type, 
			models.uuid AS id, 
			models.name AS name, 
			controllers.uuid AS parent_id,
			controllers.name AS parent_name,
			'controller' AS parent_type
	FROM models
	JOIN controllers ON models.controller_id = controllers.id
)
UNION
(
	SELECT 'service_account' AS type, 
			identities.name AS id, 
			identities.name AS name, 
			'' AS parent_id,
			'' AS parent_name,
			'' AS parent_type
	FROM identities
	WHERE name LIKE '%@serviceaccount'
)
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
func (d *Database) ListResources(ctx context.Context, limit, offset int, nameFilter string) (_ []Resource, err error) {
	const op = errors.Op("db.ListResources")
	if err := d.ready(); err != nil {
		return nil, errors.E(op, err)
	}

	durationObserver := servermon.DurationObserver(servermon.DBQueryDurationHistogram, string(op))
	defer durationObserver()
	defer servermon.ErrorCounter(servermon.DBQueryErrorCount, &err, string(op))

	db := d.DB.WithContext(ctx)
	rows, err := buildQuery(db, offset, limit, nameFilter).Rows()
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

func buildQuery(db *gorm.DB, offset, limit int, nameFilter string) *gorm.DB {
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

	queries := []*gorm.DB{
		applicationOffersQuery,
		cloudsQuery,
		controllersQuery,
		modelsQuery,
		serviceAccountsQuery,
	}

	if nameFilter != "" {
		nameFilter = "%" + nameFilter + "%"
		for i := range queries {
			queries[i] = queries[i].Where("models.name LIKE ?", nameFilter)
		}
	}
	return db.
		Raw(query,
			applicationOffersQuery,
			cloudsQuery,
			controllersQuery,
			modelsQuery,
			serviceAccountsQuery,
			offset,
			limit,
		)
}
