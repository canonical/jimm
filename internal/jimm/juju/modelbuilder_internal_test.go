// Copyright 2025 Canonical.

package juju

import (
	"math/rand/v2"
	"testing"

	qt "github.com/frankban/quicktest"

	"github.com/canonical/jimm/v3/internal/dbmodel"
)

func TestShuffleCandidateControllers(t *testing.T) {
	c := qt.New(t)

	tests := []struct {
		name       string
		setup      func()
		teardown   func()
		candidates []candidateController
		expected   []string // expected controller names in order
	}{
		{
			name:       "empty slice",
			candidates: []candidateController{},
			expected:   []string{},
		},
		{
			name: "single controller",
			candidates: []candidateController{
				{controller: dbmodel.Controller{Name: "ctrl-1"}, priority: 1},
			},
			expected: []string{"ctrl-1"},
		},
		{
			name: "controllers sorted by priority (highest first)",
			setup: func() {
				// Override shuffle to be a no-op so we can test sorting only
				shuffle = func(n int, swap func(i, j int)) {}
			},
			teardown: func() {
				// Restore original shuffle behavior for other tests
				shuffle = rand.Shuffle
			},
			candidates: []candidateController{
				{controller: dbmodel.Controller{Name: "low"}, priority: 1},
				{controller: dbmodel.Controller{Name: "high"}, priority: 10},
				{controller: dbmodel.Controller{Name: "medium"}, priority: 5},
			},
			expected: []string{"high", "medium", "low"},
		},
		{
			name: "controllers with same priority maintain relative order after stable sort",
			setup: func() {
				// Override shuffle to be a no-op
				shuffle = func(n int, swap func(i, j int)) {}
			},
			teardown: func() {
				shuffle = rand.Shuffle
			},
			candidates: []candidateController{
				{controller: dbmodel.Controller{Name: "ctrl-a"}, priority: 5},
				{controller: dbmodel.Controller{Name: "ctrl-b"}, priority: 5},
				{controller: dbmodel.Controller{Name: "ctrl-c"}, priority: 5},
			},
			expected: []string{"ctrl-a", "ctrl-b", "ctrl-c"},
		},
		{
			name: "shuffle randomizes controllers with same priority",
			setup: func() {
				// Override shuffle to reverse the slice
				shuffle = func(n int, swap func(i, j int)) {
					for i := 0; i < n/2; i++ {
						swap(i, n-1-i)
					}
				}
			},
			teardown: func() {
				shuffle = rand.Shuffle
			},
			candidates: []candidateController{
				{controller: dbmodel.Controller{Name: "ctrl-1"}, priority: 5},
				{controller: dbmodel.Controller{Name: "ctrl-2"}, priority: 5},
				{controller: dbmodel.Controller{Name: "ctrl-3"}, priority: 5},
			},
			expected: []string{"ctrl-3", "ctrl-2", "ctrl-1"},
		},
		{
			name: "mixed priorities with shuffling",
			setup: func() {
				// Override shuffle to reverse the slice
				shuffle = func(n int, swap func(i, j int)) {
					for i := 0; i < n/2; i++ {
						swap(i, n-1-i)
					}
				}
			},
			teardown: func() {
				shuffle = rand.Shuffle
			},
			candidates: []candidateController{
				{controller: dbmodel.Controller{Name: "high-1"}, priority: 10},
				{controller: dbmodel.Controller{Name: "low-1"}, priority: 1},
				{controller: dbmodel.Controller{Name: "high-2"}, priority: 10},
				{controller: dbmodel.Controller{Name: "low-2"}, priority: 1},
			},
			// After reverse shuffle: low-2, high-2, low-1, high-1
			// After stable sort by priority: high-2, high-1, low-2, low-1
			expected: []string{"high-2", "high-1", "low-2", "low-1"},
		},
	}

	for _, tt := range tests {
		c.Run(tt.name, func(c *qt.C) {
			if tt.setup != nil {
				tt.setup()
			}
			if tt.teardown != nil {
				defer tt.teardown()
			}

			shuffleCandidateControllers(tt.candidates)

			var names []string
			for _, candidate := range tt.candidates {
				names = append(names, candidate.controller.Name)
			}

			if len(tt.expected) == 0 {
				c.Assert(names, qt.HasLen, 0)
			} else {
				c.Assert(names, qt.DeepEquals, tt.expected)
			}
		})
	}
}
