// Copyright 2024 Canonical.

package rebac_admin

import (
	"context"
	"fmt"

	v1 "github.com/canonical/rebac-admin-ui-handlers/v1"
	"github.com/canonical/rebac-admin-ui-handlers/v1/resources"
	"github.com/juju/names/v5"
	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"

	"github.com/canonical/jimm/v3/internal/common/pagination"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/jimmhttp/rebac_admin/utils"
	"github.com/canonical/jimm/v3/internal/jujuapi"
	"github.com/canonical/jimm/v3/internal/openfga"
	ofganames "github.com/canonical/jimm/v3/internal/openfga/names"
	apiparams "github.com/canonical/jimm/v3/pkg/api/params"
)

type identitiesService struct {
	jimm jujuapi.JIMM
}

func newidentitiesService(jimm jujuapi.JIMM) *identitiesService {
	return &identitiesService{
		jimm: jimm,
	}
}

// ListIdentities returns a page of Identity objects of at least `size` elements if available.
func (s *identitiesService) ListIdentities(ctx context.Context, params *resources.GetIdentitiesParams) (*resources.PaginatedResponse[resources.Identity], error) {
	user, err := utils.GetUserFromContext(ctx)
	if err != nil {
		return nil, err
	}

	count, err := s.jimm.CountIdentities(ctx, user)
	if err != nil {
		return nil, err
	}
	page, nextPage, pagination := pagination.CreatePagination(params.Size, params.Page, count)

	users, err := s.jimm.ListIdentities(ctx, user, pagination)
	if err != nil {
		return nil, err
	}
	rIdentities := make([]resources.Identity, len(users))
	for i, u := range users {
		rIdentities[i] = utils.FromUserToIdentity(u)
	}

	return &resources.PaginatedResponse[resources.Identity]{
		Data: rIdentities,
		Meta: resources.ResponseMeta{
			Page:  &page,
			Size:  len(rIdentities),
			Total: &count,
		},
		Next: resources.Next{
			Page: nextPage,
		},
	}, nil
}

// CreateIdentity creates a single Identity.
func (s *identitiesService) CreateIdentity(ctx context.Context, identity *resources.Identity) (*resources.Identity, error) {
	return nil, v1.NewNotImplementedError("create identity not implemented")
}

// GetIdentity returns a single Identity.
func (s *identitiesService) GetIdentity(ctx context.Context, identityId string) (*resources.Identity, error) {
	user, err := s.jimm.FetchIdentity(ctx, identityId)
	if err != nil {
		if errors.ErrorCode(err) == errors.CodeNotFound {
			return nil, v1.NewNotFoundError(fmt.Sprintf("User with id %s not found", identityId))
		}
		return nil, err
	}
	identity := utils.FromUserToIdentity(*user)
	return &identity, nil
}

// UpdateIdentity updates an Identity.
func (s *identitiesService) UpdateIdentity(ctx context.Context, identity *resources.Identity) (*resources.Identity, error) {
	return nil, v1.NewNotImplementedError("update identity not implemented")
}

// // DeleteIdentity deletes an Identity.
func (s *identitiesService) DeleteIdentity(ctx context.Context, identityId string) (bool, error) {
	return false, v1.NewNotImplementedError("delete identity not implemented")
}

// // GetIdentityRoles returns a page of Roles for identity `identityId`.
func (s *identitiesService) GetIdentityRoles(ctx context.Context, identityId string, params *resources.GetIdentitiesItemRolesParams) (*resources.PaginatedResponse[resources.Role], error) {
	return nil, v1.NewNotImplementedError("get identity roles not implemented")
}

// // PatchIdentityRoles performs addition or removal of a Role to/from an Identity.
func (s *identitiesService) PatchIdentityRoles(ctx context.Context, identityId string, rolePatches []resources.IdentityRolesPatchItem) (bool, error) {
	return false, v1.NewNotImplementedError("get identity roles not implemented")
}

// GetIdentityGroups returns a page of Groups for identity `identityId`.
func (s *identitiesService) GetIdentityGroups(ctx context.Context, identityId string, params *resources.GetIdentitiesItemGroupsParams) (*resources.PaginatedResponse[resources.Group], error) {
	user, err := utils.GetUserFromContext(ctx)
	if err != nil {
		return nil, err
	}
	objUser, err := s.jimm.FetchIdentity(ctx, identityId)
	if err != nil {
		return nil, v1.NewNotFoundError(fmt.Sprintf("User with id %s not found", identityId))
	}
	filter := utils.CreateTokenPaginationFilter(params.Size, params.NextToken, params.NextPageToken)
	tuples, cNextToken, err := s.jimm.ListRelationshipTuples(ctx, user, apiparams.RelationshipTuple{
		Object:       objUser.ResourceTag().String(),
		Relation:     ofganames.MemberRelation.String(),
		TargetObject: openfga.GroupType.String(),
	}, int32(filter.Limit()), filter.Token()) // #nosec G115 accept integer conversion

	if err != nil {
		return nil, err
	}
	groups := make([]resources.Group, len(tuples))
	for i, t := range tuples {
		groups[i] = resources.Group{
			Id:   &t.Target.ID,
			Name: t.Target.ID,
		}
	}
	originalToken := filter.Token()
	return &resources.PaginatedResponse[resources.Group]{
		Data: groups,
		Meta: resources.ResponseMeta{
			Size:      len(groups),
			PageToken: &originalToken,
		},
		Next: resources.Next{
			PageToken: &cNextToken,
		},
	}, nil
}

