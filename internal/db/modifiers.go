// Copyright 2025 Canonical.

package db

import "gorm.io/gorm/clause"

// ForUpdate returns a copy of the database with a "FOR UPDATE" clause
// applied to subsequent queries. This is useful for ensuring that rows
// are locked for updates in a transaction.
// Exampe usage:
//
//		s.Database.Transaction(func(tx *db.Database) error {
//					model := dbmodel.Model{}
//					model.SetTag(mt)
//					err = tx.ForUpdate().GetModel(ctx, &model)
//					// Do some checks with the model
//	                err = tx.UpdateModel(ctx, &model)
//					return nil
//			})
func (d *Database) ForUpdate() *Database {
	dCopy := *d
	dCopy.DB = dCopy.DB.Clauses(clause.Locking{Strength: "UPDATE"})
	return &dCopy
}
