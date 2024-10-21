// Copyright 2024 Canonical.
package jujuclient_test

import (
	"context"
	"time"

	"github.com/google/go-cmp/cmp/cmpopts"
	jujuparams "github.com/juju/juju/rpc/params"
	jujuversion "github.com/juju/juju/version"
	"github.com/juju/names/v5"
	gc "gopkg.in/check.v1"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
)

type clientSuite struct {
	jujuclientSuite
}

var _ = gc.Suite(&clientSuite{})

func (s *clientSuite) TestStatus(c *gc.C) {
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
		UUID:              info.ControllerUUID,
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

	status, err := api.Status(ctx, []string{})
	c.Assert(err, gc.Equals, nil)
	c.Assert(status, jimmtest.CmpEquals(cmpopts.IgnoreTypes(&time.Time{})), &jujuparams.FullStatus{
		Model: jujuparams.ModelStatusInfo{
			Name:             "model-1",
			Type:             "iaas",
			CloudTag:         names.NewCloudTag(jimmtest.TestCloudName).String(),
			CloudRegion:      jimmtest.TestCloudRegionName,
			Version:          jujuversion.Current.String(),
			AvailableVersion: "",
			ModelStatus: jujuparams.DetailedStatus{
				Status: "available",
				Info:   "",
				Data:   map[string]interface{}{},
			},
			SLA: "unsupported",
		},
		Machines:           map[string]jujuparams.MachineStatus{},
		Applications:       map[string]jujuparams.ApplicationStatus{},
		RemoteApplications: map[string]jujuparams.RemoteApplicationStatus{},
		Offers:             map[string]jujuparams.ApplicationOfferStatus{},
		Relations:          []jujuparams.RelationStatus(nil),
		Branches:           map[string]jujuparams.BranchStatus{},
	})
}
