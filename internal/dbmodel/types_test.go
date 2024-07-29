// Copyright 2020 Canonical Ltd.

package dbmodel_test

import (
	"testing"

	qt "github.com/frankban/quicktest"
	jujuparams "github.com/juju/juju/rpc/params"

	"github.com/canonical/jimm/v3/internal/dbmodel"
)

func TestStringsGormDataType(t *testing.T) {
	c := qt.New(t)

	var s dbmodel.Strings
	c.Assert(s.GormDataType(), qt.Equals, "bytes")
}

func TestStringsValue(t *testing.T) {
	c := qt.New(t)

	var s dbmodel.Strings
	v, err := s.Value()
	c.Assert(err, qt.IsNil)
	c.Check(v, qt.Equals, nil)

	s = append(s, "a", "b")
	v, err = s.Value()
	c.Assert(err, qt.IsNil)

	var s2 dbmodel.Strings
	err = s2.Scan(v)
	c.Assert(err, qt.IsNil)
	c.Check(s2, qt.DeepEquals, s)
}

func TestStringsScan(t *testing.T) {
	c := qt.New(t)

	var s dbmodel.Strings
	err := s.Scan(`["a","b"]`)
	c.Assert(err, qt.IsNil)
	c.Check(s, qt.DeepEquals, dbmodel.Strings{"a", "b"})

	err = s.Scan(nil)
	c.Assert(err, qt.IsNil)
	c.Check(s, qt.IsNil)

	err = s.Scan([]byte(`["x","y"]`))
	c.Assert(err, qt.IsNil)
	c.Check(s, qt.DeepEquals, dbmodel.Strings{"x", "y"})

	err = s.Scan(0)
	c.Check(err, qt.ErrorMatches, `cannot unmarshal int as Strings`)
}

func TestStringMapGormDataType(t *testing.T) {
	c := qt.New(t)

	var s dbmodel.StringMap
	c.Assert(s.GormDataType(), qt.Equals, "bytes")
}

func TestStringMapValue(t *testing.T) {
	c := qt.New(t)

	var m dbmodel.StringMap
	v, err := m.Value()
	c.Assert(err, qt.IsNil)
	c.Check(v, qt.Equals, nil)

	m = dbmodel.StringMap{"a": "b", "c": "d"}
	v, err = m.Value()
	c.Assert(err, qt.IsNil)

	var m2 dbmodel.StringMap
	err = m2.Scan(v)
	c.Assert(err, qt.IsNil)
	c.Check(m2, qt.DeepEquals, m)
}

func TestStringMapScan(t *testing.T) {
	c := qt.New(t)

	var m dbmodel.StringMap
	err := m.Scan(`{"a":"b"}`)
	c.Assert(err, qt.IsNil)
	c.Check(m, qt.DeepEquals, dbmodel.StringMap{"a": "b"})

	err = m.Scan(nil)
	c.Assert(err, qt.IsNil)
	c.Check(m, qt.IsNil)

	err = m.Scan([]byte(`{"x":"y"}`))
	c.Assert(err, qt.IsNil)
	c.Check(m, qt.DeepEquals, dbmodel.StringMap{"x": "y"})

	err = m.Scan(0)
	c.Check(err, qt.ErrorMatches, `cannot unmarshal int as StringMap`)
}

func TestMapGormDataType(t *testing.T) {
	c := qt.New(t)

	var s dbmodel.Map
	c.Assert(s.GormDataType(), qt.Equals, "bytes")
}

func TestMapValue(t *testing.T) {
	c := qt.New(t)

	var m dbmodel.Map
	v, err := m.Value()
	c.Assert(err, qt.IsNil)
	c.Check(v, qt.Equals, nil)

	m = dbmodel.Map{"a": "b", "c": float64(0)}
	v, err = m.Value()
	c.Assert(err, qt.IsNil)

	var m2 dbmodel.Map
	err = m2.Scan(v)
	c.Assert(err, qt.IsNil)
	c.Check(m2, qt.DeepEquals, m)
}

func TestMapScan(t *testing.T) {
	c := qt.New(t)

	var m dbmodel.Map
	err := m.Scan(`{"a":1}`)
	c.Assert(err, qt.IsNil)
	c.Check(m, qt.DeepEquals, dbmodel.Map{"a": float64(1)})

	err = m.Scan(nil)
	c.Assert(err, qt.IsNil)
	c.Check(m, qt.IsNil)

	err = m.Scan([]byte(`{"x":"y"}`))
	c.Assert(err, qt.IsNil)
	c.Check(m, qt.DeepEquals, dbmodel.Map{"x": "y"})

	err = m.Scan(0)
	c.Check(err, qt.ErrorMatches, `cannot unmarshal int as Map`)
}

func TestHostPortsGormDataType(t *testing.T) {
	c := qt.New(t)

	var s dbmodel.HostPorts
	c.Assert(s.GormDataType(), qt.Equals, "bytes")
}

func TestHostPortsValue(t *testing.T) {
	c := qt.New(t)

	var hp dbmodel.HostPorts
	v, err := hp.Value()
	c.Assert(err, qt.IsNil)
	c.Check(v, qt.Equals, nil)

	hp = append(hp, []jujuparams.HostPort{{
		Address: jujuparams.Address{
			Value: "example.com",
		},
		Port: 1,
	}})
	hp = append(hp, []jujuparams.HostPort{{
		Address: jujuparams.Address{
			Value: "example.com",
		},
		Port: 2,
	}})
	v, err = hp.Value()
	c.Assert(err, qt.IsNil)

	var hp2 dbmodel.HostPorts
	err = hp2.Scan(v)
	c.Assert(err, qt.IsNil)
	c.Check(hp2, qt.DeepEquals, hp)
}

func TestHostPortsScan(t *testing.T) {
	c := qt.New(t)

	var hp dbmodel.HostPorts
	err := hp.Scan(`[[{"value":"example.com","port":1}]]`)
	c.Assert(err, qt.IsNil)
	c.Check(hp, qt.DeepEquals, dbmodel.HostPorts{{{Address: jujuparams.Address{Value: "example.com"}, Port: 1}}})

	err = hp.Scan(nil)
	c.Assert(err, qt.IsNil)
	c.Check(hp, qt.IsNil)

	err = hp.Scan([]byte(`[[{"value":"2.example.com","port":2}]]`))
	c.Assert(err, qt.IsNil)
	c.Check(hp, qt.DeepEquals, dbmodel.HostPorts{{{Address: jujuparams.Address{Value: "2.example.com"}, Port: 2}}})

	err = hp.Scan(0)
	c.Check(err, qt.ErrorMatches, `cannot unmarshal int as HostPorts`)
}