// PatchIdentityGroups performs addition or removal of a Group to/from an Identity.
func (s *identitiesService) PatchIdentityGroups(ctx context.Context, identityId string, groupPatches []resources.IdentityGroupsPatchItem) (bool, error) {
	user, err := utils.GetUserFromContext(ctx)
	if err != nil {
		return false, err
	}

	objUser, err := s.jimm.FetchIdentity(ctx, identityId)
	if err != nil {
		return false, v1.NewNotFoundError(fmt.Sprintf("User with id %s not found", identityId))
	}
	additions := make([]apiparams.RelationshipTuple, 0)
	deletions := make([]apiparams.RelationshipTuple, 0)
	for _, p := range groupPatches {
		t := apiparams.RelationshipTuple{
			Object:       objUser.ResourceTag().String(),
			Relation:     ofganames.MemberRelation.String(),
			TargetObject: p.Group,
		}
		if p.Op == "add" {
			additions = append(additions, t)
		} else if p.Op == "remove" {
			deletions = append(deletions, t)
		}
	}
	if len(additions) > 0 {
		err = s.jimm.AddRelation(ctx, user, additions)
		if err != nil {
			zapctx.Error(context.Background(), "cannot add relations", zap.Error(err))
			return false, v1.NewUnknownError(err.Error())
		}
	}
	if len(deletions) > 0 {
		err = s.jimm.RemoveRelation(ctx, user, deletions)
		if err != nil {
			zapctx.Error(context.Background(), "cannot remove relations", zap.Error(err))
			return false, v1.NewUnknownError(err.Error())
		}
	}
	return true, nil
}

// // GetIdentityEntitlements returns a page of Entitlements for identity `identityId`.
func (s *identitiesService) GetIdentityEntitlements(ctx context.Context, identityId string, params *resources.GetIdentitiesItemEntitlementsParams) (*resources.PaginatedResponse[resources.EntityEntitlement], error) {
	user, err := utils.GetUserFromContext(ctx)
	if err != nil {
		return nil, err
	}
	objUser, err := s.jimm.FetchIdentity(ctx, identityId)
	if err != nil {
		if errors.ErrorCode(err) == errors.CodeNotFound {
			return nil, v1.NewNotFoundError(fmt.Sprintf("User with id %s not found", identityId))
		}
		return nil, err
	}

	filter := utils.CreateTokenPaginationFilter(params.Size, params.NextToken, params.NextPageToken)
	entitlementToken := pagination.NewEntitlementToken(filter.Token())
	tuples, nextEntitlmentToken, err := s.jimm.ListObjectRelations(ctx, user, objUser.Tag().String(), int32(filter.Limit()), entitlementToken) // #nosec G115 accept integer conversion
	if err != nil {
		return nil, err
	}
	originalToken := filter.Token()
	resp := resources.PaginatedResponse[resources.EntityEntitlement]{
		Meta: resources.ResponseMeta{
			Size:      len(tuples),
			PageToken: &originalToken,
		},
		Data: utils.ToEntityEntitlements(tuples),
	}
	if nextEntitlmentToken.String() != "" {
		nextToken := nextEntitlmentToken.String()
		resp.Next = resources.Next{
			PageToken: &nextToken,
		}
	}
	return &resp, nil
}

// PatchIdentityEntitlements performs addition or removal of an Entitlement to/from an Identity.
func (s *identitiesService) PatchIdentityEntitlements(ctx context.Context, identityId string, entitlementPatches []resources.IdentityEntitlementsPatchItem) (bool, error) {
	user, err := utils.GetUserFromContext(ctx)
	if err != nil {
		return false, err
	}
	objUser, err := s.jimm.FetchIdentity(ctx, identityId)
	if err != nil {
		return false, v1.NewNotFoundError(fmt.Sprintf("User with id %s not found", identityId))
	}
	var toAdd []apiparams.RelationshipTuple
	var toRemove []apiparams.RelationshipTuple
	var errList utils.MultiErr
	toTargetTag := func(entitlementPatch resources.IdentityEntitlementsPatchItem) (names.Tag, error) {
		return utils.ValidateDecomposedTag(
			entitlementPatch.Entitlement.EntityType,
			entitlementPatch.Entitlement.EntityId,
		)
	}
	for _, entitlementPatch := range entitlementPatches {
		targetTag, err := toTargetTag(entitlementPatch)
		if err != nil {
			errList.AppendError(err)
			continue
		}
		t := apiparams.RelationshipTuple{
			Object:       objUser.Tag().String(),
			Relation:     entitlementPatch.Entitlement.Entitlement,
			TargetObject: targetTag.String(),
		}
		if entitlementPatch.Op == resources.IdentityEntitlementsPatchItemOpAdd {
			toAdd = append(toAdd, t)
		} else {
			toRemove = append(toRemove, t)
		}
	}
	if err := errList.Error(); err != nil {
		return false, err
	}
	if toAdd != nil {
		err := s.jimm.AddRelation(ctx, user, toAdd)
		if err != nil {
			return false, err
		}
	}
	if toRemove != nil {
		err := s.jimm.RemoveRelation(ctx, user, toRemove)
		if err != nil {
			return false, err
		}
	}
	return true, nil
}
