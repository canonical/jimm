// Copyright 20202 Canonical Ltd.

package mongodoc_test

import (
	"testing"

	qt "github.com/frankban/quicktest"
	"github.com/juju/mgo/v2/bson"

	"github.com/canonical/jimm/internal/mongodoc"
)

func TestApplicationOfferAccessMapRoundTrip(t *testing.T) {
	c := qt.New(t)

	m0 := mongodoc.ApplicationOfferAccessMap{
		"alice":     mongodoc.ApplicationOfferReadAccess,
		"bob":       mongodoc.ApplicationOfferAdminAccess,
		"test.user": mongodoc.ApplicationOfferConsumeAccess,
		"test~user": mongodoc.ApplicationOfferReadAccess,
		"test$user": mongodoc.ApplicationOfferAdminAccess,
		"~0~1~2":    mongodoc.ApplicationOfferAdminAccess,
	}
	var o0, o1 struct {
		M mongodoc.ApplicationOfferAccessMap
	}
	o0.M = m0
	buf, err := bson.Marshal(o0)
	c.Assert(err, qt.IsNil)

	err = bson.Unmarshal(buf, &o1)
	c.Assert(err, qt.IsNil)
	c.Check(o1.M, qt.DeepEquals, m0)
}

func TestUserRoundTrip(t *testing.T) {
	c := qt.New(t)

	u0 := mongodoc.User("~=~1$=~2.=~3")
	var o0, o1 struct {
		U mongodoc.User
	}
	o0.U = u0
	buf, err := bson.Marshal(o0)
	c.Assert(err, qt.IsNil)

	err = bson.Unmarshal(buf, &o1)
	c.Assert(err, qt.IsNil)
	c.Check(o1.U, qt.Equals, u0)
}
