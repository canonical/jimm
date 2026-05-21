// Copyright 2025 Canonical.

package description

import (
	"testing"

	qt "github.com/frankban/quicktest"
	descriptionv10 "github.com/juju/description/v10"
	descriptionv11 "github.com/juju/description/v11"
	descriptionv9 "github.com/juju/description/v9"
	"github.com/juju/juju/environs/config"
	"github.com/juju/names/v5"
	"github.com/juju/version/v2"
)

func TestDescriptionWrappers(t *testing.T) {
	c := qt.New(t)
	const firstDescriptionVersion = 9

	ownerTag := names.NewUserTag("admin")
	userTag := names.NewUserTag("user1")
	cloudTag := names.NewCloudTag("aws")
	updatedOwnerTag := names.NewUserTag("admin-2")
	updatedCloudTag := names.NewCloudTag("gce")

	tests := []struct {
		name               string
		descriptionVersion int
		controllerVersion  version.Number
		serialize          func(*qt.C) []byte
		matchesWrapper     func(Model) bool
	}{
		{
			name:               "v9",
			descriptionVersion: 9,
			controllerVersion:  version.MustParse("3.6.12"),
			serialize: func(c *qt.C) []byte {
				m := descriptionv9.NewModel(descriptionv9.ModelArgs{
					Owner: ownerTag,
					Config: map[string]any{
						"uuid": "model-uuid",
					},
					AgentVersion: "3.6.12",
					Type:         "iaas",
				})
				m.SetStatus(descriptionv9.StatusArgs{Value: "available"})
				m.AddUser(descriptionv9.UserArgs{Name: userTag, Access: "read"})
				m.SetCloudCredential(descriptionv9.CloudCredentialArgs{
					Owner:      ownerTag,
					Cloud:      cloudTag,
					Name:       "cred1",
					AuthType:   "oauth2",
					Attributes: map[string]string{"a": "b"},
				})
				app := m.AddApplication(descriptionv9.ApplicationArgs{Tag: names.NewApplicationTag("app1")})
				app.SetStatus(descriptionv9.StatusArgs{Value: "active"})
				app.AddOffer(descriptionv9.ApplicationOfferArgs{
					OfferName:              "offer1",
					OfferUUID:              "offer-uuid",
					ACL:                    map[string]string{"user": "read"},
					Endpoints:              map[string]string{"relation": "remote"},
					ApplicationName:        "app1",
					ApplicationDescription: "desc",
				})
				bytes, err := descriptionv9.Serialize(m)
				c.Assert(err, qt.IsNil)
				return bytes
			},
			matchesWrapper: func(m Model) bool {
				_, ok := m.(*migrationDescriptionV9)
				return ok
			},
		},
		{
			name:               "v10",
			descriptionVersion: 10,
			controllerVersion:  version.MustParse("3.6.13"),
			serialize: func(c *qt.C) []byte {
				m := descriptionv10.NewModel(descriptionv10.ModelArgs{
					Owner: ownerTag,
					Config: map[string]any{
						"uuid": "model-uuid",
					},
					AgentVersion: "3.6.13",
					Type:         "iaas",
				})
				m.SetStatus(descriptionv10.StatusArgs{Value: "available"})
				m.AddUser(descriptionv10.UserArgs{Name: userTag, Access: "read"})
				m.SetCloudCredential(descriptionv10.CloudCredentialArgs{
					Owner:      ownerTag,
					Cloud:      cloudTag,
					Name:       "cred1",
					AuthType:   "oauth2",
					Attributes: map[string]string{"a": "b"},
				})
				app := m.AddApplication(descriptionv10.ApplicationArgs{Tag: names.NewApplicationTag("app1")})
				app.SetStatus(descriptionv10.StatusArgs{Value: "active"})
				app.AddOffer(descriptionv10.ApplicationOfferArgs{
					OfferName:              "offer1",
					OfferUUID:              "offer-uuid",
					ACL:                    map[string]string{"user": "read"},
					Endpoints:              map[string]string{"relation": "remote"},
					ApplicationName:        "app1",
					ApplicationDescription: "desc",
				})
				bytes, err := descriptionv10.Serialize(m)
				c.Assert(err, qt.IsNil)
				return bytes
			},
			matchesWrapper: func(m Model) bool {
				_, ok := m.(*migrationDescriptionV10)
				return ok
			},
		},
		{
			name:               "v11",
			descriptionVersion: 11,
			controllerVersion:  version.MustParse("3.6.23"),
			serialize: func(c *qt.C) []byte {
				m := descriptionv11.NewModel(descriptionv11.ModelArgs{
					Owner: ownerTag,
					Config: map[string]any{
						"uuid": "model-uuid",
					},
					AgentVersion: "3.6.23",
					Type:         "iaas",
				})
				m.SetStatus(descriptionv11.StatusArgs{Value: "available"})
				m.AddUser(descriptionv11.UserArgs{Name: userTag, Access: "read"})
				m.SetCloudCredential(descriptionv11.CloudCredentialArgs{
					Owner:      ownerTag,
					Cloud:      cloudTag,
					Name:       "cred1",
					AuthType:   "oauth2",
					Attributes: map[string]string{"a": "b"},
				})
				app := m.AddApplication(descriptionv11.ApplicationArgs{Tag: names.NewApplicationTag("app1")})
				app.SetStatus(descriptionv11.StatusArgs{Value: "active"})
				app.AddOffer(descriptionv11.ApplicationOfferArgs{
					OfferName:              "offer1",
					OfferUUID:              "offer-uuid",
					ACL:                    map[string]string{"user": "read"},
					Endpoints:              map[string]string{"relation": "remote"},
					ApplicationName:        "app1",
					ApplicationDescription: "desc",
				})
				bytes, err := descriptionv11.Serialize(m)
				c.Assert(err, qt.IsNil)
				return bytes
			},
			matchesWrapper: func(m Model) bool {
				_, ok := m.(*migrationDescriptionV11)
				return ok
			},
		},
	}

	c.Assert(tests, qt.HasLen, latestDescriptionVersion-firstDescriptionVersion+1)
	for i, tt := range tests {
		c.Assert(tt.descriptionVersion, qt.Equals, firstDescriptionVersion+i)
	}

	for _, tt := range tests {
		c.Run(tt.name, func(c *qt.C) {
			// Create a wrapper around a specific version of the model description
			wrapper, err := Deserialize(tt.serialize(c), tt.controllerVersion)
			c.Assert(err, qt.IsNil)
			c.Assert(tt.matchesWrapper(wrapper), qt.IsTrue)

			// Check the wrapper's fields match what we expect from the original description
			assertWrappedModel(c, wrapper, ownerTag, userTag, cloudTag)
			c.Check(wrapper.Config()["uuid"], qt.Equals, "model-uuid")
			c.Check(wrapper.CloudRegion(), qt.Equals, "")

			// Update some fields in the wrapper, serialize it, and deserialize it again to check that updates are preserved
			wrapper.SetOwner(updatedOwnerTag)
			wrapper.ClearUsers()
			wrapper.SetCloudCredential(CloudCredentialArgs{
				Owner:      updatedOwnerTag,
				Cloud:      updatedCloudTag,
				Name:       "cred2",
				AuthType:   "userpass",
				Attributes: map[string]string{"c": "d"},
			})

			roundTrip, err := wrapper.Serialize()
			c.Assert(err, qt.IsNil)

			updatedWrapper, err := Deserialize(roundTrip, tt.controllerVersion)
			c.Assert(err, qt.IsNil)
			c.Check(updatedWrapper.Owner(), qt.Equals, updatedOwnerTag)
			c.Check(updatedWrapper.Users(), qt.HasLen, 0)
			assertCloudCredential(c, updatedWrapper.CloudCredential(), updatedOwnerTag.Id(), updatedCloudTag.Id(), "cred2", "userpass", map[string]string{"c": "d"})
			apps := updatedWrapper.Applications()
			c.Assert(apps, qt.HasLen, 1)
			c.Check(apps[0].Offers(), qt.HasLen, 1)
		})
	}
}

