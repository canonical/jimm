// Copyright 2021 Canonical Ltd.

package db_test

import (
	"bytes"
	"context"
	"fmt"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery/dbrootkeystore"

	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/errors"
)

var _ dbrootkeystore.Backing = (*db.Database)(nil)

func TestGetKeyUnconfiguredDatabase(t *testing.T) {
	c := qt.New(t)

	var d db.Database
	_, err := d.GetKey([]byte("test-id"))
	c.Assert(err, qt.Equals, bakery.ErrNotFound)
}

func TestInsertKeyUnconfiguredDatabase(t *testing.T) {
	c := qt.New(t)

	var d db.Database
	rk := dbrootkeystore.RootKey{
		Id:      []byte("test-id"),
		Created: time.Now().UTC().Round(time.Millisecond),
		Expires: time.Now().UTC().Round(time.Millisecond).Add(24 * time.Hour),
		RootKey: []byte("very secret"),
	}
	err := d.InsertKey(rk)
	c.Assert(errors.ErrorCode(err), qt.Equals, errors.CodeServerConfiguration)
}

func (s *dbSuite) TestInsertKeyGetKey(c *qt.C) {
	err := s.Database.Migrate(context.Background(), false)
	c.Assert(err, qt.IsNil)

	_, err = s.Database.GetKey([]byte("test-id"))
	c.Assert(err, qt.Equals, bakery.ErrNotFound)

	rk := dbrootkeystore.RootKey{
		Id:      []byte("test-id"),
		Created: time.Now().UTC().Round(time.Millisecond),
		Expires: time.Now().UTC().Round(time.Millisecond).Add(24 * time.Hour),
		RootKey: []byte("very secret"),
	}

	s.Database.InsertKey(rk)

	rk2, err := s.Database.GetKey([]byte("test-id"))
	c.Assert(err, qt.IsNil)
	c.Check(rk2, qt.DeepEquals, rk)
}

func TestFindLatestKeyUnconfiguredDatabase(t *testing.T) {
	c := qt.New(t)

	var d db.Database
	now := time.Now().UTC().Round(time.Millisecond)
	rk, err := d.FindLatestKey(now.Add(-24*time.Hour), now.Add(24*time.Hour), now.Add(48*time.Hour))
	c.Assert(err, qt.DeepEquals, bakery.ErrNotFound)
	c.Check(rk, qt.DeepEquals, dbrootkeystore.RootKey{})
}

func (s *dbSuite) TestFindLatestKey(c *qt.C) {
	err := s.Database.Migrate(context.Background(), false)
	c.Assert(err, qt.IsNil)

	now := time.Now().UTC().Round(time.Millisecond)

	// An empty database shouldn't find any keys.
	rk, err := s.Database.FindLatestKey(now.Add(-24*time.Hour), now.Add(24*time.Hour), now.Add(48*time.Hour))
	c.Assert(err, qt.IsNil)
	c.Check(rk, qt.DeepEquals, dbrootkeystore.RootKey{})

	// Add some keys
	rks := make([]dbrootkeystore.RootKey, 10)
	for i := 0; i < 10; i++ {
		var name, key bytes.Buffer
		fmt.Fprintf(&name, "test-%d", i)
		fmt.Fprintf(&key, "secret %d", i)
		rks[i] = dbrootkeystore.RootKey{
			Id:      name.Bytes(),
			Created: now.Add(time.Duration(i-10) * time.Hour),
			Expires: now.Add(time.Duration(i-10+24) * time.Hour),
			RootKey: key.Bytes(),
		}
		err := s.Database.InsertKey(rks[i])
		c.Assert(err, qt.IsNil)
	}

	rk, err = s.Database.FindLatestKey(now.Add(-5*time.Hour), now.Add((24-5)*time.Hour), now.Add((24-2)*time.Hour))
	c.Assert(err, qt.IsNil)
	c.Check(rk, qt.DeepEquals, rks[8])
}
