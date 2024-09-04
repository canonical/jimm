// Copyright 2024 Canonical.
package db

import (
	"context"
	"database/sql"

	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/servermon"
)

// MULTI_TABLES_RAW_SQL contains the raw query fetching entities from multiple tables, with their respective entity parents.
const MULTI_TABLES_RAW_SQL = `
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
func (d *Database) ListResources(ctx context.Context, limit, offset int) (_ []Resource, err error) {
	const op = errors.Op("db.GetMultipleModels")
	if err := d.ready(); err != nil {
		return nil, errors.E(op, err)
	}

	durationObserver := servermon.DurationObserver(servermon.DBQueryDurationHistogram, string(op))
	defer durationObserver()
	defer servermon.ErrorCounter(servermon.DBQueryErrorCount, &err, string(op))

	db := d.DB.WithContext(ctx)
	rows, err := db.Raw(MULTI_TABLES_RAW_SQL, offset, limit).Rows()
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