func TestTryDetermineModelUUID(t *testing.T) {
	c := qt.New(t)
	const firstDescriptionVersion = 9

	ownerTag := names.NewUserTag("admin")
	tests := []struct {
		name               string
		descriptionVersion int
		serialize          func(*qt.C) []byte
	}{
		{
			name:               "v9",
			descriptionVersion: 9,
			serialize: func(c *qt.C) []byte {
				m := descriptionv9.NewModel(descriptionv9.ModelArgs{
					Owner: ownerTag,
					Config: map[string]any{
						config.UUIDKey: "model-uuid",
					},
					AgentVersion: "3.6.12",
					Type:         "iaas",
				})
				m.SetStatus(descriptionv9.StatusArgs{Value: "available"})
				bytes, err := descriptionv9.Serialize(m)
				c.Assert(err, qt.IsNil)
				return bytes
			},
		},
		{
			name:               "v10",
			descriptionVersion: 10,
			serialize: func(c *qt.C) []byte {
				m := descriptionv10.NewModel(descriptionv10.ModelArgs{
					Owner: ownerTag,
					Config: map[string]any{
						config.UUIDKey: "model-uuid",
					},
					AgentVersion: "3.6.13",
					Type:         "iaas",
				})
				m.SetStatus(descriptionv10.StatusArgs{Value: "available"})
				bytes, err := descriptionv10.Serialize(m)
				c.Assert(err, qt.IsNil)
				return bytes
			},
		},
		{
			name:               "v11",
			descriptionVersion: 11,
			serialize: func(c *qt.C) []byte {
				m := descriptionv11.NewModel(descriptionv11.ModelArgs{
					Owner: ownerTag,
					Config: map[string]any{
						config.UUIDKey: "model-uuid",
					},
					AgentVersion: "3.6.23",
					Type:         "iaas",
				})
				m.SetStatus(descriptionv11.StatusArgs{Value: "available"})
				bytes, err := descriptionv11.Serialize(m)
				c.Assert(err, qt.IsNil)
				return bytes
			},
		},
	}

	c.Assert(tests, qt.HasLen, latestDescriptionVersion-firstDescriptionVersion+1)
	for i, tt := range tests {
		c.Assert(tt.descriptionVersion, qt.Equals, firstDescriptionVersion+i)
	}

	for _, tt := range tests {
		c.Run(tt.name, func(c *qt.C) {
			modelUUID, err := TryDetermineModelUUID(tt.serialize(c))
			c.Assert(err, qt.IsNil)
			c.Check(modelUUID, qt.Equals, "model-uuid")
		})
	}

	c.Run("missing uuid returns error", func(c *qt.C) {
		m := descriptionv11.NewModel(descriptionv11.ModelArgs{
			Owner:        ownerTag,
			Config:       map[string]any{},
			AgentVersion: "3.6.23",
			Type:         "iaas",
		})
		m.SetStatus(descriptionv11.StatusArgs{Value: "available"})
		bytes, err := descriptionv11.Serialize(m)
		c.Assert(err, qt.IsNil)

		_, err = TryDetermineModelUUID(bytes)
		c.Assert(err, qt.ErrorMatches, `model config must contain a string value for key "uuid"`)
	})
}

