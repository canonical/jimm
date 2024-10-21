// Copyright 2024 Canonical.
package jujuclient_test

import (
	"context"

	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v5"
	gc "gopkg.in/check.v1"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
)

type storageSuite struct {
	jujuclientSuite
}

var _ = gc.Suite(&storageSuite{})

func (s *storageSuite) TestListFilesystems(c *gc.C) {
	ctx := context.Background()

	cct := names.NewCloudCredentialTag(jimmtest.TestCloudName + "/bob@canonical.com/pw1")
	cred := jujuparams.TaggedCredential{
		Tag: cct.String(),
		Credential: jujuparams.CloudCredential{
			AuthType: "userpass",
			Attributes: map[string]string{
				"username": "alibaba",
				"password": "open sesame",
			},
		},
	}

	info := s.APIInfo(c)
	ctl := dbmodel.Controller{
		UUID:              s.ControllerConfig.ControllerUUID(),
		Name:              s.ControllerConfig.ControllerName(),
		CACertificate:     info.CACert,
		AdminIdentityName: info.Tag.Id(),
		AdminPassword:     info.Password,
		PublicAddress:     info.Addrs[0],
	}

	models, err := s.API.UpdateCredential(ctx, cred)
	c.Assert(err, gc.Equals, nil)
	c.Assert(models, gc.HasLen, 0)

	var modelInfo jujuparams.ModelInfo
	err = s.API.CreateModel(ctx, &jujuparams.ModelCreateArgs{
		Name:               "model-1",
		OwnerTag:           names.NewUserTag("bob@canonical.com").String(),
		CloudCredentialTag: cct.String(),
	}, &modelInfo)
	c.Assert(err, gc.Equals, nil)
	uuid := modelInfo.UUID

	api, err := s.Dialer.Dial(context.Background(), &ctl, names.NewModelTag(uuid), nil)
	c.Assert(err, gc.IsNil)
	_, err = api.ListFilesystems(ctx, nil)
	c.Assert(err, gc.IsNil)
	// TODO(ale8k): figure out how to add storage to mock models and check res after it
	// for now this just tests the facade is called correctly I guess.
}

func (s *storageSuite) TestListVolumes(c *gc.C) {
	ctx := context.Background()

	cct := names.NewCloudCredentialTag(jimmtest.TestCloudName + "/bob@canonical.com/pw1").String()
	cred := jujuparams.TaggedCredential{
		Tag: cct,
		Credential: jujuparams.CloudCredential{
			AuthType: "userpass",
			Attributes: map[string]string{
				"username": "alibaba",
				"password": "open sesame",
			},
		},
	}

	info := s.APIInfo(c)
	ctl := dbmodel.Controller{
		UUID:              s.ControllerConfig.ControllerUUID(),
		Name:              s.ControllerConfig.ControllerName(),
		CACertificate:     info.CACert,
		AdminIdentityName: info.Tag.Id(),
		AdminPassword:     info.Password,
		PublicAddress:     info.Addrs[0],
	}

	models, err := s.API.UpdateCredential(ctx, cred)
	c.Assert(err, gc.Equals, nil)
	c.Assert(models, gc.HasLen, 0)

	var modelInfo jujuparams.ModelInfo
	err = s.API.CreateModel(ctx, &jujuparams.ModelCreateArgs{
		Name:               "model-1",
		OwnerTag:           names.NewUserTag("bob@canonical.com").String(),
		CloudCredentialTag: cct,
	}, &modelInfo)
	c.Assert(err, gc.Equals, nil)
	uuid := modelInfo.UUID

	api, err := s.Dialer.Dial(context.Background(), &ctl, names.NewModelTag(uuid), nil)
	c.Assert(err, gc.IsNil)
	_, err = api.ListVolumes(ctx, nil)
	c.Assert(err, gc.IsNil)
	// TODO(ale8k): figure out how to add storage to mock models and check res after it
	// for now this just tests the facade is called correctly I guess.
}

func (s *storageSuite) TestListStorageDetails(c *gc.C) {
	ctx := context.Background()

	cct := names.NewCloudCredentialTag(jimmtest.TestCloudName + "/bob@canonical.com/pw1").String()
	cred := jujuparams.TaggedCredential{
		Tag: cct,
		Credential: jujuparams.CloudCredential{
			AuthType: "userpass",
			Attributes: map[string]string{
				"username": "alibaba",
				"password": "open sesame",
			},
		},
	}

	info := s.APIInfo(c)
	ctl := dbmodel.Controller{
		UUID:              s.ControllerConfig.ControllerUUID(),
		Name:              s.ControllerConfig.ControllerName(),
		CACertificate:     info.CACert,
		AdminIdentityName: info.Tag.Id(),
		AdminPassword:     info.Password,
		PublicAddress:     info.Addrs[0],
	}

	models, err := s.API.UpdateCredential(ctx, cred)
	c.Assert(err, gc.Equals, nil)
	c.Assert(models, gc.HasLen, 0)

	var modelInfo jujuparams.ModelInfo
	err = s.API.CreateModel(ctx, &jujuparams.ModelCreateArgs{
		Name:               "model-1",
		OwnerTag:           names.NewUserTag("bob@canonical.com").String(),
		CloudCredentialTag: cct,
	}, &modelInfo)
	c.Assert(err, gc.Equals, nil)
	uuid := modelInfo.UUID

	api, err := s.Dialer.Dial(context.Background(), &ctl, names.NewModelTag(uuid), nil)
	c.Assert(err, gc.IsNil)
	_, err = api.ListStorageDetails(ctx)
	c.Assert(err, gc.IsNil)
	// TODO(ale8k): figure out how to add storage to mock models and check res after it
	// for now this just tests the facade is called correctly I guess.
}
