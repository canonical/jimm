// Copyright 2024 Canonical.

package names_test

import (
	"fmt"
	"testing"

	qt "github.com/frankban/quicktest"
	"github.com/google/uuid"
	jujunames "github.com/juju/names/v5"

	"github.com/canonical/jimm/v3/pkg/names"
)

func TestParseTag(t *testing.T) {
	c := qt.New(t)
	uuid := uuid.NewString()
	tests := []struct {
		tagString     string
		expectedTag   jujunames.Tag
		expectedValid bool
	}{
		{
			tagString:     fmt.Sprintf("group-%s", uuid),
			expectedTag:   names.NewGroupTag(uuid),
			expectedValid: true,
		},
		{
			tagString:     fmt.Sprintf("role-%s", uuid),
			expectedTag:   names.NewRoleTag(uuid),
			expectedValid: true,
		},
		{
			tagString:     fmt.Sprintf("serviceaccount-%s@serviceaccount", uuid),
			expectedTag:   names.NewServiceAccountTag(fmt.Sprintf("%s@serviceaccount", uuid)),
			expectedValid: true,
		},
		{
			tagString:     fmt.Sprintf("not-exisintg-%s@serviceaccount", uuid),
			expectedTag:   names.NewServiceAccountTag(fmt.Sprintf("%s@serviceaccount", uuid)),
			expectedValid: false,
		},
		{
			tagString:     "group1",
			expectedValid: false,
		},
	}
	for _, test := range tests {
		tag, err := names.ParseTag(test.tagString)
		if test.expectedValid {
			c.Assert(tag, qt.Equals, test.expectedTag)
		} else {
			c.Assert(err, qt.IsNotNil)
		}

	}
}
