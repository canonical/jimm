// Copyright 2015 Canonical Ltd.

package config

import (
	"crypto/tls"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/juju/loggo"
	"go.uber.org/zap/zapcore"
	"golang.org/x/crypto/acme"
	"golang.org/x/crypto/acme/autocert"
	"gopkg.in/errgo.v1"
	"gopkg.in/macaroon-bakery.v1/bakery"
	"gopkg.in/yaml.v2"

	"github.com/CanonicalLtd/jem/params"
)

var logger = loggo.GetLogger("jem.config")

type Config struct {
	MongoAddr string `yaml:"mongo-addr"`
	DBName    string `yaml:"dbname"`
	APIAddr   string `yaml:"api-addr"`
	// TODO rename state-server-admin to controller-admin.
	ControllerAdmin   params.User       `yaml:"state-server-admin"`
	IdentityPublicKey *bakery.PublicKey `yaml:"identity-public-key"`
	IdentityLocation  string            `yaml:"identity-location"`
	AgentUsername     string            `yaml:"agent-username"`
	AgentKey          *bakery.KeyPair   `yaml:"agent-key"`
	AccessLog         string            `yaml:"access-log"`
	Autocert          bool              `yaml:"autocert"`
	AutocertURL       string            `yaml:"autocert-url"`
	TLSCert           string            `yaml:"tls-cert"`
	TLSKey            string            `yaml:"tls-key"`
	ControllerUUID    string            `yaml:"controller-uuid"`
	MaxMgoSessions    int               `yaml:"max-mgo-sessions"`
	GUILocation       string            `yaml:"gui-location"`
	LoggingLevel      zapcore.Level     `yaml:"logging-level"`
	UsageSenderURL    string            `yaml:"usage-sender-url,omitempty"`
	Domain            string            `yaml:"domain"`
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

func (c *Config) TLSConfig() (*tls.Config, error) {
	if c.TLSCert == "" && c.TLSKey == "" && !c.Autocert {
		return nil, nil
	}
	if c.Autocert {
		autocertURL := acme.LetsEncryptURL
		if c.AutocertURL != "" {
			autocertURL = c.AutocertURL
		}
		if autocertURL == "staging" {
			autocertURL = "https://acme-staging.api.letsencrypt.org/directory"
		}
		logger.Infof("configuring autocert at %v", autocertURL)
		cacheDir := filepath.Join(os.TempDir(), "acme-cache")
		if err := os.MkdirAll(cacheDir, 0700); err != nil {
			return nil, errgo.Mask(err)
		}
		// TODO whitelist only some hosts (HostPolicy: autocert.HostWhitelist)
		m := autocert.Manager{
			Prompt: autocert.AcceptTOS,
			Cache:  autocert.DirCache(cacheDir),
			Client: &acme.Client{
				DirectoryURL: autocertURL,
			},
		}
		return &tls.Config{
			GetCertificate: func(clientHello *tls.ClientHelloInfo) (*tls.Certificate, error) {
				logger.Infof("getting certificate for server name %q", clientHello.ServerName)
				// Get the locally created certificate and whether it's appropriate
				// for the SNI name. If not, we'll try to get an acme cert and
				// fall back to the local certificate if that fails.
				if !strings.HasSuffix(clientHello.ServerName, ".acme.invalid") {
					// Allow all hosts currently.
				}
				acmeCert, err := m.GetCertificate(clientHello)
				if err == nil {
					return acmeCert, nil
				}
				logger.Infof("cannot get autocert certificate for %q: %v", clientHello.ServerName, err)
				return nil, err
			},
		}, nil
	}

	cert, err := tls.X509KeyPair([]byte(c.TLSCert), []byte(c.TLSKey))
	if err != nil {
		return nil, errgo.Notef(err, "cannot create certificate")
	}
	return &tls.Config{
		Certificates: []tls.Certificate{
			cert,
		},
	}, nil
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
