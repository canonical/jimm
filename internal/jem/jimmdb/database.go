// Copyright 2016 Canonical Ltd.

package jimmdb

import (
	"context"
	"fmt"
	"regexp"

	jujuparams "github.com/juju/juju/apiserver/params"
	"gopkg.in/errgo.v1"
	"gopkg.in/macaroon-bakery.v2/bakery/identchecker"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"

	"github.com/CanonicalLtd/jimm/internal/auth"
	"github.com/CanonicalLtd/jimm/internal/conv"
	"github.com/CanonicalLtd/jimm/internal/mgosession"
	"github.com/CanonicalLtd/jimm/internal/mongodoc"
	"github.com/CanonicalLtd/jimm/internal/servermon"
	"github.com/CanonicalLtd/jimm/internal/zapctx"
	"github.com/CanonicalLtd/jimm/internal/zaputil"
	"github.com/CanonicalLtd/jimm/params"
)

// Database wraps an mgo.DB ands adds a number of methods for
// manipulating the database.
type Database struct {
	// sessionPool holds the session pool. This will be
	// reset if there's an unexpected mongodb error.
	sessionPool *mgosession.Pool
	*mgo.Database
}

// checkError inspects the value pointed to by err and marks the database
// connection as dead if it looks like the error is probably
// due to a database connection issue. There may be false positives, but
// the worst that can happen is that we do the occasional unnecessary
// Session.Copy which shouldn't be a problem.
//
// TODO if mgo supported it, a better approach would be to check whether
// the mgo.Session is permanently dead.
func (db *Database) checkError(ctx context.Context, err *error) {
	if *err == nil {
		return
	}
	_, ok := errgo.Cause(*err).(params.ErrorCode)
	if ok {
		return
	}
	db.sessionPool.Reset()

	servermon.DatabaseFailCount.Inc()
	zapctx.Warn(ctx, "discarding mongo session", zaputil.Error(*err))
}

// NewDatabase returns a new Database named dbName using
// a session taken from the given pool. The database session
// should be closed after the database is finished with.
func NewDatabase(ctx context.Context, pool *mgosession.Pool, dbName string) *Database {
	return &Database{
		sessionPool: pool,
		Database:    pool.Session(ctx).DB(dbName),
	}
}

func (db *Database) Clone() *Database {
	return &Database{
		sessionPool: db.sessionPool,
		Database:    db.Database.With(db.Database.Session.Clone()),
	}
}

func (db *Database) EnsureIndexes() error {
	indexes := []struct {
		c *mgo.Collection
		i mgo.Index
	}{{
		db.Controllers(),
		mgo.Index{Key: []string{"uuid"}},
	}, {
		db.Machines(),
		mgo.Index{Key: []string{"info.uuid"}},
	}, {
		db.Applications(),
		mgo.Index{Key: []string{"info.uuid"}},
	}, {
		db.Models(),
		mgo.Index{Key: []string{"uuid"}, Unique: true},
	}, {
		db.Models(),
		mgo.Index{Key: []string{"credential"}},
	}, {
		db.Credentials(),
		mgo.Index{Key: []string{"path.entitypath.user", "path.cloud"}},
	}, {
		db.ApplicationOffers(),
		mgo.Index{Key: []string{"offer-url"}, Unique: true},
	}, {
		db.ApplicationOffers(),
		mgo.Index{Key: []string{"owner-name", "model-name", "offer-name"}, Unique: true},
	}}
	for _, idx := range indexes {
		err := idx.c.EnsureIndex(idx.i)
		if err != nil {
			return errgo.Notef(err, "cannot ensure index with keys %v on collection %s", idx.i, idx.c.Name)
		}
	}
	return nil
}

