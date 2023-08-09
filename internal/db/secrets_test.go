// Copyright 2023 Canonical Ltd.

package db_test

import (
	"context"
	"time"

	qt "github.com/frankban/quicktest"
	"github.com/google/uuid"
	"github.com/juju/names/v4"
	"github.com/lestrrat-go/jwx/v2/jwa"
	"github.com/lestrrat-go/jwx/v2/jwk"

	"github.com/canonical/jimm/internal/db"
	"github.com/canonical/jimm/internal/dbmodel"
)

var testTime = time.Date(2013, 7, 26, 0, 0, 0, 0, time.UTC)

func (s *dbSuite) TestInsertSecret(c *qt.C) {
	err := s.Database.Migrate(context.Background(), true)
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
	c.Assert(secret.Time, qt.Equals, testTime)
	c.Assert(secret.Type, qt.Equals, "generic")
	c.Assert(secret.Tag, qt.Equals, "123")
	c.Assert(secret.Data, qt.IsNil)
}

func (s *dbSuite) TestUpsertSecret(c *qt.C) {
	err := s.Database.Migrate(context.Background(), true)
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
	c.Assert(secret.Time, qt.Equals, newTime)
	c.Assert([]byte(secret.Data), qt.DeepEquals, []byte("123"))
}

func (s *dbSuite) TestGetSecret(c *qt.C) {
	err := s.Database.Migrate(context.Background(), true)
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
	c.Assert(secret.Time, qt.Equals, testTime)
	c.Assert(secret.Type, qt.Equals, "generic")
	c.Assert(secret.Tag, qt.Equals, "123")
}

func (s *dbSuite) TestGetSecretFailsWithoutTypeAndTag(c *qt.C) {
	err := s.Database.Migrate(context.Background(), true)
	c.Assert(err, qt.Equals, nil)
	ctx := context.Background()
	secret := dbmodel.Secret{}
	c.Assert(s.Database.GetSecret(ctx, &secret), qt.ErrorMatches, "missing secret tag and type")
}

func (s *dbSuite) TestDeleteSecretFailsWithoutTypeAndTag(c *qt.C) {
	err := s.Database.Migrate(context.Background(), true)
	c.Assert(err, qt.Equals, nil)
	ctx := context.Background()

	secret := dbmodel.Secret{}
	c.Assert(s.Database.DeleteSecret(ctx, &secret), qt.ErrorMatches, "missing secret tag and type")
}

func (s *dbSuite) TestDeleteSecret(c *qt.C) {
	err := s.Database.Migrate(context.Background(), true)
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
	err := s.Database.Migrate(context.Background(), true)
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
	err := s.Database.Migrate(context.Background(), true)
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
	err := s.Database.Migrate(context.Background(), true)
	c.Assert(err, qt.Equals, nil)
	ctx := context.Background()
	controllerName := "beef1beef2-0000-0000-000011112222"
	// Get ControllerCred
	username, password, err := s.Database.GetControllerCredentials(ctx, controllerName)
	c.Assert(err, qt.IsNil)
	c.Assert(username, qt.Equals, "")
	c.Assert(password, qt.Equals, "")
}

func getJWKS(c *qt.C) jwk.Set {
	set, err := jwk.ParseString(`
	{
		"keys": [
		  {
			"alg": "RS256",
			"kty": "RSA",
			"use": "sig",
			"n": "yeNlzlub94YgerT030codqEztjfU_S6X4DbDA_iVKkjAWtYfPHDzz_sPCT1Axz6isZdf3lHpq_gYX4Sz-cbe4rjmigxUxr-FgKHQy3HeCdK6hNq9ASQvMK9LBOpXDNn7mei6RZWom4wo3CMvvsY1w8tjtfLb-yQwJPltHxShZq5-ihC9irpLI9xEBTgG12q5lGIFPhTl_7inA1PFK97LuSLnTJzW0bj096v_TMDg7pOWm_zHtF53qbVsI0e3v5nmdKXdFf9BjIARRfVrbxVxiZHjU6zL6jY5QJdh1QCmENoejj_ytspMmGW7yMRxzUqgxcAqOBpVm0b-_mW3HoBdjQ",
			"e": "AQAB",
			"kid": "32d2b213-d3fe-436c-9d4c-67a673890620"
		  },
		  {
			"alg": "RS256",
			"kty": "RSA",
			"use": "sig",
			"n": "jbglzlub94YgerT030codqEztjfU_S6X4DbDA_iVKkjAWtYfPHDzz_sPCT1Axz6isZdf3lHpq_gYX4Sz-cbe4rjmigxUxr-FgKHQy3HeCdK6hNq9ASQvMK9LBOpXDNn7mei6RZWom4wo3CMvvsY1w8tjtfLb-yQwJPltHxShZq5-ihC9irpLI9xEBTgG12q5lGIFPhTl_7inA1PFK97LuSLnTJzW0bj096v_TMDg7pOWm_zHtF53qbVsI0e3v5nmdKXdFf9BjIARRfVrbxVxiZHjU6zL6jY5QJdh1QCmENoejj_ytspMmGW7yMRxzUqgxcAqOBpVm0b-_mW3HoBdjQ",
			"e": "FEGB",
			"kid": "32d2b213-d3fe-436c-9d4c-67a673890621"
		  }
		]
	}
	`)
	c.Assert(err, qt.IsNil)
	return set
}

