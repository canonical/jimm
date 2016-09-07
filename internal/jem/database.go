// Copyright 2016 Canonical Ltd.

package jem

import (
	"fmt"
	"time"

	"gopkg.in/errgo.v1"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"

	"github.com/CanonicalLtd/jem/internal/mongodoc"
	"github.com/CanonicalLtd/jem/params"
)

// Database wraps an mgo.DB ands adds a number of methods for
// manipulating the database.
type Database struct {
	*mgo.Database
}

// Copy copies the Database and its underlying mgo session.
func (s Database) Copy() Database {
	return Database{
		&mgo.Database{
			Name:    s.Name,
			Session: s.Session.Copy(),
		},
	}
}

// Clone copies the Database and clones its underlying
// mgo session. See mgo.Session.Clone and mgo.Session.Copy
// for information on the distinction between Clone and Copy.
func (s Database) Clone() Database {
	if s.Session == nil {
		panic("nil session in clone!")
	}
	return Database{
		&mgo.Database{
			Name:    s.Name,
			Session: s.Session.Clone(),
		},
	}
}

// Close closes the database's underlying session.
func (db Database) Close() {
	db.Session.Close()
}

// AddController adds a new controller to the database. It returns an
// error with a params.ErrAlreadyExists cause if there is already a
// controller with the given name. The Id field in ctl will be set from
// its Path field.
func (db Database) AddController(ctl *mongodoc.Controller) error {
	ctl.Id = ctl.Path.String()
	err := db.Controllers().Insert(ctl)
	if err != nil {
		if mgo.IsDup(err) {
			return params.ErrAlreadyExists
		}
		return errgo.NoteMask(err, "cannot insert controller")
	}
	return nil
}

// DeleteController deletes existing controller and all of its
// associated models from the database. It returns an error if
// either deletion fails. If there is no matching controller then the
// error will have the cause params.ErrNotFound.
//
// Note that this operation is not atomic.
func (db Database) DeleteController(path params.EntityPath) error {
	// TODO (urosj) make this operation atomic.
	// Delete its models first.
	info, err := db.Models().RemoveAll(bson.D{{"controller", path}})
	if err != nil {
		return errgo.Notef(err, "error deleting controller models")
	}
	// Then delete the controller.
	err = db.Controllers().RemoveId(path.String())
	if err == mgo.ErrNotFound {
		return errgo.WithCausef(nil, params.ErrNotFound, "controller %q not found", path)
	}
	if err != nil {
		logger.Errorf("deleted %d controller models for model but could not delete controller: %v", info.Removed, err)
		return errgo.Notef(err, "cannot delete controller")
	}
	logger.Infof("deleted controller %v and %d associated models", path, info.Removed)
	return nil
}

// AddModel adds a new model to the database.
// It returns an error with a params.ErrAlreadyExists
// cause if there is already an model with the given name.
// If ignores m.Id and sets it from m.Path.
func (db Database) AddModel(m *mongodoc.Model) error {
	m.Id = m.Path.String()
	err := db.Models().Insert(m)
	if mgo.IsDup(err) {
		return errgo.WithCausef(nil, params.ErrAlreadyExists, "")
	}
	if err != nil {
		return errgo.Notef(err, "cannot insert controller model")
	}
	return nil
}

// DeleteModel deletes an model from the database. If an
// model is also a controller it will not be deleted and an error
// with a cause of params.ErrForbidden will be returned. If the
// model cannot be found then an error with a cause of
// params.ErrNotFound is returned.
func (db Database) DeleteModel(path params.EntityPath) error {
	// TODO when we monitor model health, prohibit this method
	// and delete the model automatically when it is destroyed.
	// Check if model is also a controller.
	err := db.Models().RemoveId(path.String())
	if err == mgo.ErrNotFound {
		return errgo.WithCausef(nil, params.ErrNotFound, "model %q not found", path)
	}
	if err != nil {
		return errgo.Notef(err, "could not delete model")
	}
	logger.Infof("deleted model %s", path)
	return nil
}

// Controller returns information on the controller with the given
// path. It returns an error with a params.ErrNotFound cause if the
// controller was not found.
func (db Database) Controller(path params.EntityPath) (*mongodoc.Controller, error) {
	var ctl mongodoc.Controller
	id := path.String()
	err := db.Controllers().FindId(id).One(&ctl)
	if err == mgo.ErrNotFound {
		return nil, errgo.WithCausef(nil, params.ErrNotFound, "controller %q not found", id)
	}
	if err != nil {
		return nil, errgo.Notef(err, "cannot get controller %q", id)
	}
	return &ctl, nil
}