// SetModelController updates the given model so that it's associated
// with the given controller. This should only be called when migration
// has been initiated for the model and the new controller has been
// verified to exist.
func (db *Database) SetModelController(ctx context.Context, model params.EntityPath, newController params.EntityPath) (err error) {
	defer db.checkError(ctx, &err)
	err = db.Models().UpdateId(model.String(), bson.D{{
		"$set", bson.D{{
			"controller", newController,
		}},
	}})
	if errgo.Cause(err) == mgo.ErrNotFound {
		return errgo.WithCausef(err, params.ErrNotFound, "cannot update %s", model)
	}
	return errgo.Mask(err)
}

// RemoveControllerMachines removes all of the machine information for
// the given controller.
func (db *Database) RemoveControllerMachines(ctx context.Context, ctlPath params.EntityPath) (err error) {
	defer db.checkError(ctx, &err)
	if _, err := db.Machines().RemoveAll(bson.D{{"controller", ctlPath.String()}}); err != nil {
		return errgo.Notef(err, "cannot remove machines for controller %v", ctlPath)
	}
	return nil
}

// RemoveControllerApplications removes all of the application information for
// the given controller.
func (db *Database) RemoveControllerApplications(ctx context.Context, ctlPath params.EntityPath) (err error) {
	defer db.checkError(ctx, &err)
	if _, err := db.Applications().RemoveAll(bson.D{{"controller", ctlPath.String()}}); err != nil {
		return errgo.Notef(err, "cannot remove applications for controller %v", ctlPath)
	}
	return nil
}

// UpdateMachineInfo updates the information associated with a machine.
func (db *Database) UpdateMachineInfo(ctx context.Context, m *mongodoc.Machine) (err error) {
	defer db.checkError(ctx, &err)
	m.Id = m.Controller + " " + m.Info.ModelUUID + " " + m.Info.Id
	if m.Info.Life == "dead" {
		if err := db.Machines().RemoveId(m.Id); err != nil {
			if errgo.Cause(err) == mgo.ErrNotFound {
				return nil
			}
			return errgo.Notef(err, "cannot remove machine %v in model %v", m.Info.Id, m.Info.ModelUUID)
		}
	} else {
		update := bson.D{{
			"$set", bson.D{
				{"info", m.Info},
				{"controller", m.Controller},
				{"cloud", m.Cloud},
				{"region", m.Region},
			},
		}}
		if _, err := db.Machines().UpsertId(m.Id, update); err != nil {
			return errgo.Notef(err, "cannot update machine %v in model %v", m.Info.Id, m.Info.ModelUUID)
		}
	}
	return nil
}

// MachinesForModel returns information on all the machines in the model with
// the given UUID.
func (db *Database) MachinesForModel(ctx context.Context, modelUUID string) (docs []mongodoc.Machine, err error) {
	defer db.checkError(ctx, &err)
	err = db.Machines().Find(bson.D{{"info.modeluuid", modelUUID}}).Sort("_id").All(&docs)
	if err != nil {
		return nil, errgo.Mask(err)
	}
	return docs, nil
}

// UpdateApplicationInfo updates the information associated with an application.
func (db *Database) UpdateApplicationInfo(ctx context.Context, app *mongodoc.Application) (err error) {
	defer db.checkError(ctx, &err)
	app.Id = app.Controller + " " + app.Info.ModelUUID + " " + app.Info.Name
	if app.Info.Life == "dead" {
		if err := db.Applications().RemoveId(app.Id); err != nil {
			if errgo.Cause(err) == mgo.ErrNotFound {
				return nil
			}
			return errgo.Notef(err, "cannot remove application %v in model %v", app.Info.Name, app.Info.ModelUUID)
		}
	} else {
		update := bson.D{{
			"$set", bson.D{
				{"info", app.Info},
				{"controller", app.Controller},
				{"cloud", app.Cloud},
				{"region", app.Region},
			},
		}}
		if _, err := db.Applications().UpsertId(app.Id, update); err != nil {
			return errgo.Notef(err, "cannot update application %v in model %v", app.Info.Name, app.Info.ModelUUID)
		}
	}
	return nil
}

