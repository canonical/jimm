// Copyright 2020 Canonical Ltd.

package dbmodel_test

import (
	"testing"

	qt "github.com/frankban/quicktest"
	jujuparams "github.com/juju/juju/apiserver/params"

	"github.com/canonical/jimm/internal/dbmodel"
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

func TestPortsGormDataType(t *testing.T) {
	c := qt.New(t)

	var p dbmodel.Ports
	c.Assert(p.GormDataType(), qt.Equals, "bytes")
}

func TestPortsValue(t *testing.T) {
	c := qt.New(t)

	var p dbmodel.Ports
	v, err := p.Value()
	c.Assert(err, qt.IsNil)
	c.Check(v, qt.Equals, nil)

	p = append(p, jujuparams.Port{
		Protocol: "tcp",
		Number:   1,
	})
	p = append(p, jujuparams.Port{
		Protocol: "udp",
		Number:   2,
	})
	v, err = p.Value()
	c.Assert(err, qt.IsNil)

	var p2 dbmodel.Ports
	err = p2.Scan(v)
	c.Assert(err, qt.IsNil)
	c.Check(p2, qt.DeepEquals, p)
}

func TestPortsScan(t *testing.T) {
	c := qt.New(t)

	var p dbmodel.Ports
	err := p.Scan(`[{"protocol":"tcp","number":1}]`)
	c.Assert(err, qt.IsNil)
	c.Check(p, qt.DeepEquals, dbmodel.Ports{{Protocol: "tcp", Number: 1}})

	err = p.Scan(nil)
	c.Assert(err, qt.IsNil)
	c.Check(p, qt.IsNil)

	err = p.Scan([]byte(`[{"protocol":"udp","number":2}]`))
	c.Assert(err, qt.IsNil)
	c.Check(p, qt.DeepEquals, dbmodel.Ports{{Protocol: "udp", Number: 2}})

	err = p.Scan(0)
	c.Check(err, qt.ErrorMatches, `cannot unmarshal int as Ports`)
}

func TestPortRangesGormDataType(t *testing.T) {
	c := qt.New(t)

	var pr dbmodel.PortRanges
	c.Assert(pr.GormDataType(), qt.Equals, "bytes")
}

func TestPortRangesValue(t *testing.T) {
	c := qt.New(t)

	var pr dbmodel.PortRanges
	v, err := pr.Value()
	c.Assert(err, qt.IsNil)
	c.Check(v, qt.Equals, nil)

	pr = append(pr, jujuparams.PortRange{
		FromPort: 1,
		ToPort:   2,
		Protocol: "tcp",
	})
	pr = append(pr, jujuparams.PortRange{
		FromPort: 3,
		ToPort:   4,
		Protocol: "udp",
	})
	v, err = pr.Value()
	c.Assert(err, qt.IsNil)

	var pr2 dbmodel.PortRanges
	err = pr2.Scan(v)
	c.Assert(err, qt.IsNil)
	c.Check(pr2, qt.DeepEquals, pr)
}

func TestPortRangesScan(t *testing.T) {
	c := qt.New(t)

	var pr dbmodel.PortRanges
	err := pr.Scan(`[{"protocol":"tcp","from-port":1,"to-port":2}]`)
	c.Assert(err, qt.IsNil)
	c.Check(pr, qt.DeepEquals, dbmodel.PortRanges{{FromPort: 1, ToPort: 2, Protocol: "tcp"}})

	err = pr.Scan(nil)
	c.Assert(err, qt.IsNil)
	c.Check(pr, qt.IsNil)

	err = pr.Scan([]byte(`[{"protocol":"udp","from-port":3,"to-port":4}]`))
	c.Assert(err, qt.IsNil)
	c.Check(pr, qt.DeepEquals, dbmodel.PortRanges{{FromPort: 3, ToPort: 4, Protocol: "udp"}})

	err = pr.Scan(0)
	c.Check(err, qt.ErrorMatches, `cannot unmarshal int as PortRanges`)
}

func TestNullUint64GormDataType(t *testing.T) {
	c := qt.New(t)

	var n dbmodel.NullUint64
	c.Check(n.GormDataType(), qt.Equals, "uint")
}

func TestNullUint64Value(t *testing.T) {
	c := qt.New(t)

	var n dbmodel.NullUint64
	v, err := n.Value()
	c.Assert(err, qt.IsNil)
	c.Check(v, qt.Equals, nil)

	n.Valid = true
	v, err = n.Value()
	c.Assert(err, qt.IsNil)
	c.Check(v, qt.Equals, int64(0))

	n.Uint64 = 1
	v, err = n.Value()
	c.Assert(err, qt.IsNil)
	c.Check(v, qt.Equals, int64(1))
}

func TestNullUint64Scan(t *testing.T) {
	c := qt.New(t)

	var n dbmodel.NullUint64
	err := n.Scan(uint64(0))
	c.Assert(err, qt.IsNil)
	c.Check(n, qt.DeepEquals, dbmodel.NullUint64{
		Uint64: 0,
		Valid:  true,
	})

	err = n.Scan(uint64(10))
	c.Assert(err, qt.IsNil)
	c.Check(n, qt.DeepEquals, dbmodel.NullUint64{
		Uint64: 10,
		Valid:  true,
	})

	err = n.Scan(nil)
	c.Assert(err, qt.IsNil)
	c.Check(n, qt.DeepEquals, dbmodel.NullUint64{
		Uint64: 0,
		Valid:  false,
	})

	err = n.Scan(int64(0))
	c.Assert(err, qt.IsNil)
	c.Check(n, qt.DeepEquals, dbmodel.NullUint64{
		Uint64: 0,
		Valid:  true,
	})

	err = n.Scan(int64(10))
	c.Assert(err, qt.IsNil)
	c.Check(n, qt.DeepEquals, dbmodel.NullUint64{
		Uint64: 10,
		Valid:  true,
	})
}
