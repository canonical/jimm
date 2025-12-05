// Copyright 2025 Canonical.

package description

import (
	"testing"

	qt "github.com/frankban/quicktest"
	descriptionv10 "github.com/juju/description/v10"
	descriptionv9 "github.com/juju/description/v9"
	"github.com/juju/names/v5"
	"github.com/juju/version/v2"
)

func TestDescriptionWrappers(t *testing.T) {
	c := qt.New(t)

	// Common data
	ownerTag := names.NewUserTag("admin")
	userTag := names.NewUserTag("user1")
	cloudTag := names.NewCloudTag("aws")

	// Setup v9 model
	m9 := descriptionv9.NewModel(descriptionv9.ModelArgs{
		Owner: ownerTag,
		Config: map[string]any{
			"uuid": "model-uuid",
		},
		AgentVersion: "3.6.12",
		Type:         "iaas",
	})
	m9.SetStatus(descriptionv9.StatusArgs{Value: "available"})
	m9.AddUser(descriptionv9.UserArgs{
		Name:   userTag,
		Access: "read",
	})
	m9.SetCloudCredential(descriptionv9.CloudCredentialArgs{
		Owner:      ownerTag,
		Cloud:      cloudTag,
		Name:       "cred1",
		AuthType:   "oauth2",
		Attributes: map[string]string{"a": "b"},
	})
	app9 := m9.AddApplication(descriptionv9.ApplicationArgs{
		Tag: names.NewApplicationTag("app1"),
	})
	app9.SetStatus(descriptionv9.StatusArgs{Value: "active"})
	app9.AddOffer(descriptionv9.ApplicationOfferArgs{
		OfferName:              "offer1",
		OfferUUID:              "offer-uuid",
		ACL:                    map[string]string{"user": "read"},
		Endpoints:              map[string]string{"relation": "remote"},
		ApplicationName:        "app1",
		ApplicationDescription: "desc",
	})

	// Serialize v9
	bytes9, err := descriptionv9.Serialize(m9)
	c.Assert(err, qt.IsNil)

	// Deserialize using wrapper
	wrapper9, err := Deserialize(bytes9, version.MustParse("3.6.12"))
	c.Assert(err, qt.IsNil)

	// Setup v10 model
	m10 := descriptionv10.NewModel(descriptionv10.ModelArgs{
		Owner: ownerTag,
		Config: map[string]any{
			"uuid": "model-uuid",
		},
		AgentVersion: "3.6.13",
		Type:         "iaas",
	})
	m10.SetStatus(descriptionv10.StatusArgs{Value: "available"})
	m10.AddUser(descriptionv10.UserArgs{
		Name:   userTag,
		Access: "read",
	})
	m10.SetCloudCredential(descriptionv10.CloudCredentialArgs{
		Owner:      ownerTag,
		Cloud:      cloudTag,
		Name:       "cred1",
		AuthType:   "oauth2",
		Attributes: map[string]string{"a": "b"},
	})
	app10 := m10.AddApplication(descriptionv10.ApplicationArgs{
		Tag: names.NewApplicationTag("app1"),
	})
	app10.SetStatus(descriptionv10.StatusArgs{Value: "active"})
	app10.AddOffer(descriptionv10.ApplicationOfferArgs{
		OfferName:              "offer1",
		OfferUUID:              "offer-uuid",
		ACL:                    map[string]string{"user": "read"},
		Endpoints:              map[string]string{"relation": "remote"},
		ApplicationName:        "app1",
		ApplicationDescription: "desc",
	})

	// Serialize v10
	bytes10, err := descriptionv10.Serialize(m10)
	c.Assert(err, qt.IsNil)

	// Deserialize using wrapper
	wrapper10, err := Deserialize(bytes10, version.MustParse("3.6.13"))
	c.Assert(err, qt.IsNil)

	// Verify both wrappers return same values
	wrappers := []Model{wrapper9, wrapper10}
	for _, w := range wrappers {
		c.Check(w.Owner(), qt.Equals, ownerTag)

		users := w.Users()
		c.Assert(users, qt.HasLen, 1)
		c.Check(users[0].Name(), qt.Equals, userTag)
		c.Check(users[0].Access(), qt.Equals, "read")

		cred := w.CloudCredential()
		c.Check(cred.Owner(), qt.Equals, ownerTag.Id())
		c.Check(cred.Cloud(), qt.Equals, cloudTag.Id())
		c.Check(cred.Name(), qt.Equals, "cred1")
		c.Check(cred.AuthType(), qt.Equals, "oauth2")
		c.Check(cred.Attributes(), qt.DeepEquals, map[string]string{"a": "b"})

		apps := w.Applications()
		c.Assert(apps, qt.HasLen, 1)
		c.Check(apps[0].Name(), qt.Equals, "app1")

		offers := apps[0].Offers()
		c.Assert(offers, qt.HasLen, 1)
		c.Check(offers[0].OfferName(), qt.Equals, "offer1")
		c.Check(offers[0].OfferUUID(), qt.Equals, "offer-uuid")
		c.Check(offers[0].ACL(), qt.DeepEquals, map[string]string{"user": "read"})
	}
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
			name:     "version 3.6.14 returns latest (10)",
			version:  "3.6.14",
			expected: latestDescriptionVersion,
		},
		{
			name:     "version 3.7.0 returns latest (10)",
			version:  "3.7.0",
			expected: latestDescriptionVersion,
		},
		{
			name:     "version 4.0.0 returns latest (10)",
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