// ApplicationsForModel returns information on all the applications in the model with
// the given UUID.
func (db *Database) ApplicationsForModel(ctx context.Context, modelUUID string) (docs []mongodoc.Application, err error) {
	defer db.checkError(ctx, &err)
	err = db.Applications().Find(bson.D{{"info.modeluuid", modelUUID}}).Sort("_id").All(&docs)
	if err != nil {
		return nil, errgo.Mask(err)
	}
	return docs, nil
}

var selectACL = bson.D{{"acl", 1}}

// GetACL retrieves the ACL for the document at path in coll, which must
// have been obtained from db. If the document is not found, the
// returned error will have the cause params.ErrNotFound.
func (db *Database) GetACL(ctx context.Context, coll *mgo.Collection, path params.EntityPath) (_ params.ACL, err error) {
	defer db.checkError(ctx, &err)
	var doc struct {
		ACL params.ACL
	}
	if err = coll.FindId(path.String()).Select(selectACL).One(&doc); err != nil {
		if err == mgo.ErrNotFound {
			err = params.ErrNotFound
		}
		return params.ACL{}, errgo.Mask(err, errgo.Is(params.ErrNotFound))
	}
	return doc.ACL, nil
}

// SetACL sets the ACL for the path document in c (which must
// have been obtained from db) to be equal to acl.
func (db *Database) SetACL(ctx context.Context, c *mgo.Collection, path params.EntityPath, acl params.ACL) (err error) {
	defer db.checkError(ctx, &err)
	err = c.UpdateId(path.String(), bson.D{{"$set", bson.D{{"acl", acl}}}})
	if err == nil {
		return nil
	}
	if err == mgo.ErrNotFound {
		return errgo.WithCausef(nil, params.ErrNotFound, "%q not found", path)
	}
	return errgo.Notef(err, "cannot update ACL on %q", path)
}

// Grant updates the ACL for the path document in c (which must
// have been obtained from db) to include user.
func (db *Database) Grant(ctx context.Context, c *mgo.Collection, path params.EntityPath, user params.User) (err error) {
	defer db.checkError(ctx, &err)
	err = c.UpdateId(path.String(), bson.D{{"$addToSet", bson.D{{"acl.read", user}}}})
	if err == nil {
		return nil
	}
	if err == mgo.ErrNotFound {
		return errgo.WithCausef(nil, params.ErrNotFound, "%q not found", path)
	}
	return errgo.Notef(err, "cannot update ACL on %q", path)
}

// Revoke updates the ACL for the path document in c (which must
// have been obtained from db) to not include user.
func (db *Database) Revoke(ctx context.Context, c *mgo.Collection, path params.EntityPath, user params.User) (err error) {
	defer db.checkError(ctx, &err)
	err = c.UpdateId(path.String(), bson.D{{"$pull", bson.D{{"acl.read", user}}}})
	if err == nil {
		return nil
	}
	if err == mgo.ErrNotFound {
		return errgo.WithCausef(nil, params.ErrNotFound, "%q not found", path)
	}
	return errgo.Notef(err, "cannot update ACL on %q", path)
}

// CheckReadACL checks that the entity with the given path in the given
// collection (which must have been obtained from db) can be read by the
// currently authenticated user.
func (db *Database) CheckReadACL(ctx context.Context, id identchecker.ACLIdentity, c *mgo.Collection, path params.EntityPath) (err error) {
	defer db.checkError(ctx, &err)
	// The user can always access their own entities.
	if err := auth.CheckIsUser(ctx, id, path.User); err == nil {
		return nil
	}
	acl, err := db.GetACL(ctx, c, path)
	if errgo.Cause(err) == params.ErrNotFound {
		// The document is not found - and we've already checked
		// that the currently authenticated user cannot speak for
		// path.User, so return an unauthorized error to stop
		// people probing for the existence of other people's entities.
		return params.ErrUnauthorized
	}
	if err := auth.CheckACL(ctx, id, acl.Read); err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrUnauthorized))
	}
	return nil
}

