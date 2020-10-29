// Copyright 2020 Canonical Ltd.

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

// InsertApplicationOffer stores an application offer. If an
// applicationoffer exists with the same name on the same model then an
// error with a cause of params.ErrAlreadyExists will be returned.
func (db *Database) InsertApplicationOffer(ctx context.Context, offer *mongodoc.ApplicationOffer) (err error) {
	defer db.checkError(ctx, &err)

	zapctx.Debug(ctx, "InsertApplicationOffer", zaputil.BSON("offer", offer))
	if err = db.ApplicationOffers().Insert(offer); err != nil {
		if mgo.IsDup(err) {
			return errgo.WithCausef(err, params.ErrAlreadyExists, "")
		}
		return errgo.Mask(err)
	}
	return nil
}

// GetApplicationOffer completes the given application-offer.
// GetApplicationOffer finds the application-offer using the first
// non-zero value specified in the offer from the following:
//
//     - OfferUUID & ControllerPath
//     - OfferUUID
//     - OfferURL
//
// If no matching application-offer is found then an error with a cause of
// params.ErrNotFound will be returned.
func (db *Database) GetApplicationOffer(ctx context.Context, offer *mongodoc.ApplicationOffer) (err error) {
	defer db.checkError(ctx, &err)

	q := applicationOfferQuery(offer)
	if q == nil {
		return errgo.WithCausef(nil, params.ErrNotFound, "applicationoffer not found")
	}
	zapctx.Debug(ctx, "GetApplicationOffer", zaputil.BSON("q", q))
	err = db.ApplicationOffers().Find(q).One(&offer)
	if err == mgo.ErrNotFound {
		return errgo.WithCausef(nil, params.ErrNotFound, "applicationoffer not found")
	}
	return errgo.Mask(err)
}

// ForEachApplicationOffer iterates through every application-offer that
// matches the given query, calling the given function with each
// application-offer. If a sort is specified then the cloud-regions will
// iterate in the sorted order. If the function returns an error the
// iterator stops immediately and the error is retuned with the cause
// unmasked.
func (db *Database) ForEachApplicationOffer(ctx context.Context, q Query, sort []string, f func(*mongodoc.ApplicationOffer) error) (err error) {
	defer db.checkError(ctx, &err)

	query := db.ApplicationOffers().Find(q)
	if len(sort) > 0 {
		query = query.Sort(sort...)
	}
	zapctx.Debug(ctx, "ForEachApplicationOffer", zaputil.BSON("q", q), zap.Strings("sort", sort))
	it := query.Iter()
	defer it.Close()
	var offer mongodoc.ApplicationOffer
	for it.Next(&offer) {
		if err := f(&offer); err != nil {
			return errgo.Mask(err, errgo.Any)
		}
	}
	if err := it.Err(); err != nil {
		return errgo.Notef(err, "cannot iterate application-offers")
	}
	return nil
}

// UpdateApplicationOffer updates the specified application-offer. The
// application-offer to update is found using the same criteria as used in
// GetApplicationOffer. If the applicationOffer to update cannot be found
// then an error with a cause of params.ErrNotFound is returned.
func (db *Database) UpdateApplicationOffer(ctx context.Context, offer *mongodoc.ApplicationOffer, u *Update, returnNew bool) (err error) {
	defer db.checkError(ctx, &err)

	q := applicationOfferQuery(offer)
	if q == nil {
		return errgo.WithCausef(nil, params.ErrNotFound, "applicationoffer not found")
	}
	zapctx.Debug(ctx, "UpdateApplicationOffer", zaputil.BSON("q", q), zaputil.BSON("u", u))
	_, err = db.ApplicationOffers().Find(q).Apply(mgo.Change{Update: u, ReturnNew: returnNew}, offer)
	if err == mgo.ErrNotFound {
		return errgo.WithCausef(nil, params.ErrNotFound, "applicationoffer not found")
	}
	return errgo.Mask(err)
}

// RemoveApplicationOffer removes the matching application-offer. The
// application-offer to remove is matched using the same criteria as is
// used in GetApplicationOffer. If no matching application-offer is found
// then an error with a cause of params.ErrNotFound is returned.
func (db *Database) RemoveApplicationOffer(ctx context.Context, offer *mongodoc.ApplicationOffer) (err error) {
	defer db.checkError(ctx, &err)

	q := applicationOfferQuery(offer)
	if q == nil {
		return errgo.WithCausef(nil, params.ErrNotFound, "applicationoffer not found")
	}

	zapctx.Debug(ctx, "RemoveApplicationOffer", zaputil.BSON("q", q))
	if _, err := db.ApplicationOffers().Find(q).Apply(mgo.Change{Remove: true}, offer); err != nil {
		if err == mgo.ErrNotFound {
			return errgo.WithCausef(nil, params.ErrNotFound, "")
		}
		return errgo.Mask(err)
	}
	return nil
}

func applicationOfferQuery(o *mongodoc.ApplicationOffer) Query {
	switch {
	case o.OfferUUID != "":
		if o.ControllerPath.IsZero() {
			return Eq("_id", o.OfferUUID)
		}
		return And(Eq("_id", o.OfferUUID), Eq("controller-path", o.ControllerPath))
	case o.OfferURL != "":
		return Eq("offer-url", o.OfferURL)
	default:
		return nil
	}
}
