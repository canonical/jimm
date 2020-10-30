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

// UpsertMachine inserts, or updates, a machine in the database. Machines
// are matched on a combination of controller, model-uuid, and machine-id.
// If the given machine does not specify enough information to identify it
// an error with a cause of params.ErrNotFound is returned.
func (db *Database) UpsertMachine(ctx context.Context, m *mongodoc.Machine) (err error) {
	defer db.checkError(ctx, &err)

	q := machineQuery(m)
	if q == nil {
		return errgo.WithCausef(nil, params.ErrNotFound, "machine not found")
	}
	u := new(Update)
	u.SetOnInsert("_id", m.Controller+" "+m.Info.ModelUUID+" "+m.Info.Id)
	u.Set("controller", m.Controller)
	u.Set("cloud", m.Cloud)
	u.Set("region", m.Region)
	u.Set("info", m.Info)

	zapctx.Debug(ctx, "UpsertMachine", zaputil.BSON("q", q), zaputil.BSON("u", u))
	_, err = db.Machines().Find(q).Apply(mgo.Change{Update: u, Upsert: true, ReturnNew: true}, m)
	return errgo.Mask(err)
}

// ForEachMachine iterates through every machine that matches the given
// query, calling the given function with each machine. If a sort is
// specified then the machines will iterate in the sorted order. If the
// function returns an error the iterator stops immediately and the error
// is retuned with the cause unmasked.
func (db *Database) ForEachMachine(ctx context.Context, q Query, sort []string, f func(*mongodoc.Machine) error) (err error) {
	defer db.checkError(ctx, &err)

	query := db.Machines().Find(q)
	if len(sort) > 0 {
		query = query.Sort(sort...)
	}
	zapctx.Debug(ctx, "ForEachMachine", zaputil.BSON("q", q), zap.Strings("sort", sort))
	it := query.Iter()
	defer it.Close()
	var machine mongodoc.Machine
	for it.Next(&machine) {
		if err := f(&machine); err != nil {
			return errgo.Mask(err, errgo.Any)
		}
	}
	if err := it.Err(); err != nil {
		return errgo.Notef(err, "cannot iterate machines")
	}
	return nil
}

// RemoveMachine removes the given machine from the database. The machine
// is matched using the same criteria as in UpsertMachine. If a matching
// machine cannot be found an error with a cause of params.ErrNotFound is
// returned.
func (db *Database) RemoveMachine(ctx context.Context, m *mongodoc.Machine) (err error) {
	defer db.checkError(ctx, &err)
	q := machineQuery(m)
	if q == nil {
		return errgo.WithCausef(nil, params.ErrNotFound, "machine not found")
	}
	zapctx.Debug(ctx, "RemoveMachine", zaputil.BSON("q", q))
	_, err = db.Machines().Find(q).Apply(mgo.Change{Remove: true}, m)
	if err == mgo.ErrNotFound {
		return errgo.WithCausef(nil, params.ErrNotFound, "machine not found")
	}
	return errgo.Mask(err)
}

// RemoveMachines removes all the machines that match the given query. It
// is not an error if no machines match and therefore nothing is removed.
func (db *Database) RemoveMachines(ctx context.Context, q Query) (count int, err error) {
	defer db.checkError(ctx, &err)
	zapctx.Debug(ctx, "RemoveMachines", zaputil.BSON("q", q))
	info, err := db.Machines().RemoveAll(q)
	if err != nil {
		return 0, errgo.Notef(err, "cannot remove machines")
	}
	return info.Removed, nil
}

func machineQuery(m *mongodoc.Machine) Query {
	if m.Controller == "" || m.Info == nil || m.Info.ModelUUID == "" || m.Info.Id == "" {
		return nil
	}
	return Eq("_id", m.Controller+" "+m.Info.ModelUUID+" "+m.Info.Id)
}
