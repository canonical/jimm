// Copyright 2025 Canonical.

package permissions_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/canonical/ofga"
	petname "github.com/dustinkirkland/golang-petname"
	qt "github.com/frankban/quicktest"
	"github.com/google/uuid"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v5"

	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/jimm"
	"github.com/canonical/jimm/v3/internal/jimm/permissions"
	"github.com/canonical/jimm/v3/internal/openfga"
	ofganames "github.com/canonical/jimm/v3/internal/openfga/names"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
	jimmnames "github.com/canonical/jimm/v3/pkg/names"
)

func (s *permissionManagerSuite) TestAuditLogAccess(c *qt.C) {
	c.Parallel()

	ctx := context.Background()

	// admin user can grant other users audit log access.
	err := s.manager.GrantAuditLogAccess(ctx, s.adminUser, s.user.ResourceTag())
	c.Assert(err, qt.IsNil)

	access := s.user.GetAuditLogViewerAccess(ctx, s.ctlTag)
	c.Assert(access, qt.Equals, ofganames.AuditLogViewerRelation)

	// re-granting access does not result in error.
	err = s.manager.GrantAuditLogAccess(ctx, s.adminUser, s.user.ResourceTag())
	c.Assert(err, qt.IsNil)

	// admin user can revoke other users audit log access.
	err = s.manager.RevokeAuditLogAccess(ctx, s.adminUser, s.user.ResourceTag())
	c.Assert(err, qt.IsNil)

	access = s.user.GetAuditLogViewerAccess(ctx, s.ctlTag)
	c.Assert(access, qt.Equals, ofganames.NoRelation)

	// re-revoking access does not result in error.
	err = s.manager.RevokeAuditLogAccess(ctx, s.adminUser, s.user.ResourceTag())
	c.Assert(err, qt.IsNil)

	// non-admin user cannot grant audit log access
	err = s.manager.GrantAuditLogAccess(ctx, s.user, s.adminUser.ResourceTag())
	c.Assert(err, qt.ErrorMatches, "unauthorized")

	// non-admin user cannot revoke audit log access
	err = s.manager.RevokeAuditLogAccess(ctx, s.user, s.adminUser.ResourceTag())
	c.Assert(err, qt.ErrorMatches, "unauthorized")
}

func (s *permissionManagerSuite) TestGrantServiceAccountAccess(c *qt.C) {
	c.Parallel()

	tests := []struct {
		about                     string
		grantServiceAccountAccess func(ctx context.Context, user *openfga.User, tags []string) error
		clientID                  string
		tags                      []string
		username                  string
		addGroups                 []string
		expectedError             string
	}{{
		about: "Valid request",
		grantServiceAccountAccess: func(ctx context.Context, user *openfga.User, tags []string) error {
			return nil
		},
		addGroups: []string{"1"},
		tags: []string{
			"user-alice",
			"user-bob",
			"group-1#member",
		},
		clientID: "fca1f605-736e-4d1f-bcd2-aecc726923be@serviceaccount",
		username: "alice",
	}, {
		about: "Group that doesn't exist",
		grantServiceAccountAccess: func(ctx context.Context, user *openfga.User, tags []string) error {
			return nil
		},
		tags: []string{
			"user-alice",
			"user-bob",
			// This group doesn't exist.
			"group-bar",
		},
		clientID:      "fca1f605-736e-4d1f-bcd2-aecc726923be@serviceaccount",
		username:      "alice",
		expectedError: "group bar not found",
	}, {
		about: "Invalid tags",
		grantServiceAccountAccess: func(ctx context.Context, user *openfga.User, tags []string) error {
			return nil
		},
		tags: []string{
			"user-alice",
			"user-bob",
			"controller-jimm",
		},
		clientID:      "fca1f605-736e-4d1f-bcd2-aecc726923be@serviceaccount",
		username:      "alice",
		expectedError: "invalid entity - not user or group",
	}}

	for _, test := range tests {
		c.Run(test.about, func(c *qt.C) {
			if len(test.addGroups) > 0 {
				for _, name := range test.addGroups {
					_, err := s.db.AddGroup(context.Background(), name)
					c.Assert(err, qt.IsNil)
				}
			}
			svcAccountTag := jimmnames.NewServiceAccountTag(test.clientID)

			err := s.manager.GrantServiceAccountAccess(context.Background(), s.adminUser, svcAccountTag, test.tags)
			if test.expectedError == "" {
				c.Assert(err, qt.IsNil)
				for _, tag := range test.tags {
					parsedTag, err := s.manager.ParseAndValidateTag(context.Background(), tag)
					c.Assert(err, qt.IsNil)
					tuple := openfga.Tuple{
						Object:   parsedTag,
						Relation: ofganames.AdministratorRelation,
						Target:   ofganames.ConvertTag(jimmnames.NewServiceAccountTag(test.clientID)),
					}
					ok, err := s.ofgaClient.CheckRelation(context.Background(), tuple, false)
					c.Assert(err, qt.IsNil)
					c.Assert(ok, qt.IsTrue)
				}
			} else {
				c.Assert(err, qt.ErrorMatches, test.expectedError)
			}
		})
	}
}

const testGetControllerAccessEnv = `
users:
- username: alice@canonical.com
  display-name: Alice
  controller-access: superuser
- username: bob@canonical.com
  display-name: Bob
  controller-access: login
`

func (s *permissionManagerSuite) TestGetControllerAccess(c *qt.C) {
	ctx := context.Background()

	env := jimmtest.ParseEnvironment(c, testGetControllerAccessEnv)
	env.PopulateDBAndPermissions(c, s.ctlTag, s.db, s.ofgaClient)

	access, err := s.manager.GetJimmControllerAccess(ctx, s.adminUser, s.adminUser.ResourceTag())
	c.Assert(err, qt.IsNil)
	c.Check(access, qt.Equals, "superuser")

	access, err = s.manager.GetJimmControllerAccess(ctx, s.adminUser, s.user.ResourceTag())
	c.Assert(err, qt.IsNil)
	c.Check(access, qt.Equals, "login")

	access, err = s.manager.GetJimmControllerAccess(ctx, s.adminUser, names.NewUserTag("charlie@canonical.com"))
	c.Assert(err, qt.IsNil)
	c.Check(access, qt.Equals, "login")

	access, err = s.manager.GetJimmControllerAccess(ctx, s.user, s.user.ResourceTag())
	c.Assert(err, qt.IsNil)
	c.Check(access, qt.Equals, "login")

	_, err = s.manager.GetJimmControllerAccess(ctx, s.user, names.NewUserTag("alice@canonical.com"))
	c.Assert(err, qt.ErrorMatches, "unauthorized")
}

const grantModelAccessTestEnv = `clouds:
- name: test-cloud
  type: test-provider
  regions:
  - name: test-cloud-region
cloud-credentials:
- owner: alice@canonical.com
  name: cred-1
  cloud: test-cloud
controllers:
- name: controller-1
  uuid: 00000001-0000-0000-0000-000000000001
  cloud: test-cloud
  region: test-cloud-region
models:
- name: model-1
  uuid: 00000002-0000-0000-0000-000000000001
  controller: controller-1
  cloud: test-cloud
  region: test-cloud-region
  cloud-credential: cred-1
  owner: alice@canonical.com
  users:
  - user: alice@canonical.com
    access: admin
  - user: charlie@canonical.com
    access: write
`

