// Copyright 2015 Canonical Ltd.

package config

import (
	"fmt"
	"io/ioutil"
	"os"
	"strings"

	"gopkg.in/errgo.v1"
	"gopkg.in/macaroon-bakery.v1/bakery"
	"gopkg.in/yaml.v2"
)

type Config struct {
	MongoAddr         string            `yaml:"mongo-addr"`
	APIAddr           string            `yaml:"api-addr"`
	StateServerAdmin  string            `yaml:"state-server-admin"`
	IdentityPublicKey *bakery.PublicKey `yaml:"identity-public-key"`
	IdentityLocation  string            `yaml:"identity-location"`
}

func (c *Config) validate() error {
	var missing []string
	if c.MongoAddr == "" {
		missing = append(missing, "mongo-addr")
	}
	if c.APIAddr == "" {
		missing = append(missing, "api-addr")
	}
	if c.StateServerAdmin == "" {
		missing = append(missing, "state-server-admin")
	}
	if c.IdentityPublicKey == nil {
		missing = append(missing, "identity-public-key")
	}
	if c.IdentityLocation == "" {
		missing = append(missing, "identity-location")
	}
	if len(missing) != 0 {
		return fmt.Errorf("missing fields %s in config file", strings.Join(missing, ", "))
	}
	return nil
}

// Read reads a jem configuration file from the
// given path.
func Read(path string) (*Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, errgo.Notef(err, "cannot open config file")
	}
	defer f.Close()
	data, err := ioutil.ReadAll(f)
	if err != nil {
		return nil, errgo.Notef(err, "cannot read %q", path)
	}
	var conf Config
	err = yaml.Unmarshal(data, &conf)
	if err != nil {
		return nil, errgo.Notef(err, "cannot parse %q", path)
	}
	if err := conf.validate(); err != nil {
		return nil, errgo.Mask(err)
	}
	return &conf, nil
}
