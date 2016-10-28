package params

import (
	jujuparams "github.com/juju/juju/apiserver/params"
)

// This file holds types defined for JIMM-specific
// API facades on the websocket RPC connection.

// UserModelStatsResponse holds the response from a UserStats
// RPC call in the JIMM facade.
type UserModelStatsResponse struct {
	// Models holds an entry for each of the user's models
	// indexed by the model's UUID.
	Models map[string]ModelStats
}

// ModelStats holds statistics relating to a model.
type ModelStats struct {
	jujuparams.Model
	Counts map[EntityCount]Count `json:"counts,omitempty"`
}