// AddApplicationOffer stores an application offer.
func (db *Database) AddApplicationOffer(ctx context.Context, offer *mongodoc.ApplicationOffer) (err error) {
	defer db.checkError(ctx, &err)

	if err = db.ApplicationOffers().Insert(offer); err != nil {
		if mgo.IsDup(err) {
			err = errgo.WithCausef(err, params.ErrAlreadyExists, "")
		}
	}
	return errgo.Mask(err, errgo.Is(params.ErrAlreadyExists))
}

// UpdateApplicationOffer updates the application offer.
func (db *Database) UpdateApplicationOffer(ctx context.Context, offer *mongodoc.ApplicationOffer) (err error) {
	defer db.checkError(ctx, &err)
	update := bson.D{
		{Name: "offer-url", Value: offer.OfferURL},
		{Name: "offer-name", Value: offer.OfferName},
		{Name: "owner-name", Value: offer.OwnerName},
		{Name: "application-name", Value: offer.ApplicationName},
		{Name: "application-description", Value: offer.ApplicationDescription},
		{Name: "endpoints", Value: offer.Endpoints},
		{Name: "spaces", Value: offer.Spaces},
		{Name: "bindings", Value: offer.Bindings},
		{Name: "users", Value: offer.Users},
		{Name: "charm-url", Value: offer.CharmURL},
		{Name: "connections", Value: offer.Connections},
		{Name: "model-name", Value: offer.ModelName},
	}
	err = db.ApplicationOffers().UpdateId(offer.OfferUUID, bson.D{{
		Name: "$set", Value: update,
	}})
	if errgo.Cause(err) == mgo.ErrNotFound {
		return errgo.WithCausef(err, params.ErrNotFound, "cannot update offer %s", offer.OfferUUID)
	}
	return errgo.Mask(err)
}

// GetApplicationOffer completes the given application offer.
// GetApplicationOffer finds the application offer using the first
// non-zero value specified in the offer from the following:
//
//     - OfferUUID
//     - OfferURL
func (db *Database) GetApplicationOffer(ctx context.Context, offer *mongodoc.ApplicationOffer) (err error) {
	defer db.checkError(ctx, &err)

	var q *mgo.Query
	switch {
	case offer.OfferUUID != "":
		q = db.ApplicationOffers().FindId(offer.OfferUUID)
	case offer.OfferURL != "":
		q = db.ApplicationOffers().Find(bson.M{"offer-url": offer.OfferURL})
	default:
		return errgo.WithCausef(nil, params.ErrNotFound, "")
	}
	if err = q.One(offer); err != nil {
		if err == mgo.ErrNotFound {
			return errgo.WithCausef(nil, params.ErrNotFound, "")
		}
		return errgo.Mask(err)
	}
	return nil
}

// RemoveApplicationOffer removes an application offer.
func (db *Database) RemoveApplicationOffer(ctx context.Context, offerUUID string) (err error) {
	defer db.checkError(ctx, &err)

	if err = db.ApplicationOffers().RemoveId(offerUUID); err != nil {
		if err == mgo.ErrNotFound {
			return errgo.WithCausef(nil, params.ErrNotFound, "")
		}
		return errgo.Mask(err)
	}
	return nil
}

// SetApplicationOfferAccess sets a user access level to an application offer.
func (db *Database) SetApplicationOfferAccess(ctx context.Context, user params.User, offerUUID string, access mongodoc.ApplicationOfferAccessPermission) (err error) {
	defer db.checkError(ctx, &err)

	u := mongodoc.User(user)

	// Add the new access level, if it doesn't exist. This avoids adding
	// duplicate OfferUserDetails entries to an ApplicationOffer.
	_, err = db.ApplicationOffers().UpdateAll(
		bson.D{{
			"_id", offerUUID,
		}},
		bson.D{{
			"$set", bson.D{{u.FieldName("users"), access}},
		}},
	)
	if err != nil {
		if errgo.Cause(err) == mgo.ErrNotFound {
			return errgo.WithCausef(err, params.ErrNotFound, "")
		}
		return errgo.Mask(err)
	}
	return nil
}

