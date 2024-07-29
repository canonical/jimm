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

func TestUserSSHKey(t *testing.T) {
	c := qt.New(t)
	db := gormDB(c)

	// Create an Identity to add keys to
	bob, err := dbmodel.NewIdentity("bob@canonical.com")
	c.Assert(err, qt.IsNil)
	err = db.Create(bob).Error
	c.Assert(err, qt.IsNil)

	// Add some keys for bob
	bobsSSHKeys, err := dbmodel.NewUserSSHKeys(bob.Name, []string{bobsSSHKeyString1, bobsSSHKeyString2})
	c.Assert(err, qt.IsNil)
	err = db.Create(bobsSSHKeys).Error
	c.Assert(err, qt.IsNil)

	// Get bobs keys
	retrievingBobsKeys, err := dbmodel.NewUserSSHKeys(bob.Name, nil)
	c.Assert(err, qt.IsNil)
	err = db.Find(retrievingBobsKeys).Error
	c.Assert(err, qt.IsNil)

	c.Assert(retrievingBobsKeys.Keys, qt.HasLen, 2)
	c.Assert(retrievingBobsKeys.Keys[0], qt.Equals, bobsSSHKeyString1)
	c.Assert(retrievingBobsKeys.Keys[1], qt.Equals, bobsSSHKeyString2)
}
