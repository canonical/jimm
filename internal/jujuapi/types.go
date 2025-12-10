// Copyright 2025 Canonical.

package jujuapi

import (
	"github.com/juju/juju/cloud"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v5"

	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/jimm/juju"
)

func cloudFromParams(cloudName string, p jujuparams.Cloud) cloud.Cloud {
	authTypes := make([]cloud.AuthType, len(p.AuthTypes))
	for i, authType := range p.AuthTypes {
		authTypes[i] = cloud.AuthType(authType)
	}
	regions := make([]cloud.Region, len(p.Regions))
	for i, region := range p.Regions {
		regions[i] = cloud.Region{
			Name:             region.Name,
			Endpoint:         region.Endpoint,
			IdentityEndpoint: region.IdentityEndpoint,
			StorageEndpoint:  region.StorageEndpoint,
		}
	}
	var regionConfig map[string]cloud.Attrs
	for r, attr := range p.RegionConfig {
		if regionConfig == nil {
			regionConfig = make(map[string]cloud.Attrs)
		}
		regionConfig[r] = attr
	}
	return cloud.Cloud{
		Name:              cloudName,
		Type:              p.Type,
		AuthTypes:         authTypes,
		Endpoint:          p.Endpoint,
		IdentityEndpoint:  p.IdentityEndpoint,
		StorageEndpoint:   p.StorageEndpoint,
		Regions:           regions,
		CACertificates:    p.CACertificates,
		SkipTLSVerify:     p.SkipTLSVerify,
		HostCloudRegion:   p.HostCloudRegion,
		Config:            p.Config,
		RegionConfig:      regionConfig,
		IsControllerCloud: p.IsControllerCloud,
	}
}

func toAddModelArgs(args jujuparams.ModelCreateArgs) (*juju.ModelCreateArgs, error) {
	// FromJujuModelCreateArgs converts jujuparams.ModelCreateArgs into AddModelArgs.
	var a juju.ModelCreateArgs
	if args.Name == "" {
		return nil, errors.E("name not specified")
	}
	a.Name = args.Name
	a.Config = args.Config
	a.CloudRegion = args.CloudRegion
	if args.CloudTag != "" {
		ct, err := names.ParseCloudTag(args.CloudTag)
		if err != nil {
			return nil, errors.E(err, errors.CodeBadRequest)
		}
		a.Cloud = ct
	}

	if args.OwnerTag == "" {
		return nil, errors.E("owner tag not specified")
	}
	ot, err := names.ParseUserTag(args.OwnerTag)
	if err != nil {
		return nil, errors.E(err, errors.CodeBadRequest)
	}
	a.Owner = ot

	if args.CloudCredentialTag != "" {
		ct, err := names.ParseCloudCredentialTag(args.CloudCredentialTag)
		if err != nil {
			return nil, errors.E(err, "invalid cloud credential tag")
		}
		if a.Cloud.Id() != "" && ct.Cloud().Id() != a.Cloud.Id() {
			return nil, errors.E("cloud credential cloud mismatch")
		}

		a.CloudCredential = ct
	}
	return &a, nil
}
