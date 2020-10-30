// Copyright 2020 Canonical Ltd.package jimmdb

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

// UpsertModelDefaultConfig inserts, or updates, the specified
// model-default-config. The modle-default-config is matched on a
// combination of user, cloud and region. If the given model-default-config
// does not specify enough information to uniquely identify a
// model-default-config then an error with a cause of params.ErrNotFound
// will be retuned.
func (db *Database) UpsertModelDefaultConfig(ctx context.Context, d *mongodoc.CloudRegionDefaults) (err error) {
	defer db.checkError(ctx, &err)
	q := modelDefaultConfigQuery(d)
	if q == nil {
		return errgo.WithCausef(nil, params.ErrNotFound, "model-default-config not found")
	}
	u := new(Update)
	u.SetOnInsert("cloud", d.Cloud)
	u.SetOnInsert("region", d.Region)
	u.SetOnInsert("user", d.User)
	for k, v := range d.Defaults {
		u.Set("defaults."+k, v)
	}
	zapctx.Debug(ctx, "UpsertModelDefaultConfig", zaputil.BSON("q", q), zaputil.BSON("u", u))
	_, err = db.ModelDefaultConfigs().Find(q).Apply(mgo.Change{
		Update:    u,
		Upsert:    true,
		ReturnNew: true,
	}, d)
	return errgo.Mask(err)
}

// ForEachModelDefaultConfig iterates through every model-default-config
// that matches the given query, calling the given function with each
// model-default-config. If a sort is specified then the
// model-default-configs will iterate in the sorted order. If the function
// returns an error the iterator stops immediately and the error is retuned
// with the cause unmasked.
func (db *Database) ForEachModelDefaultConfig(ctx context.Context, q Query, sort []string, f func(*mongodoc.CloudRegionDefaults) error) (err error) {
	defer db.checkError(ctx, &err)
	query := db.ModelDefaultConfigs().Find(q)
	if len(sort) > 0 {
		query = query.Sort(sort...)
	}
	zapctx.Debug(ctx, "ForEachModelDefaultConfig", zaputil.BSON("q", q), zap.Strings("sort", sort))
	it := query.Iter()
	defer it.Close()
	var d mongodoc.CloudRegionDefaults
	for it.Next(&d) {
		if err := f(&d); err != nil {
			return errgo.Mask(err, errgo.Any)
		}
	}
	if err := it.Err(); err != nil {
		return errgo.Notef(err, "cannot iterate model-default-configs")
	}
	return nil
}

// UpdateModelDefaultConfig updates the specified model-default-config
// using the given update. The model-default-config is matched on a
// combination of user, cloud and region. If the model-default-config
// cannot be found then an error with a cause of params.ErrNotFound will be
// retuned.
func (db *Database) UpdateModelDefaultConfig(ctx context.Context, d *mongodoc.CloudRegionDefaults, u *Update, returnNew bool) (err error) {
	defer db.checkError(ctx, &err)
	q := modelDefaultConfigQuery(d)
	if q == nil {
		return errgo.WithCausef(nil, params.ErrNotFound, "model-default-config not found")
	}
	if u == nil || u.IsZero() {
		return nil
	}
	zapctx.Debug(ctx, "UpdateModelDefaultConfig", zaputil.BSON("q", q), zaputil.BSON("u", u))
	_, err = db.ModelDefaultConfigs().Find(q).Apply(mgo.Change{
		Update:    u,
		Upsert:    true,
		ReturnNew: true,
	}, d)
	if err == mgo.ErrNotFound {
		return errgo.WithCausef(nil, params.ErrNotFound, "model-default-config not found")
	}
	return errgo.Mask(err)
}

func modelDefaultConfigQuery(d *mongodoc.CloudRegionDefaults) Query {
	if d.User == "" || d.Cloud == "" {
		return nil
	}
	return And(Eq("user", d.User), Eq("cloud", d.Cloud), Eq("region", d.Region))
}
