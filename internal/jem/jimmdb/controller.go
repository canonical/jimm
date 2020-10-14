// Copyright 2016 Canonical Ltd.

package jimmdb

import (
	"context"

	"go.uber.org/zap"
	"gopkg.in/errgo.v1"
	"gopkg.in/mgo.v2"

	"github.com/CanonicalLtd/jimm/internal/mongodoc"
	"github.com/CanonicalLtd/jimm/internal/zapctx"
	"github.com/CanonicalLtd/jimm/internal/zaputil"
	"github.com/CanonicalLtd/jimm/params"
)

// InsertController adds a new controller to the database.
// It returns an error with a params.ErrAlreadyExists
// cause if there is already a controller with the given name.
// If ignores c.Id and sets it from c.Path.
func (db *Database) InsertController(ctx context.Context, c *mongodoc.Controller) (err error) {
	defer db.checkError(ctx, &err)
	c.Id = c.Path.String()
	zapctx.Debug(ctx, "InsertController", zaputil.BSON("c", c))
	err = db.Controllers().Insert(c)
	if mgo.IsDup(err) {
		return errgo.WithCausef(nil, params.ErrAlreadyExists, "")
	}
	if err != nil {
		return errgo.Notef(err, "cannot insert controller")
	}
	return nil
}

// GetController completes the contents of the given controller. The
// database controller is matched using the first non-zero value in the
// given controller from the following fields:
//
//  - Path
//  - UUID
//
// If no matching controller can be found then the returned error will have
// a cause of params.ErrNotFound.
func (db *Database) GetController(ctx context.Context, c *mongodoc.Controller) (err error) {
	defer db.checkError(ctx, &err)
	q := controllerQuery(c)
	if q == nil {
		return errgo.WithCausef(nil, params.ErrNotFound, "controller not found")
	}
	zapctx.Debug(ctx, "GetController", zaputil.BSON("q", q))
	err = db.Controllers().Find(q).One(c)
	if err == mgo.ErrNotFound {
		return errgo.WithCausef(nil, params.ErrNotFound, "controller not found")
	}
	if err != nil {
		return errgo.Notef(err, "cannot get controller")
	}
	return nil
}

// CountControllers counts the number of controllers that match the given query.
func (db *Database) CountControllers(ctx context.Context, q Query) (i int, err error) {
	defer db.checkError(ctx, &err)
	zapctx.Debug(ctx, "CountControllers", zaputil.BSON("q", q))
	n, err := db.Controllers().Find(q).Count()
	if err != nil {
		return 0, errgo.Notef(err, "cannot count controllers")
	}
	return n, nil
}

// ForEachController iterates through every controller that matches the
// given query, calling the given function with each controller. If a sort
// is specified then the controllers will iterate in the sorted order. If
// the function returns an error the iterator stops immediately and the
// error is retuned unmasked.
func (db *Database) ForEachController(ctx context.Context, q Query, sort []string, f func(*mongodoc.Controller) error) (err error) {
	defer db.checkError(ctx, &err)
	query := db.Controllers().Find(q)
	if len(sort) > 0 {
		query = query.Sort(sort...)
	}
	zapctx.Debug(ctx, "ForEachController", zaputil.BSON("q", q), zap.Strings("sort", sort))
	it := query.Iter()
	defer it.Close()
	var c mongodoc.Controller
	for it.Next(&c) {
		if err := f(&c); err != nil {
			return errgo.Mask(err, errgo.Any)
		}
	}
	if err := it.Err(); err != nil {
		return errgo.Notef(err, "cannot iterate controllers")
	}
	return nil
}

// UpdateController performs the given update on the given controller in
// the database. The controller is matched using the same criteria as used
// in GetController. Following the update the given controller will contain
// the previously stored controller value, unless returnNew is true when it
// will contain the resulting controller value. If the controller cannot be
// found then an error with a cause of params.ErrNotFound will be returned.
func (db *Database) UpdateController(ctx context.Context, c *mongodoc.Controller, u *Update, returnNew bool) error {
	q := controllerQuery(c)
	if q == nil {
		return errgo.WithCausef(nil, params.ErrNotFound, "controller not found")
	}
	return db.UpdateControllerQuery(ctx, q, c, u, returnNew)
}

// UpdateControllerQuery is like UpdateController except that the
// controller to update is selected by an arbitrary query rather than
// matching the controller document.
func (db *Database) UpdateControllerQuery(ctx context.Context, q Query, c *mongodoc.Controller, u *Update, returnNew bool) (err error) {
	defer db.checkError(ctx, &err)
	if u == nil || u.IsZero() {
		return nil
	}
	if c == nil {
		c = &mongodoc.Controller{}
	}
	zapctx.Debug(ctx, "UpdateController", zaputil.BSON("q", q), zaputil.BSON("u", u))
	_, err = db.Controllers().Find(q).Apply(mgo.Change{Update: u, ReturnNew: returnNew}, c)
	if err == mgo.ErrNotFound {
		return errgo.WithCausef(nil, params.ErrNotFound, "controller not found")
	}
	if err != nil {
		return errgo.Notef(err, "cannot update controller")
	}
	return nil
}

// RemoveController removes a controller from the database. The controller
// is matched using the same criteria as used in GetController. If the
// controller cannot be found then an error with a cause of
// params.ErrNotFound is returned.
func (db *Database) RemoveController(ctx context.Context, c *mongodoc.Controller) (err error) {
	defer db.checkError(ctx, &err)
	q := controllerQuery(c)
	if q == nil {
		return errgo.WithCausef(nil, params.ErrNotFound, "controller not found")
	}
	zapctx.Debug(ctx, "RemoveController", zaputil.BSON("q", q))
	_, err = db.Controllers().Find(q).Apply(mgo.Change{Remove: true}, c)
	if err == mgo.ErrNotFound {
		return errgo.WithCausef(nil, params.ErrNotFound, "controller not found")
	}
	if err != nil {
		return errgo.Notef(err, "cannot remove controller")
	}
	zapctx.Debug(ctx, "removed controller", zap.Stringer("controller", c.Path))
	return nil
}

// controllerQuery calculates a query to use to find the matching database
// controller. This with be the first non-zero value in the given
// controller from the following fields:
//
//  - Path
//  - UUID
//
// If all of these fields are zero valued then a nil value will be
// returned.
func controllerQuery(c *mongodoc.Controller) Query {
	switch {
	case c == nil:
		return nil
	case !c.Path.IsZero():
		return Eq("path", c.Path)
	case c.UUID != "":
		return Eq("uuid", c.UUID)
	default:
		return nil
	}
}
