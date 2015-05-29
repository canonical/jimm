// Copyright 2015 Canonical Ltd.

package config_test

import (
	"io/ioutil"
	"path"
	"testing"

	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/macaroon-bakery.v1/bakery"

	"github.com/CanonicalLtd/jem/config"
)

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

type ConfigSuite struct {
	jujutesting.IsolationSuite
}

var _ = gc.Suite(&ConfigSuite{})

const testConfig = `
mongo-addr: localhost:23456
api-addr: blah:2324
foo: 1
bar: false
state-server-admin: adminuser
identity-location: localhost:18082
identity-public-key: +qNbDWly3kRTDVv2UN03hrv/CBt4W6nxY5dHdw+KJFA=
`

func (s *ConfigSuite) readConfig(c *gc.C, content string) (*config.Config, error) {
	// Write the configuration content to file.
	path := path.Join(c.MkDir(), "jemd.conf")
	err := ioutil.WriteFile(path, []byte(content), 0666)
	c.Assert(err, gc.IsNil)

	// Read the configuration.
	return config.Read(path)
}

func (s *ConfigSuite) TestRead(c *gc.C) {
	conf, err := s.readConfig(c, testConfig)
	c.Assert(err, gc.IsNil)
	c.Assert(conf, jc.DeepEquals, &config.Config{
		MongoAddr:        "localhost:23456",
		APIAddr:          "blah:2324",
		StateServerAdmin: "adminuser",

		IdentityLocation: "localhost:18082",
		IdentityPublicKey: &bakery.PublicKey{
			Key: mustParseKey("+qNbDWly3kRTDVv2UN03hrv/CBt4W6nxY5dHdw+KJFA="),
		},
	})
}

func (s *ConfigSuite) TestReadConfigError(c *gc.C) {
	cfg, err := config.Read(path.Join(c.MkDir(), "jemd.conf"))
	c.Assert(err, gc.ErrorMatches, ".* no such file or directory")
	c.Assert(cfg, gc.IsNil)
}

func (s *ConfigSuite) TestValidateConfigError(c *gc.C) {
	cfg, err := s.readConfig(c, "")
	c.Assert(err, gc.ErrorMatches, "missing fields mongo-addr, api-addr, state-server-admin, identity-public-key, identity-location in config file")
	c.Assert(cfg, gc.IsNil)
}

func mustParseKey(s string) bakery.Key {
	var k bakery.Key
	err := k.UnmarshalText([]byte(s))
	if err != nil {
		panic(err)
	}
	return k
}
