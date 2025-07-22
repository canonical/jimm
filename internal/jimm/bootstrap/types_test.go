// Copyright 2025 Canonical.

package bootstrap

import (
	"testing"

	qt "github.com/frankban/quicktest"
)

func TestValidateBootstrapParams_AllValid(t *testing.T) {
	c := qt.New(t)
	params := BootstrapParams{
		ControllerName: "ctrl",
		CloudName:      "cloud",
		CloudRegion:    "region",
		AgentVersion:   "1.2.3",
		TimeoutSeconds: 60,
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
				"controller name cannot be empty",
				"cloud name cannot be empty",
				"cloud region cannot be empty",
				"agent version cannot be empty",
			},
		},
		{
			name: "missing controller name",
			params: BootstrapParams{
				CloudName:    "cloud",
				CloudRegion:  "region",
				AgentVersion: "1.2.3",
			},
			want: []string{"controller name cannot be empty"},
		},
		{
			name: "missing cloud name",
			params: BootstrapParams{
				ControllerName: "ctrl",
				CloudRegion:    "region",
				AgentVersion:   "1.2.3",
			},
			want: []string{"cloud name cannot be empty"},
		},
		{
			name: "missing cloud region",
			params: BootstrapParams{
				ControllerName: "ctrl",
				CloudName:      "cloud",
				AgentVersion:   "1.2.3",
			},
			want: []string{"cloud region cannot be empty"},
		},
		{
			name: "missing agent version",
			params: BootstrapParams{
				ControllerName: "ctrl",
				CloudName:      "cloud",
				CloudRegion:    "region",
			},
			want: []string{"agent version cannot be empty"},
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
