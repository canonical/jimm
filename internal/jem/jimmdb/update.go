// Copyright 2020 Canonical Ltd.

package jimmdb

import "gopkg.in/mgo.v2/bson"

// An Update object is used to perform updates to documents in the
// database.
type Update struct {
	AddToSet_    bson.D `bson:"$addToSet,omitempty"`
	Pull_        bson.D `bson:"$pull,omitempty"`
	Set_         bson.M `bson:"$set,omitempty"`
	SetOnInsert_ bson.M `bson:"$setOnInsert,omitempty"`
	Unset_       bson.M `bson:"$unset,omitempty"`
}

// IsZero returns true if this update object is empty, and would therefore
// not make any changes.
func (u *Update) IsZero() bool {
	return len(u.AddToSet_) == 0 && len(u.Pull_) == 0 && len(u.Set_) == 0 &&
		len(u.SetOnInsert_) == 0 && len(u.Unset_) == 0
}

// AddToSet adds a new $addToSet operation to the update. This will push
// the given value into the given field unless it is already present.
func (u *Update) AddToSet(field string, value interface{}) *Update {
	u.AddToSet_ = append(u.AddToSet_, bson.DocElem{Name: field, Value: value})
	return u
}

// Pull adds a new $pull operation to the update. This will pull all
// instances of the given value from the given field.
func (u *Update) Pull(field string, value interface{}) *Update {
	u.Pull_ = append(u.Pull_, bson.DocElem{Name: field, Value: value})
	return u
}

// Set adds a new $set operation to the update. This will set the given
// field to the given value.
func (u *Update) Set(field string, value interface{}) *Update {
	if u.Set_ == nil {
		u.Set_ = make(bson.M)
	}
	u.Set_[field] = value
	return u
}

// SetOnInsert adds a new $setOnInsert operation to the update. This will
// set the given field to the given value when a new document is being
// created as part of an upsert operation.
func (u *Update) SetOnInsert(field string, value interface{}) *Update {
	if u.SetOnInsert_ == nil {
		u.SetOnInsert_ = make(bson.M)
	}
	u.SetOnInsert_[field] = value
	return u
}

// Unset adds a new $unset operation to the update. This will remove the
// given field from the document.
func (u *Update) Unset(field string) *Update {
	if u.Unset_ == nil {
		u.Unset_ = make(bson.M)
	}
	u.Unset_[field] = nil
	return u
}
