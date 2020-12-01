// Copyright 2020 Canonical Ltd.

package jimm_test

import (
	"context"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
	"github.com/google/go-cmp/cmp/cmpopts"
	jujuparams "github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/version"

	"github.com/CanonicalLtd/jimm/internal/db"
	"github.com/CanonicalLtd/jimm/internal/dbmodel"
	"github.com/CanonicalLtd/jimm/internal/errors"
	"github.com/CanonicalLtd/jimm/internal/jimm"
	"github.com/CanonicalLtd/jimm/internal/jimmtest"
)

func TestAddController(t *testing.T) {
	c := qt.New(t)

	now := time.Now().UTC().Round(time.Millisecond)
	api := &jimmtest.API{
		Clouds_: func(context.Context) (map[string]jujuparams.Cloud, error) {
			clouds := map[string]jujuparams.Cloud{
				"cloud-aws": jujuparams.Cloud{
					Type:             "ec2",
					AuthTypes:        []string{"userpass"},
					Endpoint:         "https://example.com",
					IdentityEndpoint: "https://identity.example.com",
					StorageEndpoint:  "https://storage.example.com",
					Regions: []jujuparams.CloudRegion{{
						Name:             "eu-west-1",
						Endpoint:         "https://eu-west-1.example.com",
						IdentityEndpoint: "https://eu-west-1.identity.example.com",
						StorageEndpoint:  "https://eu-west-1.storage.example.com",
					}, {
						Name:             "eu-west-2",
						Endpoint:         "https://eu-west-2.example.com",
						IdentityEndpoint: "https://eu-west-2.identity.example.com",
						StorageEndpoint:  "https://eu-west-2.storage.example.com",
					}},
					CACertificates: []string{"CA CERT 1", "CA CERT 2"},
					Config: map[string]interface{}{
						"A": "a",
						"B": 0xb,
					},
					RegionConfig: map[string]map[string]interface{}{
						"eu-west-1": map[string]interface{}{
							"B": 0xb0,
							"C": "C",
						},
						"eu-west-2": map[string]interface{}{
							"B": 0xb1,
							"D": "D",
						},
					},
				},
				"cloud-k8s": jujuparams.Cloud{
					Type:      "kubernetes",
					AuthTypes: []string{"userpass"},
					Endpoint:  "https://k8s.example.com",
					Regions: []jujuparams.CloudRegion{{
						Name: "default",
					}},
				},
			}
			return clouds, nil
		},
		CloudInfo_: func(_ context.Context, tag string, ci *jujuparams.CloudInfo) error {
			if tag != "cloud-k8s" {
				c.Errorf("CloudInfo called for unexpected cloud %q", tag)
				return errors.E("unexpected cloud")
			}
			ci.Type = "kubernetes"
			ci.AuthTypes = []string{"userpass"}
			ci.Endpoint = "https://k8s.example.com"
			ci.Regions = []jujuparams.CloudRegion{{
				Name: "default",
			}}
			ci.Users = []jujuparams.CloudUserInfo{{
				UserName:    "alice@external",
				DisplayName: "Alice",
				Access:      "admin",
			}, {
				UserName:    "bob@external",
				DisplayName: "Bob",
				Access:      "add-model",
			}}
			return nil
		},
		ControllerModelSummary_: func(_ context.Context, ms *jujuparams.ModelSummary) error {
			ms.Name = "controller"
			ms.UUID = "5fddf0ed-83d5-47e8-ae7b-a4b27fc04a9f"
			ms.Type = "iaas"
			ms.ControllerUUID = jimmtest.DefaultControllerUUID
			ms.IsController = true
			ms.ProviderType = "ec2"
			ms.DefaultSeries = "warty"
			ms.CloudTag = "cloud-aws"
			ms.CloudRegion = "eu-west-1"
			ms.OwnerTag = "user-admin"
			ms.Life = "alive"
			ms.Status = jujuparams.EntityStatus{
				Status: "available",
			}
			ms.UserAccess = "admin"
			ms.AgentVersion = &version.Current
			return nil
		},
	}

	j := &jimm.JIMM{
		Database: db.Database{
			DB: jimmtest.MemoryDB(c, func() time.Time { return now }),
		},
		Dialer: &jimmtest.Dialer{
			API: api,
		},
	}

	ctx := context.Background()
	err := j.Database.Migrate(ctx, false)
	c.Assert(err, qt.IsNil)

	u := dbmodel.User{
		Username:         "alice@external",
		ControllerAccess: "superuser",
	}

	ctl := dbmodel.Controller{
		Name:          "test-controller",
		AdminUser:     "admin",
		AdminPassword: "5ecret",
		PublicAddress: "example.com:443",
	}
	err = j.AddController(context.Background(), &u, &ctl)
	c.Assert(err, qt.IsNil)

	ctl2 := dbmodel.Controller{
		Name: "test-controller",
	}
	err = j.Database.GetController(ctx, &ctl2)
	c.Assert(err, qt.IsNil)
	c.Check(ctl2, qt.CmpEquals(cmpopts.EquateEmpty()), ctl)
}
