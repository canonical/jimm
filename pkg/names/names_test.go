// Copyright 2025 Canonical.

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
			tagString:     "idpgroup-engineering",
			expectedTag:   names.NewIdPGroupTag("engineering"),
			expectedValid: true,
		},
		{
			tagString:     "idpgroup-4a8f49a8-df10-4a6d-a98f-f4df1d5a16ba#member",
			expectedTag:   names.NewIdPGroupTag("4a8f49a8-df10-4a6d-a98f-f4df1d5a16ba#member"),
			expectedValid: true,
		},
		{
			tagString:     "group1",
			expectedValid: false,
		},
		{
			tagString:     "idpgroup-",
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