// Model returns information on the model with the given
// path. It returns an error with a params.ErrNotFound cause if the
// controller was not found.
func (db Database) Model(path params.EntityPath) (*mongodoc.Model, error) {
	id := path.String()
	var m mongodoc.Model
	err := db.Models().FindId(id).One(&m)
	if err == mgo.ErrNotFound {
		return nil, errgo.WithCausef(nil, params.ErrNotFound, "model %q not found", id)
	}
	if err != nil {
		return nil, errgo.Notef(err, "cannot get model %q", id)
	}
	return &m, nil
}

// ModelFromUUID returns the document representing the model with the
// given UUID. It returns an error with a params.ErrNotFound cause if the
// controller was not found.
func (db Database) ModelFromUUID(uuid string) (*mongodoc.Model, error) {
	var m mongodoc.Model
	err := db.Models().Find(bson.D{{"uuid", uuid}}).One(&m)
	if err == mgo.ErrNotFound {
		return nil, errgo.WithCausef(nil, params.ErrNotFound, "model %q not found", uuid)
	}
	if err != nil {
		return nil, errgo.Notef(err, "cannot get model %q", uuid)
	}
	return &m, nil
}

// controllerLocationQuery returns a mongo query that iterates through
// all the public controllers matching the given location attributes,
// including unavailable controllers only if includeUnavailable is true.
// It returns an error if the location attribute keys aren't valid.
func (db Database) controllerLocationQuery(cloud params.Cloud, region string, includeUnavailable bool) (*mgo.Query, error) {
	q := make(bson.D, 0, 4)
	if cloud != "" {
		q = append(q, bson.DocElem{"cloud.name", cloud})
	}
	if region != "" {
		q = append(q, bson.DocElem{"cloud.regions", bson.D{{"$elemMatch", bson.D{{"name", region}}}}})
	}
	q = append(q, bson.DocElem{"public", true})
	if !includeUnavailable {
		q = append(q, bson.DocElem{"unavailablesince", notExistsQuery})
	}
	return db.Controllers().Find(q), nil
}

// SetControllerAvailable marks the given controller as available.
// This method does not return an error when the controller doesn't exist.
func (db Database) SetControllerAvailable(ctlPath params.EntityPath) error {
	if err := db.Controllers().UpdateId(ctlPath.String(), bson.D{{
		"$unset", bson.D{{"unavailablesince", nil}},
	}}); err != nil {
		if err == mgo.ErrNotFound {
			// For symmetry with SetControllerUnavailableAt.
			return nil
		}
		return errgo.Notef(err, "cannot update %v", ctlPath)
	}
	return nil
}

// SetControllerUnavailableAt marks the controller as having been unavailable
// since at least the given time. If the controller was already marked
// as unavailable, its time isn't changed.
// This method does not return an error when the controller doesn't exist.
func (db Database) SetControllerUnavailableAt(ctlPath params.EntityPath, t time.Time) error {
	err := db.Controllers().Update(
		bson.D{
			{"_id", ctlPath.String()},
			{"unavailablesince", notExistsQuery},
		},
		bson.D{
			{"$set", bson.D{{"unavailablesince", t}}},
		},
	)
	if err == nil {
		return nil
	}
	if err == mgo.ErrNotFound {
		// We don't know whether the not-found error is because there
		// are no controllers with the given name (in which case we want
		// to return a params.ErrNotFound error) or because there was
		// one but it is already unavailable.
		// We could fetch the controller to decide whether it's actually there
		// or not, but because in practice we don't care if we're setting
		// controller-unavailable on a non-existent controller, we'll
		// save the round trip.
		return nil
	}
	return errgo.Notef(err, "cannot update controller")
}

// ErrLeaseUnavailable is the error cause returned by AcquireMonitorLease
// when it cannot acquire the lease because it is unavailable.
var ErrLeaseUnavailable = errgo.Newf("cannot acquire lease")

