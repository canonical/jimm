// Copyright 2016 Canonical Ltd.

package jimmdb

import (
	"context"

	"github.com/CanonicalLtd/jimm/internal/mongodoc"
	"github.com/CanonicalLtd/jimm/params"
	"gopkg.in/errgo.v1"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

// AddModel adds a new model to the database.
// It returns an error with a params.ErrAlreadyExists
// cause if there is already an model with the given name.
// If ignores m.Id and sets it from m.Path.
func (db *Database) AddModel(ctx context.Context, m *mongodoc.Model) (err error) {
	defer db.checkError(ctx, &err)
	m.Id = m.Path.String()
	err = db.Models().Insert(m)
	if mgo.IsDup(err) {
		return errgo.WithCausef(nil, params.ErrAlreadyExists, "")
	}
	if err != nil {
		return errgo.Notef(err, "cannot insert controller model")
	}
	return nil
}

// GetModel completes the contents of the given model. The database model
// is matched using the first non-zero value in the given model from the
// following fields:
//
//  - Path
//  - Controller & UUID
//  - UUID
//
// If no matching model can be found then the returned error will have a
// cause of params.ErrNotFound.
func (db *Database) GetModel(ctx context.Context, m *mongodoc.Model) (err error) {
	defer db.checkError(ctx, &err)
	q := modelQuery(m)
	if q == nil {
		return errgo.WithCausef(nil, params.ErrNotFound, "model not found")
	}
	err = db.Models().Find(q).One(m)
	if err == mgo.ErrNotFound {
		return errgo.WithCausef(nil, params.ErrNotFound, "model not found")
	}
	if err != nil {
		return errgo.Notef(err, "cannot get model")
	}
	return nil
}

// UpdateModel performs the given update on the given model in the
// database. The model is matched using the same criteria as used in
// GetModel. Following the update the given model will contain the
// previously stored model value, unless returnNew is true when it will
// contain the resulting model value. If the model cannot be found then
// an error with a cause of params.ErrNotFound will be returned.
func (db *Database) UpdateModel(ctx context.Context, m *mongodoc.Model, update interface{}, returnNew bool) (err error) {
	defer db.checkError(ctx, &err)
	q := modelQuery(m)
	if q == nil {
		return errgo.WithCausef(nil, params.ErrNotFound, "model not found")
	}
	_, err = db.Models().Find(q).Apply(mgo.Change{Update: update, ReturnNew: returnNew}, m)
	if err == mgo.ErrNotFound {
		return errgo.WithCausef(nil, params.ErrNotFound, "model not found")
	}
	if err != nil {
		return errgo.Notef(err, "cannot get model")
	}
	return nil
}

// modelQuery calculates a query to use to find the matching database
// model. This with be the first non-zero value in the given model from the
// following fields:
//
//  - Path
//  - Controller & UUID
//  - UUID
//
// if all of these fields are zero valued then a nil value will be
// returned.
func modelQuery(m *mongodoc.Model) bson.D {
	switch {
	case m == nil:
		return nil
	case !m.Path.IsZero():
		return bson.D{{"path", m.Path}}
	case m.UUID != "":
		q := bson.D{{"uuid", m.UUID}}
		if !m.Controller.IsZero() {
			q = append(q, bson.DocElem{"controller", m.Controller})
		}
		return q
	default:
		return nil
	}
}