func (s *dbSuite) TestPutAndGetJWKS(c *qt.C) {
	err := s.Database.Migrate(context.Background(), true)
	c.Assert(err, qt.Equals, nil)
	ctx := context.Background()
	jwks := getJWKS(c)
	c.Assert(s.Database.PutJWKS(ctx, jwks), qt.IsNil)
	// Verify the type and tag are correct
	secret := dbmodel.Secret{}
	tx := s.Database.DB.First(&secret)
	c.Assert(tx.Error, qt.IsNil)
	c.Assert(secret.Type, qt.Equals, db.JwksKind)
	c.Assert(secret.Tag, qt.Equals, db.JwksPublicKeyTag)
	// Get JWKS
	gotJwks, err := s.Database.GetJWKS(ctx)
	c.Assert(err, qt.IsNil)
	ki := gotJwks.Keys(ctx)
	gotOneKey := false
	for ki.Next(ctx) {
		gotOneKey = true
		key := ki.Pair().Value.(jwk.Key)
		_, err = uuid.Parse(key.KeyID())
		c.Assert(err, qt.IsNil)
		c.Assert(key.KeyUsage(), qt.Equals, "sig")
		c.Assert(key.Algorithm(), qt.Equals, jwa.RS256)
	}
	c.Assert(gotOneKey, qt.IsTrue)
}

func (s *dbSuite) TestPutAndGetJwksExpiry(c *qt.C) {
	err := s.Database.Migrate(context.Background(), true)
	c.Assert(err, qt.Equals, nil)
	ctx := context.Background()
	expiryTime := time.Date(2023, 7, 26, 0, 0, 0, 0, time.UTC)
	c.Assert(s.Database.PutJWKSExpiry(ctx, expiryTime), qt.IsNil)
	// Verify the type and tag are correct
	secret := dbmodel.Secret{}
	tx := s.Database.DB.First(&secret)
	c.Assert(tx.Error, qt.IsNil)
	c.Assert(secret.Type, qt.Equals, db.JwksKind)
	c.Assert(secret.Tag, qt.Equals, db.JwksExpiryTag)
	// Get ControllerCred
	gotExpiryTime, err := s.Database.GetJWKSExpiry(ctx)
	c.Assert(err, qt.IsNil)
	c.Assert(gotExpiryTime, qt.Equals, expiryTime)
}

func (s *dbSuite) TestPutAndGetJwksPrivateKey(c *qt.C) {
	err := s.Database.Migrate(context.Background(), true)
	c.Assert(err, qt.Equals, nil)
	ctx := context.Background()
	pem := []byte("123")
	c.Assert(s.Database.PutJWKSPrivateKey(ctx, pem), qt.IsNil)
	// Verify the type and tag are correct
	secret := dbmodel.Secret{}
	tx := s.Database.DB.First(&secret)
	c.Assert(tx.Error, qt.IsNil)
	c.Assert(secret.Type, qt.Equals, db.JwksKind)
	c.Assert(secret.Tag, qt.Equals, db.JwksPrivateKeyTag)
	// Get ControllerCred
	gotPem, err := s.Database.GetJWKSPrivateKey(ctx)
	c.Assert(err, qt.IsNil)
	c.Assert(gotPem, qt.DeepEquals, pem)
}

func (s *dbSuite) TestCleanupJWKS(c *qt.C) {
	err := s.Database.Migrate(context.Background(), true)
	c.Assert(err, qt.Equals, nil)
	ctx := context.Background()
	pem := []byte("123")
	c.Assert(s.Database.PutJWKSPrivateKey(ctx, pem), qt.IsNil)
	expiryTime := time.Date(2023, 7, 26, 0, 0, 0, 0, time.UTC)
	c.Assert(s.Database.PutJWKSExpiry(ctx, expiryTime), qt.IsNil)
	jwks := getJWKS(c)
	c.Assert(s.Database.PutJWKS(ctx, jwks), qt.IsNil)
	// Verify 3 secrets exist
	var count int64
	c.Assert(s.Database.DB.Model(&dbmodel.Secret{}).Count(&count).Error, qt.IsNil)
	c.Assert(count, qt.Equals, int64(3))
	// Verify all JWKS secrets are removed
	c.Assert(s.Database.CleanupJWKS(ctx), qt.IsNil)
	c.Assert(s.Database.DB.Model(&dbmodel.Secret{}).Count(&count).Error, qt.IsNil)
	c.Assert(count, qt.Equals, int64(0))
}
