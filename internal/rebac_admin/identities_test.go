// Copyright 2024 Canonical Ltd.

package rebac_admin_test

import (
	"context"
	"testing"

	qt "github.com/frankban/quicktest"

	"github.com/canonical/jimm/v3/internal/common/pagination"
	"github.com/canonical/jimm/v3/internal/common/utils"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/jimmtest"
	"github.com/canonical/jimm/v3/internal/openfga"
	"github.com/canonical/jimm/v3/internal/rebac_admin"
	rebac_handlers "github.com/canonical/rebac-admin-ui-handlers/v1"
	"github.com/canonical/rebac-admin-ui-handlers/v1/resources"
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

	testCases := []struct {
		desc         string
		size         *int
		page         *int
		wantPage     int
		wantSize     int
		wantTotal    int
		wantNextpage *int
		emails       []string
	}{
		{
			desc:         "test with first page",
			size:         utils.IntToPointer(2),
			page:         utils.IntToPointer(0),
			wantPage:     0,
			wantSize:     2,
			wantNextpage: utils.IntToPointer(1),
			wantTotal:    len(testUsers),
			emails:       []string{testUsers[0].Name, testUsers[1].Name},
		},
		{
			desc:         "test with second page",
			size:         utils.IntToPointer(2),
			page:         utils.IntToPointer(1),
			wantPage:     1,
			wantSize:     2,
			wantNextpage: utils.IntToPointer(2),
			wantTotal:    len(testUsers),
			emails:       []string{testUsers[2].Name, testUsers[3].Name},
		},
		{
			desc:         "test with last page",
			size:         utils.IntToPointer(2),
			page:         utils.IntToPointer(2),
			wantPage:     2,
			wantSize:     1,
			wantNextpage: nil,
			wantTotal:    len(testUsers),
			emails:       []string{testUsers[4].Name},
		},
	}
	for _, t := range testCases {
		c.Run(t.desc, func(c *qt.C) {
			identities, err := identitySvc.ListIdentities(ctx, &resources.GetIdentitiesParams{
				Size: t.size,
				Page: t.page,
			})
			c.Assert(err, qt.IsNil)
			c.Assert(*identities.Meta.Page, qt.Equals, t.wantPage)
			c.Assert(identities.Meta.Size, qt.Equals, t.wantSize)
			if t.wantNextpage == nil {
				c.Assert(identities.Next.Page, qt.IsNil)
			} else {
				c.Assert(*identities.Next.Page, qt.Equals, *t.wantNextpage)
			}
			c.Assert(*identities.Meta.Total, qt.Equals, t.wantTotal)
			c.Assert(identities.Data, qt.HasLen, len(t.emails))
			for i := range len(t.emails) {
				c.Assert(identities.Data[i].Email, qt.Equals, t.emails[i])
			}
		})
	}

}