// AcquireMonitorLease acquires or renews the lease on a controller.
// The lease will only be changed if the lease in the database
// has the given old expiry time and owner.
// When acquired, the lease will have the given new owner
// and expiration time.
//
// If newOwner is empty, the lease will be dropped, the
// returned time will be zero and newExpiry will be ignored.
//
// If the controller has been removed, an error with a params.ErrNotFound
// cause will be returned. If the lease has been obtained by someone else
// an error with a ErrLeaseUnavailable cause will be returned.
func (db Database) AcquireMonitorLease(ctlPath params.EntityPath, oldExpiry time.Time, oldOwner string, newExpiry time.Time, newOwner string) (time.Time, error) {
	var update bson.D
	if newOwner != "" {
		newExpiry = mongodoc.Time(newExpiry)
		update = bson.D{{"$set", bson.D{
			{"monitorleaseexpiry", newExpiry},
			{"monitorleaseowner", newOwner},
		}}}
	} else {
		newExpiry = time.Time{}
		update = bson.D{{"$unset", bson.D{
			{"monitorleaseexpiry", nil},
			{"monitorleaseowner", nil},
		}}}
	}
	var oldOwnerQuery interface{}
	var oldExpiryQuery interface{}
	if oldOwner == "" {
		oldOwnerQuery = notExistsQuery
	} else {
		oldOwnerQuery = oldOwner
	}
	if oldExpiry.IsZero() {
		oldExpiryQuery = notExistsQuery
	} else {
		oldExpiryQuery = oldExpiry
	}
	err := db.Controllers().Update(bson.D{
		{"path", ctlPath},
		{"monitorleaseexpiry", oldExpiryQuery},
		{"monitorleaseowner", oldOwnerQuery},
	}, update)
	if err == mgo.ErrNotFound {
		// Someone else got there first, or the document has been
		// removed. Technically don't need to distinguish between the
		// two cases, but it's useful to see the different error messages.
		ctl, err := db.Controller(ctlPath)
		if errgo.Cause(err) == params.ErrNotFound {
			return time.Time{}, errgo.WithCausef(nil, params.ErrNotFound, "controller removed")
		}
		if err != nil {
			return time.Time{}, errgo.Mask(err)
		}
		return time.Time{}, errgo.WithCausef(nil, ErrLeaseUnavailable, "controller has lease taken out by %q expiring at %v", ctl.MonitorLeaseOwner, ctl.MonitorLeaseExpiry.UTC())
	}
	if err != nil {
		return time.Time{}, errgo.Notef(err, "cannot acquire lease")
	}
	return newExpiry, nil
}

// SetControllerStats sets the stats associated with the controller
// with the given path. It returns an error with a params.ErrNotFound
// cause if the controller does not exist.
func (db Database) SetControllerStats(ctlPath params.EntityPath, stats *mongodoc.ControllerStats) error {
	err := db.Controllers().UpdateId(
		ctlPath.String(),
		bson.D{{"$set", bson.D{{"stats", stats}}}},
	)
	if err == mgo.ErrNotFound {
		return errgo.WithCausef(nil, params.ErrNotFound, "controller not found")
	}
	return errgo.Mask(err)
}

// SetModelLife sets the Life field of all models controlled
// by the given controller that have the given UUID.
// It does not return an error if there are no such models.
func (db Database) SetModelLife(ctlPath params.EntityPath, uuid string, life string) error {
	_, err := db.Models().UpdateAll(
		bson.D{{"uuid", uuid}, {"controller", ctlPath}},
		bson.D{{"$set", bson.D{{"life", life}}}},
	)
	if err != nil {
		return errgo.Notef(err, "cannot update model")
	}
	return nil
}

// updateCredential stores the given credential in the database. If a
// credential with the same name exists it is overwritten.
func (db Database) updateCredential(cred *mongodoc.Credential) error {
	update := bson.D{{
		"type", cred.Type,
	}, {
		"label", cred.Label,
	}, {
		"attributes", cred.Attributes,
	}}
	if len(cred.ACL.Read) > 0 {
		update = append(update, bson.DocElem{"acl", cred.ACL})
	}
	id := credentialId(cred.User, cred.Cloud, cred.Name)
	_, err := db.Credentials().UpsertId(id, bson.D{{
		"$set", update,
	}, {
		"$setOnInsert", bson.D{{
			"user", cred.User,
		}, {
			"cloud", cred.Cloud,
		}, {
			"name", cred.Name,
		}},
	}})
	if err != nil {
		return errgo.Mask(err)
	}
	return nil
}

// Credential gets the specified credential. If the credential cannot be
// found the returned error will have a cause of params.ErrNotFound.
func (db Database) Credential(user params.User, cloud params.Cloud, name params.Name) (*mongodoc.Credential, error) {
	var cred mongodoc.Credential
	id := credentialId(user, cloud, name)
	err := db.Credentials().FindId(id).One(&cred)
	if err == mgo.ErrNotFound {
		return nil, errgo.WithCausef(nil, params.ErrNotFound, "credential %q not found", id)
	}
	if err != nil {
		return nil, errgo.Notef(err, "cannot get credential %q", id)
	}
	return &cred, nil
}

