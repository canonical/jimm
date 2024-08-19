// Copyright 2024 canonical.

package utils

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"github.com/canonical/jimm/v3/internal/common/pagination"
	"github.com/canonical/jimm/v3/internal/openfga"
)

// CreateTokenPaginationFilter returns a token pagination filter based on the rebac admin request parameters.
func CreateTokenPaginationFilter(size *int, token, tokenFromHeader *string) pagination.TokenPagination {
	pageSize := 0
	if size != nil {
		pageSize = *size
	}
	var pageToken string
	if tokenFromHeader != nil {
		pageToken = *tokenFromHeader
	} else if token != nil {
		pageToken = *token
	}
	return pagination.NewTokenFilter(pageSize, pageToken)
}

// entitlementResources is used by `CreateEntitlementPaginationFilter` and `NextEntitlementToken` to
// define which resources to expose and the order in which entitlements are returned to clients.
var entitlementResources = []openfga.Kind{
	openfga.ControllerType,
	openfga.CloudType,
	openfga.ModelType,
	openfga.ApplicationOfferType,
	openfga.GroupType,
	openfga.ServiceAccountType,
}

// EntitlementPagination contains all information necessary for paging through object entitlements.
type EntitlementPagination struct {
	OriginalToken   string
	TokenPagination pagination.TokenPagination
	TargetKind      openfga.Kind
}

// CreateEntitlementPaginationFilter returns an EntitlementPagination object which contains the user's original token,
// an OpenFGA pagination token and a Kind used to indicate which resource to use when requesting entitlements.
// An error is returned if the user provided token could not be unmarshalled.
//
// Use this function alongside `NextEntitlementToken` to page over entitlements.
func CreateEntitlementPaginationFilter(size *int, token, tokenFromHeader *string) (EntitlementPagination, error) {
	filter := CreateTokenPaginationFilter(size, token, tokenFromHeader)
	// If the client sends us an empty token, they are making their first request.
	// Return a filter with an empty OpenFGA token and the first Kind.
	if filter.Token() == "" {
		return EntitlementPagination{
			OriginalToken:   filter.Token(),
			TokenPagination: pagination.NewTokenFilter(filter.Limit(), ""),
			TargetKind:      entitlementResources[0],
		}, nil
	}
	var ct EntitlementPaginationToken
	err := ct.UnmarshalToken(filter.Token())
	if err != nil {
		return EntitlementPagination{}, fmt.Errorf("failed to decode pagination token: %w", err)
	}
	return EntitlementPagination{
		OriginalToken:   filter.Token(),
		TokenPagination: pagination.NewTokenFilter(filter.Limit(), ct.OpenFGAToken),
		TargetKind:      ct.Kind,
	}, nil
}

// NextEntitlementToken converts an OpenFGA token into one that is appropriate to return to a client
// for use by all GetEntitlement endpoints.
// Use this function alongside `CreateEntitlementPaginationFilter` to page over all entitlements.
func NextEntitlementToken(kind openfga.Kind, OpenFGAToken string) (string, error) {
	var ct EntitlementPaginationToken
	ct.OpenFGAToken = OpenFGAToken
	ct.Kind = kind
	// If the OpenFGA token is empty, we are at the end of the result set for that resource.
	if OpenFGAToken == "" {
		resourceIndex := slices.Index(entitlementResources, ct.Kind)
		if resourceIndex == -1 {
			return "", errors.New("failed to generate next entitlement token: unable to determine next resource")
		}
		// Once we've reached the end of all the resources, return an empty token to indicate no more results are left.
		if resourceIndex == len(entitlementResources)-1 {
			return "", nil
		}
		ct.Kind = entitlementResources[resourceIndex+1]
	}
	res, err := ct.MarshalToken()
	if err != nil {
		return "", fmt.Errorf("failed to generate next entitlement token: %w", err)
	}
	return res, nil
}

// EntitlementPaginationToken contains information on the current resource
// and OpenFGA page token used when paginating over entitlements.
type EntitlementPaginationToken struct {
	Kind         openfga.Kind `json:"kind"`
	OpenFGAToken string       `json:"token"`
}

// MarshalToken marshals the entitlement pagination token into a base64 encoded token.
func (c *EntitlementPaginationToken) MarshalToken() (string, error) {
	if c.Kind == openfga.Kind("") {
		return "", errors.New("marshal entitlement token: kind not specified")
	}

	b, err := json.Marshal(c)
	if err != nil {
		return "", fmt.Errorf("marshal entitlement token: %w", err)
	}

	return base64.StdEncoding.EncodeToString(b), nil
}

// UnmarshalToken unmarshals a base64 encoded entitlement pagination token.
func (c *EntitlementPaginationToken) UnmarshalToken(text string) error {
	out, err := base64.StdEncoding.DecodeString(text)
	if err != nil {
		return fmt.Errorf("marshal entitlement token: %w", err)
	}

	return json.Unmarshal(out, c)
}
