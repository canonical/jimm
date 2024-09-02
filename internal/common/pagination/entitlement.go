// Copyright 2024 Canonical.

package pagination

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"github.com/canonical/jimm/v3/internal/openfga"
)

// Entitlement pagination is a method pagination used with OpenFGA's pagination
// that allows a client to retrieve all the direct relations a user/group has.

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

// EntitlementToken represents a wrapped OpenFGA token that contains
// extra information used for pagination over resource entitlements.
type EntitlementToken struct {
	token string
}

// String returns the contents of the entitlement token as a string.
func (e EntitlementToken) String() string {
	return e.token
}

// NewEntitlementToken returns a new entitlement token based on the provided string.
func NewEntitlementToken(token string) EntitlementToken {
	return EntitlementToken{token: token}
}

// TODO(Kian): Move the code below into the application layer (i.e. jimm package)
// specifically into the access related service (when one exists).
// The details on encoding/decoding EntitlementTokens is only used by `internal/jimm`.

// DecodeEntitlementToken accepts an entitlement token and decodes it to return
// the original OpenFGA page token as well as the object Kind that was encoded in the wrapped token.
//
// An error is returned if the provided token could not be decoded.
// Use this function alongside `NextEntitlementToken` to page over entitlements.
func DecodeEntitlementToken(token EntitlementToken) (string, openfga.Kind, error) {
	// If the client sends us an empty token, they are making their first request.
	// Return a filter with an empty OpenFGA token and the first Kind.
	if token.String() == "" {
		return "", entitlementResources[0], nil
	}
	var ct comboToken
	err := ct.UnmarshalToken(token.String())
	if err != nil {
		return "", "", fmt.Errorf("failed to decode pagination token: %w", err)
	}
	return ct.OpenFGAToken, ct.Kind, nil
}

// NextEntitlementToken accepts an OpenFGA token and Kind and returns a wrapped OpenFGA page token in the form
// of an Entitlement token that encodes the provided information together.
//
// Use this function alongside `DecodeEntitlementToken` to page over all entitlements.
func NextEntitlementToken(kind openfga.Kind, openFGAToken string) (EntitlementToken, error) {
	var ct comboToken
	ct.OpenFGAToken = openFGAToken
	ct.Kind = kind
	// If the OpenFGA token is empty, we are at the end of the result set for that resource.
	if openFGAToken == "" {
		resourceIndex := slices.Index(entitlementResources, ct.Kind)
		if resourceIndex == -1 {
			return EntitlementToken{}, errors.New("failed to generate next entitlement token: unable to determine next resource")
		}
		// Once we've reached the end of all the resources, return an empty token to indicate no more results are left.
		if resourceIndex == len(entitlementResources)-1 {
			return EntitlementToken{}, nil
		}
		ct.Kind = entitlementResources[resourceIndex+1]
	}
	res, err := ct.MarshalToken()
	if err != nil {
		return EntitlementToken{}, fmt.Errorf("failed to generate next entitlement token: %w", err)
	}
	return EntitlementToken{token: res}, nil
}

// comboToken contains information on the current resource
// and OpenFGA page token used when paginating over entitlements.
type comboToken struct {
	Kind         openfga.Kind `json:"kind"`
	OpenFGAToken string       `json:"token"`
}

// MarshalToken marshals the entitlement pagination token into a base64 encoded token.
func (c *comboToken) MarshalToken() (string, error) {
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
func (c *comboToken) UnmarshalToken(token string) error {
	out, err := base64.StdEncoding.DecodeString(token)
	if err != nil {
		return fmt.Errorf("failed to decode token: %w", err)
	}
	if err := json.Unmarshal(out, c); err != nil {
		return fmt.Errorf("failed to unmarshal token: %w", err)
	}
	return nil
}