func assertWrappedModel(c *qt.C, w Model, ownerTag, userTag names.UserTag, cloudTag names.CloudTag) {
	c.Check(w.Owner(), qt.Equals, ownerTag)

	users := w.Users()
	c.Assert(users, qt.HasLen, 1)
	c.Check(users[0].Name(), qt.Equals, userTag)
	c.Check(users[0].Access(), qt.Equals, "read")

	assertCloudCredential(c, w.CloudCredential(), ownerTag.Id(), cloudTag.Id(), "cred1", "oauth2", map[string]string{"a": "b"})

	apps := w.Applications()
	c.Assert(apps, qt.HasLen, 1)
	c.Check(apps[0].Name(), qt.Equals, "app1")

	offers := apps[0].Offers()
	c.Assert(offers, qt.HasLen, 1)
	c.Check(offers[0].OfferName(), qt.Equals, "offer1")
	c.Check(offers[0].OfferUUID(), qt.Equals, "offer-uuid")
	c.Check(offers[0].ACL(), qt.DeepEquals, map[string]string{"user": "read"})
}

func assertCloudCredential(c *qt.C, cred CloudCredential, owner, cloud, name, authType string, attributes map[string]string) {
	c.Check(cred.Owner(), qt.Equals, owner)
	c.Check(cred.Cloud(), qt.Equals, cloud)
	c.Check(cred.Name(), qt.Equals, name)
	c.Check(cred.AuthType(), qt.Equals, authType)
	c.Check(cred.Attributes(), qt.DeepEquals, attributes)
}

func TestMigrationDescriptionVersion(t *testing.T) {
	c := qt.New(t)

	tests := []struct {
		name        string
		version     string
		expected    int
		expectError bool
	}{
		{
			name:        "version 3.5.0 returns error",
			version:     "3.5.0",
			expectError: true,
		},
		{
			name:        "version 3.6.0 returns error",
			version:     "3.6.0",
			expectError: true,
		},
		{
			name:     "version 3.6.9 returns 9",
			version:  "3.6.9",
			expected: 9,
		},
		{
			name:     "version 3.6.12 returns 9",
			version:  "3.6.12",
			expected: 9,
		},
		{
			name:     "version 3.6.13 returns 10",
			version:  "3.6.13",
			expected: 10,
		},
		{
			name:     "version 3.6.14 returns 10",
			version:  "3.6.14",
			expected: 10,
		},
		{
			name:     "version 3.7.0 returns latest (11)",
			version:  "3.7.0",
			expected: latestDescriptionVersion,
		},
		{
			name:     "version 4.0.0 returns latest (11)",
			version:  "4.0.0",
			expected: latestDescriptionVersion,
		},
	}
	for _, tt := range tests {
		c.Run(tt.name, func(c *qt.C) {
			v, err := version.Parse(tt.version)
			c.Assert(err, qt.IsNil)
			result, err := migrationDescriptionVersion(v)
			if tt.expectError {
				c.Assert(err, qt.Not(qt.IsNil))
				return
			}
			c.Assert(err, qt.IsNil)
			c.Assert(result, qt.Equals, tt.expected)
		})
	}
}
