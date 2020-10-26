// Copyright 2016 Canonical Ltd.

package jimmdb

import (
	"context"
	"fmt"

	"go.uber.org/zap"
	"gopkg.in/errgo.v1"
	"gopkg.in/mgo.v2"

	"github.com/CanonicalLtd/jimm/internal/mongodoc"
	"github.com/CanonicalLtd/jimm/internal/zapctx"
	"github.com/CanonicalLtd/jimm/internal/zaputil"
	"github.com/CanonicalLtd/jimm/params"
)

// InsertCloudRegion inserts a new CloudRegion to the database. If the
// region already exists then an error with the cause
// params.ErrAlreadyExists is returned.
func (db *Database) InsertCloudRegion(ctx context.Context, cr *mongodoc.CloudRegion) (err error) {
	defer db.checkError(ctx, &err)
	cr.Id = fmt.Sprintf("%s/%s", cr.Cloud, cr.Region)
	zapctx.Debug(ctx, "InsertCloudRegion", zaputil.BSON("cr", cr))
	err = db.CloudRegions().Insert(cr)
	if mgo.IsDup(err) {
		err = errgo.WithCausef(nil, params.ErrAlreadyExists, "")
	}
	return errgo.Mask(err, errgo.Is(params.ErrAlreadyExists))
}

// GetCloudRegion fills in the given mongodoc.CloudRegion. GetCloudRegion
// will match either on the first available combination of:
//
//     - cloud and region name
//     - cloud type and region name
//
// If the region name is "" then the CloudRegion record will be for the
// cloud.GetCloudRegion returns an error with a params.ErrNotFound cause
// if there is no CloudRegion found.
func (db *Database) GetCloudRegion(ctx context.Context, cr *mongodoc.CloudRegion) (err error) {
	defer db.checkError(ctx, &err)

	q := db.cloudRegionQuery(cr)
	if q == nil {
		return errgo.WithCausef(nil, params.ErrNotFound, "cloudregion not found")
	}
	zapctx.Debug(ctx, "GetCloudRegion", zaputil.BSON("q", q))
	err = db.CloudRegions().Find(q).One(&cr)
	if err == mgo.ErrNotFound {
		return errgo.WithCausef(nil, params.ErrNotFound, "cloudregion not found")
	}
	if err != nil {
		return errgo.Notef(err, "cannot get cloudregion")
	}

	return nil
}

// ForEachCloudRegion iterates through every cloud-region that matches the
// given query, calling the given function with each cloud-region. If a
// sort is specified then the cloud-regions will iterate in the sorted
// order. If the function returns an error the iterator stops immediately
// and the error is retuned unmasked.
func (db *Database) ForEachCloudRegion(ctx context.Context, q Query, sort []string, f func(*mongodoc.CloudRegion) error) (err error) {
	defer db.checkError(ctx, &err)
	query := db.CloudRegions().Find(q)
	if len(sort) > 0 {
		query = query.Sort(sort...)
	}
	zapctx.Debug(ctx, "ForEachCloudRegion", zaputil.BSON("q", q), zap.Strings("sort", sort))
	it := query.Iter()
	defer it.Close()
	var cr mongodoc.CloudRegion
	for it.Next(&cr) {
		if err := f(&cr); err != nil {
			return errgo.Mask(err, errgo.Any)
		}
	}
	if err := it.Err(); err != nil {
		return errgo.Notef(err, "cannot iterate cloudregions")
	}
	return nil
}

// UpsertCloudRegion inserts, or updates, the given cloud region document.
func (db *Database) UpsertCloudRegion(ctx context.Context, cr *mongodoc.CloudRegion) (err error) {
	defer db.checkError(ctx, &err)
	cr.Id = fmt.Sprintf("%s/%s", cr.Cloud, cr.Region)
	q := Eq("_id", cr.Id)
	u := new(Update)
	u.SetOnInsert("cloud", cr.Cloud)
	u.SetOnInsert("region", cr.Region)
	u.SetOnInsert("acl", cr.ACL)
	u.Set("providertype", cr.ProviderType)
	u.Set("authtypes", cr.AuthTypes)
	u.Set("endpoint", cr.Endpoint)
	u.Set("identityendpoint", cr.IdentityEndpoint)
	u.Set("storageendpoint", cr.StorageEndpoint)
	u.Set("cacertificates", cr.CACertificates)
	for _, c := range cr.PrimaryControllers {
		u.AddToSet("primarycontrollers", c)
	}
	for _, c := range cr.SecondaryControllers {
		u.AddToSet("secondarycontrollers", c)
	}
	zapctx.Debug(ctx, "UpsertCloudRegion", zaputil.BSON("q", q), zaputil.BSON("u", u))
	_, err = db.CloudRegions().Find(q).Apply(mgo.Change{Update: u, Upsert: true, ReturnNew: true}, cr)
	if err != nil {
		err = errgo.Notef(err, "cannot upsert cloudregion")
	}
	return err
}

// UpdateCloudRegions performs the given update on all cloud-regions that
// match the given query. The number updated is returned along with any
// error.
func (db *Database) UpdateCloudRegions(ctx context.Context, q Query, u *Update) (count int, err error) {
	defer db.checkError(ctx, &err)
	zapctx.Debug(ctx, "UpdateCloudRegions", zaputil.BSON("q", q), zaputil.BSON("u", u))
	info, err := db.CloudRegions().UpdateAll(q, u)
	if err != nil {
		return 0, errgo.Notef(err, "cannot update cloudregions")
	}
	return info.Updated, nil
}

// RemoveCloudRegion removes the given cloud region. The cloud-region is
// matched using the same criteria as in GetCloudRegion. If the
// cloud-region cannot be found then an error of type params.ErrNotFound is
// returned.
func (db *Database) RemoveCloudRegion(ctx context.Context, cr *mongodoc.CloudRegion) (err error) {
	defer db.checkError(ctx, &err)
	q := db.cloudRegionQuery(cr)
	zapctx.Debug(ctx, "RemoveCloudRegion", zaputil.BSON("q", q))
	_, err = db.CloudRegions().Find(q).Apply(mgo.Change{Remove: true}, cr)
	if err == mgo.ErrNotFound {
		return errgo.WithCausef(nil, params.ErrNotFound, "cloudregion not found")
	}
	if err != nil {
		return errgo.Notef(err, "cannot remove cloudregion")
	}
	return nil
}

// RemoveCloudRegions removes all the cloud-regions that match the given
// query. It is not an error if no cloud-regions match and therefore
// nothing is removed.
func (db *Database) RemoveCloudRegions(ctx context.Context, q Query) (count int, err error) {
	defer db.checkError(ctx, &err)
	zapctx.Debug(ctx, "RemoveCloudRegions", zaputil.BSON("q", q))
	info, err := db.CloudRegions().RemoveAll(q)
	if err != nil {
		return 0, errgo.Notef(err, "cannot remove cloudregions")
	}
	return info.Removed, nil
}

func (db *Database) cloudRegionQuery(cr *mongodoc.CloudRegion) Query {
	switch {
	case cr.Cloud != "":
		return And(Eq("cloud", cr.Cloud), Eq("region", cr.Region))
	case cr.ProviderType != "" && cr.Region != "":
		return And(Eq("providertype", cr.ProviderType), Eq("region", cr.Region))
	default:
		return nil
	}
}
