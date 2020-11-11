// Copyright 2020 Canonical Ltd.

package dbmodel_test

import (
	"testing"

	"github.com/CanonicalLtd/jimm/internal/dbmodel"
	qt "github.com/frankban/quicktest"
	jujuparams "github.com/juju/juju/apiserver/params"
)

func TestControllerTag(t *testing.T) {
	c := qt.New(t)

	ctl := dbmodel.Controller{
		UUID: "11111111-2222-3333-4444-555555555555",
	}

	c.Check(ctl.Tag().String(), qt.Equals, "controller-11111111-2222-3333-4444-555555555555")
}

func TestControllerHostPorts(t *testing.T) {
	c := qt.New(t)

	var hpss [][]jujuparams.HostPort
	hpss = append(hpss, []jujuparams.HostPort{{
		Address: jujuparams.Address{
			Value:           "1.1.1.1",
			Type:            "t1",
			Scope:           "s1",
			SpaceName:       "sp1",
			ProviderSpaceID: "psp1",
		},
		Port: 1,
	}, {
		Address: jujuparams.Address{
			Value: "2.2.2.2",
		},
		Port: 2,
	}})

	hpss = append(hpss, []jujuparams.HostPort{{
		Address: jujuparams.Address{
			Value:           "1.1.1.1",
			Type:            "t1",
			Scope:           "s1",
			SpaceName:       "sp1",
			ProviderSpaceID: "psp1",
		},
		Port: 1,
	}, {
		Address: jujuparams.Address{
			Value: "3.3.3.3",
		},
		Port: 3,
	}})

	var ctl dbmodel.Controller
	ctl.SetHostPorts(hpss)
	hpss2 := ctl.HostPorts()

	c.Check(hpss2, qt.DeepEquals, hpss)
}

func TestController(t *testing.T) {
	c := qt.New(t)
	db := gormDB(c, &dbmodel.Controller{})

	ctl := dbmodel.Controller{
		Name:          "test-controller",
		UUID:          "00000000-0000-0000-0000-000000000001",
		AdminUser:     "admin",
		AdminPassword: "pw",
		CACertificate: "ca-cert",
		PublicAddress: "controller.example.com:443",
	}
	ctl.SetHostPorts([][]jujuparams.HostPort{{{
		Address: jujuparams.Address{
			Value: "1.1.1.1",
		},
		Port: 17070,
	}}})
	result := db.Create(&ctl)
	c.Assert(result.Error, qt.IsNil)

	var ctl2 dbmodel.Controller
	result = db.Where("name = ?", "test-controller").First(&ctl2)
	c.Assert(result.Error, qt.IsNil)

	c.Check(ctl2, qt.DeepEquals, ctl)
}
