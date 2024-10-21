// Copyright 2024 Canonical.
package jimm_test

import (
	"context"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
	"github.com/google/uuid"

	"github.com/canonical/jimm/v3/internal/common/pagination"
	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/jimm"
	"github.com/canonical/jimm/v3/internal/openfga"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
)

func TestGetResources(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()
	ofgaClient, _, _, err := jimmtest.SetupTestOFGAClient(c.Name())
	c.Assert(err, qt.IsNil)

	now := time.Now().UTC().Round(time.Millisecond)
	j := &jimm.JIMM{
		UUID: uuid.NewString(),
		Database: db.Database{
			DB: jimmtest.PostgresDB(c, func() time.Time { return now }),
		},
		OpenFGAClient: ofgaClient,
	}

	err = j.Database.Migrate(ctx, false)
	c.Assert(err, qt.IsNil)
	_, _, controller, model, applicationOffer, cloud, _ := createTestControllerEnvironment(ctx, c, j.Database)

	ids := []string{applicationOffer.UUID, cloud.Name, controller.UUID, model.UUID.String}

	u := openfga.NewUser(&dbmodel.Identity{Name: "admin@canonical.com"}, ofgaClient)
	u.JimmAdmin = true

	testCases := []struct {
		desc       string
		limit      int
		offset     int
		identities []string
	}{
		{
			desc:       "test with first resources",
			limit:      3,
			offset:     0,
			identities: []string{ids[0], ids[1], ids[2]},
		},
		{
			desc:       "test with remianing ids",
			limit:      3,
			offset:     3,
			identities: []string{ids[3]},
		},
		{
			desc:       "test out of range",
			limit:      3,
			offset:     6,
			identities: []string{},
		},
	}
	for _, t := range testCases {
		c.Run(t.desc, func(c *qt.C) {
			filter := pagination.NewOffsetFilter(t.limit, t.offset)
			resources, err := j.ListResources(ctx, u, filter, "", "")
			c.Assert(err, qt.IsNil)
			c.Assert(resources, qt.HasLen, len(t.identities))
			for i := range len(t.identities) {
				c.Assert(resources[i].ID.String, qt.Equals, t.identities[i])
			}
		})
	}
}
