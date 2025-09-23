// Copyright 2025 Canonical.

package bootstrap

import (
	"testing"

	qt "github.com/frankban/quicktest"
)

func TestValidateBootstrapParams_AllValid(t *testing.T) {
	c := qt.New(t)
	params := BootstrapParams{
		CLIVersion: "1.0.0",

		CloudNameAndRegion: "cloud/region",
		ControllerName:     "my-controller",
		// CloudCred & PersonalCloud are not validated.
	}
	c.Assert(params.validate(), qt.IsNil)

}

func TestValidateBootstrapParams_EmptyFields(t *testing.T) {
	tests := []struct {
		name   string
		params BootstrapParams
		want   []string
	}{
		{
			name:   "all empty",
			params: BootstrapParams{},
			want: []string{
				"CLI version cannot be empty",
				"cloud name and region cannot be empty",
				"controller name cannot be empty",
			},
		},
		{
			name: "cli version empty",
			params: BootstrapParams{
				CloudNameAndRegion: "cloud/region",
				ControllerName:     "my-controller",
			},
			want: []string{
				"CLI version cannot be empty",
			},
		},
		{
			name: "cloud name and region empty",
			params: BootstrapParams{
				CLIVersion:     "1.0.0",
				ControllerName: "my-controller",
			},
			want: []string{
				"cloud name and region cannot be empty",
			},
		},
		{
			name: "controller name empty",
			params: BootstrapParams{
				CLIVersion:         "1.0.0",
				CloudNameAndRegion: "cloud/region",
			},
			want: []string{
				"controller name cannot be empty",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := qt.New(t)
			err := tt.params.validate()
			c.Assert(err, qt.Not(qt.IsNil))
			for _, wantMsg := range tt.want {
				c.Assert(err, qt.ErrorMatches, "(?s).*"+wantMsg+".*")
			}
		})
	}
}