// GetApplicationOfferAccess returns the access level a given user has to
// the application offer with the given UUID.
func (db *Database) GetApplicationOfferAccess(ctx context.Context, user params.User, offerUUID string) (_ mongodoc.ApplicationOfferAccessPermission, err error) {
	defer db.checkError(ctx, &err)
	var offer mongodoc.ApplicationOffer
	err = db.ApplicationOffers().FindId(offerUUID).One(&offer)
	if err != nil && errgo.Cause(err) != mgo.ErrNotFound {
		return mongodoc.ApplicationOfferNoAccess, errgo.Mask(err)
	}
	return offer.Users[mongodoc.User(user)], nil
}

// An Iter is an iterator that gives access to database objects.
type Iter interface {
	Next(interface{}) bool
	Err() error
	Close() error
}

// IterApplicationOffers returns an Iter that will return all
// ApplicationOffers that the given user has at least the given level of
// access to and that pass any of the given filters. The returned Iter may
// panic if the Next method is called with anything other than a pointer
// to a mongodoc.ApplicationOffer.
func (db *Database) IterApplicationOffers(ctx context.Context, user params.User, access mongodoc.ApplicationOfferAccessPermission, filters []jujuparams.OfferFilter) Iter {
	q := make([]Query, 1, 2)
	q[0] = Or(Gte(mongodoc.User(user).FieldName("users"), access), Gte("users.everyone", access))

	filterQueries := make([]bson.D, len(filters))
	for i, f := range filters {
		filterQueries[i] = makeApplicationOfferFilterQuery(f)
	}
	if len(filterQueries) > 0 {
		q = append(q, Or(filterQueries...))
	}

	return db.ApplicationOffers().Find(And(q...)).Iter()
}

func makeApplicationOfferFilterQuery(filter jujuparams.OfferFilter) bson.D {
	query := make(bson.D, 0, 7)
	if filter.OwnerName != "" {
		query = append(query, bson.DocElem{"owner-name", filter.OwnerName})
	}
	if filter.ModelName != "" {
		query = append(query, bson.DocElem{"model-name", filter.ModelName})
	}
	if filter.ApplicationName != "" {
		query = append(query, bson.DocElem{"application-name", filter.ApplicationName})
	}
	// We match on partial names, eg "-sql"
	if filter.OfferName != "" {
		query = append(query, bson.DocElem{"offer-name", bson.D{{"$regex", fmt.Sprintf(".*%s.*", filter.OfferName)}}})
	}
	// We match descriptions by looking for containing terms.
	if filter.ApplicationDescription != "" {
		desc := regexp.QuoteMeta(filter.ApplicationDescription)
		query = append(query, bson.DocElem{"application-description", bson.D{{"$regex", fmt.Sprintf(".*%s.*", desc)}}})
	}

	if len(filter.Endpoints) > 0 {
		endpoints := make([]bson.D, 0, len(filter.Endpoints))
		for _, ep := range filter.Endpoints {
			match := make(bson.D, 0, 3)
			if ep.Interface != "" {
				match = append(match, bson.DocElem{"interface", ep.Interface})
			}
			if ep.Name != "" {
				match = append(match, bson.DocElem{"name", ep.Name})
			}
			if ep.Role != "" {
				match = append(match, bson.DocElem{"role", ep.Role})
			}
			if len(match) == 0 {
				continue
			}
			endpoints = append(endpoints, bson.D{{
				"endpoints", bson.D{{"$elemMatch", match}},
			}})
		}
		switch len(endpoints) {
		case 1:
			query = append(query, endpoints[0][0])
		default:
			query = append(query, bson.DocElem{"$or", endpoints})
		case 0:
		}
	}

	if len(filter.AllowedConsumerTags) > 0 {
		users := make([]bson.D, 0, len(filter.AllowedConsumerTags))
		for _, userTag := range filter.AllowedConsumerTags {
			user, err := conv.ParseUserTag(userTag)
			if err != nil {
				// If this user does not parse then it will never match
				// a record, add a query that can't match.
				users = append(users, bson.D{{"users.~~impossible", bson.D{{"$exists", true}}}})
				continue
			}

			users = append(users, bson.D{{
				mongodoc.User(user).FieldName("users"),
				bson.D{{"$gte", mongodoc.ApplicationOfferConsumeAccess}},
			}})
		}
		switch len(users) {
		case 1:
			query = append(query, users[0][0])
		default:
			query = append(query, bson.DocElem{"$or", users})
		case 0:
		}
	}

	return query
}

