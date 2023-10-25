// Copyright 2015 Canonical Ltd.

package config

import (
	"crypto/tls"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/juju/loggo"
	"go.uber.org/zap/zapcore"
	"golang.org/x/crypto/acme"
	"golang.org/x/crypto/acme/autocert"
	"gopkg.in/errgo.v1"
	bakeryv2 "gopkg.in/macaroon-bakery.v2/bakery"
	"gopkg.in/yaml.v2"

	"github.com/canonical/jimm/params"
)

var (
	logger                   = loggo.GetLogger("jem.config")
	defaultPubsubConcurrency = 100
)

type Config struct {
	MongoAddr string `yaml:"mongo-addr"`
	DBName    string `yaml:"dbname"`
	APIAddr   string `yaml:"api-addr"`
	// TODO rename state-server-admin to controller-admin.
	ControllerAdmin       params.User       `yaml:"state-server-admin"`
	ControllerAdmins      []params.User     `yaml:"controller-admins"`
	IdentityPublicKey     *bakery.PublicKey `yaml:"identity-public-key"`
	IdentityLocation      string            `yaml:"identity-location"`
	CharmstoreLocation    string            `yaml:"charmstore-location"`
	MeteringLocation      string            `yaml:"metering-location"`
	AgentUsername         string            `yaml:"agent-username"`
	AgentKey              *bakeryv2.KeyPair `yaml:"agent-key"`
	AccessLog             string            `yaml:"access-log"`
	Autocert              bool              `yaml:"autocert"`
	AutocertURL           string            `yaml:"autocert-url"`
	TLSCert               string            `yaml:"tls-cert"`
	TLSKey                string            `yaml:"tls-key"`
	ControllerUUID        string            `yaml:"controller-uuid"`
	MaxMgoSessions        int               `yaml:"max-mgo-sessions"`
	GUILocation           string            `yaml:"gui-location"`
	LoggingLevel          zapcore.Level     `yaml:"logging-level"`
	Domain                string            `yaml:"domain"`
	PublicCloudMetadata   string            `yaml:"public-cloud-metadata"`
	MaxPubsubConcurrency  int               `yaml:"max-pubsub-concurrency"`
	JujuDashboardLocation string            `yaml:"juju-dashboard-location"`
	Vault                 VaultConfig       `yaml:"vault"`
	PublicDNSName         string            `yaml:"public-dns-name"`
}

// A VaultConfig contains the configuration settings for a vault server
// that will be used to store cloud credentials.
type VaultConfig struct {
	// Address is the address of the vault server.
	Address string `yaml:"address"`

	// ApprolePath is the path on the vault server of the approle
	// authentication service.
	ApprolePath string `yaml:"approle-path"`

	// ApproleRoleID contains the role_id to use for approle
	// authentication.
	ApproleRoleID string `yaml:"approle-role-id"`

	// ApproleSecretID contains the secret_id to use for approle
	// authentication.
	ApproleSecretID string `yaml:"approle-secret-id"`

	// KVPath is the root path of the KV store assigned to the JIMM
	// application.
	KVPath string `yaml:"kv-path"`
}

func (c *Config) validate() error {
	var missing []string
	if c.MongoAddr == "" {
		missing = append(missing, "mongo-addr")
	}
	if c.APIAddr == "" {
		missing = append(missing, "api-addr")
	}
	if c.ControllerAdmin == "" && len(c.ControllerAdmins) == 0 {
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
	if c.MaxPubsubConcurrency == 0 {
		c.MaxPubsubConcurrency = defaultPubsubConcurrency
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
