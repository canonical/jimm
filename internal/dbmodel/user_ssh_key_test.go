// Copyright 2024 Canonical Ltd.

package dbmodel_test

import (
	"testing"

	"github.com/canonical/jimm/internal/dbmodel"
	qt "github.com/frankban/quicktest"
)

const (
	bobsSSHKeyString1 = "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQC3v1y9J6O1P1Xj8y5P8kJ3y8J5P8kJ3y8J5P8kJ3y8J5P8kJ3y8J5P8kJ3y8J5P8kJ3y8J5P8kJ3y8J5P8kJ3y8J5P8kJ3y8J5P8kJ3y8J5P8kJ3y8J5P8kJ3y8J5P8kJ3y8J5P8kJ3y8J5P8kJ3y8J5P8kJ3y8J5P8kJ3y8J bob@canonical.com"
	bobsSSHKeyString2 = "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQC3v1y9J6O1P1Xj8y5P8kJ3y8J5P8kJ3y8J5P8kJ3y8J5P8kJ3y8J5P8kJ3y8J5P8kJ3y8J5P8kJ3y8J5P8kJ3y8J5P8kJ3y8J5P8kJ3y8J5P8kJ3y8J5P8kJ3y8J5P8kJ3y8J5P8kJ3y8J5P8kJ3y8J5P8kJ3y8J5P8kJ3y8K bob@canonical.com"
)

func TestNewUserSSHKey(t *testing.T) {
	c := qt.New(t)

	_, err := dbmodel.NewUserSSHKey("", "some-key")
	c.Assert(err, qt.ErrorMatches, "identity name cannot be empty")

	_, err = dbmodel.NewUserSSHKey("some-identity", "")
	c.Assert(err, qt.ErrorMatches, "key cannot be empty")

	key, err := dbmodel.NewUserSSHKey("some-identity", "some-key")
	c.Assert(err, qt.IsNil)
	c.Assert(key, qt.DeepEquals, &dbmodel.UserSSHKey{
		IdentityName: "some-identity",
		SSHKey:       "some-key",
	})
}

func TestUserSSHKey(t *testing.T) {
	c := qt.New(t)
	db := gormDB(c)

	// Create an Identity to add keys to
	bob, err := dbmodel.NewIdentity("bob@canonical.com")
	c.Assert(err, qt.IsNil)
	err = db.Create(bob).Error
	c.Assert(err, qt.IsNil)

	// Add some keys for bob
	bobsSSHKey1, err := dbmodel.NewUserSSHKey(bob.Name, bobsSSHKeyString1)
	c.Assert(err, qt.IsNil)
	err = db.Create(bobsSSHKey1).Error
	c.Assert(err, qt.IsNil)

	// Testing unique constraint of unique name and ssh-key
	bobsSSHKey1Duplicate, err := dbmodel.NewUserSSHKey(bob.Name, bobsSSHKeyString1)
	c.Assert(err, qt.IsNil)
	err = db.Create(bobsSSHKey1Duplicate).Error
	c.Assert(err, qt.ErrorMatches, `.*duplicate key value violates unique constraint \"unique_identity_ssh_key\".*`)

	bobsSSHKey2, err := dbmodel.NewUserSSHKey(bob.Name, bobsSSHKeyString2)
	c.Assert(err, qt.IsNil)
	err = db.Create(bobsSSHKey2).Error
	c.Assert(err, qt.IsNil)

	// Get bobs keys
	sshKeys := []dbmodel.UserSSHKey{}
	result := db.Where("identity_name = ?", bob.Name).Order("created_at ASC").Find(&sshKeys)
	c.Assert(result.Error, qt.IsNil)

	c.Assert(sshKeys, qt.HasLen, 2)
	c.Assert(sshKeys[0].SSHKey, qt.Equals, bobsSSHKeyString1)
	c.Assert(sshKeys[1].SSHKey, qt.Equals, bobsSSHKeyString2)
}