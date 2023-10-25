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

// UpsertApplication inserts, or updates, an application in the database.
// Applications are matched on a combination of controller, model-uuid, and
// application-name. If the given application does not specify enough
// information to identify it an error with a cause of params.ErrNotFound
// is returned.
func (db *Database) UpsertApplication(ctx context.Context, app *mongodoc.Application) (err error) {
	defer db.checkError(ctx, &err)

	q := applicationQuery(app)
	if q == nil {
		return errgo.WithCausef(nil, params.ErrNotFound, "application not found")
	}
	u := new(Update)
	u.SetOnInsert("_id", app.Controller+" "+app.Info.ModelUUID+" "+app.Info.Name)
	u.Set("controller", app.Controller)
	u.Set("cloud", app.Cloud)
	u.Set("region", app.Region)
	u.Set("info", app.Info)

	zapctx.Debug(ctx, "UpsertApplication", zaputil.BSON("q", q), zaputil.BSON("u", u))
	_, err = db.Applications().Find(q).Apply(mgo.Change{Update: u, Upsert: true, ReturnNew: true}, app)
	return errgo.Mask(err)
}

// ForEachApplication iterates through every application that matches the
// given query, calling the given function with each application. If a sort
// is specified then the applications will iterate in the sorted order. If
// the function returns an error the iterator stops immediately and the
// error is retuned with the cause unmasked.
func (db *Database) ForEachApplication(ctx context.Context, q Query, sort []string, f func(*mongodoc.Application) error) (err error) {
	defer db.checkError(ctx, &err)

	query := db.Applications().Find(q)
	if len(sort) > 0 {
		query = query.Sort(sort...)
	}
	zapctx.Debug(ctx, "ForEachApplication", zaputil.BSON("q", q), zap.Strings("sort", sort))
	it := query.Iter()
	defer it.Close()
	var application mongodoc.Application
	for it.Next(&application) {
		if err := f(&application); err != nil {
			return errgo.Mask(err, errgo.Any)
		}
	}
	if err := it.Err(); err != nil {
		return errgo.Notef(err, "cannot iterate applications")
	}
	return nil
}

// RemoveApplication removes the given application from the database. The
// application is matched using the same criteria as in UpsertApplication.
// If a matching application cannot be found an error with a cause of
// params.ErrNotFound is returned.
func (db *Database) RemoveApplication(ctx context.Context, app *mongodoc.Application) (err error) {
	defer db.checkError(ctx, &err)
	q := applicationQuery(app)
	if q == nil {
		return errgo.WithCausef(nil, params.ErrNotFound, "application not found")
	}
	zapctx.Debug(ctx, "RemoveApplication", zaputil.BSON("q", q))
	_, err = db.Applications().Find(q).Apply(mgo.Change{Remove: true}, app)
	if err == mgo.ErrNotFound {
		return errgo.WithCausef(nil, params.ErrNotFound, "application not found")
	}
	return errgo.Mask(err)
}

// RemoveApplications removes all the applications that match the given
// query. It is not an error if no applications match and therefore nothing
// is removed.
func (db *Database) RemoveApplications(ctx context.Context, q Query) (count int, err error) {
	defer db.checkError(ctx, &err)
	zapctx.Debug(ctx, "RemoveApplications", zaputil.BSON("q", q))
	info, err := db.Applications().RemoveAll(q)
	if err != nil {
		return 0, errgo.Notef(err, "cannot remove applications")
	}
	return info.Removed, nil
}

func applicationQuery(m *mongodoc.Application) Query {
	if m.Controller == "" || m.Info == nil || m.Info.ModelUUID == "" || m.Info.Name == "" {
		return nil
	}
	return Eq("_id", m.Controller+" "+m.Info.ModelUUID+" "+m.Info.Name)
}
