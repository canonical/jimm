// Copyright 2016 Canonical Ltd.

package jimmdb

import (
	"context"

	"github.com/juju/mgo/v2"
	"go.uber.org/zap"
	"gopkg.in/errgo.v1"

	"github.com/canonical/jimm/internal/mongodoc"
	"github.com/canonical/jimm/internal/zapctx"
	"github.com/canonical/jimm/internal/zaputil"
	"github.com/canonical/jimm/params"
)

// InsertModel adds a new model to the database.
// It returns an error with a params.ErrAlreadyExists
// cause if there is already an model with the given name.
// If ignores m.Id and sets it from m.Path.
func (db *Database) InsertModel(ctx context.Context, m *mongodoc.Model) (err error) {
	defer db.checkError(ctx, &err)
	m.Id = m.Path.String()
	zapctx.Debug(ctx, "InsertModel", zaputil.BSON("m", m))
	err = db.Models().Insert(m)
	if mgo.IsDup(err) {
		return errgo.WithCausef(nil, params.ErrAlreadyExists, "")
	}
	if err != nil {
		return errgo.Notef(err, "cannot insert model")
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
	zapctx.Debug(ctx, "GetModel", zaputil.BSON("q", q))
	err = db.Models().Find(q).One(m)
	if err == mgo.ErrNotFound {
		return errgo.WithCausef(nil, params.ErrNotFound, "model not found")
	}
	if err != nil {
		return errgo.Notef(err, "cannot get model")
	}
	return nil
}

// CountModels counts the number of models that match the given query.
func (db *Database) CountModels(ctx context.Context, q Query) (i int, err error) {
	defer db.checkError(ctx, &err)
	zapctx.Debug(ctx, "CountModels", zaputil.BSON("q", q))
	n, err := db.Models().Find(q).Count()
	if err != nil {
		return 0, errgo.Notef(err, "cannot count models")
	}
	return n, nil
}

// ForEachModel iterates through every model that matches the given query,
// calling the given function with each model. If a sort is specified then
// the models will iterate in the sorted order. If the function returns an
// error the iterator stops immediately and the error is retuned unmasked.
func (db *Database) ForEachModel(ctx context.Context, q Query, sort []string, f func(*mongodoc.Model) error) (err error) {
	defer db.checkError(ctx, &err)
	query := db.Models().Find(q)
	if len(sort) > 0 {
		query = query.Sort(sort...)
	}
	zapctx.Debug(ctx, "ForEachModel", zaputil.BSON("q", q), zap.Strings("sort", sort))
	it := query.Iter()
	defer it.Close()
	var m mongodoc.Model
	for it.Next(&m) {
		if err := f(&m); err != nil {
			return errgo.Mask(err, errgo.Any)
		}
	}
	if err := it.Err(); err != nil {
		return errgo.Notef(err, "cannot iterate models")
	}
	return nil
}

// UpdateModel performs the given update on the given model in the
// database. The model is matched using the same criteria as used in
// GetModel. Following the update the given model will contain the
// previously stored model value, unless returnNew is true when it will
// contain the resulting model value. If the model cannot be found then
// an error with a cause of params.ErrNotFound will be returned.
func (db *Database) UpdateModel(ctx context.Context, m *mongodoc.Model, u *Update, returnNew bool) (err error) {
	defer db.checkError(ctx, &err)
	q := modelQuery(m)
	if q == nil {
		return errgo.WithCausef(nil, params.ErrNotFound, "model not found")
	}
	if u == nil || u.IsZero() {
		return nil
	}
	zapctx.Debug(ctx, "UpdateModel", zaputil.BSON("q", q), zaputil.BSON("u", u))
	_, err = db.Models().Find(q).Apply(mgo.Change{Update: u, ReturnNew: returnNew}, m)
	if err == mgo.ErrNotFound {
		return errgo.WithCausef(nil, params.ErrNotFound, "model not found")
	}
	if err != nil {
		return errgo.Notef(err, "cannot update model")
	}
	return nil
}

// RemoveModel removes a model from the database. The model is matched
// using the same criteria as used in GetModel. If the model cannot be
// found then an error with a cause of params.ErrNotFound is returned.
func (db *Database) RemoveModel(ctx context.Context, m *mongodoc.Model) (err error) {
	defer db.checkError(ctx, &err)
	q := modelQuery(m)
	if q == nil {
		return errgo.WithCausef(nil, params.ErrNotFound, "model not found")
	}
	zapctx.Debug(ctx, "RemoveModel", zaputil.BSON("q", q))
	_, err = db.Models().Find(q).Apply(mgo.Change{Remove: true}, m)
	if err == mgo.ErrNotFound {
		return errgo.WithCausef(nil, params.ErrNotFound, "model not found")
	}
	if err != nil {
		return errgo.Notef(err, "cannot remove model")
	}
	zapctx.Debug(ctx, "removed model", zap.Stringer("model", m.Path))
	return nil
}

// RemoveModels removes all models that match the given query from the
// database. The number of removed models will be returned.
func (db *Database) RemoveModels(ctx context.Context, q Query) (count int, err error) {
	defer db.checkError(ctx, &err)
	zapctx.Debug(ctx, "RemoveModels", zaputil.BSON("q", q))
	info, err := db.Models().RemoveAll(q)
	if err != nil {
		return 0, errgo.Notef(err, "cannot remove models")
	}
	return info.Removed, nil
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
func modelQuery(m *mongodoc.Model) Query {
	switch {
	case m == nil:
		return nil
	case !m.Path.IsZero():
		return Eq("path", m.Path)
	case m.UUID != "":
		q := Eq("uuid", m.UUID)
		if !m.Controller.IsZero() {
			q = And(q, Eq("controller", m.Controller))
		}
		return q
	default:
		return nil
	}
}
