// Copyright 2015 Canonical Ltd.

package config

import (
	"crypto/tls"
	"fmt"
	"io/ioutil"
	"os"
	"strings"

	"gopkg.in/errgo.v1"
	"gopkg.in/macaroon-bakery.v1/bakery"
	"gopkg.in/yaml.v2"

	"github.com/CanonicalLtd/jem/params"
	"github.com/juju/loggo"
)

var logger = loggo.GetLogger("jem.config")

type Config struct {
	MongoAddr string `yaml:"mongo-addr"`
	APIAddr   string `yaml:"api-addr"`
	// TODO rename state-server-admin to controller-admin.
	ControllerAdmin   params.User       `yaml:"state-server-admin"`
	IdentityPublicKey *bakery.PublicKey `yaml:"identity-public-key"`
	IdentityLocation  string            `yaml:"identity-location"`
	AgentUsername     string            `yaml:"agent-username"`
	AgentKey          *bakery.KeyPair   `yaml:"agent-key"`
	AccessLog         string            `yaml:"access-log"`
	TLSCert           string            `yaml:"tls-cert"`
	TLSKey            string            `yaml:"tls-key"`
	ControllerUUID    string            `yaml:"controller-uuid"`
	MaxMgoSessions    int               `yaml:"max-mgo-sessions"`
	GUILocation       string            `yaml:"gui-location"`
}

func (c *Config) validate() error {
	var missing []string
	if c.MongoAddr == "" {
		missing = append(missing, "mongo-addr")
	}
	if c.APIAddr == "" {
		missing = append(missing, "api-addr")
	}
	if c.ControllerAdmin == "" {
		missing = append(missing, "state-server-admin")
	}
	if c.IdentityLocation == "" {
		missing = append(missing, "identity-location")
	}
	if c.ControllerUUID == "" {
		missing = append(missing, "controller-uuid")
	}
	if len(missing) != 0 {
		return fmt.Errorf("missing fields %s in config file", strings.Join(missing, ", "))
	}
	return nil
}

func (c *Config) TLSConfig() *tls.Config {
	if c.TLSCert == "" || c.TLSKey == "" {
		return nil
	}

	cert, err := tls.X509KeyPair([]byte(c.TLSCert), []byte(c.TLSKey))
	if err != nil {
		logger.Errorf("cannot create certificate: %s", err)
		return nil
	}
	return &tls.Config{
		Certificates: []tls.Certificate{
			cert,
		},
	}
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