var grantModelAccessTests = []struct {
	name            string
	env             string
	username        string
	uuid            string
	targetUsername  string
	access          string
	expectRelations []openfga.Tuple
	expectError     string
	expectErrorCode errors.Code
}{{
	name:            "ModelNotFound",
	username:        "alice@canonical.com",
	uuid:            "00000002-0000-0000-0000-000000000001",
	targetUsername:  "bob@canonical.com",
	access:          "write",
	expectError:     `unauthorized`,
	expectErrorCode: errors.CodeUnauthorized,
}, {
	name:           "Admin grants 'admin' access to a user with no access",
	env:            grantModelAccessTestEnv,
	username:       "alice@canonical.com",
	uuid:           "00000002-0000-0000-0000-000000000001",
	targetUsername: "bob@canonical.com",
	access:         "admin",
	expectRelations: []openfga.Tuple{{
		Object:   ofganames.ConvertTag(names.NewUserTag("alice@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("bob@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("charlie@canonical.com")),
		Relation: ofganames.WriterRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}},
}, {
	name:           "Admin grants 'write' access to a user with no access",
	env:            grantModelAccessTestEnv,
	username:       "alice@canonical.com",
	uuid:           "00000002-0000-0000-0000-000000000001",
	targetUsername: "bob@canonical.com",
	access:         "write",
	expectRelations: []openfga.Tuple{{
		Object:   ofganames.ConvertTag(names.NewUserTag("alice@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("bob@canonical.com")),
		Relation: ofganames.WriterRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("charlie@canonical.com")),
		Relation: ofganames.WriterRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}},
}, {
	name:           "Admin grants 'read' access to a user with no access",
	env:            grantModelAccessTestEnv,
	username:       "alice@canonical.com",
	uuid:           "00000002-0000-0000-0000-000000000001",
	targetUsername: "bob@canonical.com",
	access:         "read",
	expectRelations: []openfga.Tuple{{
		Object:   ofganames.ConvertTag(names.NewUserTag("alice@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("bob@canonical.com")),
		Relation: ofganames.ReaderRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("charlie@canonical.com")),
		Relation: ofganames.WriterRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}},
}, {
	name:           "Admin grants 'write' access to a user who already has 'write' access",
	env:            grantModelAccessTestEnv,
	username:       "alice@canonical.com",
	uuid:           "00000002-0000-0000-0000-000000000001",
	targetUsername: "charlie@canonical.com",
	access:         "write",
	expectRelations: []openfga.Tuple{{
		Object:   ofganames.ConvertTag(names.NewUserTag("alice@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("charlie@canonical.com")),
		Relation: ofganames.WriterRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}},
}, {
	name:           "Admin grants 'read' access to a user who already has 'write' access",
	env:            grantModelAccessTestEnv,
	username:       "alice@canonical.com",
	uuid:           "00000002-0000-0000-0000-000000000001",
	targetUsername: "charlie@canonical.com",
	access:         "read",
	expectRelations: []openfga.Tuple{{
		Object:   ofganames.ConvertTag(names.NewUserTag("alice@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("charlie@canonical.com")),
		Relation: ofganames.WriterRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}},
}, {
	name:           "Admin grants 'admin' access to themselves",
	env:            grantModelAccessTestEnv,
	username:       "alice@canonical.com",
	uuid:           "00000002-0000-0000-0000-000000000001",
	targetUsername: "alice@canonical.com",
	access:         "admin",
	expectRelations: []openfga.Tuple{{
		Object:   ofganames.ConvertTag(names.NewUserTag("alice@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("charlie@canonical.com")),
		Relation: ofganames.WriterRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}},
}, {
	name:           "Admin grants 'write' access to themselves",
	env:            grantModelAccessTestEnv,
	username:       "alice@canonical.com",
	uuid:           "00000002-0000-0000-0000-000000000001",
	targetUsername: "alice@canonical.com",
	access:         "write",
	expectRelations: []openfga.Tuple{{
		Object:   ofganames.ConvertTag(names.NewUserTag("alice@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("charlie@canonical.com")),
		Relation: ofganames.WriterRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}},
}, {
	name:           "Admin grants 'read' access to themselves",
	env:            grantModelAccessTestEnv,
	username:       "alice@canonical.com",
	uuid:           "00000002-0000-0000-0000-000000000001",
	targetUsername: "alice@canonical.com",
	access:         "read",
	expectRelations: []openfga.Tuple{{
		Object:   ofganames.ConvertTag(names.NewUserTag("alice@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("charlie@canonical.com")),
		Relation: ofganames.WriterRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}},
}, {
	name:            "UserNotAuthorized",
	env:             grantModelAccessTestEnv,
	username:        "charlie@canonical.com",
	uuid:            "00000002-0000-0000-0000-000000000001",
	targetUsername:  "bob@canonical.com",
	access:          "write",
	expectError:     `unauthorized`,
	expectErrorCode: errors.CodeUnauthorized,
}, {
	name:           "unknown access",
	env:            grantModelAccessTestEnv,
	username:       "alice@canonical.com",
	uuid:           "00000002-0000-0000-0000-000000000001",
	targetUsername: "bob@canonical.com",
	access:         "some-unknown-access",
	expectError:    `failed to recognize given access: "some-unknown-access"`,
}}

func TestGrantModelAccess(t *testing.T) {
	c := qt.New(t)
	for _, tt := range grantModelAccessTests {
		c.Run(tt.name, func(c *qt.C) {
			ctx := context.Background()

			j := jimmtest.NewJIMM(c, &jimm.Parameters{})

			env := jimmtest.ParseEnvironment(c, tt.env)
			env.PopulateDBAndPermissions(c, j.ResourceTag(), j.Database, j.OpenFGAClient)

			dbUser := env.User(tt.username).DBObject(c, j.Database)
			user := openfga.NewUser(&dbUser, j.OpenFGAClient)

			err := j.PermissionManager().GrantModelAccess(ctx, user, names.NewModelTag(tt.uuid), names.NewUserTag(tt.targetUsername), jujuparams.UserAccessPermission(tt.access))
			if tt.expectError != "" {
				c.Check(err, qt.ErrorMatches, tt.expectError)
				if tt.expectErrorCode != "" {
					c.Check(errors.ErrorCode(err), qt.Equals, tt.expectErrorCode)
				}
				return
			}
			c.Assert(err, qt.IsNil)
			for _, tuple := range tt.expectRelations {
				value, err := j.OpenFGAClient.CheckRelation(ctx, tuple, false)
				c.Assert(err, qt.IsNil)
				c.Assert(value, qt.IsTrue, qt.Commentf("expected the tuple to exist after granting"))
			}
		})
	}
}

const revokeModelAccessTestEnv = `clouds:
- name: test-cloud
  type: test-provider
  regions:
  - name: test-cloud-region
cloud-credentials:
- owner: alice@canonical.com
  name: cred-1
  cloud: test-cloud
controllers:
- name: controller-1
  uuid: 00000001-0000-0000-0000-000000000001
  cloud: test-cloud
  region: test-cloud-region
models:
- name: model-1
  uuid: 00000002-0000-0000-0000-000000000001
  controller: controller-1
  cloud: test-cloud
  region: test-cloud-region
  cloud-credential: cred-1
  owner: alice@canonical.com
  users:
  - user: alice@canonical.com
    access: admin
  - user: bob@canonical.com
    access: admin
  - user: charlie@canonical.com
    access: write
  - user: daphne@canonical.com
    access: read
- name: model-2
  uuid: 00000002-0000-0000-0000-000000000002
  controller: controller-1
  cloud: test-cloud
  region: test-cloud-region
  cloud-credential: cred-1
  owner: alice@canonical.com
  users:
  - user: alice@canonical.com
    access: admin
  - user: earl@canonical.com
    access: admin
`

var revokeModelAccessTests = []struct {
	name                   string
	env                    string
	username               string
	uuid                   string
	targetUsername         string
	access                 string
	extraInitialTuples     []openfga.Tuple
	expectRelations        []openfga.Tuple
	expectRemovedRelations []openfga.Tuple
	expectError            string
	expectErrorCode        errors.Code
}{{
	name:            "ModelNotFound",
	username:        "alice@canonical.com",
	uuid:            "00000002-0000-0000-0000-000000000001",
	targetUsername:  "bob@canonical.com",
	access:          "write",
	expectError:     `unauthorized`,
	expectErrorCode: errors.CodeUnauthorized,
}, {
	name:           "Admin revokes 'admin' access from another admin",
	env:            revokeModelAccessTestEnv,
	username:       "alice@canonical.com",
	uuid:           "00000002-0000-0000-0000-000000000001",
	targetUsername: "bob@canonical.com",
	access:         "admin",
	expectRelations: []openfga.Tuple{{
		Object:   ofganames.ConvertTag(names.NewUserTag("alice@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("charlie@canonical.com")),
		Relation: ofganames.WriterRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("daphne@canonical.com")),
		Relation: ofganames.ReaderRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}},
	expectRemovedRelations: []openfga.Tuple{{
		Object:   ofganames.ConvertTag(names.NewUserTag("bob@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}},
}, {
	name:           "Admin revokes 'write' access from another admin",
	env:            revokeModelAccessTestEnv,
	username:       "alice@canonical.com",
	uuid:           "00000002-0000-0000-0000-000000000001",
	targetUsername: "bob@canonical.com",
	access:         "write",
	expectRelations: []openfga.Tuple{{
		Object:   ofganames.ConvertTag(names.NewUserTag("alice@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("charlie@canonical.com")),
		Relation: ofganames.WriterRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("daphne@canonical.com")),
		Relation: ofganames.ReaderRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}},
	expectRemovedRelations: []openfga.Tuple{{
		Object:   ofganames.ConvertTag(names.NewUserTag("bob@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}},
}, {
	name:           "Admin revokes 'read' access from another admin",
	env:            revokeModelAccessTestEnv,
	username:       "alice@canonical.com",
	uuid:           "00000002-0000-0000-0000-000000000001",
	targetUsername: "bob@canonical.com",
	access:         "read",
	expectRelations: []openfga.Tuple{{
		Object:   ofganames.ConvertTag(names.NewUserTag("alice@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("charlie@canonical.com")),
		Relation: ofganames.WriterRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("daphne@canonical.com")),
		Relation: ofganames.ReaderRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}},
	expectRemovedRelations: []openfga.Tuple{{
		Object:   ofganames.ConvertTag(names.NewUserTag("bob@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}},
}, {
	name:           "Admin revokes 'admin' access from a user who has 'write' access",
	env:            revokeModelAccessTestEnv,
	username:       "alice@canonical.com",
	uuid:           "00000002-0000-0000-0000-000000000001",
	targetUsername: "charlie@canonical.com",
	access:         "admin",
	expectRelations: []openfga.Tuple{{
		Object:   ofganames.ConvertTag(names.NewUserTag("alice@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("bob@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("charlie@canonical.com")),
		Relation: ofganames.WriterRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("daphne@canonical.com")),
		Relation: ofganames.ReaderRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}},
}, {
	name:           "Admin revokes 'write' access from a user who has 'write' access",
	env:            revokeModelAccessTestEnv,
	username:       "alice@canonical.com",
	uuid:           "00000002-0000-0000-0000-000000000001",
	targetUsername: "charlie@canonical.com",
	access:         "write",
	expectRelations: []openfga.Tuple{{
		Object:   ofganames.ConvertTag(names.NewUserTag("alice@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("bob@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("daphne@canonical.com")),
		Relation: ofganames.ReaderRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}},
	expectRemovedRelations: []openfga.Tuple{{
		Object:   ofganames.ConvertTag(names.NewUserTag("charlie@canonical.com")),
		Relation: ofganames.WriterRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}},
}, {
	name:           "Admin revokes 'read' access from a user who has 'write' access",
	env:            revokeModelAccessTestEnv,
	username:       "alice@canonical.com",
	uuid:           "00000002-0000-0000-0000-000000000001",
	targetUsername: "charlie@canonical.com",
	access:         "read",
	expectRelations: []openfga.Tuple{{
		Object:   ofganames.ConvertTag(names.NewUserTag("alice@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("bob@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("daphne@canonical.com")),
		Relation: ofganames.ReaderRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}},
	expectRemovedRelations: []openfga.Tuple{{
		Object:   ofganames.ConvertTag(names.NewUserTag("charlie@canonical.com")),
		Relation: ofganames.WriterRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}},
}, {
	name:           "Admin revokes 'admin' access from a user who has 'read' access",
	env:            revokeModelAccessTestEnv,
	username:       "alice@canonical.com",
	uuid:           "00000002-0000-0000-0000-000000000001",
	targetUsername: "daphne@canonical.com",
	access:         "admin",
	expectRelations: []openfga.Tuple{{
		Object:   ofganames.ConvertTag(names.NewUserTag("alice@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("bob@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("charlie@canonical.com")),
		Relation: ofganames.WriterRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("daphne@canonical.com")),
		Relation: ofganames.ReaderRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}},
}, {
	name:           "Admin revokes 'write' access from a user who has 'read' access",
	env:            revokeModelAccessTestEnv,
	username:       "alice@canonical.com",
	uuid:           "00000002-0000-0000-0000-000000000001",
	targetUsername: "daphne@canonical.com",
	access:         "write",
	expectRelations: []openfga.Tuple{{
		Object:   ofganames.ConvertTag(names.NewUserTag("alice@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("bob@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("charlie@canonical.com")),
		Relation: ofganames.WriterRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("daphne@canonical.com")),
		Relation: ofganames.ReaderRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}},
}, {
	name:           "Admin revokes 'read' access from a user who has 'read' access",
	env:            revokeModelAccessTestEnv,
	username:       "alice@canonical.com",
	uuid:           "00000002-0000-0000-0000-000000000001",
	targetUsername: "daphne@canonical.com",
	access:         "read",
	expectRelations: []openfga.Tuple{{
		Object:   ofganames.ConvertTag(names.NewUserTag("alice@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("bob@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("charlie@canonical.com")),
		Relation: ofganames.WriterRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}},
	expectRemovedRelations: []openfga.Tuple{{
		Object:   ofganames.ConvertTag(names.NewUserTag("daphne@canonical.com")),
		Relation: ofganames.ReaderRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}},
}, {
	name:           "Admin revokes 'admin' access from themselves",
	env:            revokeModelAccessTestEnv,
	username:       "alice@canonical.com",
	uuid:           "00000002-0000-0000-0000-000000000001",
	targetUsername: "alice@canonical.com",
	access:         "admin",
	expectRelations: []openfga.Tuple{{
		Object:   ofganames.ConvertTag(names.NewUserTag("bob@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("charlie@canonical.com")),
		Relation: ofganames.WriterRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("daphne@canonical.com")),
		Relation: ofganames.ReaderRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}},
	expectRemovedRelations: []openfga.Tuple{{
		Object:   ofganames.ConvertTag(names.NewUserTag("alice@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}},
}, {
	name:           "Admin revokes 'write' access from themselves",
	env:            revokeModelAccessTestEnv,
	username:       "alice@canonical.com",
	uuid:           "00000002-0000-0000-0000-000000000001",
	targetUsername: "alice@canonical.com",
	access:         "write",
	expectRelations: []openfga.Tuple{{
		Object:   ofganames.ConvertTag(names.NewUserTag("bob@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("charlie@canonical.com")),
		Relation: ofganames.WriterRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("daphne@canonical.com")),
		Relation: ofganames.ReaderRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}},
	expectRemovedRelations: []openfga.Tuple{{
		Object:   ofganames.ConvertTag(names.NewUserTag("alice@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}},
}, {
	name:           "Admin revokes 'read' access from themselves",
	env:            revokeModelAccessTestEnv,
	username:       "alice@canonical.com",
	uuid:           "00000002-0000-0000-0000-000000000001",
	targetUsername: "alice@canonical.com",
	access:         "read",
	expectRelations: []openfga.Tuple{{
		Object:   ofganames.ConvertTag(names.NewUserTag("bob@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("charlie@canonical.com")),
		Relation: ofganames.WriterRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("daphne@canonical.com")),
		Relation: ofganames.ReaderRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}},
	expectRemovedRelations: []openfga.Tuple{{
		Object:   ofganames.ConvertTag(names.NewUserTag("alice@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}},
}, {
	name:           "Writer revokes 'admin' access from themselves",
	env:            revokeModelAccessTestEnv,
	username:       "charlie@canonical.com",
	uuid:           "00000002-0000-0000-0000-000000000001",
	targetUsername: "charlie@canonical.com",
	access:         "admin",
	expectRelations: []openfga.Tuple{{
		Object:   ofganames.ConvertTag(names.NewUserTag("alice@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("bob@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("charlie@canonical.com")),
		Relation: ofganames.WriterRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("daphne@canonical.com")),
		Relation: ofganames.ReaderRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}},
}, {
	name:           "Writer revokes 'write' access from themselves",
	env:            revokeModelAccessTestEnv,
	username:       "charlie@canonical.com",
	uuid:           "00000002-0000-0000-0000-000000000001",
	targetUsername: "charlie@canonical.com",
	access:         "write",
	expectRelations: []openfga.Tuple{{
		Object:   ofganames.ConvertTag(names.NewUserTag("alice@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("bob@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("daphne@canonical.com")),
		Relation: ofganames.ReaderRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}},
	expectRemovedRelations: []openfga.Tuple{{
		Object:   ofganames.ConvertTag(names.NewUserTag("charlie@canonical.com")),
		Relation: ofganames.WriterRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}},
}, {
	name:           "Writer revokes 'read' access from themselves",
	env:            revokeModelAccessTestEnv,
	username:       "charlie@canonical.com",
	uuid:           "00000002-0000-0000-0000-000000000001",
	targetUsername: "charlie@canonical.com",
	access:         "read",
	expectRelations: []openfga.Tuple{{
		Object:   ofganames.ConvertTag(names.NewUserTag("alice@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("bob@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("daphne@canonical.com")),
		Relation: ofganames.ReaderRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}},
	expectRemovedRelations: []openfga.Tuple{{
		Object:   ofganames.ConvertTag(names.NewUserTag("charlie@canonical.com")),
		Relation: ofganames.WriterRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}},
}, {
	name:           "Reader revokes 'admin' access from themselves",
	env:            revokeModelAccessTestEnv,
	username:       "daphne@canonical.com",
	uuid:           "00000002-0000-0000-0000-000000000001",
	targetUsername: "daphne@canonical.com",
	access:         "admin",
	expectRelations: []openfga.Tuple{{
		Object:   ofganames.ConvertTag(names.NewUserTag("alice@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("bob@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("charlie@canonical.com")),
		Relation: ofganames.WriterRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("daphne@canonical.com")),
		Relation: ofganames.ReaderRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}},
}, {
	name:           "Reader revokes 'write' access from themselves",
	env:            revokeModelAccessTestEnv,
	username:       "daphne@canonical.com",
	uuid:           "00000002-0000-0000-0000-000000000001",
	targetUsername: "daphne@canonical.com",
	access:         "write",
	expectRelations: []openfga.Tuple{{
		Object:   ofganames.ConvertTag(names.NewUserTag("alice@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("bob@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("charlie@canonical.com")),
		Relation: ofganames.WriterRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("daphne@canonical.com")),
		Relation: ofganames.ReaderRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}},
}, {
	name:           "Reader revokes 'read' access from themselves",
	env:            revokeModelAccessTestEnv,
	username:       "daphne@canonical.com",
	uuid:           "00000002-0000-0000-0000-000000000001",
	targetUsername: "daphne@canonical.com",
	access:         "read",
	expectRelations: []openfga.Tuple{{
		Object:   ofganames.ConvertTag(names.NewUserTag("alice@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("bob@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("charlie@canonical.com")),
		Relation: ofganames.WriterRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}},
	expectRemovedRelations: []openfga.Tuple{{
		Object:   ofganames.ConvertTag(names.NewUserTag("daphne@canonical.com")),
		Relation: ofganames.ReaderRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}},
}, {
	name:           "Admin revokes 'admin' access from a user who has separate tuples for all accesses (read/write/admin)",
	env:            revokeModelAccessTestEnv,
	username:       "alice@canonical.com",
	uuid:           "00000002-0000-0000-0000-000000000001",
	targetUsername: "daphne@canonical.com",
	access:         "admin",
	extraInitialTuples: []openfga.Tuple{{
		Object:   ofganames.ConvertTag(names.NewUserTag("daphne@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("daphne@canonical.com")),
		Relation: ofganames.WriterRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	},
	// No need to add the 'read' relation, because it's already there due to the environment setup.
	},
	expectRelations: []openfga.Tuple{{
		Object:   ofganames.ConvertTag(names.NewUserTag("alice@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("bob@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("charlie@canonical.com")),
		Relation: ofganames.WriterRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("daphne@canonical.com")),
		Relation: ofganames.WriterRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("daphne@canonical.com")),
		Relation: ofganames.ReaderRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}},
	expectRemovedRelations: []openfga.Tuple{{
		Object:   ofganames.ConvertTag(names.NewUserTag("daphne@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}},
}, {
	name:           "Admin revokes 'write' access from a user who has separate tuples for all accesses (read/write/admin)",
	env:            revokeModelAccessTestEnv,
	username:       "alice@canonical.com",
	uuid:           "00000002-0000-0000-0000-000000000001",
	targetUsername: "daphne@canonical.com",
	access:         "write",
	extraInitialTuples: []openfga.Tuple{{
		Object:   ofganames.ConvertTag(names.NewUserTag("daphne@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("daphne@canonical.com")),
		Relation: ofganames.WriterRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	},
	// No need to add the 'read' relation, because it's already there due to the environment setup.
	},
	expectRelations: []openfga.Tuple{{
		Object:   ofganames.ConvertTag(names.NewUserTag("alice@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("bob@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("charlie@canonical.com")),
		Relation: ofganames.WriterRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("daphne@canonical.com")),
		Relation: ofganames.ReaderRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}},
	expectRemovedRelations: []openfga.Tuple{{
		Object:   ofganames.ConvertTag(names.NewUserTag("daphne@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("daphne@canonical.com")),
		Relation: ofganames.WriterRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}},
}, {
	name:           "Admin revokes 'read' access from a user who has separate tuples for all accesses (read/write/admin)",
	env:            revokeModelAccessTestEnv,
	username:       "alice@canonical.com",
	uuid:           "00000002-0000-0000-0000-000000000001",
	targetUsername: "daphne@canonical.com",
	access:         "read",
	extraInitialTuples: []openfga.Tuple{{
		Object:   ofganames.ConvertTag(names.NewUserTag("daphne@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("daphne@canonical.com")),
		Relation: ofganames.WriterRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	},
	// No need to add the 'read' relation, because it's already there due to the environment setup.
	},
	expectRelations: []openfga.Tuple{{
		Object:   ofganames.ConvertTag(names.NewUserTag("alice@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("bob@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("charlie@canonical.com")),
		Relation: ofganames.WriterRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}},
	expectRemovedRelations: []openfga.Tuple{{
		Object:   ofganames.ConvertTag(names.NewUserTag("daphne@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("daphne@canonical.com")),
		Relation: ofganames.WriterRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("daphne@canonical.com")),
		Relation: ofganames.ReaderRelation,
		Target:   ofganames.ConvertTag(names.NewModelTag("00000002-0000-0000-0000-000000000001")),
	}},
}, {
	name:            "UserNotAuthorized",
	env:             revokeModelAccessTestEnv,
	username:        "charlie@canonical.com",
	uuid:            "00000002-0000-0000-0000-000000000001",
	targetUsername:  "bob@canonical.com",
	access:          "write",
	expectError:     `unauthorized`,
	expectErrorCode: errors.CodeUnauthorized,
}, {
	name:           "unknown access",
	env:            revokeModelAccessTestEnv,
	username:       "alice@canonical.com",
	uuid:           "00000002-0000-0000-0000-000000000001",
	targetUsername: "bob@canonical.com",
	access:         "some-unknown-access",
	expectError:    `failed to recognize given access: "some-unknown-access"`,
}}

//nolint:gocognit
func TestRevokeModelAccess(t *testing.T) {
	c := qt.New(t)

	for _, tt := range revokeModelAccessTests {
		c.Run(tt.name, func(c *qt.C) {
			ctx := context.Background()

			j := jimmtest.NewJIMM(c, &jimm.Parameters{})

			env := jimmtest.ParseEnvironment(c, tt.env)
			env.PopulateDBAndPermissions(c, j.ResourceTag(), j.Database, j.OpenFGAClient)

			if len(tt.extraInitialTuples) > 0 {
				err := j.OpenFGAClient.AddRelation(ctx, tt.extraInitialTuples...)
				c.Assert(err, qt.IsNil)
			}

			if tt.expectRemovedRelations != nil {
				for _, tuple := range tt.expectRemovedRelations {
					value, err := j.OpenFGAClient.CheckRelation(ctx, tuple, false)
					c.Assert(err, qt.IsNil)
					c.Assert(value, qt.IsTrue, qt.Commentf("expected the tuple to exist before revoking"))
				}
			}

			dbUser := env.User(tt.username).DBObject(c, j.Database)
			user := openfga.NewUser(&dbUser, j.OpenFGAClient)

			err := j.PermissionManager().RevokeModelAccess(ctx, user, names.NewModelTag(tt.uuid), names.NewUserTag(tt.targetUsername), jujuparams.UserAccessPermission(tt.access))
			if tt.expectError != "" {
				c.Check(err, qt.ErrorMatches, tt.expectError)
				if tt.expectErrorCode != "" {
					c.Check(errors.ErrorCode(err), qt.Equals, tt.expectErrorCode)
				}
				return
			}
			c.Assert(err, qt.IsNil)
			if tt.expectRemovedRelations != nil {
				for _, tuple := range tt.expectRemovedRelations {
					value, err := j.OpenFGAClient.CheckRelation(ctx, tuple, false)
					c.Assert(err, qt.IsNil)
					c.Assert(value, qt.IsFalse, qt.Commentf("expected the tuple to be removed after revoking"))
				}
			}
			if tt.expectRelations != nil {
				for _, tuple := range tt.expectRelations {
					value, err := j.OpenFGAClient.CheckRelation(ctx, tuple, false)
					c.Assert(err, qt.IsNil)
					c.Assert(value, qt.IsTrue, qt.Commentf("expected the tuple to exist after revoking"))
				}
			}
		})
	}
}

func TestDetermineAccessLevelAfterGrant(t *testing.T) {
	c := qt.New(t)

	tests := []struct {
		about               string
		currentAccessLevel  string
		grantAccessLevel    string
		expectedAccessLevel string
	}{{
		about:               "user has no access - grant admin",
		currentAccessLevel:  "",
		grantAccessLevel:    string(jujuparams.OfferAdminAccess),
		expectedAccessLevel: "admin",
	}, {
		about:               "user has no access - grant consume",
		currentAccessLevel:  "",
		grantAccessLevel:    string(jujuparams.OfferConsumeAccess),
		expectedAccessLevel: "consume",
	}, {
		about:               "user has no access - grant read",
		currentAccessLevel:  "",
		grantAccessLevel:    string(jujuparams.OfferReadAccess),
		expectedAccessLevel: "read",
	}, {
		about:               "user has read access - grant admin",
		currentAccessLevel:  "read",
		grantAccessLevel:    string(jujuparams.OfferAdminAccess),
		expectedAccessLevel: "admin",
	}, {
		about:               "user has read access - grant consume",
		currentAccessLevel:  "read",
		grantAccessLevel:    string(jujuparams.OfferConsumeAccess),
		expectedAccessLevel: "consume",
	}, {
		about:               "user has read access - grant read",
		currentAccessLevel:  "read",
		grantAccessLevel:    string(jujuparams.OfferReadAccess),
		expectedAccessLevel: "read",
	}, {
		about:               "user has consume access - grant admin",
		currentAccessLevel:  "consume",
		grantAccessLevel:    string(jujuparams.OfferAdminAccess),
		expectedAccessLevel: "admin",
	}, {
		about:               "user has consume access - grant consume",
		currentAccessLevel:  "consume",
		grantAccessLevel:    string(jujuparams.OfferConsumeAccess),
		expectedAccessLevel: "consume",
	}, {
		about:               "user has consume access - grant read",
		currentAccessLevel:  "consume",
		grantAccessLevel:    string(jujuparams.OfferReadAccess),
		expectedAccessLevel: "consume",
	}, {
		about:               "user has admin access - grant admin",
		currentAccessLevel:  "admin",
		grantAccessLevel:    string(jujuparams.OfferAdminAccess),
		expectedAccessLevel: "admin",
	}, {
		about:               "user has admin access - grant consume",
		currentAccessLevel:  "admin",
		grantAccessLevel:    string(jujuparams.OfferConsumeAccess),
		expectedAccessLevel: "admin",
	}, {
		about:               "user has admin access - grant read",
		currentAccessLevel:  "admin",
		grantAccessLevel:    string(jujuparams.OfferReadAccess),
		expectedAccessLevel: "admin",
	}}

	for _, test := range tests {
		c.Run(test.about, func(c *qt.C) {
			level := permissions.DetermineAccessLevelAfterGrant(test.currentAccessLevel, test.grantAccessLevel)
			c.Assert(level, qt.Equals, test.expectedAccessLevel)
		})
	}
}

const revokeAndGrantOfferAccessTestEnv = `clouds:
- name: test-cloud
  type: test-provider
  regions:
  - name: test-region-1
cloud-credentials:
- owner: alice@canonical.com
  name: test-credential-1
  cloud: test-cloud
controllers:
- name: test-controller-1
  uuid: 00000000-0000-0000-0000-0000-0000000000001
  cloud: test-cloud
  region: test-region-1
models:
- name: test-model
  uuid: 00000000-0000-0000-0000-0000-0000000000003
  controller: test-controller-1
  cloud: test-cloud
  region: test-region-1
  cloud-credential: test-credential-1
  owner: alice@canonical.com
  life: alive
application-offers:
- name: test-offer
  url: test-offer-url
  uuid: 00000000-0000-0000-0000-0000-0000000000011
  model-name: test-model
  model-owner: alice@canonical.com
  application-name: application-1
  application-description: app description 1
  users:
  - user: eve@canonical.com
    access: admin
  - user: jane@canonical.com
    access: admin
  - user: bob@canonical.com
    access: consume
  - user: fred@canonical.com
    access: read
users:
- username: grant@canonical.com
  controller-access: login
- username: joe@canonical.com
  controller-access: superuser
`

func TestRevokeOfferAccess(t *testing.T) {
	c := qt.New(t)

	ctx := context.Background()

	tests := []struct {
		about                      string
		parameterFunc              func(*jimmtest.Environment, *db.Database) (dbmodel.Identity, dbmodel.Identity, string, jujuparams.OfferAccessPermission)
		setup                      func(*jimmtest.Environment, *db.Database, *openfga.OFGAClient)
		expectedError              string
		expectedAccessLevel        string
		expectedAccessLevelOnError string // This expectation is meant to ensure there'll be no unpredicted behavior (like changing existing relations) after an error has occurred
	}{{
		about: "admin revokes a model admin user's admin access - an error returns (relation is indirect)",
		parameterFunc: func(env *jimmtest.Environment, db *db.Database) (dbmodel.Identity, dbmodel.Identity, string, jujuparams.OfferAccessPermission) {
			return env.User("eve@canonical.com").DBObject(c, db), env.User("alice@canonical.com").DBObject(c, db), "test-offer-url", jujuparams.OfferAdminAccess
		},
		expectedError:              "unable to completely revoke given access due to other relations.*",
		expectedAccessLevelOnError: "admin",
	}, {
		about: "model admin revokes an admin user admin access - user has no access",
		parameterFunc: func(env *jimmtest.Environment, db *db.Database) (dbmodel.Identity, dbmodel.Identity, string, jujuparams.OfferAccessPermission) {
			return env.User("alice@canonical.com").DBObject(c, db), env.User("eve@canonical.com").DBObject(c, db), "test-offer-url", jujuparams.OfferAdminAccess

		},
		expectedAccessLevel: "",
	}, {
		about: "admin revokes an admin user admin access - user has no access",
		parameterFunc: func(env *jimmtest.Environment, db *db.Database) (dbmodel.Identity, dbmodel.Identity, string, jujuparams.OfferAccessPermission) {
			return env.User("jane@canonical.com").DBObject(c, db), env.User("eve@canonical.com").DBObject(c, db), "test-offer-url", jujuparams.OfferAdminAccess
		},
		expectedAccessLevel: "",
	}, {
		about: "superuser revokes an admin user admin access - user has no access",
		parameterFunc: func(env *jimmtest.Environment, db *db.Database) (dbmodel.Identity, dbmodel.Identity, string, jujuparams.OfferAccessPermission) {
			return env.User("joe@canonical.com").DBObject(c, db), env.User("eve@canonical.com").DBObject(c, db), "test-offer-url", jujuparams.OfferAdminAccess
		},
		expectedAccessLevel: "",
	}, {
		about: "admin revokes an admin user read access - an error returns (no direct relation to remove)",
		parameterFunc: func(env *jimmtest.Environment, db *db.Database) (dbmodel.Identity, dbmodel.Identity, string, jujuparams.OfferAccessPermission) {
			return env.User("alice@canonical.com").DBObject(c, db), env.User("eve@canonical.com").DBObject(c, db), "test-offer-url", jujuparams.OfferReadAccess
		},
		expectedError:              "unable to completely revoke given access due to other relations.*",
		expectedAccessLevelOnError: "admin",
	}, {
		about: "admin revokes a consume user admin access - user keeps consume access",
		parameterFunc: func(env *jimmtest.Environment, db *db.Database) (dbmodel.Identity, dbmodel.Identity, string, jujuparams.OfferAccessPermission) {
			return env.User("alice@canonical.com").DBObject(c, db), env.User("bob@canonical.com").DBObject(c, db), "test-offer-url", jujuparams.OfferAdminAccess
		},
		expectedAccessLevel: "consume",
	}, {
		about: "admin revokes a consume user consume access - user has no access",
		parameterFunc: func(env *jimmtest.Environment, db *db.Database) (dbmodel.Identity, dbmodel.Identity, string, jujuparams.OfferAccessPermission) {
			return env.User("alice@canonical.com").DBObject(c, db), env.User("bob@canonical.com").DBObject(c, db), "test-offer-url", jujuparams.OfferConsumeAccess
		},
		expectedAccessLevel: "",
	}, {
		about: "admin revokes a consume user read access - user still has consume access",
		parameterFunc: func(env *jimmtest.Environment, db *db.Database) (dbmodel.Identity, dbmodel.Identity, string, jujuparams.OfferAccessPermission) {
			return env.User("alice@canonical.com").DBObject(c, db), env.User("bob@canonical.com").DBObject(c, db), "test-offer-url", jujuparams.OfferReadAccess
		},
		expectedError:              "unable to completely revoke given access due to other relations.*",
		expectedAccessLevelOnError: "consume",
	}, {
		about: "admin revokes a read user admin access - user keeps read access",
		parameterFunc: func(env *jimmtest.Environment, db *db.Database) (dbmodel.Identity, dbmodel.Identity, string, jujuparams.OfferAccessPermission) {
			return env.User("alice@canonical.com").DBObject(c, db), env.User("fred@canonical.com").DBObject(c, db), "test-offer-url", jujuparams.OfferAdminAccess
		},
		expectedAccessLevel: "read",
	}, {
		about: "admin revokes a read user consume access - user keeps read access",
		parameterFunc: func(env *jimmtest.Environment, db *db.Database) (dbmodel.Identity, dbmodel.Identity, string, jujuparams.OfferAccessPermission) {
			return env.User("alice@canonical.com").DBObject(c, db), env.User("fred@canonical.com").DBObject(c, db), "test-offer-url", jujuparams.OfferConsumeAccess
		},
		expectedAccessLevel: "read",
	}, {
		about: "admin revokes a read user read access - user has no access",
		parameterFunc: func(env *jimmtest.Environment, db *db.Database) (dbmodel.Identity, dbmodel.Identity, string, jujuparams.OfferAccessPermission) {
			return env.User("alice@canonical.com").DBObject(c, db), env.User("fred@canonical.com").DBObject(c, db), "test-offer-url", jujuparams.OfferReadAccess
		},
		expectedAccessLevel: "",
	}, {
		about: "admin tries to revoke access to user that does not have access - user continues to have no access",
		parameterFunc: func(env *jimmtest.Environment, db *db.Database) (dbmodel.Identity, dbmodel.Identity, string, jujuparams.OfferAccessPermission) {
			return env.User("alice@canonical.com").DBObject(c, db), env.User("grant@canonical.com").DBObject(c, db), "test-offer-url", jujuparams.OfferReadAccess
		},
		expectedAccessLevel: "",
	}, {
		about: "user with consume access cannot revoke access",
		parameterFunc: func(env *jimmtest.Environment, db *db.Database) (dbmodel.Identity, dbmodel.Identity, string, jujuparams.OfferAccessPermission) {
			return env.User("bob@canonical.com").DBObject(c, db), env.User("fred@canonical.com").DBObject(c, db), "test-offer-url", jujuparams.OfferReadAccess
		},
		expectedError: "unauthorized",
	}, {
		about: "user with read access cannot revoke access",
		parameterFunc: func(env *jimmtest.Environment, db *db.Database) (dbmodel.Identity, dbmodel.Identity, string, jujuparams.OfferAccessPermission) {
			return env.User("fred@canonical.com").DBObject(c, db), env.User("fred@canonical.com").DBObject(c, db), "test-offer-url", jujuparams.OfferReadAccess
		},
		expectedError: "unauthorized",
	}, {
		about: "no such offer",
		parameterFunc: func(env *jimmtest.Environment, db *db.Database) (dbmodel.Identity, dbmodel.Identity, string, jujuparams.OfferAccessPermission) {
			return env.User("fred@canonical.com").DBObject(c, db), env.User("fred@canonical.com").DBObject(c, db), "no-such-offer", jujuparams.OfferReadAccess
		},
		expectedError: "application offer not found",
	}, {
		about: "admin revokes another user (who is direct admin+consumer) their consume access - an error returns (saying user still has access; hinting to use 'jimmctl' for advanced cases)",
		parameterFunc: func(env *jimmtest.Environment, db *db.Database) (dbmodel.Identity, dbmodel.Identity, string, jujuparams.OfferAccessPermission) {
			return env.User("eve@canonical.com").DBObject(c, db), env.User("grant@canonical.com").DBObject(c, db), "test-offer-url", jujuparams.OfferConsumeAccess
		},
		setup: func(env *jimmtest.Environment, db *db.Database, client *openfga.OFGAClient) {
			u := env.User("grant@canonical.com").DBObject(c, db)
			offer := env.ApplicationOffer("test-offer-url").DBObject(c, db)
			err := openfga.NewUser(&u, client).SetApplicationOfferAccess(ctx, offer.ResourceTag(), ofganames.ConsumerRelation)
			c.Assert(err, qt.IsNil)
			err = openfga.NewUser(&u, client).SetApplicationOfferAccess(ctx, offer.ResourceTag(), ofganames.AdministratorRelation)
			c.Assert(err, qt.IsNil)
		},
		expectedError:              "unable to completely revoke given access due to other relations.*jimmctl.*",
		expectedAccessLevelOnError: "admin",
	}, {
		about: "admin revokes another user (who is direct admin+reader) their read access - an error returns (saying user still has access; hinting to use 'jimmctl' for advanced cases)",
		parameterFunc: func(env *jimmtest.Environment, db *db.Database) (dbmodel.Identity, dbmodel.Identity, string, jujuparams.OfferAccessPermission) {
			return env.User("eve@canonical.com").DBObject(c, db), env.User("grant@canonical.com").DBObject(c, db), "test-offer-url", jujuparams.OfferReadAccess
		},
		setup: func(env *jimmtest.Environment, db *db.Database, client *openfga.OFGAClient) {
			u := env.User("grant@canonical.com").DBObject(c, db)
			offer := env.ApplicationOffer("test-offer-url").DBObject(c, db)
			err := openfga.NewUser(&u, client).SetApplicationOfferAccess(ctx, offer.ResourceTag(), ofganames.ReaderRelation)
			c.Assert(err, qt.IsNil)
			err = openfga.NewUser(&u, client).SetApplicationOfferAccess(ctx, offer.ResourceTag(), ofganames.AdministratorRelation)
			c.Assert(err, qt.IsNil)
		},
		expectedError:              "unable to completely revoke given access due to other relations.*jimmctl.*",
		expectedAccessLevelOnError: "admin",
	}, {
		about: "admin revokes another user (who is direct consumer+reader) their read access - an error returns (saying user still has access; hinting to use 'jimmctl' for advanced cases)",
		parameterFunc: func(env *jimmtest.Environment, db *db.Database) (dbmodel.Identity, dbmodel.Identity, string, jujuparams.OfferAccessPermission) {
			return env.User("eve@canonical.com").DBObject(c, db), env.User("grant@canonical.com").DBObject(c, db), "test-offer-url", jujuparams.OfferReadAccess
		},
		setup: func(env *jimmtest.Environment, db *db.Database, client *openfga.OFGAClient) {
			u := env.User("grant@canonical.com").DBObject(c, db)
			offer := env.ApplicationOffer("test-offer-url").DBObject(c, db)
			err := openfga.NewUser(&u, client).SetApplicationOfferAccess(ctx, offer.ResourceTag(), ofganames.ReaderRelation)
			c.Assert(err, qt.IsNil)
			err = openfga.NewUser(&u, client).SetApplicationOfferAccess(ctx, offer.ResourceTag(), ofganames.ConsumerRelation)
			c.Assert(err, qt.IsNil)
		},
		expectedError:              "unable to completely revoke given access due to other relations.*jimmctl.*",
		expectedAccessLevelOnError: "consume",
	}}

	for _, test := range tests {
		c.Run(test.about, func(c *qt.C) {
			j := jimmtest.NewJIMM(c, &jimm.Parameters{})

			env := jimmtest.ParseEnvironment(c, revokeAndGrantOfferAccessTestEnv)
			env.PopulateDBAndPermissions(c, j.ResourceTag(), j.Database, j.OpenFGAClient)

			if test.setup != nil {
				test.setup(env, j.Database, j.OpenFGAClient)
			}
			authenticatedUser, offerUser, offerURL, revokeAccessLevel := test.parameterFunc(env, j.Database)

			assertAppliedRelation := func(expectedAppliedRelation string) {
				offer := dbmodel.ApplicationOffer{
					URL: offerURL,
				}
				err := j.Database.GetApplicationOffer(ctx, &offer)
				c.Assert(err, qt.IsNil)
				appliedRelation := openfga.NewUser(&offerUser, j.OpenFGAClient).GetApplicationOfferAccess(ctx, offer.ResourceTag())
				c.Assert(permissions.ToOfferAccessString(appliedRelation), qt.Equals, expectedAppliedRelation)
			}

			err := j.PermissionManager().RevokeOfferAccess(ctx, openfga.NewUser(&authenticatedUser, j.OpenFGAClient), offerURL, offerUser.ResourceTag(), revokeAccessLevel)
			if test.expectedError == "" {
				c.Assert(err, qt.IsNil)
				assertAppliedRelation(test.expectedAccessLevel)
			} else {
				c.Assert(err, qt.ErrorMatches, test.expectedError)
				if test.expectedAccessLevelOnError != "" {
					assertAppliedRelation(test.expectedAccessLevelOnError)
				}
			}
		})
	}
}

func TestGrantOfferAccess(t *testing.T) {
	c := qt.New(t)

	ctx := context.Background()

	tests := []struct {
		about               string
		parameterFunc       func(*jimmtest.Environment, *db.Database) (dbmodel.Identity, dbmodel.Identity, string, jujuparams.OfferAccessPermission)
		expectedError       string
		expectedAccessLevel string
	}{{
		about: "model admin grants an admin user admin access - admin user keeps admin",
		parameterFunc: func(env *jimmtest.Environment, db *db.Database) (dbmodel.Identity, dbmodel.Identity, string, jujuparams.OfferAccessPermission) {
			return env.User("alice@canonical.com").DBObject(c, db), env.User("eve@canonical.com").DBObject(c, db), "test-offer-url", jujuparams.OfferAdminAccess
		},
		expectedAccessLevel: "admin",
	}, {
		about: "model admin grants an admin user consume access - admin user keeps admin",
		parameterFunc: func(env *jimmtest.Environment, db *db.Database) (dbmodel.Identity, dbmodel.Identity, string, jujuparams.OfferAccessPermission) {
			return env.User("alice@canonical.com").DBObject(c, db), env.User("eve@canonical.com").DBObject(c, db), "test-offer-url", jujuparams.OfferConsumeAccess
		},
		expectedAccessLevel: "admin",
	}, {
		about: "model admin grants an admin user read access - admin user keeps admin",
		parameterFunc: func(env *jimmtest.Environment, db *db.Database) (dbmodel.Identity, dbmodel.Identity, string, jujuparams.OfferAccessPermission) {
			return env.User("alice@canonical.com").DBObject(c, db), env.User("eve@canonical.com").DBObject(c, db), "test-offer-url", jujuparams.OfferReadAccess
		},
		expectedAccessLevel: "admin",
	}, {
		about: "model admin grants a consume user admin access - user gets admin access",
		parameterFunc: func(env *jimmtest.Environment, db *db.Database) (dbmodel.Identity, dbmodel.Identity, string, jujuparams.OfferAccessPermission) {
			return env.User("alice@canonical.com").DBObject(c, db), env.User("bob@canonical.com").DBObject(c, db), "test-offer-url", jujuparams.OfferAdminAccess
		},
		expectedAccessLevel: "admin",
	}, {
		about: "admin grants a consume user admin access - user gets admin access",
		parameterFunc: func(env *jimmtest.Environment, db *db.Database) (dbmodel.Identity, dbmodel.Identity, string, jujuparams.OfferAccessPermission) {
			return env.User("jane@canonical.com").DBObject(c, db), env.User("bob@canonical.com").DBObject(c, db), "test-offer-url", jujuparams.OfferAdminAccess
		},
		expectedAccessLevel: "admin",
	}, {
		about: "superuser grants a consume user admin access - user gets admin access",
		parameterFunc: func(env *jimmtest.Environment, db *db.Database) (dbmodel.Identity, dbmodel.Identity, string, jujuparams.OfferAccessPermission) {
			return env.User("joe@canonical.com").DBObject(c, db), env.User("bob@canonical.com").DBObject(c, db), "test-offer-url", jujuparams.OfferAdminAccess
		},
		expectedAccessLevel: "admin",
	}, {
		about: "admin grants a consume user consume access - user keeps consume access",
		parameterFunc: func(env *jimmtest.Environment, db *db.Database) (dbmodel.Identity, dbmodel.Identity, string, jujuparams.OfferAccessPermission) {
			return env.User("alice@canonical.com").DBObject(c, db), env.User("bob@canonical.com").DBObject(c, db), "test-offer-url", jujuparams.OfferConsumeAccess
		},
		expectedAccessLevel: "consume",
	}, {
		about: "admin grants a consume user read access - use keeps consume access",
		parameterFunc: func(env *jimmtest.Environment, db *db.Database) (dbmodel.Identity, dbmodel.Identity, string, jujuparams.OfferAccessPermission) {
			return env.User("alice@canonical.com").DBObject(c, db), env.User("bob@canonical.com").DBObject(c, db), "test-offer-url", jujuparams.OfferReadAccess
		},
		expectedAccessLevel: "consume",
	}, {
		about: "admin grants a read user admin access - user gets admin access",
		parameterFunc: func(env *jimmtest.Environment, db *db.Database) (dbmodel.Identity, dbmodel.Identity, string, jujuparams.OfferAccessPermission) {
			return env.User("alice@canonical.com").DBObject(c, db), env.User("fred@canonical.com").DBObject(c, db), "test-offer-url", jujuparams.OfferAdminAccess
		},
		expectedAccessLevel: "admin",
	}, {
		about: "admin grants a read user consume access - user gets consume access",
		parameterFunc: func(env *jimmtest.Environment, db *db.Database) (dbmodel.Identity, dbmodel.Identity, string, jujuparams.OfferAccessPermission) {
			return env.User("alice@canonical.com").DBObject(c, db), env.User("fred@canonical.com").DBObject(c, db), "test-offer-url", jujuparams.OfferConsumeAccess
		},
		expectedAccessLevel: "consume",
	}, {
		about: "admin grants a read user read access - user keeps read access",
		parameterFunc: func(env *jimmtest.Environment, db *db.Database) (dbmodel.Identity, dbmodel.Identity, string, jujuparams.OfferAccessPermission) {
			return env.User("alice@canonical.com").DBObject(c, db), env.User("fred@canonical.com").DBObject(c, db), "test-offer-url", jujuparams.OfferReadAccess
		},
		expectedAccessLevel: "read",
	}, {
		about: "no such offer",
		parameterFunc: func(env *jimmtest.Environment, db *db.Database) (dbmodel.Identity, dbmodel.Identity, string, jujuparams.OfferAccessPermission) {
			return env.User("alice@canonical.com").DBObject(c, db), env.User("fred@canonical.com").DBObject(c, db), "no-such-offer", jujuparams.OfferReadAccess
		},
		expectedError: "application offer not found",
	}, {
		about: "user with consume rights cannot grant any rights",
		parameterFunc: func(env *jimmtest.Environment, db *db.Database) (dbmodel.Identity, dbmodel.Identity, string, jujuparams.OfferAccessPermission) {
			return env.User("bob@canonical.com").DBObject(c, db), env.User("grant@canonical.com").DBObject(c, db), "test-offer-url", jujuparams.OfferConsumeAccess
		},
		expectedError: "unauthorized",
	}, {
		about: "user with read rights cannot grant any rights",
		parameterFunc: func(env *jimmtest.Environment, db *db.Database) (dbmodel.Identity, dbmodel.Identity, string, jujuparams.OfferAccessPermission) {
			return env.User("fred@canonical.com").DBObject(c, db), env.User("grant@canonical.com").DBObject(c, db), "test-offer-url", jujuparams.OfferConsumeAccess
		},
		expectedError: "unauthorized",
	}, {
		about: "admin grants new user admin access - new user has admin access",
		parameterFunc: func(env *jimmtest.Environment, db *db.Database) (dbmodel.Identity, dbmodel.Identity, string, jujuparams.OfferAccessPermission) {
			return env.User("alice@canonical.com").DBObject(c, db), env.User("grant@canonical.com").DBObject(c, db), "test-offer-url", jujuparams.OfferAdminAccess
		},
		expectedAccessLevel: "admin",
	}, {
		about: "admin grants new user consume access - new user has consume access",
		parameterFunc: func(env *jimmtest.Environment, db *db.Database) (dbmodel.Identity, dbmodel.Identity, string, jujuparams.OfferAccessPermission) {
			return env.User("alice@canonical.com").DBObject(c, db), env.User("grant@canonical.com").DBObject(c, db), "test-offer-url", jujuparams.OfferConsumeAccess
		},
		expectedAccessLevel: "consume",
	}, {
		about: "admin grants new user read access - new user has read access",
		parameterFunc: func(env *jimmtest.Environment, db *db.Database) (dbmodel.Identity, dbmodel.Identity, string, jujuparams.OfferAccessPermission) {
			return env.User("alice@canonical.com").DBObject(c, db), env.User("grant@canonical.com").DBObject(c, db), "test-offer-url", jujuparams.OfferReadAccess
		},
		expectedAccessLevel: "read",
	}}

	for _, test := range tests {
		c.Run(test.about, func(c *qt.C) {
			j := jimmtest.NewJIMM(c, &jimm.Parameters{})

			env := jimmtest.ParseEnvironment(c, revokeAndGrantOfferAccessTestEnv)
			env.PopulateDBAndPermissions(c, j.ResourceTag(), j.Database, j.OpenFGAClient)

			authenticatedUser, offerUser, offerURL, grantAccessLevel := test.parameterFunc(env, j.Database)

			err := j.PermissionManager().GrantOfferAccess(ctx, openfga.NewUser(&authenticatedUser, j.OpenFGAClient), offerURL, offerUser.ResourceTag(), grantAccessLevel)
			if test.expectedError == "" {
				c.Assert(err, qt.IsNil)

				offer := dbmodel.ApplicationOffer{
					URL: offerURL,
				}
				err = j.Database.GetApplicationOffer(ctx, &offer)
				c.Assert(err, qt.IsNil)
				appliedRelation := openfga.NewUser(&offerUser, j.OpenFGAClient).GetApplicationOfferAccess(ctx, offer.ResourceTag())
				c.Assert(permissions.ToOfferAccessString(appliedRelation), qt.Equals, test.expectedAccessLevel)
			} else {
				c.Assert(err, qt.ErrorMatches, test.expectedError)
			}
		})
	}
}

const grantCloudAccessTestEnv = `clouds:
- name: test-cloud
  type: test-provider
  regions:
  - name: test-cloud-region
- name: test
  type: kubernetes
  host-cloud-region: test-cloud/test-cloud-region
  regions:
  - name: default
  - name: region2
  users:
  - user: alice@canonical.com
    access: admin
controllers:
- name: controller-1
  uuid: 00000001-0000-0000-0000-000000000001
  cloud: test-cloud
  region: test-cloud-region
  cloud-regions:
  - cloud: test-cloud
    region: test-cloud-region
    priority: 10
  - cloud: test
    region: default
    priority: 1
  - cloud: test
    region: region2
    priority: 1
`

var grantCloudAccessTests = []struct {
	name            string
	env             string
	username        string
	cloud           string
	targetUsername  string
	access          string
	expectRelations []openfga.Tuple
	expectError     string
	expectErrorCode errors.Code
}{{
	name:            "CloudNotFound",
	username:        "alice@canonical.com",
	cloud:           "test2",
	targetUsername:  "bob@canonical.com",
	access:          "add-model",
	expectError:     `unauthorized`,
	expectErrorCode: errors.CodeUnauthorized,
}, {
	name:           "Admin grants admin access",
	env:            grantCloudAccessTestEnv,
	username:       "alice@canonical.com",
	cloud:          "test",
	targetUsername: "bob@canonical.com",
	access:         "admin",
	expectRelations: []openfga.Tuple{{
		Object:   ofganames.ConvertTag(names.NewUserTag("alice@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewCloudTag("test")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("bob@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewCloudTag("test")),
	}},
}, {
	name:           "Admin grants add-model access",
	env:            grantCloudAccessTestEnv,
	username:       "alice@canonical.com",
	cloud:          "test",
	targetUsername: "bob@canonical.com",
	access:         "add-model",
	expectRelations: []openfga.Tuple{{
		Object:   ofganames.ConvertTag(names.NewUserTag("alice@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewCloudTag("test")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("bob@canonical.com")),
		Relation: ofganames.CanAddModelRelation,
		Target:   ofganames.ConvertTag(names.NewCloudTag("test")),
	}},
}, {
	name:            "UserNotAuthorized",
	env:             grantCloudAccessTestEnv,
	username:        "charlie@canonical.com",
	cloud:           "test",
	targetUsername:  "bob@canonical.com",
	access:          "add-model",
	expectError:     `unauthorized`,
	expectErrorCode: errors.CodeUnauthorized,
}, {
	name:           "unknown access",
	env:            grantCloudAccessTestEnv,
	username:       "alice@canonical.com",
	cloud:          "test",
	targetUsername: "bob@canonical.com",
	access:         "some-unknown-access",
	expectError:    `failed to recognize given access: "some-unknown-access"`,
}}

func TestGrantCloudAccess(t *testing.T) {
	c := qt.New(t)

	for _, t := range grantCloudAccessTests {
		tt := t
		c.Run(tt.name, func(c *qt.C) {
			ctx := context.Background()

			env := jimmtest.ParseEnvironment(c, tt.env)

			j := jimmtest.NewJIMM(c, &jimm.Parameters{})

			env.PopulateDBAndPermissions(c, j.ResourceTag(), j.Database, j.OpenFGAClient)

			dbUser := env.User(tt.username).DBObject(c, j.Database)
			user := openfga.NewUser(&dbUser, j.OpenFGAClient)

			err := j.PermissionManager().GrantCloudAccess(ctx, user, names.NewCloudTag(tt.cloud), names.NewUserTag(tt.targetUsername), tt.access)
			if tt.expectError != "" {
				c.Check(err, qt.ErrorMatches, tt.expectError)
				if tt.expectErrorCode != "" {
					c.Check(errors.ErrorCode(err), qt.Equals, tt.expectErrorCode)
				}
				return
			}
			c.Assert(err, qt.IsNil)
			for _, tuple := range tt.expectRelations {
				value, err := j.OpenFGAClient.CheckRelation(ctx, tuple, false)
				c.Assert(err, qt.IsNil)
				c.Assert(value, qt.IsTrue, qt.Commentf("expected the tuple to exist after granting"))
			}
		})
	}
}

const revokeCloudAccessTestEnv = `clouds:
- name: test-cloud
  type: test-provider
  regions:
  - name: test-cloud-region
  users:
  - user: daphne@canonical.com
    access: admin
- name: test
  type: kubernetes
  host-cloud-region: test-cloud/test-cloud-region
  regions:
  - name: default
  users:
  - user: alice@canonical.com
    access: admin
  - user: bob@canonical.com
    access: admin
  - user: charlie@canonical.com
    access: add-model
controllers:
- name: controller-1
  uuid: 00000001-0000-0000-0000-000000000001
  cloud: test-cloud
  region: test-cloud-region
  cloud-regions:
  - cloud: test-cloud
    region: test-cloud-region
    priority: 10
  - cloud: test
    region: default
    priority: 1
`

var revokeCloudAccessTests = []struct {
	name                   string
	env                    string
	username               string
	cloud                  string
	targetUsername         string
	access                 string
	extraInitialTuples     []openfga.Tuple
	expectRelations        []openfga.Tuple
	expectRemovedRelations []openfga.Tuple
	expectError            string
	expectErrorCode        errors.Code
}{{
	name:            "CloudNotFound",
	username:        "alice@canonical.com",
	cloud:           "test2",
	targetUsername:  "bob@canonical.com",
	access:          "admin",
	expectError:     `unauthorized`,
	expectErrorCode: errors.CodeUnauthorized,
}, {
	name:           "Admin revokes 'admin' from another admin",
	env:            revokeCloudAccessTestEnv,
	username:       "alice@canonical.com",
	cloud:          "test",
	targetUsername: "bob@canonical.com",
	access:         "admin",
	expectRelations: []openfga.Tuple{{
		Object:   ofganames.ConvertTag(names.NewUserTag("alice@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewCloudTag("test")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("charlie@canonical.com")),
		Relation: ofganames.CanAddModelRelation,
		Target:   ofganames.ConvertTag(names.NewCloudTag("test")),
	}},
	expectRemovedRelations: []openfga.Tuple{{
		Object:   ofganames.ConvertTag(names.NewUserTag("bob@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewCloudTag("test")),
	}},
}, {
	name:           "Admin revokes 'add-model' from another admin",
	env:            revokeCloudAccessTestEnv,
	username:       "alice@canonical.com",
	cloud:          "test",
	targetUsername: "bob@canonical.com",
	access:         "add-model",
	expectRelations: []openfga.Tuple{{
		Object:   ofganames.ConvertTag(names.NewUserTag("alice@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewCloudTag("test")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("charlie@canonical.com")),
		Relation: ofganames.CanAddModelRelation,
		Target:   ofganames.ConvertTag(names.NewCloudTag("test")),
	}},
	expectRemovedRelations: []openfga.Tuple{{
		Object:   ofganames.ConvertTag(names.NewUserTag("bob@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewCloudTag("test")),
	}},
}, {
	name:           "Admin revokes 'add-model' from a user with 'add-model' access",
	env:            revokeCloudAccessTestEnv,
	username:       "alice@canonical.com",
	cloud:          "test",
	targetUsername: "charlie@canonical.com",
	access:         "add-model",
	expectRelations: []openfga.Tuple{{
		Object:   ofganames.ConvertTag(names.NewUserTag("alice@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewCloudTag("test")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("bob@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewCloudTag("test")),
	}},
	expectRemovedRelations: []openfga.Tuple{{
		Object:   ofganames.ConvertTag(names.NewUserTag("charlie@canonical.com")),
		Relation: ofganames.CanAddModelRelation,
		Target:   ofganames.ConvertTag(names.NewCloudTag("test")),
	}},
}, {
	name:           "Admin revokes 'add-model' from a user with no access",
	env:            revokeCloudAccessTestEnv,
	username:       "alice@canonical.com",
	cloud:          "test",
	targetUsername: "daphne@canonical.com",
	access:         "add-model",
	expectRelations: []openfga.Tuple{{
		Object:   ofganames.ConvertTag(names.NewUserTag("alice@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewCloudTag("test")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("bob@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewCloudTag("test")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("charlie@canonical.com")),
		Relation: ofganames.CanAddModelRelation,
		Target:   ofganames.ConvertTag(names.NewCloudTag("test")),
	}},
}, {
	name:           "Admin revokes 'admin' from a user with no access",
	env:            revokeCloudAccessTestEnv,
	username:       "alice@canonical.com",
	cloud:          "test",
	targetUsername: "daphne@canonical.com",
	access:         "admin",
	expectRelations: []openfga.Tuple{{
		Object:   ofganames.ConvertTag(names.NewUserTag("alice@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewCloudTag("test")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("bob@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewCloudTag("test")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("charlie@canonical.com")),
		Relation: ofganames.CanAddModelRelation,
		Target:   ofganames.ConvertTag(names.NewCloudTag("test")),
	}},
}, {
	name:           "Admin revokes 'add-model' access from a user who has separate tuples for all accesses (add-model/admin)",
	env:            revokeCloudAccessTestEnv,
	username:       "alice@canonical.com",
	cloud:          "test",
	targetUsername: "charlie@canonical.com",
	access:         "add-model",
	extraInitialTuples: []openfga.Tuple{{
		Object:   ofganames.ConvertTag(names.NewUserTag("charlie@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewCloudTag("test")),
	},
	// No need to add the 'add-model' relation, because it's already there due to the environment setup.
	},
	expectRelations: []openfga.Tuple{{
		Object:   ofganames.ConvertTag(names.NewUserTag("alice@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewCloudTag("test")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("bob@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewCloudTag("test")),
	}},
	expectRemovedRelations: []openfga.Tuple{{
		Object:   ofganames.ConvertTag(names.NewUserTag("charlie@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewCloudTag("test")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("charlie@canonical.com")),
		Relation: ofganames.CanAddModelRelation,
		Target:   ofganames.ConvertTag(names.NewCloudTag("test")),
	}},
}, {
	name:           "Admin revokes 'admin' access from a user who has separate tuples for all accesses (add-model/admin)",
	env:            revokeCloudAccessTestEnv,
	username:       "alice@canonical.com",
	cloud:          "test",
	targetUsername: "charlie@canonical.com",
	access:         "admin",
	extraInitialTuples: []openfga.Tuple{{
		Object:   ofganames.ConvertTag(names.NewUserTag("charlie@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewCloudTag("test")),
	},
	// No need to add the 'add-model' relation, because it's already there due to the environment setup.
	},
	expectRelations: []openfga.Tuple{{
		Object:   ofganames.ConvertTag(names.NewUserTag("alice@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewCloudTag("test")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("bob@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewCloudTag("test")),
	}, {
		Object:   ofganames.ConvertTag(names.NewUserTag("charlie@canonical.com")),
		Relation: ofganames.CanAddModelRelation,
		Target:   ofganames.ConvertTag(names.NewCloudTag("test")),
	}},
	expectRemovedRelations: []openfga.Tuple{{
		Object:   ofganames.ConvertTag(names.NewUserTag("charlie@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(names.NewCloudTag("test")),
	}},
}, {
	name:            "UserNotAuthorized",
	env:             revokeCloudAccessTestEnv,
	username:        "charlie@canonical.com",
	cloud:           "test",
	targetUsername:  "bob@canonical.com",
	access:          "add-model",
	expectError:     `unauthorized`,
	expectErrorCode: errors.CodeUnauthorized,
}, {
	name:           "unknown access",
	env:            revokeCloudAccessTestEnv,
	username:       "alice@canonical.com",
	cloud:          "test",
	targetUsername: "bob@canonical.com",
	access:         "some-unknown-access",
	expectError:    `failed to recognize given access: "some-unknown-access"`,
}}

//nolint:gocognit
func TestRevokeCloudAccess(t *testing.T) {
	c := qt.New(t)

	for _, t := range revokeCloudAccessTests {
		tt := t
		c.Run(tt.name, func(c *qt.C) {
			ctx := context.Background()

			env := jimmtest.ParseEnvironment(c, tt.env)

			j := jimmtest.NewJIMM(c, &jimm.Parameters{})

			env.PopulateDBAndPermissions(c, j.ResourceTag(), j.Database, j.OpenFGAClient)

			if len(tt.extraInitialTuples) > 0 {
				err := j.OpenFGAClient.AddRelation(ctx, tt.extraInitialTuples...)
				c.Assert(err, qt.IsNil)
			}

			if tt.expectRemovedRelations != nil {
				for _, tuple := range tt.expectRemovedRelations {
					value, err := j.OpenFGAClient.CheckRelation(ctx, tuple, false)
					c.Assert(err, qt.IsNil)
					c.Assert(value, qt.IsTrue, qt.Commentf("expected the tuple to exist before revoking"))
				}
			}

			dbUser := env.User(tt.username).DBObject(c, j.Database)
			user := openfga.NewUser(&dbUser, j.OpenFGAClient)

			err := j.PermissionManager().RevokeCloudAccess(ctx, user, names.NewCloudTag(tt.cloud), names.NewUserTag(tt.targetUsername), tt.access)
			if tt.expectError != "" {
				c.Check(err, qt.ErrorMatches, tt.expectError)
				if tt.expectErrorCode != "" {
					c.Check(errors.ErrorCode(err), qt.Equals, tt.expectErrorCode)
				}
				return
			}
			c.Assert(err, qt.IsNil)
			if tt.expectRemovedRelations != nil {
				for _, tuple := range tt.expectRemovedRelations {
					value, err := j.OpenFGAClient.CheckRelation(ctx, tuple, false)
					c.Assert(err, qt.IsNil)
					c.Assert(value, qt.IsFalse, qt.Commentf("expected the tuple to be removed after revoking"))
				}
			}
			if tt.expectRelations != nil {
				for _, tuple := range tt.expectRelations {
					value, err := j.OpenFGAClient.CheckRelation(ctx, tuple, false)
					c.Assert(err, qt.IsNil)
					c.Assert(value, qt.IsTrue, qt.Commentf("expected the tuple to exist after revoking"))
				}
			}
		})
	}
}

func (s *permissionManagerSuite) TestParseAndValidateTag(c *qt.C) {
	c.Parallel()
	ctx := context.Background()

	user, _, _, model, _, _, _, _ := jimmtest.CreateTestControllerEnvironment(ctx, c, s.db)

	jimmTag := "model-" + user.Name + "/" + model.Name + "#administrator"

	// JIMM tag syntax for models
	tag, err := s.manager.ParseAndValidateTag(ctx, jimmTag)
	c.Assert(err, qt.IsNil)
	c.Assert(tag.Kind.String(), qt.Equals, names.ModelTagKind)
	c.Assert(tag.ID, qt.Equals, model.UUID.String)
	c.Assert(tag.Relation.String(), qt.Equals, "administrator")

	jujuTag := "model-" + model.UUID.String + "#administrator"

	// Juju tag syntax for models
	tag, err = s.manager.ParseAndValidateTag(ctx, jujuTag)
	c.Assert(err, qt.IsNil)
	c.Assert(tag.ID, qt.Equals, model.UUID.String)
	c.Assert(tag.Kind.String(), qt.Equals, names.ModelTagKind)
	c.Assert(tag.Relation.String(), qt.Equals, "administrator")

	// JIMM tag only kind
	kindTag := "model"
	tag, err = s.manager.ParseAndValidateTag(ctx, kindTag)
	c.Assert(err, qt.IsNil)
	c.Assert(tag.ID, qt.Equals, "")
	c.Assert(tag.Kind.String(), qt.Equals, names.ModelTagKind)

	// JIMM tag not valid
	_, err = s.manager.ParseAndValidateTag(ctx, "")
	c.Assert(err, qt.ErrorMatches, "unknown tag kind")
}

func (s *permissionManagerSuite) TestResolveTags(c *qt.C) {
	c.Parallel()
	ctx := context.Background()

	identity, group, controller, model, offer, cloud, _, role := jimmtest.CreateTestControllerEnvironment(ctx, c, s.db)

	testCases := []struct {
		desc     string
		input    string
		expected *ofga.Entity
	}{{
		desc:     "map identity name with relation",
		input:    "user-" + identity.Name + "#member",
		expected: ofganames.ConvertTagWithRelation(names.NewUserTag(identity.Name), ofganames.MemberRelation),
	}, {
		desc:     "map group name with relation",
		input:    "group-" + group.Name + "#member",
		expected: ofganames.ConvertTagWithRelation(jimmnames.NewGroupTag(group.UUID), ofganames.MemberRelation),
	}, {
		desc:     "map group UUID",
		input:    "group-" + group.UUID,
		expected: ofganames.ConvertTag(jimmnames.NewGroupTag(group.UUID)),
	}, {
		desc:     "map group UUID with relation",
		input:    "group-" + group.UUID + "#member",
		expected: ofganames.ConvertTagWithRelation(jimmnames.NewGroupTag(group.UUID), ofganames.MemberRelation),
	}, {
		desc:     "map role UUID",
		input:    "role-" + role.UUID,
		expected: ofganames.ConvertTag(jimmnames.NewRoleTag(role.UUID)),
	}, {
		desc:     "map role UUID with relation",
		input:    "role-" + role.UUID + "#assignee",
		expected: ofganames.ConvertTagWithRelation(jimmnames.NewRoleTag(role.UUID), ofganames.AssigneeRelation),
	}, {
		desc:     "map jimm controller",
		input:    "controller-" + "jimm",
		expected: ofganames.ConvertTag(s.ctlTag),
	}, {
		desc:     "map controller",
		input:    "controller-" + controller.Name + "#administrator",
		expected: ofganames.ConvertTagWithRelation(names.NewControllerTag(model.UUID.String), ofganames.AdministratorRelation),
	}, {
		desc:     "map controller UUID",
		input:    "controller-" + controller.UUID,
		expected: ofganames.ConvertTag(names.NewControllerTag(model.UUID.String)),
	}, {
		desc:     "map model",
		input:    "model-" + model.OwnerIdentityName + "/" + model.Name + "#administrator",
		expected: ofganames.ConvertTagWithRelation(names.NewModelTag(model.UUID.String), ofganames.AdministratorRelation),
	}, {
		desc:     "map model UUID",
		input:    "model-" + model.UUID.String,
		expected: ofganames.ConvertTag(names.NewModelTag(model.UUID.String)),
	}, {
		desc:     "map offer",
		input:    "applicationoffer-" + offer.URL + "#administrator",
		expected: ofganames.ConvertTagWithRelation(names.NewApplicationOfferTag(offer.UUID), ofganames.AdministratorRelation),
	}, {
		desc:     "map offer UUID",
		input:    "applicationoffer-" + offer.UUID,
		expected: ofganames.ConvertTag(names.NewApplicationOfferTag(offer.UUID)),
	}, {
		desc:     "map cloud",
		input:    "cloud-" + cloud.Name + "#administrator",
		expected: ofganames.ConvertTagWithRelation(names.NewCloudTag(cloud.Name), ofganames.AdministratorRelation),
	}}

	for _, tC := range testCases {
		c.Run(tC.desc, func(c *qt.C) {
			jujuTag, err := permissions.ResolveTag(s.ctlTag.Id(), s.db, tC.input)
			c.Assert(err, qt.IsNil)
			c.Assert(jujuTag, qt.DeepEquals, tC.expected)
		})
	}
}

func (s *permissionManagerSuite) TestResolveTupleObjectHandlesErrors(c *qt.C) {
	c.Parallel()
	ctx := context.Background()

	_, _, controller, model, offer, _, _, _ := jimmtest.CreateTestControllerEnvironment(ctx, c, s.db)

	type test struct {
		input string
		want  string
	}

	tests := []test{
		// Resolves bad tuple objects in general
		{
			input: "unknowntag-blabla",
			want:  "failed to map tag, unknown kind: unknowntag",
		},
		// Resolves bad groups where they do not exist
		{
			input: "group-myspecialpokemon-his-name-is-youguessedit-diglett",
			want:  "group myspecialpokemon-his-name-is-youguessedit-diglett not found",
		},
		// Resolves bad controllers where they do not exist
		{
			input: "controller-mycontroller-that-does-not-exist",
			want:  "controller not found",
		},
		// Resolves bad models where the user cannot be obtained from the JIMM tag
		{
			input: "model-mycontroller-that-does-not-exist/mymodel",
			want:  "model not found",
		},
		// Resolves bad models where it cannot be found on the specified controller
		{
			input: "model-" + controller.Name + ":alex/",
			want:  "model name format incorrect, expected <model-owner>/<model-name>",
		},
		// Resolves bad applicationoffers where it cannot be found on the specified controller/model combo
		{
			input: "applicationoffer-" + controller.Name + ":alex/" + model.Name + "." + offer.UUID + "fluff",
			want:  "application offer not found",
		},
		{
			input: "abc",
			want:  "failed to setup tag resolver: tag is not properly formatted",
		},
		{
			input: "model-test-unknowncontroller-1:alice@canonical.com/test-model-1",
			want:  "model not found",
		},
	}
	for i, tc := range tests {
		c.Run(fmt.Sprintf("test %d", i), func(c *qt.C) {
			_, err := permissions.ResolveTag(s.ctlTag.Id(), s.db, tc.input)
			c.Assert(err, qt.ErrorMatches, tc.want)
		})
	}
}

func (s *permissionManagerSuite) TestToJAASTag(c *qt.C) {
	c.Parallel()
	ctx := context.Background()

	user, group, controller, model, applicationOffer, cloud, _, role := jimmtest.CreateTestControllerEnvironment(ctx, c, s.db)

	serviceAccountId := petname.Generate(2, "-") + "@serviceaccount"

	tests := []struct {
		tag             *ofganames.Tag
		expectedJAASTag string
		expectedError   string
	}{{
		tag:             ofganames.ConvertTag(user.ResourceTag()),
		expectedJAASTag: "user-" + user.Name,
	}, {
		tag:             ofganames.ConvertTag(jimmnames.NewServiceAccountTag(serviceAccountId)),
		expectedJAASTag: "serviceaccount-" + serviceAccountId,
	}, {
		tag:             ofganames.ConvertTag(group.ResourceTag()),
		expectedJAASTag: "group-" + group.Name,
	}, {
		tag:             ofganames.ConvertTag(controller.ResourceTag()),
		expectedJAASTag: "controller-" + controller.Name,
	}, {
		tag:             ofganames.ConvertTag(model.ResourceTag()),
		expectedJAASTag: "model-" + user.Name + "/" + model.Name,
	}, {
		tag:             ofganames.ConvertTag(applicationOffer.ResourceTag()),
		expectedJAASTag: "applicationoffer-" + applicationOffer.URL,
	}, {
		tag:           &ofganames.Tag{},
		expectedError: "unexpected tag kind: ",
	}, {
		tag:             ofganames.ConvertTag(cloud.ResourceTag()),
		expectedJAASTag: "cloud-" + cloud.Name,
	}, {
		tag:             ofganames.ConvertTag(role.ResourceTag()),
		expectedJAASTag: "role-" + role.Name,
	}}
	for _, test := range tests {
		t, err := s.manager.ToJAASTag(ctx, test.tag, true)
		if test.expectedError != "" {
			c.Assert(err, qt.ErrorMatches, test.expectedError)
		} else {
			c.Assert(err, qt.IsNil)
			c.Assert(t, qt.Equals, test.expectedJAASTag)
		}
	}
}

func (s *permissionManagerSuite) TestToJAASTagNoUUIDResolution(c *qt.C) {
	c.Parallel()
	ctx := context.Background()

	user, group, controller, model, applicationOffer, cloud, _, role := jimmtest.CreateTestControllerEnvironment(ctx, c, s.db)
	serviceAccountId := petname.Generate(2, "-") + "@serviceaccount"

	tests := []struct {
		tag             *ofganames.Tag
		expectedJAASTag string
		expectedError   string
	}{{
		tag:             ofganames.ConvertTag(user.ResourceTag()),
		expectedJAASTag: "user-" + user.Name,
	}, {
		tag:             ofganames.ConvertTag(jimmnames.NewServiceAccountTag(serviceAccountId)),
		expectedJAASTag: "serviceaccount-" + serviceAccountId,
	}, {
		tag:             ofganames.ConvertTag(group.ResourceTag()),
		expectedJAASTag: "group-" + group.UUID,
	}, {
		tag:             ofganames.ConvertTag(controller.ResourceTag()),
		expectedJAASTag: "controller-" + controller.UUID,
	}, {
		tag:             ofganames.ConvertTag(model.ResourceTag()),
		expectedJAASTag: "model-" + model.UUID.String,
	}, {
		tag:             ofganames.ConvertTag(applicationOffer.ResourceTag()),
		expectedJAASTag: "applicationoffer-" + applicationOffer.UUID,
	}, {
		tag:             ofganames.ConvertTag(cloud.ResourceTag()),
		expectedJAASTag: "cloud-" + cloud.Name,
	}, {
		tag:             ofganames.ConvertTag(role.ResourceTag()),
		expectedJAASTag: "role-" + role.UUID,
	}, {
		tag:             &ofganames.Tag{},
		expectedJAASTag: "-",
	}}
	for _, test := range tests {
		t, err := s.manager.ToJAASTag(ctx, test.tag, false)
		if test.expectedError != "" {
			c.Assert(err, qt.ErrorMatches, test.expectedError)
		} else {
			c.Assert(err, qt.IsNil)
			c.Assert(t, qt.Equals, test.expectedJAASTag)
		}
	}
}

func (s *permissionManagerSuite) TestOpenFGACleanup(c *qt.C) {
	c.Parallel()
	ctx := context.Background()

	// run cleanup on an empty authorizaton store
	err := s.manager.OpenFGACleanup(ctx)
	c.Assert(err, qt.IsNil)

	type createTagFunction func(int) *ofga.Entity

	var (
		createStringTag = func(kind openfga.Kind) createTagFunction {
			return func(i int) *ofga.Entity {
				return &ofga.Entity{
					Kind: kind,
					ID:   fmt.Sprintf("%s-%d", petname.Generate(2, "-"), i),
				}
			}
		}

		createUUIDTag = func(kind openfga.Kind) createTagFunction {
			return func(i int) *ofga.Entity {
				return &ofga.Entity{
					Kind: kind,
					ID:   uuid.NewString(),
				}
			}
		}
	)

	tagTests := []struct {
		createObjectTag createTagFunction
		relation        string
		createTargetTag createTagFunction
	}{{
		createObjectTag: createStringTag(openfga.UserType),
		relation:        "member",
		createTargetTag: createStringTag(openfga.GroupType),
	}, {
		createObjectTag: createStringTag(openfga.UserType),
		relation:        "administrator",
		createTargetTag: createUUIDTag(openfga.ControllerType),
	}, {
		createObjectTag: createStringTag(openfga.UserType),
		relation:        "reader",
		createTargetTag: createUUIDTag(openfga.ModelType),
	}, {
		createObjectTag: createStringTag(openfga.UserType),
		relation:        "administrator",
		createTargetTag: createStringTag(openfga.CloudType),
	}, {
		createObjectTag: createStringTag(openfga.UserType),
		relation:        "consumer",
		createTargetTag: createUUIDTag(openfga.ApplicationOfferType),
	}}

	orphanedTuples := []ofga.Tuple{}
	for i := 0; i < 100; i++ {
		for _, test := range tagTests {
			objectTag := test.createObjectTag(i)
			targetTag := test.createTargetTag(i)

			tuple := openfga.Tuple{
				Object:   objectTag,
				Relation: ofga.Relation(test.relation),
				Target:   targetTag,
			}
			err = s.ofgaClient.AddRelation(ctx, tuple)
			c.Assert(err, qt.IsNil)

			orphanedTuples = append(orphanedTuples, tuple)
		}
	}

	err = s.manager.OpenFGACleanup(ctx)
	c.Assert(err, qt.IsNil)

	for _, tuple := range orphanedTuples {
		c.Logf("checking relation for %+v", tuple)
		ok, err := s.ofgaClient.CheckRelation(ctx, tuple, false)
		c.Assert(err, qt.IsNil)
		c.Assert(ok, qt.IsFalse)
	}
}
