// Copyright 2024 Canonical.

package jimm

import (
	"context"

	"github.com/canonical/jimm/v3/internal/common/pagination"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/openfga"
)

// FetchIdentity fetches the user specified by the username and returns the user if it is found.
// Or error "record not found".
func (j *JIMM) FetchIdentity(ctx context.Context, id string) (*openfga.User, error) {
	const op = errors.Op("jimm.FetchIdentity")

	identity, err := dbmodel.NewIdentity(id)
	if err != nil {
		return nil, errors.E(op, err)
	}

	if err := j.Database.FetchIdentity(ctx, identity); err != nil {
		return nil, err
	}
	u := openfga.NewUser(identity, j.OpenFGAClient)

	return u, nil
}

// ListIdentities lists a page of users in our database and parse them into openfga entities.
// `match` will filter the list for fuzzy find on identity name.
func (j *JIMM) ListIdentities(ctx context.Context, user *openfga.User, pagination pagination.LimitOffsetPagination, match string) ([]openfga.User, error) {
	const op = errors.Op("jimm.ListIdentities")

	if !user.JimmAdmin {
		return nil, errors.E(op, errors.CodeUnauthorized, "unauthorized")
	}
	identities, err := j.Database.ListIdentities(ctx, pagination.Limit(), pagination.Offset(), match)
	var users []openfga.User

	for _, id := range identities {
		users = append(users, *openfga.NewUser(&id, j.OpenFGAClient))
	}
	if err != nil {
		return nil, errors.E(op, err)
	}
	return users, nil
}

// CountIdentities returns the count of all the identities in our database.
func (j *JIMM) CountIdentities(ctx context.Context, user *openfga.User) (int, error) {
	const op = errors.Op("jimm.CountIdentities")

	if !user.JimmAdmin {
		return 0, errors.E(op, errors.CodeUnauthorized, "unauthorized")
	}

	count, err := j.Database.CountIdentities(ctx)
	if err != nil {
		return 0, errors.E(op, err)
	}
	return count, nil
}
