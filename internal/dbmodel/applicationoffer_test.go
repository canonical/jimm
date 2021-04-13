// Copyright 2021 Canonical Ltd.

package dbmodel_test

import (
	"testing"

	qt "github.com/frankban/quicktest"
	"github.com/juju/names/v4"

	"github.com/CanonicalLtd/jimm/internal/dbmodel"
)

func TestApplicationOfferTag(t *testing.T) {
	c := qt.New(t)

	ao := dbmodel.ApplicationOffer{
		UUID: "00000003-0000-0000-0000-0000-000000000001",
	}

	tag := ao.Tag()
	c.Check(tag.String(), qt.Equals, "applicationoffer-00000003-0000-0000-0000-0000-000000000001")

	var ao2 dbmodel.ApplicationOffer
	ao2.SetTag(tag.(names.ApplicationOfferTag))
	c.Check(ao2, qt.DeepEquals, ao)
}
