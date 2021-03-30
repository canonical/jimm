// Copyright 2016 Canonical Ltd.

package jimmdb

import (
	"context"

	"github.com/juju/mgo/v2"
	"go.uber.org/zap"
	"gopkg.in/errgo.v1"

	"github.com/CanonicalLtd/jimm/internal/mongodoc"
	"github.com/CanonicalLtd/jimm/internal/zapctx"
	"github.com/CanonicalLtd/jimm/internal/zaputil"
	"github.com/CanonicalLtd/jimm/params"
)

// UpsertCredential adds, or updates, a credential in the database. The
// credential is matched on the Path in the given credential. The following
// credential fields are updated/inserted by this operation:
//
//     Id
//     Path
//     Type
//     Label
//     Attributes
//     Revoked
//     AttributesInVault
//     ProviderType
//
// Other fields will remain unset/unchanged. Following a successful upsert
// the given credential will contain the upserted value.
//
// If the given credential does not specify a path an error with a cause of
// params.ErrNotFound will be returned.
func (db *Database) UpsertCredential(ctx context.Context, c *mongodoc.Credential) (err error) {
	defer db.checkError(ctx, &err)
	q := credentialQuery(c)
	if q == nil {
		return errgo.WithCausef(nil, params.ErrNotFound, "credential not found")
	}
	u := new(Update)
	u.Set("type", c.Type)
	u.Set("label", c.Label)
	if c.Attributes == nil {
		u.Unset("attributes")
	} else {
		u.Set("attributes", c.Attributes)
	}
	u.Set("revoked", c.Revoked)
	u.Set("attributesinvault", c.AttributesInVault)
	u.Set("providertype", c.ProviderType)
	u.SetOnInsert("path", c.Path)
	u.SetOnInsert("_id", c.Path.String())
	c.Id = c.Path.String()
	zapctx.Debug(ctx, "UpsertCredential", zaputil.BSON("q", q), zaputil.BSON("u", u))
	_, err = db.Credentials().Find(q).Apply(mgo.Change{Update: u, Upsert: true, ReturnNew: true}, c)
	if mgo.IsDup(err) {
		return errgo.WithCausef(nil, params.ErrAlreadyExists, "")
	}
	if err != nil {
		return errgo.Notef(err, "cannot insert credential")
	}
	return nil
}

// GetCredential completes the contents of the given credential. The
// database credential is matched using the Path of the given credential.
// If no matching credential can be found then the returned error will have
// a cause of params.ErrNotFound.
func (db *Database) GetCredential(ctx context.Context, c *mongodoc.Credential) (err error) {
	defer db.checkError(ctx, &err)
	q := credentialQuery(c)
	if q == nil {
		return errgo.WithCausef(nil, params.ErrNotFound, "credential not found")
	}
	zapctx.Debug(ctx, "GetCredential", zaputil.BSON("q", q))
	err = db.Credentials().Find(q).One(c)
	if err == mgo.ErrNotFound {
		return errgo.WithCausef(nil, params.ErrNotFound, "credential not found")
	}
	if err != nil {
		return errgo.Notef(err, "cannot get credential")
	}
	return nil
}

// ForEachCredential iterates through every credential that matches the given query,
// calling the given function with each credential. If a sort is specified then
// the credentials will iterate in the sorted order. If the function returns an
// error the iterator stops immediately and the error is retuned unmasked.
func (db *Database) ForEachCredential(ctx context.Context, q Query, sort []string, f func(*mongodoc.Credential) error) (err error) {
	defer db.checkError(ctx, &err)
	query := db.Credentials().Find(q)
	if len(sort) > 0 {
		query = query.Sort(sort...)
	}
	zapctx.Debug(ctx, "ForEachCredential", zaputil.BSON("q", q), zap.Strings("sort", sort))
	it := query.Iter()
	defer it.Close()
	var m mongodoc.Credential
	for it.Next(&m) {
		if err := f(&m); err != nil {
			return errgo.Mask(err, errgo.Any)
		}
	}
	if err := it.Err(); err != nil {
		return errgo.Notef(err, "cannot iterate credentials")
	}
	return nil
}

// UpdateCredential performs the given update on the given credential in
// the database. The credential is matched using the same criteria as used
// in GetCredential. Following the update the given credential will contain
// the previously stored credential value, unless returnNew is true when it
// will contain the resulting credential value. If the credential cannot be
// found then an error with a cause of params.ErrNotFound will be returned.
func (db *Database) UpdateCredential(ctx context.Context, c *mongodoc.Credential, u *Update, returnNew bool) (err error) {
	defer db.checkError(ctx, &err)
	q := credentialQuery(c)
	if q == nil {
		return errgo.WithCausef(nil, params.ErrNotFound, "credential not found")
	}
	if u == nil || u.IsZero() {
		return nil
	}
	zapctx.Debug(ctx, "UpdateCredential", zaputil.BSON("q", q), zaputil.BSON("u", u))
	_, err = db.Credentials().Find(q).Apply(mgo.Change{Update: u, ReturnNew: returnNew}, c)
	if err == mgo.ErrNotFound {
		return errgo.WithCausef(nil, params.ErrNotFound, "credential not found")
	}
	if err != nil {
		return errgo.Notef(err, "cannot update credential")
	}
	return nil
}

// UpdateCredentials performs the given update on all credentials that
// match the given query.
func (db *Database) UpdateCredentials(ctx context.Context, q Query, u *Update) (count int, err error) {
	defer db.checkError(ctx, &err)
	zapctx.Debug(ctx, "UpdateCredentials", zaputil.BSON("q", q), zaputil.BSON("u", u))
	info, err := db.Credentials().UpdateAll(q, u)
	if err != nil {
		return 0, errgo.Notef(err, "cannot update credentials")
	}
	return info.Updated, nil
}

// credentialQuery calculates a query to use to find the matching database
// credential. Currently the only supported field is the credential Path.
// If all of these fields are zero valued then a nil value will be
// returned.
func credentialQuery(c *mongodoc.Credential) Query {
	switch {
	case c == nil:
		return nil
	case !c.Path.IsZero():
		return Eq("path", c.Path)
	default:
		return nil
	}
}
