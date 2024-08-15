// Copyright 2024 Canonical Ltd.

package utils_test

import (
	"testing"

	qt "github.com/frankban/quicktest"

	"github.com/canonical/jimm/v3/internal/openfga"
	"github.com/canonical/jimm/v3/internal/rebac_admin/utils"
)

func TestMarshalRebacToken(t *testing.T) {
	c := qt.New(t)

	tests := []struct {
		desc          string
		token         utils.RebacToken
		expectedError string
		expectedToken string
	}{{
		desc: "Valid marshal token",
		token: utils.RebacToken{
			Kind:         openfga.ModelType,
			OpenFGAToken: "continuation-token",
		},
		expectedToken: "eyJraW5kIjoibW9kZWwiLCJ0b2tlbiI6ImNvbnRpbnVhdGlvbi10b2tlbiJ9",
	}, {
		desc: "invalid - missing kind",
		token: utils.RebacToken{
			Kind:         "",
			OpenFGAToken: "continuation-token",
		},
		expectedError: "marshal rebac token: kind not specified",
	}}

	for _, tC := range tests {
		c.Run(tC.desc, func(c *qt.C) {
			data, err := tC.token.MarshalRebacToken()
			if tC.expectedError != "" {
				c.Assert(err, qt.ErrorMatches, tC.expectedError)
			} else {
				c.Assert(data, qt.Equals, tC.expectedToken)
			}
		})
	}
}

func TestUnmarshalRebacToken(t *testing.T) {
	c := qt.New(t)

	tests := []struct {
		desc          string
		in            string
		expectedToken utils.RebacToken
		expectedError string
	}{
		{
			desc: "Valid token",
			in:   "eyJraW5kIjoibW9kZWwiLCJ0b2tlbiI6ImNvbnRpbnVhdGlvbi10b2tlbiJ9",
			expectedToken: utils.RebacToken{
				Kind:         openfga.ModelType,
				OpenFGAToken: "continuation-token",
			},
		},
		{
			desc:          "Invalid token",
			in:            "abc",
			expectedError: "marshal rebac token: illegal base64 data at input byte 0",
		},
	}

	for _, tC := range tests {
		c.Run(tC.desc, func(c *qt.C) {
			var token utils.RebacToken
			err := token.UnmarshalRebacToken(tC.in)
			if tC.expectedError != "" {
				c.Assert(err, qt.ErrorMatches, tC.expectedError)
			} else {
				c.Assert(token, qt.DeepEquals, tC.expectedToken)
			}
		})
	}
}

func TestCreateEntitlementPaginationFilter(t *testing.T) {
	c := qt.New(t)
	testCases := []struct {
		desc          string
		nextPageToken func() string
		expectedToken string
		expectedKind  openfga.Kind
		expectedErr   string
	}{
		{
			desc:          "empty next page token",
			nextPageToken: func() string { return "" },
			expectedToken: "",
			expectedKind:  utils.EntitlementResources[0],
		},
		{
			desc: "model resource page token",
			nextPageToken: func() string {
				t := utils.RebacToken{Kind: openfga.ModelType, OpenFGAToken: "123"}
				res, err := t.MarshalRebacToken()
				c.Assert(err, qt.IsNil)
				return res
			},
			expectedToken: "123",
			expectedKind:  openfga.ModelType,
		},
		{
			desc: "invalid token",
			nextPageToken: func() string {
				return "123"
			},
			expectedErr: "failed to decode pagination token.*",
		},
	}
	for _, tC := range testCases {
		t.Run(tC.desc, func(t *testing.T) {
			input := tC.nextPageToken()
			filter, err := utils.CreateEntitlementPaginationFilter(nil, &input, nil)
			if tC.expectedErr == "" {
				c.Assert(err, qt.IsNil)
				c.Assert(filter.OriginalToken, qt.Equals, input)
				c.Assert(filter.TargetKind, qt.Equals, tC.expectedKind)
				c.Assert(filter.TokenPagination.Token(), qt.Equals, tC.expectedToken)
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
			kind:          utils.EntitlementResources[0],
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
			kind:          utils.EntitlementResources[len(utils.EntitlementResources)-1],
			expectedToken: "eyJraW5kIjoic2VydmljZWFjY291bnQiLCJ0b2tlbiI6IjEyMyJ9",
		},
		{
			desc:          "last resource type with no more data - expect empty token",
			openFGAToken:  "",
			kind:          utils.EntitlementResources[len(utils.EntitlementResources)-1],
			expectedToken: "",
		},
	}
	for _, tC := range testCases {
		t.Run(tC.desc, func(t *testing.T) {
			token, err := utils.NextEntitlementToken(tC.kind, tC.openFGAToken)
			if tC.expectedErr == "" {
				c.Assert(err, qt.IsNil)
				c.Assert(token, qt.Equals, tC.expectedToken)
			} else {
				c.Assert(err, qt.ErrorMatches, tC.expectedErr)
			}
		})
	}
}
