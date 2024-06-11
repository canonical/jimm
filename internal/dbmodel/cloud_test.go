// Copyright 2020 Canonical Ltd.

package dbmodel_test

import (
	"testing"

	qt "github.com/frankban/quicktest"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v5"
	"gorm.io/gorm"

	"github.com/canonical/jimm/internal/dbmodel"
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
	db := gormDB(c)

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
	c.Check(result.Error, qt.ErrorMatches, `.*violates unique constraint "clouds_name_key".*`)
}

func TestToJujuCloud(t *testing.T) {
	c := qt.New(t)

	cl := dbmodel.Cloud{
		Name:             "test-cloud",
		Type:             "test-provider",
		HostCloudRegion:  "test-cloud/test-region",
		Endpoint:         "https://cloud.example.com",
		IdentityEndpoint: "https://identity.cloud.example.com",
		StorageEndpoint:  "https://storage.cloud.example.com",
		CACertificates:   dbmodel.Strings{"cert1", "cert2"},
		Regions: []dbmodel.CloudRegion{{
			Name:             "test-region",
			Endpoint:         "https://region.example.com",
			IdentityEndpoint: "https://identity.region.example.com",
			StorageEndpoint:  "https://storage.region.example.com",
			Config: dbmodel.Map{
				"k1": float64(2),
				"k2": "B",
				"k3": map[string]interface{}{"k": []interface{}{"V"}},
			},
		}},
		Config: dbmodel.Map{
			"k1": float64(1),
			"k2": "A",
			"k3": map[string]interface{}{"k": []interface{}{"v"}},
		},
	}
	pc := cl.ToJujuCloud()
	c.Check(pc, qt.DeepEquals, jujuparams.Cloud{
		Type:             "test-provider",
		HostCloudRegion:  "test-cloud/test-region",
		Endpoint:         "https://cloud.example.com",
		IdentityEndpoint: "https://identity.cloud.example.com",
		StorageEndpoint:  "https://storage.cloud.example.com",
		Regions: []jujuparams.CloudRegion{{
			Name:             "test-region",
			Endpoint:         "https://region.example.com",
			IdentityEndpoint: "https://identity.region.example.com",
			StorageEndpoint:  "https://storage.region.example.com",
		}},
		CACertificates: []string{"cert1", "cert2"},
		Config: map[string]interface{}{
			"k1": float64(1),
			"k2": "A",
			"k3": map[string]interface{}{"k": []interface{}{string("v")}},
		},
		RegionConfig: map[string]map[string]interface{}{
			"test-region": {
				"k1": float64(2),
				"k2": "B",
				"k3": map[string]interface{}{"k": []interface{}{string("V")}},
			},
		},
	})
}

func TestFromJujuCloud(t *testing.T) {
	c := qt.New(t)

	jcld := jujuparams.Cloud{
		Type:             "test-provider",
		HostCloudRegion:  "test-cloud/test-region",
		AuthTypes:        []string{"empty"},
		Endpoint:         "https://cloud.example.com",
		IdentityEndpoint: "https://identity.cloud.example.com",
		StorageEndpoint:  "https://storage.cloud.example.com",
		Regions: []jujuparams.CloudRegion{{
			Name:             "test-region",
			Endpoint:         "https://region.example.com",
			IdentityEndpoint: "https://identity.region.example.com",
			StorageEndpoint:  "https://storage.region.example.com",
		}},
		CACertificates: []string{"cert1", "cert2"},
		Config: map[string]interface{}{
			"k1": float64(1),
			"k2": "A",
			"k3": map[string]interface{}{"k": []interface{}{string("v")}},
		},
		RegionConfig: map[string]map[string]interface{}{
			"test-region": {
				"k1": float64(2),
				"k2": "B",
				"k3": map[string]interface{}{"k": []interface{}{string("V")}},
			},
		},
	}

	cl := dbmodel.Cloud{
		Name: "test-cloud",
	}

	cl.FromJujuCloud(jcld)
	c.Check(cl, qt.DeepEquals, dbmodel.Cloud{
		Name:             "test-cloud",
		Type:             "test-provider",
		HostCloudRegion:  "test-cloud/test-region",
		AuthTypes:        dbmodel.Strings{"empty"},
		Endpoint:         "https://cloud.example.com",
		IdentityEndpoint: "https://identity.cloud.example.com",
		StorageEndpoint:  "https://storage.cloud.example.com",
		Regions: []dbmodel.CloudRegion{{
			Name:             "test-region",
			Endpoint:         "https://region.example.com",
			IdentityEndpoint: "https://identity.region.example.com",
			StorageEndpoint:  "https://storage.region.example.com",
			Config: dbmodel.Map{
				"k1": float64(2),
				"k2": "B",
				"k3": map[string]interface{}{"k": []interface{}{string("V")}},
			},
		}},
		CACertificates: dbmodel.Strings{"cert1", "cert2"},
		Config: dbmodel.Map{
			"k1": float64(1),
			"k2": "A",
			"k3": map[string]interface{}{"k": []interface{}{string("v")}},
		},
	})
}

