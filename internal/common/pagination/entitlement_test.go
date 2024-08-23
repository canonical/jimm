// Copyright 2024 Canonical Ltd.

package pagination_test

import (
	"testing"

	qt "github.com/frankban/quicktest"

	"github.com/canonical/jimm/v3/internal/common/pagination"
	"github.com/canonical/jimm/v3/internal/openfga"
)

func TestMarshalEntitlementToken(t *testing.T) {
	c := qt.New(t)

	tests := []struct {
		desc          string
		token         pagination.ComboToken
		expectedError string
		expectedToken string
	}{{
		desc: "Valid marshal token",
		token: pagination.ComboToken{
			Kind:         openfga.ModelType,
			OpenFGAToken: "continuation-token",
		},
		expectedToken: "eyJraW5kIjoibW9kZWwiLCJ0b2tlbiI6ImNvbnRpbnVhdGlvbi10b2tlbiJ9",
	}, {
		desc: "invalid - missing kind",
		token: pagination.ComboToken{
			Kind:         "",
			OpenFGAToken: "continuation-token",
		},
		expectedError: "marshal entitlement token: kind not specified",
	}}

	for _, tC := range tests {
		c.Run(tC.desc, func(c *qt.C) {
			data, err := tC.token.MarshalToken()
			if tC.expectedError != "" {
				c.Assert(err, qt.ErrorMatches, tC.expectedError)
			} else {
				c.Assert(data, qt.Equals, tC.expectedToken)
			}
		})
	}
}

func TestUnmarshalEntitlementToken(t *testing.T) {
	c := qt.New(t)

	tests := []struct {
		desc          string
		in            string
		expectedToken pagination.ComboToken
		expectedError string
	}{
		{
			desc: "Valid token",
			in:   "eyJraW5kIjoibW9kZWwiLCJ0b2tlbiI6ImNvbnRpbnVhdGlvbi10b2tlbiJ9",
			expectedToken: pagination.ComboToken{
				Kind:         openfga.ModelType,
				OpenFGAToken: "continuation-token",
			},
		},
		{
			desc:          "Invalid token",
			in:            "abc",
			expectedError: "failed to decode token: illegal base64 data at input byte 0",
		},
		{
			desc:          "Invalid JSON in valid Base64 string",
			in:            "c29tZSBpbnZhbGlkIHRva2VuCg==",
			expectedError: "failed to unmarshal token: invalid character 's' looking for beginning of value",
		},
	}

	for _, tC := range tests {
		c.Run(tC.desc, func(c *qt.C) {
			var token pagination.ComboToken
			err := token.UnmarshalToken(tC.in)
			if tC.expectedError != "" {
				c.Assert(err, qt.ErrorMatches, tC.expectedError)
			} else {
				c.Assert(token, qt.DeepEquals, tC.expectedToken)
			}
		})
	}
}

func TestDecodeEntitlementFilter(t *testing.T) {
	c := qt.New(t)
	testCases := []struct {
		desc          string
		nextPageToken func() pagination.EntitlementToken
		expectedToken string
		expectedKind  openfga.Kind
		expectedErr   string
	}{
		{
			desc:          "empty next page token",
			nextPageToken: func() pagination.EntitlementToken { return pagination.NewEntitlementToken("") },
			expectedToken: "",
			expectedKind:  pagination.EntitlementResources[0],
		},
		{
			desc: "model resource page token",
			nextPageToken: func() pagination.EntitlementToken {
				t := pagination.ComboToken{Kind: openfga.ModelType, OpenFGAToken: "123"}
				res, err := t.MarshalToken()
				c.Assert(err, qt.IsNil)
				return pagination.NewEntitlementToken(res)
			},
			expectedToken: "123",
			expectedKind:  openfga.ModelType,
		},
		{
			desc: "invalid token",
			nextPageToken: func() pagination.EntitlementToken {
				return pagination.NewEntitlementToken("123")
			},
			expectedErr: "failed to decode pagination token.*",
		},
	}
	for _, tC := range testCases {
		t.Run(tC.desc, func(t *testing.T) {
			input := tC.nextPageToken()
			openFGAToken, kind, err := pagination.DecodeEntitlementToken(input)
			if tC.expectedErr == "" {
				c.Assert(err, qt.IsNil)
				c.Assert(kind, qt.Equals, tC.expectedKind)
				c.Assert(openFGAToken, qt.Equals, tC.expectedToken)
			} else {
				c.Assert(err, qt.ErrorMatches, tC.expectedErr)
			}
		})
	}
}

func TestNextEntitlementToken(t *testing.T) {
	c := qt.New(t)
	testCases := []struct {
		desc          string
		openFGAToken  string
		kind          openfga.Kind
		expectedToken string
		expectedErr   string
	}{
		{
			desc:          "empty OpenFGA token - expect next resource type",
			openFGAToken:  "",
			kind:          pagination.EntitlementResources[0],
			expectedToken: "eyJraW5kIjoiY2xvdWQiLCJ0b2tlbiI6IiJ9",
		},
		{
			desc:          "non-empty OpenFGA token - expect same kind and token",
			openFGAToken:  "123",
			kind:          openfga.ModelType,
			expectedToken: "eyJraW5kIjoibW9kZWwiLCJ0b2tlbiI6IjEyMyJ9",
		},
		{
			desc:         "empty kind - expect error",
			openFGAToken: "123",
			kind:         "",
			expectedErr:  ".*kind not specified",
		},
		{
			desc:          "last resource type but not last page - expect same kind and token",
			openFGAToken:  "123",
			kind:          pagination.EntitlementResources[len(pagination.EntitlementResources)-1],
			expectedToken: "eyJraW5kIjoic2VydmljZWFjY291bnQiLCJ0b2tlbiI6IjEyMyJ9",
		},
		{
			desc:          "last resource type with no more data - expect empty token",
			openFGAToken:  "",
			kind:          pagination.EntitlementResources[len(pagination.EntitlementResources)-1],
			expectedToken: "",
		},
	}
	for _, tC := range testCases {
		t.Run(tC.desc, func(t *testing.T) {
			token, err := pagination.NextEntitlementToken(tC.kind, tC.openFGAToken)
			if tC.expectedErr == "" {
				c.Assert(err, qt.IsNil)
				c.Assert(token.String(), qt.Equals, tC.expectedToken)
			} else {
				c.Assert(err, qt.ErrorMatches, tC.expectedErr)
			}
		})
	}
}
