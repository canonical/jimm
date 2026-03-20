// Copyright 2026 Canonical.

package testing

import (
	"testing"

	qt "github.com/frankban/quicktest"

	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
	"github.com/canonical/jimm/v3/pkg/api"
	apiparams "github.com/canonical/jimm/v3/pkg/api/params"
)

func testControllerProfileRequest(name string) apiparams.SaveControllerProfileRequest {
	return apiparams.SaveControllerProfileRequest{
		ControllerProfile: apiparams.ControllerProfile{
			Name:        name,
			Description: "Reusable bootstrap settings",
			JujuVersion: "3.6",
			Cloud: apiparams.ControllerProfileCloud{
				Name:           "aws",
				Type:           "ec2",
				AuthTypes:      []string{"access-key"},
				CACertificates: []string{"ca-cert"},
				Config:         map[string]interface{}{"default-base": "ubuntu@24.04"},
				Endpoint:       "https://aws.example.com",
				Region: apiparams.ControllerProfileCloudRegion{
					Name:             "eu-west-1",
					Endpoint:         "https://region.example.com",
					IdentityEndpoint: "https://identity.example.com",
					StorageEndpoint:  "https://storage.example.com",
				},
			},
			BootstrapOptions: apiparams.ControllerProfileBootstrapOptions{
				BootstrapBase:         "ubuntu@24.04",
				BootstrapConstraints:  map[string]string{"mem": "8G", "cores": "2"},
				ModelConstraints:      map[string]string{"arch": "amd64"},
				ModelDefault:          map[string]string{"logging-config": "<root>=INFO"},
				BootstrapConfig:       map[string]string{"bootstrap-timeout": "20m"},
				ControllerConfig:      map[string]string{"audit-log-enabled": "true"},
				ControllerModelConfig: map[string]string{"logging-config": "<root>=INFO"},
				StoragePool: &apiparams.ControllerProfileStoragePool{
					Name:       "controller-pool",
					Type:       "ebs",
					Attributes: map[string]string{"volume-type": "gp3"},
				},
			},
		},
	}
}

func TestControllerProfileCRUD(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupJimmWithControllers(c)

	conn := s.Open(c, nil, "alice", nil)
	defer conn.Close()

	client := api.NewClient(conn)

	req := testControllerProfileRequest("aws-production")
	saved, err := client.SaveControllerProfile(&req)
	c.Assert(err, qt.IsNil)
	c.Assert(saved.Name, qt.Equals, req.Name)
	c.Assert(saved.Version, qt.Equals, uint(1))
	c.Assert(saved.CreatedAt, qt.Not(qt.Equals), "")
	c.Assert(saved.UpdatedAt, qt.Not(qt.Equals), "")

	got, err := client.GetControllerProfile(&apiparams.GetControllerProfileRequest{Name: req.Name})
	c.Assert(err, qt.IsNil)
	c.Assert(got.ControllerProfile, qt.DeepEquals, saved.ControllerProfile)

	saved.Description = "Updated reusable bootstrap settings"
	updated, err := client.SaveControllerProfile(&apiparams.SaveControllerProfileRequest{ControllerProfile: saved.ControllerProfile})
	c.Assert(err, qt.IsNil)
	c.Assert(updated.Version, qt.Equals, uint(2))
	c.Assert(updated.Description, qt.Equals, "Updated reusable bootstrap settings")

	profiles, err := client.ListControllerProfiles(&apiparams.ListControllerProfilesRequest{})
	c.Assert(err, qt.IsNil)
	c.Assert(profiles, qt.HasLen, 1)
	c.Assert(profiles[0].Name, qt.Equals, req.Name)
	c.Assert(profiles[0].Description, qt.Equals, updated.Description)
	c.Assert(profiles[0].CreatedAt, qt.Not(qt.Equals), "")
	c.Assert(profiles[0].UpdatedAt, qt.Not(qt.Equals), "")

	err = client.RemoveControllerProfile(&apiparams.RemoveControllerProfileRequest{Name: req.Name})
	c.Assert(err, qt.IsNil)

	_, err = client.GetControllerProfile(&apiparams.GetControllerProfileRequest{Name: req.Name})
	c.Assert(err, qt.ErrorMatches, ".*not found.*")
}