// CanReadIter returns an iterator that iterates over items in the given
// iterator, which must have been derived from db, returning only those
// that the currently logged in user has permission to see.
//
// The API matches that of mgo.Iter.
func (db *Database) NewCanReadIter(id identchecker.ACLIdentity, iter *mgo.Iter) *CanReadIter {
	return &CanReadIter{
		id:   id,
		iter: iter,
		db:   db,
	}
}

// CanReadIter represents an iterator that returns only items
// that the currently authenticated user has read access to.
type CanReadIter struct {
	id   identchecker.ACLIdentity
	db   *Database
	iter *mgo.Iter
	err  error
	n    int
}

// Next reads the next item from the iterator into the given
// item and returns whether it has done so.
func (iter *CanReadIter) Next(ctx context.Context, item auth.ACLEntity) bool {
	if iter.err != nil {
		return false
	}
	for iter.iter.Next(item) {
		iter.n++
		if err := auth.CheckCanRead(ctx, iter.id, item); err != nil {
			if errgo.Cause(err) == params.ErrUnauthorized {
				// No permissions to look at the entity, so don't include
				// it in the results.
				continue
			}
			iter.err = errgo.Mask(err)
			iter.iter.Close()
			return false
		}
		return true
	}
	return false
}

func (iter *CanReadIter) Close(ctx context.Context) error {
	iter.iter.Close()
	return iter.Err(ctx)
}

// Err returns any error encountered when iterating.
func (iter *CanReadIter) Err(ctx context.Context) error {
	if iter.err != nil {
		// If iter.err is set, it's because CheckCanRead
		// has failed, and that doesn't talk to mongo,
		// so no use in calling checkError in that case.
		return iter.err
	}
	err := iter.iter.Err()
	iter.db.checkError(ctx, &err)
	return err
}

// Count returns the total number of items traversed
// by the iterator, including items that were not returned
// because they were unauthorized.
func (iter *CanReadIter) Count() int {
	return iter.n
}

func (db *Database) Collections() []*mgo.Collection {
	return []*mgo.Collection{
		db.Audits(),
		db.Applications(),
		db.CloudRegions(),
		db.Controllers(),
		db.Credentials(),
		db.Macaroons(),
		db.Machines(),
		db.Models(),
		db.ApplicationOffers(),
	}
}

func (db *Database) Applications() *mgo.Collection {
	return db.C("applications")
}

func (db *Database) Audits() *mgo.Collection {
	return db.C("audits")
}

func (db *Database) CloudRegions() *mgo.Collection {
	return db.C("cloudregions")
}

func (db *Database) Controllers() *mgo.Collection {
	return db.C("controllers")
}

func (db *Database) Credentials() *mgo.Collection {
	return db.C("credentials")
}

func (db *Database) Macaroons() *mgo.Collection {
	return db.C("macaroons")
}

func (db *Database) Machines() *mgo.Collection {
	return db.C("machines")
}

func (db *Database) Models() *mgo.Collection {
	return db.C("models")
}

// ApplicationOffers returns the collection holding application offers.
func (db *Database) ApplicationOffers() *mgo.Collection {
	return db.C("application_offers")
}

func (db *Database) C(name string) *mgo.Collection {
	if db.Database == nil {
		panic(fmt.Sprintf("cannot get collection %q because JEM closed", name))
	}
	return db.Database.C(name)
}
