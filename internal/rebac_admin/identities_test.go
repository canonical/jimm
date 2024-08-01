// Copyright 2024 Canonical Ltd.

package rebac_admin_test

import (
	"context"
	"testing"

	"github.com/canonical/jimm/internal/common/pagination"
	"github.com/canonical/jimm/internal/dbmodel"
	"github.com/canonical/jimm/internal/jimmtest"
	"github.com/canonical/jimm/internal/openfga"
	"github.com/canonical/jimm/internal/rebac_admin"
	rebac_handlers "github.com/canonical/rebac-admin-ui-handlers/v1"
	"github.com/canonical/rebac-admin-ui-handlers/v1/resources"
	qt "github.com/frankban/quicktest"
)

func TestGetIdentity(t *testing.T) {
	c := qt.New(t)
	jimm := jimmtest.JIMM{
		FetchIdentity_: func(ctx context.Context, username string) (*openfga.User, error) {
			if username == "bob@canonical.com" {
				return openfga.NewUser(&dbmodel.Identity{Name: "bob@canonical.com"}, nil), nil
			}
			return nil, dbmodel.IdentityCreationError
		},
	}
	user := openfga.User{}
	user.JimmAdmin = true
	ctx := context.Background()
	ctx = rebac_handlers.ContextWithIdentity(ctx, &user)
	identitySvc := rebac_admin.NewidentitiesService(&jimm)

	// test with user found
	identity, err := identitySvc.GetIdentity(ctx, "bob@canonical.com")
	c.Assert(err, qt.IsNil)
	c.Assert(identity.Email, qt.Equals, "bob@canonical.com")

	// test with user not found
	_, err = identitySvc.GetIdentity(ctx, "bob-not-found@canonical.com")
	c.Assert(err, qt.ErrorMatches, "Not Found: User with id bob-not-found@canonical.com not found")
}

func TestListIdentities(t *testing.T) {
	testUsers := []openfga.User{
		*openfga.NewUser(&dbmodel.Identity{Name: "bob0@canonical.com"}, nil),
		*openfga.NewUser(&dbmodel.Identity{Name: "bob1@canonical.com"}, nil),
		*openfga.NewUser(&dbmodel.Identity{Name: "bob2@canonical.com"}, nil),
		*openfga.NewUser(&dbmodel.Identity{Name: "bob3@canonical.com"}, nil),
		*openfga.NewUser(&dbmodel.Identity{Name: "bob4@canonical.com"}, nil),
	}
	c := qt.New(t)
	jimm := jimmtest.JIMM{
		ListIdentities_: func(ctx context.Context, user *openfga.User, filter pagination.LimitOffsetPagination) ([]openfga.User, error) {
			start := filter.Offset()
			end := start + filter.Limit()
			if end > len(testUsers) {
				end = len(testUsers)
			}
			return testUsers[start:end], nil
		},
		CountIdentities_: func(ctx context.Context, user *openfga.User) (int, error) {
			return len(testUsers), nil
		},
	}
	user := openfga.User{}
	user.JimmAdmin = true
	ctx := context.Background()
	ctx = rebac_handlers.ContextWithIdentity(ctx, &user)
	identitySvc := rebac_admin.NewidentitiesService(&jimm)

	// test with first page
	size := 2
	page := 0
	identities, err := identitySvc.ListIdentities(ctx, &resources.GetIdentitiesParams{
		Size: &size,
		Page: &page,
	})
	c.Assert(err, qt.IsNil)
	c.Assert(*identities.Meta.Page, qt.Equals, 0)
	c.Assert(identities.Meta.Size, qt.Equals, 2)
	c.Assert(*identities.Next.Page, qt.Equals, 1)
	c.Assert(*identities.Meta.Total, qt.Equals, len(testUsers))
	c.Assert(identities.Data[0].Email, qt.Equals, testUsers[0].Name)
	c.Assert(identities.Data[1].Email, qt.Equals, testUsers[1].Name)

	// test with second page
	size = 2
	page = 1
	identities, err = identitySvc.ListIdentities(ctx, &resources.GetIdentitiesParams{
		Size: &size,
		Page: &page,
	})
	c.Assert(err, qt.IsNil)
	c.Assert(*identities.Meta.Page, qt.Equals, 1)
	c.Assert(identities.Meta.Size, qt.Equals, 2)
	c.Assert(*identities.Next.Page, qt.Equals, 2)
	c.Assert(*identities.Meta.Total, qt.Equals, len(testUsers))
	c.Assert(identities.Data[0].Email, qt.Equals, testUsers[2].Name)
	c.Assert(identities.Data[1].Email, qt.Equals, testUsers[3].Name)

	// test with last page
	size = 2
	page = 2
	identities, err = identitySvc.ListIdentities(ctx, &resources.GetIdentitiesParams{
		Size: &size,
		Page: &page,
	})
	c.Assert(err, qt.IsNil)
	c.Assert(*identities.Meta.Page, qt.Equals, 2)
	c.Assert(identities.Meta.Size, qt.Equals, 1)
	c.Assert(identities.Next.Page, qt.IsNil)
	c.Assert(*identities.Meta.Total, qt.Equals, len(testUsers))
	c.Assert(identities.Data[0].Email, qt.Equals, testUsers[4].Name)

}
