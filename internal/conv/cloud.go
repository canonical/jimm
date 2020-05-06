// Copyright 2020 Canonical Ltd.

package conv

import (
	"github.com/juju/names/v4"

	"github.com/CanonicalLtd/jimm/params"
)

// ToCloudTag creates a juju cloud tag from a params.Cloud
func ToCloudTag(c params.Cloud) names.CloudTag {
	return names.NewCloudTag(string(c))
}

// FromCloudTag creates a params.Cloud from the given juju cloud tag.
func FromCloudTag(t names.CloudTag) params.Cloud {
	return params.Cloud(t.Id())
}
