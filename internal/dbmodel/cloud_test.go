// Copyright 2020 Canonical Ltd.

package dbmodel_test

import (
	"testing"

	qt "github.com/frankban/quicktest"
	"gorm.io/gorm"

	"github.com/CanonicalLtd/jimm/internal/dbmodel"
)

func TestCloudTag(t *testing.T) {
	c := qt.New(t)

	cl := dbmodel.Cloud{
		Name: "test",
	}

	c.Check(cl.Tag().String(), qt.Equals, "cloud-test")
}

func TestCloud(t *testing.T) {
	c := qt.New(t)
	db := gormDB(c, &dbmodel.Cloud{})

	var cl0 dbmodel.Cloud
	result := db.Where("name = ?", "test-cloud").First(&cl0)
	c.Check(result.Error, qt.Equals, gorm.ErrRecordNotFound)

	cl1 := dbmodel.Cloud{
		Name:             "test-cloud",
		Type:             "test-provider",
		HostCloudRegion:  "test-cloud/test-region",
		Endpoint:         "https://cloud.example.com",
		IdentityEndpoint: "https://identity.cloud.example.com",
		StorageEndpoint:  "https://storage.cloud.example.com",
	}
	cl1.SetCACertificates([]string{"cert1", "cert2"})
	cl1.SetConfig(map[string]interface{}{
		"k1": float64(1),
		"k2": "A",
		"k3": map[string]interface{}{"k": []interface{}{"v"}},
	})
	result = db.Create(&cl1)
	c.Assert(result.Error, qt.IsNil)
	c.Check(result.RowsAffected, qt.Equals, int64(1))

	var cl2 dbmodel.Cloud
	result = db.Where("name = ?", "test-cloud").First(&cl2)
	c.Assert(result.Error, qt.IsNil)
	c.Check(cl2, qt.DeepEquals, cl1)
	c.Check(cl2.CACertificates(), qt.DeepEquals, []string{"cert1", "cert2"})
	c.Check(cl2.Config(), qt.DeepEquals, map[string]interface{}{
		"k1": float64(1),
		"k2": "A",
		"k3": map[string]interface{}{"k": []interface{}{"v"}},
	})

	cl2.SetCACertificates([]string{"cert2", "cert3"})
	result = db.Save(&cl2)
	c.Assert(result.Error, qt.IsNil)
	c.Check(result.RowsAffected, qt.Equals, int64(1))

	var cl3 dbmodel.Cloud
	result = db.Where("name = ?", "test-cloud").First(&cl3)
	c.Assert(result.Error, qt.IsNil)
	c.Check(cl3, qt.DeepEquals, cl2)

	cl4 := dbmodel.Cloud{
		Name: "test-cloud",
	}
	result = db.Create(&cl4)
	c.Check(result.Error, qt.ErrorMatches, `UNIQUE constraint failed: clouds.name`)
}

func TestCloudAuthTypes(t *testing.T) {
	c := qt.New(t)
	db := gormDB(c, &dbmodel.AuthType{}, &dbmodel.Cloud{})

	cl1 := dbmodel.Cloud{
		Name: "test-cloud",
	}
	cl1.SetAuthTypes([]string{"empty", "userpass"})
	result := db.Create(&cl1)
	c.Assert(result.Error, qt.IsNil)

	var cl2 dbmodel.Cloud
	result = db.Where("name = ?", "test-cloud").Preload("AuthTypes_").First(&cl2)
	c.Assert(result.Error, qt.IsNil)
	c.Check(cl2, qt.DeepEquals, cl1)

	c.Check(cl2.AuthTypes(), qt.DeepEquals, []string{"empty", "userpass"})
}

func TestCloudRegions(t *testing.T) {
	c := qt.New(t)
	db := gormDB(c, &dbmodel.Cloud{}, &dbmodel.CloudRegion{})

	cl1 := dbmodel.Cloud{
		Name:             "test-cloud",
		Type:             "test-provider",
		Endpoint:         "https://cloud.example.com",
		IdentityEndpoint: "https://identity.cloud.example.com",
		StorageEndpoint:  "https://storage.cloud.example.com",
	}
	cr1 := dbmodel.CloudRegion{
		Name:             "region1",
		Endpoint:         "https://region1.cloud.example.com",
		IdentityEndpoint: "https://identity.region1.cloud.example.com",
		StorageEndpoint:  "https://storage.region1.cloud.example.com",
	}
	cr1.SetConfig(map[string]interface{}{"A": "a"})
	cr2 := dbmodel.CloudRegion{
		Name:             "region2",
		Endpoint:         "https://region2.cloud.example.com",
		IdentityEndpoint: "https://identity.region2.cloud.example.com",
		StorageEndpoint:  "https://storage.region2.cloud.example.com",
	}
	cr2.SetConfig(map[string]interface{}{"A": "b"})
	cl1.Regions = append(cl1.Regions, cr1, cr2)
	result := db.Create(&cl1)
	c.Assert(result.Error, qt.IsNil)

	var cl2 dbmodel.Cloud
	result = db.Where("name = ?", "test-cloud").Preload("Regions").First(&cl2)
	c.Assert(result.Error, qt.IsNil)
	c.Check(cl2, qt.DeepEquals, cl1)
	c.Check(cl2.Region("region2").Config(), qt.DeepEquals, map[string]interface{}{"A": "b"})
}
