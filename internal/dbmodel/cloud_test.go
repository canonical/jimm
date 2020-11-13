// Copyright 2020 Canonical Ltd.

package dbmodel_test

import (
	"testing"

	qt "github.com/frankban/quicktest"
	"github.com/juju/names/v4"
	"gorm.io/gorm"

	"github.com/CanonicalLtd/jimm/internal/dbmodel"
)

func TestCloudTag(t *testing.T) {
	c := qt.New(t)

	cl := dbmodel.Cloud{
		Name: "test",
	}

	tag := cl.Tag()
	c.Check(tag.String(), qt.Equals, "cloud-test")

	var cl2 dbmodel.Cloud
	cl2.SetTag(tag.(names.CloudTag))
	c.Check(cl2, qt.DeepEquals, cl)
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
		CACertificates:   dbmodel.Strings{"cert1", "cert2"},
		Config: dbmodel.Map{
			"k1": float64(1),
			"k2": "A",
			"k3": map[string]interface{}{"k": []interface{}{"v"}},
		},
	}
	result = db.Create(&cl1)
	c.Assert(result.Error, qt.IsNil)
	c.Check(result.RowsAffected, qt.Equals, int64(1))

	var cl2 dbmodel.Cloud
	result = db.Where("name = ?", "test-cloud").First(&cl2)
	c.Assert(result.Error, qt.IsNil)
	c.Check(cl2, qt.DeepEquals, cl1)

	cl2.CACertificates = dbmodel.Strings{"cert2", "cert3"}
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
	db := gormDB(c, &dbmodel.Cloud{})

	cl1 := dbmodel.Cloud{
		Name:      "test-cloud",
		AuthTypes: dbmodel.Strings{"empty", "userpass"},
	}
	result := db.Create(&cl1)
	c.Assert(result.Error, qt.IsNil)

	var cl2 dbmodel.Cloud
	result = db.Where("name = ?", "test-cloud").First(&cl2)
	c.Assert(result.Error, qt.IsNil)
	c.Check(cl2, qt.DeepEquals, cl1)
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
		Config:           dbmodel.Map{"A": "a"},
	}
	cr2 := dbmodel.CloudRegion{
		Name:             "region2",
		Endpoint:         "https://region2.cloud.example.com",
		IdentityEndpoint: "https://identity.region2.cloud.example.com",
		StorageEndpoint:  "https://storage.region2.cloud.example.com",
		Config:           dbmodel.Map{"A": "b"},
	}
	cl1.Regions = append(cl1.Regions, cr1, cr2)
	result := db.Create(&cl1)
	c.Assert(result.Error, qt.IsNil)

	var cl2 dbmodel.Cloud
	result = db.Where("name = ?", "test-cloud").Preload("Regions").First(&cl2)
	c.Assert(result.Error, qt.IsNil)
	c.Check(cl2, qt.DeepEquals, cl1)
}

func TestCloudRegionControllers(t *testing.T) {
	c := qt.New(t)
	db := gormDB(c, &dbmodel.Cloud{}, &dbmodel.CloudRegion{}, &dbmodel.CloudRegionControllerPriority{}, &dbmodel.Controller{})

	cl := dbmodel.Cloud{
		Name: "test-cloud",
	}
	result := db.Create(&cl)
	c.Assert(result.Error, qt.IsNil)

	cr1 := dbmodel.CloudRegion{
		Cloud: cl,
		Name:  "test-region-1",
	}
	result = db.Create(&cr1)
	c.Assert(result.Error, qt.IsNil)

	cr2 := dbmodel.CloudRegion{
		Cloud: cl,
		Name:  "test-region-2",
	}
	result = db.Create(&cr2)
	c.Assert(result.Error, qt.IsNil)

	ctl1 := dbmodel.Controller{
		Name: "test-controller-1",
		CloudRegions: []dbmodel.CloudRegionControllerPriority{{
			CloudRegion: cr1,
			Priority:    dbmodel.CloudRegionControllerPrioritySupported,
		}, {
			CloudRegion: cr2,
			Priority:    dbmodel.CloudRegionControllerPriorityDeployed,
		}},
	}
	result = db.Create(&ctl1)
	c.Assert(result.Error, qt.IsNil)

	ctl2 := dbmodel.Controller{
		Name: "test-controller-2",
		CloudRegions: []dbmodel.CloudRegionControllerPriority{{
			CloudRegion: cr1,
			Priority:    dbmodel.CloudRegionControllerPriorityDeployed,
		}, {
			CloudRegion: cr2,
			Priority:    dbmodel.CloudRegionControllerPrioritySupported,
		}},
	}
	result = db.Create(&ctl2)
	c.Assert(result.Error, qt.IsNil)

	var crcps []dbmodel.CloudRegionControllerPriority
	err := db.Model(cr1).Order("priority desc").Preload("Controller").Association("Controllers").Find(&crcps)
	c.Assert(err, qt.IsNil)
	c.Check(crcps, qt.HasLen, 2)
	c.Check(crcps[0].Controller, qt.DeepEquals, dbmodel.Controller{
		Model: ctl2.Model,
		Name:  ctl2.Name,
	})
	c.Check(crcps[1].Controller, qt.DeepEquals, dbmodel.Controller{
		Model: ctl1.Model,
		Name:  ctl1.Name,
	})

	crcps = crcps[:0]
	err = db.Model(cr2).Order("priority desc").Preload("Controller").Association("Controllers").Find(&crcps)
	c.Assert(err, qt.IsNil)
	c.Check(crcps, qt.HasLen, 2)
	c.Check(crcps[0].Controller, qt.DeepEquals, dbmodel.Controller{
		Model: ctl1.Model,
		Name:  ctl1.Name,
	})
	c.Check(crcps[1].Controller, qt.DeepEquals, dbmodel.Controller{
		Model: ctl2.Model,
		Name:  ctl2.Name,
	})
}

func TestCloudRegion(t *testing.T) {
	c := qt.New(t)

	cl := dbmodel.Cloud{
		Name: "test-cloud",
		Regions: []dbmodel.CloudRegion{{
			Name:     "test-region",
			Endpoint: "example.com",
		}},
	}
	r := cl.Region("test-region")
	c.Check(r.Endpoint, qt.Equals, "example.com")
	c.Check(cl.Region("test-region-2"), qt.DeepEquals, dbmodel.CloudRegion{})
}
