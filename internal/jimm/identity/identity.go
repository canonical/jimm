// Copyright 2025 Canonical.

package identity

import (
	"context"

	"github.com/canonical/jimm/v3/internal/common/pagination"
	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/openfga"
)

// IdentityManager provides a means to manage identities within JIMM.
type IdentityManager struct {
	store   *db.Database
	authSvc *openfga.OFGAClient
}

// NewIdentityManager returns a new identityManager that persists the roles in the provided store.
func NewIdentityManager(store *db.Database, authSvc *openfga.OFGAClient) (*IdentityManager, error) {
	if store == nil {
		return nil, errors.New("identity store cannot be nil")
	}
	if authSvc == nil {
		return nil, errors.New("identity authorisation service cannot be nil")
	}
	return &IdentityManager{store, authSvc}, nil
}

// FetchIdentity fetches the user specified by the username and returns the user if it is found.
// Or error "record not found".
func (j *IdentityManager) FetchIdentity(ctx context.Context, id string) (*openfga.User, error) {

	identity, err := dbmodel.NewIdentity(id)
	if err != nil {
		return nil, err
	}

	if err := j.store.FetchIdentity(ctx, identity); err != nil {
		return nil, err
	}

	return openfga.NewUser(identity, j.authSvc), nil
}

// ListIdentities lists a page of users in our database and parse them into openfga entities.
// `match` will filter the list for fuzzy find on identity name.
func (j *IdentityManager) ListIdentities(ctx context.Context, user *openfga.User, pagination pagination.LimitOffsetPagination, match string) ([]openfga.User, error) {

	if !user.JimmAdmin {
		return nil, errors.Codef(errors.CodeUnauthorized, "unauthorized")
	}
	identities, err := j.store.ListIdentities(ctx, pagination.Limit(), pagination.Offset(), match)
	var users []openfga.User

	for _, id := range identities {
		users = append(users, *openfga.NewUser(&id, j.authSvc))
	}
	if err != nil {
		return nil, err
	}
	return users, nil
}

// CountIdentities returns the count of all the identities in our database.
func (j *IdentityManager) CountIdentities(ctx context.Context, user *openfga.User) (int, error) {

	if !user.JimmAdmin {
		return 0, errors.Codef(errors.CodeUnauthorized, "unauthorized")
	}

	count, err := j.store.CountIdentities(ctx)
	if err != nil {
		return 0, err
	}
	return count, nil
}