// credentialId calculates the id for a credential with the specified
// user, cloud and name.
func credentialId(user params.User, cloud params.Cloud, name params.Name) string {
	return fmt.Sprintf("%s/%s/%s", user, cloud, name)
}

// credentialAddController stores the fact that the credential with the
// given user, cloud and name is present on the given controller.
func (db Database) credentialAddController(user params.User, cloud params.Cloud, name params.Name, controller params.EntityPath) error {
	id := credentialId(user, cloud, name)
	err := db.Credentials().UpdateId(id, bson.D{{
		"$addToSet", bson.D{{"controllers", controller}},
	}})
	if err != nil {
		if err == mgo.ErrNotFound {
			return errgo.WithCausef(nil, params.ErrNotFound, "credential %q not found", id)
		}
		return errgo.Notef(err, "cannot update credential %q", id)
	}
	return nil
}

// credentialRemoveController stores the fact that the credential with
// the given user, cloud and name is not present on the given controller.
func (db Database) credentialRemoveController(user params.User, cloud params.Cloud, name params.Name, controller params.EntityPath) error {
	id := credentialId(user, cloud, name)
	err := db.Credentials().UpdateId(id, bson.D{{
		"$pull", bson.D{{"controllers", controller}},
	}})
	if err != nil {
		if err == mgo.ErrNotFound {
			return errgo.WithCausef(nil, params.ErrNotFound, "credential %q not found", id)
		}
		return errgo.Notef(err, "cannot update credential %q", id)
	}
	return nil
}

// Cloud gets the details of the given cloud.
//
// Note that there may be many controllers with the given cloud name. We
// return an arbitrary choice, assuming that cloud definitions are the
// same across all possible controllers.
func (db Database) Cloud(cloud params.Cloud) (*mongodoc.Cloud, error) {
	var ctl mongodoc.Controller
	err := db.Controllers().Find(bson.D{{"cloud.name", cloud}}).One(&ctl)
	if err == mgo.ErrNotFound {
		return nil, errgo.WithCausef(nil, params.ErrNotFound, "cloud %q not found", cloud)
	}
	if err != nil {
		return nil, errgo.Notef(err, "cannot get cloud %q", cloud)
	}
	return &ctl.Cloud, nil
}

func (db Database) Collections() []*mgo.Collection {
	return []*mgo.Collection{
		db.Macaroons(),
		db.Controllers(),
		db.Models(),
		db.Credentials(),
	}
}

func (db Database) Macaroons() *mgo.Collection {
	return db.C("macaroons")
}

func (db Database) Controllers() *mgo.Collection {
	return db.C("controllers")
}

func (db Database) Models() *mgo.Collection {
	return db.C("models")
}

func (db Database) Credentials() *mgo.Collection {
	return db.C("credentials")
}

func (db Database) C(name string) *mgo.Collection {
	if db.Database == nil {
		panic(fmt.Sprintf("cannot get collection %q because JEM closed", name))
	}
	return db.Database.C(name)
}

// SetACL sets the ACL for the path document in c to be equal to acl.
func SetACL(c *mgo.Collection, path params.EntityPath, acl params.ACL) error {
	err := c.UpdateId(path.String(), bson.D{{"$set", bson.D{{"acl", acl}}}})
	if err == nil {
		return nil
	}
	if err == mgo.ErrNotFound {
		return errgo.WithCausef(nil, params.ErrNotFound, "%q not found", path)
	}
	return errgo.Notef(err, "cannot update ACL on %q", path)
}

// Grant updates the ACL for the path document in c to include user.
func Grant(c *mgo.Collection, path params.EntityPath, user params.User) error {
	err := c.UpdateId(path.String(), bson.D{{"$addToSet", bson.D{{"acl.read", user}}}})
	if err == nil {
		return nil
	}
	if err == mgo.ErrNotFound {
		return errgo.WithCausef(nil, params.ErrNotFound, "%q not found", path)
	}
	return errgo.Notef(err, "cannot update ACL on %q", path)
}

// Revoke updates the ACL for the path document in c to not include user.
func Revoke(c *mgo.Collection, path params.EntityPath, user params.User) error {
	err := c.UpdateId(path.String(), bson.D{{"$pull", bson.D{{"acl.read", user}}}})
	if err == nil {
		return nil
	}
	if err == mgo.ErrNotFound {
		return errgo.WithCausef(nil, params.ErrNotFound, "%q not found", path)
	}
	return errgo.Notef(err, "cannot update ACL on %q", path)
}