func TestToJujuCloudInfo(t *testing.T) {
	c := qt.New(t)

	cl := dbmodel.Cloud{
		Name:             "test-cloud",
		Type:             "test-provider",
		HostCloudRegion:  "test-cloud/test-region",
		Endpoint:         "https://cloud.example.com",
		IdentityEndpoint: "https://identity.cloud.example.com",
		StorageEndpoint:  "https://storage.cloud.example.com",
		CACertificates:   dbmodel.Strings{"cert1", "cert2"},
		Regions: []dbmodel.CloudRegion{{
			Name:             "test-region",
			Endpoint:         "https://region.example.com",
			IdentityEndpoint: "https://identity.region.example.com",
			StorageEndpoint:  "https://storage.region.example.com",
			Config: dbmodel.Map{
				"k1": float64(2),
				"k2": "B",
				"k3": map[string]interface{}{"k": []interface{}{"V"}},
			},
		}},
		Config: dbmodel.Map{
			"k1": float64(1),
			"k2": "A",
			"k3": map[string]interface{}{"k": []interface{}{"v"}},
		},
	}
	pci := cl.ToJujuCloudInfo()
	c.Check(pci, qt.DeepEquals, jujuparams.CloudInfo{
		CloudDetails: jujuparams.CloudDetails{
			Type:             "test-provider",
			Endpoint:         "https://cloud.example.com",
			IdentityEndpoint: "https://identity.cloud.example.com",
			StorageEndpoint:  "https://storage.cloud.example.com",
			Regions: []jujuparams.CloudRegion{{
				Name:             "test-region",
				Endpoint:         "https://region.example.com",
				IdentityEndpoint: "https://identity.region.example.com",
				StorageEndpoint:  "https://storage.region.example.com",
			}},
		},
	})
}

func TestCloudAuthTypes(t *testing.T) {
	c := qt.New(t)
	db := gormDB(c)

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
	db := gormDB(c)

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
	db := gormDB(c)

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
		Name:        "test-controller-1",
		CloudName:   "test-cloud",
		CloudRegion: "test-region-2",
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
		Name:        "test-controller-2",
		CloudName:   "test-cloud",
		CloudRegion: "test-region-1",
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
		ID:          ctl2.ID,
		CreatedAt:   ctl2.CreatedAt,
		UpdatedAt:   ctl2.UpdatedAt,
		Name:        ctl2.Name,
		CloudName:   ctl2.CloudName,
		CloudRegion: ctl2.CloudRegion,
	})
	c.Check(crcps[1].Controller, qt.DeepEquals, dbmodel.Controller{
		ID:          ctl1.ID,
		CreatedAt:   ctl1.CreatedAt,
		UpdatedAt:   ctl1.UpdatedAt,
		Name:        ctl1.Name,
		CloudName:   ctl1.CloudName,
		CloudRegion: ctl1.CloudRegion,
	})

	crcps = crcps[:0]
	err = db.Model(cr2).Order("priority desc").Preload("Controller").Association("Controllers").Find(&crcps)
	c.Assert(err, qt.IsNil)
	c.Check(crcps, qt.HasLen, 2)
	c.Check(crcps[0].Controller, qt.DeepEquals, dbmodel.Controller{
		ID:          ctl1.ID,
		CreatedAt:   ctl1.CreatedAt,
		UpdatedAt:   ctl1.UpdatedAt,
		Name:        ctl1.Name,
		CloudName:   ctl1.CloudName,
		CloudRegion: ctl1.CloudRegion,
	})
	c.Check(crcps[1].Controller, qt.DeepEquals, dbmodel.Controller{
		ID:          ctl2.ID,
		CreatedAt:   ctl2.CreatedAt,
		UpdatedAt:   ctl2.UpdatedAt,
		Name:        ctl2.Name,
		CloudName:   ctl2.CloudName,
		CloudRegion: ctl2.CloudRegion,
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

func TestReuseDeletedCloudName(t *testing.T) {
	c := qt.New(t)
	db := gormDB(c)

	cl1 := dbmodel.Cloud{
		Name: "test-cloud",
	}
	result := db.Create(&cl1)
	c.Assert(result.Error, qt.IsNil)

	cl2 := dbmodel.Cloud{
		Name: "test-cloud",
	}
	result = db.Create(&cl2)
	c.Check(result.Error, qt.ErrorMatches, `.*violates unique constraint "clouds_name_key".*`)

	result = db.Delete(&cl1)
	c.Assert(result.Error, qt.IsNil)

	result = db.First(&cl1)
	c.Check(result.Error, qt.Equals, gorm.ErrRecordNotFound)

	result = db.Create(&cl2)
	c.Assert(result.Error, qt.IsNil)
}
