// Copyright 2024 Canonical Ltd.

package db_test

import (
	"context"

	"github.com/canonical/jimm/internal/dbmodel"
	qt "github.com/frankban/quicktest"
)

const (
	bobsSSHKeyString1 = "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQC3v1y9J6O1P1Xj8y5P8kJ3y8J5P8kJ3y8J5P8kJ3y8J5P8kJ3y8J5P8kJ3y8J5P8kJ3y8J5P8kJ3y8J5P8kJ3y8J5P8kJ3y8J5P8kJ3y8J5P8kJ3y8J5P8kJ3y8J5P8kJ3y8J5P8kJ3y8J5P8kJ3y8J5P8kJ3y8J5P8kJ3y8J bob@canonical.com"
	bobsSSHKeyString2 = "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQC3v1y9J6O1P1Xj8y5P8kJ3y8J5P8kJ3y8J5P8kJ3y8J5P8kJ3y8J5P8kJ3y8J5P8kJ3y8J5P8kJ3y8J5P8kJ3y8J5P8kJ3y8J5P8kJ3y8J5P8kJ3y8J5P8kJ3y8J5P8kJ3y8J5P8kJ3y8J5P8kJ3y8J5P8kJ3y8J5P8kJ3y8L bob@canonical.com"
)

func createIdentityBob(ctx context.Context, c *qt.C, s *dbSuite) *dbmodel.Identity {
	bob, err := dbmodel.NewIdentity("bob@canonical.com")
	c.Assert(err, qt.IsNil)
	err = s.Database.GetIdentity(ctx, bob)
	c.Assert(err, qt.IsNil)

	return bob
}

func (s *dbSuite) TestAddUserSSHKey(c *qt.C) {
	ctx := context.Background()
	err := s.Database.Migrate(ctx, true)
	c.Assert(err, qt.Equals, nil)

	bob := createIdentityBob(ctx, c, s)

	key, err := dbmodel.NewUserSSHKey(bob.Name, bobsSSHKeyString1)
	c.Assert(err, qt.IsNil)
	keys := []dbmodel.UserSSHKey{*key}
	err = s.Database.AddUserSSHKeys(ctx, keys)
	c.Assert(err, qt.IsNil)

	retrievedKey := dbmodel.UserSSHKey{}
	tx := s.Database.DB.First(&retrievedKey)
	c.Assert(tx.Error, qt.IsNil)
	c.Assert(retrievedKey.IdentityName, qt.Equals, bob.Name)
	c.Assert(retrievedKey.SSHKey, qt.Equals, bobsSSHKeyString1)
}

func (s *dbSuite) TestDeleteUserSSHKey(c *qt.C) {
	ctx := context.Background()
	err := s.Database.Migrate(ctx, true)
	c.Assert(err, qt.Equals, nil)

	bob := createIdentityBob(ctx, c, s)

	key, err := dbmodel.NewUserSSHKey(bob.Name, bobsSSHKeyString1)
	c.Assert(err, qt.IsNil)
	keys := []dbmodel.UserSSHKey{*key}
	err = s.Database.AddUserSSHKeys(ctx, keys)
	c.Assert(err, qt.IsNil)

	// Ensure key exists
	key = &dbmodel.UserSSHKey{}
	err = s.Database.DB.First(key).Error
	c.Assert(err, qt.IsNil)

	// Now delete bobs ssh key
	err = s.Database.DeleteUserSSHKey(ctx, key)
	c.Assert(err, qt.IsNil)

	// Attempt to delete it again and result in no error as
	// there's nothing to delete
	err = s.Database.DeleteUserSSHKey(ctx, key)
	c.Assert(err, qt.IsNil)

	// Attempt to get the key
	err = s.Database.DB.First(key).Error
	c.Assert(err, qt.ErrorMatches, ".*record not found.*")
}

func (s *dbSuite) TestListUserSSHKeys(c *qt.C) {
	ctx := context.Background()
	err := s.Database.Migrate(ctx, true)
	c.Assert(err, qt.Equals, nil)

	bob := createIdentityBob(ctx, c, s)

	// Ensure bob has no keys
	keys, err := s.Database.ListUserSSHKeys(ctx, bob)
	c.Assert(err, qt.IsNil)
	c.Assert(keys, qt.HasLen, 0)

	// Add keys
	key1, err := dbmodel.NewUserSSHKey(bob.Name, bobsSSHKeyString1)
	c.Assert(err, qt.IsNil)
	key2, err := dbmodel.NewUserSSHKey(bob.Name, bobsSSHKeyString2)
	c.Assert(err, qt.IsNil)
	keysToAdd := []dbmodel.UserSSHKey{*key1, *key2}
	err = s.Database.AddUserSSHKeys(ctx, keysToAdd)
	c.Assert(err, qt.IsNil)

	// Get bob's keys
	keys, err = s.Database.ListUserSSHKeys(ctx, bob)
	c.Assert(err, qt.IsNil)
	c.Assert(keys, qt.HasLen, 2)
	c.Assert(keys[0], qt.Equals, bobsSSHKeyString1)
	c.Assert(keys[1], qt.Equals, bobsSSHKeyString2)
}
