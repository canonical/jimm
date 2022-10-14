// Copyright 2020 Canonical Ltd.

package conv

import (
	jujuparams "github.com/juju/juju/apiserver/params"
	"github.com/juju/names/v4"

	"github.com/CanonicalLtd/jimm/internal/mongodoc"
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

// FromCloud creates a slice of mongodoc.CloudRegion from the provided
// cloud information.
func FromCloud(name params.Cloud, cloud jujuparams.Cloud) []mongodoc.CloudRegion {
	crs := make([]mongodoc.CloudRegion, len(cloud.Regions)+1)

	crs[0] = mongodoc.CloudRegion{
		Cloud:            name,
		ProviderType:     cloud.Type,
		AuthTypes:        cloud.AuthTypes,
		Endpoint:         cloud.Endpoint,
		IdentityEndpoint: cloud.IdentityEndpoint,
		StorageEndpoint:  cloud.StorageEndpoint,
		CACertificates:   cloud.CACertificates,
	}

	for i, r := range cloud.Regions {
		crs[i+1] = mongodoc.CloudRegion{
			Cloud:            name,
			Region:           r.Name,
			ProviderType:     cloud.Type,
			Endpoint:         r.Endpoint,
			IdentityEndpoint: r.IdentityEndpoint,
			StorageEndpoint:  r.StorageEndpoint,
		}
	}

	return crs
}
