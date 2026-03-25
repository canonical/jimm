// Copyright 2025 Canonical.

package db_test

import (
	"context"
	"time"

	qt "github.com/frankban/quicktest"
	"github.com/juju/names/v5"

	"github.com/canonical/jimm/v3/internal/dbmodel"
)

var testTime = time.Date(2013, 7, 26, 0, 0, 0, 0, time.UTC)

func (s *dbSuite) TestInsertSecret(c *qt.C) {
	err := s.Database.Migrate(context.Background())
	c.Assert(err, qt.Equals, nil)

	u := dbmodel.Secret{
		Time: testTime,
		Type: "generic",
		Tag:  "123",
		Data: nil,
	}
	c.Assert(s.Database.DB.Create(&u).Error, qt.IsNil)
	secret := dbmodel.Secret{}
	tx := s.Database.DB.First(&secret)
	c.Assert(tx.Error, qt.IsNil)
	c.Assert(secret.Time, qt.DeepEquals, testTime)
	c.Assert(secret.Type, qt.Equals, "generic")
	c.Assert(secret.Tag, qt.Equals, "123")
	c.Assert(secret.Data, qt.IsNil)
}

func (s *dbSuite) TestUpsertSecret(c *qt.C) {
	err := s.Database.Migrate(context.Background())
	c.Assert(err, qt.Equals, nil)
	ctx := context.Background()

	u := dbmodel.Secret{
		Time: testTime,
		Type: "generic",
		Tag:  "123",
		Data: nil,
	}
	c.Assert(s.Database.UpsertSecret(ctx, &u), qt.IsNil)
	newTime := testTime.Add(time.Hour)
	y := dbmodel.Secret{
		Time: newTime,
		Type: "generic",
		Tag:  "123",
		Data: []byte("123"),
	}
	c.Assert(s.Database.UpsertSecret(ctx, &y), qt.IsNil)
	secret := dbmodel.Secret{}
	tx := s.Database.DB.First(&secret)
	c.Assert(tx.Error, qt.IsNil)
	c.Assert(secret.Time, qt.DeepEquals, newTime)
	c.Assert([]byte(secret.Data), qt.DeepEquals, []byte("123"))
}

func (s *dbSuite) TestGetSecret(c *qt.C) {
	err := s.Database.Migrate(context.Background())
	c.Assert(err, qt.Equals, nil)
	ctx := context.Background()

	u := dbmodel.Secret{
		Time: testTime,
		Type: "generic",
		Tag:  "123",
		Data: nil,
	}
	c.Assert(s.Database.DB.Create(&u).Error, qt.IsNil)
	secret := dbmodel.Secret{Type: "generic", Tag: "123"}
	c.Assert(s.Database.GetSecret(ctx, &secret), qt.IsNil)
	c.Assert(secret.Time, qt.DeepEquals, testTime)
	c.Assert(secret.Type, qt.Equals, "generic")
	c.Assert(secret.Tag, qt.Equals, "123")
}

func (s *dbSuite) TestGetSecretFailsWithoutTypeAndTag(c *qt.C) {
	err := s.Database.Migrate(context.Background())
	c.Assert(err, qt.Equals, nil)
	ctx := context.Background()
	secret := dbmodel.Secret{}
	c.Assert(s.Database.GetSecret(ctx, &secret), qt.ErrorMatches, "missing secret tag and type")
}

func (s *dbSuite) TestDeleteSecretFailsWithoutTypeAndTag(c *qt.C) {
	err := s.Database.Migrate(context.Background())
	c.Assert(err, qt.Equals, nil)
	ctx := context.Background()

	secret := dbmodel.Secret{}
	c.Assert(s.Database.DeleteSecret(ctx, &secret), qt.ErrorMatches, "missing secret tag and type")
}

func (s *dbSuite) TestDeleteSecret(c *qt.C) {
	err := s.Database.Migrate(context.Background())
	c.Assert(err, qt.Equals, nil)
	ctx := context.Background()

	u := dbmodel.Secret{
		Time: testTime,
		Type: "generic",
		Tag:  "123",
		Data: nil,
	}
	c.Assert(s.Database.DB.Create(&u).Error, qt.IsNil)
	secret := dbmodel.Secret{Type: "generic", Tag: "123"}
	c.Assert(s.Database.DeleteSecret(ctx, &secret), qt.IsNil)
	var count int64
	c.Assert(s.Database.DB.Model(&dbmodel.Secret{}).Count(&count).Error, qt.IsNil)
	c.Assert(count, qt.Equals, int64(0))
}

func (s *dbSuite) TestPutAndGetCloudCredential(c *qt.C) {
	err := s.Database.Migrate(context.Background())
	c.Assert(err, qt.Equals, nil)
	ctx := context.Background()
	cloudCred := names.NewCloudCredentialTag("foo/bar/bob")
	setAttr := map[string]string{"key": "value"}
	c.Assert(s.Database.Put(ctx, cloudCred, setAttr), qt.IsNil)
	// Verify the type and tag are correct
	secret := dbmodel.Secret{}
	tx := s.Database.DB.First(&secret)
	c.Assert(tx.Error, qt.IsNil)
	c.Assert(secret.Type, qt.Equals, names.CloudCredentialTagKind)
	c.Assert(secret.Tag, qt.Equals, cloudCred.String())
	// Get CloudCred
	attr, err := s.Database.Get(ctx, cloudCred)
	c.Assert(err, qt.IsNil)
	c.Assert(attr, qt.DeepEquals, setAttr)
}

func (s *dbSuite) TestPutAndGetControllerCredential(c *qt.C) {
	err := s.Database.Migrate(context.Background())
	c.Assert(err, qt.Equals, nil)
	ctx := context.Background()
	controllerName := "beef1beef2-0000-0000-000011112222"
	c.Assert(s.Database.PutControllerCredentials(ctx, controllerName, "user", "pass"), qt.IsNil)
	// Verify the type and tag are correct
	secret := dbmodel.Secret{}
	tx := s.Database.DB.First(&secret)
	c.Assert(tx.Error, qt.IsNil)
	c.Assert(secret.Type, qt.Equals, names.ControllerTagKind)
	c.Assert(secret.Tag, qt.Equals, controllerName)
	// Get ControllerCred
	username, password, err := s.Database.GetControllerCredentials(ctx, controllerName)
	c.Assert(err, qt.IsNil)
	c.Assert(username, qt.Equals, "user")
	c.Assert(password, qt.Equals, "pass")
}

func (s *dbSuite) TestGetMissingControllerCredentialDoesNotError(c *qt.C) {
	err := s.Database.Migrate(context.Background())
	c.Assert(err, qt.Equals, nil)
	ctx := context.Background()
	controllerName := "beef1beef2-0000-0000-000011112222"
	username, password, err := s.Database.GetControllerCredentials(ctx, controllerName)
	c.Assert(err, qt.IsNil)
	c.Assert(username, qt.Equals, "")
	c.Assert(password, qt.Equals, "")
}