func TestControllerProfileValidation(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupJimmWithControllers(c)

	conn := s.Open(c, nil, "alice", nil)
	defer conn.Close()

	client := api.NewClient(conn)

	builtIn := testControllerProfileRequest("local-profile")
	builtIn.Cloud.Name = "localhost"
	_, err := client.SaveControllerProfile(&builtIn)
	c.Assert(err, qt.ErrorMatches, ".*built-in clouds like \"localhost\".*")

	malformedPool := testControllerProfileRequest("bad-storage-pool")
	malformedPool.BootstrapOptions.StoragePool = &apiparams.ControllerProfileStoragePool{Name: "controller-pool"}
	_, err = client.SaveControllerProfile(&malformedPool)
	c.Assert(err, qt.ErrorMatches, ".*storage pool requires both name and type.*")

	missingVersion := testControllerProfileRequest("missing-version")
	missingVersion.JujuVersion = ""
	_, err = client.SaveControllerProfile(&missingVersion)
	c.Assert(err, qt.ErrorMatches, ".*juju version must be provided.*")

	invalidVersion := testControllerProfileRequest("invalid-version")
	invalidVersion.JujuVersion = "3.6.x"
	_, err = client.SaveControllerProfile(&invalidVersion)
	c.Assert(err, qt.ErrorMatches, ".*partial/full version string with one, two, or three dot-separated numeric components.*")

	buildVersion := testControllerProfileRequest("build-version")
	buildVersion.JujuVersion = "3.6.4.1"
	_, err = client.SaveControllerProfile(&buildVersion)
	c.Assert(err, qt.ErrorMatches, ".*partial/full version string with one, two, or three dot-separated numeric components.*")

	taggedVersion := testControllerProfileRequest("tagged-version")
	taggedVersion.JujuVersion = "3.6-beta1"
	_, err = client.SaveControllerProfile(&taggedVersion)
	c.Assert(err, qt.ErrorMatches, ".*partial/full version string with one, two, or three dot-separated numeric components.*")

	_, err = client.ListControllerProfiles(&apiparams.ListControllerProfilesRequest{JujuVersion: "3.6.x"})
	c.Assert(err, qt.ErrorMatches, ".*juju version filter must be a partial/full version string with one, two, or three dot-separated numeric components.*")

	_, err = client.ListControllerProfiles(&apiparams.ListControllerProfilesRequest{JujuVersion: "3.6.4.1"})
	c.Assert(err, qt.ErrorMatches, ".*juju version filter must be a partial/full version string with one, two, or three dot-separated numeric components.*")

	_, err = client.ListControllerProfiles(&apiparams.ListControllerProfilesRequest{JujuVersion: "3.6-beta1"})
	c.Assert(err, qt.ErrorMatches, ".*juju version filter must be a partial/full version string with one, two, or three dot-separated numeric components.*")
}

func TestControllerProfileListFiltering(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupJimmWithControllers(c)

	conn := s.Open(c, nil, "alice", nil)
	defer conn.Close()

	client := api.NewClient(conn)

	for _, tc := range []struct {
		name        string
		jujuVersion string
	}{
		{name: "profile-3", jujuVersion: "3"},
		{name: "profile-3-6", jujuVersion: "3.6"},
		{name: "profile-3-6-4", jujuVersion: "3.6.4"},
		{name: "profile-4", jujuVersion: "4"},
	} {
		req := testControllerProfileRequest(tc.name)
		req.JujuVersion = tc.jujuVersion
		_, err := client.SaveControllerProfile(&req)
		c.Assert(err, qt.IsNil)
	}

	profiles, err := client.ListControllerProfiles(&apiparams.ListControllerProfilesRequest{JujuVersion: "3.6.4"})
	c.Assert(err, qt.IsNil)
	c.Assert(profiles, qt.HasLen, 3)
	c.Assert(profiles[0].Name, qt.Equals, "profile-3")
	c.Assert(profiles[1].Name, qt.Equals, "profile-3-6")
	c.Assert(profiles[2].Name, qt.Equals, "profile-3-6-4")
}

func TestControllerProfileUnauthorized(t *testing.T) {
	c := qt.New(t)
	s := jimmtest.SetupJimmWithControllers(c)

	adminConn := s.Open(c, nil, "alice", nil)
	defer adminConn.Close()
	adminClient := api.NewClient(adminConn)

	req := testControllerProfileRequest("shared-profile")
	_, err := adminClient.SaveControllerProfile(&req)
	c.Assert(err, qt.IsNil)

	conn := s.Open(c, nil, "not-authorized-user", nil)
	defer conn.Close()
	client := api.NewClient(conn)

	unauthorizedReq := testControllerProfileRequest("unauthorized-profile")
	_, err = client.SaveControllerProfile(&unauthorizedReq)
	c.Assert(err, qt.ErrorMatches, ".*unauthorized.*")

	_, err = client.GetControllerProfile(&apiparams.GetControllerProfileRequest{Name: req.Name})
	c.Assert(err, qt.ErrorMatches, ".*unauthorized.*")

	_, err = client.ListControllerProfiles(&apiparams.ListControllerProfilesRequest{})
	c.Assert(err, qt.ErrorMatches, ".*unauthorized.*")

	err = client.RemoveControllerProfile(&apiparams.RemoveControllerProfileRequest{Name: req.Name})
	c.Assert(err, qt.ErrorMatches, ".*unauthorized.*")
}
